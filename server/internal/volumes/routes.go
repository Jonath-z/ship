package volumes

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
	ServiceID   *string `json:"serviceId"`
	AccessoryID *string `json:"accessoryId"`
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
}

type updateRequest struct {
	Name        *string `json:"name"`
	Source      *string `json:"source"`
	Destination *string `json:"destination"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/volumes"
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
			httpx.WriteError(c, 500, "volumes_unavailable", "volumes are unavailable", nil)
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
			ServiceID: request.ServiceID, AccessoryID: request.AccessoryID,
			Name: request.Name, Source: request.Source, Destination: request.Destination,
		})
		writeMutationResult(c, created, err, 201)
	})

	router.GET(base+"/:volumeId", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := service.Get(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("volumeId"))
		writeReadResult(c, item, err)
	})

	router.PATCH(base+"/:volumeId", access.ConfigurationManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		updated, err := service.Update(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("volumeId"), UpdateInput{
			Name: request.Name, Source: request.Source, Destination: request.Destination,
		})
		writeMutationResult(c, updated, err, 200)
	})

	router.DELETE(base+"/:volumeId", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("volumeId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrVolumeNotFound):
			httpx.WriteError(c, 404, "volume_not_found", "volume was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "volume_delete_failed", "volume could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeReadResult(c *gin.Context, item VolumeResource, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrVolumeNotFound):
		httpx.WriteError(c, 404, "volume_not_found", "volume was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "volume_unavailable", "volume is unavailable", nil)
	default:
		c.JSON(200, item)
	}
}

func writeMutationResult(c *gin.Context, item VolumeResource, err error, successStatus int) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrVolumeNotFound):
		httpx.WriteError(c, 404, "volume_not_found", "volume was not found", nil)
	case errors.Is(err, ErrOwnerNotFound):
		httpx.WriteError(c, 404, "volume_owner_not_found", "volume owner was not found in the environment", nil)
	case errors.Is(err, ErrSourceExists):
		httpx.WriteError(c, 409, "volume_source_exists", "a volume with this source already exists in the environment", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "volume_mutation_failed", "volume could not be saved", nil)
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
