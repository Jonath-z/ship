package webhooks

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/deployments"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

// RegisterRoutes exposes the GitHub webhook receiver. The route is public by
// design: GitHub cannot hold a Ship session, and every deployment-triggering
// path is gated by the per-environment HMAC verification instead. Responses
// stay uniform so unverified callers cannot probe what exists.
func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	router.POST("/webhooks/github", access.Public, func(c *gin.Context) {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body could not be read", nil)
			return
		}
		switch c.GetHeader("X-GitHub-Event") {
		case "ping":
			c.JSON(200, gin.H{"ok": true})
		case "push":
			ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
			defer cancel()
			queued, err := service.HandlePush(ctx, body, c.GetHeader("X-Hub-Signature-256"), deployments.RequestContext{
				Actor:     access.Principal{Email: "github-webhook"},
				SourceIP:  httpx.ClientIP(c, cfg.TrustForwardedIP),
				RequestID: c.GetString("requestID"),
			})
			switch {
			case errors.Is(err, errInvalidPayload):
				httpx.WriteError(c, 400, "invalid_request", "payload is not a GitHub push event", nil)
			case err != nil:
				httpx.WriteError(c, 500, "webhook_failed", "webhook could not be processed", nil)
			default:
				c.JSON(202, gin.H{"queued": queued})
			}
		default:
			c.JSON(202, gin.H{"queued": 0})
		}
	})
}
