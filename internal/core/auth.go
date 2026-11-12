package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
)

const sessionLifetime = 30 * 24 * time.Hour

// ErrThrottled means too many failed logins; try again later.
var ErrThrottled = errors.New("too many attempts")

// ErrBadLogin is deliberately the same for unknown emails and wrong passwords.
var ErrBadLogin = errors.New("invalid credentials")

type ThrottleConfig struct {
	Threshold int
	Window    time.Duration
	Block     time.Duration
}

type Auth struct {
	db       *db.DB
	metrics  *Metrics
	throttle ThrottleConfig

	mu       sync.Mutex
	attempts map[string]*attempt
}

type attempt struct {
	fails        []time.Time
	blockedUntil time.Time
}

func NewAuth(d *db.DB, m *Metrics, t ThrottleConfig) *Auth {
	return &Auth{db: d, metrics: m, throttle: t, attempts: map[string]*attempt{}}
}

// HouseholdRef is one household an account belongs to.
type HouseholdRef struct {
	ID   uuid.UUID
	Name string
	Role string
}

// Identity is the resolved caller for one request.
type Identity struct {
	Token      string // hashed session key
	Account    sqlc.Account
	Households []HouseholdRef
	Household  *HouseholdRef // active household, nil before onboarding
	Member     *sqlc.HouseholdMember
}

func (i *Identity) Owner() bool { return i.Household != nil && i.Household.Role == "owner" }

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || !strings.Contains(s, ".") || len(s) > 254 {
		return "", invalid("Enter a valid email address")
	}
	return strings.ToLower(s), nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func (a *Auth) Signup(ctx context.Context, name, email, password string) (sqlc.Account, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return sqlc.Account{}, invalid("Tell us what to call you")
	}
	email, err := normEmail(email)
	if err != nil {
		return sqlc.Account{}, err
	}
	if len(password) < 8 {
		return sqlc.Account{}, invalid("Use a password of at least eight characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return sqlc.Account{}, err
	}
	acc, err := a.db.Q.CreateAccount(ctx, sqlc.CreateAccountParams{Email: email, PasswordHash: string(hash), Name: name})
	if isUnique(err) {
		return sqlc.Account{}, invalid("That email already has an account. Try signing in")
	}
	return acc, err
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

// Login checks credentials, slowing repeated failures per IP and per email.
func (a *Auth) Login(ctx context.Context, email, password, ip string) (sqlc.Account, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	keys := []string{"ip:" + ip, "email:" + email}
	if a.blocked(keys) {
		a.metrics.Inc("login_attempts_total{result=\"throttled\"}")
		return sqlc.Account{}, ErrThrottled
	}
	acc, err := a.db.Q.GetAccountByEmail(ctx, email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Account{}, err
	}
	hash := dummyHash
	if err == nil {
		hash = []byte(acc.PasswordHash)
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil || err != nil {
		a.fail(keys)
		a.metrics.Inc("login_attempts_total{result=\"failure\"}")
		return sqlc.Account{}, ErrBadLogin
	}
	a.clear("email:" + email)
	a.metrics.Inc("login_attempts_total{result=\"success\"}")
	return acc, nil
}

func (a *Auth) blocked(keys []string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		if at := a.attempts[k]; at != nil && now.Before(at.blockedUntil) {
			return true
		}
	}
	return false
}

func (a *Auth) fail(keys []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		at := a.attempts[k]
		if at == nil {
			at = &attempt{}
			a.attempts[k] = at
		}
		recent := at.fails[:0]
		for _, t := range at.fails {
			if now.Sub(t) < a.throttle.Window {
				recent = append(recent, t)
			}
		}
		at.fails = append(recent, now)
		if len(at.fails) >= a.throttle.Threshold {
			at.blockedUntil = now.Add(a.throttle.Block)
			at.fails = nil
		}
	}
	// Keep the map from growing without bound.
	if len(a.attempts) > 10000 {
		for k, at := range a.attempts {
			if now.After(at.blockedUntil) && len(at.fails) == 0 {
				delete(a.attempts, k)
			}
		}
	}
}

func (a *Auth) clear(key string) {
	a.mu.Lock()
	delete(a.attempts, key)
	a.mu.Unlock()
}

