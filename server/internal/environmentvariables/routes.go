package environmentvariables

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
)

type createValueRequest struct {
	ServiceID *string `json:"serviceId"`
	Name      string  `json:"name"`
	Value     string  `json:"value"`
}

type updateValueRequest struct {
	Name  *string `json:"name"`
	Value *string `json:"value"`
}

type importRequest struct {
	ServiceID *string `json:"serviceId"`
	Content   string  `json:"content"`
}

func RegisterRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	registerVariableRoutes(router, cfg, service)
	registerSecretRoutes(router, cfg, service)
}

func registerVariableRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/environment-variables"
	router.GET(base, access.ConfigurationRead, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.ListVariables(ctx, c.Param("projectId"), c.Param("environmentId"), pagination.Cursor, pagination.Limit)
		writeVariableListResult(c, page, err)
	})

	router.POST(base, access.ConfigurationManage, func(c *gin.Context) {
		var request createValueRequest
		if !bindJSON(c, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		item, err := service.CreateVariable(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), CreateVariableInput{
			ServiceID: request.ServiceID, Name: request.Name, Value: request.Value,
		})
		writeVariableMutationResult(c, item, err, 201)
	})

	router.POST(base+"/import", access.ConfigurationManage, func(c *gin.Context) {
		var request importRequest
		if !bindJSON(c, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		result, err := service.ImportVariables(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), ImportInput(request))
		writeImportResult(c, result, err)
	})

	router.GET(base+"/:variableId", access.ConfigurationRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := service.GetVariable(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("variableId"))
		writeVariableReadResult(c, item, err)
	})

	router.PATCH(base+"/:variableId", access.ConfigurationManage, func(c *gin.Context) {
		var request updateValueRequest
		if !bindJSON(c, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		item, err := service.UpdateVariable(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("variableId"), UpdateVariableInput(request))
		writeVariableMutationResult(c, item, err, 200)
	})

	router.DELETE(base+"/:variableId", access.ConfigurationManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.DeleteVariable(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("variableId"))
		writeDeleteResult(c, err, ErrVariableNotFound, "environment_variable_not_found", "environment variable")
	})
}

func registerSecretRoutes(router *httpx.Router, cfg config.Config, service *Service) {
	base := "/projects/:projectId/environments/:environmentId/secrets"
	router.GET(base, access.SecretsReadMetadata, func(c *gin.Context) {
		pagination, ok := httpx.ParsePagination(c)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		page, err := service.ListSecrets(ctx, c.Param("projectId"), c.Param("environmentId"), pagination.Cursor, pagination.Limit)
		switch {
		case errors.Is(err, pagecursor.ErrInvalid):
			httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "secrets_unavailable", "secret metadata is unavailable", nil)
		default:
			c.JSON(200, page)
		}
	})

	router.POST(base, access.SecretsManage, func(c *gin.Context) {
		var request createValueRequest
		if !bindJSON(c, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		item, err := service.CreateSecret(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), CreateSecretInput{
			ServiceID: request.ServiceID, Name: request.Name, Value: request.Value,
		})
		writeSecretMutationResult(c, item, err, 201)
	})

	router.POST(base+"/import", access.SecretsManage, func(c *gin.Context) {
		var request importRequest
		if !bindJSON(c, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		result, err := service.ImportSecrets(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), ImportInput(request))
		writeImportResult(c, result, err)
	})

	router.GET(base+"/:secretId", access.SecretsReadMetadata, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		item, err := service.GetSecret(ctx, c.Param("projectId"), c.Param("environmentId"), c.Param("secretId"))
		writeSecretReadResult(c, item, err)
	})

	router.PATCH(base+"/:secretId", access.SecretsManage, func(c *gin.Context) {
		var request updateValueRequest
		if !bindJSON(c, &request) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		item, err := service.UpdateSecret(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("secretId"), UpdateSecretInput(request))
		writeSecretMutationResult(c, item, err, 200)
	})

	router.DELETE(base+"/:secretId", access.SecretsManage, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		err := service.DeleteSecret(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("secretId"))
		writeDeleteResult(c, err, ErrSecretNotFound, "secret_not_found", "secret")
	})

	router.POST(base+"/:secretId/reveal", access.SecretsReveal, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
		defer cancel()
		value, err := service.RevealSecret(ctx, requestContext(c, cfg), c.Param("projectId"), c.Param("environmentId"), c.Param("secretId"))
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
		case errors.Is(err, ErrSecretNotFound):
			httpx.WriteError(c, 404, "secret_not_found", "secret was not found", nil)
		case err != nil:
			httpx.WriteError(c, 500, "secret_reveal_failed", "secret could not be revealed", nil)
		default:
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")
			c.JSON(200, gin.H{"value": value})
		}
	})
}

