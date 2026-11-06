package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/icylionz/mealplanner/internal/observability"
	"github.com/icylionz/mealplanner/internal/platform/database"
	"github.com/icylionz/mealplanner/internal/repo/db"
	"github.com/icylionz/mealplanner/internal/units"
)

const GeneratedListName = "Generated list"

type GroceryService struct {
	db      *database.DB
	metrics *observability.Metrics
	planner *PlannerService
}

func NewGroceryService(d *database.DB, m *observability.Metrics, p *PlannerService) *GroceryService {
	return &GroceryService{db: d, metrics: m, planner: p}
}

func (s *GroceryService) Lists(ctx context.Context, householdID uuid.UUID) ([]db.ListGroceryListsRow, error) {
	return s.db.Q.ListGroceryLists(ctx, householdID)
}

type ListDetail struct {
	List    db.GroceryList
	Items   []db.GroceryItem
	Sources map[uuid.UUID]int
	Total   float64
	Priced  int
}

func (d ListDetail) Checked() int {
	n := 0
	for _, i := range d.Items {
		if i.Checked {
			n++
		}
	}
	return n
}

func (s *GroceryService) List(ctx context.Context, householdID, id uuid.UUID) (ListDetail, error) {
	l, err := s.db.Q.GetGroceryList(ctx, db.GetGroceryListParams{ID: id, HouseholdID: householdID})
	if noRows(err) {
		return ListDetail{}, ErrNotFound
	}
	if err != nil {
		return ListDetail{}, err
	}
	items, err := s.db.Q.ListGroceryItems(ctx, id)
	if err != nil {
		return ListDetail{}, err
	}
	srcs, err := s.db.Q.ListGrocerySourcesForList(ctx, id)
	if err != nil {
		return ListDetail{}, err
	}
	d := ListDetail{List: l, Items: items, Sources: map[uuid.UUID]int{}}
	for _, src := range srcs {
		d.Sources[src.GroceryItemID]++
	}
	for _, i := range items {
		if i.Cost != nil {
			d.Total += *i.Cost
			d.Priced++
		}
	}
	return d, nil
}

func (s *GroceryService) CreateList(ctx context.Context, a Actor, name string) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return uuid.Nil, invalid("Enter a list name.")
	}
	l, err := s.db.Q.InsertGroceryList(ctx, db.InsertGroceryListParams{ID: uuid.New(), HouseholdID: a.HouseholdID, Name: name, CreatedBy: a.ptr()})
	return l.ID, err
}

func (s *GroceryService) RenameList(ctx context.Context, a Actor, id uuid.UUID, name string) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Enter a list name.")
	}
	n, err := s.db.Q.RenameGroceryList(ctx, db.RenameGroceryListParams{ID: id, HouseholdID: a.HouseholdID, Name: name, UpdatedBy: a.ptr()})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *GroceryService) DeleteList(ctx context.Context, a Actor, id uuid.UUID) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	n, err := s.db.Q.SoftDeleteGroceryList(ctx, db.SoftDeleteGroceryListParams{ID: id, HouseholdID: a.HouseholdID, UpdatedBy: a.ptr()})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *GroceryService) item(ctx context.Context, a Actor, id uuid.UUID) (db.GroceryItem, error) {
	it, err := s.db.Q.GetGroceryItem(ctx, db.GetGroceryItemParams{ID: id, HouseholdID: a.HouseholdID})
	if noRows(err) {
		return it, ErrNotFound
	}
	return it, err
}

// AddItem adds an ad-hoc line, linking it to a catalog food by name when one
// matches so recorded costs feed estimates.
func (s *GroceryService) AddItem(ctx context.Context, a Actor, listID uuid.UUID, name string, amount float64, unit, note string) error {
	if err := a.requireOwner(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return invalid("Enter an item name.")
	}
	if amount < 0 {
		return invalid("Amount cannot be negative.")
	}
	if _, err := s.db.Q.GetGroceryList(ctx, db.GetGroceryListParams{ID: listID, HouseholdID: a.HouseholdID}); err != nil {
		return ErrNotFound
	}
	cat, err := loadCatalog(ctx, s.db.Q, a.HouseholdID)
	if err != nil {
		return err
	}
	var ingredient *uuid.UUID
	if f := cat.FindByName(name); f != nil {
		ingredient = &f.ID
		name = f.Name
	}
	sort, err := s.db.Q.NextGrocerySort(ctx, listID)
	if err != nil {
		return err
	}
	_, err = s.db.Q.InsertGroceryItem(ctx, db.InsertGroceryItemParams{
		ID: uuid.New(), ListID: listID, IngredientID: ingredient, Name: name, Amount: amount,
		Unit: strings.TrimSpace(unit), Note: strings.TrimSpace(note), SortOrder: sort, SourceType: "adhoc", CreatedBy: a.ptr(),
	})
	return err
}

