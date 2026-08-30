package environments

import (
	"strings"
	"testing"
)

func TestValidateCreateNormalizesNameAndDerivesSlug(t *testing.T) {
	name, slug, err := validateCreate(CreateInput{Name: "  Production Europe  "})
	if err != nil {
		t.Fatal(err)
	}
	if name != "Production Europe" || slug != "production-europe" {
		t.Fatalf("normalized environment = %q/%q", name, slug)
	}
}

func TestValidateCreateRejectsInvalidFields(t *testing.T) {
	_, _, err := validateCreate(CreateInput{Name: strings.Repeat("界", 101), Slug: "production_eu"})
	if err == nil || len(err.Fields) != 2 {
		t.Fatalf("validation error = %#v", err)
	}
}
