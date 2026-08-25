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
	"golang.org/x/crypto/bcrypt"

	"mealplanner/internal/database/db"
)

// SessionTTL is how long a profile session stays valid.
const SessionTTL = 180 * 24 * time.Hour

// minPasswordLen is the shortest accepted password at registration.
const minPasswordLen = 8

// ErrInvalidCredentials is returned when an email/password pair does not match.
var ErrInvalidCredentials = errors.New("invalid email or password")

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

// EndSession invalidates a session token (logout). Unknown tokens are a no-op.
func (s *Service) EndSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, token)
}

// Register creates (or claims) a member profile with login credentials and
// returns it. If a passwordless member already exists with the same name it is
// claimed — its credentials are set — rather than creating a duplicate. The
// first member in an empty household becomes the owner.
func (s *Service) Register(ctx context.Context, name, email, password string) (*Member, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	switch {
	case name == "":
		return nil, errors.New("name is required")
	case !strings.Contains(email, "@") || strings.Contains(email, " "):
		return nil, errors.New("a valid email is required")
	case len(password) < minPasswordLen:
		return nil, errors.New("password must be at least 8 characters")
	}

	if _, err := s.q.GetMemberByEmail(ctx, email); err == nil {
		return nil, errors.New("that email is already registered")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	hashStr := string(hash)

	// Claim an existing passwordless member with the same name, if any.
	if claim, err := s.q.GetPasswordlessMemberByName(ctx, name); err == nil {
		row, err := s.q.SetMemberCredentials(ctx, db.SetMemberCredentialsParams{
			ID: claim.ID, Email: &email, PasswordHash: &hashStr,
		})
		if err != nil {
			return nil, err
		}
		m := fromRow(row)
		return &m, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	count, err := s.q.CountMembers(ctx)
	if err != nil {
		return nil, err
	}
	role := "member"
	if count == 0 {
		role = "owner"
	}
	row, err := s.q.CreateMemberWithAuth(ctx, db.CreateMemberWithAuthParams{
		Name:         name,
		Role:         role,
		Initials:     initialsOf(name),
		Color:        memberColors[int(count)%len(memberColors)],
		Email:        &email,
		PasswordHash: &hashStr,
	})
	if err != nil {
		return nil, err
	}
	m := fromRow(row)
	return &m, nil
}

// Authenticate verifies an email/password pair and returns the member. It
// returns ErrInvalidCredentials for any mismatch, without distinguishing an
// unknown email from a bad password.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*Member, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	row, err := s.q.GetMemberByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if row.PasswordHash == nil {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*row.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	m := fromRow(row)
	return &m, nil
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
