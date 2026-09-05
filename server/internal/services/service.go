package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrEnvironmentNotFound = errors.New("environment was not found")
	ErrServiceNotFound     = errors.New("service was not found")
	ErrNameExists          = errors.New("service name already exists in this environment")
	rolePattern            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	typePattern            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// DependentsError blocks deletion while other services still depend on the
// target; remove the dependency edges first.
type DependentsError struct {
	Names []string
}

func (*DependentsError) Error() string { return "other services depend on this service" }

type ServiceResource struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environmentId"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Repository    string    `json:"repository,omitempty"`
	Branch        string    `json:"branch,omitempty"`
	Image         string    `json:"image,omitempty"`
	Port          *int      `json:"port,omitempty"`
	Command       string    `json:"command,omitempty"`
	Role          string    `json:"role"`
	ServerGroupID string    `json:"serverGroupId"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Page struct {
	Items      []ServiceResource `json:"items"`
	NextCursor string            `json:"nextCursor,omitempty"`
}

type CreateInput struct {
	Name       string
	Type       string
	Repository string
	Branch     string
	Image      string
	Port       *int
	Command    string
	Role       string
}

type UpdateInput struct {
	Name       *string
	Type       *string
	Repository *string
	Branch     *string
	Image      *string
	PortSet    bool
	Port       *int
	Command    *string
	Role       *string
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

func (*ValidationError) Error() string {
	return "service validation failed"
}

type Service struct {
	repository *Repository
	audit      audit.Recorder
}

func NewService(repository *Repository, recorder audit.Recorder) *Service {
	return &Service{repository: repository, audit: recorder}
}

func (service *Service) List(ctx context.Context, projectID, environmentID, cursor string, limit int) (Page, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if !validID(projectID) || !validID(environmentID) {
		return Page{}, ErrEnvironmentNotFound
	}
	exists, err := service.repository.EnvironmentExists(ctx, projectID, environmentID)
	if err != nil {
		return Page{}, fmt.Errorf("find environment: %w", err)
	}
	if !exists {
		return Page{}, ErrEnvironmentNotFound
	}
	rows, nextCursor, err := service.repository.List(ctx, environmentID, cursor, limit)
	if errors.Is(err, pagecursor.ErrInvalid) {
		return Page{}, pagecursor.ErrInvalid
	}
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]ServiceResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, response(row))
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, projectID, environmentID, serviceID string) (ServiceResource, error) {
	if !validID(projectID) || !validID(environmentID) || !validID(serviceID) {
		return ServiceResource{}, ErrServiceNotFound
	}
	exists, err := service.repository.EnvironmentExists(ctx, projectID, environmentID)
	if err != nil {
		return ServiceResource{}, fmt.Errorf("find environment: %w", err)
	}
	if !exists {
		return ServiceResource{}, ErrEnvironmentNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, serviceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ServiceResource{}, ErrServiceNotFound
	}
	if err != nil {
		return ServiceResource{}, fmt.Errorf("get service: %w", err)
	}
	return response(row), nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateInput) (ServiceResource, error) {
	normalized, validationError := validateCreate(input)
	if validationError != nil {
		return ServiceResource{}, validationError
	}
	if !validID(projectID) || !validID(environmentID) {
		return ServiceResource{}, ErrEnvironmentNotFound
	}
	id, err := identity.New()
	if err != nil {
		return ServiceResource{}, err
	}
	var row migrations.Service
	err = service.repository.Transaction(ctx, func(repository *Repository) error {
		exists, findErr := repository.EnvironmentExists(ctx, projectID, environmentID)
		if findErr != nil {
			return findErr
		}
		if !exists {
			return ErrEnvironmentNotFound
		}
		group, roleErr := repository.EnsureRole(ctx, environmentID, normalized.Role)
		if roleErr != nil {
			return roleErr
		}
		row = migrations.Service{
			ID: id, EnvironmentID: environmentID, ServerGroupID: &group.ID,
			Name: normalized.Name, Type: normalized.Type, Repository: normalized.Repository,
			Branch: normalized.Branch, Image: normalized.Image, Port: normalized.Port,
			Command: normalized.Command, ServerGroup: &group,
		}
		return repository.Create(ctx, &row)
	})
	if err != nil {
		if errors.Is(err, ErrEnvironmentNotFound) {
			return ServiceResource{}, ErrEnvironmentNotFound
		}
		if uniqueViolation(err) {
			return ServiceResource{}, ErrNameExists
		}
		return ServiceResource{}, fmt.Errorf("create service: %w", err)
	}
	service.record(ctx, requestContext, "service.created", row, nil)
	return response(row), nil
}

func (service *Service) Update(ctx context.Context, requestContext RequestContext, projectID, environmentID, serviceID string, input UpdateInput) (ServiceResource, error) {
	if validationError := validateUpdateShape(input); validationError != nil {
		return ServiceResource{}, validationError
	}
	if !validID(projectID) || !validID(environmentID) || !validID(serviceID) {
		return ServiceResource{}, ErrServiceNotFound
	}
	var updated migrations.Service
	err := service.repository.Transaction(ctx, func(repository *Repository) error {
		exists, findErr := repository.EnvironmentExists(ctx, projectID, environmentID)
		if findErr != nil {
			return findErr
		}
		if !exists {
			return ErrEnvironmentNotFound
		}
		row, findErr := repository.FindForUpdate(ctx, environmentID, serviceID)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrServiceNotFound
		}
		if findErr != nil {
			return findErr
		}
		values, violations := applyUpdate(row, input)
		if len(violations) > 0 {
			return &ValidationError{Fields: violations}
		}
		if input.Role != nil {
			group, roleErr := repository.EnsureRole(ctx, environmentID, strings.TrimSpace(*input.Role))
			if roleErr != nil {
				return roleErr
			}
			values["server_group_id"] = group.ID
		}
		if updateErr := repository.Update(ctx, &row, values); updateErr != nil {
			return updateErr
		}
		updated = row
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrEnvironmentNotFound):
			return ServiceResource{}, ErrEnvironmentNotFound
		case errors.Is(err, ErrServiceNotFound):
			return ServiceResource{}, ErrServiceNotFound
		case uniqueViolation(err):
			return ServiceResource{}, ErrNameExists
		default:
			var validationError *ValidationError
			if errors.As(err, &validationError) {
				return ServiceResource{}, validationError
			}
			return ServiceResource{}, fmt.Errorf("update service: %w", err)
		}
	}
	service.record(ctx, requestContext, "service.updated", updated, nil)
	return response(updated), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, projectID, environmentID, serviceID string) error {
	if !validID(projectID) || !validID(environmentID) || !validID(serviceID) {
		return ErrServiceNotFound
	}
	exists, err := service.repository.EnvironmentExists(ctx, projectID, environmentID)
	if err != nil {
		return fmt.Errorf("find environment: %w", err)
	}
	if !exists {
		return ErrEnvironmentNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, serviceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("find service: %w", err)
	}
	dependents, err := service.repository.DependentServiceNames(ctx, environmentID, serviceID)
	if err != nil {
		return fmt.Errorf("find dependent services: %w", err)
	}
	if len(dependents) > 0 {
		return &DependentsError{Names: dependents}
	}
	if err := service.repository.Delete(ctx, &row); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	service.record(ctx, requestContext, "service.deleted", row, nil)
	return nil
}

func validateCreate(input CreateInput) (CreateInput, *ValidationError) {
	input.Name = strings.TrimSpace(input.Name)
	input.Type = strings.TrimSpace(input.Type)
	input.Repository = strings.TrimSpace(input.Repository)
	input.Branch = strings.TrimSpace(input.Branch)
	input.Image = strings.TrimSpace(input.Image)
	input.Command = strings.TrimSpace(input.Command)
	input.Role = strings.TrimSpace(input.Role)
	if input.Role == "" {
		input.Role = "web"
	}
	if input.Repository != "" && input.Branch == "" {
		input.Branch = "main"
	}
	violations := validateValues(input.Name, input.Type, input.Repository, input.Branch, input.Image, input.Port, input.Command, input.Role)
	if len(violations) > 0 {
		return CreateInput{}, &ValidationError{Fields: violations}
	}
	return input, nil
}

func validateUpdateShape(input UpdateInput) *ValidationError {
	if input.Name == nil && input.Type == nil && input.Repository == nil && input.Branch == nil &&
		input.Image == nil && !input.PortSet && input.Command == nil && input.Role == nil {
		return &ValidationError{Fields: []FieldViolation{{Field: "body", Code: "required", Message: "provide at least one field"}}}
	}
	return nil
}

func applyUpdate(row migrations.Service, input UpdateInput) (map[string]any, []FieldViolation) {
	name, serviceType := row.Name, row.Type
	repository, branch, image := row.Repository, row.Branch, row.Image
	port, command, role := row.Port, row.Command, "web"
	if row.ServerGroup != nil {
		role = row.ServerGroup.Name
	}
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if input.Type != nil {
		serviceType = strings.TrimSpace(*input.Type)
	}
	if input.Repository != nil {
		repository = strings.TrimSpace(*input.Repository)
	}
	if input.Branch != nil {
		branch = strings.TrimSpace(*input.Branch)
	} else if input.Repository != nil {
		if repository == "" {
			branch = ""
		} else if branch == "" {
			branch = "main"
		}
	}
	if input.Image != nil {
		image = strings.TrimSpace(*input.Image)
	}
	if input.PortSet {
		port = input.Port
	}
	if input.Command != nil {
		command = strings.TrimSpace(*input.Command)
	}
	if input.Role != nil {
		role = strings.TrimSpace(*input.Role)
	}
	violations := validateValues(name, serviceType, repository, branch, image, port, command, role)
	if len(violations) > 0 {
		return nil, violations
	}
	values := make(map[string]any, 8)
	if input.Name != nil {
		values["name"] = name
	}
	if input.Type != nil {
		values["type"] = serviceType
	}
	if input.Repository != nil {
		values["repository"] = repository
		if input.Branch == nil {
			values["branch"] = branch
		}
	}
	if input.Branch != nil {
		values["branch"] = branch
	}
	if input.Image != nil {
		values["image"] = image
	}
	if input.PortSet {
		values["port"] = port
	}
	if input.Command != nil {
		values["command"] = command
	}
	return values, nil
}

func validateValues(name, serviceType, repository, branch, image string, port *int, command, role string) []FieldViolation {
	violations := make([]FieldViolation, 0, 4)
	if length := utf8.RuneCountInString(name); length == 0 || length > 100 {
		violations = append(violations, FieldViolation{Field: "name", Code: "invalid", Message: "must be between 1 and 100 characters"})
	}
	if serviceType == "" || len(serviceType) > 32 || !typePattern.MatchString(serviceType) {
		violations = append(violations, FieldViolation{Field: "type", Code: "invalid", Message: "must be lowercase letters, numbers, or single hyphens"})
	}
	if repository == "" && image == "" {
		violations = append(violations, FieldViolation{Field: "repository", Code: "required", Message: "provide a repository or image"})
	}
	if len(repository) > 2048 {
		violations = append(violations, FieldViolation{Field: "repository", Code: "length", Message: "must be at most 2048 characters"})
	}
	if repository == "" && branch != "" {
		violations = append(violations, FieldViolation{Field: "branch", Code: "invalid", Message: "requires a repository"})
	}
	if len(branch) > 255 {
		violations = append(violations, FieldViolation{Field: "branch", Code: "length", Message: "must be at most 255 characters"})
	}
	if len(image) > 512 {
		violations = append(violations, FieldViolation{Field: "image", Code: "length", Message: "must be at most 512 characters"})
	}
	if port != nil && (*port < 1 || *port > 65535) {
		violations = append(violations, FieldViolation{Field: "port", Code: "range", Message: "must be between 1 and 65535"})
	}
	if len(command) > 4096 {
		violations = append(violations, FieldViolation{Field: "command", Code: "length", Message: "must be at most 4096 characters"})
	}
	if role == "" || len(role) > 63 || !rolePattern.MatchString(role) {
		violations = append(violations, FieldViolation{Field: "role", Code: "invalid", Message: "must be lowercase letters, numbers, or single hyphens"})
	}
	return violations
}

func response(row migrations.Service) ServiceResource {
	result := ServiceResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, Name: row.Name, Type: row.Type,
		Repository: row.Repository, Branch: row.Branch, Image: row.Image, Port: row.Port,
		Command: row.Command, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
	if row.ServerGroupID != nil {
		result.ServerGroupID = *row.ServerGroupID
	}
	if row.ServerGroup != nil {
		result.Role = row.ServerGroup.Name
	}
	return result
}

func validID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func uniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Service, metadata map[string]any) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "service", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP,
		RequestID: requestContext.RequestID, Metadata: metadata,
	})
}
