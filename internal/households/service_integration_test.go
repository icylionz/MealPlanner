package households

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
)

func TestOwnershipRoleQueriesReportExactTransitions(t *testing.T) {
	fixture := newOwnershipFixture(t)
	ctx := t.Context()

	promoted, err := fixture.service.q.PromoteMemberToOwner(ctx, db.PromoteMemberToOwnerParams{
		HouseholdID: fixture.householdID, AccountID: fixture.targetID,
	})
	if err != nil || promoted != 1 {
		t.Fatalf("first promotion = (%d, %v), want (1, nil)", promoted, err)
	}
	promoted, err = fixture.service.q.PromoteMemberToOwner(ctx, db.PromoteMemberToOwnerParams{
		HouseholdID: fixture.householdID, AccountID: fixture.targetID,
	})
	if err != nil || promoted != 0 {
		t.Fatalf("repeated promotion = (%d, %v), want (0, nil)", promoted, err)
	}

	demoted, err := fixture.service.q.DemoteOwnerToMember(ctx, db.DemoteOwnerToMemberParams{
		HouseholdID: fixture.householdID, AccountID: fixture.targetID,
	})
	if err != nil || demoted != 1 {
		t.Fatalf("first demotion = (%d, %v), want (1, nil)", demoted, err)
	}
	demoted, err = fixture.service.q.DemoteOwnerToMember(ctx, db.DemoteOwnerToMemberParams{
		HouseholdID: fixture.householdID, AccountID: fixture.targetID,
	})
	if err != nil || demoted != 0 {
		t.Fatalf("repeated demotion = (%d, %v), want (0, nil)", demoted, err)
	}
}

func TestTransferOwnershipValidatesAndChangesExactlyOneOwner(t *testing.T) {
	fixture := newOwnershipFixture(t)
	ctx := t.Context()

	if err := fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.ownerID, fixture.ownerID); err == nil {
		t.Fatal("self-transfer unexpectedly succeeded")
	}
	if err := fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.ownerID, uuid.New()); err == nil {
		t.Fatal("transfer to a non-member unexpectedly succeeded")
	}
	if err := fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.targetID, fixture.otherID); err == nil {
		t.Fatal("transfer by a non-owner unexpectedly succeeded")
	}
	assertSoleOwner(t, fixture.pool, fixture.householdID, fixture.ownerID)

	if err := fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.ownerID, fixture.targetID); err != nil {
		t.Fatalf("transfer ownership: %v", err)
	}
	assertSoleOwner(t, fixture.pool, fixture.householdID, fixture.targetID)
}

func TestConcurrentOwnershipTransfersPreserveSingleOwner(t *testing.T) {
	fixture := newOwnershipFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	blocker, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, `SELECT id FROM households WHERE id = $1 FOR UPDATE`, fixture.householdID); err != nil {
		t.Fatal(err)
	}

	results := make(chan error, 2)
	go func() {
		results <- fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.ownerID, fixture.targetID)
	}()
	go func() {
		results <- fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.ownerID, fixture.otherID)
	}()
	waitForBlockedQueries(t, fixture.pool, "LockHousehold", 2)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	succeeded := 0
	for range 2 {
		if err := receiveResult(t, results); err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful concurrent transfers = %d, want 1", succeeded)
	}
	ownerID := soleOwner(t, fixture.pool, fixture.householdID)
	if ownerID != fixture.targetID && ownerID != fixture.otherID {
		t.Fatalf("sole owner = %s, want one of the transfer targets", ownerID)
	}
}

