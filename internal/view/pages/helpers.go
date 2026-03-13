package pages

import (
	"fmt"
	"strings"
	"time"

	"mealplanner/internal/models"

	"github.com/google/uuid"
)

func formatDateTimeLocal(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02T15:04")
}

func joinCSV(values []string) string {
	return strings.Join(values, ", ")
}

func agendaMealCount(days []models.AgendaDay) int {
	total := 0
	for _, day := range days {
		total += len(day.Meals)
	}
	return total
}

func agendaRecipeSelectionCount(days []models.AgendaDay) int {
	total := 0
	for _, day := range days {
		for _, meal := range day.Meals {
			total += len(meal.Recipes)
		}
	}
	return total
}

func totalRecipeIngredientCount(items []models.RecipeView) int {
	total := 0
	for _, item := range items {
		total += len(item.Ingredients)
	}
	return total
}

func totalRecipeComponentCount(items []models.RecipeView) int {
	total := 0
	for _, item := range items {
		total += len(item.Components)
	}
	return total
}

func distinctRecipeTagCount(items []models.RecipeView) int {
	seen := make(map[string]struct{})
	for _, item := range items {
		for _, tag := range item.Tags {
			trimmed := strings.TrimSpace(strings.ToLower(tag))
			if trimmed == "" {
				continue
			}
			seen[trimmed] = struct{}{}
		}
	}
	return len(seen)
}

func ingredientAliasCount(items []models.IngredientView) int {
	total := 0
	for _, item := range items {
		total += len(item.Aliases)
	}
	return total
}

func ingredientDensityCount(items []models.IngredientView) int {
	total := 0
	for _, item := range items {
		if item.DensityGPerML != nil {
			total++
		}
	}
	return total
}

func groceryCheckedCount(snapshot *models.GrocerySnapshotView) int {
	if snapshot == nil {
		return 0
	}
	total := 0
	for _, item := range snapshot.Items {
		if item.Checked {
			total++
		}
	}
	return total
}

func groceryOpenCount(snapshot *models.GrocerySnapshotView) int {
	if snapshot == nil {
		return 0
	}
	return len(snapshot.Items) - groceryCheckedCount(snapshot)
}

func groceryReviewCount(snapshot *models.GrocerySnapshotView) int {
	if snapshot == nil {
		return 0
	}
	total := 0
	for _, item := range snapshot.Items {
		if item.NeedsReview {
			total++
		}
	}
	return total
}

func groceryItemClass(item models.GroceryItemView) string {
	classes := []string{"grocery-item"}
	if item.Checked {
		classes = append(classes, "checked")
	}
	if item.NeedsReview {
		classes = append(classes, "review")
	}
	return strings.Join(classes, " ")
}

func snapshotCardClass(selected *models.GrocerySnapshotView, item models.GrocerySnapshotView) string {
	if selected != nil && selected.ID == item.ID {
		return "snapshot-card active"
	}
	return "snapshot-card"
}

func memberInitials(member models.CurrentUser) string {
	return labelInitials(memberLabel(member))
}

func inviteState(invite models.InviteView) string {
	now := time.Now()
	switch {
	case invite.RevokedAt != nil:
		return "Revoked"
	case invite.ExpiresAt != nil && invite.ExpiresAt.Before(now):
		return "Expired"
	default:
		return "Active"
	}
}

func inviteStateClass(invite models.InviteView) string {
	switch inviteState(invite) {
	case "Revoked", "Expired":
		return "status-pill warn"
	default:
		return "status-pill success"
	}
}

func inviteExpiryText(invite models.InviteView) string {
	switch {
	case invite.RevokedAt != nil:
		return "Revoked"
	case invite.ExpiresAt == nil:
		return "No expiry"
	case invite.ExpiresAt.Before(time.Now()):
		return "Expired " + invite.ExpiresAt.Format("Jan 2, 2006")
	default:
		return "Expires " + invite.ExpiresAt.Format("Jan 2, 2006")
	}
}

