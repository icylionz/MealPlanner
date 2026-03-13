package pages

import "mealplanner/internal/models"

type AuthPageData struct {
	Page       models.AppPageData
	Identifier string
}

type OnboardingPageData struct {
	Page          models.AppPageData
	HouseholdName string
	InviteCode    string
}

type AgendaPageData struct {
	Page        models.AppPageData
	Days        []models.AgendaDay
	Recipes     []models.RecipeView
	EditMeal    *models.MealView
	FilterDate  string
	DefaultDate string
}

type IngredientsPageData struct {
	Page     models.AppPageData
	Items    []models.IngredientView
	Search   string
	EditItem *models.IngredientView
}

type RecipesPageData struct {
	Page        models.AppPageData
	Items       []models.RecipeView
	Tags        []string
	Ingredients []models.IngredientView
	AllRecipes  []models.RecipeView
	EditRecipe  *models.RecipeView
	Search      string
	SelectedTag string
}

type GroceryPageData struct {
	Page      models.AppPageData
	Snapshots []models.GrocerySnapshotView
	Snapshot  *models.GrocerySnapshotView
}

type SettingsPageData struct {
	Page    models.AppPageData
	Members []models.CurrentUser
	Invites []models.InviteView
}
