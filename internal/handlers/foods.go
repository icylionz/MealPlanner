package handlers

import (
	"log"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
	"mealplanner/internal/services"
	"mealplanner/internal/utils"
	"mealplanner/internal/views/components"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
)

type FoodHandler struct {
	service *services.FoodService
}

func (h *FoodHandler) HandleFoodsPage(c echo.Context) error {
	return components.FoodsPage().Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleViewFoodDetailsModal(c echo.Context) error {
	id := c.QueryParam("id")
	food, err := h.service.GetFoodDetails(c.Request().Context(), id, 1)
	if err != nil {
		return c.String(500, "Error getting food")
	}
	return components.ViewFoodDetailsModal(food).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleSearchFoods(c echo.Context) error {
	// Get query parameters
	query := c.QueryParam("search")

	// Get foods from service
	foods, err := h.service.GetFoods(c.Request().Context(), query)
	if err != nil {
		return c.String(500, "Error searching foods")
	}

	// Render only the food list component
	return components.FoodList(foods).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleDeleteFood(c echo.Context) error {
	id := c.Param("id")
	err := h.service.DeleteFood(c.Request().Context(), id)
	if err != nil {
		return c.String(500, "Error deleting food")
	}
	return c.NoContent(204)
}

func (h *FoodHandler) HandleCreateFoodModal(c echo.Context) error {
	// If POST, handle form submission
	if c.Request().Method == "POST" {
		log.Default().Printf("POST /foods")
		form := new(utils.FoodForm)
		if err := c.Bind(form); err != nil {
			log.Default().Printf("Error binding food form: %v", err)
			return err
		}

		// If this is a recipe, we need to bind the ingredients separately
		if form.IsRecipe {
			if err := form.BindIngredients(c); err != nil {
				log.Default().Printf("Error binding ingredients form: %v", err)
				return err
			}
		}

		// Validate form
		if err := form.Validate(); err != nil {
			log.Default().Printf("Error validating food form: %v\nWith Fields: %v", err, err.Fields())
			props := utils.FoodFormProps{
				IsEdit: false,
				Food:   form.ToModel(),
				Errors: err.Fields(),
			}

			// Only fetch foods list if this was a recipe submission
			if form.IsRecipe {
				availableFoods, err := h.service.GetFoods(c.Request().Context(), "")
				if err != nil {
					return err
				}
				props.Foods = utils.ValidateAndFilterDependencies(availableFoods, 0)
			}

			c.Response().Writer.WriteHeader(http.StatusBadRequest)
			return components.CreateEditFoodModal(&props).Render(c.Request().Context(), c.Response().Writer)
		}

		// Create food
		food, err := h.service.CreateFood(c.Request().Context(), db.CreateFoodParams{
			Name:     form.Name,
			UnitType: form.UnitType,
			BaseUnit: form.BaseUnit,
			IsRecipe: form.IsRecipe,
			// Calculate density
		})
		if err != nil {
			log.Default().Printf("Error creating food: %v", err)
			return err
		}

		if form.IsRecipe {
			dbIngredients := make([]db.AddRecipeIngredientParams, len(form.Ingredients))
			for i, ing := range form.Ingredients {
				ingredientID, err := strconv.Atoi(ing.FoodID)
				if err != nil {
					return err
				}
				dbIngredients[i] = db.AddRecipeIngredientParams{
					IngredientID: int32(ingredientID),
					Quantity:     utils.Float64ToNumeric(ing.Quantity),
					Unit:         ing.Unit,
				}
			}
			err = h.service.CreateRecipeWithIngredients(c.Request().Context(), db.CreateRecipeParams{
				FoodID:        food.ID,
				Url:           pgtype.Text{String: form.RecipeURL},
				Instructions:  pgtype.Text{String: form.Instructions},
				YieldQuantity: utils.Float64ToNumeric(form.YieldQuantity),
				YieldUnit:     form.YieldUnit,
			}, dbIngredients)
			if err != nil {
				return err
			}
		}

		c.Response().Header().Set("HX-Trigger", "refreshFoodView")
		return c.NoContent(http.StatusOK)
	}

	// Initial GET - show empty form
	log.Default().Printf("GET /foods")
	props := utils.FoodFormProps{
		IsEdit: false,
		Food: &models.Food{
			UnitType: "mass",
			BaseUnit: "grams",
			Recipe:   &models.Recipe{},
		},
		Errors: make(map[string]string),
	}

	return components.CreateEditFoodModal(&props).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleEditFoodModal(c echo.Context) error {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid ID")
	}

	// If PUT, handle form submission
	if c.Request().Method == "PUT" {
		form := new(utils.FoodForm)
		if err := c.Bind(form); err != nil {
			return err
		}

		// If this is a recipe, bind ingredients
		if form.IsRecipe {
			if err := form.BindIngredients(c); err != nil {
				return err
			}
		}

		// Validate form
		if err := form.Validate(); err != nil {
			props := utils.FoodFormProps{
				IsEdit: true,
				Food:   form.ToModel(),
				Errors: err.Fields(),
			}

			// Only fetch foods list if this was a recipe submission
			if form.IsRecipe {
				availableFoods, err := h.service.GetFoods(c.Request().Context(), "")
				if err != nil {
					return err
				}
				props.Foods = utils.ValidateAndFilterDependencies(availableFoods, idNum)
			}

			c.Response().Status = http.StatusBadRequest
			return components.CreateEditFoodModal(&props).Render(c.Request().Context(), c.Response().Writer)
		}

		updateParams := db.UpdateFoodWithRecipeParams{
			ID:       int32(idNum),
			Name:     form.Name,
			UnitType: form.UnitType,
			BaseUnit: form.BaseUnit,
			IsRecipe: form.IsRecipe,
			// Calculate density
		}
		dbIngredients := make([]db.AddRecipeIngredientParams, len(form.Ingredients))
		for i, ing := range form.Ingredients {
			ingredientID, err := strconv.Atoi(ing.FoodID)
			if err != nil {
				return err
			}
			dbIngredients[i] = db.AddRecipeIngredientParams{
				IngredientID: int32(ingredientID),
				Quantity:     utils.Float64ToNumeric(ing.Quantity),
				Unit:         ing.Unit,
			}
		}

		_, err = h.service.UpdateFood(c.Request().Context(), updateParams, dbIngredients, false)
		if err != nil {
			return err
		}

		c.Response().Header().Set("HX-Trigger", "refreshFoodView")
		return c.NoContent(http.StatusOK)
	}

	// Initial GET - show form with existing data
	food, err := h.service.GetFoodDetails(c.Request().Context(), id, 1)
	if err != nil {
		return err
	}

	props := utils.FoodFormProps{
		IsEdit: true,
		Food:   food,
		Errors: make(map[string]string),
	}

	// Only fetch foods list if editing a recipe
	if food.IsRecipe {
		availableFoods, err := h.service.GetFoods(c.Request().Context(), "")
		if err != nil {
			return err
		}
		props.Foods = utils.ValidateAndFilterDependencies(availableFoods, idNum)
	}

	return components.CreateEditFoodModal(&props).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) GetRecipeFields(c echo.Context) error {
	isRecipe := c.QueryParam("is_recipe") == "true"
    if !isRecipe {
        return c.HTML(http.StatusOK, "")
    }
    
	availableFoods, err := h.service.GetFoods(c.Request().Context(), "")
	if err != nil {
		return err
	}
	validFoods := utils.ValidateAndFilterDependencies(availableFoods, 0)

	props := &utils.FoodFormProps{
		Food: &models.Food{
			IsRecipe: true,
			Recipe: &models.Recipe{
				Ingredients:   make([]*models.RecipeItem, 0),
				YieldQuantity: 1,
				YieldUnit:     "servings",
			},
		},
		Foods: validFoods,
	}

	return components.RecipeFields(props).Render(c.Request().Context(), c.Response().Writer)
}

func NewFoodHandler(service *services.FoodService) *FoodHandler {
	return &FoodHandler{
		service: service,
	}
}
