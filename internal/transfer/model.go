// Package transfer implements full-fidelity JSON export and import of all
// household data (FR15). The archive carries a schema_version and stable UUIDs
// so a later import merges by ID rather than duplicating.
package transfer

import (
	"time"

	"github.com/google/uuid"
)

// SchemaVersion is the only archive format this build can read. Import rejects
// any other value with a clear compatibility error (FR15.4).
const SchemaVersion = 1

// Section names selectable at import time (FR15.3). A section groups a parent
// entity with its children (e.g. "foods" also carries tags, components, steps).
const (
	SectionMembers = "members"
	SectionFoods   = "foods"
	SectionMeals   = "meals"
	SectionGrocery = "grocery"
	SectionPrep    = "prep"
)

// AllSections lists every importable/exportable section in a stable order.
var AllSections = []string{SectionMembers, SectionFoods, SectionMeals, SectionGrocery, SectionPrep}

// Archive is the top-level export document.
type Archive struct {
	SchemaVersion int           `json:"schema_version"`
	ExportedAt    time.Time     `json:"exported_at"`
	Members       []Member      `json:"members,omitempty"`
	Foods         []Food        `json:"foods,omitempty"`
	MealSeries    []MealSeries  `json:"meal_series,omitempty"`
	MealPlans     []MealPlan    `json:"meal_plans,omitempty"`
	GroceryLists  []GroceryList `json:"grocery_lists,omitempty"`
	PrepSessions  []PrepSession `json:"prep_sessions,omitempty"`
}

type Member struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Initials  string    `json:"initials"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

type Food struct {
	ID            uuid.UUID   `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	PrepTimeMin   int32       `json:"prep_time_min"`
	CookTimeMin   int32       `json:"cook_time_min"`
	Servings      int32       `json:"servings"`
	DefaultUnit   string      `json:"default_unit"`
	DensityGPerMl float64     `json:"density_g_per_ml"`
	DensitySource string      `json:"density_source"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Tags          []string    `json:"tags,omitempty"`
	Components    []Component `json:"components,omitempty"`
	Steps         []Step      `json:"steps,omitempty"`
}

// Component's parent food is implied by the food that nests it.
type Component struct {
	ID          uuid.UUID `json:"id"`
	ChildFoodID uuid.UUID `json:"child_food_id"`
	Amount      float64   `json:"amount"`
	Unit        string    `json:"unit"`
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
	ID           uuid.UUID  `json:"id"`
	PlanDate     time.Time  `json:"plan_date"`
	PlanTime     string     `json:"plan_time"`
	FoodID       uuid.UUID  `json:"food_id"`
	Servings     int32      `json:"servings"`
	SeriesID     *uuid.UUID `json:"series_id,omitempty"`
	LinkURL      string     `json:"link_url,omitempty"`
	LinkTitle    string     `json:"link_title,omitempty"`
	LinkImageURL string     `json:"link_image_url,omitempty"`
}

type GroceryList struct {
	ID        uuid.UUID     `json:"id"`
	Name      string        `json:"name"`
	CreatedAt time.Time     `json:"created_at"`
	Items     []GroceryItem `json:"items,omitempty"`
}

type GroceryItem struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Amount    float64   `json:"amount"`
	Unit      string    `json:"unit"`
	Checked   bool      `json:"checked"`
	Note      string    `json:"note"`
	SortOrder int32     `json:"sort_order"`
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
