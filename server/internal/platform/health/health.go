// Package health provides dependency-aware Gin health handlers.
package health

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/platform/buildinfo"
)

type Check func(context.Context) error

type Dependency struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Response struct {
	Status    string                `json:"status"`
	Component string                `json:"component"`
	BuildSHA  string                `json:"buildSha"`
	Version   string                `json:"version"`
	Checks    map[string]Dependency `json:"checks"`
}

func Handler(component string, checks map[string]Check) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		response := Response{
			Status:    "ok",
			Component: component,
			BuildSHA:  buildinfo.SHA,
			Version:   buildinfo.Version,
			Checks:    make(map[string]Dependency, len(checks)),
		}
		status := 200

		for name, check := range checks {
			dependency := Dependency{Status: "ok"}
			if err := check(ctx); err != nil {
				dependency.Status = "error"
				dependency.Error = err.Error()
				response.Status = "degraded"
				status = 503
			}
			response.Checks[name] = dependency
		}

		c.JSON(status, response)
	}
}
