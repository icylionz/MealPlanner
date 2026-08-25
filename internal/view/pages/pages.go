// Package pages renders the Backbone Plate screens.
package pages

import (
	"strconv"
	"strings"

	"mealplanner/internal/foods"
	"mealplanner/internal/grocery"
	"mealplanner/internal/households"
	"mealplanner/internal/planner"
	"mealplanner/internal/units"
)

// ConvertOption is one unit-conversion target in a unit popover.
type ConvertOption struct {
	Unit   string
	Amount float64
}

// ConvertOptionsFor lists conversion targets with pre-computed amounts.
func ConvertOptionsFor(amount float64, unit string) []ConvertOption {
	var out []ConvertOption
	for _, u := range units.ConvertTargets(unit) {
		out = append(out, ConvertOption{Unit: u, Amount: units.Convert(amount, unit, u)})
	}
	return out
}

// ConvertOptionsForDensity lists same-dimension targets plus, when a density is
// known, cross-dimension (volume<->weight) targets with density-computed
// amounts (FR10.2).
func ConvertOptionsForDensity(amount float64, unit string, density float64) []ConvertOption {
	out := ConvertOptionsFor(amount, unit)
	for _, u := range units.CrossTargets(unit, density) {
		if v, ok := units.ConvertDensity(amount, unit, u, density); ok {
			out = append(out, ConvertOption{Unit: u, Amount: v})
		}
	}
	return out
}

// FormatAmount proxies units.Format for templates.
func FormatAmount(n float64) string { return units.Format(n) }

// FormatDensity renders a density with full precision (no rounding), since
// densities carry meaningful sub-decimal values (e.g. 0.918, 1.25 g/ml).
func FormatDensity(n float64) string { return strconv.FormatFloat(n, 'g', -1, 64) }

// MealVM pairs a scheduled meal with its food for display.
type MealVM struct {
	Meal   planner.Meal
	Food   foods.Food
	IsPast bool
	IsNext bool
}

// TodayData feeds the Today screen.
type TodayData struct {
	Member    *households.Member
	DateLabel string
	Date      string
	Meals     []MealVM
}

// PlanDay is one selectable day in the Plan sidebar / date strip.
type PlanDay struct {
	Date      string
	DayAbbr   string
	DayNum    string
	MealCount int
	IsToday   bool
	IsSel     bool
}

// CalCell is one mini-calendar cell.
type CalCell struct {
	Date     string // empty for leading blanks
	DayNum   string
	HasMeals bool
	IsToday  bool
	IsSel    bool
}

// PlanData feeds the Plan screen.
type PlanData struct {
	Member        *households.Member
	SelectedDate  string
	SelectedLabel string
	IsSelToday    bool
	Layout        string // "list" or "calendar"
	MonthLabel    string
	PrevMonth     string // YYYY-MM
	NextMonth     string
	CalCells      []CalCell
	WeekDays      []PlanDay
	DayMeals      []MealVM            // meals for the selected day (list layout)
	WeekMeals     map[string][]MealVM // meals per date (calendar layout)
}

// FoodsData feeds the Foods screen.
type FoodsData struct {
	Member  *households.Member
	Search  string
	Tag     string
	AllTags []string
	Foods   []foods.Food
}

// SubCount returns how many components of a food are themselves recipes.
func SubCount(f foods.Food) int {
	n := 0
	for _, c := range f.Components {
		if c.ChildIsRecipe {
			n++
		}
	}
	return n
}

// IngNode is a resolved node in the food detail component tree.
type IngNode struct {
	Component  foods.Component
	Amount     float64 // scaled
	Unit       string  // display unit (after ?u= conversion)
	SubFood    *foods.Food
	SubScale   float64
	Children   []IngNode
	Depth      int
	ConvertKey string // query key controlling this line's display unit
}

// FoodDetailData feeds the food detail screen.
type FoodDetailData struct {
	Member  *households.Member
	Food    foods.Food
	Scale   int
	Tree    []IngNode
	BaseURL string // detail path with scale/unit params preserved except scale
}

// ComponentForm is one editable component row in the editor. Every component
// references an existing food (chosen via the picker), never free text.
type ComponentForm struct {
	FoodID   string
	FoodName string
	Amount   string
	Unit     string
}

// FoodEditData feeds the food editor (also the New Food flow).
type FoodEditData struct {
	Member        *households.Member
	IsNew         bool
	FoodID        string
	Action        string
	Name          string
	Description   string
	PrepTime      string
	CookTime      string
	Servings      string
	DefaultUnit   string
	Density       string // g/ml, blank = unset (falls back to starter set)
	DensitySource string // "starter", "custom", or "none" (display hint)
	Version       string // optimistic-lock version, posted back as a hidden field (FR16)
	Tags          []string
	Components    []ComponentForm
	Steps         []string
	AllFoods      []foods.Food // options for the component search-select
	Error         string
	Conflict      *FoodConflict // set when a save was rejected as stale (FR16)
	Units         []string
}

