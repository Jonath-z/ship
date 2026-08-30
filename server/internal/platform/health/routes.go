package health

import (
	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func RegisterRoutes(router *httpx.Router, component string, checks map[string]Check) {
	router.GET("/healthz", access.Public, Handler(component, checks))
}