func labelInitials(label string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(label), func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-' || r == '@'
	})
	initials := make([]string, 0, 2)
	for _, part := range parts {
		if part == "" {
			continue
		}
		initials = append(initials, strings.ToUpper(part[:1]))
		if len(initials) == 2 {
			break
		}
	}
	if len(initials) == 0 {
		return "MP"
	}
	return strings.Join(initials, "")
}

func ingredientLinesText(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	lines := make([]string, 0, len(recipe.Ingredients))
	for _, line := range recipe.Ingredients {
		lines = append(lines, fmt.Sprintf("%s|%g|%s|%s|%s|%t", line.IngredientName, line.Quantity, line.Unit, line.VariantText, line.PrepNote, line.Optional))
	}
	return strings.Join(lines, "\n")
}

func componentLinesText(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	lines := make([]string, 0, len(recipe.Components))
	for _, line := range recipe.Components {
		lines = append(lines, fmt.Sprintf("%s|%g|%s", line.ComponentTitle, line.Quantity, line.Unit))
	}
	return strings.Join(lines, "\n")
}

func stepsText(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	return strings.Join(recipe.Steps, "\n")
}

func recipeSelected(meal *models.MealView, recipeID uuid.UUID) bool {
	if meal == nil {
		return false
	}
	for _, recipe := range meal.Recipes {
		if recipe.RecipeID == recipeID {
			return true
		}
	}
	return false
}

func recipeOverride(meal *models.MealView, recipeID uuid.UUID) string {
	if meal == nil {
		return ""
	}
	for _, recipe := range meal.Recipes {
		if recipe.RecipeID == recipeID && recipe.ServingsOverride != nil {
			return fmt.Sprintf("%g", *recipe.ServingsOverride)
		}
	}
	return ""
}

func ingredientAliases(item *models.IngredientView) string {
	if item == nil {
		return ""
	}
	return strings.Join(item.Aliases, ", ")
}

func mealTitleValue(meal *models.MealView) string {
	if meal == nil {
		return ""
	}
	return meal.Title
}

func mealDateValue(data AgendaPageData) string {
	if data.EditMeal != nil {
		return formatDateTimeLocal(data.EditMeal.ScheduledAt)
	}
	return data.DefaultDate
}

func mealServingsValue(meal *models.MealView) string {
	if meal == nil {
		return "1"
	}
	return fmt.Sprintf("%g", meal.Servings)
}

func mealNotesValue(meal *models.MealView) string {
	if meal == nil {
		return ""
	}
	return meal.Notes
}

func mealLinkURLValue(meal *models.MealView) string {
	if meal == nil {
		return ""
	}
	return meal.LinkURL
}

func mealLinkTitleValue(meal *models.MealView) string {
	if meal == nil {
		return ""
	}
	return meal.LinkTitle
}

func ingredientNameValue(item *models.IngredientView) string {
	if item == nil {
		return ""
	}
	return item.CanonicalName
}

func ingredientDensityValue(item *models.IngredientView) string {
	if item == nil || item.DensityGPerML == nil {
		return ""
	}
	return fmt.Sprintf("%g", *item.DensityGPerML)
}

func ingredientNoteValue(item *models.IngredientView) string {
	if item == nil {
		return ""
	}
	return item.Note
}

func recipeTitleValue(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	return recipe.Title
}

func recipeDescriptionValue(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	return recipe.Description
}

func recipeYieldAmountValue(recipe *models.RecipeView) string {
	if recipe == nil {
		return "1"
	}
	return fmt.Sprintf("%g", recipe.YieldAmount)
}

func recipeYieldUnitValue(recipe *models.RecipeView) string {
	if recipe == nil || recipe.YieldUnit == "" {
		return "servings"
	}
	return recipe.YieldUnit
}

func recipeSourceURLValue(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	return recipe.SourceURL
}

func recipeTagsValue(recipe *models.RecipeView) string {
	if recipe == nil {
		return ""
	}
	return joinCSV(recipe.Tags)
}

func memberLabel(member models.CurrentUser) string {
	if member.Username != "" {
		return member.Username
	}
	return member.Email
}