func bindJSON(c *gin.Context, destination any) bool {
	if err := c.ShouldBindJSON(destination); err != nil {
		httpx.WriteError(c, 400, "invalid_request", "request body must be valid JSON", nil)
		return false
	}
	return true
}

func writeVariableListResult(c *gin.Context, page VariablePage, err error) {
	switch {
	case errors.Is(err, pagecursor.ErrInvalid):
		httpx.WriteError(c, 400, "invalid_cursor", "pagination cursor is invalid", nil)
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "environment_variables_unavailable", "environment variables are unavailable", nil)
	default:
		c.JSON(200, page)
	}
}

func writeVariableReadResult(c *gin.Context, item VariableResource, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrVariableNotFound):
		httpx.WriteError(c, 404, "environment_variable_not_found", "environment variable was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "environment_variable_unavailable", "environment variable is unavailable", nil)
	default:
		c.JSON(200, item)
	}
}

func writeVariableMutationResult(c *gin.Context, item VariableResource, err error, status int) {
	if writeCommonMutationError(c, err, ErrVariableNotFound, "environment_variable_not_found", "environment variable") {
		return
	}
	c.JSON(status, item)
}

func writeSecretReadResult(c *gin.Context, item SecretResource, err error) {
	switch {
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrSecretNotFound):
		httpx.WriteError(c, 404, "secret_not_found", "secret was not found", nil)
	case err != nil:
		httpx.WriteError(c, 500, "secret_unavailable", "secret metadata is unavailable", nil)
	default:
		c.JSON(200, item)
	}
}

func writeSecretMutationResult(c *gin.Context, item SecretResource, err error, status int) {
	if writeCommonMutationError(c, err, ErrSecretNotFound, "secret_not_found", "secret") {
		return
	}
	c.JSON(status, item)
}

func writeImportResult(c *gin.Context, result ImportResult, err error) {
	if writeCommonMutationError(c, err, nil, "", "configuration values") {
		return
	}
	c.JSON(200, result)
}

func writeCommonMutationError(c *gin.Context, err, notFound error, notFoundCode, resourceName string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrEnvironmentNotFound):
		httpx.WriteError(c, 404, "environment_not_found", "environment was not found", nil)
	case errors.Is(err, ErrServiceNotFound):
		httpx.WriteError(c, 404, "service_not_found", "service was not found in this environment", nil)
	case notFound != nil && errors.Is(err, notFound):
		httpx.WriteError(c, 404, notFoundCode, resourceName+" was not found", nil)
	case errors.Is(err, ErrNameExists):
		httpx.WriteError(c, 409, "configuration_name_exists", "this name already exists in the selected scope", nil)
	default:
		if writeValidationError(c, err) {
			return true
		}
		if errors.Is(err, shipcrypto.ErrVaultEntryNotFound) {
			httpx.WriteError(c, 500, "secret_value_missing", "encrypted secret value is missing", nil)
		} else {
			httpx.WriteError(c, 500, "configuration_mutation_failed", resourceName+" could not be saved", nil)
		}
	}
	return true
}

func writeDeleteResult(c *gin.Context, err, notFound error, notFoundCode, resourceName string) {
	if writeCommonMutationError(c, err, notFound, notFoundCode, resourceName) {
		return
	}
	c.Status(204)
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
