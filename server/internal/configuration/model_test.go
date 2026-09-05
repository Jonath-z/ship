package configuration

import (
	"encoding/json"
	"reflect"
	"testing"
)

// fixtureState exercises every field in the model.
func fixtureState() DesiredState {
	return DesiredState{
		EnvironmentID: "5e5cc9d6-8a4e-4bb1-8b62-6d80536df6e7",
		Services: map[string]ServiceSpec{
			"api": {
				Type: "web", Repository: "acme/api", Branch: "main", Port: 3000,
				Command: "bin/server", Role: "web", Hosts: []string{"203.0.113.10", "203.0.113.11"},
				Domains:    []Domain{{Hostname: "api.example.com", SSLEnabled: true}},
				Volumes:    []Volume{{Name: "uploads", Source: "api_uploads", Destination: "/data/uploads"}},
				Env:        map[string]string{"RAILS_ENV": "production"},
				SecretRefs: []string{"API_KEY"},
				DependsOn:  []string{"accessory:postgres", "service:worker"},
			},
			"worker": {Type: "worker", Image: "acme/worker:v3", Role: "jobs", Hosts: []string{"203.0.113.12"}},
		},
		Accessories: map[string]Accessory{
			"postgres": {
				Type: "postgres", Image: "postgres:16", Hosts: []string{"203.0.113.12"}, Port: 5432,
				Volumes: []Volume{{Name: "data", Source: "postgres_data", Destination: "/var/lib/postgresql/data"}},
			},
		},
		Roles: map[string][]string{
			"web":  {"203.0.113.10", "203.0.113.11"},
			"jobs": {"203.0.113.12"},
		},
		Env:        map[string]string{"LOG_LEVEL": "info"},
		SecretRefs: []string{"DATABASE_URL"},
	}
}

// SH-050 acceptance: a full environment round-trips model -> JSON -> model
// with no loss.
func TestDesiredStateRoundTripsThroughJSON(t *testing.T) {
	original := fixtureState()
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DesiredState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, decoded) {
		t.Fatalf("round trip lost data:\noriginal: %#v\ndecoded:  %#v", original, decoded)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(reencoded) {
		t.Fatal("marshalling the same state twice produced different bytes")
	}
}
