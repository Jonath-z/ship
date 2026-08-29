package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("SHIP_API_ADDR", "")
	t.Setenv("SHIP_WORKER_ADDR", ":8081")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIAddr != ":8080" {
		t.Fatalf("expected the default API address, got %q", cfg.APIAddr)
	}
}

func TestLoadRejectsInvalidBoolean(t *testing.T) {
	t.Setenv("SHIP_RUN_MIGRATIONS", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid SHIP_RUN_MIGRATIONS to fail")
	}
}
