package servers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/docker"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

// RegisterContainerRoutes exposes read-only container state per server
// (SH-046, SH-091 tail mode). Follow mode arrives with the logs screen work
// if V1 usage demands it; tail covers troubleshooting.
func RegisterContainerRoutes(router *httpx.Router, service *Service) {
	client := docker.NewClient(service.runner)

	router.GET("/servers/:serverId/containers", access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		row, err := service.find(ctx, c.Param("serverId"))
		if err != nil {
			httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
			return
		}
		signer, target, err := service.connection(ctx, row)
		if err != nil {
			httpx.WriteError(c, 409, "ssh_key_missing", "server has no usable SSH key", nil)
			return
		}
		containers, err := client.Containers(ctx, target, signer)
		if err != nil {
			httpx.WriteError(c, 502, "containers_unavailable", "containers could not be listed", nil)
			return
		}
		c.JSON(200, gin.H{"items": containers})
	})

	router.GET("/servers/:serverId/containers/:containerName/logs", access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		row, err := service.find(ctx, c.Param("serverId"))
		if err != nil {
			httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
			return
		}
		signer, target, err := service.connection(ctx, row)
		if err != nil {
			httpx.WriteError(c, 409, "ssh_key_missing", "server has no usable SSH key", nil)
			return
		}
		lines := 200
		if requested := c.Query("lines"); requested != "" {
			if parsed, parseErr := parsePositive(requested); parseErr == nil {
				lines = parsed
			}
		}
		output, err := client.Logs(ctx, target, signer, c.Param("containerName"), lines, nil)
		if err != nil {
			httpx.WriteError(c, 502, "container_logs_unavailable", "container logs could not be read", nil)
			return
		}
		c.JSON(200, gin.H{"log": output})
	})
}

func parsePositive(value string) (int, error) {
	parsed := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, errInvalidNumber
		}
		parsed = parsed*10 + int(character-'0')
		if parsed > 2000 {
			return 2000, nil
		}
	}
	return parsed, nil
}

var errInvalidNumber = errInvalid("not a number")

type errInvalid string

func (err errInvalid) Error() string { return string(err) }
