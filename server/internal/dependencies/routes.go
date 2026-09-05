package dependencies

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
)

type createRequest struct {
	SourceServiceID   string  `json:"sourceServiceId"`
	TargetServiceID   *string `json:"targetServiceId"`
	TargetAccessoryID *string `json:"targetAccessoryId"`
	Type              string  `json:"type"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/dependencies"
	router.GET(base, access.ConfigurationRead, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.List(ctx, c.Param("projectId"), c.Param("environmentId"), pagination.Cursor, pagination.Limit)
		switch {
		case errors.Is(err, pagecursor.ErrInvalid):
			httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "dependencies_unavailable", "dependencies are unavailable", nil)
		default:
			c.JSON(200, page)
		}
	})

	router.POST(base, access.ConfigurationManage, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		item, err := service.Create(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), CreateInput(request))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrSourceNotFound):
			httpx.WriteError(c, 404, "source_service_not_found", "source service was not found in this environment", nil)
		case errors.Is(err, ErrTargetNotFound):
			httpx.WriteError(c, 404, "dependency_target_not_found", "dependency target was not found in this environment", nil)
		case errors.Is(err, ErrDependencyExists):
			httpx.WriteError(c, 409, "dependency_exists", "this dependency already exists", nil)
		case errors.Is(err, ErrDependencyCycle):
			httpx.WriteError(c, 409, "dependency_cycle", "this dependency would create a cycle", nil)
		case err != nil:
			if writeValidationError(c, err) {
				return
			}
			httpx.WriteError(c, 500, "dependency_mutation_failed", "dependency could not be saved", nil)
		default:
			c.JSON(201, item)
		}
	})

	router.DELETE(base+"/:dependencyId", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("dependencyId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrDependencyNotFound):
			httpx.WriteError(c, 404, "dependency_not_found", "dependency was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "dependency_delete_failed", "dependency could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeValidationError(c *gin.Context, err error) bool {
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		return false
	}
	details := make([]httpx.FieldError, 0, len(validationError.Fields))
	for _, violation := range validationError.Fields {
		details = append(details, httpx.FieldError{Field: violation.Field, Code: violation.Code, Message: violation.Message})
	}
	httpx.WriteError(c, 422, "validation_error", "request validation failed", details)
	return true
}

func requestContext(c *gin.Context, cfg config.Config) RequestContext {
	principal, _ := access.PrincipalFrom(c)
	return RequestContext{
		Actor: principal, SourceIP: httpx.ClientIP(c, cfg.TrustForwardedIP), RequestID: c.GetString("requestID"),
	}
}
