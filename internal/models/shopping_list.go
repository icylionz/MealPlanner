package models

import (
	"mealplanner/internal/database/db"
	"time"
)

type ShoppingList struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
	CreatedAt time.Time `json:"createdAt"`
	Items     []*ShoppingListItem `json:"items,omitempty"`
	Meals     []*ShoppingListMeal `json:"meals,omitempty"`
}

type ShoppingListItem struct {
	ID             int     `json:"id"`
	ShoppingListID int     `json:"shoppingListId"`
	FoodID         int     `json:"foodId"`
	FoodName       string  `json:"foodName"`
	UnitType       string  `json:"unitType"`
	BaseUnit       string  `json:"baseUnit"`
	Quantity       float64 `json:"quantity"`
	Unit           string  `json:"unit"`
	Purchased      bool    `json:"purchased"`
	ActualQuantity float64 `json:"actualQuantity,omitempty"`
	Price          float64 `json:"price,omitempty"`
}

type ShoppingListMeal struct {
	ID             int       `json:"id"`
	ShoppingListID int       `json:"shoppingListId"`
	ScheduleID     int       `json:"scheduleId"`
	FoodName       string    `json:"foodName"`
	ScheduledAt    time.Time `json:"scheduledAt"`
}

// Conversion functions
func ToShoppingListModel(sl *db.ShoppingList) *ShoppingList {
	return &ShoppingList{
		ID:        int(sl.ID),
		Name:      sl.Name,
		StartDate: sl.StartDate.Time,
		EndDate:   sl.EndDate.Time,
		CreatedAt: sl.CreatedAt.Time,
		Items:     []*ShoppingListItem{},
		Meals:     []*ShoppingListMeal{},
	}
}

func ToShoppingListsModel(slList []*db.ShoppingList) []*ShoppingList {
	result := make([]*ShoppingList, len(slList))
	for i, sl := range slList {
		result[i] = ToShoppingListModel(sl)
	}
	return result
}

func ToShoppingListItemModel(item *db.GetShoppingListItemsRow) *ShoppingListItem {
	quantity, _ := item.Quantity.Float64Value()
	
	return &ShoppingListItem{
		ID:             int(item.ID),
		ShoppingListID: int(item.ShoppingListID.Int32),
		FoodID:         int(item.FoodID.Int32),
		FoodName:       item.FoodName,
		UnitType:       item.UnitType,
		BaseUnit:       item.BaseUnit,
		Quantity:       quantity.Float64,
		Unit:           item.Unit,
		Purchased:      false, // Would need to be calculated based on purchases
	}
}

func ToShoppingListItemsModel(items []*db.GetShoppingListItemsRow) []*ShoppingListItem {
	result := make([]*ShoppingListItem, len(items))
	for i, item := range items {
		result[i] = ToShoppingListItemModel(item)
	}
	return result
}

func ToShoppingListMealModel(meal *db.GetShoppingListMealsRow) *ShoppingListMeal {
	return &ShoppingListMeal{
		ID:             int(meal.ID),
		ShoppingListID: int(meal.ShoppingListID.Int32),
		ScheduleID:     int(meal.ScheduleID.Int32),
		FoodName:       meal.FoodName,
		ScheduledAt:    meal.ScheduledAt.Time,
	}
}

func ToShoppingListMealsModel(meals []*db.GetShoppingListMealsRow) []*ShoppingListMeal {
	result := make([]*ShoppingListMeal, len(meals))
	for i, meal := range meals {
		result[i] = ToShoppingListMealModel(meal)
	}
	return result
}