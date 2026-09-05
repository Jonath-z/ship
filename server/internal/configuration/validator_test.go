package configuration

import "testing"

func violationCodes(violations []Violation) map[string]int {
	codes := map[string]int{}
	for _, violation := range violations {
		codes[violation.Code]++
	}
	return codes
}

func TestValidateAcceptsHealthyState(t *testing.T) {
	for _, violation := range Validate(fixtureState(), Facts{}) {
		if violation.Severity == SeverityBlock {
			t.Fatalf("unexpected blocking violation: %#v", violation)
		}
	}
}

func TestValidateRules(t *testing.T) {
	state := DesiredState{
		Services: map[string]ServiceSpec{
			"api": { // unplaced, has a domain but no port
				Domains:   []Domain{{Hostname: "api.example.com"}},
				DependsOn: []string{"service:worker"},
			},
			"worker": { // completes the cycle, port conflict with postgres
				Hosts: []string{"203.0.113.12"}, Port: 5432,
				DependsOn: []string{"service:api"},
			},
			"admin": { // SSL warning with several hosts
				Hosts: []string{"203.0.113.10", "203.0.113.11"}, Port: 8080,
				Domains: []Domain{{Hostname: "admin.example.com", SSLEnabled: true}},
			},
		},
		Accessories: map[string]Accessory{
			"postgres": {Type: "postgres", Image: "postgres:16", Hosts: []string{"203.0.113.12"}, Port: 5432},
			"redis":    {Type: "redis", Image: "redis:7"}, // unplaced
		},
	}
	facts := Facts{
		HostStatus:          map[string]string{"203.0.113.12": "disconnected"},
		SecretsWithoutValue: []string{"DATABASE_URL"},
	}
	codes := violationCodes(Validate(state, facts))
	expected := map[string]int{
		"service_unplaced":             1, // api
		"domain_service_port_missing":  1, // api
		"dependency_cycle":             2, // api and worker
		"port_conflict":                1, // worker vs postgres on .12:5432
		"ssl_multi_host":               1, // admin domain
		"accessory_unplaced":           1, // redis
		"accessory_server_unreachable": 1,
		"secret_missing_value":         1,
	}
	for code, count := range expected {
		if codes[code] != count {
			t.Errorf("code %s: got %d, want %d (all: %#v)", code, codes[code], count, codes)
		}
	}
	if len(codes) != len(expected) {
		t.Errorf("unexpected extra codes: %#v", codes)
	}
}

func TestValidateDeterministicOrder(t *testing.T) {
	state := DesiredState{Services: map[string]ServiceSpec{"b": {}, "a": {}, "c": {}}}
	first := Validate(state, Facts{})
	second := Validate(state, Facts{})
	if len(first) != 3 || first[0].EntityName != "a" || first[2].EntityName != "c" {
		t.Fatalf("violations = %#v", first)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatal("validation order is not deterministic")
		}
	}
}
