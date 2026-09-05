package sshkeys

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func TestRoutesDeclareSSHKeyPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(gin.New(), func(access.Permission) gin.HandlerFunc {
		return func(c *gin.Context) { c.Next() }
	})
	RegisterRoutes(router, config.Config{}, nil)
	want := map[string]access.Permission{
		"GET /ssh-keys":           access.ServersRead,
		"POST /ssh-keys":          access.ServersManage,
		"GET /ssh-keys/:keyId":    access.ServersRead,
		"DELETE /ssh-keys/:keyId": access.ServersManage,
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
