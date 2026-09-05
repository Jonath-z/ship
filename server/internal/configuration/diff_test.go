package configuration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiffReportsAddedChangedRemovedAndUnchanged(t *testing.T) {
	before := DesiredState{
		Services: map[string]ServiceSpec{
			"api":    {Type: "web", Image: "acme/api:v1", Hosts: []string{"203.0.113.10"}},
			"legacy": {Type: "web", Image: "acme/legacy:v9"},
		},
		Accessories: map[string]Accessory{"postgres": {Type: "postgres", Image: "postgres:16"}},
		Roles:       map[string][]string{"web": {"203.0.113.10"}},
		Env:         map[string]string{"LOG_LEVEL": "info", "REGION": "eu"},
		SecretRefs:  []string{"DATABASE_URL", "OLD_TOKEN"},
	}
	after := DesiredState{
		Services: map[string]ServiceSpec{
			"api":    {Type: "web", Image: "acme/api:v2", Hosts: []string{"203.0.113.10", "203.0.113.11"}},
			"worker": {Type: "worker", Image: "acme/worker:v1"},
		},
		Accessories: map[string]Accessory{"postgres": {Type: "postgres", Image: "postgres:16"}},
		Roles:       map[string][]string{"web": {"203.0.113.10", "203.0.113.11"}},
		Env:         map[string]string{"LOG_LEVEL": "debug", "REGION": "eu"},
		SecretRefs:  []string{"DATABASE_URL", "REDIS_URL"},
	}

	entities := Diff(before, after)
	byKey := map[string]EntityDiff{}
	for _, entity := range entities {
		byKey[entity.Kind+"/"+entity.Name] = entity
	}
	expect := map[string]string{
		"service/api":         ChangeChanged,
		"service/legacy":      ChangeRemoved,
		"service/worker":      ChangeAdded,
		"accessory/postgres":  ChangeUnchanged,
		"role/web":            ChangeChanged,
		"env/LOG_LEVEL":       ChangeChanged,
		"env/REGION":          ChangeUnchanged,
		"secret/DATABASE_URL": ChangeUnchanged,
		"secret/OLD_TOKEN":    ChangeRemoved,
		"secret/REDIS_URL":    ChangeAdded,
	}
	for key, change := range expect {
		if byKey[key].Change != change {
			t.Errorf("%s: got %q, want %q", key, byKey[key].Change, change)
		}
	}
	if len(entities) != len(expect) {
		t.Errorf("entity count = %d, want %d: %#v", len(entities), len(expect), entities)
	}

	changedFields := map[string]bool{}
	for _, field := range byKey["service/api"].Fields {
		changedFields[field.Field] = true
	}
	if !changedFields["image"] || !changedFields["hosts"] || changedFields["type"] {
		t.Errorf("service/api fields = %#v", byKey["service/api"].Fields)
	}
}

// SH-054 acceptance: secret values never appear in a diff.
func TestDiffNeverContainsSecretValues(t *testing.T) {
	before := DesiredState{SecretRefs: []string{"DATABASE_URL"}}
	after := DesiredState{SecretRefs: []string{"DATABASE_URL", "API_KEY"}}
	encoded, err := json.Marshal(Diff(before, after))
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range Diff(before, after) {
		if entity.Kind == "secret" && len(entity.Fields) > 0 {
			t.Fatalf("secret diff carries fields: %#v", entity)
		}
	}
	if strings.Contains(string(encoded), "value") {
		t.Fatalf("secret diff output mentions values: %s", encoded)
	}
}
