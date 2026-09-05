package domains

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	ErrServiceNotFound     = errors.New("service was not found in this environment")
	ErrDomainNotFound      = errors.New("domain was not found")
	ErrHostnameExists      = errors.New("hostname already exists in this environment")
	hostnameLabelPattern   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)
)

// DomainResource mirrors Kamal's model: a domain is just a hostname attached
// to a service. Ship does not manage or verify DNS.
type DomainResource struct {
	ID            string    `json:"id"`
	EnvironmentID string    `json:"environmentId"`
	ServiceID     string    `json:"serviceId"`
	Hostname      string    `json:"hostname"`
	SSLEnabled    bool      `json:"sslEnabled"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Page struct {
	Items      []DomainResource `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type CreateInput struct {
	ServiceID  string
	Hostname   string
	SSLEnabled bool
}

type UpdateInput struct {
	Hostname   *string
	SSLEnabled *bool
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

func (*ValidationError) Error() string { return "domain validation failed" }

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
	page := Page{Items: make([]DomainResource, 0, len(rows)), NextCursor: nextCursor}
	for _, row := range rows {
		page.Items = append(page.Items, response(row))
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, projectID, environmentID, domainID string) (DomainResource, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return DomainResource{}, err
	}
	if !validID(domainID) {
		return DomainResource{}, ErrDomainNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, domainID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DomainResource{}, ErrDomainNotFound
	}
	if err != nil {
		return DomainResource{}, fmt.Errorf("get domain: %w", err)
	}
	return response(row), nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID, environmentID string, input CreateInput) (DomainResource, error) {
	hostname, validationError := validateCreate(input)
	if validationError != nil {
		return DomainResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return DomainResource{}, err
	}
	exists, err := service.repository.ServiceExists(ctx, environmentID, input.ServiceID)
	if err != nil {
		return DomainResource{}, fmt.Errorf("find domain service: %w", err)
	}
	if !exists {
		return DomainResource{}, ErrServiceNotFound
	}
	id, err := identity.New()
	if err != nil {
		return DomainResource{}, err
	}
	row := migrations.Domain{
		ID: id, EnvironmentID: environmentID, ServiceID: input.ServiceID,
		Hostname: hostname, SSLEnabled: input.SSLEnabled,
	}
	if err := service.repository.Create(ctx, &row); err != nil {
		if uniqueViolation(err) {
			return DomainResource{}, ErrHostnameExists
		}
		return DomainResource{}, fmt.Errorf("create domain: %w", err)
	}
	service.record(ctx, requestContext, "domain.created", row)
	return response(row), nil
}

func (service *Service) Update(ctx context.Context, requestContext RequestContext, projectID, environmentID, domainID string, input UpdateInput) (DomainResource, error) {
	values, validationError := validateUpdate(input)
	if validationError != nil {
		return DomainResource{}, validationError
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return DomainResource{}, err
	}
	if !validID(domainID) {
		return DomainResource{}, ErrDomainNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, domainID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DomainResource{}, ErrDomainNotFound
	}
	if err != nil {
		return DomainResource{}, fmt.Errorf("find domain: %w", err)
	}
	if err := service.repository.Update(ctx, &row, values); err != nil {
		if uniqueViolation(err) {
			return DomainResource{}, ErrHostnameExists
		}
		return DomainResource{}, fmt.Errorf("update domain: %w", err)
	}
	service.record(ctx, requestContext, "domain.updated", row)
	return response(row), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, projectID, environmentID, domainID string) error {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return err
	}
	if !validID(domainID) {
		return ErrDomainNotFound
	}
	row, err := service.repository.Find(ctx, environmentID, domainID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrDomainNotFound
	}
	if err != nil {
		return fmt.Errorf("find domain: %w", err)
	}
	if err := service.repository.Delete(ctx, &row); err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	service.record(ctx, requestContext, "domain.deleted", row)
	return nil
}

func response(row migrations.Domain) DomainResource {
	return DomainResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, ServiceID: row.ServiceID,
		Hostname: row.Hostname, SSLEnabled: row.SSLEnabled,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
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

func validateCreate(input CreateInput) (string, *ValidationError) {
	violations := make([]FieldViolation, 0, 2)
	if !validID(input.ServiceID) {
		violations = append(violations, FieldViolation{Field: "serviceId", Code: "invalid", Message: "must be a UUID"})
	}
	hostname, hostnameViolation := normalizeHostname(input.Hostname)
	if hostnameViolation != nil {
		violations = append(violations, *hostnameViolation)
	}
	if len(violations) > 0 {
		return "", &ValidationError{Fields: violations}
	}
	return hostname, nil
}

func validateUpdate(input UpdateInput) (map[string]any, *ValidationError) {
	if input.Hostname == nil && input.SSLEnabled == nil {
		return nil, &ValidationError{Fields: []FieldViolation{{Field: "body", Code: "required", Message: "provide at least one field"}}}
	}
	values := make(map[string]any, 2)
	if input.Hostname != nil {
		hostname, violation := normalizeHostname(*input.Hostname)
		if violation != nil {
			return nil, &ValidationError{Fields: []FieldViolation{*violation}}
		}
		values["hostname"] = hostname
	}
	if input.SSLEnabled != nil {
		values["ssl_enabled"] = *input.SSLEnabled
	}
	return values, nil
}

func normalizeHostname(value string) (string, *FieldViolation) {
	hostname := strings.ToLower(strings.TrimSpace(value))
	hostname = strings.TrimSuffix(hostname, ".")
	invalid := func(message string) (string, *FieldViolation) {
		return "", &FieldViolation{Field: "hostname", Code: "invalid", Message: message}
	}
	if hostname == "" || len(hostname) > 253 {
		return invalid("must be between 1 and 253 characters")
	}
	if net.ParseIP(hostname) != nil || strings.Contains(hostname, "://") {
		return invalid("must be a hostname, not an IP address or URL")
	}
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 {
		return invalid("must be a fully qualified hostname such as app.example.com")
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || !hostnameLabelPattern.MatchString(label) {
			return invalid("contains an invalid DNS label")
		}
	}
	return hostname, nil
}

func validID(value string) bool { return uuid.Validate(value) == nil }

func uniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Domain) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "domain", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP, RequestID: requestContext.RequestID,
		Metadata: map[string]any{"environmentId": row.EnvironmentID, "serviceId": row.ServiceID, "hostname": row.Hostname},
	})
}
