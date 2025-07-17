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

func (s *ShoppingService) addBasicFood(ctx context.Context, q *db.Queries, listId int32, sourceID int, food *models.Food, quantity float64) error {
	// Use same batch approach for consistency
	collected := map[string]*CollectedIngredient{
		fmt.Sprintf("%d|%s", food.ID, food.BaseUnit): {
			FoodID:   food.ID,
			FoodName: food.Name,
			Unit:     food.BaseUnit,
			UnitType: food.UnitType,
			Quantity: quantity,
		},
	}

	return s.batchInsertIngredients(ctx, q, listId, sourceID, collected)
}

// Adding items from different sources
func (s *ShoppingService) AddManualItem(ctx context.Context, listId int, req *models.AddManualItemRequest) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		// Get food details
		food, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", req.FoodID), 1)
		if err != nil {
			return fmt.Errorf("failed to get food %d: %w", req.FoodID, err)
		}

		// Create source record
		source, err := q.CreateShoppingListSource(ctx, db.CreateShoppingListSourceParams{
			ShoppingListID: pgtype.Int4{Int32: int32(listId), Valid: true},
			SourceType:     "manual",
			SourceName:     fmt.Sprintf("Manual: %s", food.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to create manual source: %w", err)
		}

		// Add single item using batch approach for consistency
		collected := map[string]*CollectedIngredient{
			fmt.Sprintf("%d|%s", req.FoodID, req.Unit): {
				FoodID:   req.FoodID,
				FoodName: food.Name,
				Unit:     req.Unit,
				UnitType: food.UnitType,
				Quantity: req.Quantity,
			},
		}

		return s.batchInsertIngredients(ctx, q, int32(listId), int(source.ID), collected)
	})
}

func (s *ShoppingService) AddRecipe(ctx context.Context, listId int, req *models.AddRecipeRequest) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		// Get recipe details
		recipe, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", req.RecipeID), 1)
		if err != nil {
			return fmt.Errorf("failed to get recipe %d: %w", req.RecipeID, err)
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
			return fmt.Errorf("failed to create source: %w", err)
		}

		// Calculate scaling factor and add ingredients
		scaleFactor := req.Servings / recipe.Recipe.YieldQuantity
		return s.addRecipeIngredients(ctx, q, int32(listId), recipe, scaleFactor, int(source.ID))
	})
}

