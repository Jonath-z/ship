package registry

import "testing"

func TestRegistryKind(t *testing.T) {
	cases := map[string]kind{
		"":                          kindDockerHub,
		"docker.io":                 kindDockerHub,
		"index.docker.io":           kindDockerHub,
		"https://hub.docker.com":    kindDockerHub,
		"ghcr.io":                   kindGHCR,
		"https://ghcr.io":           kindGHCR,
		"registry.example.com":      kindV2,
		"registry.example.com:5000": kindV2,
		"https://harbor.internal/":  kindV2,
	}
	for server, expected := range cases {
		if actual := registryKind(server); actual != expected {
			t.Errorf("registryKind(%q) = %d, want %d", server, actual, expected)
		}
	}
}

func TestParseBearerChallenge(t *testing.T) {
	params := parseBearerChallenge(
		`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:app:pull"`,
	)
	if params["realm"] != "https://auth.example.com/token" {
		t.Errorf("realm = %q", params["realm"])
	}
	if params["service"] != "registry.example.com" {
		t.Errorf("service = %q", params["service"])
	}
	if len(parseBearerChallenge("Basic realm=x")) != 0 {
		t.Error("non-Bearer challenge should yield no params")
	}
}
