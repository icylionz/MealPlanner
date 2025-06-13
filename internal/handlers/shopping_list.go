package handlers

import (
	"log"
	"mealplanner/internal/models"
	"mealplanner/internal/services"
	"mealplanner/internal/utils"
	"mealplanner/internal/views/components"
	"mealplanner/internal/views/pages"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

type ShoppingListHandler struct {
	shoppingService *services.ShoppingService
	scheduleService *services.ScheduleService
	foodService     *services.FoodService
}

func NewShoppingListHandler(
	shoppingService *services.ShoppingService,
	scheduleService *services.ScheduleService,
	foodService *services.FoodService,
) *ShoppingListHandler {
	return &ShoppingListHandler{
		shoppingService: shoppingService,
		scheduleService: scheduleService,
		foodService:     foodService,
	}
}

// Page handlers
func (h *ShoppingListHandler) HandleShoppingListsPage(c echo.Context) error {
	lists, err := h.shoppingService.GetShoppingLists(c.Request().Context())
	if err != nil {
		log.Printf("Error getting shopping lists: %v", err)
		return err
	}

	return pages.ShoppingListsPage(lists).Render(c.Request().Context(), c.Response().Writer)
}
func (h *ShoppingListHandler) HandleViewShoppingList(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid ID")
	}

	list, err := h.shoppingService.GetShoppingListById(c.Request().Context(), id)
	if err != nil {
		log.Printf("Error getting shopping list: %v", err)
		return err
	}

	// Check if this is an HTMX request
	if c.Request().Header.Get("HX-Request") != "" {
		// Return partial for HTMX
		return components.ShoppingListDetail(list).Render(c.Request().Context(), c.Response().Writer)
	}

	// Return full page for direct navigation
	return pages.ShoppingListDetailPage(list).Render(c.Request().Context(), c.Response().Writer)
}

// Shopping list CRUD
func (h *ShoppingListHandler) HandleCreateShoppingListModal(c echo.Context) error {
	if c.Request().Method == "POST" {
		return h.handleCreateShoppingList(c)
	}

	// GET - show creation modal
	props := &utils.ShoppingListFormProps{
		Errors: make(map[string]string),
	}
	return components.CreateShoppingListModal(props).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ShoppingListHandler) handleCreateShoppingList(c echo.Context) error {
	var form struct {
		Name  string `form:"name"`
		Notes string `form:"notes"`
	}

	if err := c.Bind(&form); err != nil {
		return err
	}

	// Validate
	errors := make(map[string]string)
	if form.Name == "" {
		errors["name"] = "Name is required"
	}

	if len(errors) > 0 {
		props := &utils.ShoppingListFormProps{
			Name:   form.Name,
			Notes:  form.Notes,
			Errors: errors,
		}
		c.Response().WriteHeader(http.StatusBadRequest)
		return components.CreateShoppingListModal(props).Render(c.Request().Context(), c.Response().Writer)
	}

	// Create list
	_, err := h.shoppingService.CreateShoppingList(c.Request().Context(), form.Name, form.Notes)
	if err != nil {
		log.Printf("Error creating shopping list: %v", err)
		return err
	}

	return c.NoContent(http.StatusOK)
}

func (h *ShoppingListHandler) HandleDeleteShoppingList(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid ID")
	}

	err = h.shoppingService.DeleteShoppingList(c.Request().Context(), id)
	if err != nil {
		log.Printf("Error deleting shopping list: %v", err)
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

// Add items modals and handlers
func (h *ShoppingListHandler) HandleAddItemsModal(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid ID")
	}

	// Get available foods for manual addition
	foods, err := h.foodService.GetFoods(c.Request().Context(), "")
	if err != nil {
		return err
	}

	// Get recent schedules for schedule addition
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	weekFromNow := now.AddDate(0, 0, 7)
	timeZone := utils.GetTimezone(c)
	schedules, err := h.scheduleService.GetSchedulesForRange(c.Request().Context(), &weekAgo, &weekFromNow, timeZone)
	if err != nil {
		log.Printf("Error getting schedules: %v", err)
		schedules = []*models.Schedule{} // Continue with empty schedules
	}

	props := &utils.AddItemsModalProps{
		ListID:    listId,
		Foods:     foods,
		Schedules: schedules,
		Errors:    make(map[string]string),
	}

	return components.AddItemsModal(props).Render(c.Request().Context(), c.Response().Writer)
}

