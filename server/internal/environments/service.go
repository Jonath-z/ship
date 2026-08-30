package environments

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
	ErrProjectNotFound     = errors.New("project was not found")
	ErrEnvironmentNotFound = errors.New("environment was not found")
	ErrSlugExists          = errors.New("environment slug already exists in this project")
	ErrConfirmationFailed  = errors.New("environment slug confirmation does not match")
	environmentSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type Environment struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Page struct {
	Items      []Environment `json:"items"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

type CreateInput struct {
	Name string
	Slug string
}

type UpdateInput struct {
	Name *string
	Slug *string
}

type CloneInput struct {
	Name           string
	Slug           string
	IncludeSecrets bool
}

type DeletionImpact struct {
	EnvironmentID        string `json:"environmentId"`
	Slug                 string `json:"slug"`
	ServerGroups         int64  `json:"serverGroups"`
	Services             int64  `json:"services"`
	Accessories          int64  `json:"accessories"`
	Volumes              int64  `json:"volumes"`
	Domains              int64  `json:"domains"`
	EnvironmentVariables int64  `json:"environmentVariables"`
	Secrets              int64  `json:"secrets"`
	Dependencies         int64  `json:"dependencies"`
	Configurations       int64  `json:"configurations"`
	Deployments          int64  `json:"deployments"`
	Backups              int64  `json:"backups"`
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

func (validationError *ValidationError) Error() string {
	return "environment validation failed"
}

type Service struct {
	repository *Repository
	audit      audit.Recorder
}

func NewService(repository *Repository, recorder audit.Recorder) *Service {
	return &Service{repository: repository, audit: recorder}
}

func (service *Service) List(ctx context.Context, projectID, cursor string, limit int) (Page, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if !validID(projectID) {
		return Page{}, ErrProjectNotFound
	}
	if _, err := service.repository.FindProject(ctx, projectID); errors.Is(err, gorm.ErrRecordNotFound) {
		return Page{}, ErrProjectNotFound
	} else if err != nil {
		return Page{}, fmt.Errorf("find project: %w", err)
	}
	rows, nextCursor, err := service.repository.List(ctx, projectID, cursor, limit)
	if errors.Is(err, pagecursor.ErrInvalid) {
		return Page{}, pagecursor.ErrInvalid
	}
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]Environment, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, environmentResponse(row))
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, projectID, environmentID string) (Environment, error) {
	if !validID(projectID) || !validID(environmentID) {
		return Environment{}, ErrEnvironmentNotFound
	}
	row, err := service.repository.Find(ctx, projectID, environmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Environment{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return Environment{}, fmt.Errorf("get environment: %w", err)
	}
	return environmentResponse(row), nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID string, input CreateInput) (Environment, error) {
	name, slug, validationError := validateCreate(input)
	if validationError != nil {
		return Environment{}, validationError
	}
	if !validID(projectID) {
		return Environment{}, ErrProjectNotFound
	}
	id, err := identity.New()
	if err != nil {
		return Environment{}, err
	}
	row := migrations.Environment{ID: id, ProjectID: projectID, Name: name, Slug: slug}
	err = service.repository.Transaction(ctx, func(repository *Repository) error {
		if _, findErr := repository.FindProject(ctx, projectID); errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrProjectNotFound
		} else if findErr != nil {
			return findErr
		}
		if createErr := repository.Create(ctx, &row); createErr != nil {
			return createErr
		}
		return repository.CreateConfiguration(ctx, row.ID)
	})
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return Environment{}, ErrProjectNotFound
		}
		if uniqueViolation(err) {
			return Environment{}, ErrSlugExists
		}
		return Environment{}, fmt.Errorf("create environment: %w", err)
	}
	service.record(ctx, requestContext, "environment.created", row, nil)
	return environmentResponse(row), nil
}

func (service *Service) Update(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input UpdateInput) (Environment, error) {
	values, validationError := validateUpdate(input)
	if validationError != nil {
		return Environment{}, validationError
	}
	if !validID(projectID) || !validID(environmentID) {
		return Environment{}, ErrEnvironmentNotFound
	}
	row, err := service.repository.Find(ctx, projectID, environmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Environment{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return Environment{}, fmt.Errorf("find environment: %w", err)
	}
	if err := service.repository.Update(ctx, &row, values); err != nil {
		if uniqueViolation(err) {
			return Environment{}, ErrSlugExists
		}
		return Environment{}, fmt.Errorf("update environment: %w", err)
	}
	service.record(ctx, requestContext, "environment.updated", row, map[string]any{"fields": updatedFields(values)})
	return environmentResponse(row), nil
}

func (service *Service) Clone(ctx context.Context, requestContext RequestContext, projectID, sourceEnvironmentID string, input CloneInput) (Environment, error) {
	name, slug, validationError := validateCreate(CreateInput{Name: input.Name, Slug: input.Slug})
	if input.IncludeSecrets {
		if validationError == nil {
			validationError = &ValidationError{}
		}
		validationError.Fields = append(validationError.Fields, FieldViolation{
			Field: "includeSecrets", Code: "unsupported",
			Message: "secret cloning is unavailable until encrypted values can be re-scoped safely",
		})
	}
	if validationError != nil {
		return Environment{}, validationError
	}
	if !validID(projectID) {
		return Environment{}, ErrProjectNotFound
	}
	if !validID(sourceEnvironmentID) {
		return Environment{}, ErrEnvironmentNotFound
	}
	id, err := identity.New()
	if err != nil {
		return Environment{}, err
	}
	target := migrations.Environment{ID: id, ProjectID: projectID, Name: name, Slug: slug}
	err = service.repository.Transaction(ctx, func(repository *Repository) error {
		if _, findErr := repository.FindProject(ctx, projectID); errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrProjectNotFound
		} else if findErr != nil {
			return findErr
		}
		if _, findErr := repository.FindForUpdate(ctx, projectID, sourceEnvironmentID); errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrEnvironmentNotFound
		} else if findErr != nil {
			return findErr
		}
		if createErr := repository.Create(ctx, &target); createErr != nil {
			return createErr
		}
		if createErr := repository.CreateConfiguration(ctx, target.ID); createErr != nil {
			return createErr
		}
		return repository.CopyDesiredState(ctx, sourceEnvironmentID, target.ID)
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrProjectNotFound):
			return Environment{}, ErrProjectNotFound
		case errors.Is(err, ErrEnvironmentNotFound):
			return Environment{}, ErrEnvironmentNotFound
		case uniqueViolation(err):
			return Environment{}, ErrSlugExists
		default:
			return Environment{}, fmt.Errorf("clone environment: %w", err)
		}
	}
	service.record(ctx, requestContext, "environment.cloned", target, map[string]any{
		"sourceEnvironmentId": sourceEnvironmentID, "secretsCopied": false,
	})
	return environmentResponse(target), nil
}

func (service *Service) DeletionImpact(ctx context.Context, projectID, environmentID string) (DeletionImpact, error) {
	if !validID(projectID) || !validID(environmentID) {
		return DeletionImpact{}, ErrEnvironmentNotFound
	}
	row, err := service.repository.Find(ctx, projectID, environmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeletionImpact{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return DeletionImpact{}, fmt.Errorf("find environment: %w", err)
	}
	impact, err := service.repository.DeletionImpact(ctx, row)
	if err != nil {
		return DeletionImpact{}, fmt.Errorf("calculate environment deletion impact: %w", err)
	}
	return impact, nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, projectID, environmentID, confirmedSlug string) error {
	confirmedSlug = strings.TrimSpace(confirmedSlug)
	if confirmedSlug == "" {
		return &ValidationError{Fields: []FieldViolation{{
			Field: "confirmSlug", Code: "required", Message: "enter the current environment slug",
		}}}
	}
	if !validID(projectID) || !validID(environmentID) {
		return ErrEnvironmentNotFound
	}
	var deleted migrations.Environment
	err := service.repository.Transaction(ctx, func(repository *Repository) error {
		row, findErr := repository.FindForUpdate(ctx, projectID, environmentID)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return ErrEnvironmentNotFound
		}
		if findErr != nil {
			return findErr
		}
		if confirmedSlug != row.Slug {
			return ErrConfirmationFailed
		}
		if deleteErr := repository.Delete(ctx, &row); deleteErr != nil {
			return deleteErr
		}
		deleted = row
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	service.record(ctx, requestContext, "environment.deleted", deleted, map[string]any{"slug": deleted.Slug})
	return nil
}

func validateCreate(input CreateInput) (string, string, *ValidationError) {
	name, nameViolation := validName(input.Name)
	slug := strings.TrimSpace(input.Slug)
	if slug == "" && nameViolation == nil {
		slug = slugFromName(name)
	}
	violations := make([]FieldViolation, 0, 2)
	if nameViolation != nil {
		violations = append(violations, *nameViolation)
	}
	if violation := validSlug(slug); violation != nil {
		violations = append(violations, *violation)
	}
	if len(violations) > 0 {
		return "", "", &ValidationError{Fields: violations}
	}
	return name, slug, nil
}

func validateUpdate(input UpdateInput) (map[string]any, *ValidationError) {
	values := make(map[string]any, 2)
	violations := make([]FieldViolation, 0, 2)
	if input.Name != nil {
		name, violation := validName(*input.Name)
		if violation != nil {
			violations = append(violations, *violation)
		} else {
			values["name"] = name
		}
	}
	if input.Slug != nil {
		slug := strings.TrimSpace(*input.Slug)
		if violation := validSlug(slug); violation != nil {
			violations = append(violations, *violation)
		} else {
			values["slug"] = slug
		}
	}
	if input.Name == nil && input.Slug == nil {
		violations = append(violations, FieldViolation{Field: "body", Code: "required", Message: "provide name or slug"})
	}
	if len(violations) > 0 {
		return nil, &ValidationError{Fields: violations}
	}
	return values, nil
}

func validName(value string) (string, *FieldViolation) {
	value = strings.TrimSpace(value)
	length := utf8.RuneCountInString(value)
	if length == 0 {
		return "", &FieldViolation{Field: "name", Code: "required", Message: "is required"}
	}
	if length > 100 {
		return "", &FieldViolation{Field: "name", Code: "length", Message: "must be at most 100 characters"}
	}
	return value, nil
}

func validSlug(value string) *FieldViolation {
	if value == "" {
		return &FieldViolation{Field: "slug", Code: "required", Message: "enter a lowercase ASCII slug"}
	}
	if len(value) > 63 || !environmentSlugPattern.MatchString(value) {
		return &FieldViolation{Field: "slug", Code: "invalid", Message: "must be 1–63 lowercase letters, numbers, or single hyphens"}
	}
	return nil
}

func slugFromName(value string) string {
	var slug strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			if separator && slug.Len() > 0 && slug.Len() < 63 {
				slug.WriteByte('-')
			}
			separator = false
			if slug.Len() < 63 {
				slug.WriteRune(character)
			}
		} else if slug.Len() > 0 {
			separator = true
		}
	}
	return strings.TrimRight(slug.String(), "-")
}

func environmentResponse(row migrations.Environment) Environment {
	return Environment{
		ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, Slug: row.Slug,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func uniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func validID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func updatedFields(values map[string]any) []string {
	fields := make([]string, 0, 2)
	for _, field := range []string{"name", "slug"} {
		if _, ok := values[field]; ok {
			fields = append(fields, field)
		}
	}
	return fields
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Environment, metadata map[string]any) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "environment", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP,
		RequestID: requestContext.RequestID, Metadata: metadata,
	})
}
