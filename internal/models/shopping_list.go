package models

import (
    "time"
)

type ShoppingList struct {
    ID        int                  `json:"id"`
    Name      string               `json:"name"`
    Notes     string               `json:"notes,omitempty"`
    CreatedAt time.Time            `json:"createdAt"`
    UpdatedAt time.Time            `json:"updatedAt"`
    Items     []*ShoppingListItem  `json:"items,omitempty"`
    Sources   []*ShoppingListSource `json:"sources,omitempty"`
}

type ShoppingListItem struct {
    ID               int                         `json:"id"`
    ShoppingListID   int                         `json:"shoppingListId"`
    FoodID           int                         `json:"foodId"`
    FoodName         string                      `json:"foodName"`
    Quantity         float64                     `json:"quantity"`
    Unit             string                      `json:"unit"`
    UnitType         string                      `json:"unitType"`
    Notes            string                      `json:"notes,omitempty"`
    Purchased        bool                        `json:"purchased"`
    ActualQuantity   float64                     `json:"actualQuantity,omitempty"`
    ActualPrice      float64                     `json:"actualPrice,omitempty"`
    Sources          []*ShoppingListItemSource   `json:"sources,omitempty"`
}

type ShoppingListSource struct {
    ID             int       `json:"id"`
    ShoppingListID int       `json:"shoppingListId"`
    SourceType     string    `json:"sourceType"` // 'schedule', 'recipe', 'manual', 'copy'
    SourceID       int       `json:"sourceId,omitempty"`
    SourceName     string    `json:"sourceName"`
    Servings       float64   `json:"servings,omitempty"`
    AddedAt        time.Time `json:"addedAt"`
}

type ShoppingListItemSource struct {
    ItemID              int     `json:"itemId"`
    SourceID            int     `json:"sourceId"`
    ContributedQuantity float64 `json:"contributedQuantity"`
}

// Request types for different add operations
type AddManualItemRequest struct {
    FoodID   int     `json:"foodId"`
    Quantity float64 `json:"quantity"`
    Unit     string  `json:"unit"`
    Notes    string  `json:"notes,omitempty"`
}

type AddRecipeRequest struct {
    RecipeID int     `json:"recipeId"`
    Servings float64 `json:"servings"`
}

type AddSchedulesRequest struct {
    ScheduleIDs []int `json:"scheduleIds"`
}

type AddDateRangeRequest struct {
    StartDate time.Time `json:"startDate"`
    EndDate   time.Time `json:"endDate"`
}
