package identity

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewReturnsUUIDVersionFour(t *testing.T) {
	value, err := New()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		t.Fatalf("New() returned an invalid UUID: %v", err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("UUID version = %d, want 4", parsed.Version())
	}
}
