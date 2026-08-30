package setup

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/auth"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

type createOwnerRequest struct {
	Token    string `json:"token"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, db *database.Connection, authService *auth.Service, recorder audit.Recorder) {
	service := NewService(db.ORM, cfg.FirstRunTokenHash, cfg.Hostname)

	router.GET("/setup", access.Setup, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		status, err := service.Status(ctx)
		if err != nil {
			httpx.WriteError(c, 500, "setup_status_failed", "setup status is unavailable", nil)
			return
		}
		c.JSON(200, status)
	})

	router.POST("/setup", access.Setup, authService.SameOrigin(), authService.LimitByIP("setup", 10, 15*time.Minute), func(c *gin.Context) {
		var request createOwnerRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		if details := validateRequest(request); len(details) > 0 {
			httpx.WriteError(c, 422, "validation_error", "request validation failed", details)
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		owner, err := service.CreateOwner(ctx, request.Token, request.Email, request.Password)
		switch {
		case errors.Is(err, ErrInvalidToken):
			httpx.WriteError(c, 401, "invalid_setup_token", "first-run token is invalid", nil)
		case errors.Is(err, ErrAlreadyComplete):
			httpx.WriteError(c, 409, "setup_complete", "setup is already complete", nil)
		case err != nil:
			httpx.WriteError(c, 500, "setup_failed", "owner account could not be created", nil)
		default:
			issued, sessionErr := authService.IssueForUser(ctx, owner.ID)
			if sessionErr != nil {
				httpx.WriteError(c, 500, "setup_session_failed", "owner was created; sign in to continue", nil)
				return
			}
			if recorder != nil {
				_ = recorder.Record(ctx, audit.Event{
					ActorUserID: owner.ID, ActorEmail: owner.Email,
					Action: "user.created", ResourceType: "user", ResourceID: owner.ID,
					Outcome: audit.OutcomeSuccess, SourceIP: httpx.ClientIP(c, cfg.TrustForwardedIP),
					RequestID: c.GetString("requestID"), Metadata: map[string]any{"role": owner.Role, "bootstrap": true},
				})
			}
			authService.WriteCookie(c, issued.Token)
			c.Header("Cache-Control", "no-store")
			c.JSON(201, owner)
		}
	})
}

func validateRequest(request createOwnerRequest) []httpx.FieldError {
	details := make([]httpx.FieldError, 0, 3)
	email := strings.TrimSpace(request.Email)
	if len(request.Token) < 32 {
		details = append(details, httpx.FieldError{
			Field: "token", Code: "invalid", Message: "enter the token printed by the installer",
		})
	}
	if len(email) < 3 || len(email) > 320 || !strings.Contains(email, "@") {
		details = append(details, httpx.FieldError{
			Field: "email", Code: "invalid", Message: "enter a valid email address",
		})
	}
	if len(request.Password) < 12 || len(request.Password) > 128 {
		details = append(details, httpx.FieldError{
			Field: "password", Code: "length", Message: "must be between 12 and 128 characters",
		})
	}
	return details
}
