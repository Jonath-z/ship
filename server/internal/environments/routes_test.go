package environments

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func TestRoutesDeclareEnvironmentPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(gin.New(), func(access.Permission) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	})
	RegisterRoutes(router, config.Config{}, nil)

	want := map[string]access.Permission{
		"GET /projects/:projectId/environments":                                access.ConfigurationRead,
		"POST /projects/:projectId/environments":                               access.ConfigurationManage,
		"GET /projects/:projectId/environments/:environmentId":                 access.ConfigurationRead,
		"PATCH /projects/:projectId/environments/:environmentId":               access.ConfigurationManage,
		"POST /projects/:projectId/environments/:environmentId/clone":          access.ConfigurationManage,
		"GET /projects/:projectId/environments/:environmentId/deletion-impact": access.ConfigurationManage,
		"DELETE /projects/:projectId/environments/:environmentId":              access.ConfigurationManage,
	}
	for _, declaration := range router.Declarations() {
		key := declaration.Method + " " + declaration.Path
		permission, ok := want[key]
		if !ok {
			t.Fatalf("unexpected route %s", key)
		}
		if declaration.Permission != permission {
			t.Errorf("%s permission = %q, want %q", key, declaration.Permission, permission)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing route declarations: %#v", want)
	}
}
