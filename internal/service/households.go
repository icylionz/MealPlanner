package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
)

var ErrInviteInvalid = errors.New("this invite is invalid, expired, or has been revoked")

var memberColors = []string{"#22386A", "#1E8E5A", "#B57415", "#CE3B36", "#5874AE", "#3B4048"}

type HouseholdService struct {
	db      *database.DB
	metrics *observability.Metrics
}

func NewHouseholdService(d *database.DB, m *observability.Metrics) *HouseholdService {
	return &HouseholdService{db: d, metrics: m}
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		out = append(out, []rune(strings.ToUpper(w))[0])
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

// Create makes a household owned by the account and seeds it from the
// Starter Template (FR2.1).
func (s *HouseholdService) Create(ctx context.Context, account db.Account, name string) (db.Household, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.Household{}, invalid("Enter a household name.")
	}
	var h db.Household
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		var err error
		if h, err = q.CreateHousehold(ctx, name); err != nil {
			return err
		}
		if _, err = q.AddMember(ctx, db.AddMemberParams{
			HouseholdID: h.ID, AccountID: account.ID, Role: "owner", Initials: initials(account.Name), Color: memberColors[0],
		}); err != nil {
			return err
		}
		tpl, err := q.TemplateHouseholdID(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return cloneFoods(ctx, q, tpl, h.ID, &account.ID)
	})
	return h, err
}

// cloneFoods copies every active food (with lines, aliases, tags, steps) from
// one household into another under fresh IDs.
func cloneFoods(ctx context.Context, q *db.Queries, from, to uuid.UUID, by *uuid.UUID) error {
	cat, err := loadCatalog(ctx, q, from)
	if err != nil {
		return err
	}
	steps, err := q.ListHouseholdSteps(ctx, from)
	if err != nil {
		return err
	}
	ids := map[uuid.UUID]uuid.UUID{}
	for _, f := range cat.Active() {
		ids[f.ID] = uuid.New()
	}
	for _, f := range cat.Active() {
		if _, err := q.InsertFood(ctx, db.InsertFoodParams{
			ID: ids[f.ID], HouseholdID: to, Name: f.Name, Description: f.Description,
			PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin, Servings: f.Servings,
			YieldAmount: f.YieldAmount, DefaultUnit: f.DefaultUnit, DensityGPerMl: f.DensityGPerMl,
			DensitySource: f.DensitySource, SourceUrl: f.SourceUrl, SourceLastImportedAt: f.SourceLastImportedAt, CreatedBy: by,
		}); err != nil {
			return err
		}
		for _, a := range f.Aliases {
			if err := q.InsertFoodAlias(ctx, db.InsertFoodAliasParams{FoodID: ids[f.ID], Alias: a, CreatedBy: by}); err != nil {
				return err
			}
		}
		for _, t := range f.Tags {
			if err := q.InsertFoodTag(ctx, db.InsertFoodTagParams{FoodID: ids[f.ID], Tag: t}); err != nil {
				return err
			}
		}
	}
	for _, f := range cat.Active() {
		for _, cp := range f.Components {
			child, ok := ids[cp.ChildFoodID]
			if !ok {
				continue
			}
			if err := q.InsertFoodComponent(ctx, db.InsertFoodComponentParams{
				ID: uuid.New(), ParentFoodID: ids[f.ID], ChildFoodID: child, Amount: cp.Amount,
				Unit: cp.Unit, VariantText: cp.VariantText, SortOrder: cp.SortOrder,
			}); err != nil {
				return err
			}
		}
	}
	for _, st := range steps {
		if id, ok := ids[st.FoodID]; ok {
			if err := q.InsertFoodStep(ctx, db.InsertFoodStepParams{FoodID: id, StepNumber: st.StepNumber, Instruction: st.Instruction}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Join adds the account to the invite's household as a member (FR2.2-2.4).
func (s *HouseholdService) Join(ctx context.Context, account db.Account, code string) (uuid.UUID, error) {
	code = strings.TrimSpace(code)
	var householdID uuid.UUID
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		inv, err := q.GetInviteByCodeForUpdate(ctx, code)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInviteInvalid
		}
		if err != nil {
			return err
		}
		if inv.RevokedAt != nil || time.Now().After(inv.ExpiresAt) || (inv.MaxUses != nil && inv.UseCount >= *inv.MaxUses) {
			return ErrInviteInvalid
		}
		householdID = inv.HouseholdID
		if _, err := q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: inv.HouseholdID, AccountID: account.ID}); err == nil {
			return nil // already a member; nothing to consume
		}
		members, err := q.ListMembers(ctx, inv.HouseholdID)
		if err != nil {
			return err
		}
		if _, err := q.AddMember(ctx, db.AddMemberParams{
			HouseholdID: inv.HouseholdID, AccountID: account.ID, Role: "member",
			Initials: initials(account.Name), Color: memberColors[len(members)%len(memberColors)],
		}); err != nil {
			return err
		}
		return q.IncrementInviteUse(ctx, inv.ID)
	})
	if err == nil {
		s.metrics.Inc("mealplanner_invite_acceptances_total", "result", "accepted")
	} else if errors.Is(err, ErrInviteInvalid) {
		s.metrics.Inc("mealplanner_invite_acceptances_total", "result", "rejected")
	}
	return householdID, err
}

