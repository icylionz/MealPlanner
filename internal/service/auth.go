package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
)

const SessionTTL = 30 * 24 * time.Hour

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

var (
	ErrInvalidCredentials = errors.New("email or password is incorrect")
	ErrThrottled          = errors.New("too many sign-in attempts; try again later")
	ErrEmailTaken         = errors.New("that email is already in use")
)

type AuthService struct {
	db      *database.DB
	metrics *observability.Metrics
	limiter *Throttle
}

func NewAuthService(d *database.DB, m *observability.Metrics, t *Throttle) *AuthService {
	return &AuthService{db: d, metrics: m, limiter: t}
}

func normalizeEmail(e string) (string, error) {
	e = strings.TrimSpace(e)
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || !strings.Contains(e, "@") {
		return "", invalid("Enter a valid email address.")
	}
	return e, nil
}

func validatePassword(p string) error {
	if len(p) < 8 {
		return invalid("Password must be at least 8 characters.")
	}
	if len(p) > 72 {
		return invalid("Password must be at most 72 characters.")
	}
	return nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// Register creates an account (FR1.1).
func (s *AuthService) Register(ctx context.Context, name, email, password string) (db.Account, error) {
	name = strings.TrimSpace(name)
	var problems []string
	if name == "" {
		problems = append(problems, "Enter a display name.")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		problems = append(problems, err.Error())
	}
	if err := validatePassword(password); err != nil {
		problems = append(problems, err.Error())
	}
	if len(problems) > 0 {
		return db.Account{}, invalid(problems...)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return db.Account{}, err
	}
	acc, err := s.db.Q.CreateAccount(ctx, db.CreateAccountParams{Email: email, PasswordHash: string(hash), Name: name})
	if isUnique(err) {
		return db.Account{}, ErrEmailTaken
	}
	return acc, err
}

// Login verifies credentials with throttling (FR1.2-1.4).
func (s *AuthService) Login(ctx context.Context, email, password, ip string) (db.Account, error) {
	keys := []string{"ip:" + ip, "email:" + strings.ToLower(strings.TrimSpace(email))}
	if s.limiter.Blocked(keys...) {
		s.metrics.Inc("mealplanner_login_attempts_total", "result", "throttled")
		return db.Account{}, ErrThrottled
	}
	acc, err := s.db.Q.GetAccountByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		// Spend similar time as a real comparison to avoid an email oracle.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		s.limiter.Fail(keys...)
		s.metrics.Inc("mealplanner_login_attempts_total", "result", "failure")
		return db.Account{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(password)) != nil {
		s.limiter.Fail(keys...)
		s.metrics.Inc("mealplanner_login_attempts_total", "result", "failure")
		return db.Account{}, ErrInvalidCredentials
	}
	s.limiter.Reset(keys[1])
	s.metrics.Inc("mealplanner_login_attempts_total", "result", "success")
	return acc, nil
}

// StartSession creates a session, selecting the account's first household.
func (s *AuthService) StartSession(ctx context.Context, accountID uuid.UUID) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	var active *uuid.UUID
	if hs, err := s.db.Q.ListMembershipsForAccount(ctx, accountID); err == nil && len(hs) > 0 {
		id := hs[0].ID
		active = &id
	}
	err = s.db.Q.CreateSession(ctx, db.CreateSessionParams{
		Token: token, AccountID: accountID, ActiveHouseholdID: active, ExpiresAt: time.Now().Add(SessionTTL),
	})
	return token, err
}

// Principal is the resolved identity of a request.
type Principal struct {
	Token      string
	Account    db.Account
	Household  *db.Household
	Role       string
	Households []db.ListMembershipsForAccountRow
}

func (p *Principal) Actor() Actor {
	a := Actor{AccountID: p.Account.ID, Role: p.Role}
	if p.Household != nil {
		a.HouseholdID = p.Household.ID
	}
	return a
}

