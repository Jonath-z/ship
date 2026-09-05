package servers

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
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	IPAddress string `json:"ipAddress"`
	SSHUser   string `json:"sshUser"`
	SSHPort   int    `json:"sshPort"`
	SSHKeyID  string `json:"sshKeyId"`
}

type updateRequest struct {
	Name      *string `json:"name"`
	Hostname  *string `json:"hostname"`
	IPAddress *string `json:"ipAddress"`
	SSHUser   *string `json:"sshUser"`
	SSHPort   *int    `json:"sshPort"`
	SSHKeyID  *string `json:"sshKeyId"`
}

type memberRequest struct {
	ServerID string `json:"serverId"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	router.GET("/servers", access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		servers, err := service.List(ctx)
		if err != nil {
			httpx.WriteError(c, 500, "servers_unavailable", "servers are unavailable", nil)
			return
		}
		c.JSON(200, gin.H{"items": servers})
	})

	router.POST("/servers", access.ServersManage, func(c *gin.Context) {
		var request createRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		server, err := service.Create(ctx, requestContext(c, cfg), CreateInput(request))
		writeMutationResult(c, server, err, 201)
	})

	router.GET("/servers/:serverId", access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		server, err := service.Get(ctx, c.Param("serverId"))
		switch {
		case errors.Is(err, ErrServerNotFound):
			httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "server_unavailable", "server is unavailable", nil)
		default:
			c.JSON(200, server)
		}
	})

	router.PATCH("/servers/:serverId", access.ServersManage, func(c *gin.Context) {
		var request updateRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		server, err := service.Update(ctx, requestContext(c, cfg), c.Param("serverId"), UpdateInput(request))
		writeMutationResult(c, server, err, 200)
	})

	router.DELETE("/servers/:serverId", access.ServersManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.Delete(ctx, requestContext(c, cfg), c.Param("serverId"))
		switch {
		case errors.Is(err, ErrServerNotFound):
			httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
		case err != nil:
			if writeDependentsError(c, err) {
				return
			}
			httpx.WriteError(c, 500, "server_delete_failed", "server could not be deleted", nil)
		default:
			c.Status(204)
		}
	})

	// Connection test (SH-043): re-runnable on demand.
	router.POST("/servers/:serverId/checks", access.ServersManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		report, err := service.RunChecks(ctx, requestContext(c, cfg), c.Param("serverId"))
		writeReportResult(c, report, err)
	})

	// Preparation (SH-044): installs Docker, then re-checks. Runs
	// synchronously with a generous timeout until the job system (E7) exists.
	router.POST("/servers/:serverId/prepare", access.ServersManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
		defer cancel()
		report, err := service.Prepare(ctx, requestContext(c, cfg), c.Param("serverId"))
		switch {
		case errors.Is(err, ErrServerNotFound):
			httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
		case errors.Is(err, ErrKeyNotFound):
			httpx.WriteError(c, 409, "ssh_key_missing", "server has no usable SSH key", nil)
		case err != nil:
			httpx.WriteError(c, 502, "server_prepare_failed", "server could not be prepared", nil)
		default:
			c.JSON(200, report)
		}
	})

	groupBase := "/projects/:projectId/environments/:environmentId/server-groups"
	router.GET(groupBase, access.ServersRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		groups, err := service.Groups(ctx, c.Param("projectId"), c.Param("environmentId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "server_groups_unavailable", "server groups are unavailable", nil)
		default:
			c.JSON(200, gin.H{"items": groups})
		}
	})

	router.POST(groupBase+"/:groupId/servers", access.ServersManage, func(c *gin.Context) {
		var request memberRequest
		if err := c.ShouldBindJSON(&request); err != nil {
			httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.AddMember(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("groupId"), request.ServerID)
		writeMembershipResult(c, err, 201)
	})

	router.DELETE(groupBase+"/:groupId/servers/:serverId", access.ServersManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.RemoveMember(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("groupId"), c.Param("serverId"))
		writeMembershipResult(c, err, 204)
	})
}

func writeMutationResult(c *gin.Context, server ServerResource, err error, status int) {
	switch {
	case errors.Is(err, ErrServerNotFound):
		httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
	case errors.Is(err, ErrKeyNotFound):
		httpx.WriteError(c, 404, "ssh_key_not_found", "SSH key was not found", nil)
	case errors.Is(err, ErrNameExists):
		httpx.WriteError(c, 409, "server_name_exists", "a server with this name already exists", nil)
	case err != nil:
		if writeValidationError(c, err) {
			return
		}
		httpx.WriteError(c, 500, "server_mutation_failed", "server could not be saved", nil)
	default:
		c.JSON(status, server)
	}
}

func writeReportResult(c *gin.Context, report CheckReport, err error) {
	switch {
	case errors.Is(err, ErrServerNotFound):
		httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
	case errors.Is(err, ErrKeyNotFound):
		httpx.WriteError(c, 409, "ssh_key_missing", "server has no usable SSH key", nil)
	case err != nil:
		httpx.WriteError(c, 502, "server_check_failed", "server checks could not run", nil)
	default:
		c.JSON(200, report)
	}
}

func writeMembershipResult(c *gin.Context, err error, status int) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrGroupNotFound):
		httpx.WriteError(c, 404, "server_group_not_found", "server group was not found", nil)
	case errors.Is(err, ErrServerNotFound):
		httpx.WriteError(c, 404, "server_not_found", "server was not found", nil)
	case errors.Is(err, ErrAlreadyMember):
		httpx.WriteError(c, 409, "server_already_member", "server is already in this group", nil)
	case errors.Is(err, ErrNotMember):
		httpx.WriteError(c, 404, "server_not_member", "server is not in this group", nil)
	case errors.Is(err, ErrLastMemberInUse):
		httpx.WriteError(c, 409, "server_group_in_use", "cannot remove the last server from a group that services or accessories target", nil)
	case err != nil:
		httpx.WriteError(c, 500, "server_group_mutation_failed", "server group could not be changed", nil)
	default:
		c.Status(status)
	}
}

func writeDependentsError(c *gin.Context, err error) bool {
	var dependentsError *DependentsError
	if !errors.As(err, &dependentsError) {
		return false
	}
	details := make([]httpx.FieldError, 0, len(dependentsError.Names))
	for _, name := range dependentsError.Names {
		details = append(details, httpx.FieldError{Field: "dependents", Code: "in_use", Message: name})
	}
	httpx.WriteError(c, 409, "server_in_use", "server is still used by services or accessories; move or delete them first", details)
	return true
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
