package core

import (
	"context"
	"crypto/rand"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
)

// memberColours is the number of member colour tokens in the design system.
const memberColours = 8

type Households struct {
	db      *db.DB
	metrics *Metrics
}

func NewHouseholds(d *db.DB, m *Metrics) *Households { return &Households{db: d, metrics: m} }

// Create makes a household owned by the account, seeded from the Starter
// Template's foods, and returns its id.
func (s *Households) Create(ctx context.Context, account sqlc.Account, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, invalid("Give your household a name")
	}
	var id uuid.UUID
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		h, err := q.CreateHousehold(ctx, name)
		if err != nil {
			return err
		}
		id = h.ID
		if err := q.CreateMember(ctx, sqlc.CreateMemberParams{
			HouseholdID: h.ID, AccountID: account.ID, Role: "owner", Initials: Initials(account.Name), Color: "1",
		}); err != nil {
			return err
		}
		tpl, err := q.GetTemplateHousehold(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := q.CloneTemplateFoods(ctx, sqlc.CloneTemplateFoodsParams{HouseholdID: h.ID, TemplateID: tpl}); err != nil {
			return err
		}
		if err := q.CloneTemplateAliases(ctx, sqlc.CloneTemplateAliasesParams{HouseholdID: h.ID, TemplateID: tpl}); err != nil {
			return err
		}
		return q.CloneTemplateTags(ctx, sqlc.CloneTemplateTagsParams{HouseholdID: h.ID, TemplateID: tpl})
	})
	return id, err
}

// Join adds the account to the invite's household as a member.
func (s *Households) Join(ctx context.Context, account sqlc.Account, code string) (uuid.UUID, error) {
	code = strings.TrimSpace(code)
	if i := strings.LastIndex(code, "/join/"); i >= 0 { // a pasted invite link
		code = code[i+len("/join/"):]
	}
	var id uuid.UUID
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		hh, err := q.UseInvite(ctx, code)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("That invite isn't valid any more. Ask for a new one")
		}
		if err != nil {
			return err
		}
		id = hh
		if _, err := q.GetMember(ctx, sqlc.GetMemberParams{HouseholdID: hh, AccountID: account.ID}); err == nil {
			return invalid("You're already in that household")
		}
		n, err := q.CountMembers(ctx, hh)
		if err != nil {
			return err
		}
		return q.CreateMember(ctx, sqlc.CreateMemberParams{
			HouseholdID: hh, AccountID: account.ID, Role: "member", Initials: Initials(account.Name),
			Color: strconv.Itoa(int(n)%memberColours + 1),
		})
	})
	if err == nil {
		s.metrics.Inc("invite_acceptances_total")
	}
	return id, err
}

func (s *Households) Members(ctx context.Context, hh uuid.UUID) ([]sqlc.ListMembersRow, error) {
	return s.db.Q.ListMembers(ctx, hh)
}

func (s *Households) Invites(ctx context.Context, hh uuid.UUID) ([]sqlc.Invite, error) {
	return s.db.Q.ListInvites(ctx, hh)
}

func (s *Households) Rename(ctx context.Context, hh uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Give your household a name")
	}
	return s.db.Q.RenameHousehold(ctx, sqlc.RenameHouseholdParams{ID: hh, Name: name})
}

const inviteAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CreateInvite issues a code valid for the given days and optional use limit.
func (s *Households) CreateInvite(ctx context.Context, hh, actor uuid.UUID, days, maxUses int) error {
	if days < 1 || days > 90 {
		return invalid("Choose between 1 and 90 days")
	}
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	for i, b := range buf {
		buf[i] = inviteAlphabet[int(b)%len(inviteAlphabet)]
	}
	p := sqlc.CreateInviteParams{
		HouseholdID: hh, Code: string(buf), ExpiresAt: time.Now().Add(time.Duration(days) * 24 * time.Hour),
		CreatedBy: actorPtr(actor),
	}
	if maxUses > 0 {
		p.MaxUses = ptr(int32(maxUses))
	}
	_, err := s.db.Q.CreateInvite(ctx, p)
	return err
}

func (s *Households) RevokeInvite(ctx context.Context, hh, invite uuid.UUID) error {
	return s.db.Q.RevokeInvite(ctx, sqlc.RevokeInviteParams{ID: invite, HouseholdID: hh})
}

// RemoveMember drops a non-owner member; their sessions lose the household at once.
func (s *Households) RemoveMember(ctx context.Context, hh, member uuid.UUID) error {
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		acc, err := q.DeleteMember(ctx, sqlc.DeleteMemberParams{ID: member, HouseholdID: hh})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return q.ClearSessionsHousehold(ctx, sqlc.ClearSessionsHouseholdParams{AccountID: acc, ActiveHouseholdID: &hh})
	})
}

// TransferOwnership promotes the target member and demotes the current owner
// in one transaction, so the household always has exactly one owner.
func (s *Households) TransferOwnership(ctx context.Context, hh, current, target uuid.UUID) error {
	if current == target {
		return invalid("You're already the owner")
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		n, err := q.SetMemberRole(ctx, sqlc.SetMemberRoleParams{HouseholdID: hh, AccountID: target, Role: "owner"})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		_, err = q.SetMemberRole(ctx, sqlc.SetMemberRoleParams{HouseholdID: hh, AccountID: current, Role: "member"})
		return err
	})
}
