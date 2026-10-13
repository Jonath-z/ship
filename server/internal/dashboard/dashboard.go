// Package dashboard serves the statically exported web dashboard from the
// same origin as the API. The API owns everything under httpx.APIPrefix;
// every other GET falls through to the exported files.
package dashboard

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

// Register installs the NoRoute handler. dir is the directory holding the
// Next.js export (SHIP_WEB_DIR); when empty the server is API-only and every
// unknown path gets the JSON 404.
func Register(engine *gin.Engine, dir string, secure, development bool) {
	engine.NoRoute(handler(dir, secure, development))
}

func handler(dir string, secure, development bool) gin.HandlerFunc {
	policy := contentSecurityPolicy(secure, development)
	return func(c *gin.Context) {
		requestPath := c.Request.URL.Path
		if dir == "" ||
			requestPath == httpx.APIPrefix || strings.HasPrefix(requestPath, httpx.APIPrefix+"/") ||
			(c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
			httpx.NotFound(c)
			return
		}

		// path.Clean collapses any ".." segments, so the join cannot escape dir.
		relative := strings.TrimPrefix(path.Clean(requestPath), "/")
		if relative == "" {
			relative = "index.html"
		}
		file, ok := resolve(dir, relative)
		if !ok {
			pageHeaders(c, policy, false)
			// c.File would reset the status to 200, so write the body directly.
			if page, err := os.ReadFile(filepath.Join(dir, "404.html")); err == nil {
				c.Data(http.StatusNotFound, "text/html; charset=utf-8", page)
			} else {
				c.Status(http.StatusNotFound)
			}
			return
		}
		pageHeaders(c, policy, strings.HasPrefix(relative, "_next/"))
		c.File(file)
	}
}

// resolve maps a URL path onto the export layout: an exact file, or the
// "/env/applications" → "env/applications.html" form Next produces.
func resolve(dir, relative string) (string, bool) {
	for _, candidate := range []string{relative, relative + ".html"} {
		full := filepath.Join(dir, filepath.FromSlash(candidate))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, true
		}
	}
	return "", false
}

// pageHeaders replaces the restrictive API-wide security headers with ones
// that let the dashboard's own scripts and styles run (previously set by the
// Next.js middleware). Hashed build assets are immutable; HTML is not.
func pageHeaders(c *gin.Context, policy string, immutable bool) {
	c.Header("Content-Security-Policy", policy)
	c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
	if immutable {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		c.Header("Cache-Control", "no-cache")
	}
}

func contentSecurityPolicy(secure, development bool) string {
	scriptSources := "'self' 'unsafe-inline'"
	if development {
		scriptSources += " 'unsafe-eval'"
	}
	directives := []string{
		"default-src 'self'",
		"script-src " + scriptSources,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}
	if secure {
		directives = append(directives, "upgrade-insecure-requests")
	}
	return strings.Join(directives, "; ")
}
