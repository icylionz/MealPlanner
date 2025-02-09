package utils

import "errors"

var (
    ErrFoodNotFound = errors.New("food not found")
    ErrRecipeNotFound = errors.New("recipe not found")
    ErrInvalidUnit = errors.New("invalid unit")
    ErrCircularDependency = errors.New("circular recipe dependency detected")
)