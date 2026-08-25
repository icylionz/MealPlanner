// Package households owns login accounts, households, membership and the
// active-household session. An account is the login identity; it belongs to one
// or more households, each of which owns its own foods, plans, grocery lists and
// prep sessions. The active household is tracked per session.
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

// SessionTTL is how long a session stays valid.
const SessionTTL = 180 * 24 * time.Hour

const minPasswordLen = 8

// templateHouseholdID is the seeded starter household new households clone.
var templateHouseholdID = uuid.MustParse("00000000-0000-0000-0000-0000000000ff")

// ErrInvalidCredentials is returned when an email/password pair does not match.
var ErrInvalidCredentials = errors.New("invalid email or password")

// ErrInviteNotFound is returned when an invite code matches no household.
var ErrInviteNotFound = errors.New("no household found for that invite code")

// Prototype avatar palette, assigned round-robin within a household.
var memberColors = []string{"#22386A", "#1E8E5A", "#B57415", "#CE3B36", "#6B4EBF"}

// Account is the login identity.
type Account struct {
	ID    uuid.UUID
	Name  string
	Email string
}

// Household is a tenant. Role/Initials/Color are populated relative to a given
// account when the household is listed for that account.
type Household struct {
	ID         uuid.UUID
	Name       string
	InviteCode string
	Role       string
	Initials   string
	Color      string
}

// Member is a household membership joined with its account identity, for the
// household roster and for the active-profile chrome.
type Member struct {
	AccountID uuid.UUID
	Name      string
	Email     string
	Role      string
	Initials  string
	Color     string
}

// SessionContext is the resolved session: the account plus its active household.
type SessionContext struct {
	Account           Account
	ActiveHouseholdID *uuid.UUID
	Token             string
}

// Service owns account, household and session use cases.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService constructs the households service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// Register creates a login account. It does not create or join a household —
// that happens next in onboarding.
func (s *Service) Register(ctx context.Context, name, email, password string) (*Account, error) {
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

	if _, err := s.q.GetAccountByEmail(ctx, email); err == nil {
		return nil, errors.New("that email is already registered")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	acc, err := s.q.CreateAccount(ctx, db.CreateAccountParams{
		Email: email, PasswordHash: string(hash), Name: name,
	})
	if err != nil {
		return nil, err
	}
	return &Account{ID: acc.ID, Name: acc.Name, Email: acc.Email}, nil
}

// Authenticate verifies an email/password pair and returns the account. It
// returns ErrInvalidCredentials for any mismatch.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*Account, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	acc, err := s.q.GetAccountByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return &Account{ID: acc.ID, Name: acc.Name, Email: acc.Email}, nil
}

// StartSession mints a session token for an account with an optional active
// household.
func (s *Service) StartSession(ctx context.Context, accountID uuid.UUID, activeHousehold *uuid.UUID) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	if err := s.q.CreateSession(ctx, db.CreateSessionParams{
		Token:             token,
		AccountID:         accountID,
		ActiveHouseholdID: activeHousehold,
		ExpiresAt:         time.Now().Add(SessionTTL),
	}); err != nil {
		return "", err
	}
	return token, nil
}

// ResolveSession returns the account and active household for a token, or nil
// when the token is unknown or expired.
func (s *Service) ResolveSession(ctx context.Context, token string) (*SessionContext, error) {
	if token == "" {
		return nil, nil
	}
	row, err := s.q.GetSessionAccount(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &SessionContext{
		Account:           Account{ID: row.ID, Name: row.Name, Email: row.Email},
		ActiveHouseholdID: row.ActiveHouseholdID,
		Token:             token,
	}, nil
}

// SetActiveHousehold repoints a session at another household the account belongs
// to. It verifies membership before switching.
func (s *Service) SetActiveHousehold(ctx context.Context, token string, accountID, householdID uuid.UUID) error {
	if _, err := s.q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: householdID, AccountID: accountID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("not a member of that household")
		}
		return err
	}
	return s.q.SetActiveHousehold(ctx, db.SetActiveHouseholdParams{Token: token, ActiveHouseholdID: &householdID})
}

