package services

import (
	"context"
	"errors"
	"log"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
	"mealplanner/internal/utils"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

type FoodService struct {
	db *database.DB
}

func NewFoodService(db *database.DB) *FoodService {
	return &FoodService{db: db}
}

func (s *FoodService) CreateFood(ctx context.Context, params db.CreateFoodParams) (*db.Food, error) {
	log.Default().Printf("Creating food: %v", params.Name)
	var food *db.Food
	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		log.Default().Printf("Add food to db: %v", params.Name)
		var err error
		food, err = q.CreateFood(ctx, params)
		return err
	})
	return food, err
}

func (s *FoodService) CreateRecipeWithIngredients(ctx context.Context, recipeParams db.CreateRecipeParams, ingredients []db.AddRecipeIngredientParams) error {
	log.Default().Printf("adding recipe with ingredients: %v", recipeParams.FoodID)
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		recipe, err := q.CreateRecipe(ctx, recipeParams)
		if err != nil {
			return err
		}

		for _, ing := range ingredients {
			ing.RecipeID = recipe.FoodID
			if err := q.AddRecipeIngredient(ctx, ing); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *FoodService) GetFoods(ctx context.Context, queryString string) ([]*models.Food, error) {
	dbFoods, err := s.db.Queries.SearchFoods(ctx, queryString)
	if err != nil {
		return nil, err
	}

	foods := make([]*models.Food, len(dbFoods))
	for i, dbFood := range dbFoods {
		density, err := dbFood.Density.Float64Value()
		if err != nil {
			log.Default().Println("Error parsing density: ", err)
			return nil, err
		}
		foods[i] = &models.Food{
			ID:       int(dbFood.ID),
			Name:     dbFood.Name,
			UnitType: dbFood.UnitType,
			BaseUnit: dbFood.BaseUnit,
			Density:  density.Float64,
			IsRecipe: dbFood.IsRecipe,
		}
	}

	return foods, nil
}

func (s *FoodService) DeleteFood(ctx context.Context, id string) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		idNum, err := strconv.ParseInt(id, 10, 32)
		if err != nil {
			// TODO: Handle proper error handling for when foreign key constraint fails
			log.Default().Printf("Error parsing id: %v", err)
			return err
		}
		return q.DeleteFood(ctx, int32(idNum))
	})
}

func (s *FoodService) GetFoodDetails(ctx context.Context, id string, depth int) (*models.Food, error) {
	idNum, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return nil, err
	}
	dbFoods, err := s.db.Queries.SearchFoodsWithDependencies(ctx, db.SearchFoodsWithDependenciesParams{
		SearchID: int32(idNum),
		MaxDepth: int32(depth),
	})
	log.Default().Printf("Found foods: %v", *dbFoods[0])

	if err != nil {
		return nil, err
	}
	if len(dbFoods) == 0 {
		return nil, errors.New("food not found")
	}

	foods := SearchResultToFoods(dbFoods)

	return foods[0], nil
}

func (s *FoodService) UpdateFood(ctx context.Context, updateParams db.UpdateFoodWithRecipeParams, ingredients []db.AddRecipeIngredientParams, returnUpdated bool) (*models.Food, error) {

	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		var err error
		// we update the food details and delete the recipe ingredients in this transaction
		updatedFood, err := q.UpdateFoodWithRecipe(ctx, updateParams)
		if err != nil {
			return err
		}
		for _, ing := range ingredients {
			ing.RecipeID = updatedFood.ID
			if err := q.AddRecipeIngredient(ctx, ing); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if returnUpdated {
		updatedFood, err := s.GetFoodDetails(ctx, strconv.Itoa(int(updateParams.ID)), 0)
		if err != nil {
			return nil, err
		}
		return updatedFood, nil
	}
	return nil, nil
}

func SearchResultToFoods(rows []*db.SearchFoodsWithDependenciesRow) []*models.Food {
    foodMap := make(map[int32]*models.Food)

    // First pass: Create all food objects
    for _, row := range rows {
        if _, exists := foodMap[row.ID]; exists {
            continue
        }

        food := &models.Food{
            ID:       int(row.ID),
            Name:     row.Name,
            UnitType: row.UnitType,
            BaseUnit: row.BaseUnit,
            IsRecipe: row.IsRecipe,
        }

        if row.Density.Valid {
            val, _ := row.Density.Float64Value()
            food.Density = val.Float64
        }

        if row.IsRecipe {
            yieldQty := 0.0
            if row.YieldQuantity.Valid {
                val, _ := row.YieldQuantity.Float64Value()
                yieldQty = val.Float64
            }

            food.Recipe = &models.Recipe{
                Instructions:   row.Instructions.String,
                URL:           row.Url.String,
                YieldQuantity: yieldQty,
                Ingredients:   make([]*models.RecipeItem, 0),
            }
        }

        foodMap[row.ID] = food
    }

    // Second pass: Build recipe relationships
    for _, row := range rows {
        if row.Depth > 0 && row.Quantity.Valid {
            parentFood := foodMap[rows[0].ID] // Root food
            if parentFood != nil && parentFood.Recipe != nil {
                quantity, _ := row.Quantity.Float64Value()

                ingredient := &models.RecipeItem{
                    FoodID:   int(row.ID),
                    Food:     foodMap[row.ID],
                    Quantity: quantity.Float64,
                    Unit:     row.Unit.String,
                }

                parentFood.Recipe.Ingredients = append(parentFood.Recipe.Ingredients, ingredient)
            }
        }
    }

    // Return root foods
    var result []*models.Food
    for _, row := range rows {
        if row.Depth == 0 {
            if food, exists := foodMap[row.ID]; exists {
                result = append(result, food)
            }
        }
    }

    return result
}

// Helper function to convert pgtype.Numeric to float64
func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	val, _ := n.Float64Value()
	return val.Float64
}

func (s *FoodService) GetFoodUnits(ctx context.Context, id string) ([]string, string, error) {
	targetFood, err := s.GetFoodDetails(ctx, id, 0)
	if err != nil {
		log.Default().Printf("Error getting food to find units: %v", err)
		return nil, "", err
	}
	units := utils.GetUnitsByType(targetFood.UnitType)
	if units == nil ||  len(units) == 0 {
		log.Default().Printf("Invalid unit type: %s", targetFood.UnitType)
		return nil, "", errors.New("invalid unit type")
	}

	return units, targetFood.BaseUnit, nil
}
