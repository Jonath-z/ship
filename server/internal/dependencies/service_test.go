package dependencies

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateCreate(t *testing.T) {
	source := uuid.NewString()
	target := uuid.NewString()

	validated, validationError := validateCreate(CreateInput{SourceServiceID: source, TargetServiceID: &target})
	if validationError != nil || validated.Type != "runtime" {
		t.Fatalf("validated = %#v, error = %#v", validated, validationError)
	}

	if _, validationError := validateCreate(CreateInput{SourceServiceID: source}); validationError == nil {
		t.Fatal("expected missing target to be rejected")
	}
	if _, validationError := validateCreate(CreateInput{
		SourceServiceID: source, TargetServiceID: &target, TargetAccessoryID: &target,
	}); validationError == nil {
		t.Fatal("expected two targets to be rejected")
	}
	if _, validationError := validateCreate(CreateInput{SourceServiceID: "not-a-uuid", TargetServiceID: &target}); validationError == nil {
		t.Fatal("expected invalid source id to be rejected")
	}
	if _, validationError := validateCreate(CreateInput{
		SourceServiceID: source, TargetServiceID: &target, Type: "Not Valid!",
	}); validationError == nil {
		t.Fatal("expected invalid type to be rejected")
	}
}
