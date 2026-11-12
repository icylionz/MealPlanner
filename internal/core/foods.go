package core

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
	"github.com/icylionz/mealplanner/internal/units"
)

type Foods struct {
	db      *db.DB
	metrics *Metrics
}

func NewFoods(d *db.DB, m *Metrics) *Foods { return &Foods{db: d, metrics: m} }

// Catalog loads the household's food graph.
func (s *Foods) Catalog(ctx context.Context, hh uuid.UUID) (*Catalog, error) {
	return loadCatalog(ctx, s.db.Q, hh)
}

func (s *Foods) Steps(ctx context.Context, food uuid.UUID) ([]string, error) {
	rows, err := s.db.Q.ListSteps(ctx, food)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Instruction
	}
	return out, nil
}

// FoodInput is an attempted save from the food editor or an import review.
type FoodInput struct {
	Name        string
	Description string
	Tags        []string
	Aliases     []string
	Servings    float64
	PrepMin     int
	CookMin     int
	Yield       *float64
	Unit        string
	Density     *float64
	SourceURL   string
	// Imported stamps source_last_imported_at with the time of this save.
	Imported bool
	Lines    []LineInput
	Steps    []string
}

type LineInput struct {
	Amount  float64
	Unit    string
	Name    string
	Variant string
}