// SetChecked is available to every household member (FR3.3, FR12.3).
func (s *GroceryService) SetChecked(ctx context.Context, a Actor, itemID uuid.UUID, checked bool) (uuid.UUID, error) {
	it, err := s.item(ctx, a, itemID)
	if err != nil {
		return uuid.Nil, err
	}
	return it.ListID, s.db.Q.SetGroceryItemChecked(ctx, db.SetGroceryItemCheckedParams{ID: itemID, Checked: checked, UpdatedBy: a.ptr()})
}

type ItemEdit struct {
	Name    string
	Amount  float64
	Unit    string
	Variant string
	Note    string
}

// EditItem changes only the list line, never the source foods (FR12.4).
func (s *GroceryService) EditItem(ctx context.Context, a Actor, itemID uuid.UUID, e ItemEdit) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	it, err := s.item(ctx, a, itemID)
	if err != nil {
		return uuid.Nil, err
	}
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || e.Amount < 0 {
		return it.ListID, invalid("Enter a name and a non-negative amount.")
	}
	return it.ListID, s.db.Q.EditGroceryItem(ctx, db.EditGroceryItemParams{
		ID: itemID, Name: e.Name, Amount: e.Amount, Unit: strings.TrimSpace(e.Unit),
		VariantText: strings.TrimSpace(e.Variant), Note: strings.TrimSpace(e.Note), UpdatedBy: a.ptr(),
	})
}

// ConvertItem re-expresses a line in another unit using the food's density.
func (s *GroceryService) ConvertItem(ctx context.Context, a Actor, itemID uuid.UUID, unit string) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	it, err := s.item(ctx, a, itemID)
	if err != nil {
		return uuid.Nil, err
	}
	var density *float64
	if it.IngredientID != nil {
		if f, err := s.db.Q.GetFood(ctx, db.GetFoodParams{ID: *it.IngredientID, HouseholdID: a.HouseholdID}); err == nil {
			density = f.DensityGPerMl
		}
	}
	v, err := units.Convert(it.Amount, it.Unit, unit, density)
	if err == units.ErrNeedsDensity {
		return it.ListID, invalid("This conversion needs a density on the food. Add one on the food page.")
	}
	if err != nil {
		return it.ListID, invalid("Those units cannot be converted.")
	}
	return it.ListID, s.db.Q.UpdateGroceryItemAmount(ctx, db.UpdateGroceryItemAmountParams{ID: itemID, Amount: v, Unit: unit, Note: it.Note, UpdatedBy: a.ptr()})
}

// SetCost records what the line cost (FR19.1). A nil cost clears it.
func (s *GroceryService) SetCost(ctx context.Context, a Actor, itemID uuid.UUID, cost *float64) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	it, err := s.item(ctx, a, itemID)
	if err != nil {
		return uuid.Nil, err
	}
	if cost != nil && *cost < 0 {
		return it.ListID, invalid("Cost cannot be negative.")
	}
	var at *time.Time
	if cost != nil {
		now := time.Now()
		at = &now
	}
	return it.ListID, s.db.Q.SetGroceryItemCost(ctx, db.SetGroceryItemCostParams{ID: itemID, Cost: cost, CostRecordedAt: at, UpdatedBy: a.ptr()})
}

func (s *GroceryService) RemoveItem(ctx context.Context, a Actor, itemID uuid.UUID) (uuid.UUID, error) {
	if err := a.requireOwner(); err != nil {
		return uuid.Nil, err
	}
	it, err := s.item(ctx, a, itemID)
	if err != nil {
		return uuid.Nil, err
	}
	return it.ListID, s.db.Q.SoftDeleteGroceryItem(ctx, db.SoftDeleteGroceryItemParams{ID: itemID, UpdatedBy: a.ptr()})
}

// SourceView explains one contribution to a grocery line (FR12.2, FR12.5).
type SourceView struct {
	Source     db.GroceryItemSource
	MealTitle  string
	MealDate   *time.Time
	MealID     *uuid.UUID
	RecipeName string
	RecipeID   *uuid.UUID
}

type ItemDetail struct {
	Item        db.GroceryItem
	List        db.GroceryList
	Sources     []SourceView
	Food        *Food
	CostPerUnit *float64
}