// Manual item addition
func (h *ShoppingListHandler) HandleAddManualItem(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	var form struct {
		FoodID   string  `form:"food_id"`
		Quantity float64 `form:"quantity"`
		Unit     string  `form:"unit"`
		Notes    string  `form:"notes"`
	}

	if err := c.Bind(&form); err != nil {
		return err
	}

	// Validate
	errors := make(map[string]string)
	if form.FoodID == "" {
		errors["food_id"] = "Please select a food"
	}
	if form.Quantity <= 0 {
		errors["quantity"] = "Quantity must be greater than 0"
	}
	if form.Unit == "" {
		errors["unit"] = "Unit is required"
	}

	if len(errors) > 0 {
		return h.returnAddItemsModalWithErrors(c, listId, errors)
	}

	foodId, err := strconv.Atoi(form.FoodID)
	if err != nil {
		errors["food_id"] = "Invalid food selection"
		return h.returnAddItemsModalWithErrors(c, listId, errors)
	}

	// Add item
	req := &models.AddManualItemRequest{
		FoodID:   foodId,
		Quantity: form.Quantity,
		Unit:     form.Unit,
		Notes:    form.Notes,
	}

	err = h.shoppingService.AddManualItem(c.Request().Context(), listId, req)
	if err != nil {
		log.Printf("Error adding manual item: %v", err)
		return err
	}
	
	c.Response().Header().Set("HX-Trigger", "refreshShoppingList,closeModal")
	return c.NoContent(http.StatusOK)
}

// Recipe addition
func (h *ShoppingListHandler) HandleAddRecipe(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	var form struct {
		RecipeID string  `form:"recipe_id"`
		Servings float64 `form:"servings"`
	}

	if err := c.Bind(&form); err != nil {
		return err
	}

	// Validate
	errors := make(map[string]string)
	if form.RecipeID == "" {
		errors["recipe_id"] = "Please select a recipe"
	}
	if form.Servings <= 0 {
		errors["servings"] = "Servings must be greater than 0"
	}

	if len(errors) > 0 {
		return h.returnAddItemsModalWithErrors(c, listId, errors)
	}

	recipeId, err := strconv.Atoi(form.RecipeID)
	if err != nil {
		errors["recipe_id"] = "Invalid recipe selection"
		return h.returnAddItemsModalWithErrors(c, listId, errors)
	}

	// Add recipe
	req := &models.AddRecipeRequest{
		RecipeID: recipeId,
		Servings: form.Servings,
	}

	err = h.shoppingService.AddRecipe(c.Request().Context(), listId, req)
	if err != nil {
		log.Printf("Error adding recipe: %v", err)
		return err
	}


	c.Response().Header().Set("HX-Trigger", "refreshShoppingList,closeModal")
	return c.NoContent(http.StatusOK)

}

// Schedule addition
func (h *ShoppingListHandler) HandleAddSchedules(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	err = c.Request().ParseForm()
	if err != nil {
		log.Default().Printf("Error parsing form: %s", err)
		return err
	}

	// Parse schedule IDs from form
	scheduleIDStrs := c.Request().Form["schedule_ids"]
	log.Default().Printf("Schedule IDs: %v", scheduleIDStrs)
	if len(scheduleIDStrs) == 0 {
		errors := map[string]string{"schedule_ids": "Please select at least one meal"}
		return h.returnAddItemsModalWithErrors(c, listId, errors)
	}

	scheduleIDs := make([]int, 0, len(scheduleIDStrs))
	for _, idStr := range scheduleIDStrs {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			errors := map[string]string{"schedule_ids": "Invalid meal selection"}
			return h.returnAddItemsModalWithErrors(c, listId, errors)
		}
		scheduleIDs = append(scheduleIDs, id)
	}

	// Add schedules
	req := &models.AddSchedulesRequest{
		ScheduleIDs: scheduleIDs,
	}
	timeZone := utils.GetTimezone(c)
	err = h.shoppingService.AddSchedules(c.Request().Context(), listId, req, timeZone)
	if err != nil {
		log.Printf("Error adding schedules: %v", err)
		return err
	}


	c.Response().Header().Set("HX-Trigger", "refreshShoppingList,closeModal")
	return c.NoContent(http.StatusOK)

}

// Date range addition
func (h *ShoppingListHandler) HandleAddDateRange(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	var form struct {
		StartDate string `form:"start_date"`
		EndDate   string `form:"end_date"`
	}

	if err := c.Bind(&form); err != nil {
		return err
	}

	// Validate and parse dates
	errors := make(map[string]string)

	startDate, err := time.Parse("2006-01-02", form.StartDate)
	if err != nil {
		errors["start_date"] = "Invalid start date"
	}

	endDate, err := time.Parse("2006-01-02", form.EndDate)
	if err != nil {
		errors["end_date"] = "Invalid end date"
	}

	if len(errors) == 0 && endDate.Before(startDate) {
		errors["end_date"] = "End date must be after start date"
	}

	if len(errors) > 0 {
		return h.returnAddItemsModalWithErrors(c, listId, errors)
	}

	// Add date range
	req := &models.AddDateRangeRequest{
		StartDate: startDate,
		EndDate:   endDate,
	}

	timeZone := utils.GetTimezone(c)
	err = h.shoppingService.AddDateRange(c.Request().Context(), listId, req, timeZone)
	if err != nil {
		log.Printf("Error adding date range: %v", err)
		return err
	}


	c.Response().Header().Set("HX-Trigger", "refreshShoppingList,closeModal")
	return c.NoContent(http.StatusOK)

}

