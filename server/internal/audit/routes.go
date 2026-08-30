package audit

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

func RegisterRoutes(router *httpx.Router, service *Service) {
	router.GET("/audit", access.AuditRead, func(c *gin.Context) {
		page, ok := parseAuditFilters(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		result, err := service.List(ctx, page)
		if err != nil {
			if strings.Contains(err.Error(), "cursor") {
				httpx.WriteError(c, 400, "invalid_cursor", "audit cursor is invalid", nil)
				return
			}
			httpx.WriteError(c, 500, "audit_unavailable", "audit events are unavailable", nil)
			return
		}
		c.JSON(200, result)
	})
}

func parseAuditFilters(c *gin.Context) (Filters, bool) {
	pagination, ok := httpx.ParsePagination(c)
	if !ok {
		return Filters{}, false
	}
	filters := Filters{
		Action: c.Query("action"), ResourceType: c.Query("resourceType"),
		ActorUserID: c.Query("actorUserId"), Cursor: pagination.Cursor, Limit: pagination.Limit,
	}
	if value := c.Query("from"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", []httpx.FieldError{{
				Field: "from", Code: "format", Message: "must be an RFC3339 timestamp",
			}})
			return Filters{}, false
		}
		filters.From = &parsed
	}
	if value := c.Query("to"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", []httpx.FieldError{{
				Field: "to", Code: "format", Message: "must be an RFC3339 timestamp",
			}})
			return Filters{}, false
		}
		filters.To = &parsed
	}
	return filters, true
}
