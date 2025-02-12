package handlers

import (
	"mealplanner/internal/services"
	"mealplanner/internal/views/components"

	"github.com/labstack/echo/v4"
)

type FoodHandler struct {
	service *services.FoodService
}

func (h *FoodHandler) HandleFoodsPage(c echo.Context) error {
	return components.FoodsPage().Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleViewFoodDetailsModal(c echo.Context) error {
	id := c.QueryParam("id")
	food, err := h.service.GetFoodDetails(c.Request().Context(), id)
	if err != nil {
		return c.String(500, "Error getting food")
	}
	return components.ViewFoodDetailsModal(food).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleSearchFoods(c echo.Context) error {
	// Get query parameters
	query := c.QueryParam("search")


	// Get foods from service
	foods, err := h.service.GetFoods(c.Request().Context(), query)
	if err != nil {
		return c.String(500, "Error searching foods")
	}

	// Render only the food list component
	return components.FoodList(foods).Render(c.Request().Context(), c.Response().Writer)
}

func (h *FoodHandler) HandleDeleteFood(c echo.Context) error {
	id := c.Param("id")
	err := h.service.DeleteFood(c.Request().Context(), id)
	if err != nil {
		return c.String(500, "Error deleting food")
	}
	return c.NoContent(204)
}

func NewFoodHandler(service *services.FoodService) *FoodHandler {
	return &FoodHandler{
		service: service,
	}
}
