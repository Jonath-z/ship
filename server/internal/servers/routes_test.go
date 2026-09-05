package servers

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func TestRoutesDeclareServerPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(gin.New(), func(access.Permission) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	})
	RegisterRoutes(router, config.Config{}, nil)
	groupBase := "/projects/:projectId/environments/:environmentId/server-groups"
	want := map[string]access.Permission{
		"GET /servers":                                        access.ServersRead,
		"POST /servers":                                       access.ServersManage,
		"GET /servers/:serverId":                              access.ServersRead,
		"PATCH /servers/:serverId":                            access.ServersManage,
		"DELETE /servers/:serverId":                           access.ServersManage,
		"POST /servers/:serverId/checks":                      access.ServersManage,
		"POST /servers/:serverId/prepare":                     access.ServersManage,
		"GET " + groupBase:                                    access.ServersRead,
		"POST " + groupBase + "/:groupId/servers":             access.ServersManage,
		"DELETE " + groupBase + "/:groupId/servers/:serverId": access.ServersManage,
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
