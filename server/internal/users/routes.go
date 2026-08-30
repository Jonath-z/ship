package users

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

type createUserRequest struct {
	Email    string      `json:"email"`
	Password string      `json:"password"`
	Role     access.Role `json:"role"`
}

type changeRoleRequest struct {
	Role access.Role `json:"role"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	router.GET("/users", access.UsersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		users, err := service.List(ctx)
		if err != nil {
			httpx.WriteError(c, 500, "users_unavailable", "users are unavailable", nil)
			return
		}
		c.JSON(200, map[string]any{"items": users})
	})

	router.POST("/users", access.UsersManage, func(c *gin.Context) {
		var request createUserRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		if details := validateCreate(request); len(details) > 0 {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", details)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		user, err := service.Create(ctx, requestContext(c, cfg), request.Email, request.Password, request.Role)
		switch {
		case errors.Is(err, ErrEmailExists):
			httpx.WriteError(c, 409, "email_exists", "a user with this email already exists", nil)
		case errors.Is(err, ErrOwnerOnly):
			httpx.WriteError(c, 403, "owner_required", "only an owner can create another owner", nil)
		case err != nil:
			httpx.WriteError(c, 500, "user_create_failed", "user could not be created", nil)
		default:
			c.JSON(201, user)
		}
	})

	router.PATCH("/users/:id/role", access.UsersManage, func(c *gin.Context) {
		var request changeRoleRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		if !access.ValidRole(request.Role) {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", []httpx.FieldError{{
				Field: "role", Code: "invalid", Message: "must be owner, admin, deployer, or viewer",
			}})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		user, err := service.ChangeRole(ctx, requestContext(c, cfg), c.Param("id"), request.Role)
		writeUserMutationResult(c, user, err)
	})

	router.DELETE("/users/:id", access.UsersManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		err := service.Disable(ctx, requestContext(c, cfg), c.Param("id"))
		switch {
		case errors.Is(err, ErrUserNotFound):
			httpx.WriteError(c, 404, "user_not_found", "user was not found", nil)
		case errors.Is(err, ErrOwnerRequired):
			httpx.WriteError(c, 409, "owner_required", "the installation must retain an active owner", nil)
		case errors.Is(err, ErrOwnerOnly), errors.Is(err, ErrCannotDisableSelf):
			httpx.WriteError(c, 403, "user_disable_forbidden", err.Error(), nil)
		case err != nil:
			httpx.WriteError(c, 500, "user_disable_failed", "user could not be disabled", nil)
		default:
			c.Status(204)
		}
	})
}

func validateCreate(request createUserRequest) []httpx.FieldError {
	details := make([]httpx.FieldError, 0, 3)
	email := strings.TrimSpace(request.Email)
	if len(email) < 3 || len(email) > 320 || !strings.Contains(email, "@") {
		details = append(details, httpx.FieldError{Field: "email", Code: "invalid", Message: "enter a valid email address"})
	}
	if len(request.Password) < 12 || len(request.Password) > 128 {
		details = append(details, httpx.FieldError{Field: "password", Code: "length", Message: "must be between 12 and 128 characters"})
	}
	if !access.ValidRole(request.Role) {
		details = append(details, httpx.FieldError{Field: "role", Code: "invalid", Message: "must be owner, admin, deployer, or viewer"})
	}
	return details
}

func requestContext(c *gin.Context, cfg config.Config) RequestContext {
	principal, _ := access.PrincipalFrom(c)
	return RequestContext{
		Actor: principal, SourceIP: httpx.ClientIP(c, cfg.TrustForwardedIP),
		RequestID: c.GetString("requestID"),
	}
}

func writeUserMutationResult(c *gin.Context, user User, err error) {
	switch {
	case errors.Is(err, ErrUserNotFound):
		httpx.WriteError(c, 404, "user_not_found", "user was not found", nil)
	case errors.Is(err, ErrOwnerRequired):
		httpx.WriteError(c, 409, "owner_required", "the installation must retain an active owner", nil)
	case errors.Is(err, ErrOwnerOnly):
		httpx.WriteError(c, 403, "owner_required", "only an owner can manage owner accounts", nil)
	case err != nil:
		httpx.WriteError(c, 500, "user_update_failed", "user could not be updated", nil)
	default:
		c.JSON(200, user)
	}
}
