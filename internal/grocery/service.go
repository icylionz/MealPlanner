// Package grocery owns grocery lists, items, and generation from the plan.
package grocery

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
	"mealplanner/internal/foods"
	"mealplanner/internal/units"
)

// List is a grocery list with its items.
type List struct {
	ID    uuid.UUID
	Name  string
	Items []Item
}

// Item is one grocery line.
type Item struct {
	ID           uuid.UUID
	IngredientID *uuid.UUID
	Name         string
	Amount       float64
	Unit         string
	Checked      bool
	Note         string
	SourceType   string
	Variant      string
	Density      float64 // g/ml of the matching food, 0 if unknown (view-only)
}

// ItemDetail is one household-scoped grocery item and its generation history.
type ItemDetail struct {
	ListID  uuid.UUID
	Item    Item
	Sources []ItemSource
}

// ItemSource is one raw contribution to a generated grocery item. Names are
// resolved without filtering soft-deleted meals or foods so history stays
// readable.
type ItemSource struct {
	MealID         *uuid.UUID
	MealName       string
	MealDate       string
	MealTime       string
	RecipeID       *uuid.UUID
	RecipeName     string
	LineRecipeID   *uuid.UUID
	LineRecipeName string
	Variant        string
	Amount         float64
	Unit           string
}

// UncheckedCount returns how many items remain unchecked.
func (l List) UncheckedCount() int {
	n := 0
	for _, i := range l.Items {
		if !i.Checked {
			n++
		}
	}
	return n
}

// CheckedCount returns how many items are checked.
func (l List) CheckedCount() int {
	n := 0
	for _, i := range l.Items {
		if i.Checked {
			n++
		}
	}
	return n
}

// Service owns grocery use cases.
type Service struct {
	q               *db.Queries
	beginGeneration func(context.Context) (generationTx, error)
}

type generationTx interface {
	LockGroceryList(context.Context, db.LockGroceryListParams) (db.GroceryList, error)
	CreateGroceryList(context.Context, db.CreateGroceryListParams) (db.GroceryList, error)
	GroceryIngredientBelongsToHousehold(context.Context, db.GroceryIngredientBelongsToHouseholdParams) (bool, error)
	ListGroceryItems(context.Context, uuid.UUID) ([]db.GroceryItem, error)
	MaxGrocerySortOrder(context.Context, uuid.UUID) (int32, error)
	AddGroceryItemAmount(context.Context, db.AddGroceryItemAmountParams) error
	CreateGroceryItem(context.Context, db.CreateGroceryItemParams) (db.GroceryItem, error)
	CreateGroceryItemSource(context.Context, db.CreateGroceryItemSourceParams) (uuid.UUID, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

type pgxGenerationTx struct {
	pgx.Tx
	*db.Queries
}

// NewService constructs the grocery service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		q: db.New(pool),
		beginGeneration: func(ctx context.Context) (generationTx, error) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return nil, err
			}
			return &pgxGenerationTx{Tx: tx, Queries: db.New(tx)}, nil
		},
	}
}

// ListAll returns every list with items loaded.
func (s *Service) ListAll(ctx context.Context, householdID uuid.UUID) ([]List, error) {
	lists, err := s.q.ListGroceryLists(ctx, householdID)
	if err != nil {
		return nil, err
	}
	items, err := s.q.ListAllGroceryItems(ctx, householdID)
	if err != nil {
		return nil, err
	}
	byList := map[uuid.UUID][]Item{}
	for _, i := range items {
		byList[i.ListID] = append(byList[i.ListID], Item{
			ID: i.ID, IngredientID: i.IngredientID, Name: i.Name, Amount: i.Amount,
			Unit: i.Unit, Checked: i.Checked, Note: i.Note, SourceType: i.SourceType,
			Variant: i.VariantText,
		})
	}
	out := make([]List, 0, len(lists))
	for _, l := range lists {
		out = append(out, List{ID: l.ID, Name: l.Name, Items: byList[l.ID]})
	}
	return out, nil
}

