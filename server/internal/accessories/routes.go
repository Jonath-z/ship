package accessories

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
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Image         string  `json:"image"`
	ServerID      *string `json:"serverId"`
	ServerGroupID *string `json:"serverGroupId"`
	Port          *int    `json:"port"`
}

type updateRequest struct {
	Name          *string                    `json:"name"`
	Type          *string                    `json:"type"`
	Image         *string                    `json:"image"`
	ServerID      jsonfield.Nullable[string] `json:"serverId"`
	ServerGroupID jsonfield.Nullable[string] `json:"serverGroupId"`
	Port          jsonfield.Nullable[int]    `json:"port"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/accessories"
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
			httpx.WriteError(c, 500, "accessories_unavailable", "accessories are unavailable", nil)
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
			Name: request.Name, Type: request.Type, Image: request.Image,
			ServerID: request.ServerID, ServerGroupID: request.ServerGroupID, Port: request.Port,
		})
		writeMutationResult(c, created, err, 201)
	})

	router.GET(base+"/:accessoryId", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := service.Get(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("accessoryId"))
		writeReadResult(c, item, err)
	})

	router.PATCH(base+"/:accessoryId", access.ConfigurationManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		updated, err := service.Update(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("accessoryId"), UpdateInput{
			Name: request.Name, Type: request.Type, Image: request.Image,
			PlacementSet: request.ServerID.Set || request.ServerGroupID.Set,
			ServerID:     request.ServerID.Value, ServerGroupID: request.ServerGroupID.Value,
			PortSet: request.Port.Set, Port: request.Port.Value,
		})
		writeMutationResult(c, updated, err, 200)
	})

	router.DELETE(base+"/:accessoryId", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("accessoryId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrAccessoryNotFound):
			httpx.WriteError(c, 404, "accessory_not_found", "accessory was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "accessory_delete_failed", "accessory could not be deleted", nil)
		default:
			c.Status(204)
		}
	})
}

func writeReadResult(c *gin.Context, item AccessoryResource, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrAccessoryNotFound):
		httpx.WriteError(c, 404, "accessory_not_found", "accessory was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "accessory_unavailable", "accessory is unavailable", nil)
	default:
		c.JSON(200, item)
	}
}

func writeMutationResult(c *gin.Context, item AccessoryResource, err error, successStatus int) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrAccessoryNotFound):
		httpx.WriteError(c, 404, "accessory_not_found", "accessory was not found", nil)
	case errors.Is(err, ErrPlacementNotFound):
		httpx.WriteError(c, 404, "placement_not_found", "server or server group was not found", nil)
	case errors.Is(err, ErrNameExists):
		httpx.WriteError(c, 409, "accessory_name_exists", "an accessory with this name already exists in the environment", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "accessory_mutation_failed", "accessory could not be saved", nil)
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
