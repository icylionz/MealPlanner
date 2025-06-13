package services

import (
	"context"
	"fmt"
	"log"
	"mealplanner/internal/database"
	"mealplanner/internal/database/db"
	"mealplanner/internal/models"
	"mealplanner/internal/utils"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ShoppingService struct {
	db              *database.DB
	scheduleService *ScheduleService
	foodService     *FoodService
}

func NewShoppingService(db *database.DB, scheduleService *ScheduleService, foodService *FoodService) *ShoppingService {
	return &ShoppingService{
		db:              db,
		scheduleService: scheduleService,
		foodService:     foodService,
	}
}

// Basic CRUD operations
func (s *ShoppingService) CreateShoppingList(ctx context.Context, name, notes string) (*models.ShoppingList, error) {
	dbList, err := s.db.CreateShoppingList(ctx, db.CreateShoppingListParams{
		Name:  name,
		Notes: pgtype.Text{String: notes, Valid: notes != ""},
	})
	if err != nil {
		return nil, err
	}

	return &models.ShoppingList{
		ID:        int(dbList.ID),
		Name:      dbList.Name,
		Notes:     dbList.Notes.String,
		CreatedAt: dbList.CreatedAt.Time,
		UpdatedAt: dbList.UpdatedAt.Time,
		Items:     []*models.ShoppingListItem{},
		Sources:   []*models.ShoppingListSource{},
	}, nil
}

func (s *ShoppingService) GetShoppingLists(ctx context.Context) ([]*models.ShoppingList, error) {
	dbLists, err := s.db.GetShoppingLists(ctx)
	if err != nil {
		return nil, err
	}

	lists := make([]*models.ShoppingList, len(dbLists))
	for i, dbList := range dbLists {
		lists[i] = &models.ShoppingList{
			ID:        int(dbList.ID),
			Name:      dbList.Name,
			Notes:     dbList.Notes.String,
			CreatedAt: dbList.CreatedAt.Time,
			UpdatedAt: dbList.UpdatedAt.Time,
		}
	}
	return lists, nil
}

func (s *ShoppingService) GetShoppingListById(ctx context.Context, id int) (*models.ShoppingList, error) {
	dbList, err := s.db.GetShoppingListById(ctx, int32(id))
	if err != nil {
		return nil, err
	}

	list := &models.ShoppingList{
		ID:        int(dbList.ID),
		Name:      dbList.Name,
		Notes:     dbList.Notes.String,
		CreatedAt: dbList.CreatedAt.Time,
		UpdatedAt: dbList.UpdatedAt.Time,
	}

	// Get items with sources
	items, err := s.getShoppingListItemsWithSources(ctx, id)
	if err != nil {
		return nil, err
	}
	list.Items = items

	// Get sources
	sources, err := s.getShoppingListSources(ctx, id)
	if err != nil {
		return nil, err
	}
	list.Sources = sources

	return list, nil
}

func (s *ShoppingService) DeleteShoppingList(ctx context.Context, id int) error {
	return s.db.DeleteShoppingList(ctx, int32(id))
}

// Adding items from different sources
func (s *ShoppingService) AddManualItem(ctx context.Context, listId int, req *models.AddManualItemRequest) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		// Get food details for proper unit type
		food, err := q.GetFood(ctx, int32(req.FoodID))
		if err != nil {
			return err
		}

		// Create source record
		source, err := q.CreateShoppingListSource(ctx, db.CreateShoppingListSourceParams{
			ShoppingListID: pgtype.Int4{Int32: int32(listId), Valid: true},
			SourceType:     "manual",
			SourceName:     fmt.Sprintf("Manual: %s", food.Name),
		})
		if err != nil {
			return err
		}

		// Add or update item
		return s.addOrUpdateItem(ctx, q, int32(listId), &itemInfo{
			FoodID:    req.FoodID,
			FoodName:  food.Name,
			Quantity:  req.Quantity,
			Unit:      req.Unit,
			UnitType:  food.UnitType,
			Notes:     req.Notes,
			SourceID:  int(source.ID),
			SourceQty: req.Quantity,
		})
	})
}

