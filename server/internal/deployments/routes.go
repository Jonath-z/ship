package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
)

type createRequest struct {
	ServiceID string `json:"serviceId"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/deployments"

	router.POST(base, access.DeploymentsExecute, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		deployment, err := service.Create(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), request.ServiceID)
		writeDeploymentResult(c, deployment, err, 202)
	})

	router.GET(base, access.DeploymentsRead, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.List(ctx, c.Param("projectId"), c.Param("environmentId"),
			c.Query("serviceId"), c.Query("status"), pagination.Cursor, pagination.Limit)
		switch {
		case errors.Is(err, pagecursor.ErrInvalid):
			httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "deployments_unavailable", "deployments are unavailable", nil)
		default:
			c.JSON(200, page)
		}
	})

	router.GET(base+"/:deploymentId", access.DeploymentsRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		deployment, err := service.Get(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("deploymentId"))
		writeDeploymentResult(c, deployment, err, 200)
	})

	router.POST(base+"/:deploymentId/rollback", access.DeploymentsExecute, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		deployment, err := service.Rollback(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("deploymentId"))
		writeDeploymentResult(c, deployment, err, 202)
	})

	router.GET(base+"/:deploymentId/logs", access.DeploymentsRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		after, _ := strconv.ParseInt(c.DefaultQuery("after", "0"), 10, 64)
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))
		entries, err := service.Logs(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("deploymentId"), after, limit)
		switch {
		case errors.Is(err, ErrEnvironmentNotFound), errors.Is(err, ErrDeploymentNotFound):
			httpx.WriteError(c, 404, "deployment_not_found", "deployment was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "deployment_logs_unavailable", "deployment logs are unavailable", nil)
		default:
			c.JSON(200, gin.H{"items": entries})
		}
	})

	// SSE stream (SH-082): replays persisted events after the cursor, then
	// relays live pub/sub. EventSource reconnects resume via Last-Event-ID.
	router.GET(base+"/:deploymentId/stream", access.DeploymentsRead, func(c *gin.Context) {
		deploymentID := c.Param("deploymentId")
		deployment, err := service.Get(c.Request.Context(), c.Param("projectId"), c.Param("environmentId"), deploymentID)
		if err != nil {
			httpx.WriteError(c, 404, "deployment_not_found", "deployment was not found", nil)
			return
		}
		after, _ := strconv.ParseInt(c.DefaultQuery("after", c.GetHeader("Last-Event-ID")), 10, 64)

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-store")
		c.Header("X-Accel-Buffering", "no")
		flusher := c.Writer

		// Subscribe before replay so nothing published in between is lost;
		// duplicates are filtered by sequence.
		subscription := service.redis.Subscribe(c.Request.Context(), channelFor(deploymentID))
		defer subscription.Close()

		lastSent := after
		send := func(event Event) {
			if event.Sequence <= lastSent {
				return
			}
			lastSent = event.Sequence
			payload, err := json.Marshal(event)
			if err != nil {
				return
			}
			fmt.Fprintf(flusher, "id: %d\ndata: %s\n\n", event.Sequence, payload)
			flusher.Flush()
		}

		replay, err := service.Logs(c.Request.Context(), c.Param("projectId"), c.Param("environmentId"), deploymentID, after, 5000)
		if err != nil {
			return
		}
		for _, entry := range replay {
			send(Event{Sequence: entry.Sequence, Type: "log", Stream: entry.Stream, Message: entry.Message})
		}
		if deployment.Status.Terminal() && len(replay) < 5000 {
			fmt.Fprintf(flusher, "event: done\ndata: %q\n\n", deployment.Status)
			flusher.Flush()
			return
		}

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-heartbeat.C:
				fmt.Fprint(flusher, ": keepalive\n\n")
				flusher.Flush()
			case message, open := <-subscription.Channel():
				if !open {
					return
				}
				var event Event
				if json.Unmarshal([]byte(message.Payload), &event) != nil {
					continue
				}
				send(event)
				if event.Type == "status" && event.Status.Terminal() {
					fmt.Fprintf(flusher, "event: done\ndata: %q\n\n", event.Status)
					flusher.Flush()
					return
				}
			}
		}
	})
}

func writeDeploymentResult(c *gin.Context, deployment DeploymentResource, err error, status int) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrServiceNotFound):
		httpx.WriteError(c, 404, "service_not_found", "service was not found in this environment", nil)
	case errors.Is(err, ErrDeploymentNotFound):
		httpx.WriteError(c, 404, "deployment_not_found", "deployment was not found", nil)
	case errors.Is(err, ErrRollbackUnavailable):
		httpx.WriteError(c, 409, "rollback_unavailable", "only successful deployments can be rolled back", nil)
	case err != nil:
		httpx.WriteError(c, 500, "deployment_failed", "deployment could not be created", nil)
	default:
		c.JSON(status, deployment)
	}
}

func requestContext(c *gin.Context, cfg config.Config) RequestContext {
	principal, _ := access.PrincipalFrom(c)
	return RequestContext{
		Actor: principal, SourceIP: httpx.ClientIP(c, cfg.TrustForwardedIP), RequestID: c.GetString("requestID"),
	}
}
