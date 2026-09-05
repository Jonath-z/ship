package dependencies

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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
	ErrSourceNotFound      = errors.New("source service was not found in this environment")
	ErrTargetNotFound      = errors.New("dependency target was not found in this environment")
	ErrDependencyNotFound  = errors.New("dependency was not found")
	ErrDependencyExists    = errors.New("this dependency already exists")
	ErrDependencyCycle     = errors.New("this dependency would create a cycle")
	typePattern            = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type DependencyResource struct {
	ID                string    `json:"id"`
	EnvironmentID     string    `json:"environmentId"`
	SourceServiceID   string    `json:"sourceServiceId"`
	TargetServiceID   *string   `json:"targetServiceId,omitempty"`
	TargetAccessoryID *string   `json:"targetAccessoryId,omitempty"`
	Type              string    `json:"type"`
	CreatedAt         time.Time `json:"createdAt"`
}

type Page struct {
	Items      []DependencyResource `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

type CreateInput struct {
	SourceServiceID   string
	TargetServiceID   *string
	TargetAccessoryID *string
	Type              string
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

func (*ValidationError) Error() string { return "dependency validation failed" }

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
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return Page{}, err
	}
	rows, nextCursor, err := service.repository.List(ctx, environmentID, cursor, limit)
	if errors.Is(err, pagecursor.ErrInvalid) {
		return Page{}, pagecursor.ErrInvalid
	}
	if err != nil {
		return Page{}, err
	}
	page := Page{Items: make([]DependencyResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, response(row))
	}
	return page, nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateInput) (DependencyResource, error) {
	input, validationError := validateCreate(input)
	if validationError != nil {
		return DependencyResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return DependencyResource{}, err
	}
	sourceExists, err := service.repository.ServiceExists(ctx, environmentID, input.SourceServiceID)
	if err != nil {
		return DependencyResource{}, fmt.Errorf("find source service: %w", err)
	}
	if !sourceExists {
		return DependencyResource{}, ErrSourceNotFound
	}
	if input.TargetServiceID != nil {
		targetExists, err := service.repository.ServiceExists(ctx, environmentID, *input.TargetServiceID)
		if err != nil {
			return DependencyResource{}, fmt.Errorf("find target service: %w", err)
		}
		if !targetExists {
			return DependencyResource{}, ErrTargetNotFound
		}
		if err := service.rejectCycle(ctx, environmentID, input.SourceServiceID, *input.TargetServiceID); err != nil {
			return DependencyResource{}, err
		}
	} else {
		targetExists, err := service.repository.AccessoryExists(ctx, environmentID, *input.TargetAccessoryID)
		if err != nil {
			return DependencyResource{}, fmt.Errorf("find target accessory: %w", err)
		}
		if !targetExists {
			return DependencyResource{}, ErrTargetNotFound
		}
	}

	id, err := identity.New()
	if err != nil {
		return DependencyResource{}, err
	}
	row := migrations.ServiceDependency{
		ID: id, EnvironmentID: environmentID, SourceServiceID: input.SourceServiceID,
		TargetServiceID: input.TargetServiceID, TargetAccessoryID: input.TargetAccessoryID,
		Type: input.Type,
	}
	if err := service.repository.Create(ctx, &row); err != nil {
		if uniqueViolation(err) {
			return DependencyResource{}, ErrDependencyExists
		}
		return DependencyResource{}, fmt.Errorf("create dependency: %w", err)
	}
	service.record(ctx, requestContext, "dependency.created", row)
	return response(row), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, projectID, environmentID, dependencyID string) error {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	if !validID(dependencyID) {
		return ErrDependencyNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, dependencyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrDependencyNotFound
	}
	if err != nil {
		return fmt.Errorf("find dependency: %w", err)
	}
	if err := service.repository.Delete(ctx, &row); err != nil {
		return fmt.Errorf("delete dependency: %w", err)
	}
	service.record(ctx, requestContext, "dependency.deleted", row)
	return nil
}

// rejectCycle refuses a source -> target edge when target can already reach
// source through existing service-to-service edges. Accessories never have
// outgoing edges, so only service targets can close a cycle.
func (service *Service) rejectCycle(ctx context.Context, environmentID, sourceID, targetID string) error {
	if sourceID == targetID {
		return ErrDependencyCycle
	}
	edges, err := service.repository.ServiceEdges(ctx, environmentID)
	if err != nil {
		return fmt.Errorf("load dependency edges: %w", err)
	}
	visited := map[string]bool{}
	stack := []string{targetID}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == sourceID {
			return ErrDependencyCycle
		}
		if visited[current] {
			continue
		}
		visited[current] = true
		stack = append(stack, edges[current]...)
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

func validateCreate(input CreateInput) (CreateInput, *ValidationError) {
	violations := make([]FieldViolation, 0, 3)
	if !validID(input.SourceServiceID) {
		violations = append(violations, FieldViolation{Field: "sourceServiceId", Code: "invalid", Message: "must be a UUID"})
	}
	hasServiceTarget := input.TargetServiceID != nil
	hasAccessoryTarget := input.TargetAccessoryID != nil
	if hasServiceTarget == hasAccessoryTarget {
		violations = append(violations, FieldViolation{Field: "target", Code: "invalid", Message: "provide exactly one of targetServiceId or targetAccessoryId"})
	} else if hasServiceTarget && !validID(*input.TargetServiceID) {
		violations = append(violations, FieldViolation{Field: "targetServiceId", Code: "invalid", Message: "must be a UUID"})
	} else if hasAccessoryTarget && !validID(*input.TargetAccessoryID) {
		violations = append(violations, FieldViolation{Field: "targetAccessoryId", Code: "invalid", Message: "must be a UUID"})
	}
	input.Type = strings.TrimSpace(input.Type)
	if input.Type == "" {
		input.Type = "runtime"
	}
	if len(input.Type) > 32 || !typePattern.MatchString(input.Type) {
		violations = append(violations, FieldViolation{Field: "type", Code: "invalid", Message: "must be lowercase letters, digits, and single dashes, at most 32 characters"})
	}
	if len(violations) > 0 {
		return input, &ValidationError{Fields: violations}
	}
	return input, nil
}

func response(row migrations.ServiceDependency) DependencyResource {
	return DependencyResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, SourceServiceID: row.SourceServiceID,
		TargetServiceID: row.TargetServiceID, TargetAccessoryID: row.TargetAccessoryID,
		Type: row.Type, CreatedAt: row.CreatedAt.UTC(),
	}
}

func validID(value string) bool { return uuid.Validate(value) == nil }

func uniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.ServiceDependency) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "dependency", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP, RequestID: requestContext.RequestID,
		Metadata: map[string]any{"environmentId": row.EnvironmentID, "sourceServiceId": row.SourceServiceID},
	})
}
