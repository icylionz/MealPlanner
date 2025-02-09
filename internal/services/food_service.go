package services

import (
	"context"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
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
