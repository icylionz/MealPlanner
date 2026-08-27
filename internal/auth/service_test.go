package auth

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"mealplanner/internal/households"
)

type fakeAuthenticator struct {
	calls       atomic.Int64
	lastAccount atomic.Value
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, account, password string) (*households.Account, error) {
	f.calls.Add(1)
	f.lastAccount.Store(account)
	if password == "correct" {
		return &households.Account{Email: account}, nil
	}
	return nil, households.ErrInvalidCredentials
}

func testService(authenticator Authenticator, cfg Config, now *time.Time) *Service {
	return newService(authenticator, cfg, func() time.Time { return *now })
}

func TestLoginThrottleNormalizesAndBlocksAccountIndependently(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 2, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	for _, attempt := range []struct{ account, ip string }{
		{" User@Example.COM ", "192.0.2.1"},
		{"user@example.com", "192.0.2.2"},
	} {
		if _, err := svc.Login(context.Background(), attempt.account, "wrong", attempt.ip); !errors.Is(err, households.ErrInvalidCredentials) {
			t.Fatalf("failed login error = %v", err)
		}
	}
	if got := fake.lastAccount.Load(); got != "user@example.com" {
		t.Fatalf("normalized account = %v", got)
	}
	if _, err := svc.Login(context.Background(), "USER@example.com", "wrong", "192.0.2.3"); blockedDuration(err) != 5*time.Minute {
		t.Fatalf("account block = %v, want 5m", err)
	}
	if fake.calls.Load() != 2 {
		t.Fatalf("authenticator calls = %d, want 2", fake.calls.Load())
	}
	if _, err := svc.Login(context.Background(), "other@example.com", "wrong", "192.0.2.3"); !errors.Is(err, households.ErrInvalidCredentials) {
		t.Fatalf("independent account/IP attempt = %v", err)
	}
}

func TestLoginThrottleBlocksClientIPAcrossAccounts(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 2, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	for _, account := range []string{"one@example.com", "two@example.com"} {
		_, _ = svc.Login(context.Background(), account, "wrong", "192.0.2.1")
	}
	if _, err := svc.Login(context.Background(), "three@example.com", "wrong", "192.0.2.1"); blockedDuration(err) != 5*time.Minute {
		t.Fatalf("client block = %v, want 5m", err)
	}
	if fake.calls.Load() != 2 {
		t.Fatalf("authenticator calls = %d, want 2", fake.calls.Load())
	}
}

func TestLoginThrottleWindowAndBlockExpiry(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 2, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	_, _ = svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1")
	now = now.Add(time.Minute)
	_, _ = svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1")
	_, _ = svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1")
	if _, err := svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1"); blockedDuration(err) != 5*time.Minute {
		t.Fatalf("block after reset window = %v", err)
	}
	now = now.Add(5 * time.Minute)
	if _, err := svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1"); !errors.Is(err, households.ErrInvalidCredentials) {
		t.Fatalf("login after block expiry = %v", err)
	}
	if fake.calls.Load() != 4 {
		t.Fatalf("authenticator calls = %d, want 4", fake.calls.Load())
	}
}

func TestLoginThrottleSuccessClearsOnlyNormalizedAccountBucket(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 2, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	_, _ = svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1")
	if _, err := svc.Login(context.Background(), " USER@example.com ", "correct", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.2"); !errors.Is(err, households.ErrInvalidCredentials) {
		t.Fatalf("first account failure after success = %v", err)
	}
	if _, err := svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.3"); !errors.Is(err, households.ErrInvalidCredentials) {
		t.Fatalf("second account failure after success = %v", err)
	}

	// The successful account must not erase failures accumulated by its client.
	if _, err := svc.Login(context.Background(), "other@example.com", "wrong", "192.0.2.1"); !errors.Is(err, households.ErrInvalidCredentials) {
		t.Fatalf("client threshold failure = %v", err)
	}
	if _, err := svc.Login(context.Background(), "fresh@example.com", "wrong", "192.0.2.1"); blockedDuration(err) != 5*time.Minute {
		t.Fatalf("client history was cleared by successful auth: %v", err)
	}
}

func TestLoginThrottleConcurrentFailures(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 1_000, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	const attempts = 200
	done := make(chan struct{}, attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			_, _ = svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1")
			done <- struct{}{}
		}()
	}
	for i := 0; i < attempts; i++ {
		<-done
	}
	if fake.calls.Load() != attempts {
		t.Fatalf("authenticator calls = %d, want %d", fake.calls.Load(), attempts)
	}
	svc.limiter.mu.Lock()
	accountFailures := svc.limiter.accounts.buckets[bucketKey("user@example.com")].failures
	clientFailures := svc.limiter.clients.buckets[bucketKey("192.0.2.1")].failures
	svc.limiter.mu.Unlock()
	if accountFailures != attempts || clientFailures != attempts {
		t.Fatalf("concurrent failures = account %d, client %d; want %d each", accountFailures, clientFailures, attempts)
	}
}

