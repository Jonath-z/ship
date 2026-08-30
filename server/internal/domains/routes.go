package domains

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
	ServiceID  string `json:"serviceId"`
	Hostname   string `json:"hostname"`
	SSLEnabled bool   `json:"sslEnabled"`
}

type updateRequest struct {
	Hostname   *string `json:"hostname"`
	SSLEnabled *bool   `json:"sslEnabled"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/domains"
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
			httpx.WriteError(c, 500, "domains_unavailable", "domains are unavailable", nil)
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
		writeMutationResult(c, item, err, 201)
	})

	router.GET(base+"/:domainId", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := service.Get(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("domainId"))
		writeReadResult(c, item, err)
	})

	router.PATCH(base+"/:domainId", access.ConfigurationManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		item, err := service.Update(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("domainId"), UpdateInput(request))
		writeMutationResult(c, item, err, 200)
	})

	router.DELETE(base+"/:domainId", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("domainId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrDomainNotFound):
			httpx.WriteError(c, 404, "domain_not_found", "domain was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "domain_delete_failed", "domain could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeReadResult(c *gin.Context, item DomainResource, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrDomainNotFound):
		httpx.WriteError(c, 404, "domain_not_found", "domain was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "domain_unavailable", "domain is unavailable", nil)
	default:
		c.JSON(200, item)
	}
}

func writeMutationResult(c *gin.Context, item DomainResource, err error, status int) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrServiceNotFound):
		httpx.WriteError(c, 404, "service_not_found", "service was not found in this environment", nil)
	case errors.Is(err, ErrDomainNotFound):
		httpx.WriteError(c, 404, "domain_not_found", "domain was not found", nil)
	case errors.Is(err, ErrHostnameExists):
		httpx.WriteError(c, 409, "domain_hostname_exists", "this hostname is already used in the environment", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "domain_mutation_failed", "domain could not be saved", nil)
	default:
		c.JSON(status, item)
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
