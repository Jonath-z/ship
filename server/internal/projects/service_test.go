package projects

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCreateNormalizesNameAndDerivesSlug(t *testing.T) {
	name, slug, err := validateCreate(CreateInput{Name: "  Acme API  "})
	if err != nil {
		t.Fatal(err)
	}
	if name != "Acme API" || slug != "acme-api" {
		t.Fatalf("normalized project = %q/%q", name, slug)
	}
}

func TestValidateCreateRejectsInvalidFields(t *testing.T) {
	_, _, err := validateCreate(CreateInput{Name: strings.Repeat("é", 101), Slug: "Not Valid"})
	if err == nil || len(err.Fields) != 2 {
		t.Fatalf("validation error = %#v", err)
	}
}

func TestValidationErrorCanBeMatched(t *testing.T) {
	_, _, err := validateCreate(CreateInput{})
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
}
