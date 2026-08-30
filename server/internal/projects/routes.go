package projects

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

type deleteRequest struct {
	ConfirmSlug string `json:"confirmSlug"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	router.GET("/projects", access.ProjectsRead, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.List(ctx, pagination.Cursor, pagination.Limit)
		if errors.Is(err, pagecursor.ErrInvalid) {
			httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
			return
		}
		if err != nil {
			httpx.WriteError(c, 500, "projects_unavailable", "projects are unavailable", nil)
			return
		}
		c.JSON(200, page)
	})

	router.POST("/projects", access.ProjectsManage, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		project, err := service.Create(ctx, requestContext(c, cfg), CreateInput{Name: request.Name, Slug: request.Slug})
		writeMutationResult(c, project, err, 201)
	})

	router.GET("/projects/:projectId", access.ProjectsRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		project, err := service.Get(ctx, c.Param("projectId"))
		writeReadResult(c, project, err)
	})

	router.PATCH("/projects/:projectId", access.ProjectsManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		project, err := service.Update(ctx, requestContext(c, cfg), c.Param("projectId"), UpdateInput{
			Name: request.Name, Slug: request.Slug,
		})
		writeMutationResult(c, project, err, 200)
	})

	router.GET("/projects/:projectId/deletion-impact", access.ProjectsManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		impact, err := service.DeletionImpact(ctx, c.Param("projectId"))
		if errors.Is(err, ErrProjectNotFound) {
			httpx.WriteError(c, 404, "project_not_found", "project was not found", nil)
			return
		}
		if err != nil {
			httpx.WriteError(c, 500, "project_impact_failed", "project deletion impact is unavailable", nil)
			return
		}
		c.JSON(200, impact)
	})

	router.DELETE("/projects/:projectId", access.ProjectsManage, func(c *gin.Context) {
		var request deleteRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("projectId"), request.ConfirmSlug)
		switch {
		case errors.Is(err, ErrProjectNotFound):
			httpx.WriteError(c, 404, "project_not_found", "project was not found", nil)
		case errors.Is(err, ErrConfirmationFailed):
			httpx.WriteError(c, 409, "confirmation_mismatch", "project slug confirmation does not match", nil)
		case err != nil:
			if writeValidationError(c, err) {
				return
			}
			httpx.WriteError(c, 500, "project_delete_failed", "project could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeReadResult(c *gin.Context, project Project, err error) {
	if errors.Is(err, ErrProjectNotFound) {
		httpx.WriteError(c, 404, "project_not_found", "project was not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(c, 500, "project_unavailable", "project is unavailable", nil)
		return
	}
	c.JSON(200, project)
}

func writeMutationResult(c *gin.Context, project Project, err error, successStatus int) {
	switch {
	case errors.Is(err, ErrProjectNotFound):
		httpx.WriteError(c, 404, "project_not_found", "project was not found", nil)
	case errors.Is(err, ErrSlugExists):
		httpx.WriteError(c, 409, "project_slug_exists", "a project with this slug already exists", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "project_mutation_failed", "project could not be saved", nil)
	default:
		c.JSON(successStatus, project)
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
