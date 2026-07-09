// Package pages renders the Backbone Plate screens.
package pages

import (
	"mealplanner/internal/grocery"
	"mealplanner/internal/households"
	"mealplanner/internal/planner"
	"mealplanner/internal/recipes"
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

// FormatAmount proxies units.Format for templates.
func FormatAmount(n float64) string { return units.Format(n) }

// MealVM pairs a scheduled meal with its recipe for display.
type MealVM struct {
	Meal    planner.Meal
	Recipe  recipes.Recipe
	IsPast  bool
	IsNext  bool
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
	Member       *households.Member
	SelectedDate string
	SelectedLabel string
	IsSelToday   bool
	Layout       string // "list" or "calendar"
	MonthLabel   string
	PrevMonth    string // YYYY-MM
	NextMonth    string
	CalCells     []CalCell
	WeekDays     []PlanDay
	DayMeals     []MealVM            // meals for the selected day (list layout)
	WeekMeals    map[string][]MealVM // meals per date (calendar layout)
}

// FoodsData feeds the Foods screen.
type FoodsData struct {
	Member  *households.Member
	Search  string
	Tag     string
	AllTags []string
	Recipes []recipes.Recipe
}

// SubCount returns how many lines of a recipe are sub-recipes.
func SubCount(r recipes.Recipe) int {
	n := 0
	for _, l := range r.Ingredients {
		if l.IsRecipe() {
			n++
		}
	}
	return n
}

// IngNode is a resolved node in the recipe detail ingredient tree.
type IngNode struct {
	Line       recipes.Line
	Amount     float64 // scaled
	Unit       string  // display unit (after ?u= conversion)
	SubRecipe  *recipes.Recipe
	SubScale   float64
	Children   []IngNode
	Depth      int
	ConvertKey string // query key controlling this line's display unit
}

// RecipeDetailData feeds the recipe detail screen.
type RecipeDetailData struct {
	Member  *households.Member
	Recipe  recipes.Recipe
	Scale   int
	Tree    []IngNode
	BaseURL string // detail path with scale/unit params preserved except scale
}

// LineForm is one editable ingredient row in the editor.
type LineForm struct {
	Name        string
	Amount      string
	Unit        string
	IsRecipe    bool
	SubRecipeID string
	SubName     string
}

// RecipeEditData feeds the recipe editor.
type RecipeEditData struct {
	Member      *households.Member
	IsNew       bool
	RecipeID    string
	Action      string
	Name        string
	Description string
	PrepTime    string
	CookTime    string
	Servings    string
	Tags        []string
	Ingredients []LineForm
	Steps       []string
	AllRecipes  []recipes.Recipe // for sub-recipe pickers
	PickerFor   int              // index of row picking a sub-recipe, -1 none
	Error       string
	Units       []string
}

// GroceryData feeds the Grocery screen.
type GroceryData struct {
	Member     *households.Member
	Lists      []grocery.List
	Active     *grocery.List
	Renaming   bool
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
	Recipe   recipes.Recipe
	DayLabel string
	Time     string
	Servings int
}

// GroceryGenData feeds the generate-ingredients modal page.
type GroceryGenData struct {
	Member         *households.Member
	ListID         string
	Mode           string // planned-meal | recipe | date-range
	MealSearch     string
	Meals          []GenMealOption
	SelectedMeal   string
	RecipeSearch   string
	Recipes        []recipes.Recipe
	SelectedRecipe string
	RecipeServings int
	FromDate       string
	ToDate         string
	DateError      bool
	Preview        []GenPreviewItem
	HasPreview     bool
	QueryBase      string // current query string minus preview, for tab links
}

// PrepMealVM is a prep session meal with its recipe and breakdown.
type PrepMealVM struct {
	Recipe    recipes.Recipe
	Servings  int
	Breakdown []GenPreviewItem
}

// PrepData feeds the Prep screen.
type PrepData struct {
	Member    *households.Member
	Sessions  []PrepSessionVM
	Active    *PrepSessionVM
	Aggregate []GenPreviewItem
	AddingMeal bool
	RecipeSearch string
	Recipes   []recipes.Recipe
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
	Member    *households.Member
	Members   []households.Member
	ShowInvite bool
}

// AddMealData feeds the add-meal modal page.
type AddMealData struct {
	Member    *households.Member
	Date      string
	Time      string
	Servings  int
	Search    string
	Recipes   []recipes.Recipe
	Selected  string
	SelectedRecipe *recipes.Recipe
	ReturnTo  string
	Active    string // nav section the modal was opened from
}
