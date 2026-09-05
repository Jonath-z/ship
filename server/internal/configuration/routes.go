package configuration

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
)

type snapshotRequest struct {
	ChangeSummary string `json:"changeSummary"`
}

type previewResponse struct {
	EnvironmentID string            `json:"environmentId"`
	State         DesiredState      `json:"state"`
	Validation    []Violation       `json:"validation"`
	Rendered      map[string]string `json:"rendered"` // service name -> Kamal YAML, secrets by name only
}

type diffResponse struct {
	From     int          `json:"from"`
	To       int          `json:"to"`
	Entities []EntityDiff `json:"entities"`
}

// RegisterRoutes exposes the configuration engine read paths and the snapshot
// action (SH-053, SH-054, SH-057). Everything returned is redacted by
// construction: the model and rendered YAML never contain secret values.
func RegisterRoutes(router *httpx.Router, cfg config.Config, repository *Repository, recorder audit.Recorder) {
	base := "/projects/:projectId/environments/:environmentId/configuration"

	requireEnvironment := func(c *gin.Context, ctx context.Context) (string, bool) {
		projectID, environmentID := c.Param("projectId"), c.Param("environmentId")
		exists, err := repository.EnvironmentExists(ctx, projectID, environmentID)
		if err != nil || !exists {
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
			return "", false
		}
		return environmentID, true
	}

	router.GET(base+"/preview", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		environmentID, ok := requireEnvironment(c, ctx)
		if !ok {
			return
		}
		state, facts, err := repository.Compile(ctx, environmentID)
		if err != nil {
			httpx.WriteError(c, 500, "configuration_unavailable", "configuration could not be compiled", nil)
			return
		}
		rows, err := repository.Load(ctx, environmentID)
		if err != nil {
			httpx.WriteError(c, 500, "configuration_unavailable", "configuration could not be compiled", nil)
			return
		}
		rendered, err := Render(RenderInput{ProjectSlug: rows.ProjectSlug, EnvironmentSlug: rows.EnvironmentSlug}, state)
		if err != nil {
			httpx.WriteError(c, 500, "configuration_render_failed", "configuration could not be rendered", nil)
			return
		}
		response := previewResponse{
			EnvironmentID: environmentID, State: state,
			Validation: Validate(state, facts), Rendered: map[string]string{},
		}
		for name, document := range rendered {
			response.Rendered[name] = string(document)
		}
		c.JSON(200, response)
	})

	router.POST(base+"/versions", access.ConfigurationManage, func(c *gin.Context) {
		var request snapshotRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		environmentID, ok := requireEnvironment(c, ctx)
		if !ok {
			return
		}
		principal, _ := access.PrincipalFrom(c)
		var actorUserID *string
		if principal.UserID != "" {
			actorUserID = &principal.UserID
		}
		record, err := repository.Snapshot(ctx, environmentID, actorUserID, request.ChangeSummary)
		if err != nil {
			httpx.WriteError(c, 500, "configuration_snapshot_failed", "configuration version could not be created", nil)
			return
		}
		if recorder != nil {
			_ = recorder.Record(ctx, audit.Event{
				ActorUserID: principal.UserID, ActorEmail: principal.Email,
				Action: "configuration.version.created", ResourceType: "configuration", ResourceID: environmentID,
				Outcome:  audit.OutcomeSuccess,
				SourceIP: httpx.ClientIP(c, cfg.TrustForwardedIP), RequestID: c.GetString("requestID"),
				Metadata: map[string]any{"version": record.Version},
			})
		}
		c.JSON(201, record)
	})

	router.GET(base+"/versions", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		environmentID, ok := requireEnvironment(c, ctx)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		records, err := repository.Versions(ctx, environmentID, limit)
		if err != nil {
			httpx.WriteError(c, 500, "configuration_unavailable", "configuration versions are unavailable", nil)
			return
		}
		c.JSON(200, gin.H{"items": records})
	})

	router.GET(base+"/versions/:version", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		environmentID, ok := requireEnvironment(c, ctx)
		if !ok {
			return
		}
		version, ok := parseVersion(c)
		if !ok {
			return
		}
		record, err := repository.Version(ctx, environmentID, version)
		if errors.Is(err, ErrVersionNotFound) {
			httpx.WriteError(c, 404, "version_not_found", "configuration version was not found", nil)
			return
		}
		if err != nil {
			httpx.WriteError(c, 500, "configuration_unavailable", "configuration version is unavailable", nil)
			return
		}
		c.JSON(200, record)
	})

	// Diff of ?against=N (default: the previous version) onto :version.
	router.GET(base+"/versions/:version/diff", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		environmentID, ok := requireEnvironment(c, ctx)
		if !ok {
			return
		}
		version, ok := parseVersion(c)
		if !ok {
			return
		}
		record, err := repository.Version(ctx, environmentID, version)
		if errors.Is(err, ErrVersionNotFound) {
			httpx.WriteError(c, 404, "version_not_found", "configuration version was not found", nil)
			return
		}
		if err != nil {
			httpx.WriteError(c, 500, "configuration_unavailable", "configuration version is unavailable", nil)
			return
		}
		against := version - 1
		if raw := c.Query("against"); raw != "" {
			against, err = strconv.Atoi(raw)
			if err != nil || against < 0 {
				httpx.WriteError(c, 400, "invalid_version", "against must be a non-negative integer", nil)
				return
			}
		}
		before := DesiredState{Services: map[string]ServiceSpec{}, Accessories: map[string]Accessory{}, Roles: map[string][]string{}}
		if against > 0 {
			beforeRecord, err := repository.Version(ctx, environmentID, against)
			if errors.Is(err, ErrVersionNotFound) {
				httpx.WriteError(c, 404, "version_not_found", "configuration version was not found", nil)
				return
			}
			if err != nil {
				httpx.WriteError(c, 500, "configuration_unavailable", "configuration version is unavailable", nil)
				return
			}
			before = beforeRecord.State
		}
		c.JSON(200, diffResponse{From: against, To: version, Entities: Diff(before, record.State)})
	})
}

func parseVersion(c *gin.Context) (int, bool) {
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version < 1 {
		httpx.WriteError(c, 400, "invalid_version", "version must be a positive integer", nil)
		return 0, false
	}
	return version, true
}
