package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name        string
		secure      bool
		expectsHSTS bool
	}{
		{name: "HTTPS", secure: true, expectsHSTS: true},
		{name: "HTTP bootstrap", secure: false, expectsHSTS: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(SecurityHeaders(test.secure))
			engine.GET("/", func(c *gin.Context) { c.Status(204) })
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest("GET", "http://ship.test/", nil))

			for _, name := range []string{
				"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options",
				"Referrer-Policy", "Permissions-Policy",
			} {
				if recorder.Header().Get(name) == "" {
					t.Fatalf("missing %s", name)
				}
			}
			hasHSTS := recorder.Header().Get("Strict-Transport-Security") != ""
			if hasHSTS != test.expectsHSTS {
				t.Fatalf("HSTS present = %v, want %v", hasHSTS, test.expectsHSTS)
			}
		})
	}
}
