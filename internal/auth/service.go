// Package auth owns login throttling and authentication orchestration.
package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"time"

	"mealplanner/internal/households"
)

const (
	DefaultThreshold     = 5
	DefaultWindow        = 10 * time.Minute
	DefaultBlockDuration = 15 * time.Minute
	DefaultMaxBuckets    = 10_000
)

// Config controls the in-memory login failure limiter.
type Config struct {
	Threshold     int
	Window        time.Duration
	BlockDuration time.Duration
	MaxBuckets    int
}

// DefaultConfig returns conservative defaults for the single-app deployment.
func DefaultConfig() Config {
	return Config{
		Threshold:     DefaultThreshold,
		Window:        DefaultWindow,
		BlockDuration: DefaultBlockDuration,
		MaxBuckets:    DefaultMaxBuckets,
	}
}

// Authenticator verifies an account identifier and password.
type Authenticator interface {
	Authenticate(context.Context, string, string) (*households.Account, error)
}

// BlockedError indicates that either the account or client network bucket is
// currently blocked.
type BlockedError struct {
	RetryAfter time.Duration
}

func (e *BlockedError) Error() string { return "too many login attempts" }

// Service applies throttling around the account authenticator.
type Service struct {
	auth    Authenticator
	limiter *limiter
	stripes [256]sync.Mutex
}

// NewService constructs a login service.
func NewService(authenticator Authenticator, cfg Config) *Service {
	return newService(authenticator, cfg, time.Now)
}

func newService(authenticator Authenticator, cfg Config, now func() time.Time) *Service {
	defaults := DefaultConfig()
	if cfg.Threshold <= 0 {
		cfg.Threshold = defaults.Threshold
	}
	if cfg.Window <= 0 {
		cfg.Window = defaults.Window
	}
	if cfg.BlockDuration <= 0 {
		cfg.BlockDuration = defaults.BlockDuration
	}
	if cfg.MaxBuckets <= 0 {
		cfg.MaxBuckets = defaults.MaxBuckets
	}
	return &Service{
		auth: authenticator,
		limiter: &limiter{
			threshold:     cfg.Threshold,
			window:        cfg.Window,
			blockDuration: cfg.BlockDuration,
			now:           now,
			accounts:      newBucketStore(cfg.MaxBuckets),
			clients:       newBucketStore(cfg.MaxBuckets),
		},
	}
}

// Login authenticates credentials unless either independent failure bucket is
// blocked. Only invalid credentials count as failures; infrastructure errors do
// not lock users out.
func (s *Service) Login(ctx context.Context, account, password, clientIP string) (*households.Account, error) {
	account = NormalizeAccount(account)
	clientIP = NormalizeClientIP(clientIP)
	unlock := s.lockIdentities(account, clientIP)
	defer unlock()
	accountKey := bucketKey(account)
	clientKey := bucketKey(clientIP)

	if retry := s.limiter.retryAfter(accountKey, clientKey); retry > 0 {
		return nil, &BlockedError{RetryAfter: retry}
	}
	acc, err := s.auth.Authenticate(ctx, account, password)
	if err != nil {
		if errors.Is(err, households.ErrInvalidCredentials) {
			s.limiter.failure(accountKey, clientKey)
			return nil, households.ErrInvalidCredentials
		}
		return nil, err
	}
	s.limiter.success(accountKey)
	return acc, nil
}

// lockIdentities serializes attempts sharing either bucket without imposing a
// global bcrypt lock. Fixed lock stripes keep this coordination bounded.
func (s *Service) lockIdentities(account, clientIP string) func() {
	a := stripeIndex("account:" + account)
	b := stripeIndex("client:" + clientIP)
	if a == b {
		s.stripes[a].Lock()
		return s.stripes[a].Unlock
	}
	if a > b {
		a, b = b, a
	}
	s.stripes[a].Lock()
	s.stripes[b].Lock()
	return func() {
		s.stripes[b].Unlock()
		s.stripes[a].Unlock()
	}
}

func stripeIndex(key string) uint8 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= 16777619
	}
	return uint8(hash)
}

func bucketKey(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return string(sum[:])
}

// NormalizeAccount gives equivalent email input one throttle identity.
func NormalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}

