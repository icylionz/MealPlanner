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
	"fmt"
	"io"
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

const (
	minPasswordLen = 8
	inviteTTL      = 7 * 24 * time.Hour
)

// templateHouseholdID is the seeded starter household new households clone.
var templateHouseholdID = uuid.MustParse("00000000-0000-0000-0000-0000000000ff")

// ErrInvalidCredentials is returned when an email/password pair does not match.
var ErrInvalidCredentials = errors.New("invalid email or password")

// dummyPasswordHash ensures an unknown account performs the same bcrypt work as
// a known account with a bad password.
var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("mealplanner-dummy-password"), bcrypt.DefaultCost)

var (
	// ErrInviteInvalid is returned when an invite code does not exist.
	ErrInviteInvalid = errors.New("that invite code is invalid")
	// ErrInviteExpired is returned when an invite is past its expiry time.
	ErrInviteExpired = errors.New("that invite code has expired")
	// ErrInviteRevoked is returned when an owner has revoked an invite.
	ErrInviteRevoked = errors.New("that invite code has been revoked")
	// ErrInviteExhausted is returned when an invite has reached its use limit.
	ErrInviteExhausted = errors.New("that invite code has reached its use limit")
)

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
	ID       uuid.UUID
	Name     string
	Role     string
	Initials string
	Color    string
}

