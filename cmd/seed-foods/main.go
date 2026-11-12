// Command seed-foods enriches the Starter Template with raw ingredients from
// the UK CoFID 2021 dataset. Households created afterwards start with them.
// It is idempotent: foods already in the template, by name, are left alone.
//
//	seed-foods                 download from gov.uk and seed
//	seed-foods -file cofid.xlsx
//	seed-foods -dry-run        parse and report without writing
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/icylionz/mealplanner/internal/cofid"
	"github.com/icylionz/mealplanner/internal/config"
	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed-foods:", err)
		os.Exit(1)
	}
}

func run() error {
	file := flag.String("file", "", "local CoFID .xlsx (default: download from gov.uk)")
	url := flag.String("url", cofid.DatasetURL, "download URL")
	dry := flag.Bool("dry-run", false, "parse and report without writing")
	flag.Parse()

	ctx := context.Background()
	var data []byte
	var err error
	if *file != "" {
		data, err = os.ReadFile(*file)
	} else {
		fmt.Println("downloading", *url)
		data, err = cofid.Fetch(ctx, *url)
	}
	if err != nil {
		return err
	}
	all, err := cofid.Parse(data)
	if err != nil {
		return err
	}
	seeds := cofid.Select(all)
	fmt.Printf("parsed %d foods, %d raw ingredients\n", len(all), len(seeds))
	if *dry {
		return nil
	}

	config.LoadDotEnv(".env")
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if err := db.Migrate(dsn); err != nil {
		return err
	}
	database, err := db.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer database.Close()

	added := 0
	err = database.Tx(ctx, func(q *sqlc.Queries) error {
		tpl, err := q.GetTemplateHousehold(ctx)
		if err != nil {
			return fmt.Errorf("starter template not found: %w", err)
		}
		existing, err := q.ListFoods(ctx, tpl)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, f := range existing {
			have[strings.ToLower(f.Name)] = true
		}
		for _, s := range seeds {
			if have[strings.ToLower(s.Name)] {
				continue
			}
			id, err := q.CreateFood(ctx, sqlc.CreateFoodParams{
				HouseholdID: tpl, Name: s.Name, Servings: 1, DefaultUnit: "g", DensitySource: "none",
			})
			if err != nil {
				return err
			}
			if s.Alias != "" {
				if err := q.InsertAlias(ctx, sqlc.InsertAliasParams{FoodID: id, Alias: s.Alias}); err != nil {
					return err
				}
			}
			if s.Tag != "" {
				if err := q.InsertTag(ctx, sqlc.InsertTagParams{FoodID: id, Tag: s.Tag}); err != nil {
					return err
				}
			}
			added++
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("added %d foods to the Starter Template (%d already there)\n", added, len(seeds)-added)
	return nil
}