func (s *GroceryService) Item(ctx context.Context, a Actor, cat *Catalog, itemID uuid.UUID) (ItemDetail, error) {
	it, err := s.item(ctx, a, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	l, err := s.db.Q.GetGroceryList(ctx, db.GetGroceryListParams{ID: it.ListID, HouseholdID: a.HouseholdID})
	if err != nil {
		return ItemDetail{}, err
	}
	srcs, err := s.db.Q.ListGroceryItemSources(ctx, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	var mealIDs []uuid.UUID
	for _, src := range srcs {
		if src.ScheduledMealID != nil {
			mealIDs = append(mealIDs, *src.ScheduledMealID)
		}
	}
	meals := map[uuid.UUID]MealView{}
	if len(mealIDs) > 0 {
		rows, err := s.db.Q.ListMealsByIDs(ctx, db.ListMealsByIDsParams{HouseholdID: a.HouseholdID, Ids: mealIDs})
		if err != nil {
			return ItemDetail{}, err
		}
		views, err := buildMealViews(ctx, s.db.Q, cat, a.HouseholdID, rows)
		if err != nil {
			return ItemDetail{}, err
		}
		for _, v := range views {
			meals[v.Meal.ID] = v
		}
	}
	d := ItemDetail{Item: it, List: l}
	if it.IngredientID != nil {
		d.Food = cat.Get(*it.IngredientID)
	}
	if it.Cost != nil && it.Amount > 0 {
		cpu := *it.Cost / it.Amount
		d.CostPerUnit = &cpu
	}
	for _, src := range srcs {
		v := SourceView{Source: src, RecipeID: src.RecipeID, RecipeName: src.LineRecipeName}
		if src.RecipeID != nil && v.RecipeName == "" {
			v.RecipeName = cat.Name(*src.RecipeID)
		}
		if src.ScheduledMealID != nil {
			v.MealID = src.ScheduledMealID
			if m, ok := meals[*src.ScheduledMealID]; ok {
				v.MealTitle = m.Title()
				if m.Meal.DeletedAt != nil {
					v.MealTitle += " (deleted)"
				}
				date := m.Meal.PlanDate
				v.MealDate = &date
			} else {
				v.MealTitle = "Deleted meal"
			}
		}
		d.Sources = append(d.Sources, v)
	}
	return d, nil
}

// GenerateRequest selects the source of generated ingredients (FR11).
type GenerateRequest struct {
	Mode     string // "range", "meal", "food"
	From, To time.Time
	MealID   uuid.UUID
	FoodID   uuid.UUID
	Servings float64
	ListID   uuid.UUID // zero → create a new list
	NewName  string
}

type GenerateResult struct {
	ListID  uuid.UUID
	Added   int
	Merged  int
	Review  int
	Sources int
}

// Generate expands the chosen source and merges it into a live list.
func (s *GroceryService) Generate(ctx context.Context, a Actor, req GenerateRequest) (GenerateResult, error) {
	if err := a.requireOwner(); err != nil {
		return GenerateResult{}, err
	}
	start := time.Now()
	var res GenerateResult
	err := s.db.Tx(ctx, func(q *db.Queries) error {
		cat, err := loadCatalog(ctx, q, a.HouseholdID)
		if err != nil {
			return err
		}
		leaves, err := s.collect(ctx, q, cat, a.HouseholdID, req)
		if err != nil {
			return err
		}
		if len(leaves) == 0 {
			return invalid("Nothing to add: the selection has no ingredients.")
		}
		listID := req.ListID
		if listID == uuid.Nil {
			name := strings.TrimSpace(req.NewName)
			if name == "" {
				name = GeneratedListName
			}
			l, err := q.InsertGroceryList(ctx, db.InsertGroceryListParams{ID: uuid.New(), HouseholdID: a.HouseholdID, Name: name, CreatedBy: a.ptr()})
			if err != nil {
				return err
			}
			listID = l.ID
		} else if _, err := q.GetGroceryList(ctx, db.GetGroceryListParams{ID: listID, HouseholdID: a.HouseholdID}); err != nil {
			return ErrNotFound
		}
		res.ListID = listID
		existing, err := q.ListGroceryItems(ctx, listID)
		if err != nil {
			return err
		}
		sort, err := q.NextGrocerySort(ctx, listID)
		if err != nil {
			return err
		}
		for _, agg := range cat.Aggregate(leaves) {
			itemID, merged, err := mergeLine(ctx, q, cat, listID, existing, agg, &sort, a.ptr())
			if err != nil {
				return err
			}
			if merged {
				res.Merged++
			} else {
				res.Added++
			}
			if agg.Review != "" {
				res.Review++
			}
			for _, p := range agg.Parts {
				if err := q.InsertGroceryItemSource(ctx, db.InsertGroceryItemSourceParams{
					ID: uuid.New(), GroceryItemID: itemID, ScheduledMealID: p.MealID, RecipeID: &p.RecipeID,
					RecipeIngredientLineID: p.LineID, QuantityContributed: p.Amount, UnitContributed: p.Unit,
					VariantText: p.Variant, LineRecipeName: p.RecipeName,
				}); err != nil {
					return err
				}
				res.Sources++
			}
			if !merged {
				existing = append(existing, db.GroceryItem{ID: itemID, IngredientID: &agg.FoodID, Unit: agg.Unit, VariantText: agg.Variant, Amount: agg.Amount, Note: agg.Review})
			}
		}
		return nil
	})
	s.metrics.Observe("mealplanner_grocery_generation_duration", time.Since(start))
	if err != nil {
		if _, ok := err.(*ValidationError); !ok {
			s.metrics.Inc("mealplanner_grocery_generation_failures_total")
			slog.ErrorContext(ctx, "grocery generation failed", "err", err, "request_id", observability.RequestID(ctx))
		}
	}
	return res, err
}

func (s *GroceryService) collect(ctx context.Context, q *db.Queries, cat *Catalog, householdID uuid.UUID, req GenerateRequest) ([]Leaf, error) {
	tag := func(ls []Leaf, meal *uuid.UUID) []Leaf {
		for i := range ls {
			ls[i].MealID = meal
		}
		return ls
	}
	switch req.Mode {
	case "range":
		if req.From.IsZero() || req.To.IsZero() || req.To.Before(req.From) {
			return nil, invalid("Choose a valid date range.")
		}
		meals, err := q.ListMealsInRange(ctx, db.ListMealsInRangeParams{HouseholdID: householdID, FromDate: req.From, ToDate: req.To})
		if err != nil {
			return nil, err
		}
		views, err := buildMealViews(ctx, q, cat, householdID, meals)
		if err != nil {
			return nil, err
		}
		var out []Leaf
		for _, v := range views {
			id := v.Meal.ID
			out = append(out, tag(v.Leaves(cat), &id)...)
		}
		return out, nil
	case "meal":
		m, err := q.GetMeal(ctx, db.GetMealParams{ID: req.MealID, HouseholdID: householdID})
		if err != nil || m.DeletedAt != nil {
			return nil, invalid("Choose a planned meal.")
		}
		views, err := buildMealViews(ctx, q, cat, householdID, []db.MealPlan{m})
		if err != nil {
			return nil, err
		}
		return tag(views[0].Leaves(cat), &m.ID), nil
	case "food":
		f := cat.Get(req.FoodID)
		if f == nil || f.Deleted() {
			return nil, invalid("Choose a food.")
		}
		if req.Servings <= 0 {
			return nil, invalid("Servings must be above zero.")
		}
		return cat.Expand(f.ID, req.Servings), nil
	}
	return nil, invalid("Choose what to add ingredients from.")
}

// mergeLine adds an aggregated quantity to a compatible unchecked, unpriced
// line (a priced line keeps its cost-per-unit meaning) or
// inserts a new generated line (FR11.4, FR12.1).
func mergeLine(ctx context.Context, q *db.Queries, cat *Catalog, listID uuid.UUID, existing []db.GroceryItem, agg Aggregated, sort *int32, by *uuid.UUID) (uuid.UUID, bool, error) {
	density := cat.density(agg.FoodID)
	for _, it := range existing {
		if it.Checked || it.Cost != nil || it.IngredientID == nil || *it.IngredientID != agg.FoodID || !strings.EqualFold(it.VariantText, agg.Variant) {
			continue
		}
		v, err := units.Convert(agg.Amount, agg.Unit, it.Unit, density)
		if err != nil {
			continue
		}
		note := it.Note
		if agg.Review != "" && note == "" {
			note = agg.Review
		}
		if err := q.UpdateGroceryItemAmount(ctx, db.UpdateGroceryItemAmountParams{ID: it.ID, Amount: it.Amount + v, Unit: it.Unit, Note: note, UpdatedBy: by}); err != nil {
			return uuid.Nil, false, err
		}
		return it.ID, true, nil
	}
	foodID := agg.FoodID
	row, err := q.InsertGroceryItem(ctx, db.InsertGroceryItemParams{
		ID: uuid.New(), ListID: listID, IngredientID: &foodID, Name: agg.Name, Amount: agg.Amount, Unit: agg.Unit,
		VariantText: agg.Variant, Note: agg.Review, SortOrder: *sort, SourceType: "generated", CreatedBy: by,
	})
	*sort++
	return row.ID, false, err
}
