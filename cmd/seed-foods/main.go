// Command seed-foods enriches the Starter Template household with raw
// ingredients from CoFID 2021. It is optional and idempotent: foods already in
// the template (by case-insensitive name) are left alone. Households created
// afterwards clone the enriched catalog.
//
//	seed-foods                 # download the workbook from gov.uk
//	seed-foods -file cofid.xlsx
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/cofid"
	"github.com/icylionz/mealplanner/internal/config"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
)

func main() {
	file := flag.String("file", "", "local CoFID .xlsx (default: download from gov.uk)")
	url := flag.String("url", "", "override the download URL")
	dry := flag.Bool("dry-run", false, "parse and report without writing")
	flag.Parse()
	if err := run(*file, *url, *dry); err != nil {
		slog.Error("seed-foods failed", "err", err)
		os.Exit(1)
	}
}

func run(file, url string, dry bool) error {
	ctx := context.Background()
	var raw []byte
	var err error
	if file != "" {
		raw, err = os.ReadFile(file)
	} else {
		raw, err = cofid.Fetch(ctx, url)
	}
	if err != nil {
		return err
	}
	all, err := cofid.Parse(raw)
	if err != nil {
		return err
	}
	foods := cofid.Select(all)
	fmt.Printf("parsed %d CoFID rows, selected %d raw ingredients\n", len(all), len(foods))
	if dry {
		return nil
	}
	config.LoadDotEnv(".env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if err := database.Migrate(dsn); err != nil {
		return err
	}
	d, err := database.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer d.Close()
	added := 0
	err = d.Tx(ctx, func(q *db.Queries) error {
		tpl, err := q.TemplateHouseholdID(ctx)
		if err != nil {
			return fmt.Errorf("starter template missing: %w", err)
		}
		existing, err := q.ListFoods(ctx, tpl)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, f := range existing {
			have[strings.ToLower(f.Name)] = true
		}
		for _, f := range foods {
			if have[strings.ToLower(f.Name)] {
				continue
			}
			src := "none"
			if f.Density != nil {
				src = "starter"
			}
			id := uuid.New()
			if _, err := q.InsertFood(ctx, db.InsertFoodParams{
				ID: id, HouseholdID: tpl, Name: f.Name, Description: "CoFID " + f.Code, Servings: 1,
				DefaultUnit: f.Unit, DensityGPerMl: f.Density, DensitySource: src,
			}); err != nil {
				return err
			}
			if err := q.InsertFoodTag(ctx, db.InsertFoodTagParams{FoodID: id, Tag: "cofid"}); err != nil {
				return err
			}
			have[strings.ToLower(f.Name)] = true
			added++
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("added %d foods to the Starter Template\n", added)
	return nil
}
