package handlers

import (
	"mealplanner/internal/views/pages"

	"github.com/labstack/echo/v4"
)

type PageHandler struct{}

func NewPageHandler() *PageHandler {
	return &PageHandler{}
}

func (h *PageHandler) HandleIndex(c echo.Context) error {
	component := pages.Index()
	return component.Render(c.Request().Context(), c.Response().Writer)
}