func (s *ShoppingService) AddRecipe(ctx context.Context, listId int, req *models.AddRecipeRequest) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		// Get recipe details
		recipe, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", req.RecipeID), 15)
		if err != nil {
			return err
		}

		if !recipe.IsRecipe || recipe.Recipe == nil {
			return fmt.Errorf("food %d is not a recipe", req.RecipeID)
		}

		// Create source record
		source, err := q.CreateShoppingListSource(ctx, db.CreateShoppingListSourceParams{
			ShoppingListID: pgtype.Int4{Int32: int32(listId), Valid: true},
			SourceType:     "recipe",
			SourceID:       pgtype.Int4{Int32: int32(req.RecipeID), Valid: true},
			SourceName:     fmt.Sprintf("Recipe: %s (%.1fx)", recipe.Name, req.Servings),
			Servings:       utils.Float64ToNumeric(req.Servings),
		})
		if err != nil {
			return err
		}

		// Calculate scaling factor
		scaleFactor := req.Servings / recipe.Recipe.YieldQuantity

		// Add all base ingredients (recursive extraction)
		return s.addRecipeIngredients(ctx, q, int32(listId), recipe, scaleFactor, int(source.ID))
	})
}

func (s *ShoppingService) AddSchedules(ctx context.Context, listId int, req *models.AddSchedulesRequest, timeZone *time.Location) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		for _, scheduleID := range req.ScheduleIDs {
			// Get schedule details
			schedule, err := s.scheduleService.GetScheduleById(ctx, scheduleID, timeZone)
			if err != nil {
				log.Default().Printf("Error getting schedule %d: %v", scheduleID, err)
				return err
			}

			// Get food/recipe details
			food, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", schedule.FoodID), 15)
			if err != nil {
				log.Default().Printf("Error getting food details for schedule %d: %v", scheduleID, err)
				return err
			}

			// Create source record
			source, err := q.CreateShoppingListSource(ctx, db.CreateShoppingListSourceParams{
				ShoppingListID: pgtype.Int4{Int32: int32(listId), Valid: true},
				SourceType:     "schedule",
				SourceID:       pgtype.Int4{Int32: int32(scheduleID), Valid: true},
				SourceName:     fmt.Sprintf("Scheduled: %s on %s", food.Name, schedule.ScheduledAt.Format("Jan 2")),
				Servings:       utils.Float64ToNumeric(schedule.Servings),
			})
			if err != nil {
				log.Default().Printf("Error creating source for schedule %d: %v", scheduleID, err)
				return err
			}

			if food.IsRecipe && food.Recipe != nil {
				// Add recipe ingredients
				scaleFactor := schedule.Servings / food.Recipe.YieldQuantity
				err = s.addRecipeIngredients(ctx, q, int32(listId), food, scaleFactor, int(source.ID))
			} else {
				// Add basic food
				err = s.addOrUpdateItem(ctx, q, int32(listId), &itemInfo{
					FoodID:    food.ID,
					FoodName:  food.Name,
					Quantity:  schedule.Servings,
					Unit:      food.BaseUnit,
					UnitType:  food.UnitType,
					SourceID:  int(source.ID),
					SourceQty: schedule.Servings,
				})
			}
			if err != nil {
				log.Default().Printf("Error adding schedule %d to list %d: %v", scheduleID, listId, err)
				return err
			}
		}
		return nil
	})
}

func (s *ShoppingService) AddDateRange(ctx context.Context, listId int, req *models.AddDateRangeRequest, timeZone *time.Location) error {
	// Get all schedules in range
	schedules, err := s.scheduleService.GetSchedulesForRange(ctx, &req.StartDate, &req.EndDate, timeZone)
	if err != nil {
		return err
	}

	// Convert to schedule IDs and add
	scheduleIDs := make([]int, len(schedules))
	for i, schedule := range schedules {
		scheduleIDs[i] = schedule.ID
	}

	return s.AddSchedules(ctx, listId, &models.AddSchedulesRequest{
		ScheduleIDs: scheduleIDs,
	}, timeZone)
}

func (s *ShoppingService) UpdateItemNotes(ctx context.Context, itemId int, notes string) error {
	return s.db.UpdateShoppingListItemNotes(ctx, db.UpdateShoppingListItemNotesParams{
		ID:    int32(itemId),
		Notes: pgtype.Text{String: notes, Valid: notes != ""},
	})
}

func (s *ShoppingService) MarkItemPurchased(ctx context.Context, itemId int, purchased bool, actualQuantity, actualPrice float64) error {
	return s.db.MarkShoppingListItemPurchased(ctx, db.MarkShoppingListItemPurchasedParams{
		ID:             int32(itemId),
		Purchased:      pgtype.Bool{Bool: purchased, Valid: purchased},
		ActualQuantity: pgtype.Numeric{Valid: actualQuantity > 0},
		ActualPrice:    pgtype.Numeric{Valid: actualPrice > 0},
	})
}