// NormalizeClientIP canonicalizes IPv4 addresses and groups IPv6 clients by
// /64, avoiding trivial address rotation within a typical client subnet.
func NormalizeClientIP(clientIP string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(clientIP))
	if err != nil {
		return "unknown"
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is6() {
		return netip.PrefixFrom(addr, 64).Masked().String()
	}
	return addr.String()
}

type limiter struct {
	mu            sync.Mutex
	threshold     int
	window        time.Duration
	blockDuration time.Duration
	now           func() time.Time
	nextCleanup   time.Time
	accounts      bucketStore
	clients       bucketStore
}

func (l *limiter) retryAfter(account, clientIP string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.cleanup(now)
	accountRetry := l.accounts.retryAfter(account, now)
	clientRetry := l.clients.retryAfter(clientIP, now)
	if accountRetry > clientRetry {
		return accountRetry
	}
	return clientRetry
}

func (l *limiter) failure(account, clientIP string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.cleanup(now)
	l.accounts.failure(account, now, l.threshold, l.window, l.blockDuration)
	l.clients.failure(clientIP, now, l.threshold, l.window, l.blockDuration)
}

func (l *limiter) cleanup(now time.Time) {
	if !l.nextCleanup.IsZero() && now.Before(l.nextCleanup) {
		return
	}
	l.accounts.cleanup(now, l.window)
	l.clients.cleanup(now, l.window)
	interval := min(l.window, time.Minute)
	l.nextCleanup = now.Add(interval)
}

func (l *limiter) success(account string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.accounts.success(account)
}

type failureBucket struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
}

func (b *failureBucket) retryAfter(now time.Time) time.Duration {
	if now.Before(b.blockedUntil) {
		return b.blockedUntil.Sub(now)
	}
	if !b.blockedUntil.IsZero() {
		*b = failureBucket{}
	}
	return 0
}

func (b *failureBucket) failure(now time.Time, threshold int, window, blockDuration time.Duration) {
	if b.retryAfter(now) > 0 {
		return
	}
	if b.windowStart.IsZero() || !now.Before(b.windowStart.Add(window)) {
		b.failures = 0
		b.windowStart = now
	}
	b.failures++
	if b.failures >= threshold {
		b.failures = 0
		b.windowStart = time.Time{}
		b.blockedUntil = now.Add(blockDuration)
	}
}

func (b *failureBucket) stale(now time.Time, window time.Duration) bool {
	if now.Before(b.blockedUntil) {
		return false
	}
	return b.windowStart.IsZero() || !now.Before(b.windowStart.Add(window))
}

type bucketEntry struct {
	failureBucket
	lastUsed uint64
}

// bucketStore bounds memory while retaining a separate bucket per identity.
// Its monotonic access sequence makes least-recently-used eviction deterministic
// even when the clock does not advance.
type bucketStore struct {
	buckets  map[string]*bucketEntry
	max      int
	sequence uint64
}

func newBucketStore(max int) bucketStore {
	return bucketStore{buckets: make(map[string]*bucketEntry), max: max}
}

func (s *bucketStore) existing(key string) *bucketEntry {
	b, ok := s.buckets[key]
	if !ok {
		return nil
	}
	s.touch(b)
	return b
}

func (s *bucketStore) retryAfter(key string, now time.Time) time.Duration {
	b := s.existing(key)
	if b == nil {
		return 0
	}
	return b.retryAfter(now)
}

func (s *bucketStore) failure(key string, now time.Time, threshold int, window, blockDuration time.Duration) {
	b := s.existing(key)
	if b == nil {
		if len(s.buckets) >= s.max {
			s.evictLRU()
		}
		b = &bucketEntry{}
		s.touch(b)
		s.buckets[key] = b
	}
	b.failure(now, threshold, window, blockDuration)
}

func (s *bucketStore) success(key string) {
	delete(s.buckets, key)
}

func (s *bucketStore) cleanup(now time.Time, window time.Duration) {
	for key, b := range s.buckets {
		if b.stale(now, window) {
			delete(s.buckets, key)
		}
	}
}

func (s *bucketStore) touch(b *bucketEntry) {
	s.sequence++
	b.lastUsed = s.sequence
}

func (s *bucketStore) evictLRU() {
	var oldestKey string
	var oldest *bucketEntry
	for key, b := range s.buckets {
		if oldest == nil || b.lastUsed < oldest.lastUsed {
			oldestKey, oldest = key, b
		}
	}
	if oldest != nil {
		delete(s.buckets, oldestKey)
	}
}