func (s *ShoppingService) AddSchedules(ctx context.Context, listId int, req *models.AddSchedulesRequest, timeZone *time.Location) error {
	return s.db.WithTx(ctx, func(q *db.Queries) error {
		for _, scheduleID := range req.ScheduleIDs {
			// Get schedule details
			schedule, err := s.scheduleService.GetScheduleById(ctx, scheduleID, timeZone)
			if err != nil {
				return fmt.Errorf("failed to get schedule %d: %w", scheduleID, err)
			}

			// Get food/recipe details
			food, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", schedule.FoodID), 1)
			if err != nil {
				return fmt.Errorf("failed to get food %d for schedule %d: %w", schedule.FoodID, scheduleID, err)
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
				return fmt.Errorf("failed to create source for schedule %d: %w", scheduleID, err)
			}

			// Add ingredients based on food type
			if food.IsRecipe && food.Recipe != nil {
				scaleFactor := schedule.Servings / food.Recipe.YieldQuantity
				err = s.addRecipeIngredients(ctx, q, int32(listId), food, scaleFactor, int(source.ID))
			} else {
				err = s.addBasicFood(ctx, q, int32(listId), int(source.ID), food, schedule.Servings)
			}

			if err != nil {
				return fmt.Errorf("failed to add ingredients for schedule %d: %w", scheduleID, err)
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

type CollectedIngredient struct {
	FoodID   int
	FoodName string
	Unit     string
	UnitType string
	Quantity float64
}

func (s *ShoppingService) addRecipeIngredients(ctx context.Context, q *db.Queries, listId int32, recipe *models.Food, scaleFactor float64, sourceID int) error {
	// Step 1: Collect all base ingredients (simple recursive logic)
	collected := make(map[string]*CollectedIngredient)
	err := s.collectBaseIngredients(ctx, recipe, scaleFactor, collected, 0)
	if err != nil {
		return fmt.Errorf("failed to collect ingredients: %w", err)
	}

	if len(collected) == 0 {
		log.Default().Printf("No base ingredients found for recipe %s", recipe.Name)
		return nil
	}

	// Step 2: Batch insert items and sources (2 DB calls total)
	return s.batchInsertIngredients(ctx, q, listId, sourceID, collected)
}

func (s *ShoppingService) collectBaseIngredients(ctx context.Context, recipe *models.Food, scaleFactor float64, collected map[string]*CollectedIngredient, depth int) error {
	if depth > 15 {
		return fmt.Errorf("recipe depth limit exceeded")
	}

	for _, ingredient := range recipe.Recipe.Ingredients {
		scaledQty := ingredient.Quantity * scaleFactor

		if ingredient.Food.IsRecipe && ingredient.Food.Recipe != nil {
			// Get full recipe details and recurse
			fullRecipe, err := s.foodService.GetFoodDetails(ctx, fmt.Sprintf("%d", ingredient.Food.ID), 1)
			if err != nil {
				return fmt.Errorf("failed to get recipe %d: %w", ingredient.Food.ID, err)
			}

			nestedScale := scaledQty / fullRecipe.Recipe.YieldQuantity
			err = s.collectBaseIngredients(ctx, fullRecipe, nestedScale, collected, depth+1)
			if err != nil {
				return err
			}
		} else {
			// Base ingredient - aggregate by food+unit key
			key := fmt.Sprintf("%d|%s", ingredient.Food.ID, ingredient.Unit)

			if existing := collected[key]; existing != nil {
				existing.Quantity += scaledQty
			} else {
				collected[key] = &CollectedIngredient{
					FoodID:   ingredient.Food.ID,
					FoodName: ingredient.Food.Name,
					Unit:     ingredient.Unit,
					UnitType: ingredient.Food.UnitType,
					Quantity: scaledQty,
				}
			}
		}
	}
	return nil
}

func (s *ShoppingService) batchInsertIngredients(ctx context.Context, q *db.Queries, listId int32, sourceID int, collected map[string]*CollectedIngredient) error {
	if len(collected) == 0 {
		return nil
	}

	ingredients := make([]*CollectedIngredient, 0, len(collected))
	for _, ing := range collected {
		ingredients = append(ingredients, ing)
	}

	// Prepare arrays for finding existing items
	foodIds := make([]int32, len(ingredients))
	units := make([]string, len(ingredients))
	
	for i, ing := range ingredients {
		foodIds[i] = int32(ing.FoodID)
		units[i] = ing.Unit
	}

	// Find existing compatible items
	existingItems, err := q.BatchFindCompatibleItems(ctx, db.BatchFindCompatibleItemsParams{
		ShoppingListID: pgtype.Int4{Int32: int32(listId), Valid: true},
		Column2:        foodIds,
		Column3:          units,
	})
	if err != nil {
		return fmt.Errorf("failed to find existing items: %w", err)
	}

	// Build lookup for existing items
	existingLookup := make(map[string]int32)
	for _, item := range existingItems {
		key := fmt.Sprintf("%d|%s", item.FoodID.Int32, item.Unit)
		existingLookup[key] = item.ID
	}

	// Separate ingredients into existing vs new
	var newIngredients []*CollectedIngredient
	itemIds := make([]int32, len(ingredients))
	quantities := make([]pgtype.Numeric, len(ingredients))

	for i, ing := range ingredients {
		key := fmt.Sprintf("%d|%s", ing.FoodID, ing.Unit)
		quantities[i] = utils.Float64ToNumeric(ing.Quantity)
		
		if existingId, exists := existingLookup[key]; exists {
			// Use existing item
			itemIds[i] = existingId
		} else {
			// Mark for creation
			newIngredients = append(newIngredients, ing)
			itemIds[i] = -1 // Placeholder
		}
	}

	// Create new items if needed
	if len(newIngredients) > 0 {
		newFoodIds := make([]int32, len(newIngredients))
		newFoodNames := make([]string, len(newIngredients))
		newUnits := make([]string, len(newIngredients))
		newUnitTypes := make([]string, len(newIngredients))
		newNotes := make([]string, len(newIngredients))

		for i, ing := range newIngredients {
			newFoodIds[i] = int32(ing.FoodID)
			newFoodNames[i] = ing.FoodName
			newUnits[i] = ing.Unit
			newUnitTypes[i] = ing.UnitType
			newNotes[i] = ""
		}

		newItems, err := q.BatchCreateShoppingListItems(ctx, db.BatchCreateShoppingListItemsParams{
			ShoppingListID: listId,
			FoodIds:        newFoodIds,
			FoodNames:      newFoodNames,
			Units:          newUnits,
			UnitTypes:      newUnitTypes,
			Notes:          newNotes,
		})
		if err != nil {
			return fmt.Errorf("failed to create new items: %w", err)
		}

		// Map new items back to the main arrays
		newItemLookup := make(map[string]int32)
		for _, item := range newItems {
			key := fmt.Sprintf("%d|%s", item.FoodID.Int32, item.Unit)
			newItemLookup[key] = item.ID
		}

		// Fill in the -1 placeholders
		for i, ing := range ingredients {
			if itemIds[i] == -1 {
				key := fmt.Sprintf("%d|%s", ing.FoodID, ing.Unit)
				itemIds[i] = newItemLookup[key]
			}
		}
	}

	// Batch create all source links
	err = q.BatchCreateShoppingListItemSources(ctx, db.BatchCreateShoppingListItemSourcesParams{
		ItemIds:    itemIds,
		SourceID:   int32(sourceID),
		Quantities: quantities,
	})
	if err != nil {
		return fmt.Errorf("failed to create source links: %w", err)
	}

	log.Default().Printf("Successfully added %d ingredients (%d new, %d existing) for source %d", 
		len(ingredients), len(newIngredients), len(ingredients)-len(newIngredients), sourceID)
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
