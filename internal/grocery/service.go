// Package grocery owns grocery lists, items, and generation from the plan.
package grocery

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
	"mealplanner/internal/recipes"
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
	ID      uuid.UUID
	Name    string
	Amount  float64
	Unit    string
	Checked bool
	Note    string
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
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService constructs the grocery service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// ListAll returns every list with items loaded.
func (s *Service) ListAll(ctx context.Context) ([]List, error) {
	lists, err := s.q.ListGroceryLists(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.q.ListAllGroceryItems(ctx)
	if err != nil {
		return nil, err
	}
	byList := map[uuid.UUID][]Item{}
	for _, i := range items {
		byList[i.ListID] = append(byList[i.ListID], Item{
			ID: i.ID, Name: i.Name, Amount: i.Amount, Unit: i.Unit, Checked: i.Checked, Note: i.Note,
		})
	}
	out := make([]List, 0, len(lists))
	for _, l := range lists {
		out = append(out, List{ID: l.ID, Name: l.Name, Items: byList[l.ID]})
	}
	return out, nil
}

// Create adds a new empty list.
func (s *Service) Create(ctx context.Context, name string) (uuid.UUID, error) {
	if strings.TrimSpace(name) == "" {
		name = "New list"
	}
	l, err := s.q.CreateGroceryList(ctx, name)
	if err != nil {
		return uuid.Nil, err
	}
	return l.ID, nil
}

// Rename changes a list's name.
func (s *Service) Rename(ctx context.Context, id uuid.UUID, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	return s.q.RenameGroceryList(ctx, db.RenameGroceryListParams{ID: id, Name: name})
}

// Delete removes a list and its items.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.q.DeleteGroceryList(ctx, id)
}

// ToggleItem flips an item's checked state.
func (s *Service) ToggleItem(ctx context.Context, itemID uuid.UUID) error {
	return s.q.ToggleGroceryItem(ctx, itemID)
}

// DeleteItem removes one item.
func (s *Service) DeleteItem(ctx context.Context, itemID uuid.UUID) error {
	return s.q.DeleteGroceryItem(ctx, itemID)
}

// ClearChecked removes all checked items from a list.
func (s *Service) ClearChecked(ctx context.Context, listID uuid.UUID) error {
	return s.q.DeleteCheckedGroceryItems(ctx, listID)
}

// ConvertItem converts an item to another unit of the same dimension.
func (s *Service) ConvertItem(ctx context.Context, itemID uuid.UUID, toUnit string) error {
	item, err := s.q.GetGroceryItem(ctx, itemID)
	if err != nil {
		return err
	}
	if units.TypeOf(item.Unit) != units.TypeOf(toUnit) || units.TypeOf(toUnit) == units.Count {
		return nil
	}
	converted := units.Round3(units.Convert(item.Amount, item.Unit, toUnit))
	return s.q.UpdateGroceryItemAmount(ctx, db.UpdateGroceryItemAmountParams{
		ID: itemID, Amount: converted, Unit: toUnit,
	})
}

// AddIngredients merges generated ingredients into a list: amounts add up for
// same name+unit matches, everything else is appended — mirroring the
// prototype's onGenerate merge. When listID is nil a new list is created.
func (s *Service) AddIngredients(ctx context.Context, listID *uuid.UUID, ings []recipes.LeafIngredient) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	var target uuid.UUID
	if listID == nil {
		l, err := q.CreateGroceryList(ctx, "Generated list")
		if err != nil {
			return uuid.Nil, err
		}
		target = l.ID
	} else {
		target = *listID
	}

	existing, err := q.ListGroceryItems(ctx, target)
	if err != nil {
		return uuid.Nil, err
	}
	maxSort, err := q.MaxGrocerySortOrder(ctx, target)
	if err != nil {
		return uuid.Nil, err
	}
	next := int(maxSort) + 1

	for _, ing := range ings {
		key := strings.ToLower(strings.TrimSpace(ing.Name))
		var matched *db.GroceryItem
		for i := range existing {
			if strings.ToLower(strings.TrimSpace(existing[i].Name)) == key && existing[i].Unit == ing.Unit {
				matched = &existing[i]
				break
			}
		}
		if matched != nil {
			if err := q.AddGroceryItemAmount(ctx, db.AddGroceryItemAmountParams{
				ID: matched.ID, Amount: units.Round3(ing.Amount),
			}); err != nil {
				return uuid.Nil, err
			}
			matched.Amount += ing.Amount
			continue
		}
		created, err := q.CreateGroceryItem(ctx, db.CreateGroceryItemParams{
			ListID: target, Name: ing.Name, Amount: units.Round3(ing.Amount), Unit: ing.Unit,
			Checked: false, Note: "", SortOrder: int32(next),
		})
		if err != nil {
			return uuid.Nil, err
		}
		existing = append(existing, created)
		next++
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}
	return target, nil
}
