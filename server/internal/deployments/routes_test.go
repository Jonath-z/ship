package deployments

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func TestRoutesDeclareDeploymentPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(gin.New(), func(access.Permission) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	})
	RegisterRoutes(router, config.Config{}, nil)
	base := "/projects/:projectId/environments/:environmentId/deployments"
	want := map[string]access.Permission{
		"POST " + base:                             access.DeploymentsExecute,
		"GET " + base:                              access.DeploymentsRead,
		"GET " + base + "/:deploymentId":           access.DeploymentsRead,
		"POST " + base + "/:deploymentId/rollback": access.DeploymentsExecute,
		"GET " + base + "/:deploymentId/logs":      access.DeploymentsRead,
		"GET " + base + "/:deploymentId/stream":    access.DeploymentsRead,
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