// FoodConflict holds the current saved food shown on the conflict screen so the
// user can compare it with their attempted edit before re-saving (FR16.2).
type FoodConflict struct {
	Version     int
	Name        string
	Description string
	PrepTime    int
	CookTime    int
	Servings    int
	DefaultUnit string
}

// GroceryData feeds the Grocery screen.
type GroceryData struct {
	Member   *households.Member
	Lists    []grocery.List
	Active   *grocery.List
	Renaming bool
}

// GenPreviewItem is one previewed generated ingredient.
type GenPreviewItem struct {
	Name   string
	Amount float64
	Unit   string
}

// GenMealOption is a planned meal selectable as a generation source.
type GenMealOption struct {
	ID       string
	Food     foods.Food
	DayLabel string
	Time     string
	Servings int
}

// GroceryGenData feeds the generate-ingredients modal page.
type GroceryGenData struct {
	Member       *households.Member
	ListID       string
	Mode         string // planned-meal | food | date-range
	MealSearch   string
	Meals        []GenMealOption
	SelectedMeal string
	FoodSearch   string
	Foods        []foods.Food
	SelectedFood string
	FoodServings int
	FromDate     string
	ToDate       string
	DateError    bool
	Preview      []GenPreviewItem
	HasPreview   bool
	QueryBase    string // current query string minus preview, for tab links
}

// PrepMealVM is a prep session meal with its food and breakdown.
type PrepMealVM struct {
	Food      foods.Food
	Servings  int
	Breakdown []GenPreviewItem
}

// PrepData feeds the Prep screen.
type PrepData struct {
	Member     *households.Member
	Sessions   []PrepSessionVM
	Active     *PrepSessionVM
	Aggregate  []GenPreviewItem
	AddingMeal bool
	FoodSearch string
	Foods      []foods.Food
}

type PrepSessionVM struct {
	ID        string
	Name      string
	Date      string
	MealCount int
	Meals     []PrepMealVM
}

// NewPrepSessionVM builds a session view model.
func NewPrepSessionVM(id, name, date string, meals []PrepMealVM) PrepSessionVM {
	return PrepSessionVM{ID: id, Name: name, Date: date, MealCount: len(meals), Meals: meals}
}

// PrepPrintData feeds the printable prep view.
type PrepPrintData struct {
	Member    *households.Member
	Session   PrepSessionVM
	Aggregate []GenPreviewItem
}

// HouseholdData feeds the Household screen.
type HouseholdData struct {
	Member     *households.Member
	Members    []households.Member
	ShowInvite bool
}

// ImportLine is one parsed ingredient line awaiting reconciliation to a food.
type ImportLine struct {
	Name   string // parsed free-text name
	Amount string
	Unit   string
	FoodID string // selected food id, "" when unmatched
}

// Matched reports whether the line has been mapped to a food.
func (l ImportLine) Matched() bool { return l.FoodID != "" }

// ImportReconcileData feeds the import reconcile screen: an imported recipe
// whose ingredient lines are being mapped to existing foods before saving.
type ImportReconcileData struct {
	Member      *households.Member
	Name        string
	Description string
	Prep        string
	Cook        string
	Servings    string
	DefaultUnit string
	Tags        []string
	Steps       []string
	Lines       []ImportLine
	AllFoods    []foods.Food
	Error       string
}

// AddMealData feeds the add-meal modal page.
type AddMealData struct {
	Member       *households.Member
	Date         string
	Time         string
	Servings     int
	Search       string
	Foods        []foods.Food
	Selected     string
	SelectedFood *foods.Food
	ReturnTo     string
	Active       string // nav section the modal was opened from
}

// EditMealData feeds the edit-meal modal page.
type EditMealData struct {
	Member       *households.Member
	MealID       string
	Date         string
	Time         string
	Servings     int
	Search       string
	Foods        []foods.Food
	Selected     string
	SelectedFood *foods.Food
	ReturnTo     string
	Active       string
	Recurring    bool            // meal belongs to a series -> offer scope choice
	Series       *planner.Series // recurrence rule for display (may be nil)
}

// WeekdayLabels are the single-letter column headers, Sunday-first (0..6).
var weekdayLabels = []string{"S", "M", "T", "W", "T", "F", "S"}

// WeekdayName returns the abbreviated name for a weekday int (0=Sun..6=Sat).
func WeekdayName(n int) string {
	names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	if n < 0 || n > 6 {
		return ""
	}
	return names[n]
}

// WeekdayLabel returns the single-letter label for a weekday int.
func WeekdayLabel(n int) string {
	if n < 0 || n > 6 {
		return ""
	}
	return weekdayLabels[n]
}

// SeriesSummary renders a human-readable recurrence description.
func SeriesSummary(s *planner.Series) string {
	if s == nil {
		return ""
	}
	var b string
	if s.Freq == "weekly" {
		if len(s.Weekdays) == 0 {
			b = "Weekly"
		} else {
			days := make([]string, len(s.Weekdays))
			for i, w := range s.Weekdays {
				days[i] = WeekdayName(int(w))
			}
			b = "Weekly on " + strings.Join(days, ", ")
		}
	} else {
		b = "Daily"
	}
	return b + " until " + s.Until
}
