package httpx

import (
	_ "embed"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
)

// OpenAPISpec is served by the API and consumed by the TypeScript generator.
//
//go:embed openapi.yaml
var OpenAPISpec []byte

func RegisterOpenAPIRoute(router *Router) {
	router.GET("/openapi.yaml", access.Public, func(c *gin.Context) {
		c.Data(200, "application/yaml", OpenAPISpec)
	})
}
