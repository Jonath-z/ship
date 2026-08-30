package auth

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func RegisterRoutes(router *httpx.Router, service *Service) {
	router.POST("/auth/login", access.Public, service.SameOrigin(), func(c *gin.Context) {
		var request loginRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		if details := validateLogin(request); len(details) > 0 {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", details)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		user, session, retryAfter, err := service.Login(
			ctx, request.Email, request.Password,
			httpx.ClientIP(c, service.config.TrustForwardedIP), c.GetString("requestID"),
		)
		switch {
		case errors.Is(err, ErrRateLimited):
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			httpx.WriteError(c, 429, "login_rate_limited", "too many login attempts; try again later", nil)
		case errors.Is(err, ErrInvalidCredentials):
			httpx.WriteError(c, 401, "invalid_credentials", "email or password is incorrect", nil)
		case err != nil:
			httpx.WriteError(c, 500, "login_failed", "login is temporarily unavailable", nil)
		default:
			service.WriteCookie(c, session.Token)
			c.Header("Cache-Control", "no-store")
			c.JSON(200, Session{User: userResponse(user), CSRFToken: session.Record.CSRFToken, ExpiresAt: session.Record.ExpiresAt})
		}
	})

	router.GET("/auth/session", access.Authenticated, func(c *gin.Context) {
		session, err := service.CurrentSession(c)
		if err != nil {
			httpx.WriteError(c, 401, "authentication_required", "authentication is required", nil)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(200, session)
	})

	router.POST("/auth/logout", access.Authenticated, func(c *gin.Context) {
		if err := service.Logout(c); err != nil {
			httpx.WriteError(c, 500, "logout_failed", "logout could not be completed", nil)
			return
		}
		c.Status(204)
	})

	router.POST("/auth/password", access.Authenticated, func(c *gin.Context) {
		var request changePasswordRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		details := validatePasswordChange(request)
		if len(details) > 0 {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", details)
			return
		}
		principal, _ := access.PrincipalFrom(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		issued, err := service.ChangePassword(ctx, principal, request.CurrentPassword, request.NewPassword)
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.WriteError(c, 401, "invalid_current_password", "current password is incorrect", nil)
		case err != nil:
			httpx.WriteError(c, 500, "password_change_failed", "password could not be changed", nil)
		default:
			service.record(ctx, audit.Event{
				ActorUserID: principal.UserID, ActorEmail: principal.Email,
				Action: "user.password_changed", ResourceType: "user", ResourceID: principal.UserID,
				Outcome: audit.OutcomeSuccess, SourceIP: httpx.ClientIP(c, service.config.TrustForwardedIP),
				RequestID: c.GetString("requestID"),
			})
			service.WriteCookie(c, issued.Token)
			c.JSON(200, Session{
				User:      User{ID: principal.UserID, Email: principal.Email, Role: principal.Role},
				CSRFToken: issued.Record.CSRFToken, ExpiresAt: issued.Record.ExpiresAt,
			})
		}
	})
}

func validateLogin(request loginRequest) []httpx.FieldError {
	details := make([]httpx.FieldError, 0, 2)
	email := strings.TrimSpace(request.Email)
	if len(email) < 3 || len(email) > 320 || !strings.Contains(email, "@") {
		details = append(details, httpx.FieldError{Field: "email", Code: "invalid", Message: "enter a valid email address"})
	}
	if request.Password == "" || len(request.Password) > 128 {
		details = append(details, httpx.FieldError{Field: "password", Code: "invalid", Message: "enter a valid password"})
	}
	return details
}

func validatePasswordChange(request changePasswordRequest) []httpx.FieldError {
	details := make([]httpx.FieldError, 0, 2)
	if request.CurrentPassword == "" || len(request.CurrentPassword) > 128 {
		details = append(details, httpx.FieldError{Field: "currentPassword", Code: "invalid", Message: "enter your current password"})
	}
	if len(request.NewPassword) < 12 || len(request.NewPassword) > 128 {
		details = append(details, httpx.FieldError{Field: "newPassword", Code: "length", Message: "must be between 12 and 128 characters"})
	} else if request.NewPassword == request.CurrentPassword {
		details = append(details, httpx.FieldError{Field: "newPassword", Code: "unchanged", Message: "must differ from your current password"})
	}
	return details
}
