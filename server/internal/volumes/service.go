package volumes

import (
	"context"
	"errors"
	"fmt"
	"path"
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
	ErrVolumeNotFound      = errors.New("volume was not found")
	ErrOwnerNotFound       = errors.New("volume owner was not found in this environment")
	ErrSourceExists        = errors.New("volume source already exists in this environment")
)

type VolumeResource struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environmentId"`
	ServiceID     *string   `json:"serviceId,omitempty"`
	AccessoryID   *string   `json:"accessoryId,omitempty"`
	Name          string    `json:"name"`
	Source        string    `json:"source"`
	Destination   string    `json:"destination"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Page struct {
	Items      []VolumeResource `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type CreateInput struct {
	ServiceID   *string
	AccessoryID *string
	Name        string
	Source      string
	Destination string
}

type UpdateInput struct {
	Name        *string
	Source      *string
	Destination *string
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
	return "volume validation failed"
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
	page := Page{Items: make([]VolumeResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, response(row))
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, projectID, environmentID, volumeID string) (VolumeResource, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return VolumeResource{}, err
	}
	if !validID(volumeID) {
		return VolumeResource{}, ErrVolumeNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, volumeID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return VolumeResource{}, ErrVolumeNotFound
	}
	if err != nil {
		return VolumeResource{}, fmt.Errorf("get volume: %w", err)
	}
	return response(row), nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateInput) (VolumeResource, error) {
	normalized, validationError := validateCreate(input)
	if validationError != nil {
		return VolumeResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return VolumeResource{}, err
	}
	if err := service.requireOwner(ctx, environmentID, normalized.ServiceID, normalized.AccessoryID); err != nil {
		return VolumeResource{}, err
	}
	id, err := identity.New()
	if err != nil {
		return VolumeResource{}, err
	}
	row := migrations.Volume{
		ID: id, EnvironmentID: environmentID, ServiceID: normalized.ServiceID,
		AccessoryID: normalized.AccessoryID, Name: normalized.Name,
		Source: normalized.Source, Destination: normalized.Destination,
	}
	if err := service.repository.Create(ctx, &row); err != nil {
		if uniqueViolation(err) {
			return VolumeResource{}, ErrSourceExists
		}
		return VolumeResource{}, fmt.Errorf("create volume: %w", err)
	}
	service.record(ctx, requestContext, "volume.created", row)
	return response(row), nil
}