func (s *ShoppingService) RemoveItem(ctx context.Context, itemId int) error {
	return s.db.DeleteShoppingListItem(ctx, int32(itemId))
}

func (s *ShoppingService) RemoveItemsBySource(ctx context.Context, sourceId int) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		// Remove all item-source links for this source
		err := q.DeleteShoppingListItemSourcesBySource(ctx, int32(sourceId))
		if err != nil {
			return err
		}

		// Remove items that no longer have any sources
		err = q.DeleteOrphanedShoppingListItems(ctx)
		if err != nil {
			return err
		}

		// Remove the source itself
		return q.DeleteShoppingListSource(ctx, int32(sourceId))
	})
}

// Helper types and functions
type itemInfo struct {
	FoodID    int
	FoodName  string
	Quantity  float64
	Unit      string
	UnitType  string
	Notes     string
	SourceID  int
	SourceQty float64
}

func (s *ShoppingService) addOrUpdateItem(ctx context.Context, q *db.Queries, listId int32, info *itemInfo) error {
	existingItem, err := q.FindCompatibleShoppingListItem(ctx, db.FindCompatibleShoppingListItemParams{
		ShoppingListID: pgtype.Int4{Int32: listId, Valid: true},
		FoodID:         pgtype.Int4{Int32: int32(info.FoodID), Valid: true},
		Unit:           info.Unit,
	})

	if err == nil {
		return q.CreateShoppingListItemSource(ctx, db.CreateShoppingListItemSourceParams{
			ShoppingListItemID:   existingItem.ID,
			ShoppingListSourceID: int32(info.SourceID),
			ContributedQuantity:  utils.Float64ToNumeric(info.SourceQty),
		})
	}

	newItem, err := q.CreateShoppingListItem(ctx, db.CreateShoppingListItemParams{
		ShoppingListID: pgtype.Int4{Int32: listId, Valid: true},
		FoodID:         pgtype.Int4{Int32: int32(info.FoodID), Valid: true},
		FoodName:       info.FoodName,
		Unit:           info.Unit,
		UnitType:       info.UnitType,
		Notes:          pgtype.Text{String: info.Notes, Valid: info.Notes != ""},
	})
	if err != nil {
		return err
	}

	return q.CreateShoppingListItemSource(ctx, db.CreateShoppingListItemSourceParams{
		ShoppingListItemID:   newItem.ID,
		ShoppingListSourceID: int32(info.SourceID),
		ContributedQuantity:  utils.Float64ToNumeric(info.SourceQty),
	})
}

func (s *ShoppingService) addRecipeIngredients(ctx context.Context, q *db.Queries, listId int32, recipe *models.Food, scaleFactor float64, sourceID int) error {
	return s.addRecipeIngredientsRecursive(ctx, q, listId, recipe, scaleFactor, sourceID, 0)
}

func (s *ShoppingService) addRecipeIngredientsRecursive(ctx context.Context, q *db.Queries, listId int32, recipe *models.Food, scaleFactor float64, sourceID int, depth int) error {
	if depth > 15 {
		return fmt.Errorf("maximum recipe depth exceeded")
	}

	for _, ingredient := range recipe.Recipe.Ingredients {
		scaledQuantity := ingredient.Quantity * scaleFactor

		if ingredient.Food.IsRecipe && ingredient.Food.Recipe != nil {
			// Recursive case: get full ingredient details and recurse
			fullIngredient, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", ingredient.Food.ID), 15)
			if err != nil {
				log.Default().Printf("Error getting full ingredient details: %v", err)
				return err
			}

			// Calculate new scale factor
			ingredientScale := scaledQuantity / fullIngredient.Recipe.YieldQuantity
			err = s.addRecipeIngredientsRecursive(ctx, q, listId, fullIngredient, ingredientScale, sourceID, depth+1)
			if err != nil {
				log.Default().Printf("Error adding recipe ingredients: %v", err)
				return err
			}
		} else {
			// Base case: add the ingredient
			err := s.addOrUpdateItem(ctx, q, listId, &itemInfo{
				FoodID:    ingredient.Food.ID,
				FoodName:  ingredient.Food.Name,
				Quantity:  scaledQuantity,
				Unit:      ingredient.Unit,
				UnitType:  ingredient.Food.UnitType,
				SourceID:  sourceID,
				SourceQty: scaledQuantity,
			})
			if err != nil {
				log.Default().Printf("Error adding recipe ingredient: %v", err)
				return err
			}
		}
	}
	log.Default().Printf("Added recipe ingredients for %s", recipe.Name)
	return nil
}

