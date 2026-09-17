package configuration

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var goldenInput = RenderInput{ProjectSlug: "acme", EnvironmentSlug: "production"}

// Golden scenarios per the SH-055 acceptance criteria. Regenerate with:
//
//	UPDATE_GOLDEN=1 go test ./server/internal/configuration -run TestRenderGolden
func goldenScenarios() map[string]DesiredState {
	return map[string]DesiredState{
		"single-server": {
			EnvironmentID: "env-1",
			Services: map[string]ServiceSpec{
				"api": {
					Type: "web", Image: "acme/api:v1", Port: 3000, Role: "web",
					Hosts:   []string{"203.0.113.10"},
					Arch:    []string{"arm64"},
					Domains: []Domain{{Hostname: "api.example.com", SSLEnabled: true}},
					Env:     map[string]string{"RAILS_ENV": "production"},
				},
			},
			Accessories: map[string]Accessory{},
			Roles:       map[string][]string{"web": {"203.0.113.10"}},
			SSH:         SSHSpec{User: "root"},
		},
		"multi-server-role": {
			EnvironmentID: "env-2",
			Services: map[string]ServiceSpec{
				"api": {
					Type: "web", Image: "acme/api:v1", Port: 3000, Role: "web", Command: "bin/server",
					Hosts:   []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"},
					Arch:    []string{"amd64", "arm64"},
					Domains: []Domain{{Hostname: "api.example.com", SSLEnabled: false}, {Hostname: "www.example.com", SSLEnabled: false}},
				},
				"worker": {
					Type: "worker", Repository: "acme/api", Branch: "main", Role: "jobs",
					Hosts: []string{"203.0.113.13"},
				},
			},
			Accessories: map[string]Accessory{},
			Roles: map[string][]string{
				"web":  {"203.0.113.10", "203.0.113.11", "203.0.113.12"},
				"jobs": {"203.0.113.13"},
			},
			Env: map[string]string{"LOG_LEVEL": "info"},
			SSH: SSHSpec{User: "deploy", Port: 2222},
		},
		"registry-auth": {
			EnvironmentID: "env-5",
			Services: map[string]ServiceSpec{
				"api": {
					Type: "web", Image: "ghcr.io/acme/api:v1", Port: 3000, Role: "web",
					Hosts:      []string{"203.0.113.10"},
					SecretRefs: []string{"APP_KEY"},
				},
			},
			Accessories: map[string]Accessory{},
			Roles:       map[string][]string{"web": {"203.0.113.10"}},
			Env: map[string]string{
				RegistryServerVar: "ghcr.io",
				"LOG_LEVEL":       "info",
			},
			SecretRefs: []string{RegistryPasswordKey, RegistryUsernameKey},
			SSH:        SSHSpec{User: "root"},
		},
		"accessory-heavy": {
			EnvironmentID: "env-3",
			Services: map[string]ServiceSpec{
				"api": {
					Type: "web", Image: "acme/api:v1", Port: 3000, Role: "web",
					Hosts:      []string{"203.0.113.10"},
					SecretRefs: []string{"DATABASE_URL", "REDIS_URL"},
					DependsOn:  []string{"accessory:postgres", "accessory:redis"},
					Volumes:    []Volume{{Name: "uploads", Source: "api_uploads", Destination: "/data/uploads"}},
				},
				"metrics": {Type: "web", Image: "acme/metrics:v1", Role: "web", Hosts: []string{"203.0.113.10"}},
			},
			Accessories: map[string]Accessory{
				"postgres": {
					Type: "postgres", Image: "postgres:16", Hosts: []string{"203.0.113.20"}, Port: 5432,
					Volumes: []Volume{{Name: "data", Source: "postgres_data", Destination: "/var/lib/postgresql/data"}},
				},
				"redis": {Type: "redis", Image: "redis:7", Hosts: []string{"203.0.113.20"}, Port: 6379},
			},
			Roles: map[string][]string{"web": {"203.0.113.10"}},
			SSH:   SSHSpec{User: "root"},
		},
	}
}

func TestRenderGolden(t *testing.T) {
	for scenario, state := range goldenScenarios() {
		t.Run(scenario, func(t *testing.T) {
			rendered, err := Render(goldenInput, state)
			if err != nil {
				t.Fatal(err)
			}
			for serviceName, document := range rendered {
				goldenPath := filepath.Join("testdata", scenario+"-"+serviceName+".yml")
				if os.Getenv("UPDATE_GOLDEN") == "1" {
					if err := os.MkdirAll("testdata", 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(goldenPath, document, 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				expected, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatalf("missing golden file %s (run with UPDATE_GOLDEN=1): %v", goldenPath, err)
				}
				if !bytes.Equal(expected, document) {
					t.Errorf("%s drifted from golden:\n--- want\n%s\n--- got\n%s", goldenPath, expected, document)
				}
			}
		})
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	for scenario, state := range goldenScenarios() {
		first, err := Render(goldenInput, state)
		if err != nil {
			t.Fatal(err)
		}
		second, err := Render(goldenInput, state)
		if err != nil {
			t.Fatal(err)
		}
		for name := range first {
			if !bytes.Equal(first[name], second[name]) {
				t.Fatalf("scenario %s service %s rendered differently on repeat", scenario, name)
			}
		}
	}
}

// Kamal 2 rejects configs without builder.arch, so every render must emit it:
// the recorded host architecture, a list when a role mixes fleets, and the
// default when checks never captured one.
func TestRenderBuilderArch(t *testing.T) {
	cases := map[string]struct {
		arch []string
		want any
	}{
		"recorded": {arch: []string{"arm64"}, want: "arm64"},
		"mixed":    {arch: []string{"amd64", "arm64"}, want: []string{"amd64", "arm64"}},
		"unknown":  {arch: nil, want: DefaultArch},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			builder := renderBuilder(ServiceSpec{Arch: testCase.arch})
			if want, ok := testCase.want.(string); ok {
				if builder.Arch != want {
					t.Fatalf("arch = %v, want %q", builder.Arch, want)
				}
				return
			}
			got, ok := builder.Arch.([]string)
			if !ok || !slices.Equal(got, testCase.want.([]string)) {
				t.Fatalf("arch = %v, want %v", builder.Arch, testCase.want)
			}
		})
	}
}

// Secrets render as names under env.secret; values must never appear.
func TestRenderListsSecretsByNameOnly(t *testing.T) {
	rendered, err := Render(goldenInput, goldenScenarios()["accessory-heavy"])
	if err != nil {
		t.Fatal(err)
	}
	document := string(rendered["api"])
	if !strings.Contains(document, "DATABASE_URL") || !strings.Contains(document, "secret:") {
		t.Fatalf("expected secret names in env.secret:\n%s", document)
	}
}
