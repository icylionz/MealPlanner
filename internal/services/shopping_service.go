package services

import (
	"context"
	"log"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
	"mealplanner/internal/utils"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ShoppingService struct {
	db             *database.DB
	scheduleService *ScheduleService
}

func NewShoppingService(db *database.DB, scheduleService *ScheduleService) *ShoppingService {
	return &ShoppingService{
		db:             db,
		scheduleService: scheduleService,
	}
}

func (s *ShoppingService) CreateShoppingList(ctx context.Context, name string, startDate, endDate time.Time) (*models.ShoppingList, error) {
	var shoppingList *db.ShoppingList
	
	err := s.db.WithTx(ctx, func(q *db.Queries) error {
		var err error
		shoppingList, err = q.CreateShoppingList(ctx, db.CreateShoppingListParams{
			Name:      name,
			StartDate: pgtype.Date{Time: startDate, Valid: true},
			EndDate:   pgtype.Date{Time: endDate, Valid: true},
		})
		return err
	})
	
	if err != nil {
		return nil, err
	}
	
	return models.ToShoppingListModel(shoppingList), nil
}

func (s *ShoppingService) GetShoppingLists(ctx context.Context) ([]*models.ShoppingList, error) {
	dbShoppingLists, err := s.db.GetShoppingLists(ctx)
	if err != nil {
		return nil, err
	}
	
	return models.ToShoppingListsModel(dbShoppingLists), nil
}

func (s *ShoppingService) GetShoppingListById(ctx context.Context, id int) (*models.ShoppingList, error) {
	dbShoppingList, err := s.db.GetShoppingListById(ctx, int32(id))
	if err != nil {
		return nil, err
	}
	
	shoppingList := models.ToShoppingListModel(dbShoppingList)
	
	// Get items
	items, err := s.db.GetShoppingListItems(ctx, pgtype.Int4{Int32: dbShoppingList.ID})
	if err != nil {
		return nil, err
	}
	shoppingList.Items = models.ToShoppingListItemsModel(items)
	
	// Get meals
	meals, err := s.db.GetShoppingListMeals(ctx, pgtype.Int4{Int32: dbShoppingList.ID})
	if err != nil {
		return nil, err
	}
	shoppingList.Meals = models.ToShoppingListMealsModel(meals)
	
	return shoppingList, nil
}

func (s *ShoppingService) DeleteShoppingList(ctx context.Context, id int) error {
	return s.db.DeleteShoppingList(ctx, int32(id))
}

func (s *ShoppingService) AddMealToShoppingList(ctx context.Context, shoppingListId, scheduleId int) error {
	return s.db.AddShoppingListMeal(ctx, db.AddShoppingListMealParams{
		ShoppingListID: pgtype.Int4{Int32: int32(shoppingListId)},
		ScheduleID:     pgtype.Int4{Int32: int32(scheduleId)},
	})
}

func (s *ShoppingService) RemoveMealFromShoppingList(ctx context.Context, shoppingListId, scheduleId int) error {
	return s.db.RemoveShoppingListMeal(ctx, db.RemoveShoppingListMealParams{
		ShoppingListID: pgtype.Int4{Int32: int32(shoppingListId)},
		ScheduleID:     pgtype.Int4{Int32: int32(scheduleId)},
	})
}

func (s *ShoppingService) GenerateShoppingListFromDateRange(ctx context.Context, name string, startDate, endDate time.Time) (*models.ShoppingList, error) {
	// Create shopping list
	shoppingList, err := s.CreateShoppingList(ctx, name, startDate, endDate)
	if err != nil {
		return nil, err
	}
	
	// Get schedules in date range
	schedules, err := s.scheduleService.GetSchedulesForRange(ctx, &startDate, &endDate)
	if err != nil {
		return nil, err
	}
	
	// Add each schedule to the shopping list
	for _, schedule := range schedules {
		err = s.AddMealToShoppingList(ctx, shoppingList.ID, schedule.ID)
		if err != nil {
			log.Printf("Error adding meal to shopping list: %v", err)
		}
	}
	
	// Generate shopping list items
	err = s.RegenerateShoppingListItems(ctx, shoppingList.ID)
	if err != nil {
		return nil, err
	}
	
	// Get the fully populated shopping list
	return s.GetShoppingListById(ctx, shoppingList.ID)
}

func (s *ShoppingService) RegenerateShoppingListItems(ctx context.Context, shoppingListId int) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		// Delete existing items
		// (This would need an additional query to be added)
		_, err := q.DeleteShoppingListItemsByListId(ctx, pgtype.Int4{Int32: int32(shoppingListId)})
		if err != nil {
			return err
		}
		
		// Get all ingredients needed
		ingredients, err := q.GenerateShoppingListItems(ctx, pgtype.Int4{Int32: int32(shoppingListId)})
		if err != nil {
			return err
		}
		
		// Create shopping list items
		for _, ing := range ingredients {
			quantity := ing.TotalQuantity
			_, err = q.CreateShoppingListItem(ctx, db.CreateShoppingListItemParams{
				ShoppingListID: ing.ShoppingListID,
				FoodID:         pgtype.Int4{Int32: ing.FoodID},
				Quantity:       quantity,
				Unit:           ing.Unit,
			})
			if err != nil {
				return err
			}
		}
		
		return nil
	})
}

func (s *ShoppingService) RecordItemPurchase(ctx context.Context, itemId int, actualQuantity float64, price float64) error {
	_, err := s.db.RecordItemPurchase(ctx, db.RecordItemPurchaseParams{
		ShoppingListItemID: pgtype.Int4{Int32: int32(itemId)},
		ActualQuantity:     utils.Float64ToNumeric(actualQuantity),
		Price:              utils.Float64ToNumeric(price),
	})
	return err
}

func (s *ShoppingService) ExportShoppingListAsText(ctx context.Context, id int) (string, error) {
	shoppingList, err := s.GetShoppingListById(ctx, id)
	if err != nil {
		return "", err
	}
	
	// Generate text format
	text := "SHOPPING LIST: " + shoppingList.Name + "\n"
	text += "Date Range: " + shoppingList.StartDate.Format("Jan 2") + " - " + shoppingList.EndDate.Format("Jan 2, 2006") + "\n\n"
	
	text += "ITEMS:\n"
	for _, item := range shoppingList.Items {
		text += "- " + item.FoodName + ": " + utils.FormatQuantity(item.Quantity) + " " + item.Unit + "\n"
	}
	
	text += "\nMEALS:\n"
	for _, meal := range shoppingList.Meals {
		text += "- " + meal.FoodName + " (" + meal.ScheduledAt.Format("Mon, Jan 2 3:04PM") + ")\n"
	}
	
	return text, nil
}