func (s *ShoppingService) getShoppingListItemsWithSources(ctx context.Context, listId int) ([]*models.ShoppingListItem, error) {
	dbItems, err := s.db.GetShoppingListItemsWithCalculatedQuantities(ctx, pgtype.Int4{Int32: int32(listId), Valid: true})
	if err != nil {
		return nil, err
	}

	itemMap := make(map[int32]*models.ShoppingListItem)

	for _, dbItem := range dbItems {
		item, exists := itemMap[dbItem.ID]
		if !exists {
			calculatedQuantity, _ := dbItem.CalculatedQuantity.Float64Value()
			actualQuantity, _ := dbItem.ActualQuantity.Float64Value()
			actualPrice, _ := dbItem.ActualPrice.Float64Value()

			item = &models.ShoppingListItem{
				ID:             int(dbItem.ID),
				ShoppingListID: int(dbItem.ShoppingListID.Int32),
				FoodID:         int(dbItem.FoodID.Int32),
				FoodName:       dbItem.FoodName,
				Quantity:       calculatedQuantity.Float64, // Now calculated from sources
				Unit:           dbItem.Unit,
				UnitType:       dbItem.UnitType,
				Notes:          dbItem.Notes.String,
				Purchased:      dbItem.Purchased.Bool,
				ActualQuantity: actualQuantity.Float64,
				ActualPrice:    actualPrice.Float64,
				Sources:        []*models.ShoppingListItemSource{},
			}
			itemMap[dbItem.ID] = item
		}

		// Add source if present
		if dbItem.SourceID.Valid {
			contributedQty, _ := dbItem.ContributedQuantity.Float64Value()
			item.Sources = append(item.Sources, &models.ShoppingListItemSource{
				ItemID:              int(dbItem.ID),
				SourceID:            int(dbItem.SourceID.Int32),
				ContributedQuantity: contributedQty.Float64,
			})
		}
	}

	// Convert map to slice
	items := make([]*models.ShoppingListItem, 0, len(itemMap))
	for _, item := range itemMap {
		items = append(items, item)
	}

	return items, nil
}

func (s *ShoppingService) getShoppingListSources(ctx context.Context, listId int) ([]*models.ShoppingListSource, error) {
	dbSources, err := s.db.GetShoppingListSources(ctx, pgtype.Int4{Int32: int32(listId), Valid: true})
	if err != nil {
		return nil, err
	}

	sources := make([]*models.ShoppingListSource, len(dbSources))
	for i, dbSource := range dbSources {
		servings, _ := dbSource.Servings.Float64Value()
		sources[i] = &models.ShoppingListSource{
			ID:             int(dbSource.ID),
			ShoppingListID: int(dbSource.ShoppingListID.Int32),
			SourceType:     dbSource.SourceType,
			SourceID:       int(dbSource.SourceID.Int32),
			SourceName:     dbSource.SourceName,
			Servings:       servings.Float64,
			AddedAt:        dbSource.AddedAt.Time,
		}
	}
	return sources, nil
}

// Export functionality
func (s *ShoppingService) ExportShoppingListAsText(ctx context.Context, id int) (string, error) {
	list, err := s.GetShoppingListById(ctx, id)
	if err != nil {
		return "", err
	}

	text := fmt.Sprintf("SHOPPING LIST: %s\n", list.Name)
	if list.Notes != "" {
		text += fmt.Sprintf("Notes: %s\n", list.Notes)
	}
	text += fmt.Sprintf("Created: %s\n\n", list.CreatedAt.Format("Jan 2, 2006"))

	text += "ITEMS TO BUY:\n"
	for _, item := range list.Items {
		status := ""
		if item.Purchased {
			status = " ✓"
		}
		text += fmt.Sprintf("□ %s: %s %s%s\n",
			item.FoodName,
			utils.FormatQuantity(item.Quantity),
			item.Unit,
			status)

		if item.Notes != "" {
			text += fmt.Sprintf("  Note: %s\n", item.Notes)
		}
	}

	if len(list.Sources) > 0 {
		text += "\nSOURCES:\n"
		for _, source := range list.Sources {
			text += fmt.Sprintf("- %s\n", source.SourceName)
		}
	}

	return text, nil
}
