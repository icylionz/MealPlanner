package utils

import (
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

var (
	ErrFoodNotFound       = errors.New("food not found")
	ErrRecipeNotFound     = errors.New("recipe not found")
	ErrInvalidUnit        = errors.New("invalid unit")
	ErrCircularDependency = errors.New("circular recipe dependency detected")
)

type ValidationError struct {
	OffendingFields map[string]string
}

func (ve *ValidationError) Error() string {
	return "Validation failed"
}

// Returns the field errors map, matches what the template expects
func (ve *ValidationError) Fields() map[string]string {
	return ve.OffendingFields
}

// Helper to create new validation errors
func NewValidationError() *ValidationError {
	return &ValidationError{
		OffendingFields: make(map[string]string),
	}
}

// Helper to add an error for a field
func (ve *ValidationError) Add(field, message string) {
	ve.OffendingFields[field] = message
}

func CustomErrorHandler(err error, c echo.Context) {
	// Log the error with context
	log.Printf("Error on %s %s: %v", c.Request().Method, c.Request().URL.Path, err)

	// Handle different error types
	if he, ok := err.(*echo.HTTPError); ok {
		// Echo HTTP errors (like 404, 400, etc.)
		if he.Internal != nil {
			log.Printf("Internal error: %v", he.Internal)
		}
		c.JSON(he.Code, map[string]interface{}{
			"error": he.Message,
		})
		return
	}

	// Handle validation errors (preserve existing format)
	if ve, ok := err.(*ValidationError); ok {
		c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error":  "Validation failed",
			"fields": ve.Fields(),
		})
		return
	}

	// Handle all other errors as 500
	c.JSON(http.StatusInternalServerError, map[string]interface{}{
		"error": "Internal server error",
	})
}
