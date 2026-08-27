// Package transfer implements full-fidelity JSON export and import of all
// household data (FR15). The archive carries a schema_version and stable UUIDs
// so a later import merges by ID rather than duplicating.
package transfer

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"mealplanner/internal/weburl"
)

// SchemaVersion is the format written by this build. Version 2 adds G1-G6
// metadata and relationships; version 1 remains readable.
const (
	SchemaVersion    = 2
	minSchemaVersion = 1
)

// Section names selectable at import time (FR15.3). A section groups a parent
// entity with its children (e.g. "foods" also carries tags, components, steps).
const (
	SectionFoods   = "foods"
	SectionMeals   = "meals"
	SectionGrocery = "grocery"
	SectionPrep    = "prep"
)

// AllSections lists every importable/exportable section in a stable order.
// Membership/accounts are not part of an archive — they are managed via invites.
var AllSections = []string{SectionFoods, SectionMeals, SectionGrocery, SectionPrep}

// Archive is the top-level export document, scoped to one household.
type Archive struct {
	SchemaVersion int           `json:"schema_version"`
	ExportedAt    time.Time     `json:"exported_at"`
	Foods         []Food        `json:"foods,omitempty"`
	MealSeries    []MealSeries  `json:"meal_series,omitempty"`
	MealPlans     []MealPlan    `json:"meal_plans,omitempty"`
	GroceryLists  []GroceryList `json:"grocery_lists,omitempty"`
	PrepSessions  []PrepSession `json:"prep_sessions,omitempty"`
}

// ValidateURLs rejects unsafe URLs before an archive reaches transfer logic.
func (a *Archive) ValidateURLs() error {
	for i, food := range a.Foods {
		if food.SourceURL != "" {
			if _, err := weburl.ParseHTTP(food.SourceURL); err != nil {
				return fmt.Errorf("foods[%d].source_url: %w", i, err)
			}
		}
	}
	for i, meal := range a.MealPlans {
		if meal.LinkURL != "" {
			if _, err := weburl.ParseHTTP(meal.LinkURL); err != nil {
				return fmt.Errorf("meal_plans[%d].link_url: %w", i, err)
			}
		}
		if meal.LinkImageURL != "" {
			if _, err := weburl.ParseHTTP(meal.LinkImageURL); err != nil {
				return fmt.Errorf("meal_plans[%d].link_image_url: %w", i, err)
			}
		}
	}
	return nil
}

type Food struct {
	ID                   uuid.UUID   `json:"id"`
	Name                 string      `json:"name"`
	Description          string      `json:"description"`
	PrepTimeMin          int32       `json:"prep_time_min"`
	CookTimeMin          int32       `json:"cook_time_min"`
	Servings             int32       `json:"servings"`
	DefaultUnit          string      `json:"default_unit"`
	DensityGPerMl        float64     `json:"density_g_per_ml"`
	DensitySource        string      `json:"density_source"`
	SourceURL            string      `json:"source_url,omitempty"`
	SourceLastImportedAt *time.Time  `json:"source_last_imported_at,omitempty"`
	CreatedAt            time.Time   `json:"created_at"`
	UpdatedAt            time.Time   `json:"updated_at"`
	DeletedAt            *time.Time  `json:"deleted_at,omitempty"`
	CreatedBy            *uuid.UUID  `json:"created_by,omitempty"`
	UpdatedBy            *uuid.UUID  `json:"updated_by,omitempty"`
	Aliases              []Alias     `json:"aliases,omitempty"`
	Tags                 []string    `json:"tags,omitempty"`
	Components           []Component `json:"components,omitempty"`
	Steps                []Step      `json:"steps,omitempty"`
}

