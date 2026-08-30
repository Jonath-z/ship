package accessories

import (
	"context"
	"errors"
	"fmt"
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
	ErrAccessoryNotFound   = errors.New("accessory was not found")
	ErrPlacementNotFound   = errors.New("accessory placement target was not found")
	ErrNameExists          = errors.New("accessory name already exists in this environment")
	ErrConnectionSecret    = errors.New("postgresql connection secret could not be created")
)

type VolumeSuggestion struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type AccessoryResource struct {
	ID                        string            `json:"id"`
	EnvironmentID             string            `json:"environmentId"`
	Name                      string            `json:"name"`
	Type                      string            `json:"type"`
	Image                     string            `json:"image"`
	ServerID                  *string           `json:"serverId,omitempty"`
	ServerGroupID             *string           `json:"serverGroupId,omitempty"`
	Role                      string            `json:"role,omitempty"`
	Port                      *int              `json:"port,omitempty"`
	SuggestedVolume           *VolumeSuggestion `json:"suggestedVolume,omitempty"`
	SuggestedConnectionSecret string            `json:"suggestedConnectionSecret,omitempty"`
	ConnectionSecret          string            `json:"connectionSecret,omitempty"`
	CreatedAt                 time.Time         `json:"createdAt"`
	UpdatedAt                 time.Time         `json:"updatedAt"`
}