// GetItemDetail returns an item and its contributing meals, recipes, and exact
// nested component lines. Both reads enforce the active household boundary.
func (s *Service) GetItemDetail(ctx context.Context, householdID, itemID uuid.UUID) (*ItemDetail, error) {
	item, err := s.q.GetGroceryItem(ctx, db.GetGroceryItemParams{ID: itemID, HouseholdID: householdID})
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListGroceryItemSources(ctx, db.ListGroceryItemSourcesParams{
		GroceryItemID: itemID, HouseholdID: householdID,
	})
	if err != nil {
		return nil, err
	}
	detail := &ItemDetail{
		ListID: item.ListID,
		Item: Item{
			ID: item.ID, IngredientID: item.IngredientID, Name: item.Name,
			Amount: item.Amount, Unit: item.Unit, Checked: item.Checked,
			Note: item.Note, SourceType: item.SourceType,
			Variant: item.VariantText,
		},
	}
	for _, row := range rows {
		detail.Sources = append(detail.Sources, ItemSource{
			MealID: row.ScheduledMealID, MealName: row.MealName,
			MealDate: row.MealDate, MealTime: row.MealTime,
			RecipeID: row.RecipeID, RecipeName: row.RecipeName,
			LineRecipeID: row.LineRecipeID, LineRecipeName: row.LineRecipeName,
			Variant: row.VariantText,
			Amount:  row.QuantityContributed, Unit: row.UnitContributed,
		})
	}
	return detail, nil
}

// byPtr returns a nil pointer for the zero account id so authorship columns stay
// NULL when no actor is known, and a pointer to the account otherwise (G2).
func byPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// Create adds a new empty list. actor is recorded as the author (G2).
func (s *Service) Create(ctx context.Context, householdID, actor uuid.UUID, name string) (uuid.UUID, error) {
	if strings.TrimSpace(name) == "" {
		name = "New list"
	}
	l, err := s.q.CreateGroceryList(ctx, db.CreateGroceryListParams{HouseholdID: householdID, Name: name, CreatedBy: byPtr(actor)})
	if err != nil {
		return uuid.Nil, err
	}
	return l.ID, nil
}

// Rename changes a list's name.
func (s *Service) Rename(ctx context.Context, householdID, actor, id uuid.UUID, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return s.q.RenameGroceryList(ctx, db.RenameGroceryListParams{ID: id, Name: name, HouseholdID: householdID, UpdatedBy: byPtr(actor)})
}

// Delete removes a list and its items.
func (s *Service) Delete(ctx context.Context, householdID, actor, id uuid.UUID) error {
	return s.q.DeleteGroceryList(ctx, db.DeleteGroceryListParams{ID: id, HouseholdID: householdID, UpdatedBy: byPtr(actor)})
}

// ToggleItem flips an item's checked state.
func (s *Service) ToggleItem(ctx context.Context, householdID, actor, itemID uuid.UUID) error {
	return s.q.ToggleGroceryItem(ctx, db.ToggleGroceryItemParams{ID: itemID, HouseholdID: householdID, UpdatedBy: byPtr(actor)})
}

// DeleteItem removes one item.
func (s *Service) DeleteItem(ctx context.Context, householdID, actor, itemID uuid.UUID) error {
	return s.q.DeleteGroceryItem(ctx, db.DeleteGroceryItemParams{ID: itemID, HouseholdID: householdID, UpdatedBy: byPtr(actor)})
}

// ClearChecked removes all checked items from a list the household owns.
func (s *Service) ClearChecked(ctx context.Context, householdID, actor, listID uuid.UUID) error {
	if _, err := s.q.GetGroceryList(ctx, db.GetGroceryListParams{ID: listID, HouseholdID: householdID}); err != nil {
		return err
	}
	return s.q.DeleteCheckedGroceryItems(ctx, db.DeleteCheckedGroceryItemsParams{ListID: listID, UpdatedBy: byPtr(actor)})
}

// ConvertItem converts an item to another unit, crossing the mass<->volume
// boundary when a density for the item is known (densities keyed by lowercased
// name). If the target needs a density that is not available, the item is left
// unchanged and flagged for review instead of guessing (FR10.5).
func (s *Service) ConvertItem(ctx context.Context, householdID, actor, itemID uuid.UUID, toUnit string, densities map[string]float64) error {
	item, err := s.q.GetGroceryItem(ctx, db.GetGroceryItemParams{ID: itemID, HouseholdID: householdID})
	if err != nil {
		return err
	}
	if units.TypeOf(toUnit) == units.Count || units.TypeOf(item.Unit) == units.Count {
		return nil
	}
	density := densities[strings.ToLower(strings.TrimSpace(item.Name))]
	if item.IngredientID != nil {
		density = densities[item.IngredientID.String()]
	}
	converted, ok := units.ConvertDensity(item.Amount, item.Unit, toUnit, density)
	if !ok {
		// Cross-dimension conversion requested but no density available.
		return s.q.SetGroceryItemNote(ctx, db.SetGroceryItemNoteParams{
			ID: itemID, Note: "needs density to convert to " + toUnit, HouseholdID: householdID, UpdatedBy: byPtr(actor),
		})
	}
	return s.q.UpdateGroceryItemAmount(ctx, db.UpdateGroceryItemAmountParams{
		ID: itemID, Amount: units.Round3(converted), Unit: toUnit, HouseholdID: householdID, UpdatedBy: byPtr(actor),
	})
}

