package transfer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mealplanner/internal/database/db"
)

// Service reads and writes the full data archive against the database.
type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewService constructs the transfer service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

// Export reads one household's entities into a single archive.
func (s *Service) Export(ctx context.Context, householdID uuid.UUID) (*Archive, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	arc := &Archive{SchemaVersion: SchemaVersion, ExportedAt: nowUTC()}

	foods, err := q.ExportFoods(ctx, householdID)
	if err != nil {
		return nil, err
	}
	tags, err := q.ExportFoodTags(ctx, householdID)
	if err != nil {
		return nil, err
	}
	aliases, err := q.ExportFoodAliases(ctx, householdID)
	if err != nil {
		return nil, err
	}
	comps, err := q.ExportFoodComponents(ctx, householdID)
	if err != nil {
		return nil, err
	}
	steps, err := q.ExportFoodSteps(ctx, householdID)
	if err != nil {
		return nil, err
	}
	tagsBy := map[uuid.UUID][]string{}
	for _, t := range tags {
		tagsBy[t.FoodID] = append(tagsBy[t.FoodID], t.Tag)
	}
	aliasesBy := map[uuid.UUID][]Alias{}
	for _, alias := range aliases {
		aliasesBy[alias.FoodID] = append(aliasesBy[alias.FoodID], Alias{
			ID: alias.ID, Alias: alias.Alias, CreatedBy: alias.CreatedBy, CreatedAt: alias.CreatedAt,
		})
	}
	compsBy := map[uuid.UUID][]Component{}
	for _, c := range comps {
		compsBy[c.ParentFoodID] = append(compsBy[c.ParentFoodID], Component{
			ID: c.ID, ChildFoodID: c.ChildFoodID, Amount: c.Amount,
			Unit: c.Unit, Variant: c.VariantText, SortOrder: c.SortOrder,
		})
	}
	stepsBy := map[uuid.UUID][]Step{}
	for _, st := range steps {
		stepsBy[st.FoodID] = append(stepsBy[st.FoodID], Step{
			StepNumber: st.StepNumber, Instruction: st.Instruction,
		})
	}
	for _, f := range foods {
		arc.Foods = append(arc.Foods, Food{
			ID: f.ID, Name: f.Name, Description: f.Description,
			PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin, Servings: f.Servings,
			DefaultUnit: f.DefaultUnit, DensityGPerMl: f.DensityGPerMl, DensitySource: f.DensitySource,
			SourceURL: stringValue(f.SourceUrl), SourceLastImportedAt: timestampPtr(f.SourceLastImportedAt),
			CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt, DeletedAt: timestampPtr(f.DeletedAt),
			CreatedBy: f.CreatedBy, UpdatedBy: f.UpdatedBy,
			Aliases: aliasesBy[f.ID], Tags: tagsBy[f.ID], Components: compsBy[f.ID], Steps: stepsBy[f.ID],
		})
	}

	series, err := q.ExportMealSeries(ctx, householdID)
	if err != nil {
		return nil, err
	}
	for _, m := range series {
		arc.MealSeries = append(arc.MealSeries, MealSeries{
			ID: m.ID, FoodID: m.FoodID, PlanTime: m.PlanTime, Servings: m.Servings,
			Freq: m.Freq, Byweekday: m.Byweekday, StartDate: m.StartDate, UntilDate: m.UntilDate,
		})
	}

	plans, err := q.ExportMealPlans(ctx, householdID)
	if err != nil {
		return nil, err
	}
	mealRecipes, err := q.ExportScheduledMealRecipes(ctx, householdID)
	if err != nil {
		return nil, err
	}
	recipesByMeal := map[uuid.UUID][]ScheduledMealRecipe{}
	for _, recipe := range mealRecipes {
		recipesByMeal[recipe.MealID] = append(recipesByMeal[recipe.MealID], ScheduledMealRecipe{
			ID: recipe.ID, FoodID: recipe.FoodID, ServingsOverride: recipe.ServingsOverride,
			SortOrder: recipe.SortOrder, CreatedBy: recipe.CreatedBy, UpdatedBy: recipe.UpdatedBy,
		})
	}
	for _, m := range plans {
		arc.MealPlans = append(arc.MealPlans, MealPlan{
			ID: m.ID, PlanDate: m.PlanDate, PlanTime: m.PlanTime,
			FoodID: m.FoodID, Servings: m.Servings, SeriesID: m.SeriesID,
			LinkURL: m.LinkUrl, LinkTitle: m.LinkTitle, LinkImageURL: m.LinkImageUrl,
			Title: m.Title, Notes: m.Notes, Version: m.Version,
			DeletedAt: timestampPtr(m.DeletedAt), CreatedBy: m.CreatedBy, UpdatedBy: m.UpdatedBy,
			Recipes: recipesByMeal[m.ID],
		})
	}

	lists, err := q.ExportGroceryLists(ctx, householdID)
	if err != nil {
		return nil, err
	}
	items, err := q.ExportGroceryItems(ctx, householdID)
	if err != nil {
		return nil, err
	}
	sources, err := q.ExportGroceryItemSources(ctx, householdID)
	if err != nil {
		return nil, err
	}
	sourcesBy := map[uuid.UUID][]GroceryItemSource{}
	for _, source := range sources {
		sourcesBy[source.GroceryItemID] = append(sourcesBy[source.GroceryItemID], GroceryItemSource{
			ID: source.ID, ScheduledMealID: source.ScheduledMealID, RecipeID: source.RecipeID,
			RecipeIngredientLineID: source.RecipeIngredientLineID,
			QuantityContributed:    source.QuantityContributed, UnitContributed: source.UnitContributed,
			Variant: source.VariantText, LineRecipeName: source.LineRecipeName,
		})
	}
	itemsBy := map[uuid.UUID][]GroceryItem{}
	for _, it := range items {
		itemsBy[it.ListID] = append(itemsBy[it.ListID], GroceryItem{
			ID: it.ID, Name: it.Name, Amount: it.Amount, Unit: it.Unit,
			Checked: it.Checked, Note: it.Note, SortOrder: it.SortOrder,
			IngredientID: it.IngredientID, SourceType: it.SourceType, Sources: sourcesBy[it.ID],
			Variant: it.VariantText, DeletedAt: timestampPtr(it.DeletedAt),
			CreatedBy: it.CreatedBy, UpdatedBy: it.UpdatedBy,
		})
	}
	for _, l := range lists {
		arc.GroceryLists = append(arc.GroceryLists, GroceryList{
			ID: l.ID, Name: l.Name, CreatedAt: l.CreatedAt, DeletedAt: timestampPtr(l.DeletedAt),
			CreatedBy: l.CreatedBy, UpdatedBy: l.UpdatedBy, Items: itemsBy[l.ID],
		})
	}

	sessions, err := q.ExportPrepSessions(ctx, householdID)
	if err != nil {
		return nil, err
	}
	pmeals, err := q.ExportPrepSessionMeals(ctx, householdID)
	if err != nil {
		return nil, err
	}
	mealsBy := map[uuid.UUID][]PrepMeal{}
	for _, pm := range pmeals {
		mealsBy[pm.SessionID] = append(mealsBy[pm.SessionID], PrepMeal{
			FoodID: pm.FoodID, Servings: pm.Servings, SortOrder: pm.SortOrder,
		})
	}
	for _, ps := range sessions {
		arc.PrepSessions = append(arc.PrepSessions, PrepSession{
			ID: ps.ID, Name: ps.Name, SessionDate: ps.SessionDate,
			CreatedAt: ps.CreatedAt, Meals: mealsBy[ps.ID],
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return arc, nil
}

// Import merges the selected sections of an archive by stable UUID. It runs in a
// single transaction; rows whose foreign keys cannot be resolved (e.g. a meal
// referencing a food that is neither present in the DB nor in an imported foods
// section) are skipped and reported rather than aborting the whole import
// (FR15.2, FR15.3, FR15.5). An unsupported schema_version is rejected (FR15.4).
func (s *Service) Import(ctx context.Context, householdID uuid.UUID, arc *Archive, sections map[string]bool) (*Report, error) {
	if !supportsSchemaVersion(arc.SchemaVersion) {
		return nil, fmt.Errorf("unsupported archive schema_version %d (this build reads versions %d through %d)", arc.SchemaVersion, minSchemaVersion, SchemaVersion)
	}
	if err := arc.ValidateURLs(); err != nil {
		return nil, fmt.Errorf("archive contains an invalid URL: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	owners, err := q.TransferIDOwners(ctx, archiveStableIDs(arc))
	if err != nil {
		return nil, err
	}
	ownership := newOwnerIndex(owners)

	// Resolve foreign-key targets: rows already in this household, plus rows about
	// to be inserted by a selected section.
	knownFoods, err := idSet(q.ExistingFoodIDs(ctx, householdID))
	if err != nil {
		return nil, err
	}
	knownSeries, err := idSet(q.ExistingMealSeriesIDs(ctx, householdID))
	if err != nil {
		return nil, err
	}
	knownMeals, err := idSet(q.ExistingMealPlanIDs(ctx, householdID))
	if err != nil {
		return nil, err
	}
	knownComponents, err := idSet(q.ExistingFoodComponentIDs(ctx, householdID))
	if err != nil {
		return nil, err
	}
	knownAccounts, err := idSet(q.ExistingHouseholdAccountIDs(ctx, householdID))
	if err != nil {
		return nil, err
	}

	rep := &Report{}
	scopedResult := func(inserted bool, importErr error, sr *SectionReport, entity string) (bool, error) {
		if errors.Is(importErr, pgx.ErrNoRows) {
			sr.skip(entity + " was skipped because its UUID is owned outside this household")
			return false, nil
		}
		if importErr != nil {
			return false, importErr
		}
		sr.count(inserted)
		return true, nil
	}
	scopedChild := func(importErr error, sr *SectionReport, entity string) (bool, error) {
		if errors.Is(importErr, pgx.ErrNoRows) {
			sr.skip(entity + " was skipped because its UUID or parent is owned outside this household")
			return false, nil
		}
		return importErr == nil, importErr
	}

	if sections[SectionFoods] {
		sr := SectionReport{Name: SectionFoods}
		// Insert every food first so components can reference any other food.
		for _, f := range arc.Foods {
			if ownership.ownedElsewhere("food", f.ID, householdID) {
				sr.skip(fmt.Sprintf("food %q (%s) has a UUID owned by another household", f.Name, f.ID))
				continue
			}
			var ins bool
			var err error
			if arc.SchemaVersion == 1 {
				ins, err = q.ImportFoodV1(ctx, db.ImportFoodV1Params{
					ID: f.ID, HouseholdID: householdID, Name: f.Name, Description: f.Description,
					PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin, Servings: f.Servings,
					DefaultUnit: f.DefaultUnit, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
					DensityGPerMl: f.DensityGPerMl, DensitySource: densitySourceOrNone(f.DensitySource),
				})
			} else {
				ins, err = q.ImportFoodV2(ctx, db.ImportFoodV2Params{
					ID: f.ID, HouseholdID: householdID, Name: f.Name, Description: f.Description,
					PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin, Servings: f.Servings,
					DefaultUnit: f.DefaultUnit, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
					DensityGPerMl: f.DensityGPerMl, DensitySource: densitySourceOrNone(f.DensitySource),
					SourceUrl: stringPtr(f.SourceURL), SourceLastImportedAt: nullableTimestamp(f.SourceLastImportedAt),
					DeletedAt: nullableTimestamp(f.DeletedAt), CreatedBy: scopedAuthor(f.CreatedBy, knownAccounts),
					UpdatedBy: scopedAuthor(f.UpdatedBy, knownAccounts),
				})
			}
			ok, err := scopedResult(ins, err, &sr, fmt.Sprintf("food %q (%s)", f.Name, f.ID))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			knownFoods[f.ID] = true
			// Replace tags/steps for a clean merge of the food's child rows.
			if err := q.DeleteFoodTags(ctx, f.ID); err != nil {
				return nil, err
			}
			for _, t := range f.Tags {
				if err := q.ImportFoodTag(ctx, db.ImportFoodTagParams{FoodID: f.ID, Tag: t}); err != nil {
					return nil, err
				}
			}
			// Aliases did not exist in v1; old archives must leave them untouched.
			if arc.SchemaVersion >= 2 {
				if reason := aliasReplacementIssue(f, ownership, householdID); reason != "" {
					sr.skip(reason)
				} else {
					if err := q.DeleteFoodAliases(ctx, f.ID); err != nil {
						return nil, err
					}
					for _, alias := range f.Aliases {
						_, importErr := q.ImportFoodAlias(ctx, db.ImportFoodAliasParams{
							ID: alias.ID, FoodID: f.ID, Alias: alias.Alias,
							CreatedAt: alias.CreatedAt, CreatedBy: scopedAuthor(alias.CreatedBy, knownAccounts),
							HouseholdID: householdID,
						})
						if _, err := scopedChild(importErr, &sr, fmt.Sprintf("alias %q (%s)", alias.Alias, alias.ID)); err != nil {
							return nil, err
						}
					}
				}
			}
			if err := q.DeleteFoodSteps(ctx, f.ID); err != nil {
				return nil, err
			}
			for _, st := range f.Steps {
				if err := q.ImportFoodStep(ctx, db.ImportFoodStepParams{
					FoodID: f.ID, StepNumber: st.StepNumber, Instruction: st.Instruction,
				}); err != nil {
					return nil, err
				}
			}
		}
		// Now components, skipping any whose child food is unknown.
		for _, f := range arc.Foods {
			if !knownFoods[f.ID] || ownership.ownedElsewhere("food", f.ID, householdID) {
				continue
			}
			if reason := componentReplacementIssue(f, ownership, knownFoods, householdID); reason != "" {
				sr.skip(reason)
				continue
			}
			if arc.SchemaVersion >= 2 {
				if err := q.DeleteFoodComponents(ctx, f.ID); err != nil {
					return nil, err
				}
			}
			componentIDs := make([]uuid.UUID, 0, len(f.Components))
			for _, c := range f.Components {
				componentIDs = append(componentIDs, c.ID)
				var importErr error
				if arc.SchemaVersion == 1 {
					_, importErr = q.ImportFoodComponentV1(ctx, db.ImportFoodComponentV1Params{
						ID: c.ID, ParentFoodID: f.ID, ChildFoodID: c.ChildFoodID,
						Amount: c.Amount, Unit: c.Unit, SortOrder: c.SortOrder, HouseholdID: householdID,
					})
				} else {
					_, importErr = q.ImportFoodComponentV2(ctx, db.ImportFoodComponentV2Params{
						ID: c.ID, ParentFoodID: f.ID, ChildFoodID: c.ChildFoodID,
						Amount: c.Amount, Unit: c.Unit, VariantText: c.Variant, SortOrder: c.SortOrder,
						HouseholdID: householdID,
					})
				}
				if _, err := scopedChild(importErr, &sr, fmt.Sprintf("food %q component %s", f.Name, c.ID)); err != nil {
					return nil, err
				}
			}
			if arc.SchemaVersion == 1 {
				if err := q.DeleteImportedFoodComponentsExcept(ctx, db.DeleteImportedFoodComponentsExceptParams{
					ParentFoodID: f.ID, HouseholdID: householdID, KeepIds: componentIDs,
				}); err != nil {
					return nil, err
				}
			}
		}
		knownComponents, err = idSet(q.ExistingFoodComponentIDs(ctx, householdID))
		if err != nil {
			return nil, err
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionMeals] {
		sr := SectionReport{Name: SectionMeals}
		for _, m := range arc.MealSeries {
			if ownership.ownedElsewhere("meal_series", m.ID, householdID) {
				sr.skip(fmt.Sprintf("meal series %s has a UUID owned by another household", m.ID))
				continue
			}
			if !knownFoods[m.FoodID] {
				sr.skip(fmt.Sprintf("meal series references unknown food %s", m.FoodID))
				continue
			}
			ins, err := q.ImportMealSeries(ctx, db.ImportMealSeriesParams{
				ID: m.ID, HouseholdID: householdID, FoodID: m.FoodID, PlanTime: m.PlanTime, Servings: m.Servings,
				Freq: m.Freq, Byweekday: m.Byweekday, StartDate: m.StartDate, UntilDate: m.UntilDate,
			})
			ok, err := scopedResult(ins, err, &sr, fmt.Sprintf("meal series %s", m.ID))
			if err != nil {
				return nil, err
			}
			if ok {
				knownSeries[m.ID] = true
			}
		}
		for _, m := range arc.MealPlans {
			if ownership.ownedElsewhere("meal_plan", m.ID, householdID) {
				sr.skip(fmt.Sprintf("meal %s has a UUID owned by another household", m.ID))
				continue
			}
			if !knownFoods[m.FoodID] {
				sr.skip(fmt.Sprintf("meal on %s references unknown food %s", m.PlanDate.Format("2006-01-02"), m.FoodID))
				continue
			}
			series := m.SeriesID
			if series != nil && !knownSeries[*series] {
				series = nil // keep the meal but drop the dangling series link
			}
			var ins bool
			var err error
			if arc.SchemaVersion == 1 {
				ins, err = q.ImportMealPlanV1(ctx, db.ImportMealPlanV1Params{
					ID: m.ID, HouseholdID: householdID, PlanDate: m.PlanDate, PlanTime: m.PlanTime,
					FoodID: m.FoodID, Servings: m.Servings, SeriesID: series,
					LinkUrl: m.LinkURL, LinkTitle: m.LinkTitle, LinkImageUrl: m.LinkImageURL,
				})
			} else {
				ins, err = q.ImportMealPlanV2(ctx, db.ImportMealPlanV2Params{
					ID: m.ID, HouseholdID: householdID, PlanDate: m.PlanDate, PlanTime: m.PlanTime,
					FoodID: m.FoodID, Servings: m.Servings, SeriesID: series,
					LinkUrl: m.LinkURL, LinkTitle: m.LinkTitle, LinkImageUrl: m.LinkImageURL,
					Title: m.Title, Notes: m.Notes, Version: m.Version,
					DeletedAt: nullableTimestamp(m.DeletedAt), CreatedBy: scopedAuthor(m.CreatedBy, knownAccounts),
					UpdatedBy: scopedAuthor(m.UpdatedBy, knownAccounts),
				})
			}
			ok, err := scopedResult(ins, err, &sr, fmt.Sprintf("meal %s", m.ID))
			if err != nil {
				return nil, err
			}
			if ok {
				knownMeals[m.ID] = true
			}
			if !ok || arc.SchemaVersion < 2 {
				continue
			}
			if reason := scheduledRecipeReplacementIssue(m, ownership, knownFoods, householdID); reason != "" {
				sr.skip(reason)
				continue
			}
			if err := q.DeleteImportedScheduledMealRecipes(ctx, db.DeleteImportedScheduledMealRecipesParams{
				MealID: m.ID, HouseholdID: householdID,
			}); err != nil {
				return nil, err
			}
			for _, recipe := range m.Recipes {
				_, importErr := q.ImportScheduledMealRecipe(ctx, db.ImportScheduledMealRecipeParams{
					ID: recipe.ID, MealID: m.ID, FoodID: recipe.FoodID,
					ServingsOverride: recipe.ServingsOverride, SortOrder: recipe.SortOrder,
					CreatedBy: scopedAuthor(recipe.CreatedBy, knownAccounts),
					UpdatedBy: scopedAuthor(recipe.UpdatedBy, knownAccounts), HouseholdID: householdID,
				})
				if _, err := scopedChild(importErr, &sr, fmt.Sprintf("meal %s recipe %s", m.ID, recipe.ID)); err != nil {
					return nil, err
				}
			}
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionGrocery] {
		sr := SectionReport{Name: SectionGrocery}
		for _, l := range arc.GroceryLists {
			if ownership.ownedElsewhere("grocery_list", l.ID, householdID) {
				sr.skip(fmt.Sprintf("grocery list %q (%s) has a UUID owned by another household", l.Name, l.ID))
				continue
			}
			var ins bool
			var err error
			if arc.SchemaVersion == 1 {
				ins, err = q.ImportGroceryListV1(ctx, db.ImportGroceryListV1Params{
					ID: l.ID, HouseholdID: householdID, Name: l.Name, CreatedAt: l.CreatedAt,
				})
			} else {
				ins, err = q.ImportGroceryListV2(ctx, db.ImportGroceryListV2Params{
					ID: l.ID, HouseholdID: householdID, Name: l.Name, CreatedAt: l.CreatedAt,
					DeletedAt: nullableTimestamp(l.DeletedAt), CreatedBy: scopedAuthor(l.CreatedBy, knownAccounts),
					UpdatedBy: scopedAuthor(l.UpdatedBy, knownAccounts),
				})
			}
			ok, err := scopedResult(ins, err, &sr, fmt.Sprintf("grocery list %q (%s)", l.Name, l.ID))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			for _, it := range l.Items {
				if ownership.ownedElsewhere("grocery_item", it.ID, householdID) {
					sr.skip(fmt.Sprintf("grocery item %q (%s) has a UUID owned by another household", it.Name, it.ID))
					continue
				}
				if arc.SchemaVersion >= 2 && it.IngredientID != nil && !knownFoods[*it.IngredientID] {
					sr.skip(fmt.Sprintf("grocery item %q references unavailable ingredient %s", it.Name, *it.IngredientID))
					continue
				}
				var importErr error
				if arc.SchemaVersion == 1 {
					_, importErr = q.ImportGroceryItemV1(ctx, db.ImportGroceryItemV1Params{
						ID: it.ID, ListID: l.ID, Name: it.Name, Amount: it.Amount, Unit: it.Unit,
						Checked: it.Checked, Note: it.Note, SortOrder: it.SortOrder, HouseholdID: householdID,
					})
				} else {
					_, importErr = q.ImportGroceryItemV2(ctx, db.ImportGroceryItemV2Params{
						ID: it.ID, ListID: l.ID, Name: it.Name, Amount: it.Amount, Unit: it.Unit,
						Checked: it.Checked, Note: it.Note, SortOrder: it.SortOrder,
						IngredientID: it.IngredientID, SourceType: sourceTypeOrAdhoc(it.SourceType),
						VariantText: it.Variant, DeletedAt: nullableTimestamp(it.DeletedAt),
						CreatedBy: scopedAuthor(it.CreatedBy, knownAccounts), UpdatedBy: scopedAuthor(it.UpdatedBy, knownAccounts),
						HouseholdID: householdID,
					})
				}
				itemOK, err := scopedChild(importErr, &sr, fmt.Sprintf("grocery item %q (%s)", it.Name, it.ID))
				if err != nil {
					return nil, err
				}
				if !itemOK {
					continue
				}
				if arc.SchemaVersion < 2 {
					continue
				}
				if reason := sourceReplacementIssue(it, ownership, knownMeals, knownFoods, knownComponents, householdID); reason != "" {
					sr.skip(reason)
					continue
				}
				if err := q.DeleteImportedGroceryItemSources(ctx, db.DeleteImportedGroceryItemSourcesParams{
					GroceryItemID: it.ID, HouseholdID: householdID,
				}); err != nil {
					return nil, err
				}
				for _, source := range it.Sources {
					_, importErr := q.ImportGroceryItemSource(ctx, db.ImportGroceryItemSourceParams{
						ID: source.ID, GroceryItemID: it.ID, ScheduledMealID: source.ScheduledMealID,
						RecipeID: source.RecipeID, RecipeIngredientLineID: source.RecipeIngredientLineID,
						QuantityContributed: source.QuantityContributed, UnitContributed: source.UnitContributed,
						VariantText: source.Variant, LineRecipeName: source.LineRecipeName,
						HouseholdID: householdID,
					})
					if _, err := scopedChild(importErr, &sr, fmt.Sprintf("grocery item %q source %s", it.Name, source.ID)); err != nil {
						return nil, err
					}
				}
			}
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionPrep] {
		sr := SectionReport{Name: SectionPrep}
		for _, ps := range arc.PrepSessions {
			if ownership.ownedElsewhere("prep_session", ps.ID, householdID) {
				sr.skip(fmt.Sprintf("prep session %q (%s) has a UUID owned by another household", ps.Name, ps.ID))
				continue
			}
			ins, err := q.ImportPrepSession(ctx, db.ImportPrepSessionParams{
				ID: ps.ID, HouseholdID: householdID, Name: ps.Name, SessionDate: ps.SessionDate, CreatedAt: ps.CreatedAt,
			})
			ok, err := scopedResult(ins, err, &sr, fmt.Sprintf("prep session %q (%s)", ps.Name, ps.ID))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			for _, pm := range ps.Meals {
				if !knownFoods[pm.FoodID] {
					sr.skip(fmt.Sprintf("prep session %q references unknown food %s", ps.Name, pm.FoodID))
					continue
				}
				if err := q.ImportPrepSessionMeal(ctx, db.ImportPrepSessionMealParams{
					SessionID: ps.ID, FoodID: pm.FoodID, Servings: pm.Servings, SortOrder: pm.SortOrder,
					HouseholdID: householdID,
				}); err != nil {
					return nil, err
				}
			}
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return rep, nil
}

func (sr *SectionReport) count(inserted bool) {
	if inserted {
		sr.Inserted++
	} else {
		sr.Updated++
	}
}

func (sr *SectionReport) skip(reason string) {
	sr.Skipped++
	// Cap the noise from a pathological import.
	if len(sr.Notes) < 25 {
		sr.Notes = append(sr.Notes, reason)
	}
}

func nowUTC() time.Time { return time.Now().UTC() }

func idSet(ids []uuid.UUID, err error) (map[uuid.UUID]bool, error) {
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m, nil
}

func supportsSchemaVersion(version int) bool {
	return version >= minSchemaVersion && version <= SchemaVersion
}

type ownerIndex map[string]map[uuid.UUID]uuid.UUID

func newOwnerIndex(rows []db.TransferIDOwnersRow) ownerIndex {
	index := ownerIndex{}
	for _, row := range rows {
		if index[row.EntityType] == nil {
			index[row.EntityType] = map[uuid.UUID]uuid.UUID{}
		}
		index[row.EntityType][row.ID] = row.HouseholdID
	}
	return index
}

func (index ownerIndex) ownedElsewhere(entityType string, id, householdID uuid.UUID) bool {
	owner, exists := index[entityType][id]
	return exists && owner != householdID
}

func archiveStableIDs(arc *Archive) []uuid.UUID {
	ids := map[uuid.UUID]bool{}
	add := func(id uuid.UUID) { ids[id] = true }
	addPtr := func(id *uuid.UUID) {
		if id != nil {
			add(*id)
		}
	}
	for _, food := range arc.Foods {
		add(food.ID)
		for _, alias := range food.Aliases {
			add(alias.ID)
		}
		for _, component := range food.Components {
			add(component.ID)
			add(component.ChildFoodID)
		}
	}
	for _, series := range arc.MealSeries {
		add(series.ID)
		add(series.FoodID)
	}
	for _, meal := range arc.MealPlans {
		add(meal.ID)
		add(meal.FoodID)
		addPtr(meal.SeriesID)
		for _, recipe := range meal.Recipes {
			add(recipe.ID)
			add(recipe.FoodID)
		}
	}
	for _, list := range arc.GroceryLists {
		add(list.ID)
		for _, item := range list.Items {
			add(item.ID)
			addPtr(item.IngredientID)
			for _, source := range item.Sources {
				add(source.ID)
				addPtr(source.ScheduledMealID)
				addPtr(source.RecipeID)
				addPtr(source.RecipeIngredientLineID)
			}
		}
	}
	for _, session := range arc.PrepSessions {
		add(session.ID)
		for _, meal := range session.Meals {
			add(meal.FoodID)
		}
	}
	out := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out
}

func unavailableSourceReference(source GroceryItemSource, meals, foods, components map[uuid.UUID]bool) string {
	if source.ScheduledMealID != nil && !meals[*source.ScheduledMealID] {
		return "meal " + source.ScheduledMealID.String()
	}
	if source.RecipeID != nil && !foods[*source.RecipeID] {
		return "recipe " + source.RecipeID.String()
	}
	if source.RecipeIngredientLineID != nil && !components[*source.RecipeIngredientLineID] {
		return "ingredient line " + source.RecipeIngredientLineID.String()
	}
	return ""
}

func aliasReplacementIssue(food Food, ownership ownerIndex, householdID uuid.UUID) string {
	for _, alias := range food.Aliases {
		if ownership.ownedElsewhere("food_alias", alias.ID, householdID) {
			return fmt.Sprintf("food %q aliases were not replaced because alias %q (%s) is owned by another household", food.Name, alias.Alias, alias.ID)
		}
	}
	return ""
}

func componentReplacementIssue(food Food, ownership ownerIndex, foods map[uuid.UUID]bool, householdID uuid.UUID) string {
	for _, component := range food.Components {
		if ownership.ownedElsewhere("food_component", component.ID, householdID) {
			return fmt.Sprintf("food %q components were not replaced because component %s is owned by another household", food.Name, component.ID)
		}
		if !foods[component.ChildFoodID] {
			return fmt.Sprintf("food %q components were not replaced because component %s references unknown food %s", food.Name, component.ID, component.ChildFoodID)
		}
	}
	return ""
}

func scheduledRecipeReplacementIssue(meal MealPlan, ownership ownerIndex, foods map[uuid.UUID]bool, householdID uuid.UUID) string {
	for _, recipe := range meal.Recipes {
		if ownership.ownedElsewhere("scheduled_meal_recipe", recipe.ID, householdID) {
			return fmt.Sprintf("meal %s recipes were not replaced because recipe link %s is owned by another household", meal.ID, recipe.ID)
		}
		if !foods[recipe.FoodID] {
			return fmt.Sprintf("meal %s recipes were not replaced because recipe link %s references unknown food %s", meal.ID, recipe.ID, recipe.FoodID)
		}
	}
	return ""
}

func sourceReplacementIssue(item GroceryItem, ownership ownerIndex, meals, foods, components map[uuid.UUID]bool, householdID uuid.UUID) string {
	for _, source := range item.Sources {
		if ownership.ownedElsewhere("grocery_item_source", source.ID, householdID) {
			return fmt.Sprintf("grocery item %q sources were not replaced because source %s is owned by another household", item.Name, source.ID)
		}
		if reason := unavailableSourceReference(source, meals, foods, components); reason != "" {
			return fmt.Sprintf("grocery item %q sources were not replaced because source %s references unavailable %s", item.Name, source.ID, reason)
		}
	}
	return ""
}

func scopedAuthor(author *uuid.UUID, accounts map[uuid.UUID]bool) *uuid.UUID {
	if author == nil || !accounts[*author] {
		return nil
	}
	return author
}

func densitySourceOrNone(s string) string {
	switch s {
	case "starter", "custom":
		return s
	default:
		return "none"
	}
}

func sourceTypeOrAdhoc(s string) string {
	if s == "generated" {
		return s
	}
	return "adhoc"
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func timestampPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	return &v.Time
}

func nullableTimestamp(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *v, Valid: true}
}
