package auth

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/platform/config"
)

func TestMutationRequiresMatchingOriginAndCSRFToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &Service{config: config.Config{PublicOrigin: "https://ship.example.com"}}
	tests := []struct {
		name   string
		origin string
		token  string
		valid  bool
	}{
		{"matching", "https://ship.example.com", "correct-token", true},
		{"cross origin", "https://attacker.example", "correct-token", false},
		{"missing origin", "", "correct-token", false},
		{"wrong token", "https://ship.example.com", "wrong-token", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "https://ship.example.com/auth/password", nil)
			c.Request.Header.Set("Origin", test.origin)
			c.Request.Header.Set("X-CSRF-Token", test.token)
			if got := service.validMutation(c, "correct-token"); got != test.valid {
				t.Fatalf("validMutation() = %v, want %v", got, test.valid)
			}
		})
	}
}

func TestProductionSessionCookieAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &Service{config: config.Config{PublicURL: "https://ship.example.com", PublicOrigin: "https://ship.example.com"}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	service.WriteCookie(c, "opaque-token")
	cookie := recorder.Header().Get("Set-Cookie")
	for _, required := range []string{
		"__Host-ship_session=opaque-token", "Path=/", "HttpOnly", "SameSite=Strict", "Secure",
	} {
		if !strings.Contains(cookie, required) {
			t.Fatalf("session cookie %q does not contain %q", cookie, required)
		}
	}
	if strings.Contains(strings.ToLower(cookie), "domain=") {
		t.Fatalf("__Host- cookie must not declare a domain: %q", cookie)
	}
}

func TestDevelopmentSessionCookieIsExplicitlyInsecure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &Service{config: config.Config{PublicURL: "http://localhost:3000", PublicOrigin: "http://localhost:3000"}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	service.WriteCookie(c, "opaque-token")
	cookie := recorder.Header().Get("Set-Cookie")
	if !strings.HasPrefix(cookie, "ship_session=opaque-token;") || strings.Contains(cookie, "; Secure") {
		t.Fatalf("unexpected development cookie: %q", cookie)
	}
}
