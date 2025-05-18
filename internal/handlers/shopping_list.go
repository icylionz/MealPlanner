package handlers

import (
	"log"
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
}

func NewShoppingListHandler(shoppingService *services.ShoppingService, scheduleService *services.ScheduleService) *ShoppingListHandler {
	return &ShoppingListHandler{
		shoppingService: shoppingService,
		scheduleService: scheduleService,
	}
}

func (h *ShoppingListHandler) HandleShoppingListsPage(c echo.Context) error {
	shoppingLists, err := h.shoppingService.GetShoppingLists(c.Request().Context())
	if err != nil {
		log.Printf("Error getting shopping lists: %v", err)
		return err
	}
	
	return pages.ShoppingListsPage(shoppingLists).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ShoppingListHandler) HandleShoppingListGenerateForm(c echo.Context) error {
	return components.GenerateShoppingListForm(&utils.ShoppingListFormProps{
		Errors: make(map[string]string),
	}).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ShoppingListHandler) HandleCreateShoppingList(c echo.Context) error {
	// Parse form data
	var form struct {
		Name      string `form:"name"`
		StartDate string `form:"start_date"`
		EndDate   string `form:"end_date"`
	}
	
	if err := c.Bind(&form); err != nil {
		return err
	}
	
	// Validate form
	errors := make(map[string]string)
	if form.Name == "" {
		errors["name"] = "Please enter a name for this shopping list"
	}
	
	startDate, err := time.Parse("2006-01-02", form.StartDate)
	if err != nil {
		errors["start_date"] = "Invalid start date format"
	}
	
	endDate, err := time.Parse("2006-01-02", form.EndDate)
	if err != nil {
		errors["end_date"] = "Invalid end date format"
	}
	
	if len(errors) > 0 {
		// Re-render form with errors
		return components.GenerateShoppingListForm(&utils.ShoppingListFormProps{
			Name:      form.Name,
			StartDate: startDate,
			EndDate:   endDate,
			Errors:    errors,
		}).Render(c.Request().Context(), c.Response().Writer)
	}
	
	if endDate.Before(startDate) {
		errors["end_date"] = "End date must be after start date"
		return components.GenerateShoppingListForm(&utils.ShoppingListFormProps{
			Name:      form.Name,
			StartDate: startDate,
			EndDate:   endDate,
			Errors:    errors,
		}).Render(c.Request().Context(), c.Response().Writer)
	}
	
	// Generate shopping list
	_, err = h.shoppingService.GenerateShoppingListFromDateRange(
		c.Request().Context(),
		form.Name,
		startDate,
		endDate,
	)
	
	if err != nil {
		log.Printf("Error generating shopping list: %v", err)
		return err
	}
	
	// Redirect to shopping lists page
	c.Response().Header().Set("HX-Redirect", "/shopping-lists")
	return c.NoContent(http.StatusOK)
}

func (h *ShoppingListHandler) HandleViewShoppingList(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid ID")
	}
	
	shoppingList, err := h.shoppingService.GetShoppingListById(c.Request().Context(), id)
	if err != nil {
		log.Printf("Error getting shopping list: %v", err)
		return err
	}
	
	return components.ShoppingListDetail(shoppingList).Render(c.Request().Context(), c.Response().Writer)
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

func (h *ShoppingListHandler) HandleRemoveMealFromShoppingList(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid shopping list ID")
	}
	
	mealId, err := strconv.Atoi(c.Param("mealId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid meal ID")
	}
	
	err = h.shoppingService.RemoveMealFromShoppingList(c.Request().Context(), listId, mealId)
	if err != nil {
		log.Printf("Error removing meal from shopping list: %v", err)
		return err
	}
	
	// Regenerate shopping list items
	err = h.shoppingService.RegenerateShoppingListItems(c.Request().Context(), listId)
	if err != nil {
		log.Printf("Error regenerating shopping list items: %v", err)
		return err
	}
	
	// Return updated shopping list
	shoppingList, err := h.shoppingService.GetShoppingListById(c.Request().Context(), listId)
	if err != nil {
		log.Printf("Error getting updated shopping list: %v", err)
		return err
	}
	
	return components.ShoppingListItems(shoppingList.Items).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ShoppingListHandler) HandleDeleteShoppingListItem(c echo.Context) error {
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid shopping list ID")
	}
	
	itemId, err := strconv.Atoi(c.Param("itemId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid item ID")
	}
	
	// This would need an additional query to be added
	err = h.shoppingService.DeleteShoppingListItem(c.Request().Context(), itemId)
	if err != nil {
		log.Printf("Error deleting shopping list item: %v", err)
		return err
	}
	
	// Return updated shopping list
	shoppingList, err := h.shoppingService.GetShoppingListById(c.Request().Context(), listId)
	if err != nil {
		log.Printf("Error getting updated shopping list: %v", err)
		return err
	}
	
	return components.ShoppingListItems(shoppingList.Items).Render(c.Request().Context(), c.Response().Writer)
}

func (h *ShoppingListHandler) HandleRecordPurchase(c echo.Context) error {
	var form struct {
		ActualQuantity string `form:"actual_quantity"`
		Price          string `form:"price"`
	}
	
	if err := c.Bind(&form); err != nil {
		return err
	}
	
	listId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid shopping list ID")
	}
	
	itemId, err := strconv.Atoi(c.Param("itemId"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid item ID")
	}
	
	actualQuantity, err := strconv.ParseFloat(form.ActualQuantity, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid quantity")
	}
	
	price, err := strconv.ParseFloat(form.Price, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid price")
	}
	
	err = h.shoppingService.RecordItemPurchase(c.Request().Context(), itemId, actualQuantity, price)
	if err != nil {
		log.Printf("Error recording purchase: %v", err)
		return err
	}
	
	// Return updated shopping list
	shoppingList, err := h.shoppingService.GetShoppingListById(c.Request().Context(), listId)
	if err != nil {
		log.Printf("Error getting updated shopping list: %v", err)
		return err
	}
	
	return components.ShoppingListItems(shoppingList.Items).Render(c.Request().Context(), c.Response().Writer)
}

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