// Resolve loads the session, account and active household membership. A
// session whose active household no longer includes the account falls back to
// another membership, so removed members lose access immediately (FR3.4).
func (s *AuthService) Resolve(ctx context.Context, token string) (*Principal, error) {
	sess, err := s.db.Q.GetSession(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	acc, err := s.db.Q.GetAccount(ctx, sess.AccountID)
	if err != nil {
		return nil, err
	}
	hs, err := s.db.Q.ListMembershipsForAccount(ctx, acc.ID)
	if err != nil {
		return nil, err
	}
	p := &Principal{Token: token, Account: acc, Households: hs}
	var pick *db.ListMembershipsForAccountRow
	for i := range hs {
		if sess.ActiveHouseholdID != nil && hs[i].ID == *sess.ActiveHouseholdID {
			pick = &hs[i]
		}
	}
	if pick == nil && len(hs) > 0 {
		pick = &hs[0]
		_ = s.db.Q.SetSessionHousehold(ctx, db.SetSessionHouseholdParams{Token: token, ActiveHouseholdID: &pick.ID})
	}
	if pick != nil {
		h, err := s.db.Q.GetHousehold(ctx, pick.ID)
		if err != nil {
			return nil, err
		}
		p.Household = &h
		p.Role = pick.Role
	}
	return p, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	return s.db.Q.DeleteSession(ctx, token)
}

// SwitchHousehold changes the active household if the account belongs to it.
func (s *AuthService) SwitchHousehold(ctx context.Context, p *Principal, householdID uuid.UUID) error {
	if _, err := s.db.Q.GetMembership(ctx, db.GetMembershipParams{HouseholdID: householdID, AccountID: p.Account.ID}); err != nil {
		return ErrForbidden
	}
	return s.db.Q.SetSessionHousehold(ctx, db.SetSessionHouseholdParams{Token: p.Token, ActiveHouseholdID: &householdID})
}

// UpdateProfile changes display name and email (FR18.1).
func (s *AuthService) UpdateProfile(ctx context.Context, accountID uuid.UUID, name, email string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Enter a display name.")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	err = s.db.Q.UpdateAccountProfile(ctx, db.UpdateAccountProfileParams{ID: accountID, Name: name, Email: email})
	if isUnique(err) {
		return ErrEmailTaken
	}
	return err
}

// ChangePassword verifies the current password first (FR18.2).
func (s *AuthService) ChangePassword(ctx context.Context, accountID uuid.UUID, current, next string) error {
	acc, err := s.db.Q.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(acc.PasswordHash), []byte(current)) != nil {
		return invalid("Current password is incorrect.")
	}
	if err := validatePassword(next); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.Q.UpdateAccountPassword(ctx, db.UpdateAccountPasswordParams{ID: accountID, PasswordHash: string(hash)})
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Throttle is a minimal in-memory failed-login limiter keyed by IP and email.
type Throttle struct {
	mu        sync.Mutex
	threshold int
	window    time.Duration
	block     time.Duration
	entries   map[string]*throttleEntry
	now       func() time.Time
}

type throttleEntry struct {
	first, blockedUntil time.Time
	failures            int
}

func NewThrottle(threshold int, window, block time.Duration) *Throttle {
	return &Throttle{threshold: threshold, window: window, block: block, entries: map[string]*throttleEntry{}, now: time.Now}
}

func (t *Throttle) Blocked(keys ...string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for _, k := range keys {
		if e := t.entries[k]; e != nil && now.Before(e.blockedUntil) {
			return true
		}
	}
	return false
}

func (t *Throttle) Fail(keys ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for _, k := range keys {
		e := t.entries[k]
		if e == nil || now.Sub(e.first) > t.window {
			e = &throttleEntry{first: now}
			t.entries[k] = e
		}
		e.failures++
		if t.threshold > 0 && e.failures >= t.threshold {
			e.blockedUntil = now.Add(t.block)
			e.failures = 0
			e.first = now
		}
	}
	if len(t.entries) > 10000 {
		for k, e := range t.entries {
			if now.Sub(e.first) > t.window && now.After(e.blockedUntil) {
				delete(t.entries, k)
			}
		}
	}
}

func (t *Throttle) Reset(key string) {
	t.mu.Lock()
	delete(t.entries, key)
	t.mu.Unlock()
}
