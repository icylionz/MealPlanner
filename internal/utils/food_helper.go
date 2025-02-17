package utils

import (
	"fmt"
	"mealplanner/internal/models"
	"strconv"

	"github.com/labstack/echo/v4"
)

type FoodFormProps struct {
    Food    *models.Food
    Foods   []*models.Food  // For ingredient selection
    Errors  map[string]string
    IsEdit  bool
}

type IngredientForm struct {
    FoodID   string  `form:"ingredients[].food_id"`    // Matches name="ingredients[%d].food_id"
    Quantity float64 `form:"ingredients[].quantity"`   // Matches name="ingredients[%d].quantity"
    Unit     string  `form:"ingredients[].unit"`       // Matches name="ingredients[%d].unit"
}

type FoodForm struct {
    // Basic Info
    Name     string `form:"name"`
    UnitType string `form:"unit_type"`
    BaseUnit string `form:"base_unit"`
    IsRecipe bool   `form:"is_recipe"`

    RecipeURL       string           `form:"recipe_url"`      // Matches name="recipe_url"
    Instructions    string           `form:"instructions"`    // Matches name="instructions"
    Ingredients     []IngredientForm `form:"-"`              // Handled specially due to array indexing
    YieldQuantity   float64          `form:"yield_quantity"` // Matches name="yield_quantity"
    YieldUnit       string           `form:"yield_unit"`     // Matches name="yield_unit"
}

// Special binding method needed for ingredients array
func (f *FoodForm) BindIngredients(c echo.Context) error {
    var ingredients []IngredientForm
    i := 0
    for {
        ing := IngredientForm{}
        foodID := c.FormValue(fmt.Sprintf("ingredients[%d].food_id", i))
        if foodID == "" {
            break
        }
        ing.FoodID = foodID

        quantity, err := strconv.ParseFloat(c.FormValue(fmt.Sprintf("ingredients[%d].quantity", i)), 64)
        if err != nil {
            return err
        }
        ing.Quantity = quantity

        ing.Unit = c.FormValue(fmt.Sprintf("ingredients[%d].unit", i))
        ingredients = append(ingredients, ing)
        i++
    }
    f.Ingredients = ingredients
    return nil
}

func (f *FoodForm) Validate() *ValidationError {
    errors := make(map[string]string)

    // Basic validation
    if f.Name == "" {
        errors["name"] = "Name is required"
    }

    if !isValidUnitType(f.UnitType) {
        errors["unit_type"] = "Invalid unit type"
    }

    if !isValidUnit(f.UnitType, f.BaseUnit) {
        errors["base_unit"] = "Invalid base unit for selected unit type"
    }

    if f.IsRecipe {
        if f.YieldQuantity <= 0 {
            errors["yield_quantity"] = "Yield quantity must be greater than 0"
        }
        if !isValidUnit(f.UnitType, f.YieldUnit) {
            errors["yield_unit"] = "Invalid yield unit"
        }

        // Ingredient validation
        if len(f.Ingredients) == 0 {
            errors["ingredients"] = "Recipe must have at least one ingredient"
        }

        for i, ing := range f.Ingredients {
            if ing.FoodID == "" {
                errors[fmt.Sprintf("ingredients[%d].food_id", i)] = "Food is required"
            }
            if ing.Quantity <= 0 {
                errors[fmt.Sprintf("ingredients[%d].quantity", i)] = "Quantity must be greater than 0"
            }
            if ing.Unit == "" {
                errors[fmt.Sprintf("ingredients[%d].unit", i)] = "Unit is required"
            }
        }
    }

    if len(errors) > 0 {
        return &ValidationError{OffendingFields: errors}
    }
    return nil
}

func (f *FoodForm) ToModel() *models.Food {
    food := &models.Food{
        Name:     f.Name,
        UnitType: f.UnitType,
        BaseUnit: f.BaseUnit,
        IsRecipe: f.IsRecipe,
    }

    if f.IsRecipe {
        food.Recipe = &models.Recipe{
            URL:           f.RecipeURL,
            Instructions:  f.Instructions,
            YieldQuantity: f.YieldQuantity,
            YieldUnit:     f.YieldUnit,
            Ingredients:   make([]*models.RecipeItem, len(f.Ingredients)),
        }

        for i, ing := range f.Ingredients {
        	foodId, err := strconv.Atoi(ing.FoodID)
        	if err != nil {
        		return nil
        	}
            food.Recipe.Ingredients[i] = &models.RecipeItem{
                FoodID:   foodId,
                Quantity: ing.Quantity,
                Unit:     ing.Unit,
            }
        }
    }

    return food
}

func ValidateAndFilterDependencies(foods []*models.Food, targetID int) []*models.Food {
    // Create graph representation of dependencies
    deps := make(map[int][]int)
    for _, food := range foods {
        if food.IsRecipe && food.Recipe != nil {
            deps[food.ID] = make([]int, 0)
            for _, ing := range food.Recipe.Ingredients {
                deps[food.ID] = append(deps[food.ID], ing.Food.ID)
            }
        }
    }

    // Helper function to check dependency chain depth and circularity
    var checkDependencyChain func(foodID int, visited map[int]bool, depth int) bool
    checkDependencyChain = func(foodID int, visited map[int]bool, depth int) bool {
        // Check maximum depth
        if depth > 15 {
            return false
        }

        // Check for circular dependencies
        if visited[foodID] {
            return false
        }

        // Mark current food as visited
        visited[foodID] = true
        defer delete(visited, foodID) // Clean up after checking this branch

        // Check all ingredients recursively
        for _, depID := range deps[foodID] {
            if !checkDependencyChain(depID, visited, depth+1) {
                return false
            }
        }

        return true
    }

    // Filter foods based on dependency rules
    validFoods := make([]*models.Food, 0)
    for _, food := range foods {
        // Skip the target food to prevent self-reference
        if food.ID == targetID {
            continue
        }

        // Basic foods are always valid
        if !food.IsRecipe {
            validFoods = append(validFoods, food)
            continue
        }

        // Check if this food would create valid dependencies if used
        // We create a temporary dependency to the target food to validate
        if targetID > 0 {
            deps[targetID] = append(deps[targetID], food.ID)
        }

        // Validate the dependency chain
        isValid := checkDependencyChain(food.ID, make(map[int]bool), 1)

        // Clean up temporary dependency
        if targetID > 0 {
            deps[targetID] = deps[targetID][:len(deps[targetID])-1]
        }

        if isValid {
            validFoods = append(validFoods, food)
        }
    }

    return validFoods
}

func isValidUnitType(unitType string) bool {
    validTypes := []string{"mass", "volume", "count"}
    for _, t := range validTypes {
        if t == unitType {
            return true
        }
    }
    return false
}

func isValidUnit(unitType, unit string) bool {
    unitsByType := map[string][]string{
        "mass":   {"grams", "kilograms", "ounces", "pounds"},
        "volume": {"milliliters", "liters", "teaspoons", "tablespoons", "cups", "fluidOunces"},
        "count":  {"pieces", "servings"},
    }
    
    units, ok := unitsByType[unitType]
    if !ok {
        return false
    }
    
    for _, u := range units {
        if u == unit {
            return true
        }
    }
    return false
}