// EndSession invalidates a session token (logout).
func (s *Service) EndSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.q.DeleteSession(ctx, token)
}

// ListForAccount returns the households an account belongs to.
func (s *Service) ListForAccount(ctx context.Context, accountID uuid.UUID) ([]Household, error) {
	rows, err := s.q.ListHouseholdsForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make([]Household, 0, len(rows))
	for _, h := range rows {
		out = append(out, Household{
			ID: h.ID, Name: h.Name, InviteCode: h.InviteCode,
			Role: h.Role, Initials: h.Initials, Color: h.Color,
		})
	}
	return out, nil
}

// GetHousehold returns a household by id (never the template).
func (s *Service) GetHousehold(ctx context.Context, id uuid.UUID) (*Household, error) {
	h, err := s.q.GetHousehold(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Household{ID: h.ID, Name: h.Name, InviteCode: h.InviteCode}, nil
}

// GetMembership returns an account's membership in a household, or nil if none.
func (s *Service) GetMembership(ctx context.Context, householdID, accountID uuid.UUID) (*Member, error) {
	m, err := s.q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: householdID, AccountID: accountID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	acc, err := s.q.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return &Member{
		AccountID: accountID, Name: acc.Name, Email: acc.Email,
		Role: m.Role, Initials: m.Initials, Color: m.Color,
	}, nil
}

// ListMembers returns the roster of a household.
func (s *Service) ListMembers(ctx context.Context, householdID uuid.UUID) ([]Member, error) {
	rows, err := s.q.ListHouseholdMembers(ctx, householdID)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(rows))
	for _, m := range rows {
		out = append(out, Member{
			AccountID: m.AccountID, Name: m.AccountName, Email: m.AccountEmail,
			Role: m.Role, Initials: m.Initials, Color: m.Color,
		})
	}
	return out, nil
}

// CreateHousehold creates a new household owned by the account, seeded with a
// clone of the starter food catalog, all in one transaction.
func (s *Service) CreateHousehold(ctx context.Context, accountID uuid.UUID, name string) (*Household, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("household needs a name")
	}
	acc, err := s.q.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)

	code, err := s.uniqueInviteCode(ctx, qtx)
	if err != nil {
		return nil, err
	}
	hh, err := qtx.CreateHousehold(ctx, db.CreateHouseholdParams{Name: name, InviteCode: code})
	if err != nil {
		return nil, err
	}
	if _, err := qtx.AddMember(ctx, db.AddMemberParams{
		HouseholdID: hh.ID, AccountID: accountID, Role: "owner",
		Initials: initialsOf(acc.Name), Color: memberColors[0],
	}); err != nil {
		return nil, err
	}
	if err := cloneTemplateFoods(ctx, qtx, hh.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Household{ID: hh.ID, Name: hh.Name, InviteCode: hh.InviteCode, Role: "owner"}, nil
}

// JoinByInvite adds the account to the household with the given invite code.
func (s *Service) JoinByInvite(ctx context.Context, accountID uuid.UUID, code string) (*Household, error) {
	code = strings.TrimSpace(code)
	hh, err := s.q.GetHouseholdByInvite(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.addMembership(ctx, s.q, hh.ID, accountID); err != nil {
		return nil, err
	}
	return &Household{ID: hh.ID, Name: hh.Name, InviteCode: hh.InviteCode}, nil
}

// AddMemberByEmail adds an existing account (looked up by email) to a household.
func (s *Service) AddMemberByEmail(ctx context.Context, householdID uuid.UUID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	acc, err := s.q.GetAccountByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("no account with that email — they must register first")
	}
	if err != nil {
		return err
	}
	if existing, err := s.q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: householdID, AccountID: acc.ID}); err == nil {
		_ = existing
		return errors.New("that person is already a member")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return s.addMembership(ctx, s.q, householdID, acc.ID)
}

// RemoveMember removes a non-owner member from a household.
func (s *Service) RemoveMember(ctx context.Context, householdID, accountID uuid.UUID) error {
	return s.q.RemoveMember(ctx, db.RemoveMemberParams{HouseholdID: householdID, AccountID: accountID})
}

