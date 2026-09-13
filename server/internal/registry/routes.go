package registry

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

// Repository names per the OCI distribution spec: lowercase path segments
// separated by slashes. Rejecting anything else keeps the name safe to embed
// in upstream URLs.
var repositoryPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)

// RegisterRoutes exposes read-only registry browsing for the image picker.
// Both endpoints use the environment's stored KAMAL_REGISTRY_* credentials;
// responses carry names and tags only, never credential material.
func RegisterRoutes(router *httpx.Router, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/registry"

	router.GET(base+"/repositories", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		names, err := service.Repositories(ctx, c.Param("projectId"), c.Param("environmentId"))
		if !writeBrowseError(c, err) {
			c.JSON(200, gin.H{"items": names})
		}
	})

	router.GET(base+"/tags", access.ConfigurationRead, func(c *gin.Context) {
		repository := c.Query("repository")
		if !repositoryPattern.MatchString(repository) {
			httpx.WriteError(c, 422, "validation_failed", "repository must be a valid registry path", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer cancel()
		tags, err := service.Tags(ctx, c.Param("projectId"), c.Param("environmentId"), repository)
		if !writeBrowseError(c, err) {
			c.JSON(200, gin.H{"items": tags})
		}
	})
}

func writeBrowseError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrNotConfigured):
		httpx.WriteError(c, 409, "registry_not_configured",
			"registry credentials are not configured for this environment", nil)
	default:
		// Upstream registry failures: surface the reason without credentials.
		httpx.WriteError(c, 502, "registry_unreachable", err.Error(), nil)
	}
	return true
}