func TestLoginThrottleConcurrentBurstStopsAtThreshold(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 5, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	const attempts = 100
	done := make(chan struct{}, attempts)
	var blocked atomic.Int64
	for i := 0; i < attempts; i++ {
		go func() {
			_, err := svc.Login(context.Background(), "user@example.com", "wrong", "192.0.2.1")
			if blockedDuration(err) > 0 {
				blocked.Add(1)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < attempts; i++ {
		<-done
	}
	if fake.calls.Load() != 5 {
		t.Fatalf("authenticator calls = %d, want threshold 5", fake.calls.Load())
	}
	if blocked.Load() != attempts-5 {
		t.Fatalf("blocked attempts = %d, want %d", blocked.Load(), attempts-5)
	}
}

func TestLoginThrottleConcurrentIPv6PrefixCannotBypassClientStripe(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 5, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 20}, &now)

	const attempts = 100
	done := make(chan struct{}, attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			_, _ = svc.Login(context.Background(), fmt.Sprintf("user%d@example.com", i), "wrong", fmt.Sprintf("2001:db8:abcd:12::%x", i+1))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < attempts; i++ {
		<-done
	}
	if fake.calls.Load() != 5 {
		t.Fatalf("authenticator calls = %d, want client threshold 5", fake.calls.Load())
	}
}

func TestLoginThrottleBoundsAndCleansBuckets(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 1_000, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 3}, &now)

	for i := 0; i < 20; i++ {
		_, _ = svc.Login(context.Background(), fmt.Sprintf("user%d@example.com", i), "wrong", fmt.Sprintf("192.0.2.%d", i+1))
	}
	if got := len(svc.limiter.accounts.buckets); got != 3 {
		t.Fatalf("account bucket count = %d, want cap 3", got)
	}
	if got := len(svc.limiter.clients.buckets); got != 3 {
		t.Fatalf("client bucket count = %d, want cap 3", got)
	}
	if _, ok := svc.limiter.accounts.buckets[bucketKey("user19@example.com")]; !ok {
		t.Fatal("most recently used account was not retained")
	}
	if _, ok := svc.limiter.clients.buckets[bucketKey("192.0.2.20")]; !ok {
		t.Fatal("most recently used client was not retained")
	}

	now = now.Add(time.Minute)
	_, _ = svc.Login(context.Background(), "fresh@example.com", "wrong", "198.51.100.1")
	if len(svc.limiter.accounts.buckets) != 1 || len(svc.limiter.clients.buckets) != 1 {
		t.Fatalf("stale cleanup left account %d, client %d buckets", len(svc.limiter.accounts.buckets), len(svc.limiter.clients.buckets))
	}
}

func TestLoginThrottleCapacityNeverBlocksUnseenIdentity(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	fake := &fakeAuthenticator{}
	svc := testService(fake, Config{Threshold: 2, Window: time.Minute, BlockDuration: 5 * time.Minute, MaxBuckets: 1}, &now)

	_, _ = svc.Login(context.Background(), "first@example.com", "wrong", "192.0.2.1")
	_, _ = svc.Login(context.Background(), "second@example.com", "wrong", "192.0.2.2")
	if _, err := svc.Login(context.Background(), "unseen@example.com", "wrong", "192.0.2.3"); !errors.Is(err, households.ErrInvalidCredentials) {
		t.Fatalf("unseen identity inherited another bucket's block: %v", err)
	}
	if fake.calls.Load() != 3 {
		t.Fatalf("authenticator calls = %d, want 3", fake.calls.Load())
	}
}

func TestBucketStoreEvictsLeastRecentlyUsedDeterministically(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	store := newBucketStore(2)
	store.failure("a", now, 5, time.Minute, time.Minute)
	store.failure("b", now, 5, time.Minute, time.Minute)
	_ = store.retryAfter("a", now)
	store.failure("c", now, 5, time.Minute, time.Minute)

	if _, ok := store.buckets["b"]; ok {
		t.Fatal("least recently used bucket b was retained")
	}
	if _, ok := store.buckets["a"]; !ok {
		t.Fatal("recently touched bucket a was evicted")
	}
	if _, ok := store.buckets["c"]; !ok {
		t.Fatal("new bucket c was not inserted")
	}
}

func TestNormalizeClientIP(t *testing.T) {
	tests := map[string]string{
		"192.0.2.4":              "192.0.2.4",
		"::ffff:192.0.2.4":       "192.0.2.4",
		"2001:0db8:1234:5678::1": "2001:db8:1234:5678::/64",
		"2001:db8:1234:5678::9":  "2001:db8:1234:5678::/64",
		"invalid":                "unknown",
	}
	for in, want := range tests {
		if got := NormalizeClientIP(in); got != want {
			t.Errorf("NormalizeClientIP(%q) = %q, want %q", in, got, want)
		}
	}
}

func blockedDuration(err error) time.Duration {
	var blocked *BlockedError
	if errors.As(err, &blocked) {
		return blocked.RetryAfter
	}
	return 0
}