type Page struct {
	Items      []AccessoryResource `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

type CreateInput struct {
	Name          string
	Type          string
	Image         string
	ServerID      *string
	ServerGroupID *string
	Port          *int
}

type UpdateInput struct {
	Name          *string
	Type          *string
	Image         *string
	PlacementSet  bool
	ServerID      *string
	ServerGroupID *string
	PortSet       bool
	Port          *int
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
	return "accessory validation failed"
}

type Service struct {
	repository        *Repository
	audit             audit.Recorder
	secretProvisioner PostgresConnectionProvisioner
}

type PostgresConnectionProvisioner interface {
	ProvisionPostgresConnection(context.Context, access.Principal, string, string, string, string, string, int) (string, error)
}

func NewService(repository *Repository, recorder audit.Recorder, provisioners ...PostgresConnectionProvisioner) *Service {
	service := &Service{repository: repository, audit: recorder}
	if len(provisioners) > 0 {
		service.secretProvisioner = provisioners[0]
	}
	return service
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
	page := Page{Items: make([]AccessoryResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, response(row))
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, projectID, environmentID, accessoryID string) (AccessoryResource, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return AccessoryResource{}, err
	}
	if !validID(accessoryID) {
		return AccessoryResource{}, ErrAccessoryNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, accessoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AccessoryResource{}, ErrAccessoryNotFound
	}
	if err != nil {
		return AccessoryResource{}, fmt.Errorf("get accessory: %w", err)
	}
	return response(row), nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateInput) (AccessoryResource, error) {
	normalized, validationError := validateCreate(input)
	if validationError != nil {
		return AccessoryResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return AccessoryResource{}, err
	}
	if err := service.requirePlacement(ctx, environmentID, normalized.ServerID, normalized.ServerGroupID); err != nil {
		return AccessoryResource{}, err
	}
	id, err := identity.New()
	if err != nil {
		return AccessoryResource{}, err
	}
	row := migrations.Accessory{
		ID: id, EnvironmentID: environmentID, Name: normalized.Name, Type: normalized.Type,
		Image: normalized.Image, ServerID: normalized.ServerID,
		ServerGroupID: normalized.ServerGroupID, Port: normalized.Port,
	}
	if err := service.repository.Create(ctx, &row); err != nil {
		if uniqueViolation(err) {
			return AccessoryResource{}, ErrNameExists
		}
		return AccessoryResource{}, fmt.Errorf("create accessory: %w", err)
	}
	result := response(row)
	if row.Type == "postgres" {
		result.SuggestedVolume = &VolumeSuggestion{
			Name: row.Name + " data", Source: identifier(row.Name) + "_data",
			Destination: "/var/lib/postgresql/data",
		}
		if service.secretProvisioner == nil {
			result.SuggestedConnectionSecret = "DATABASE_URL"
		} else {
			port := 5432
			if row.Port != nil {
				port = *row.Port
			}
			secretName, provisionErr := service.secretProvisioner.ProvisionPostgresConnection(
				ctx, requestContext.Actor, requestContext.SourceIP, requestContext.RequestID,
				projectID, environmentID, row.Name, port,
			)
			if provisionErr != nil {
				if cleanupErr := service.repository.Delete(ctx, &row); cleanupErr != nil {
					return AccessoryResource{}, fmt.Errorf("%w: %v; accessory cleanup failed: %v", ErrConnectionSecret, provisionErr, cleanupErr)
				}
				return AccessoryResource{}, fmt.Errorf("%w: %v", ErrConnectionSecret, provisionErr)
			}
			result.ConnectionSecret = secretName
		}
	}
	service.record(ctx, requestContext, "accessory.created", row)
	return result, nil
}

func (service *Service) Update(ctx context.Context, requestContext RequestContext, projectID, environmentID, accessoryID string, input UpdateInput) (AccessoryResource, error) {
	if validationError := validateUpdateShape(input); validationError != nil {
		return AccessoryResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return AccessoryResource{}, err
	}
	if !validID(accessoryID) {
		return AccessoryResource{}, ErrAccessoryNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, accessoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AccessoryResource{}, ErrAccessoryNotFound
	}
	if err != nil {
		return AccessoryResource{}, fmt.Errorf("find accessory: %w", err)
	}
	values, violations := applyUpdate(row, input)
	if len(violations) > 0 {
		return AccessoryResource{}, &ValidationError{Fields: violations}
	}
	if input.PlacementSet {
		if err := service.requirePlacement(ctx, environmentID, input.ServerID, input.ServerGroupID); err != nil {
			return AccessoryResource{}, err
		}
	}
	if err := service.repository.Update(ctx, &row, values); err != nil {
		if uniqueViolation(err) {
			return AccessoryResource{}, ErrNameExists
		}
		return AccessoryResource{}, fmt.Errorf("update accessory: %w", err)
	}
	service.record(ctx, requestContext, "accessory.updated", row)
	return response(row), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, projectID, environmentID, accessoryID string) error {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	if !validID(accessoryID) {
		return ErrAccessoryNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, accessoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAccessoryNotFound
	}
	if err != nil {
		return fmt.Errorf("find accessory: %w", err)
	}
	if err := service.repository.Delete(ctx, &row); err != nil {
		return fmt.Errorf("delete accessory: %w", err)
	}
	service.record(ctx, requestContext, "accessory.deleted", row)
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

func (service *Service) requirePlacement(ctx context.Context, environmentID string, serverID, serverGroupID *string) error {
	if serverID == nil && serverGroupID == nil {
		return nil
	}
	if serverID != nil {
		exists, err := service.repository.ServerExists(ctx, *serverID)
		if err != nil {
			return fmt.Errorf("find accessory server: %w", err)
		}
		if !exists {
			return ErrPlacementNotFound
		}
		return nil
	}
	exists, err := service.repository.ServerGroupExists(ctx, environmentID, *serverGroupID)
	if err != nil {
		return fmt.Errorf("find accessory server group: %w", err)
	}
	if !exists {
		return ErrPlacementNotFound
	}
	return nil
}

func validateCreate(input CreateInput) (CreateInput, *ValidationError) {
	input.Name = strings.TrimSpace(input.Name)
	input.Type = strings.TrimSpace(input.Type)
	input.Image = strings.TrimSpace(input.Image)
	if input.Port == nil {
		port := defaultPort(input.Type)
		if port != 0 {
			input.Port = &port
		}
	}
	violations := validateValues(input.Name, input.Type, input.Image, input.Port, input.ServerID, input.ServerGroupID)
	if len(violations) > 0 {
		return CreateInput{}, &ValidationError{Fields: violations}
	}
	return input, nil
}

func validateUpdateShape(input UpdateInput) *ValidationError {
	if input.Name == nil && input.Type == nil && input.Image == nil && !input.PlacementSet && !input.PortSet {
		return &ValidationError{Fields: []FieldViolation{{Field: "body", Code: "required", Message: "provide at least one field"}}}
	}
	return nil
}

func applyUpdate(row migrations.Accessory, input UpdateInput) (map[string]any, []FieldViolation) {
	name, accessoryType, image, port := row.Name, row.Type, row.Image, row.Port
	serverID, serverGroupID := row.ServerID, row.ServerGroupID
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
	}
	if input.Type != nil {
		accessoryType = strings.TrimSpace(*input.Type)
	}
	if input.Image != nil {
		image = strings.TrimSpace(*input.Image)
	}
	if input.PortSet {
		port = input.Port
	}
	if input.PlacementSet {
		serverID, serverGroupID = input.ServerID, input.ServerGroupID
	}
	violations := validateValues(name, accessoryType, image, port, serverID, serverGroupID)
	if len(violations) > 0 {
		return nil, violations
	}
	values := make(map[string]any, 7)
	if input.Name != nil {
		values["name"] = name
	}
	if input.Type != nil {
		values["type"] = accessoryType
	}
	if input.Image != nil {
		values["image"] = image
	}
	if input.PortSet {
		values["port"] = port
	}
	if input.PlacementSet {
		values["server_id"] = serverID
		values["server_group_id"] = serverGroupID
	}
	return values, nil
}

func validateValues(name, accessoryType, image string, port *int, serverID, serverGroupID *string) []FieldViolation {
	violations := make([]FieldViolation, 0, 4)
	if length := utf8.RuneCountInString(name); length == 0 || length > 100 {
		violations = append(violations, FieldViolation{Field: "name", Code: "invalid", Message: "must be between 1 and 100 characters"})
	}
	if accessoryType != "postgres" && accessoryType != "redis" {
		violations = append(violations, FieldViolation{Field: "type", Code: "invalid", Message: "must be postgres or redis"})
	}
	if image == "" || len(image) > 512 {
		violations = append(violations, FieldViolation{Field: "image", Code: "invalid", Message: "must be between 1 and 512 characters"})
	}
	if port != nil && (*port < 1 || *port > 65535) {
		violations = append(violations, FieldViolation{Field: "port", Code: "range", Message: "must be between 1 and 65535"})
	}
	if serverID != nil && serverGroupID != nil {
		violations = append(violations, FieldViolation{Field: "placement", Code: "invalid", Message: "provide a serverId or serverGroupId, not both"})
	}
	if serverID != nil && !validID(*serverID) {
		violations = append(violations, FieldViolation{Field: "serverId", Code: "invalid", Message: "must be a UUID"})
	}
	if serverGroupID != nil && !validID(*serverGroupID) {
		violations = append(violations, FieldViolation{Field: "serverGroupId", Code: "invalid", Message: "must be a UUID"})
	}
	return violations
}

func defaultPort(accessoryType string) int {
	switch accessoryType {
	case "postgres":
		return 5432
	case "redis":
		return 6379
	default:
		return 0
	}
}

func identifier(value string) string {
	var result strings.Builder
	separator := false
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			if separator && result.Len() > 0 {
				result.WriteByte('_')
			}
			separator = false
			result.WriteRune(character)
		} else if result.Len() > 0 {
			separator = true
		}
	}
	if result.Len() == 0 {
		return "postgres"
	}
	return strings.TrimRight(result.String(), "_")
}

func response(row migrations.Accessory) AccessoryResource {
	result := AccessoryResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, Name: row.Name, Type: row.Type,
		Image: row.Image, ServerID: row.ServerID, ServerGroupID: row.ServerGroupID, Port: row.Port,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
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

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Accessory) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "accessory", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP,
		RequestID: requestContext.RequestID,
	})
}
