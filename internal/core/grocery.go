package core

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/db"
	"github.com/icylionz/mealplanner/internal/db/sqlc"
	"github.com/icylionz/mealplanner/internal/units"
)

const generatedListName = "Generated list"

type Grocery struct {
	db      *db.DB
	metrics *Metrics
}

func NewGrocery(d *db.DB, m *Metrics) *Grocery { return &Grocery{db: d, metrics: m} }

func (s *Grocery) Lists(ctx context.Context, hh uuid.UUID) ([]sqlc.ListGroceryListsRow, error) {
	return s.db.Q.ListGroceryLists(ctx, hh)
}

func (s *Grocery) List(ctx context.Context, hh, id uuid.UUID) (sqlc.GroceryList, []sqlc.ListGroceryItemsRow, error) {
	list, err := s.db.Q.GetGroceryList(ctx, sqlc.GetGroceryListParams{ID: id, HouseholdID: hh})
	if noRows(err) {
		return list, nil, ErrNotFound
	}
	if err != nil {
		return list, nil, err
	}
	items, err := s.db.Q.ListGroceryItems(ctx, id)
	return list, items, err
}

func (s *Grocery) CreateList(ctx context.Context, hh, actor uuid.UUID, name string) (uuid.UUID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, invalid("Give the list a name")
	}
	return s.db.Q.CreateGroceryList(ctx, sqlc.CreateGroceryListParams{HouseholdID: hh, Name: name, CreatedBy: actorPtr(actor)})
}

func (s *Grocery) RenameList(ctx context.Context, hh, actor, id uuid.UUID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Give the list a name")
	}
	return s.db.Q.RenameGroceryList(ctx, sqlc.RenameGroceryListParams{ID: id, HouseholdID: hh, Name: name, UpdatedBy: actorPtr(actor)})
}

func (s *Grocery) DeleteList(ctx context.Context, hh, actor, id uuid.UUID) error {
	return s.db.Q.SoftDeleteGroceryList(ctx, sqlc.SoftDeleteGroceryListParams{ID: id, HouseholdID: hh, UpdatedBy: actorPtr(actor)})
}

// AddItem adds a hand-written line. A name matching a catalog food links to it
// so its cost can feed estimates.
func (s *Grocery) AddItem(ctx context.Context, hh, actor, list uuid.UUID, name string, amount float64, unit string) error {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return invalid("What do you need? Type an item name")
	}
	if amount < 0 {
		return invalid("The amount can't be negative")
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if _, err := q.GetGroceryList(ctx, sqlc.GetGroceryListParams{ID: list, HouseholdID: hh}); err != nil {
			if noRows(err) {
				return ErrNotFound
			}
			return err
		}
		cat, err := loadCatalog(ctx, q, hh)
		if err != nil {
			return err
		}
		p := sqlc.CreateGroceryItemParams{
			ListID: list, Name: name, Amount: amount, Unit: units.Normalize(unit), SourceType: "adhoc", CreatedBy: actorPtr(actor),
		}
		if f := cat.Find(name); f != nil {
			p.IngredientID, p.Name = &f.ID, f.Name
			if amount > 0 && p.Unit == "" {
				p.Unit = f.Unit
			}
		}
		_, err = q.CreateGroceryItem(ctx, p)
		return err
	})
}

// Source is one contribution to a grocery line: why it is on the list.
type Source struct {
	Meal     string // meal title or primary food; "" when not from a meal
	MealDate *time.Time
	Recipe   string
	Amount   float64
	Unit     string
	Variant  string
}

func (s *Grocery) Item(ctx context.Context, hh, id uuid.UUID) (sqlc.GetGroceryItemRow, []Source, error) {
	item, err := s.db.Q.GetGroceryItem(ctx, sqlc.GetGroceryItemParams{ID: id, HouseholdID: hh})
	if noRows(err) {
		return item, nil, ErrNotFound
	}
	if err != nil {
		return item, nil, err
	}
	rows, err := s.db.Q.ListGroceryItemSources(ctx, id)
	if err != nil {
		return item, nil, err
	}
	out := make([]Source, len(rows))
	for i, r := range rows {
		src := Source{MealDate: r.PlanDate, Recipe: r.LineRecipeName, Amount: r.QuantityContributed, Unit: r.UnitContributed, Variant: r.VariantText}
		if src.Recipe == "" && r.RecipeName != nil {
			src.Recipe = *r.RecipeName
		}
		if r.MealTitle != nil && *r.MealTitle != "" {
			src.Meal = *r.MealTitle
		} else if r.MealFoodName != nil {
			src.Meal = *r.MealFoodName
		}
		out[i] = src
	}
	return item, out, nil
}