func (service *Service) Update(ctx context.Context, requestContext RequestContext, projectID, environmentID, volumeID string, input UpdateInput) (VolumeResource, error) {
	values, validationError := validateUpdate(input)
	if validationError != nil {
		return VolumeResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return VolumeResource{}, err
	}
	if !validID(volumeID) {
		return VolumeResource{}, ErrVolumeNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, volumeID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return VolumeResource{}, ErrVolumeNotFound
	}
	if err != nil {
		return VolumeResource{}, fmt.Errorf("find volume: %w", err)
	}
	if err := service.repository.Update(ctx, &row, values); err != nil {
		if uniqueViolation(err) {
			return VolumeResource{}, ErrSourceExists
		}
		return VolumeResource{}, fmt.Errorf("update volume: %w", err)
	}
	service.record(ctx, requestContext, "volume.updated", row)
	return response(row), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, projectID, environmentID, volumeID string) error {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	if !validID(volumeID) {
		return ErrVolumeNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, volumeID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrVolumeNotFound
	}
	if err != nil {
		return fmt.Errorf("find volume: %w", err)
	}
	if err := service.repository.Delete(ctx, &row); err != nil {
		return fmt.Errorf("delete volume: %w", err)
	}
	service.record(ctx, requestContext, "volume.deleted", row)
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

func (service *Service) requireOwner(ctx context.Context, environmentID string, serviceID, accessoryID *string) error {
	if serviceID != nil {
		exists, err := service.repository.ServiceExists(ctx, environmentID, *serviceID)
		if err != nil {
			return fmt.Errorf("find volume service: %w", err)
		}
		if !exists {
			return ErrOwnerNotFound
		}
		return nil
	}
	exists, err := service.repository.AccessoryExists(ctx, environmentID, *accessoryID)
	if err != nil {
		return fmt.Errorf("find volume accessory: %w", err)
	}
	if !exists {
		return ErrOwnerNotFound
	}
	return nil
}

func validateCreate(input CreateInput) (CreateInput, *ValidationError) {
	input.Name = strings.TrimSpace(input.Name)
	input.Source = strings.TrimSpace(input.Source)
	input.Destination = strings.TrimSpace(input.Destination)
	if path.IsAbs(input.Destination) {
		input.Destination = path.Clean(input.Destination)
	}
	violations := validateValues(input.Name, input.Source, input.Destination)
	if (input.ServiceID == nil) == (input.AccessoryID == nil) {
		violations = append(violations, FieldViolation{
			Field: "owner", Code: "invalid", Message: "provide exactly one serviceId or accessoryId",
		})
	}
	if input.ServiceID != nil && !validID(*input.ServiceID) {
		violations = append(violations, FieldViolation{Field: "serviceId", Code: "invalid", Message: "must be a UUID"})
	}
	if input.AccessoryID != nil && !validID(*input.AccessoryID) {
		violations = append(violations, FieldViolation{Field: "accessoryId", Code: "invalid", Message: "must be a UUID"})
	}
	if len(violations) > 0 {
		return CreateInput{}, &ValidationError{Fields: violations}
	}
	return input, nil
}

func validateUpdate(input UpdateInput) (map[string]any, *ValidationError) {
	if input.Name == nil && input.Source == nil && input.Destination == nil {
		return nil, &ValidationError{Fields: []FieldViolation{{Field: "body", Code: "required", Message: "provide at least one field"}}}
	}
	values := make(map[string]any, 3)
	violations := make([]FieldViolation, 0, 3)
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if violation := validateName(name); violation != nil {
			violations = append(violations, *violation)
		} else {
			values["name"] = name
		}
	}
	if input.Source != nil {
		source := strings.TrimSpace(*input.Source)
		if violation := validateSource(source); violation != nil {
			violations = append(violations, *violation)
		} else {
			values["source"] = source
		}
	}
	if input.Destination != nil {
		destination := strings.TrimSpace(*input.Destination)
		if path.IsAbs(destination) {
			destination = path.Clean(destination)
		}
		if violation := validateDestination(destination); violation != nil {
			violations = append(violations, *violation)
		} else {
			values["destination"] = destination
		}
	}
	if len(violations) > 0 {
		return nil, &ValidationError{Fields: violations}
	}
	return values, nil
}

func validateValues(name, source, destination string) []FieldViolation {
	violations := make([]FieldViolation, 0, 3)
	if violation := validateName(name); violation != nil {
		violations = append(violations, *violation)
	}
	if violation := validateSource(source); violation != nil {
		violations = append(violations, *violation)
	}
	if violation := validateDestination(destination); violation != nil {
		violations = append(violations, *violation)
	}
	return violations
}

func validateName(value string) *FieldViolation {
	if length := utf8.RuneCountInString(value); length == 0 || length > 100 {
		return &FieldViolation{Field: "name", Code: "invalid", Message: "must be between 1 and 100 characters"}
	}
	return nil
}

func validateSource(value string) *FieldViolation {
	if value == "" || len(value) > 255 {
		return &FieldViolation{Field: "source", Code: "invalid", Message: "must be between 1 and 255 characters"}
	}
	return nil
}

func validateDestination(value string) *FieldViolation {
	if value == "" || value == "/" || !path.IsAbs(value) || len(value) > 4096 {
		return &FieldViolation{Field: "destination", Code: "invalid", Message: "must be an absolute container path below /"}
	}
	return nil
}

func response(row migrations.Volume) VolumeResource {
	return VolumeResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, ServiceID: row.ServiceID, AccessoryID: row.AccessoryID,
		Name: row.Name, Source: row.Source, Destination: row.Destination,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func validID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func uniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Volume) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "volume", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP,
		RequestID: requestContext.RequestID,
	})
}
