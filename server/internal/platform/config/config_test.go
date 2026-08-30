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

func TestProductionRequiresHTTPSUnlessBootstrapIsExplicit(t *testing.T) {
	t.Setenv("SHIP_ENV", "production")
	t.Setenv("SHIP_PUBLIC_URL", "http://ship.example.com:3000")
	t.Setenv("SHIP_ALLOW_INSECURE_HTTP", "false")
	t.Setenv("SHIP_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Fatal("expected production HTTP to be rejected")
	}

	t.Setenv("SHIP_ALLOW_INSECURE_HTTP", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SecureCookies() {
		t.Fatal("HTTP bootstrap must not claim to use secure cookies")
	}
}

func TestHTTPSPublicURLEnablesSecureCookies(t *testing.T) {
	t.Setenv("SHIP_ENV", "production")
	t.Setenv("SHIP_PUBLIC_URL", "https://ship.example.com")
	t.Setenv("SHIP_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SecureCookies() || cfg.PublicOrigin != "https://ship.example.com" {
		t.Fatalf("unexpected HTTPS configuration: %#v", cfg)
	}
}
