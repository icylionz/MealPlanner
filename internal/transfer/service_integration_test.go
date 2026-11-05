package transfer

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	foodservice "mealplanner/internal/foods"
)

// These focused tests are opt-in because they require a migrated PostgreSQL
// database. They exercise archive transaction behavior rather than mocking sqlc.
func TestFoodImportVersionAndGraphInvariants(t *testing.T) {
	databaseURL := os.Getenv("TRANSFER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TRANSFER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	t.Run("local versions invalidate stale editor and valid nesting imports", func(t *testing.T) {
		householdID := createImportTestHousehold(t, pool)
		parentID, childID, nestedID := uuid.New(), uuid.New(), uuid.New()
		if _, err := pool.Exec(ctx, `
			INSERT INTO foods (id, household_id, name, version)
			VALUES ($1, $3, 'Before import', 7), ($2, $3, 'Leaf', 3)`, parentID, childID, householdID); err != nil {
			t.Fatal(err)
		}

		now := time.Now().UTC()
		arc := &Archive{SchemaVersion: SchemaVersion, Foods: []Food{
			{
				ID: parentID, Name: "Imported parent", Servings: 2, DefaultUnit: "count",
				CreatedAt: now, UpdatedAt: now,
				Components: []Component{{ID: uuid.New(), ChildFoodID: nestedID, Amount: 1, Unit: "count"}},
			},
			{
				ID: nestedID, Name: "Imported nested", Servings: 1, DefaultUnit: "count",
				CreatedAt: now, UpdatedAt: now,
				Components: []Component{{ID: uuid.New(), ChildFoodID: childID, Amount: 1, Unit: "count"}},
			},
		}}
		report, err := NewService(pool).Import(ctx, householdID, arc, map[string]bool{SectionFoods: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Sections) != 1 || report.Sections[0].Updated != 1 || report.Sections[0].Inserted != 1 {
			t.Fatalf("food import report = %+v", report.Sections)
		}

		var parentName string
		var parentVersion, nestedVersion int
		if err := pool.QueryRow(ctx, `SELECT name, version FROM foods WHERE id = $1`, parentID).Scan(&parentName, &parentVersion); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `SELECT version FROM foods WHERE id = $1`, nestedID).Scan(&nestedVersion); err != nil {
			t.Fatal(err)
		}
		if parentName != "Imported parent" || parentVersion != 8 {
			t.Fatalf("updated food = %q version %d, want imported parent version 8", parentName, parentVersion)
		}
		if nestedVersion != 1 {
			t.Fatalf("inserted food version = %d, want local default 1", nestedVersion)
		}

		_, err = foodservice.NewService(pool).Save(ctx, householdID, uuid.Nil, &parentID, foodservice.Form{
			Name: "Stale overwrite", Servings: 1, DefaultUnit: "g", Version: 7,
		})
		if !errors.Is(err, foodservice.ErrConflict) {
			t.Fatalf("stale editor save error = %v, want conflict", err)
		}
		if err := pool.QueryRow(ctx, `SELECT name, version FROM foods WHERE id = $1`, parentID).Scan(&parentName, &parentVersion); err != nil {
			t.Fatal(err)
		}
		if parentName != "Imported parent" || parentVersion != 8 {
			t.Fatalf("stale editor changed imported food to %q version %d", parentName, parentVersion)
		}
	})

	for _, tc := range []struct {
		name  string
		foods func(time.Time) []Food
	}{
		{
			name: "direct cycle rolls back",
			foods: func(now time.Time) []Food {
				a := uuid.New()
				return []Food{{
					ID: a, Name: "Direct cycle", Servings: 1, DefaultUnit: "count", CreatedAt: now, UpdatedAt: now,
					Components: []Component{{ID: uuid.New(), ChildFoodID: a, Amount: 1, Unit: "count"}},
				}}
			},
		},
		{
			name: "multi-node cycle rolls back",
			foods: func(now time.Time) []Food {
				a, b, c := uuid.New(), uuid.New(), uuid.New()
				return []Food{
					{ID: a, Name: "A", Servings: 1, DefaultUnit: "count", CreatedAt: now, UpdatedAt: now, Components: []Component{{ID: uuid.New(), ChildFoodID: b, Amount: 1, Unit: "count"}}},
					{ID: b, Name: "B", Servings: 1, DefaultUnit: "count", CreatedAt: now, UpdatedAt: now, Components: []Component{{ID: uuid.New(), ChildFoodID: c, Amount: 1, Unit: "count"}}},
					{ID: c, Name: "C", Servings: 1, DefaultUnit: "count", CreatedAt: now, UpdatedAt: now, Components: []Component{{ID: uuid.New(), ChildFoodID: a, Amount: 1, Unit: "count"}}},
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			householdID := createImportTestHousehold(t, pool)
			arc := &Archive{SchemaVersion: SchemaVersion, Foods: tc.foods(time.Now().UTC())}
			_, err := NewService(pool).Import(ctx, householdID, arc, map[string]bool{SectionFoods: true})
			if !errors.Is(err, foodservice.ErrCycle) {
				t.Fatalf("cyclic archive error = %v, want clear cycle error", err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM foods WHERE household_id = $1`, householdID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("cyclic archive left %d foods after rollback", count)
			}
		})
	}
}

func createImportTestHousehold(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	householdID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO households (id, name, is_template) VALUES ($1, 'G15-G16 test', false)`, householdID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM households WHERE id = $1`, householdID)
	})
	return householdID
}