// StartSession creates a session and returns the raw cookie token.
func (a *Auth) StartSession(ctx context.Context, account uuid.UUID) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	raw := hex.EncodeToString(buf)
	expires := time.Now().Add(sessionLifetime)
	_ = a.db.Q.DeleteExpiredSessions(ctx)
	err := a.db.Q.CreateSession(ctx, sqlc.CreateSessionParams{Token: hashToken(raw), AccountID: account, ExpiresAt: expires})
	return raw, expires, err
}

// Resolve turns a cookie token into the caller, their households and the
// active membership. Role is re-read every request, so a demotion or removal
// takes effect immediately.
func (a *Auth) Resolve(ctx context.Context, raw string) (*Identity, error) {
	if raw == "" {
		return nil, ErrNotFound
	}
	token := hashToken(raw)
	sess, err := a.db.Q.GetSession(ctx, token)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	acc, err := a.db.Q.GetAccount(ctx, sess.AccountID)
	if err != nil {
		return nil, err
	}
	rows, err := a.db.Q.ListHouseholdsForAccount(ctx, acc.ID)
	if err != nil {
		return nil, err
	}
	id := &Identity{Token: token, Account: acc}
	for _, r := range rows {
		id.Households = append(id.Households, HouseholdRef{ID: r.ID, Name: r.Name, Role: r.Role})
	}
	for i := range id.Households {
		if sess.ActiveHouseholdID != nil && id.Households[i].ID == *sess.ActiveHouseholdID {
			id.Household = &id.Households[i]
		}
	}
	if id.Household == nil && len(id.Households) > 0 {
		id.Household = &id.Households[0]
		if err := a.db.Q.SetSessionHousehold(ctx, sqlc.SetSessionHouseholdParams{Token: token, ActiveHouseholdID: &id.Household.ID}); err != nil {
			return nil, err
		}
	}
	if id.Household != nil {
		m, err := a.db.Q.GetMember(ctx, sqlc.GetMemberParams{HouseholdID: id.Household.ID, AccountID: acc.ID})
		if err != nil {
			return nil, err
		}
		id.Member = &m
	}
	return id, nil
}

func (a *Auth) Logout(ctx context.Context, raw string) error {
	return a.db.Q.DeleteSession(ctx, hashToken(raw))
}

// SwitchHousehold makes another of the caller's households active.
func (a *Auth) SwitchHousehold(ctx context.Context, id *Identity, hh uuid.UUID) error {
	if _, err := a.db.Q.GetMember(ctx, sqlc.GetMemberParams{HouseholdID: hh, AccountID: id.Account.ID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return a.db.Q.SetSessionHousehold(ctx, sqlc.SetSessionHouseholdParams{Token: id.Token, ActiveHouseholdID: &hh})
}

func (a *Auth) UpdateProfile(ctx context.Context, account uuid.UUID, name, email string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Your name can't be empty")
	}
	email, err := normEmail(email)
	if err != nil {
		return err
	}
	return a.db.Tx(ctx, func(q *sqlc.Queries) error {
		err := q.UpdateAccountProfile(ctx, sqlc.UpdateAccountProfileParams{ID: account, Name: name, Email: email})
		if isUnique(err) {
			return invalid("That email is already used by another account")
		}
		if err != nil {
			return err
		}
		return q.UpdateMemberInitials(ctx, sqlc.UpdateMemberInitialsParams{AccountID: account, Initials: Initials(name)})
	})
}

// ChangePassword verifies the current password and signs out other devices.
func (a *Auth) ChangePassword(ctx context.Context, id *Identity, current, next string) error {
	if bcrypt.CompareHashAndPassword([]byte(id.Account.PasswordHash), []byte(current)) != nil {
		return invalid("That isn't your current password")
	}
	if len(next) < 8 {
		return invalid("Use a new password of at least eight characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(next), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return a.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.UpdateAccountPassword(ctx, sqlc.UpdateAccountPasswordParams{ID: id.Account.ID, PasswordHash: string(hash)}); err != nil {
			return err
		}
		return q.DeleteOtherSessions(ctx, sqlc.DeleteOtherSessionsParams{AccountID: id.Account.ID, Token: id.Token})
	})
}

// Initials makes up to two initials from a display name.
func Initials(name string) string {
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