func TestTransferOwnershipLosesConcurrentTargetRemovalCleanly(t *testing.T) {
	fixture := newOwnershipFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	blocker, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err := blocker.Exec(ctx, `SELECT id FROM household_members WHERE household_id = $1 AND account_id = $2 FOR UPDATE`, fixture.householdID, fixture.targetID); err != nil {
		t.Fatal(err)
	}

	removeConn, err := fixture.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer removeConn.Release()
	removePID := removeConn.Conn().PgConn().PID()
	removeResult := make(chan error, 1)
	go func() {
		removeResult <- db.New(removeConn).RemoveMember(ctx, db.RemoveMemberParams{
			HouseholdID: fixture.householdID, AccountID: fixture.targetID,
		})
	}()
	waitForBackendLock(t, fixture.pool, removePID)

	transferResult := make(chan error, 1)
	go func() {
		transferResult <- fixture.service.TransferOwnership(ctx, fixture.householdID, fixture.ownerID, fixture.targetID)
	}()
	waitForBlockedQueries(t, fixture.pool, "LockHouseholdMembers", 1)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := receiveResult(t, removeResult); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if err := receiveResult(t, transferResult); err == nil || !strings.Contains(err.Error(), "not a member") {
		t.Fatalf("transfer after concurrent removal = %v, want target-not-member error", err)
	}
	assertSoleOwner(t, fixture.pool, fixture.householdID, fixture.ownerID)
}

type ownershipFixture struct {
	pool        *pgxpool.Pool
	service     *Service
	householdID uuid.UUID
	ownerID     uuid.UUID
	targetID    uuid.UUID
	otherID     uuid.UUID
}

func newOwnershipFixture(t *testing.T) ownershipFixture {
	t.Helper()
	databaseURL := os.Getenv("HOUSEHOLDS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HOUSEHOLDS_TEST_DATABASE_URL is not set")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	fixture := ownershipFixture{
		pool: pool, householdID: uuid.New(), ownerID: uuid.New(), targetID: uuid.New(), otherID: uuid.New(),
	}
	fixture.service = NewService(pool)
	accountIDs := []uuid.UUID{fixture.ownerID, fixture.targetID, fixture.otherID}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM households WHERE id = $1`, fixture.householdID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = ANY($1::uuid[])`, accountIDs)
	})
	for _, accountID := range accountIDs {
		if _, err := pool.Exec(ctx, `INSERT INTO accounts (id, email, password_hash, name) VALUES ($1, $2, 'test', $3)`, accountID, accountID.String()+"@g18.test", "G18 member"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO households (id, name, is_template) VALUES ($1, 'G18 test', false)`, fixture.householdID); err != nil {
		t.Fatal(err)
	}
	for i, accountID := range accountIDs {
		role := "member"
		if i == 0 {
			role = "owner"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO household_members (household_id, account_id, role, initials, color) VALUES ($1, $2, $3, 'G', '#000000')`, fixture.householdID, accountID, role); err != nil {
			t.Fatal(err)
		}
	}
	return fixture
}

func waitForBackendLock(t *testing.T, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	waitForDatabaseState(t, func() (bool, error) {
		var blocked bool
		err := pool.QueryRow(t.Context(), `SELECT COALESCE(wait_event_type = 'Lock', false) FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&blocked)
		return blocked, err
	})
}

func waitForBlockedQueries(t *testing.T, pool *pgxpool.Pool, queryName string, want int) {
	t.Helper()
	waitForDatabaseState(t, func() (bool, error) {
		var count int
		err := pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE $1`, "%-- name: "+queryName+" :%").Scan(&count)
		return count >= want, err
	})
}

func waitForDatabaseState(t *testing.T, ready func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ok, err := ready()
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for database lock state")
}

func receiveResult(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for database operation")
		return nil
	}
}

func assertSoleOwner(t *testing.T, pool *pgxpool.Pool, householdID, want uuid.UUID) {
	t.Helper()
	if got := soleOwner(t, pool, householdID); got != want {
		t.Fatalf("sole owner = %s, want %s", got, want)
	}
}

func soleOwner(t *testing.T, pool *pgxpool.Pool, householdID uuid.UUID) uuid.UUID {
	t.Helper()
	var ownerID uuid.UUID
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT min(account_id::text)::uuid, count(*) FROM household_members WHERE household_id = $1 AND role = 'owner'`, householdID).Scan(&ownerID, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("owner count = %d, want 1", count)
	}
	return ownerID
}