// AddIngredients merges generated ingredients into a list by canonical leaf
// food and unit. Ad-hoc items never absorb generated quantities. Item updates
// and their raw provenance rows are committed atomically.
func (s *Service) AddIngredients(ctx context.Context, householdID, actor uuid.UUID, listID *uuid.UUID, ings []foods.LeafIngredient) (uuid.UUID, error) {
	tx, err := s.beginGeneration(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var target uuid.UUID
	if listID == nil {
		l, err := tx.CreateGroceryList(ctx, db.CreateGroceryListParams{HouseholdID: householdID, Name: "Generated list", CreatedBy: byPtr(actor)})
		if err != nil {
			return uuid.Nil, err
		}
		target = l.ID
	} else {
		// The row lock both validates the active household boundary and serializes
		// generation with concurrent generation/deletion of this list.
		if _, err := tx.LockGroceryList(ctx, db.LockGroceryListParams{ID: *listID, HouseholdID: householdID}); err != nil {
			return uuid.Nil, err
		}
		target = *listID
	}

	existing, err := tx.ListGroceryItems(ctx, target)
	if err != nil {
		return uuid.Nil, err
	}
	maxSort, err := tx.MaxGrocerySortOrder(ctx, target)
	if err != nil {
		return uuid.Nil, err
	}
	next := int(maxSort) + 1

	for _, ing := range ings {
		displayName := foods.IngredientDisplayName(ing.Name, ing.Variant)
		belongs, err := tx.GroceryIngredientBelongsToHousehold(ctx, db.GroceryIngredientBelongsToHouseholdParams{
			ID: ing.FoodID, HouseholdID: householdID,
		})
		if err != nil {
			return uuid.Nil, err
		}
		if !belongs {
			return uuid.Nil, errors.New("grocery ingredient does not belong to household")
		}
		var matched *db.GroceryItem
		amount := ing.Amount
		for i := range existing {
			if existing[i].SourceType == "generated" && existing[i].IngredientID != nil &&
				*existing[i].IngredientID == ing.FoodID &&
				foods.NormalizeVariant(existing[i].VariantText) == foods.NormalizeVariant(ing.Variant) {
				switch {
				case existing[i].Unit == ing.Unit:
					matched = &existing[i]
				case units.TypeOf(existing[i].Unit) == units.TypeOf(ing.Unit) && units.TypeOf(ing.Unit) != units.Count:
					matched = &existing[i]
					amount = units.FromBase(units.ToBase(ing.Amount, ing.Unit), existing[i].Unit)
				default:
					if converted, ok := units.ConvertDensity(ing.Amount, ing.Unit, existing[i].Unit, ing.Density); ok &&
						units.TypeOf(ing.Unit) != units.Count && units.TypeOf(existing[i].Unit) != units.Count {
						matched = &existing[i]
						amount = converted
					}
				}
				if matched != nil {
					break
				}
			}
		}
		var itemID uuid.UUID
		if matched != nil {
			if err := tx.AddGroceryItemAmount(ctx, db.AddGroceryItemAmountParams{
				ID: matched.ID, Amount: units.Round3(amount), HouseholdID: householdID, UpdatedBy: byPtr(actor),
			}); err != nil {
				return uuid.Nil, err
			}
			matched.Amount += amount
			itemID = matched.ID
		} else {
			ingredientID := ing.FoodID
			created, err := tx.CreateGroceryItem(ctx, db.CreateGroceryItemParams{
				ListID: target, Name: displayName, Amount: units.Round3(ing.Amount), Unit: ing.Unit,
				Checked: false, Note: "", SortOrder: int32(next), IngredientID: &ingredientID,
				SourceType: "generated", VariantText: strings.Join(strings.Fields(ing.Variant), " "), CreatedBy: byPtr(actor),
			})
			if err != nil {
				return uuid.Nil, err
			}
			existing = append(existing, created)
			itemID = created.ID
			next++
		}
		for _, source := range ing.Sources {
			recipeID, componentID := source.RecipeID, source.ComponentID
			variant := source.Variant
			if variant == "" {
				variant = ing.Variant
			}
			if _, err := tx.CreateGroceryItemSource(ctx, db.CreateGroceryItemSourceParams{
				GroceryItemID: itemID, ScheduledMealID: source.MealID,
				RecipeID: &recipeID, RecipeIngredientLineID: &componentID,
				QuantityContributed: units.Round3(source.Amount), UnitContributed: source.Unit,
				VariantText: variant, LineRecipeName: source.LineRecipe,
				HouseholdID: householdID,
			}); err != nil {
				return uuid.Nil, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return target, nil
}
