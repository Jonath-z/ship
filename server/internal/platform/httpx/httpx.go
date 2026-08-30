// Package httpx contains the small set of conventions shared by Gin routes.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorEnvelope struct {
	Error struct {
		Code      string       `json:"code"`
		Message   string       `json:"message"`
		RequestID string       `json:"requestId"`
		Details   []FieldError `json:"details,omitempty"`
	} `json:"error"`
}

func Middleware(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if requestID == "" || len(requestID) > 128 {
			requestID = newRequestID()
		}
		c.Set("requestID", requestID)
		c.Header(requestIDHeader, requestID)

		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error("panic serving request", "panic", recovered, "request_id", requestID)
				WriteError(c, 500, "internal_error", "an unexpected error occurred", nil)
			}
			log.Info("http request",
				"request_id", requestID,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", c.Writer.Status(),
				"duration_ms", time.Since(started).Milliseconds(),
			)
		}()

		c.Next()
	}
}

func SecurityHeaders(secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if secure {
			c.Header("Strict-Transport-Security", "max-age=31536000")
		}
		c.Next()
	}
}

func NotFound(c *gin.Context) {
	WriteError(c, 404, "not_found", "resource not found", nil)
}

func WriteError(c *gin.Context, status int, code, message string, details []FieldError) {
	response := ErrorEnvelope{}
	response.Error.Code = code
	response.Error.Message = message
	response.Error.RequestID = c.GetString("requestID")
	response.Error.Details = details
	c.AbortWithStatusJSON(status, response)
}

type Pagination struct {
	Cursor string
	Limit  int
}

func ParsePagination(c *gin.Context) (Pagination, bool) {
	page := Pagination{Cursor: c.Query("cursor"), Limit: 20}
	value := c.Query("limit")
	if value == "" {
		return page, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 100 {
		WriteError(c, 400, "validation_error", "request validation failed", []FieldError{{
			Field: "limit", Code: "range", Message: "must be between 1 and 100",
		}})
		return Pagination{}, false
	}
	page.Limit = limit
	return page, true
}

func ClientIP(c *gin.Context, trustForwarded bool) string {
	if trustForwarded {
		value := strings.TrimSpace(c.GetHeader("X-Ship-Client-IP"))
		if parsed := net.ParseIP(value); parsed != nil {
			return parsed.String()
		}
	}
	value := strings.TrimSpace(c.ClientIP())
	if parsed := net.ParseIP(value); parsed != nil {
		return parsed.String()
	}
	return "unknown"
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(buffer)
}