// RegenerateInvite issues a fresh invite code for a household and returns it.
func (s *Service) RegenerateInvite(ctx context.Context, householdID uuid.UUID) (string, error) {
	code, err := s.uniqueInviteCode(ctx, s.q)
	if err != nil {
		return "", err
	}
	if err := s.q.RegenerateInviteCode(ctx, db.RegenerateInviteCodeParams{ID: householdID, InviteCode: code}); err != nil {
		return "", err
	}
	return code, nil
}

// addMembership inserts a member row with round-robin colour, idempotently.
func (s *Service) addMembership(ctx context.Context, q *db.Queries, householdID, accountID uuid.UUID) error {
	acc, err := q.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	count, err := q.CountHouseholdMembers(ctx, householdID)
	if err != nil {
		return err
	}
	_, err = q.AddMember(ctx, db.AddMemberParams{
		HouseholdID: householdID, AccountID: accountID, Role: "member",
		Initials: initialsOf(acc.Name), Color: memberColors[int(count)%len(memberColors)],
	})
	return err
}

// uniqueInviteCode returns a short human-friendly code not already in use.
func (s *Service) uniqueInviteCode(ctx context.Context, q *db.Queries) (string, error) {
	for i := 0; i < 10; i++ {
		code := randomInviteCode()
		if _, err := q.GetHouseholdByInvite(ctx, code); errors.Is(err, pgx.ErrNoRows) {
			return code, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("could not generate a unique invite code")
}

// cloneTemplateFoods copies the template household's food catalog (foods, tags,
// components, steps, densities) into a new household, remapping food ids.
func cloneTemplateFoods(ctx context.Context, q *db.Queries, householdID uuid.UUID) error {
	tmplFoods, err := q.ListFoods(ctx, templateHouseholdID)
	if err != nil {
		return err
	}
	tagRows, err := q.ListTagsForFoods(ctx, templateHouseholdID)
	if err != nil {
		return err
	}
	tagsByFood := map[uuid.UUID][]string{}
	for _, t := range tagRows {
		tagsByFood[t.FoodID] = append(tagsByFood[t.FoodID], t.Tag)
	}

	idMap := make(map[uuid.UUID]uuid.UUID, len(tmplFoods))
	for _, f := range tmplFoods {
		nf, err := q.CreateFood(ctx, db.CreateFoodParams{
			HouseholdID:   householdID,
			Name:          f.Name,
			Description:   f.Description,
			PrepTimeMin:   f.PrepTimeMin,
			CookTimeMin:   f.CookTimeMin,
			Servings:      f.Servings,
			DefaultUnit:   f.DefaultUnit,
			DensityGPerMl: f.DensityGPerMl,
			DensitySource: f.DensitySource,
		})
		if err != nil {
			return err
		}
		idMap[f.ID] = nf.ID
	}

	for oldID, newID := range idMap {
		for _, tag := range tagsByFood[oldID] {
			if err := q.AddFoodTag(ctx, db.AddFoodTagParams{FoodID: newID, Tag: tag}); err != nil {
				return err
			}
		}
		steps, err := q.ListFoodSteps(ctx, oldID)
		if err != nil {
			return err
		}
		for _, st := range steps {
			if err := q.AddFoodStep(ctx, db.AddFoodStepParams{FoodID: newID, StepNumber: st.StepNumber, Instruction: st.Instruction}); err != nil {
				return err
			}
		}
		comps, err := q.ListFoodComponents(ctx, oldID)
		if err != nil {
			return err
		}
		for _, c := range comps {
			child, ok := idMap[c.ChildFoodID]
			if !ok {
				continue
			}
			if err := q.AddFoodComponent(ctx, db.AddFoodComponentParams{
				ParentFoodID: newID, ChildFoodID: child,
				Amount: c.Amount, Unit: c.Unit, SortOrder: c.SortOrder,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func randomInviteCode() string {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // no I, L, O, 0, 1
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
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