// UpdateItem edits a line on its list only; source foods and meals are untouched.
func (s *Grocery) UpdateItem(ctx context.Context, hh, actor, id uuid.UUID, amount float64, unit, note string, cost *float64) error {
	if amount < 0 {
		return invalid("The amount can't be negative")
	}
	if cost != nil && *cost < 0 {
		return invalid("The cost can't be negative")
	}
	unit = units.Normalize(unit)
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetGroceryItem(ctx, sqlc.GetGroceryItemParams{ID: id, HouseholdID: hh})
		if noRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// The cost date moves only when what was paid, or what it paid for, changes.
		recorded := cur.CostRecordedAt
		switch {
		case cost == nil:
			recorded = nil
		case cur.Cost == nil || *cur.Cost != *cost || cur.Amount != amount || cur.Unit != unit || recorded == nil:
			recorded = ptr(time.Now())
		}
		return q.UpdateGroceryItem(ctx, sqlc.UpdateGroceryItemParams{
			ID: id, Amount: amount, Unit: unit, Note: strings.TrimSpace(note), Cost: cost,
			CostRecordedAt: recorded, UpdatedBy: actorPtr(actor),
		})
	})
}

// ConvertItem re-expresses a line in another unit, using the food's density
// for volume to weight. It refuses rather than guesses.
func (s *Grocery) ConvertItem(ctx context.Context, hh, actor, id uuid.UUID, to string) error {
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetGroceryItem(ctx, sqlc.GetGroceryItemParams{ID: id, HouseholdID: hh})
		if noRows(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var density *float64
		if cur.IngredientID != nil {
			cat, err := loadCatalog(ctx, q, hh)
			if err != nil {
				return err
			}
			if f := cat.Get(*cur.IngredientID); f != nil {
				density = f.Density
			}
		}
		amount, err := units.Convert(cur.Amount, cur.Unit, to, density)
		if err != nil {
			return invalid("%s can't be converted to %s here. Set a density on the food to convert between volume and weight", cur.Name, to)
		}
		return q.UpdateGroceryItem(ctx, sqlc.UpdateGroceryItemParams{
			ID: id, Amount: amount, Unit: units.Normalize(to), Note: cur.Note, Cost: cur.Cost,
			CostRecordedAt: cur.CostRecordedAt, UpdatedBy: actorPtr(actor),
		})
	})
}

// ConvertTargets lists the units an item can be converted to.
func (s *Grocery) ConvertTargets(ctx context.Context, hh uuid.UUID, item sqlc.GetGroceryItemRow) ([]string, error) {
	var density *float64
	if item.IngredientID != nil {
		cat, err := loadCatalog(ctx, s.db.Q, hh)
		if err != nil {
			return nil, err
		}
		if f := cat.Get(*item.IngredientID); f != nil {
			density = f.Density
		}
	}
	return units.Targets(item.Unit, density), nil
}

// SetChecked is the one grocery change every household member may make.
func (s *Grocery) SetChecked(ctx context.Context, hh, actor, id uuid.UUID, checked bool) (uuid.UUID, error) {
	cur, err := s.db.Q.GetGroceryItem(ctx, sqlc.GetGroceryItemParams{ID: id, HouseholdID: hh})
	if noRows(err) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return cur.ListID, s.db.Q.SetGroceryItemChecked(ctx, sqlc.SetGroceryItemCheckedParams{ID: id, Checked: checked, UpdatedBy: actorPtr(actor)})
}

func (s *Grocery) DeleteItem(ctx context.Context, hh, actor, id uuid.UUID) (uuid.UUID, error) {
	cur, err := s.db.Q.GetGroceryItem(ctx, sqlc.GetGroceryItemParams{ID: id, HouseholdID: hh})
	if noRows(err) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return cur.ListID, s.db.Q.SoftDeleteGroceryItem(ctx, sqlc.SoftDeleteGroceryItemParams{ID: id, UpdatedBy: actorPtr(actor)})
}

// GenerateInput picks what to add and where.
type GenerateInput struct {
	Mode     string // "range", "meal", "food"
	From, To time.Time
	MealID   uuid.UUID
	FoodID   uuid.UUID
	Servings float64
	ListID   *uuid.UUID // nil creates a new "Generated list"
}