// Invite is a share code and its lifecycle state.
type Invite struct {
	ID        uuid.UUID
	Code      string
	ExpiresAt time.Time
	RevokedAt *time.Time
	MaxUses   *int32
	UseCount  int32
	CreatedBy *uuid.UUID
	CreatedAt time.Time
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

// IsOwner reports whether the membership carries the owner role. Nil-safe so
// templates and guards can call it on an unresolved membership.
func (m *Member) IsOwner() bool {
	return m != nil && m.Role == "owner"
}

// SessionContext is the resolved session: the account plus its active household.
type SessionContext struct {
	Account           Account
	ActiveHouseholdID *uuid.UUID
	Token             string
}

// Service owns account, household and session use cases.
type Service struct {
	pool   *pgxpool.Pool
	q      *db.Queries
	random io.Reader
}

// NewService constructs the households service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool), random: rand.Reader}
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

// UpdateProfile changes an account's display name and email. Email uniqueness is
// enforced case-insensitively against other accounts.
func (s *Service) UpdateProfile(ctx context.Context, accountID uuid.UUID, name, email string) (*Account, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	switch {
	case name == "":
		return nil, errors.New("name is required")
	case !strings.Contains(email, "@") || strings.Contains(email, " "):
		return nil, errors.New("a valid email is required")
	}

	if existing, err := s.q.GetAccountByEmail(ctx, email); err == nil {
		if existing.ID != accountID {
			return nil, errors.New("that email is already registered")
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	acc, err := s.q.UpdateAccountProfile(ctx, db.UpdateAccountProfileParams{
		ID: accountID, Name: name, Email: email,
	})
	if err != nil {
		return nil, err
	}
	return &Account{ID: acc.ID, Name: acc.Name, Email: acc.Email}, nil
}

// ChangePassword verifies the current password then stores a new one. It returns
// ErrInvalidCredentials when the current password does not match.
func (s *Service) ChangePassword(ctx context.Context, accountID uuid.UUID, current, next string) error {
	acc, err := s.q.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(current)) != nil {
		return ErrInvalidCredentials
	}
	if len(next) < minPasswordLen {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.q.UpdateAccountPassword(ctx, db.UpdateAccountPasswordParams{
		ID: accountID, PasswordHash: string(hash),
	})
}

// Authenticate verifies an email/password pair and returns the account. It
// returns ErrInvalidCredentials for any mismatch.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*Account, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	acc, err := s.q.GetAccountByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = passwordMatches("", password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !passwordMatches(acc.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return &Account{ID: acc.ID, Name: acc.Name, Email: acc.Email}, nil
}

func passwordMatches(passwordHash, password string) bool {
	if passwordHash == "" {
		passwordHash = string(dummyPasswordHash)
	}
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
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
			ID: h.ID, Name: h.Name,
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
	return &Household{ID: h.ID, Name: h.Name}, nil
}

// GetLatestInvite returns the household's most recently created invite. It may
// be expired or revoked so the owner can see why no code is currently usable.
func (s *Service) GetLatestInvite(ctx context.Context, householdID uuid.UUID) (*Invite, error) {
	row, err := s.q.GetLatestInviteForHousehold(ctx, householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return inviteFromRow(row), nil
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

	hh, err := qtx.CreateHousehold(ctx, name)
	if err != nil {
		return nil, err
	}
	if _, err := qtx.AddMember(ctx, db.AddMemberParams{
		HouseholdID: hh.ID, AccountID: accountID, Role: "owner",
		Initials: initialsOf(acc.Name), Color: memberColors[0],
	}); err != nil {
		return nil, err
	}
	if _, err := s.createInvite(ctx, qtx, hh.ID, accountID); err != nil {
		return nil, err
	}
	if err := cloneTemplateFoods(ctx, qtx, hh.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Household{ID: hh.ID, Name: hh.Name, Role: "owner"}, nil
}

// JoinByInvite validates and consumes an invite under a row lock, then adds the
// membership and increments use_count in the same transaction.
func (s *Service) JoinByInvite(ctx context.Context, accountID uuid.UUID, code string) (*Household, error) {
	code = normalizeInviteCode(code)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)

	row, err := qtx.GetInviteByCodeForUpdate(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInviteInvalid
	}
	if err != nil {
		return nil, err
	}
	invite := inviteFromRow(row)
	if err := invite.validationError(time.Now()); err != nil {
		return nil, err
	}
	hh, err := qtx.GetHousehold(ctx, row.HouseholdID)
	if err != nil {
		return nil, err
	}
	if _, err := qtx.GetMembership(ctx, db.GetMembershipParams{HouseholdID: hh.ID, AccountID: accountID}); err == nil {
		return nil, errors.New("you are already a member of that household")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err := s.addMembership(ctx, qtx, hh.ID, accountID); err != nil {
		return nil, err
	}
	if err := qtx.IncrementInviteUse(ctx, row.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Household{ID: hh.ID, Name: hh.Name}, nil
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

// TransferOwnership promotes another member to owner and demotes the current
// owner to member, in one transaction (FR3 AC5). It verifies the caller is the
// household's owner and that the target is an existing non-owner member.
func (s *Service) TransferOwnership(ctx context.Context, householdID, fromAccountID, toAccountID uuid.UUID) error {
	if fromAccountID == toAccountID {
		return errors.New("you are already the owner")
	}

	from, err := s.q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: householdID, AccountID: fromAccountID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("you are not a member of this household")
		}
		return err
	}
	if from.Role != "owner" {
		return errors.New("only the owner can transfer ownership")
	}

	if _, err := s.q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: householdID, AccountID: toAccountID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("that person is not a member of this household")
		}
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)

	if err := qtx.SetMemberRole(ctx, db.SetMemberRoleParams{HouseholdID: householdID, AccountID: toAccountID, Role: "owner"}); err != nil {
		return err
	}
	if err := qtx.SetMemberRole(ctx, db.SetMemberRoleParams{HouseholdID: householdID, AccountID: fromAccountID, Role: "member"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RevokeInvite revokes the household's current invite, if any.
func (s *Service) RevokeInvite(ctx context.Context, householdID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)
	if _, err := qtx.LockHousehold(ctx, householdID); err != nil {
		return err
	}
	if err := qtx.RevokeActiveInvites(ctx, householdID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RegenerateInvite revokes the current invite and creates a fresh one in a
// transaction so a household never has two active codes.
func (s *Service) RegenerateInvite(ctx context.Context, householdID, createdBy uuid.UUID) (*Invite, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := db.New(tx)
	if _, err := qtx.LockHousehold(ctx, householdID); err != nil {
		return nil, err
	}
	if err := qtx.RevokeActiveInvites(ctx, householdID); err != nil {
		return nil, err
	}
	invite, err := s.createInvite(ctx, qtx, householdID, createdBy)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return invite, nil
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

// createInvite creates a short-lived, unlimited-use invite. max_uses remains
// nullable in the schema so limited-use invites can be introduced without a
// migration or changes to acceptance semantics.
func (s *Service) createInvite(ctx context.Context, q *db.Queries, householdID, createdBy uuid.UUID) (*Invite, error) {
	code, err := s.uniqueInviteCode(ctx, q)
	if err != nil {
		return nil, err
	}
	row, err := q.CreateInvite(ctx, db.CreateInviteParams{
		HouseholdID: householdID,
		Code:        code,
		ExpiresAt:   time.Now().Add(inviteTTL),
		CreatedBy:   &createdBy,
	})
	if err != nil {
		return nil, err
	}
	return inviteFromRow(row), nil
}

// uniqueInviteCode returns a short human-friendly code not already in use.
func (s *Service) uniqueInviteCode(ctx context.Context, q *db.Queries) (string, error) {
	for i := 0; i < 10; i++ {
		code, err := randomInviteCode(s.random)
		if err != nil {
			return "", fmt.Errorf("generate invite code: %w", err)
		}
		exists, err := q.InviteCodeExists(ctx, code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", errors.New("could not generate a unique invite code")
}

func inviteFromRow(row db.Invite) *Invite {
	invite := &Invite{
		ID: row.ID, Code: row.Code, ExpiresAt: row.ExpiresAt,
		MaxUses: row.MaxUses, UseCount: row.UseCount, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt,
	}
	if row.RevokedAt.Valid {
		revokedAt := row.RevokedAt.Time
		invite.RevokedAt = &revokedAt
	}
	return invite
}

func (i Invite) validationError(now time.Time) error {
	switch {
	case i.RevokedAt != nil:
		return ErrInviteRevoked
	case !now.Before(i.ExpiresAt):
		return ErrInviteExpired
	case i.MaxUses != nil && i.UseCount >= *i.MaxUses:
		return ErrInviteExhausted
	default:
		return nil
	}
}

// IsUsable reports whether the invite currently permits a join.
func (i Invite) IsUsable() bool { return i.validationError(time.Now()) == nil }

// cloneTemplateFoods copies the template household's food catalog (foods, tags,
// components, steps, aliases, densities, and source metadata) into a new
// household, remapping food ids.
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
	aliasRows, err := q.ListAliasesForFoods(ctx, templateHouseholdID)
	if err != nil {
		return err
	}
	aliasesByFood := map[uuid.UUID][]string{}
	for _, alias := range aliasRows {
		aliasesByFood[alias.FoodID] = append(aliasesByFood[alias.FoodID], alias.Alias)
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
		if f.SourceUrl != nil {
			if err := q.SetFoodSourceMetadata(ctx, db.SetFoodSourceMetadataParams{
				ID: nf.ID, SourceUrl: f.SourceUrl,
				SourceLastImportedAt: f.SourceLastImportedAt,
			}); err != nil {
				return err
			}
		}
		idMap[f.ID] = nf.ID
	}

	for oldID, newID := range idMap {
		for _, tag := range tagsByFood[oldID] {
			if err := q.AddFoodTag(ctx, db.AddFoodTagParams{FoodID: newID, Tag: tag}); err != nil {
				return err
			}
		}
		for _, alias := range aliasesByFood[oldID] {
			if err := q.AddFoodAlias(ctx, db.AddFoodAliasParams{FoodID: newID, Alias: alias}); err != nil {
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
				Amount: c.Amount, Unit: c.Unit, VariantText: c.VariantText,
				SortOrder: c.SortOrder,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func randomInviteCode(random io.Reader) (string, error) {
	// Exactly 32 symbols makes each random byte's low five bits unbiased. Twenty
	// symbols provide 100 bits of entropy; separators keep the code readable.
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, 20)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	code := make([]byte, 0, 23)
	for i, b := range raw {
		if i > 0 && i%5 == 0 {
			code = append(code, '-')
		}
		code = append(code, alphabet[int(b&31)])
	}
	return string(code), nil
}

func normalizeInviteCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	compact := strings.NewReplacer("-", "", " ", "").Replace(code)
	if len(compact) != 20 {
		return code
	}
	return compact[:5] + "-" + compact[5:10] + "-" + compact[10:15] + "-" + compact[15:]
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
