package sshkeys

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

type createRequest struct {
	Name       string `json:"name"`
	PrivateKey string `json:"privateKey"` // optional: import instead of generate
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	router.GET("/ssh-keys", access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		keys, err := service.List(ctx)
		if err != nil {
			httpx.WriteError(c, 500, "ssh_keys_unavailable", "SSH keys are unavailable", nil)
			return
		}
		c.JSON(200, gin.H{"items": keys})
	})

	router.POST("/ssh-keys", access.ServersManage, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		var key KeyResource
		var err error
		if request.PrivateKey != "" {
			key, err = service.Import(ctx, requestContext(c, cfg), request.Name, request.PrivateKey)
		} else {
			key, err = service.Create(ctx, requestContext(c, cfg), request.Name)
		}
		switch {
		case errors.Is(err, ErrNameExists):
			httpx.WriteError(c, 409, "ssh_key_name_exists", "an SSH key with this name already exists", nil)
		case errors.Is(err, ErrInvalidImport):
			httpx.WriteError(c, 422, "ssh_key_invalid", "private key could not be parsed", nil)
		case err != nil:
			if writeValidationError(c, err) {
				return
			}
			httpx.WriteError(c, 500, "ssh_key_mutation_failed", "SSH key could not be saved", nil)
		default:
			c.JSON(201, key)
		}
	})

	router.GET("/ssh-keys/:keyId", access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		key, err := service.Get(ctx, c.Param("keyId"))
		switch {
		case errors.Is(err, ErrKeyNotFound):
			httpx.WriteError(c, 404, "ssh_key_not_found", "SSH key was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "ssh_key_unavailable", "SSH key is unavailable", nil)
		default:
			c.JSON(200, key)
		}
	})

	router.DELETE("/ssh-keys/:keyId", access.ServersManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("keyId"))
		switch {
		case errors.Is(err, ErrKeyNotFound):
			httpx.WriteError(c, 404, "ssh_key_not_found", "SSH key was not found", nil)
		case errors.Is(err, ErrKeyInUse):
			httpx.WriteError(c, 409, "ssh_key_in_use", "SSH key is assigned to one or more servers", nil)
		case err != nil:
			httpx.WriteError(c, 500, "ssh_key_delete_failed", "SSH key could not be deleted", nil)
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
