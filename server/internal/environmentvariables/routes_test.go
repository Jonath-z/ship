package environmentvariables

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func TestRoutesDeclareConfigurationValuePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(gin.New(), func(access.Permission) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	})
	RegisterRoutes(router, config.Config{}, nil)
	variables := "/projects/:projectId/environments/:environmentId/environment-variables"
	secrets := "/projects/:projectId/environments/:environmentId/secrets"
	want := map[string]access.Permission{
		"GET " + variables:                      access.ConfigurationRead,
		"POST " + variables:                     access.ConfigurationManage,
		"POST " + variables + "/import":         access.ConfigurationManage,
		"GET " + variables + "/:variableId":     access.ConfigurationRead,
		"PATCH " + variables + "/:variableId":   access.ConfigurationManage,
		"DELETE " + variables + "/:variableId":  access.ConfigurationManage,
		"GET " + secrets:                        access.SecretsReadMetadata,
		"POST " + secrets:                       access.SecretsManage,
		"POST " + secrets + "/import":           access.SecretsManage,
		"GET " + secrets + "/:secretId":         access.SecretsReadMetadata,
		"PATCH " + secrets + "/:secretId":       access.SecretsManage,
		"DELETE " + secrets + "/:secretId":      access.SecretsManage,
		"POST " + secrets + "/:secretId/reveal": access.SecretsReveal,
	}
	for _, declaration := range router.Declarations() {
		key := declaration.Method + " " + declaration.Path
		if permission, ok := want[key]; !ok || permission != declaration.Permission {
			t.Fatalf("unexpected declaration %#v", declaration)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing declarations: %#v", want)
	}
}
