package households

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestInviteValidationError(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Hour)
	limit := int32(2)

	tests := []struct {
		name string
		in   Invite
		want error
	}{
		{name: "valid", in: Invite{ExpiresAt: now.Add(time.Hour)}},
		{name: "revoked", in: Invite{ExpiresAt: now.Add(time.Hour), RevokedAt: &revokedAt}, want: ErrInviteRevoked},
		{name: "expired", in: Invite{ExpiresAt: now}, want: ErrInviteExpired},
		{name: "exhausted", in: Invite{ExpiresAt: now.Add(time.Hour), MaxUses: &limit, UseCount: limit}, want: ErrInviteExhausted},
		{name: "uses remain", in: Invite{ExpiresAt: now.Add(time.Hour), MaxUses: &limit, UseCount: limit - 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.validationError(now); !errors.Is(got, tt.want) {
				t.Fatalf("validationError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRandomInviteCodeHasUnbiasedHundredBitFormat(t *testing.T) {
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = byte(i)
	}
	code, err := randomInviteCode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if code != "ABCDE-FGHJK-LMNPQ-RSTUV" {
		t.Fatalf("code = %q", code)
	}
	if len(strings.ReplaceAll(code, "-", "")) != 20 {
		t.Fatalf("code has wrong symbol count: %q", code)
	}
}

func TestRandomInviteCodePropagatesReaderFailure(t *testing.T) {
	if _, err := randomInviteCode(io.LimitReader(bytes.NewReader(make([]byte, 19)), 19)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want unexpected EOF", err)
	}
}

func TestNormalizeInviteCodeAcceptsGroupedOrCompactEntry(t *testing.T) {
	want := "ABCDE-FGHJK-LMNPQ-RSTUV"
	for _, input := range []string{
		"abcde-fghjk-lmnpq-rstuv",
		"ABCDEFGHJKLMNPQRSTUV",
		"ABCDE FGHJK LMNPQ RSTUV",
	} {
		if got := normalizeInviteCode(input); got != want {
			t.Errorf("normalizeInviteCode(%q) = %q, want %q", input, got, want)
		}
	}
	if got := normalizeInviteCode(" legacy8 "); got != "LEGACY8" {
		t.Errorf("legacy code = %q", got)
	}
}

func TestUniqueInviteCodePropagatesRandomFailure(t *testing.T) {
	want := errors.New("random unavailable")
	svc := &Service{random: errorReader{err: want}}
	if _, err := svc.uniqueInviteCode(t.Context(), nil); !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped random error", err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func TestPasswordMatchesUsesDummyBcryptHash(t *testing.T) {
	if len(dummyPasswordHash) == 0 {
		t.Fatal("dummy bcrypt hash was not initialized")
	}
	if passwordMatches("", "not-the-dummy-password") {
		t.Error("dummy bcrypt comparison unexpectedly matched")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if !passwordMatches(string(hash), "correct-password") {
		t.Error("real bcrypt comparison did not match")
	}
}