// Item management
func (h *ShoppingListHandler) HandleUpdateItem(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	itemId, err := strconv.Atoi(c.Param("itemId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid item ID")
	}

	var form struct {
		Quantity float64 `form:"quantity"`
		Notes    string  `form:"notes"`
	}

	if err := c.Bind(&form); err != nil {
		return err
	}

	// Update quantity if provided
	if form.Quantity > 0 {
		err = h.shoppingService.UpdateItemQuantity(c.Request().Context(), itemId, form.Quantity)
		if err != nil {
			log.Printf("Error updating item quantity: %v", err)
			return err
		}
	}

	// Update notes
	err = h.shoppingService.UpdateItemNotes(c.Request().Context(), itemId, form.Notes)
	if err != nil {
		log.Printf("Error updating item notes: %v", err)
		return err
	}

	// Return updated list items
	return h.returnUpdatedItems(c, listId)
}

func (h *ShoppingListHandler) HandleMarkItemPurchased(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	itemId, err := strconv.Atoi(c.Param("itemId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid item ID")
	}

	var form struct {
		Purchased      bool    `form:"purchased"`
		ActualQuantity float64 `form:"actual_quantity"`
		ActualPrice    float64 `form:"actual_price"`
	}

	if err := c.Bind(&form); err != nil {
		return err
	}

	err = h.shoppingService.MarkItemPurchased(c.Request().Context(), itemId, form.Purchased, form.ActualQuantity, form.ActualPrice)
	if err != nil {
		log.Printf("Error marking item purchased: %v", err)
		return err
	}

	return h.returnUpdatedItems(c, listId)
}

func (h *ShoppingListHandler) HandleDeleteItem(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	itemId, err := strconv.Atoi(c.Param("itemId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid item ID")
	}

	err = h.shoppingService.RemoveItem(c.Request().Context(), itemId)
	if err != nil {
		log.Printf("Error deleting item: %v", err)
		return err
	}

	return h.returnUpdatedItems(c, listId)
}

func (h *ShoppingListHandler) HandleDeleteItemsBySource(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid list ID")
	}

	sourceId, err := strconv.Atoi(c.Param("sourceId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid source ID")
	}

	err = h.shoppingService.RemoveItemsBySource(c.Request().Context(), sourceId)
	if err != nil {
		log.Printf("Error deleting items by source: %v", err)
		return err
	}

	// Return the full shopping list detail
	list, err := h.shoppingService.GetShoppingListById(c.Request().Context(), listId)
	if err != nil {
		return err
	}

	return components.ShoppingListDetail(list).Render(c.Request().Context(), c.Response().Writer)
}

// Export
func (h *ShoppingListHandler) HandleExportShoppingList(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid ID")
	}

	text, err := h.shoppingService.ExportShoppingListAsText(c.Request().Context(), id)
	if err != nil {
		log.Printf("Error exporting shopping list: %v", err)
		return err
	}

	c.Response().Header().Set("Content-Disposition", "attachment; filename=shopping-list.txt")
	c.Response().Header().Set("Content-Type", "text/plain")
	return c.String(http.StatusOK, text)
}

// Helper methods
func (h *ShoppingListHandler) returnAddItemsModalWithErrors(c echo.Context, listId int, errors map[string]string) error {
	// Re-fetch data for modal
	foods, err := h.foodService.GetFoods(c.Request().Context(), "")
	if err != nil {
		return err
	}

	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	weekFromNow := now.AddDate(0, 0, 7)
	timeZone := utils.GetTimezone(c)
	schedules, err := h.scheduleService.GetSchedulesForRange(c.Request().Context(), &weekAgo, &weekFromNow, timeZone)
	if err != nil {
		schedules = []*models.Schedule{}
	}

	props := &utils.AddItemsModalProps{
		ListID:    listId,
		Foods:     foods,
		Schedules: schedules,
		Errors:    errors,
	}

	c.Response().WriteHeader(http.StatusBadRequest)
	return components.AddItemsModal(props).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ShoppingListHandler) returnUpdatedItems(c echo.Context, listId int) error {
	list, err := h.shoppingService.GetShoppingListById(c.Request().Context(), listId)
	if err != nil {
		return err
	}

	return components.ShoppingListItems(list.Items).Render(c.Request().Context(), c.Response().Writer)
}
