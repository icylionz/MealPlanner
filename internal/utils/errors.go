package utils

import "errors"

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