type HouseholdOverview struct {
	Household db.Household
	Members   []db.ListMembersRow
	Invites   []db.Invite
}

func (s *HouseholdService) Overview(ctx context.Context, a Actor) (HouseholdOverview, error) {
	h, err := s.db.Q.GetHousehold(ctx, a.HouseholdID)
	if err != nil {
		return HouseholdOverview{}, err
	}
	members, err := s.db.Q.ListMembers(ctx, a.HouseholdID)
	if err != nil {
		return HouseholdOverview{}, err
	}
	out := HouseholdOverview{Household: h, Members: members}
	if a.IsOwner() {
		if out.Invites, err = s.db.Q.ListInvites(ctx, a.HouseholdID); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *HouseholdService) Rename(ctx context.Context, a Actor, name string) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Enter a household name.")
	}
	return s.db.Q.RenameHousehold(ctx, db.RenameHouseholdParams{ID: a.HouseholdID, Name: name})
}

// CreateInvite issues a code valid for the given hours and optional use cap.
func (s *HouseholdService) CreateInvite(ctx context.Context, a Actor, hours int, maxUses *int32) (db.Invite, error) {
	if err := a.requireOwner(); err != nil {
		return db.Invite{}, err
	}
	if hours <= 0 || hours > 24*90 {
		return db.Invite{}, invalid("Expiry must be between 1 hour and 90 days.")
	}
	if maxUses != nil && *maxUses <= 0 {
		return db.Invite{}, invalid("Max uses must be positive.")
	}
	code, err := randomToken(9)
	if err != nil {
		return db.Invite{}, err
	}
	return s.db.Q.CreateInvite(ctx, db.CreateInviteParams{
		HouseholdID: a.HouseholdID, Code: code, ExpiresAt: time.Now().Add(time.Duration(hours) * time.Hour),
		MaxUses: maxUses, CreatedBy: a.ptr(),
	})
}

func (s *HouseholdService) RevokeInvite(ctx context.Context, a Actor, id uuid.UUID) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	n, err := s.db.Q.RevokeInvite(ctx, db.RevokeInviteParams{ID: id, HouseholdID: a.HouseholdID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// RemoveMember removes another member; their sessions lose access on the next
// request because membership is checked every time (FR3.4).
func (s *HouseholdService) RemoveMember(ctx context.Context, a Actor, accountID uuid.UUID) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	if accountID == a.AccountID {
		return invalid("Transfer ownership before leaving the household.")
	}
	n, err := s.db.Q.RemoveMember(ctx, db.RemoveMemberParams{HouseholdID: a.HouseholdID, AccountID: accountID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	if err == nil {
		err = s.db.Q.ClearHouseholdFromSessions(ctx, db.ClearHouseholdFromSessionsParams{AccountID: accountID, ActiveHouseholdID: &a.HouseholdID})
	}
	return err
}

// TransferOwnership promotes a member, optionally demoting the current owner,
// atomically (FR3.5).
func (s *HouseholdService) TransferOwnership(ctx context.Context, a Actor, to uuid.UUID, demoteSelf bool) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	if to == a.AccountID {
		return invalid("Choose another member.")
	}
	return s.db.Tx(ctx, func(q *db.Queries) error {
		n, err := q.SetMemberRole(ctx, db.SetMemberRoleParams{HouseholdID: a.HouseholdID, AccountID: to, Role: "owner"})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if demoteSelf {
			_, err = q.SetMemberRole(ctx, db.SetMemberRoleParams{HouseholdID: a.HouseholdID, AccountID: a.AccountID, Role: "member"})
		}
		return err
	})
}

// Leave removes the account from a household; the last owner cannot leave.
func (s *HouseholdService) Leave(ctx context.Context, a Actor) error {
	if a.IsOwner() {
		n, err := s.db.Q.CountOwners(ctx, a.HouseholdID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return invalid("You are the only owner. Transfer ownership before leaving.")
		}
	}
	_, err := s.db.Q.RemoveMember(ctx, db.RemoveMemberParams{HouseholdID: a.HouseholdID, AccountID: a.AccountID})
	return err
}
