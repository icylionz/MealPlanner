// Package households owns household member profiles and the active-profile
// session, matching the prototype's profile-switching model.
package households

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
)

// SessionTTL is how long a profile session stays valid.
const SessionTTL = 180 * 24 * time.Hour

// Prototype avatar palette, assigned round-robin to new members.
var memberColors = []string{"#22386A", "#1E8E5A", "#B57415", "#CE3B36", "#6B4EBF"}

// Member is a household profile.
type Member struct {
	ID       uuid.UUID
	Name     string
	Role     string
	Initials string
	Color    string
}

// Service owns household use cases.
type Service struct {
	q *db.Queries
}

// NewService constructs the households service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

// List returns all members in creation order.
func (s *Service) List(ctx context.Context) ([]Member, error) {
	rows, err := s.q.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(rows))
	for _, m := range rows {
		out = append(out, fromRow(m))
	}
	return out, nil
}

// Add creates a member profile from a full name.
func (s *Service) Add(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("member needs a name")
	}
	count, err := s.q.CountMembers(ctx)
	if err != nil {
		return err
	}
	_, err = s.q.CreateMember(ctx, db.CreateMemberParams{
		Name:     name,
		Role:     "member",
		Initials: initialsOf(name),
		Color:    memberColors[int(count)%len(memberColors)],
	})
	return err
}

// Remove deletes a non-owner member.
func (s *Service) Remove(ctx context.Context, id uuid.UUID) error {
	count, err := s.q.CountMembers(ctx)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errors.New("cannot remove the last member")
	}
	return s.q.DeleteMember(ctx, id)
}

// ResolveSession returns the member for a session token, or nil when the
// token is unknown or expired.
func (s *Service) ResolveSession(ctx context.Context, token string) (*Member, error) {
	if token == "" {
		return nil, nil
	}
	row, err := s.q.GetSessionMember(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := fromRow(row)
	return &m, nil
}

// StartSession creates a session bound to the given (or default owner) member
// and returns the new token.
func (s *Service) StartSession(ctx context.Context, memberID *uuid.UUID) (string, *Member, error) {
	var target db.HouseholdMember
	if memberID != nil {
		m, err := s.q.GetMember(ctx, *memberID)
		if err != nil {
			return "", nil, err
		}
		target = m
	} else {
		members, err := s.q.ListMembers(ctx)
		if err != nil {
			return "", nil, err
		}
		if len(members) == 0 {
			return "", nil, errors.New("no household members exist")
		}
		target = members[0]
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(buf)

	if err := s.q.CreateSession(ctx, db.CreateSessionParams{
		Token:     token,
		MemberID:  target.ID,
		ExpiresAt: time.Now().Add(SessionTTL),
	}); err != nil {
		return "", nil, err
	}
	m := fromRow(target)
	return token, &m, nil
}

// Switch repoints an existing session at another member profile.
func (s *Service) Switch(ctx context.Context, token string, memberID uuid.UUID) error {
	if _, err := s.q.GetMember(ctx, memberID); err != nil {
		return err
	}
	return s.q.UpdateSessionMember(ctx, db.UpdateSessionMemberParams{Token: token, MemberID: memberID})
}

func initialsOf(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		b.WriteString(strings.ToUpper(string([]rune(w)[0])))
		if b.Len() >= 2 {
			break
		}
	}
	if b.Len() == 0 {
		return "?"
	}
	return b.String()
}

func fromRow(m db.HouseholdMember) Member {
	return Member{ID: m.ID, Name: m.Name, Role: m.Role, Initials: m.Initials, Color: m.Color}
}
