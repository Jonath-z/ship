package services

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/jsonfield"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
)

type createRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
	Image      string `json:"image"`
	Port       *int   `json:"port"`
	Command    string `json:"command"`
	Role       string `json:"role"`
}

type updateRequest struct {
	Name       *string                 `json:"name"`
	Type       *string                 `json:"type"`
	Repository *string                 `json:"repository"`
	Branch     *string                 `json:"branch"`
	Image      *string                 `json:"image"`
	Port       jsonfield.Nullable[int] `json:"port"`
	Command    *string                 `json:"command"`
	Role       *string                 `json:"role"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/services"
	router.GET(base, access.ConfigurationRead, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.List(ctx, c.Param("projectId"), c.Param("environmentId"), pagination.Cursor, pagination.Limit)
		if errors.Is(err, pagecursor.ErrInvalid) {
			httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
			return
		}
		if errors.Is(err, ErrEnvironmentNotFound) {
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
			return
		}
		if err != nil {
			httpx.WriteError(c, 500, "services_unavailable", "services are unavailable", nil)
			return
		}
		c.JSON(200, page)
	})

	router.POST(base, access.ConfigurationManage, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		created, err := service.Create(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), CreateInput{
			Name: request.Name, Type: request.Type, Repository: request.Repository, Branch: request.Branch,
			Image: request.Image, Port: request.Port, Command: request.Command, Role: request.Role,
		})
		writeMutationResult(c, created, err, 201)
	})

	router.GET(base+"/:serviceId", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := service.Get(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("serviceId"))
		writeReadResult(c, item, err)
	})

	router.PATCH(base+"/:serviceId", access.ConfigurationManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		updated, err := service.Update(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("serviceId"), UpdateInput{
			Name: request.Name, Type: request.Type, Repository: request.Repository, Branch: request.Branch,
			Image: request.Image, PortSet: request.Port.Set, Port: request.Port.Value,
			Command: request.Command, Role: request.Role,
		})
		writeMutationResult(c, updated, err, 200)
	})

	router.DELETE(base+"/:serviceId", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("serviceId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrServiceNotFound):
			httpx.WriteError(c, 404, "service_not_found", "service was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "service_delete_failed", "service could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeReadResult(c *gin.Context, item ServiceResource, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrServiceNotFound):
		httpx.WriteError(c, 404, "service_not_found", "service was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "service_unavailable", "service is unavailable", nil)
	default:
		c.JSON(200, item)
	}
}

func writeMutationResult(c *gin.Context, item ServiceResource, err error, successStatus int) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrServiceNotFound):
		httpx.WriteError(c, 404, "service_not_found", "service was not found", nil)
	case errors.Is(err, ErrNameExists):
		httpx.WriteError(c, 409, "service_name_exists", "a service with this name already exists in the environment", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "service_mutation_failed", "service could not be saved", nil)
	default:
		c.JSON(successStatus, item)
	}
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
