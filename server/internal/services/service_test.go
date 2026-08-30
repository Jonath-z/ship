package services

import (
	"testing"

	"github.com/Jonath-z/ship/server/migrations"
)

func TestValidateCreateDefaultsRepositoryBranchAndRole(t *testing.T) {
	input, err := validateCreate(CreateInput{
		Name: " API ", Type: "web", Repository: " https://example.com/acme/api.git ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "API" || input.Branch != "main" || input.Role != "web" {
		t.Fatalf("normalized input = %#v", input)
	}
}

func TestValidateCreateRequiresRepositoryOrImage(t *testing.T) {
	_, err := validateCreate(CreateInput{Name: "API", Type: "web"})
	if err == nil {
		t.Fatal("expected missing source validation error")
	}
}

func TestApplyUpdateRejectsRemovingLastSource(t *testing.T) {
	empty := ""
	_, violations := applyUpdate(migrations.Service{
		Name: "API", Type: "web", Repository: "https://example.com/acme/api.git", Branch: "main",
	}, UpdateInput{Repository: &empty})
	if len(violations) == 0 {
		t.Fatal("expected update to reject service without repository or image")
	}
}
