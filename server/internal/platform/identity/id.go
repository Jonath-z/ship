// Package identity generates opaque identifiers for persisted Ship records.
package identity

import (
	"fmt"

	"github.com/google/uuid"
)

// New returns a cryptographically random UUID v4.
func New() (string, error) {
	value, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}
	return value.String(), nil
}
