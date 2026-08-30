package environments

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
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type updateRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
}

type cloneRequest struct {
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	IncludeSecrets bool   `json:"includeSecrets"`
}

type deleteRequest struct {
	ConfirmSlug string `json:"confirmSlug"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	router.GET("/projects/:projectId/environments", access.ConfigurationRead, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.List(ctx, c.Param("projectId"), pagination.Cursor, pagination.Limit)
		switch {
		case errors.Is(err, ErrProjectNotFound):
			httpx.WriteError(c, 404, "project_not_found", "project was not found", nil)
		case errors.Is(err, pagecursor.ErrInvalid):
			httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
		case err != nil:
			httpx.WriteError(c, 500, "environments_unavailable", "environments are unavailable", nil)
		default:
			c.JSON(200, page)
		}
	})

	router.POST("/projects/:projectId/environments", access.ConfigurationManage, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		environment, err := service.Create(ctx, requestContext(c, cfg), c.Param("projectId"), CreateInput{
			Name: request.Name, Slug: request.Slug,
		})
		writeMutationResult(c, environment, err, 201)
	})

	router.GET("/projects/:projectId/environments/:environmentId", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		environment, err := service.Get(ctx, c.Param("projectId"), c.Param("environmentId"))
		writeReadResult(c, environment, err)
	})

	router.PATCH("/projects/:projectId/environments/:environmentId", access.ConfigurationManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		environment, err := service.Update(
			ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"),
			UpdateInput{Name: request.Name, Slug: request.Slug},
		)
		writeMutationResult(c, environment, err, 200)
	})

	router.POST("/projects/:projectId/environments/:environmentId/clone", access.ConfigurationManage, func(c *gin.Context) {
		var request cloneRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		environment, err := service.Clone(
			ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"),
			CloneInput{Name: request.Name, Slug: request.Slug, IncludeSecrets: request.IncludeSecrets},
		)
		writeMutationResult(c, environment, err, 201)
	})

	router.GET("/projects/:projectId/environments/:environmentId/deletion-impact", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		impact, err := service.DeletionImpact(ctx, c.Param("projectId"), c.Param("environmentId"))
		if errors.Is(err, ErrEnvironmentNotFound) {
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
			return
		}
		if err != nil {
			httpx.WriteError(c, 500, "environment_impact_failed", "environment deletion impact is unavailable", nil)
			return
		}
		c.JSON(200, impact)
	})

	router.DELETE("/projects/:projectId/environments/:environmentId", access.ConfigurationManage, func(c *gin.Context) {
		var request deleteRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		err := service.Delete(
			ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), request.ConfirmSlug,
		)
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrConfirmationFailed):
			httpx.WriteError(c, 409, "confirmation_mismatch", "environment slug confirmation does not match", nil)
		case err != nil:
			if writeValidationError(c, err) {
				return
			}
			httpx.WriteError(c, 500, "environment_delete_failed", "environment could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeReadResult(c *gin.Context, environment Environment, err error) {
	if errors.Is(err, ErrEnvironmentNotFound) {
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(c, 500, "environment_unavailable", "environment is unavailable", nil)
		return
	}
	c.JSON(200, environment)
}

func writeMutationResult(c *gin.Context, environment Environment, err error, successStatus int) {
	switch {
	case errors.Is(err, ErrProjectNotFound):
		httpx.WriteError(c, 404, "project_not_found", "project was not found", nil)
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrSlugExists):
		httpx.WriteError(c, 409, "environment_slug_exists", "an environment with this slug already exists in the project", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "environment_mutation_failed", "environment could not be saved", nil)
	default:
		c.JSON(successStatus, environment)
	}
}

func writeValidationError(c *gin.Context, err error) bool {
	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		return false
	}
	details := make([]httpx.FieldError, 0, len(validationError.Fields))
	for _, violation := range validationError.Fields {
		details = append(details, httpx.FieldError{
			Field: violation.Field, Code: violation.Code, Message: violation.Message,
		})
	}
	httpx.WriteError(c, 422, "validation_error", "request validation failed", details)
	return true
}

func requestContext(c *gin.Context, cfg config.Config) RequestContext {
	principal, _ := access.PrincipalFrom(c)
	return RequestContext{
		Actor: principal, SourceIP: httpx.ClientIP(c, cfg.TrustForwardedIP),
		RequestID: c.GetString("requestID"),
	}
}
