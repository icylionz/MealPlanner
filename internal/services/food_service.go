package services

import (
	"context"
	"errors"
	"log"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
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
    var food *db.Food
    err := s.db.WithTx(ctx, func(q *db.Queries) error {
        var err error
        food, err = q.CreateFood(ctx, params)
        return err
    })
    return food, err
}

func (s *FoodService) CreateRecipeWithIngredients(ctx context.Context, recipeParams db.CreateRecipeParams, ingredients []db.AddRecipeIngredientParams) error {
    return s.db.WithTx(ctx, func(q *db.Queries) error {
        _, err := q.CreateRecipe(ctx, recipeParams)
        if err != nil {
            return err
        }

        for _, ing := range ingredients {
            if err := q.AddRecipeIngredient(ctx, ing); err != nil {
                return err
            }
        }
        return nil
    })
}

func (s *FoodService) GetFoods(ctx context.Context, queryString string) ([]*models.Food, error) {
	dbFoods, err := s.db.Queries.SearchFoods(ctx, pgtype.Text{String: queryString})
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
			ID: int(dbFood.ID),
			Name: dbFood.Name,
			UnitType: dbFood.UnitType,
			BaseUnit: dbFood.BaseUnit,
			Density: density.Float64,
			IsRecipe: dbFood.IsRecipe,
		}
	}

	return foods, nil
}

func (s *FoodService) DeleteFood(ctx context.Context, id string) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		idNum, err := strconv.ParseInt(id, 10, 32)
		if err != nil {
			return err
		}
		return q.DeleteFood(ctx, int32(idNum))
	})
}

func (s *FoodService) GetFoodDetails(ctx context.Context, id string) (*models.Food, error) {
	idNum, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return nil, err
	}
	dbFoods, err := s.db.Queries.SearchFoodsWithDependencies(ctx, db.SearchFoodsWithDependenciesParams{
		SearchID: int32(idNum),
		MaxDepth: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(dbFoods) == 0 {
		return nil, errors.New("food not found")
	}

	foods := SearchResultToFoods(dbFoods)


	return foods[0], nil
}

func SearchResultToFoods(rows []*db.SearchFoodsWithDependenciesRow) []*models.Food {
    foodMap := make(map[int32]*models.Food)

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
            food.Recipe = &models.Recipe{
                Instructions: "",  // Will be filled in second pass
                Ingredients: make([]models.RecipeItem, 0),
            }
        }

        foodMap[row.ID] = food
    }

    for _, row := range rows {
        if row.Depth > 0 && row.Unit.Valid && row.Quantity.Valid {
            parentID := rows[0].ID
            if parent, exists := foodMap[parentID]; exists && parent.Recipe != nil {
                quantity, _ := row.Quantity.Float64Value()

                ingredient := models.RecipeItem{
                    FoodID:   int(row.ID),
                    Food:     foodMap[row.ID],
                    Quantity: quantity.Float64,
                    Unit:     row.Unit.String,
                }

                parent.Recipe.Ingredients = append(parent.Recipe.Ingredients, ingredient)
            }
        }
    }

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