// Generate adds aggregated ingredients to a live list and returns the list and
// how many lines were added or topped up.
func (s *Grocery) Generate(ctx context.Context, hh, actor uuid.UUID, in GenerateInput) (uuid.UUID, int, error) {
	start := time.Now()
	var list uuid.UUID
	var count int
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		cat, err := loadCatalog(ctx, q, hh)
		if err != nil {
			return err
		}
		var leaves []Leaf
		switch in.Mode {
		case "range":
			if in.From.IsZero() || in.To.IsZero() || in.To.Before(in.From) {
				return invalid("Choose a start date and an end date that isn't before it")
			}
			rows, err := q.ListMealsRange(ctx, sqlc.ListMealsRangeParams{HouseholdID: hh, FromDate: in.From, ToDate: in.To})
			if err != nil {
				return err
			}
			meals, err := loadMeals(ctx, q, rows)
			if err != nil {
				return err
			}
			if len(meals) == 0 {
				return invalid("There are no meals planned between those dates")
			}
			for _, m := range meals {
				leaves = append(leaves, m.Leaves(cat)...)
			}
		case "meal":
			row, err := q.GetMeal(ctx, sqlc.GetMealParams{ID: in.MealID, HouseholdID: hh})
			if noRows(err) {
				return invalid("Choose a planned meal")
			}
			if err != nil {
				return err
			}
			meals, err := loadMeals(ctx, q, []sqlc.MealPlan{row})
			if err != nil {
				return err
			}
			leaves = meals[0].Leaves(cat)
		case "food":
			f := cat.Get(in.FoodID)
			if f == nil || f.Deleted {
				return invalid("Choose a food")
			}
			if in.Servings <= 0 {
				return invalid("Servings must be above zero")
			}
			leaves = cat.Expand(f.ID, in.Servings)
		default:
			return invalid("Choose what to add")
		}
		aggs := cat.Aggregate(leaves)
		if len(aggs) == 0 {
			return invalid("Nothing to add: those recipes have no ingredients yet")
		}

		var existing []sqlc.ListGroceryItemsRow
		if in.ListID != nil {
			if _, err := q.GetGroceryList(ctx, sqlc.GetGroceryListParams{ID: *in.ListID, HouseholdID: hh}); err != nil {
				if noRows(err) {
					return invalid("That list isn't available any more")
				}
				return err
			}
			list = *in.ListID
			if existing, err = q.ListGroceryItems(ctx, list); err != nil {
				return err
			}
		} else {
			if list, err = q.CreateGroceryList(ctx, sqlc.CreateGroceryListParams{HouseholdID: hh, Name: generatedListName, CreatedBy: actorPtr(actor)}); err != nil {
				return err
			}
		}

		for _, a := range aggs {
			var density *float64
			if f := cat.Get(a.FoodID); f != nil {
				density = f.Density
			}
			item := uuid.Nil
			// Top up a matching line only while it is still open: unchecked and unpriced.
			for _, e := range existing {
				if e.IngredientID == nil || *e.IngredientID != a.FoodID || e.Checked || e.Cost != nil || fold(e.VariantText) != fold(a.Variant) {
					continue
				}
				conv, err := units.Convert(a.Amount, a.Unit, e.Unit, density)
				if err != nil {
					continue
				}
				if err := q.AddGroceryItemAmount(ctx, sqlc.AddGroceryItemAmountParams{ID: e.ID, Amount: conv, UpdatedBy: actorPtr(actor)}); err != nil {
					return err
				}
				item = e.ID
				break
			}
			if item == uuid.Nil {
				foodID := a.FoodID
				if item, err = q.CreateGroceryItem(ctx, sqlc.CreateGroceryItemParams{
					ListID: list, IngredientID: &foodID, Name: a.Name, Amount: a.Amount, Unit: a.Unit,
					VariantText: a.Variant, Note: a.Review, SourceType: "generated", CreatedBy: actorPtr(actor),
				}); err != nil {
					return err
				}
			}
			for _, src := range a.Sources {
				recipe := src.RecipeID
				if err := q.InsertGroceryItemSource(ctx, sqlc.InsertGroceryItemSourceParams{
					GroceryItemID: item, ScheduledMealID: src.MealID, RecipeID: &recipe, RecipeIngredientLineID: src.LineID,
					QuantityContributed: src.Amount, UnitContributed: src.Unit, VariantText: src.Variant, LineRecipeName: src.RecipeName,
				}); err != nil {
					return err
				}
			}
			count++
		}
		return nil
	})
	s.metrics.Add("grocery_generation_seconds_sum", time.Since(start).Seconds())
	s.metrics.Inc("grocery_generation_seconds_count")
	if err != nil {
		if _, ok := InvalidMessage(err); !ok {
			slog.ErrorContext(ctx, "grocery generation failed", "household", hh, "mode", in.Mode, "err", err)
			s.metrics.Inc("grocery_generation_failures_total")
		}
	}
	return list, count, err
}
