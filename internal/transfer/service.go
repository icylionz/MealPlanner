package transfer

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// Export reads every household entity into a single archive.
func (s *Service) Export(ctx context.Context) (*Archive, error) {
	arc := &Archive{SchemaVersion: SchemaVersion, ExportedAt: nowUTC()}

	members, err := s.q.ExportMembers(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range members {
		arc.Members = append(arc.Members, Member{
			ID: m.ID, Name: m.Name, Role: m.Role, Initials: m.Initials,
			Color: m.Color, CreatedAt: m.CreatedAt,
		})
	}

	foods, err := s.q.ExportFoods(ctx)
	if err != nil {
		return nil, err
	}
	tags, err := s.q.ExportFoodTags(ctx)
	if err != nil {
		return nil, err
	}
	comps, err := s.q.ExportFoodComponents(ctx)
	if err != nil {
		return nil, err
	}
	steps, err := s.q.ExportFoodSteps(ctx)
	if err != nil {
		return nil, err
	}
	tagsBy := map[uuid.UUID][]string{}
	for _, t := range tags {
		tagsBy[t.FoodID] = append(tagsBy[t.FoodID], t.Tag)
	}
	compsBy := map[uuid.UUID][]Component{}
	for _, c := range comps {
		compsBy[c.ParentFoodID] = append(compsBy[c.ParentFoodID], Component{
			ID: c.ID, ChildFoodID: c.ChildFoodID, Amount: c.Amount,
			Unit: c.Unit, SortOrder: c.SortOrder,
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
			CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
			Tags: tagsBy[f.ID], Components: compsBy[f.ID], Steps: stepsBy[f.ID],
		})
	}

	series, err := s.q.ExportMealSeries(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range series {
		arc.MealSeries = append(arc.MealSeries, MealSeries{
			ID: m.ID, FoodID: m.FoodID, PlanTime: m.PlanTime, Servings: m.Servings,
			Freq: m.Freq, Byweekday: m.Byweekday, StartDate: m.StartDate, UntilDate: m.UntilDate,
		})
	}

	plans, err := s.q.ExportMealPlans(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range plans {
		arc.MealPlans = append(arc.MealPlans, MealPlan{
			ID: m.ID, PlanDate: m.PlanDate, PlanTime: m.PlanTime,
			FoodID: m.FoodID, Servings: m.Servings, SeriesID: m.SeriesID,
			LinkURL: m.LinkUrl, LinkTitle: m.LinkTitle, LinkImageURL: m.LinkImageUrl,
		})
	}

	lists, err := s.q.ExportGroceryLists(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.q.ExportGroceryItems(ctx)
	if err != nil {
		return nil, err
	}
	itemsBy := map[uuid.UUID][]GroceryItem{}
	for _, it := range items {
		itemsBy[it.ListID] = append(itemsBy[it.ListID], GroceryItem{
			ID: it.ID, Name: it.Name, Amount: it.Amount, Unit: it.Unit,
			Checked: it.Checked, Note: it.Note, SortOrder: it.SortOrder,
		})
	}
	for _, l := range lists {
		arc.GroceryLists = append(arc.GroceryLists, GroceryList{
			ID: l.ID, Name: l.Name, CreatedAt: l.CreatedAt, Items: itemsBy[l.ID],
		})
	}

	sessions, err := s.q.ExportPrepSessions(ctx)
	if err != nil {
		return nil, err
	}
	pmeals, err := s.q.ExportPrepSessionMeals(ctx)
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

	return arc, nil
}

// Import merges the selected sections of an archive by stable UUID. It runs in a
// single transaction; rows whose foreign keys cannot be resolved (e.g. a meal
// referencing a food that is neither present in the DB nor in an imported foods
// section) are skipped and reported rather than aborting the whole import
// (FR15.2, FR15.3, FR15.5). An unsupported schema_version is rejected (FR15.4).
func (s *Service) Import(ctx context.Context, arc *Archive, sections map[string]bool) (*Report, error) {
	if arc.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported archive schema_version %d (this build reads version %d)", arc.SchemaVersion, SchemaVersion)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)

	// Resolve foreign-key targets: rows already in the DB, plus rows about to be
	// inserted by a selected section.
	knownFoods, err := idSet(s.q.ExistingFoodIDs(ctx))
	if err != nil {
		return nil, err
	}
	knownSeries, err := idSet(s.q.ExistingMealSeriesIDs(ctx))
	if err != nil {
		return nil, err
	}
	if sections[SectionFoods] {
		for _, f := range arc.Foods {
			knownFoods[f.ID] = true
		}
	}
	if sections[SectionMeals] {
		for _, m := range arc.MealSeries {
			knownSeries[m.ID] = true
		}
	}

	rep := &Report{}

	if sections[SectionMembers] {
		sr := SectionReport{Name: SectionMembers}
		for _, m := range arc.Members {
			ins, err := q.ImportMember(ctx, db.ImportMemberParams{
				ID: m.ID, Name: m.Name, Role: roleOrMember(m.Role),
				Initials: m.Initials, Color: m.Color, CreatedAt: m.CreatedAt,
			})
			if err != nil {
				return nil, err
			}
			sr.count(ins)
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionFoods] {
		sr := SectionReport{Name: SectionFoods}
		// Insert every food first so components can reference any other food.
		for _, f := range arc.Foods {
			ins, err := q.ImportFood(ctx, db.ImportFoodParams{
				ID: f.ID, Name: f.Name, Description: f.Description,
				PrepTimeMin: f.PrepTimeMin, CookTimeMin: f.CookTimeMin, Servings: f.Servings,
				DefaultUnit: f.DefaultUnit, CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
				DensityGPerMl: f.DensityGPerMl, DensitySource: densitySourceOrNone(f.DensitySource),
			})
			if err != nil {
				return nil, err
			}
			sr.count(ins)
			// Replace tags/steps for a clean merge of the food's child rows.
			if err := q.DeleteFoodTags(ctx, f.ID); err != nil {
				return nil, err
			}
			for _, t := range f.Tags {
				if err := q.ImportFoodTag(ctx, db.ImportFoodTagParams{FoodID: f.ID, Tag: t}); err != nil {
					return nil, err
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
			if err := q.DeleteFoodComponents(ctx, f.ID); err != nil {
				return nil, err
			}
			for _, c := range f.Components {
				if !knownFoods[c.ChildFoodID] {
					sr.skip(fmt.Sprintf("food %q component references unknown food %s", f.Name, c.ChildFoodID))
					continue
				}
				if _, err := q.ImportFoodComponent(ctx, db.ImportFoodComponentParams{
					ID: c.ID, ParentFoodID: f.ID, ChildFoodID: c.ChildFoodID,
					Amount: c.Amount, Unit: c.Unit, SortOrder: c.SortOrder,
				}); err != nil {
					return nil, err
				}
			}
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionMeals] {
		sr := SectionReport{Name: SectionMeals}
		for _, m := range arc.MealSeries {
			if !knownFoods[m.FoodID] {
				sr.skip(fmt.Sprintf("meal series references unknown food %s", m.FoodID))
				continue
			}
			ins, err := q.ImportMealSeries(ctx, db.ImportMealSeriesParams{
				ID: m.ID, FoodID: m.FoodID, PlanTime: m.PlanTime, Servings: m.Servings,
				Freq: m.Freq, Byweekday: m.Byweekday, StartDate: m.StartDate, UntilDate: m.UntilDate,
			})
			if err != nil {
				return nil, err
			}
			sr.count(ins)
		}
		for _, m := range arc.MealPlans {
			if !knownFoods[m.FoodID] {
				sr.skip(fmt.Sprintf("meal on %s references unknown food %s", m.PlanDate.Format("2006-01-02"), m.FoodID))
				continue
			}
			series := m.SeriesID
			if series != nil && !knownSeries[*series] {
				series = nil // keep the meal but drop the dangling series link
			}
			ins, err := q.ImportMealPlan(ctx, db.ImportMealPlanParams{
				ID: m.ID, PlanDate: m.PlanDate, PlanTime: m.PlanTime,
				FoodID: m.FoodID, Servings: m.Servings, SeriesID: series,
				LinkUrl: m.LinkURL, LinkTitle: m.LinkTitle, LinkImageUrl: m.LinkImageURL,
			})
			if err != nil {
				return nil, err
			}
			sr.count(ins)
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionGrocery] {
		sr := SectionReport{Name: SectionGrocery}
		for _, l := range arc.GroceryLists {
			ins, err := q.ImportGroceryList(ctx, db.ImportGroceryListParams{
				ID: l.ID, Name: l.Name, CreatedAt: l.CreatedAt,
			})
			if err != nil {
				return nil, err
			}
			sr.count(ins)
			for _, it := range l.Items {
				if _, err := q.ImportGroceryItem(ctx, db.ImportGroceryItemParams{
					ID: it.ID, ListID: l.ID, Name: it.Name, Amount: it.Amount, Unit: it.Unit,
					Checked: it.Checked, Note: it.Note, SortOrder: it.SortOrder,
				}); err != nil {
					return nil, err
				}
			}
		}
		rep.Sections = append(rep.Sections, sr)
	}

	if sections[SectionPrep] {
		sr := SectionReport{Name: SectionPrep}
		for _, ps := range arc.PrepSessions {
			ins, err := q.ImportPrepSession(ctx, db.ImportPrepSessionParams{
				ID: ps.ID, Name: ps.Name, SessionDate: ps.SessionDate, CreatedAt: ps.CreatedAt,
			})
			if err != nil {
				return nil, err
			}
			sr.count(ins)
			for _, pm := range ps.Meals {
				if !knownFoods[pm.FoodID] {
					sr.skip(fmt.Sprintf("prep session %q references unknown food %s", ps.Name, pm.FoodID))
					continue
				}
				if err := q.ImportPrepSessionMeal(ctx, db.ImportPrepSessionMealParams{
					SessionID: ps.ID, FoodID: pm.FoodID, Servings: pm.Servings, SortOrder: pm.SortOrder,
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

func roleOrMember(role string) string {
	if role == "owner" {
		return "owner"
	}
	return "member"
}

func densitySourceOrNone(s string) string {
	switch s {
	case "starter", "custom":
		return s
	default:
		return "none"
	}
}