type Alias struct {
	ID        uuid.UUID  `json:"id"`
	Alias     string     `json:"alias"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Component's parent food is implied by the food that nests it.
type Component struct {
	ID          uuid.UUID `json:"id"`
	ChildFoodID uuid.UUID `json:"child_food_id"`
	Amount      float64   `json:"amount"`
	Unit        string    `json:"unit"`
	Variant     string    `json:"variant,omitempty"`
	SortOrder   int32     `json:"sort_order"`
}

type Step struct {
	StepNumber  int32  `json:"step_number"`
	Instruction string `json:"instruction"`
}

type MealSeries struct {
	ID        uuid.UUID `json:"id"`
	FoodID    uuid.UUID `json:"food_id"`
	PlanTime  string    `json:"plan_time"`
	Servings  int32     `json:"servings"`
	Freq      string    `json:"freq"`
	Byweekday string    `json:"byweekday"`
	StartDate time.Time `json:"start_date"`
	UntilDate time.Time `json:"until_date"`
}

type MealPlan struct {
	ID           uuid.UUID             `json:"id"`
	PlanDate     time.Time             `json:"plan_date"`
	PlanTime     string                `json:"plan_time"`
	FoodID       uuid.UUID             `json:"food_id"`
	Servings     int32                 `json:"servings"`
	SeriesID     *uuid.UUID            `json:"series_id,omitempty"`
	LinkURL      string                `json:"link_url,omitempty"`
	LinkTitle    string                `json:"link_title,omitempty"`
	LinkImageURL string                `json:"link_image_url,omitempty"`
	Title        string                `json:"title,omitempty"`
	Notes        string                `json:"notes,omitempty"`
	Version      int32                 `json:"version"`
	DeletedAt    *time.Time            `json:"deleted_at,omitempty"`
	CreatedBy    *uuid.UUID            `json:"created_by,omitempty"`
	UpdatedBy    *uuid.UUID            `json:"updated_by,omitempty"`
	Recipes      []ScheduledMealRecipe `json:"scheduled_meal_recipes,omitempty"`
}

// ScheduledMealRecipe's meal is implied by the meal plan that nests it.
type ScheduledMealRecipe struct {
	ID               uuid.UUID  `json:"id"`
	FoodID           uuid.UUID  `json:"food_id"`
	ServingsOverride *int32     `json:"servings_override,omitempty"`
	SortOrder        int32      `json:"sort_order"`
	CreatedBy        *uuid.UUID `json:"created_by,omitempty"`
	UpdatedBy        *uuid.UUID `json:"updated_by,omitempty"`
}

type GroceryList struct {
	ID        uuid.UUID     `json:"id"`
	Name      string        `json:"name"`
	CreatedAt time.Time     `json:"created_at"`
	DeletedAt *time.Time    `json:"deleted_at,omitempty"`
	CreatedBy *uuid.UUID    `json:"created_by,omitempty"`
	UpdatedBy *uuid.UUID    `json:"updated_by,omitempty"`
	Items     []GroceryItem `json:"items,omitempty"`
}

type GroceryItem struct {
	ID           uuid.UUID           `json:"id"`
	Name         string              `json:"name"`
	Amount       float64             `json:"amount"`
	Unit         string              `json:"unit"`
	Checked      bool                `json:"checked"`
	Note         string              `json:"note"`
	SortOrder    int32               `json:"sort_order"`
	IngredientID *uuid.UUID          `json:"ingredient_id,omitempty"`
	SourceType   string              `json:"source_type,omitempty"`
	Variant      string              `json:"variant,omitempty"`
	DeletedAt    *time.Time          `json:"deleted_at,omitempty"`
	CreatedBy    *uuid.UUID          `json:"created_by,omitempty"`
	UpdatedBy    *uuid.UUID          `json:"updated_by,omitempty"`
	Sources      []GroceryItemSource `json:"sources,omitempty"`
}

type GroceryItemSource struct {
	ID                     uuid.UUID  `json:"id"`
	ScheduledMealID        *uuid.UUID `json:"scheduled_meal_id,omitempty"`
	RecipeID               *uuid.UUID `json:"recipe_id,omitempty"`
	RecipeIngredientLineID *uuid.UUID `json:"recipe_ingredient_line_id,omitempty"`
	QuantityContributed    float64    `json:"quantity_contributed"`
	UnitContributed        string     `json:"unit_contributed"`
	Variant                string     `json:"variant,omitempty"`
	LineRecipeName         string     `json:"line_recipe_name,omitempty"`
}

type PrepSession struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	SessionDate time.Time  `json:"session_date"`
	CreatedAt   time.Time  `json:"created_at"`
	Meals       []PrepMeal `json:"meals,omitempty"`
}

// PrepMeal's session is implied by the prep session that nests it.
type PrepMeal struct {
	FoodID    uuid.UUID `json:"food_id"`
	Servings  int32     `json:"servings"`
	SortOrder int32     `json:"sort_order"`
}

// Report summarizes an import run for display (FR15.5).
type Report struct {
	Sections []SectionReport
}

// SectionReport records what happened to one section's primary entities.
type SectionReport struct {
	Name     string
	Inserted int
	Updated  int
	Skipped  int
	Notes    []string // human-readable reasons for skipped rows
}
