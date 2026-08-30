package environmentvariables

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
	"github.com/Jonath-z/ship/server/migrations"
)

const maxValueBytes = 64 * 1024

var (
	ErrEnvironmentNotFound = errors.New("environment was not found")
	ErrServiceNotFound     = errors.New("service was not found in this environment")
	ErrVariableNotFound    = errors.New("environment variable was not found")
	ErrSecretNotFound      = errors.New("secret was not found")
	ErrNameExists          = errors.New("configuration name already exists in this scope")
	variableNamePattern    = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

type VariableResource struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environmentId"`
	ServiceID     *string   `json:"serviceId,omitempty"`
	Name          string    `json:"name"`
	Value         string    `json:"value"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type SecretResource struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environmentId"`
	ServiceID     *string   `json:"serviceId,omitempty"`
	Name          string    `json:"name"`
	HasValue      bool      `json:"hasValue"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type VariablePage struct {
	Items      []VariableResource `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

type SecretPage struct {
	Items      []SecretResource `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type CreateVariableInput struct {
	ServiceID *string
	Name      string
	Value     string
}

type UpdateVariableInput struct {
	Name  *string
	Value *string
}

type CreateSecretInput struct {
	ServiceID *string
	Name      string
	Value     string
}

type UpdateSecretInput struct {
	Name  *string
	Value *string
}

type ImportInput struct {
	ServiceID *string
	Content   string
}

type ImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
}

type RequestContext struct {
	Actor     access.Principal
	SourceIP  string
	RequestID string
}

type FieldViolation struct {
	Field   string
	Code    string
	Message string
}

type ValidationError struct {
	Fields []FieldViolation
}

func (*ValidationError) Error() string { return "configuration value validation failed" }

type Service struct {
	repository *Repository
	vault      *shipcrypto.Vault
	audit      audit.Recorder
}

func NewService(repository *Repository, vault *shipcrypto.Vault, recorder audit.Recorder) *Service {
	return &Service{repository: repository, vault: vault, audit: recorder}
}

func (service *Service) ListVariables(ctx context.Context, projectID, environmentID, cursor string, limit int) (VariablePage, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return VariablePage{}, err
	}
	rows, nextCursor, err := service.repository.ListVariables(ctx, environmentID, cursor, limit)
	if errors.Is(err, pagecursor.ErrInvalid) {
		return VariablePage{}, pagecursor.ErrInvalid
	}
	if err != nil {
		return VariablePage{}, err
	}
	page := VariablePage{Items: make([]VariableResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, variableResponse(row))
	}
	return page, nil
}

func (service *Service) GetVariable(ctx context.Context, projectID, environmentID, variableID string) (VariableResource, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return VariableResource{}, err
	}
	if !validID(variableID) {
		return VariableResource{}, ErrVariableNotFound
	}
	row, err := service.repository.FindVariable(ctx, environmentID, variableID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return VariableResource{}, ErrVariableNotFound
	}
	if err != nil {
		return VariableResource{}, fmt.Errorf("get environment variable: %w", err)
	}
	return variableResponse(row), nil
}

func (service *Service) CreateVariable(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateVariableInput) (VariableResource, error) {
	input.Name = strings.TrimSpace(input.Name)
	if validationError := validateNameAndValue(input.Name, input.Value, false); validationError != nil {
		return VariableResource{}, validationError
	}
	if err := service.requireScope(ctx, projectID, environmentID, input.ServiceID); err != nil {
		return VariableResource{}, err
	}
	exists, err := service.repository.NameExists(ctx, environmentID, input.ServiceID, input.Name, "")
	if err != nil {
		return VariableResource{}, fmt.Errorf("check environment variable name: %w", err)
	}
	if exists {
		return VariableResource{}, ErrNameExists
	}
	id, err := identity.New()
	if err != nil {
		return VariableResource{}, err
	}
	row := migrations.EnvironmentVariable{
		ID: id, EnvironmentID: environmentID, ServiceID: input.ServiceID, Name: input.Name, Value: input.Value,
	}
	if err := service.repository.CreateVariable(ctx, &row); err != nil {
		if uniqueViolation(err) {
			return VariableResource{}, ErrNameExists
		}
		return VariableResource{}, fmt.Errorf("create environment variable: %w", err)
	}
	service.record(ctx, requestContext, "environment_variable.created", "environment_variable", row.ID, environmentID, row.ServiceID, row.Name, nil)
	return variableResponse(row), nil
}

func (service *Service) UpdateVariable(ctx context.Context, requestContext RequestContext, projectID, environmentID, variableID string, input UpdateVariableInput) (VariableResource, error) {
	values, validationError := validateUpdate(input.Name, input.Value, false)
	if validationError != nil {
		return VariableResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return VariableResource{}, err
	}
	if !validID(variableID) {
		return VariableResource{}, ErrVariableNotFound
	}
	row, err := service.repository.FindVariable(ctx, environmentID, variableID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return VariableResource{}, ErrVariableNotFound
	}
	if err != nil {
		return VariableResource{}, fmt.Errorf("find environment variable: %w", err)
	}
	if name, ok := values["name"].(string); ok {
		exists, checkErr := service.repository.NameExists(ctx, environmentID, row.ServiceID, name, row.ID)
		if checkErr != nil {
			return VariableResource{}, fmt.Errorf("check environment variable name: %w", checkErr)
		}
		if exists {
			return VariableResource{}, ErrNameExists
		}
	}
	if err := service.repository.UpdateVariable(ctx, &row, values); err != nil {
		if uniqueViolation(err) {
			return VariableResource{}, ErrNameExists
		}
		return VariableResource{}, fmt.Errorf("update environment variable: %w", err)
	}
	service.record(ctx, requestContext, "environment_variable.updated", "environment_variable", row.ID, environmentID, row.ServiceID, row.Name, nil)
	return variableResponse(row), nil
}

func (service *Service) DeleteVariable(ctx context.Context, requestContext RequestContext, projectID, environmentID, variableID string) error {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	if !validID(variableID) {
		return ErrVariableNotFound
	}
	row, err := service.repository.FindVariable(ctx, environmentID, variableID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrVariableNotFound
	}
	if err != nil {
		return fmt.Errorf("find environment variable: %w", err)
	}
	if err := service.repository.DeleteVariable(ctx, &row); err != nil {
		return fmt.Errorf("delete environment variable: %w", err)
	}
	service.record(ctx, requestContext, "environment_variable.deleted", "environment_variable", row.ID, environmentID, row.ServiceID, row.Name, nil)
	return nil
}

func (service *Service) ImportVariables(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input ImportInput) (ImportResult, error) {
	entries, err := parseAndValidateImport(input.Content, false)
	if err != nil {
		return ImportResult{}, err
	}
	if err := service.requireScope(ctx, projectID, environmentID, input.ServiceID); err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{}
	err = service.repository.Transaction(ctx, func(repository *Repository) error {
		for _, entry := range entries {
			row, findErr := repository.FindVariableByName(ctx, environmentID, input.ServiceID, entry.Name)
			switch {
			case findErr == nil:
				if updateErr := repository.UpdateVariable(ctx, &row, map[string]any{"value": entry.Value}); updateErr != nil {
					return updateErr
				}
				result.Updated++
			case !errors.Is(findErr, gorm.ErrRecordNotFound):
				return findErr
			default:
				exists, checkErr := repository.NameExists(ctx, environmentID, input.ServiceID, entry.Name, "")
				if checkErr != nil {
					return checkErr
				}
				if exists {
					return ErrNameExists
				}
				id, idErr := identity.New()
				if idErr != nil {
					return idErr
				}
				row = migrations.EnvironmentVariable{ID: id, EnvironmentID: environmentID, ServiceID: input.ServiceID, Name: entry.Name, Value: entry.Value}
				if createErr := repository.CreateVariable(ctx, &row); createErr != nil {
					return createErr
				}
				result.Created++
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNameExists) || uniqueViolation(err) {
			return ImportResult{}, ErrNameExists
		}
		return ImportResult{}, fmt.Errorf("import environment variables: %w", err)
	}
	service.record(ctx, requestContext, "environment_variable.imported", "environment", environmentID, environmentID, input.ServiceID, "", map[string]any{"created": result.Created, "updated": result.Updated})
	return result, nil
}

func (service *Service) ListSecrets(ctx context.Context, projectID, environmentID, cursor string, limit int) (SecretPage, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return SecretPage{}, err
	}
	rows, nextCursor, err := service.repository.ListSecrets(ctx, environmentID, cursor, limit)
	if errors.Is(err, pagecursor.ErrInvalid) {
		return SecretPage{}, pagecursor.ErrInvalid
	}
	if err != nil {
		return SecretPage{}, err
	}
	page := SecretPage{Items: make([]SecretResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, secretResponse(row))
	}
	return page, nil
}

func (service *Service) GetSecret(ctx context.Context, projectID, environmentID, secretID string) (SecretResource, error) {
	row, err := service.findSecret(ctx, projectID, environmentID, secretID)
	if err != nil {
		return SecretResource{}, err
	}
	return secretResponse(row), nil
}

func (service *Service) CreateSecret(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateSecretInput) (SecretResource, error) {
	input.Name = strings.TrimSpace(input.Name)
	if validationError := validateNameAndValue(input.Name, input.Value, true); validationError != nil {
		return SecretResource{}, validationError
	}
	if err := service.requireScope(ctx, projectID, environmentID, input.ServiceID); err != nil {
		return SecretResource{}, err
	}
	var row migrations.Secret
	err := service.repository.Transaction(ctx, func(repository *Repository) error {
		exists, checkErr := repository.NameExists(ctx, environmentID, input.ServiceID, input.Name, "")
		if checkErr != nil {
			return checkErr
		}
		if exists {
			return ErrNameExists
		}
		id, idErr := identity.New()
		if idErr != nil {
			return idErr
		}
		row = migrations.Secret{ID: id, EnvironmentID: environmentID, ServiceID: input.ServiceID, Name: input.Name}
		if createErr := repository.CreateSecret(ctx, &row); createErr != nil {
			return createErr
		}
		scopeType, scopeID := secretScope(environmentID, input.ServiceID)
		_, storeErr := service.vault.WithDB(repository.db).Store(ctx, shipcrypto.StoreInput{
			SecretID: &row.ID, Kind: shipcrypto.KindApplicationSecret,
			ScopeType: scopeType, ScopeID: scopeID, Name: row.Name, Plaintext: []byte(input.Value),
		})
		return storeErr
	})
	if err != nil {
		if errors.Is(err, ErrNameExists) || uniqueViolation(err) {
			return SecretResource{}, ErrNameExists
		}
		return SecretResource{}, fmt.Errorf("create secret: %w", err)
	}
	service.record(ctx, requestContext, "secret.created", "secret", row.ID, environmentID, row.ServiceID, row.Name, nil)
	return secretResponse(row), nil
}

func (service *Service) UpdateSecret(ctx context.Context, requestContext RequestContext, projectID, environmentID, secretID string, input UpdateSecretInput) (SecretResource, error) {
	values, validationError := validateUpdate(input.Name, input.Value, true)
	if validationError != nil {
		return SecretResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return SecretResource{}, err
	}
	if !validID(secretID) {
		return SecretResource{}, ErrSecretNotFound
	}
	var row migrations.Secret
	err := service.repository.Transaction(ctx, func(repository *Repository) error {
		var findErr error
		row, findErr = repository.FindSecret(ctx, environmentID, secretID)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrSecretNotFound
		}
		if findErr != nil {
			return findErr
		}
		vaultEntry, findErr := repository.VaultEntry(ctx, row.ID)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return shipcrypto.ErrVaultEntryNotFound
		}
		if findErr != nil {
			return findErr
		}
		if name, ok := values["name"].(string); ok {
			exists, checkErr := repository.NameExists(ctx, environmentID, row.ServiceID, name, row.ID)
			if checkErr != nil {
				return checkErr
			}
			if exists {
				return ErrNameExists
			}
			if updateErr := repository.UpdateSecret(ctx, &row, map[string]any{"name": name}); updateErr != nil {
				return updateErr
			}
			if updateErr := repository.UpdateVaultName(ctx, vaultEntry.ID, name); updateErr != nil {
				return updateErr
			}
		}
		if input.Value != nil {
			if replaceErr := service.vault.WithDB(repository.db).Replace(ctx, vaultEntry.ID, []byte(*input.Value)); replaceErr != nil {
				return replaceErr
			}
			if _, renamed := values["name"]; !renamed {
				if updateErr := repository.UpdateSecret(ctx, &row, map[string]any{}); updateErr != nil {
					return updateErr
				}
			}
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrSecretNotFound):
			return SecretResource{}, ErrSecretNotFound
		case errors.Is(err, ErrNameExists), uniqueViolation(err):
			return SecretResource{}, ErrNameExists
		default:
			return SecretResource{}, fmt.Errorf("update secret: %w", err)
		}
	}
	service.record(ctx, requestContext, "secret.updated", "secret", row.ID, environmentID, row.ServiceID, row.Name, nil)
	return secretResponse(row), nil
}

func (service *Service) RevealSecret(ctx context.Context, requestContext RequestContext, projectID, environmentID, secretID string) (string, error) {
	row, err := service.findSecret(ctx, projectID, environmentID, secretID)
	if err != nil {
		return "", err
	}
	vaultEntry, err := service.repository.VaultEntry(ctx, row.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", shipcrypto.ErrVaultEntryNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find encrypted secret value: %w", err)
	}
	plaintext, err := service.vault.Reveal(ctx, vaultEntry.ID)
	if err != nil {
		service.recordOutcome(ctx, requestContext, "secret.reveal_failed", "secret", row.ID, environmentID, row.ServiceID, row.Name, audit.OutcomeFailure, nil)
		return "", fmt.Errorf("reveal secret: %w", err)
	}
	if err := service.recordRequired(ctx, requestContext, "secret.revealed", "secret", row.ID, environmentID, row.ServiceID, row.Name, audit.OutcomeSuccess, nil); err != nil {
		clear(plaintext)
		return "", fmt.Errorf("audit secret reveal: %w", err)
	}
	value := string(plaintext)
	clear(plaintext)
	return value, nil
}

func (service *Service) DeleteSecret(ctx context.Context, requestContext RequestContext, projectID, environmentID, secretID string) error {
	row, err := service.findSecret(ctx, projectID, environmentID, secretID)
	if err != nil {
		return err
	}
	if err := service.repository.DeleteSecret(ctx, &row); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	service.record(ctx, requestContext, "secret.deleted", "secret", row.ID, environmentID, row.ServiceID, row.Name, nil)
	return nil
}

func (service *Service) ImportSecrets(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input ImportInput) (ImportResult, error) {
	entries, err := parseAndValidateImport(input.Content, true)
	if err != nil {
		return ImportResult{}, err
	}
	if err := service.requireScope(ctx, projectID, environmentID, input.ServiceID); err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{}
	err = service.repository.Transaction(ctx, func(repository *Repository) error {
		vault := service.vault.WithDB(repository.db)
		for _, entry := range entries {
			row, findErr := repository.FindSecretByName(ctx, environmentID, input.ServiceID, entry.Name)
			switch {
			case findErr == nil:
				vaultEntry, entryErr := repository.VaultEntry(ctx, row.ID)
				if entryErr != nil {
					return entryErr
				}
				if replaceErr := vault.Replace(ctx, vaultEntry.ID, []byte(entry.Value)); replaceErr != nil {
					return replaceErr
				}
				if updateErr := repository.UpdateSecret(ctx, &row, map[string]any{}); updateErr != nil {
					return updateErr
				}
				result.Updated++
			case !errors.Is(findErr, gorm.ErrRecordNotFound):
				return findErr
			default:
				exists, checkErr := repository.NameExists(ctx, environmentID, input.ServiceID, entry.Name, "")
				if checkErr != nil {
					return checkErr
				}
				if exists {
					return ErrNameExists
				}
				id, idErr := identity.New()
				if idErr != nil {
					return idErr
				}
				row = migrations.Secret{ID: id, EnvironmentID: environmentID, ServiceID: input.ServiceID, Name: entry.Name}
				if createErr := repository.CreateSecret(ctx, &row); createErr != nil {
					return createErr
				}
				scopeType, scopeID := secretScope(environmentID, input.ServiceID)
				if _, storeErr := vault.Store(ctx, shipcrypto.StoreInput{
					SecretID: &row.ID, Kind: shipcrypto.KindApplicationSecret,
					ScopeType: scopeType, ScopeID: scopeID, Name: row.Name, Plaintext: []byte(entry.Value),
				}); storeErr != nil {
					return storeErr
				}
				result.Created++
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNameExists) || uniqueViolation(err) {
			return ImportResult{}, ErrNameExists
		}
		return ImportResult{}, fmt.Errorf("import secrets: %w", err)
	}
	service.record(ctx, requestContext, "secret.imported", "environment", environmentID, environmentID, input.ServiceID, "", map[string]any{"created": result.Created, "updated": result.Updated})
	return result, nil
}

// ProvisionPostgresConnection generates the initial environment-scoped
// DATABASE_URL when a PostgreSQL accessory is created.
func (service *Service) ProvisionPostgresConnection(ctx context.Context, actor access.Principal, sourceIP, requestID, projectID, environmentID, accessoryName string, port int) (string, error) {
	passwordBytes := make([]byte, 24)
	if _, err := cryptorand.Read(passwordBytes); err != nil {
		return "", fmt.Errorf("generate PostgreSQL password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(passwordBytes)
	clear(passwordBytes)
	if port < 1 || port > 65535 {
		port = 5432
	}
	databaseName := postgresIdentifier(accessoryName)
	host := postgresHostname(accessoryName)
	connectionURL := &url.URL{
		Scheme: "postgres", User: url.UserPassword("ship", password),
		Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + databaseName,
	}
	query := connectionURL.Query()
	query.Set("sslmode", "disable")
	connectionURL.RawQuery = query.Encode()
	names := []string{"DATABASE_URL", strings.ToUpper(databaseName) + "_DATABASE_URL"}
	for _, name := range names {
		_, err := service.CreateSecret(ctx, RequestContext{Actor: actor, SourceIP: sourceIP, RequestID: requestID}, projectID, environmentID, CreateSecretInput{
			Name: name, Value: connectionURL.String(),
		})
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, ErrNameExists) {
			return "", err
		}
	}
	return "", ErrNameExists
}

func (service *Service) findSecret(ctx context.Context, projectID, environmentID, secretID string) (migrations.Secret, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return migrations.Secret{}, err
	}
	if !validID(secretID) {
		return migrations.Secret{}, ErrSecretNotFound
	}
	row, err := service.repository.FindSecret(ctx, environmentID, secretID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return migrations.Secret{}, ErrSecretNotFound
	}
	if err != nil {
		return migrations.Secret{}, fmt.Errorf("find secret: %w", err)
	}
	return row, nil
}

func (service *Service) requireScope(ctx context.Context, projectID, environmentID string, serviceID *string) error {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	if serviceID == nil {
		return nil
	}
	if !validID(*serviceID) {
		return &ValidationError{Fields: []FieldViolation{{Field: "serviceId", Code: "invalid", Message: "must be a UUID"}}}
	}
	exists, err := service.repository.ServiceExists(ctx, environmentID, *serviceID)
	if err != nil {
		return fmt.Errorf("find configuration service: %w", err)
	}
	if !exists {
		return ErrServiceNotFound
	}
	return nil
}

func (service *Service) requireEnvironment(ctx context.Context, projectID, environmentID string) error {
	if !validID(projectID) || !validID(environmentID) {
		return ErrEnvironmentNotFound
	}
	exists, err := service.repository.EnvironmentExists(ctx, projectID, environmentID)
	if err != nil {
		return fmt.Errorf("find environment: %w", err)
	}
	if !exists {
		return ErrEnvironmentNotFound
	}
	return nil
}

func validateNameAndValue(name, value string, secret bool) *ValidationError {
	violations := make([]FieldViolation, 0, 2)
	if len(name) == 0 || len(name) > 128 || !variableNamePattern.MatchString(name) {
		violations = append(violations, FieldViolation{Field: "name", Code: "invalid", Message: "must match [A-Z_][A-Z0-9_]* and be at most 128 characters"})
	}
	if len(value) > maxValueBytes || !utf8.ValidString(value) {
		violations = append(violations, FieldViolation{Field: "value", Code: "invalid", Message: "must be valid UTF-8 and at most 64 KiB"})
	} else if secret && value == "" {
		violations = append(violations, FieldViolation{Field: "value", Code: "required", Message: "secret value is required"})
	}
	if len(violations) == 0 {
		return nil
	}
	return &ValidationError{Fields: violations}
}

func validateUpdate(name, value *string, secret bool) (map[string]any, *ValidationError) {
	if name == nil && value == nil {
		return nil, &ValidationError{Fields: []FieldViolation{{Field: "body", Code: "required", Message: "provide at least one field"}}}
	}
	values := make(map[string]any, 2)
	violations := make([]FieldViolation, 0, 2)
	if name != nil {
		normalized := strings.TrimSpace(*name)
		if validationError := validateNameAndValue(normalized, "placeholder", false); validationError != nil {
			violations = append(violations, validationError.Fields[0])
		} else {
			values["name"] = normalized
		}
	}
	if value != nil {
		if validationError := validateNameAndValue("VALID_NAME", *value, secret); validationError != nil {
			for _, violation := range validationError.Fields {
				if violation.Field == "value" {
					violations = append(violations, violation)
				}
			}
		}
		values["value"] = *value
	}
	if len(violations) > 0 {
		return nil, &ValidationError{Fields: violations}
	}
	return values, nil
}

func parseAndValidateImport(content string, secret bool) ([]dotenvEntry, error) {
	entries, err := parseDotenv(content)
	if err != nil {
		return nil, &ValidationError{Fields: []FieldViolation{{Field: "content", Code: "invalid", Message: err.Error()}}}
	}
	violations := make([]FieldViolation, 0)
	for _, entry := range entries {
		if validationError := validateNameAndValue(strings.TrimSpace(entry.Name), entry.Value, secret); validationError != nil {
			for _, violation := range validationError.Fields {
				violation.Field = "content." + entry.Name
				violations = append(violations, violation)
			}
		}
	}
	if len(violations) > 0 {
		return nil, &ValidationError{Fields: violations}
	}
	return entries, nil
}

func variableResponse(row migrations.EnvironmentVariable) VariableResource {
	return VariableResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, ServiceID: row.ServiceID,
		Name: row.Name, Value: row.Value, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func secretResponse(row migrations.Secret) SecretResource {
	return SecretResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, ServiceID: row.ServiceID,
		Name: row.Name, HasValue: true, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func secretScope(environmentID string, serviceID *string) (string, string) {
	if serviceID == nil {
		return "environment", environmentID
	}
	return "service", *serviceID
}

func postgresIdentifier(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			builder.WriteRune(character)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		result = "ship"
	}
	if len(result) > 63 {
		result = result[:63]
	}
	return result
}

func postgresHostname(value string) string {
	return strings.ReplaceAll(postgresIdentifier(value), "_", "-")
}

func validID(value string) bool { return uuid.Validate(value) == nil }

func uniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action, resourceType, resourceID, environmentID string, serviceID *string, name string, metadata map[string]any) {
	service.recordOutcome(ctx, requestContext, action, resourceType, resourceID, environmentID, serviceID, name, audit.OutcomeSuccess, metadata)
}

func (service *Service) recordOutcome(ctx context.Context, requestContext RequestContext, action, resourceType, resourceID, environmentID string, serviceID *string, name, outcome string, metadata map[string]any) {
	_ = service.recordRequired(ctx, requestContext, action, resourceType, resourceID, environmentID, serviceID, name, outcome, metadata)
}

func (service *Service) recordRequired(ctx context.Context, requestContext RequestContext, action, resourceType, resourceID, environmentID string, serviceID *string, name, outcome string, metadata map[string]any) error {
	if service.audit == nil {
		return nil
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["environmentId"] = environmentID
	if serviceID != nil {
		metadata["serviceId"] = *serviceID
	}
	if name != "" {
		metadata["name"] = name
	}
	return service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: resourceType, ResourceID: resourceID, Outcome: outcome,
		SourceIP: requestContext.SourceIP, RequestID: requestContext.RequestID, Metadata: metadata,
	})
}