// Save creates (id nil) or updates a food. Component lines name other foods;
// names not in the catalog yet become new atomic foods. The returned warning
// is set when the recipe nests deeper than NestingWarnDepth.
func (s *Foods) Save(ctx context.Context, hh, actor uuid.UUID, id *uuid.UUID, version int32, in FoodInput) (uuid.UUID, string, error) {
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	if in.Name == "" {
		return uuid.Nil, "", invalid("Give the food a name")
	}
	if in.Servings <= 0 {
		in.Servings = 1
	}
	if in.PrepMin < 0 || in.CookMin < 0 {
		return uuid.Nil, "", invalid("Times can't be negative")
	}
	if in.Yield != nil && *in.Yield <= 0 {
		in.Yield = nil
	}
	if in.Density != nil && *in.Density <= 0 {
		in.Density = nil
	}
	in.Unit = units.Normalize(in.Unit)
	if !validURL(in.SourceURL) {
		return uuid.Nil, "", invalid("The source link must start with http:// or https://")
	}

	var saved uuid.UUID
	var warn string
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cat, err := loadCatalog(ctx, q, hh)
		if err != nil {
			return err
		}
		var existing *Food
		if id != nil {
			existing = cat.Get(*id)
			if existing == nil || existing.Deleted {
				return ErrNotFound
			}
			if existing.Version != version {
				return ErrConflict
			}
		}
		if other := cat.Find(in.Name); other != nil && (id == nil || other.ID != *id) && fold(other.Name) == fold(in.Name) {
			return invalid("You already have a food called %s", other.Name)
		}

		type resolved struct {
			child uuid.UUID
			line  LineInput
		}
		var lines []resolved
		created := map[string]uuid.UUID{}
		depth := 0
		for _, l := range in.Lines {
			l.Name = strings.Join(strings.Fields(l.Name), " ")
			if l.Name == "" {
				continue
			}
			if l.Amount <= 0 {
				return invalid("Give %s an amount above zero", l.Name)
			}
			l.Unit = units.Normalize(l.Unit)
			var child uuid.UUID
			if f := cat.Find(l.Name); f != nil {
				child = f.ID
				if id != nil && cat.Reaches(child, *id) {
					return invalid("%s already contains this recipe, so adding it would make a loop", f.Name)
				}
				if d := cat.Depth(child); d > depth {
					depth = d
				}
			} else if c, ok := created[fold(l.Name)]; ok {
				child = c
			} else {
				// A new ingredient: add it to the catalog so the line is never free text.
				unit := l.Unit
				if unit == "serving" || unit == "batch" {
					unit = ""
				}
				child, err = q.CreateFood(ctx, sqlc.CreateFoodParams{
					HouseholdID: hh, Name: l.Name, Servings: 1, DefaultUnit: unit, DensitySource: "none",
					CreatedBy: actorPtr(actor),
				})
				if err != nil {
					return err
				}
				created[fold(l.Name)] = child
			}
			lines = append(lines, resolved{child: child, line: l})
		}
		if len(lines) > 0 && depth+1 > NestingWarnDepth {
			warn = "This recipe nests other recipes more than three levels deep. It's saved, but amounts get harder to follow."
		}

		source := "none"
		if in.Density != nil {
			source = "custom"
			if existing != nil && existing.DensitySource == "starter" && existing.Density != nil && *existing.Density == *in.Density {
				source = "starter"
			}
		}
		var importedAt *time.Time
		if existing != nil {
			importedAt = existing.ImportedAt
		}
		if in.Imported {
			importedAt = ptr(time.Now())
		}

		if id == nil {
			saved, err = q.CreateFood(ctx, sqlc.CreateFoodParams{
				HouseholdID: hh, Name: in.Name, Description: in.Description, PrepTimeMin: int32(in.PrepMin),
				CookTimeMin: int32(in.CookMin), Servings: in.Servings, YieldAmount: in.Yield, DefaultUnit: in.Unit,
				DensityGPerMl: in.Density, DensitySource: source, SourceUrl: in.SourceURL,
				SourceLastImportedAt: importedAt, CreatedBy: actorPtr(actor),
			})
			if err != nil {
				return err
			}
		} else {
			saved = *id
			n, err := q.UpdateFood(ctx, sqlc.UpdateFoodParams{
				ID: saved, HouseholdID: hh, Version: version, Name: in.Name, Description: in.Description,
				PrepTimeMin: int32(in.PrepMin), CookTimeMin: int32(in.CookMin), Servings: in.Servings,
				YieldAmount: in.Yield, DefaultUnit: in.Unit, DensityGPerMl: in.Density, DensitySource: source,
				SourceUrl: in.SourceURL, SourceLastImportedAt: importedAt, UpdatedBy: actorPtr(actor),
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrConflict
			}
		}

		if err := q.DeleteComponents(ctx, saved); err != nil {
			return err
		}
		for i, l := range lines {
			if l.child == saved {
				return invalid("A recipe can't contain itself")
			}
			if err := q.InsertComponent(ctx, sqlc.InsertComponentParams{
				ParentFoodID: saved, ChildFoodID: l.child, Amount: l.line.Amount, Unit: l.line.Unit,
				VariantText: strings.TrimSpace(l.line.Variant), SortOrder: int32(i),
			}); err != nil {
				return err
			}
		}
		if err := q.DeleteSteps(ctx, saved); err != nil {
			return err
		}
		n := int32(0)
		for _, step := range in.Steps {
			if step = strings.TrimSpace(step); step == "" {
				continue
			}
			n++
			if err := q.InsertStep(ctx, sqlc.InsertStepParams{FoodID: saved, StepNumber: n, Instruction: step}); err != nil {
				return err
			}
		}
		if err := q.DeleteTags(ctx, saved); err != nil {
			return err
		}
		for _, t := range cleanList(in.Tags, true) {
			if err := q.InsertTag(ctx, sqlc.InsertTagParams{FoodID: saved, Tag: t}); err != nil {
				return err
			}
		}
		if err := q.DeleteAliases(ctx, saved); err != nil {
			return err
		}
		for _, a := range cleanList(in.Aliases, false) {
			if err := q.InsertAlias(ctx, sqlc.InsertAliasParams{FoodID: saved, Alias: a, CreatedBy: actorPtr(actor)}); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, ErrConflict) {
		s.metrics.Inc("edit_conflicts_total{entity=\"food\"}")
	}
	return saved, warn, err
}

// Delete soft-deletes a food; meals and lists that used it keep its name.
func (s *Foods) Delete(ctx context.Context, hh, actor, id uuid.UUID) error {
	n, err := s.db.Q.SoftDeleteFood(ctx, sqlc.SoftDeleteFoodParams{ID: id, HouseholdID: hh, UpdatedBy: actorPtr(actor)})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// cleanList trims, drops blanks and de-duplicates case-insensitively.
func cleanList(in []string, lower bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		v = strings.Join(strings.Fields(v), " ")
		if lower {
			v = strings.ToLower(v)
		}
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	return out
}

// SplitList splits a comma-separated form field.
func SplitList(s string) []string {
	return cleanList(strings.Split(s, ","), false)
}

func validURL(s string) bool {
	if s == "" {
		return true
	}
	return (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) && !strings.ContainsAny(s, " \t\r\n\"<>")
}

func noRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
