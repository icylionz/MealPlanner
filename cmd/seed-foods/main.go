// Command seed-foods populates a household's food catalog with raw ingredients
// from the live UK CoFID dataset (McCance and Widdowson, Public Health England).
//
// It targets the seeded "Starter Template" household by default, so every
// household created afterwards clones the enriched catalog. Running it is
// idempotent: foods whose name already exists in the target household are
// skipped, so it is safe to re-run to top up after a dataset refresh.
//
//	go run ./cmd/seed-foods                 # fetch live, seed template
//	go run ./cmd/seed-foods --file cofid.xlsx   # seed from a local copy
//	go run ./cmd/seed-foods --dry-run           # report counts, write nothing
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/cofid"
	"mealplanner/internal/config"
	"mealplanner/internal/database/db"
	"mealplanner/internal/platform/database"
)

// templateHouseholdID mirrors households.templateHouseholdID: the seeded starter
// household that new households clone (migration 0008).
var templateHouseholdID = uuid.MustParse("00000000-0000-0000-0000-0000000000ff")

func main() {
	var (
		fileFlag      = flag.String("file", "", "read the CoFID .xlsx from this local path instead of fetching gov.uk")
		urlFlag       = flag.String("url", cofid.DatasetURL, "CoFID dataset URL")
		householdFlag = flag.String("household", templateHouseholdID.String(), "target household id")
		dryRun        = flag.Bool("dry-run", false, "parse and report counts without writing")
	)
	flag.Parse()

	if err := run(*fileFlag, *urlFlag, *householdFlag, *dryRun); err != nil {
		log.Fatalf("seed-foods: %v", err)
	}
}

func run(file, url, household string, dryRun bool) error {
	householdID, err := uuid.Parse(household)
	if err != nil {
		return err
	}

	ctx := context.Background()

	var foods []cofid.Food
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		log.Printf("parsing local CoFID workbook %s", file)
		foods, err = cofid.FromBytes(raw)
		if err != nil {
			return err
		}
	} else {
		log.Printf("fetching live CoFID dataset from %s", url)
		foods, err = cofid.Load(ctx, url)
		if err != nil {
			return err
		}
	}
	log.Printf("parsed %d raw ingredients from CoFID", len(foods))

	if dryRun {
		log.Printf("dry run: no changes written")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Existing names in the target household, lowercased, so re-runs don't
	// duplicate and demo-seed ingredients (migration 0002) aren't shadowed.
	existing, err := existingNames(ctx, pool, householdID)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	inserted, skipped := 0, 0
	for _, f := range foods {
		if existing[strings.ToLower(f.Name)] {
			skipped++
			continue
		}
		created, err := q.CreateFood(ctx, db.CreateFoodParams{
			HouseholdID:   householdID,
			Name:          f.Name,
			DefaultUnit:   f.Unit,
			DensityGPerMl: f.Density,
			DensitySource: f.Source,
			Servings:      1,
		})
		if err != nil {
			return err
		}
		for _, tag := range f.Tags {
			if err := q.AddFoodTag(ctx, db.AddFoodTagParams{FoodID: created.ID, Tag: tag}); err != nil {
				return err
			}
		}
		existing[strings.ToLower(f.Name)] = true
		inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("done: %d inserted, %d skipped (already present) into household %s", inserted, skipped, householdID)
	return nil
}

func existingNames(ctx context.Context, pool *pgxpool.Pool, householdID uuid.UUID) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT lower(name) FROM foods WHERE household_id = $1`, householdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		set[name] = true
	}
	return set, rows.Err()
}
