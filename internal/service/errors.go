// Package service holds MealPlanner business logic. Handlers call services;
// services own transactions and talk to the sqlc query layer.
package service

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("the record was changed by someone else")
)

// ValidationError carries user-facing field problems.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string { return strings.Join(e.Problems, "; ") }

func invalid(msgs ...string) error { return &ValidationError{Problems: msgs} }

// Actor identifies who performs a household-scoped operation.
type Actor struct {
	AccountID   uuid.UUID
	HouseholdID uuid.UUID
	Role        string
}

func (a Actor) IsOwner() bool { return a.Role == "owner" }

func (a Actor) requireOwner() error {
	if !a.IsOwner() {
		return ErrForbidden
	}
	return nil
}

func (a Actor) ptr() *uuid.UUID { id := a.AccountID; return &id }
