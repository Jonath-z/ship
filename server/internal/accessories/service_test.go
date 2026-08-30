package accessories

import "testing"

func TestValidateCreateDefaultsKnownPorts(t *testing.T) {
	for accessoryType, expected := range map[string]int{"postgres": 5432, "redis": 6379} {
		input, err := validateCreate(CreateInput{Name: accessoryType, Type: accessoryType, Image: accessoryType + ":latest"})
		if err != nil {
			t.Fatal(err)
		}
		if input.Port == nil || *input.Port != expected {
			t.Fatalf("%s port = %#v, want %d", accessoryType, input.Port, expected)
		}
	}
}

func TestValidateCreateRejectsUnknownTypeAndTwoPlacements(t *testing.T) {
	serverID := "f8898fcb-6412-478f-bca3-ea474525ca34"
	groupID := "7f810ee9-45d5-434e-a8f4-2925b0761e65"
	_, err := validateCreate(CreateInput{
		Name: "Database", Type: "mysql", Image: "mysql:latest",
		ServerID: &serverID, ServerGroupID: &groupID,
	})
	if err == nil || len(err.Fields) != 2 {
		t.Fatalf("validation error = %#v", err)
	}
}

func TestIdentifierCreatesDockerVolumeName(t *testing.T) {
	if value := identifier(" Primary PostgreSQL "); value != "primary_postgresql" {
		t.Fatalf("identifier = %q", value)
	}
}
