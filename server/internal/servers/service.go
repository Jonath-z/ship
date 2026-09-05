// Package servers manages the registered VPS inventory (SH-042), connection
// checks (SH-043), preparation (SH-044), and role membership (SH-045).
package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	cryptossh "golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	shipssh "github.com/Jonath-z/ship/server/internal/ssh"
	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrServerNotFound = errors.New("server was not found")
	ErrNameExists     = errors.New("a server with this name already exists")
	ErrServerInUse    = errors.New("server is still used by services or accessories")
	ErrKeyNotFound    = errors.New("SSH key was not found")
)

// Runner abstracts the SSH transport so checks are testable without a VPS.
type Runner interface {
	Run(ctx context.Context, target shipssh.Target, signer cryptossh.Signer, command shipssh.Command, stream func(line string)) (shipssh.Result, error)
}

// SignerSource abstracts private-key access (implemented by sshkeys.Service).
type SignerSource interface {
	Signer(ctx context.Context, keyID string) (cryptossh.Signer, error)
}

type Resources struct {
	CPUCores    int   `json:"cpuCores,omitempty"`
	MemoryBytes int64 `json:"memoryBytes,omitempty"`
	DiskBytes   int64 `json:"diskBytes,omitempty"`
}

type ServerResource struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Hostname     string    `json:"hostname,omitempty"`
	IPAddress    string    `json:"ipAddress,omitempty"`
	SSHUser      string    `json:"sshUser"`
	SSHPort      int       `json:"sshPort"`
	SSHKeyID     *string   `json:"sshKeyId,omitempty"`
	Architecture string    `json:"architecture,omitempty"`
	OS           string    `json:"os,omitempty"`
	Status       string    `json:"status"`
	Resources    Resources `json:"resources"`
	HostKeySaved bool      `json:"hostKeySaved"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type CreateInput struct {
	Name      string
	Hostname  string
	IPAddress string
	SSHUser   string
	SSHPort   int
	SSHKeyID  string
}

type UpdateInput struct {
	Name      *string
	Hostname  *string
	IPAddress *string
	SSHUser   *string
	SSHPort   *int
	SSHKeyID  *string
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

func (*ValidationError) Error() string { return "server validation failed" }

// DependentsError blocks deletion while the server is still in use.
type DependentsError struct {
	Names []string
}

func (*DependentsError) Error() string { return "server is still used by services or accessories" }

type Service struct {
	db      *gorm.DB
	signers SignerSource
	runner  Runner
	audit   audit.Recorder
}

func NewService(db *gorm.DB, signers SignerSource, runner Runner, recorder audit.Recorder) *Service {
	return &Service{db: db, signers: signers, runner: runner, audit: recorder}
}

func (service *Service) List(ctx context.Context) ([]ServerResource, error) {
	var rows []migrations.Server
	if err := service.db.WithContext(ctx).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	servers := make([]ServerResource, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, response(row))
	}
	return servers, nil
}

func (service *Service) Get(ctx context.Context, serverID string) (ServerResource, error) {
	row, err := service.find(ctx, serverID)
	if err != nil {
		return ServerResource{}, err
	}
	return response(row), nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, input CreateInput) (ServerResource, error) {
	input, validationError := validateCreate(input)
	if validationError != nil {
		return ServerResource{}, validationError
	}
	if err := service.requireKey(ctx, input.SSHKeyID); err != nil {
		return ServerResource{}, err
	}
	id, err := identity.New()
	if err != nil {
		return ServerResource{}, err
	}
	row := migrations.Server{
		ID: id, Name: input.Name, Hostname: input.Hostname, IPAddress: input.IPAddress,
		SSHUser: input.SSHUser, SSHPort: input.SSHPort, SSHKeyID: &input.SSHKeyID,
		Status: "pending", Resources: "{}",
	}
	if err := service.db.WithContext(ctx).Create(&row).Error; err != nil {
		if uniqueViolation(err) {
			return ServerResource{}, ErrNameExists
		}
		return ServerResource{}, fmt.Errorf("create server: %w", err)
	}
	service.record(ctx, requestContext, "server.created", row)
	return response(row), nil
}

func (service *Service) Update(ctx context.Context, requestContext RequestContext, serverID string, input UpdateInput) (ServerResource, error) {
	values, validationError := validateUpdate(input)
	if validationError != nil {
		return ServerResource{}, validationError
	}
	row, err := service.find(ctx, serverID)
	if err != nil {
		return ServerResource{}, err
	}
	if input.SSHKeyID != nil {
		if err := service.requireKey(ctx, *input.SSHKeyID); err != nil {
			return ServerResource{}, err
		}
	}
	// Changing the address invalidates the recorded host key: the next
	// connection re-establishes trust-on-first-use for the new machine.
	if input.Hostname != nil || input.IPAddress != nil {
		values["host_key"] = ""
		values["status"] = "pending"
	}
	values["updated_at"] = time.Now().UTC()
	if err := service.db.WithContext(ctx).Model(&row).Updates(values).Error; err != nil {
		if uniqueViolation(err) {
			return ServerResource{}, ErrNameExists
		}
		return ServerResource{}, fmt.Errorf("update server: %w", err)
	}
	if err := service.db.WithContext(ctx).First(&row, "id = ?", row.ID).Error; err != nil {
		return ServerResource{}, fmt.Errorf("reload server: %w", err)
	}
	service.record(ctx, requestContext, "server.updated", row)
	return response(row), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, serverID string) error {
	row, err := service.find(ctx, serverID)
	if err != nil {
		return err
	}
	users, err := service.serverUsers(ctx, row.ID)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return &DependentsError{Names: users}
	}
	if err := service.db.WithContext(ctx).Delete(&row).Error; err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	service.record(ctx, requestContext, "server.deleted", row)
	return nil
}

// serverUsers lists what blocks removal: services whose role includes this
// server, and accessories placed on it directly or through a group.
func (service *Service) serverUsers(ctx context.Context, serverID string) ([]string, error) {
	var serviceNames []string
	err := service.db.WithContext(ctx).Model(&migrations.Service{}).Distinct("services.name").
		Joins("JOIN server_group_memberships ON server_group_memberships.server_group_id = services.server_group_id").
		Where("server_group_memberships.server_id = ?", serverID).
		Order("services.name ASC").Pluck("services.name", &serviceNames).Error
	if err != nil {
		return nil, fmt.Errorf("find dependent services: %w", err)
	}
	var accessoryNames []string
	err = service.db.WithContext(ctx).Model(&migrations.Accessory{}).Distinct("accessories.name").
		Joins("LEFT JOIN server_group_memberships ON server_group_memberships.server_group_id = accessories.server_group_id").
		Where("accessories.server_id = ? OR server_group_memberships.server_id = ?", serverID, serverID).
		Order("accessories.name ASC").Pluck("accessories.name", &accessoryNames).Error
	if err != nil {
		return nil, fmt.Errorf("find dependent accessories: %w", err)
	}
	users := make([]string, 0, len(serviceNames)+len(accessoryNames))
	for _, name := range serviceNames {
		users = append(users, "service "+name)
	}
	for _, name := range accessoryNames {
		users = append(users, "accessory "+name)
	}
	return users, nil
}

func (service *Service) requireKey(ctx context.Context, keyID string) error {
	if uuid.Validate(keyID) != nil {
		return ErrKeyNotFound
	}
	var count int64
	if err := service.db.WithContext(ctx).Model(&migrations.SSHKey{}).Where("id = ?", keyID).Count(&count).Error; err != nil {
		return fmt.Errorf("find SSH key: %w", err)
	}
	if count != 1 {
		return ErrKeyNotFound
	}
	return nil
}

func (service *Service) find(ctx context.Context, serverID string) (migrations.Server, error) {
	if uuid.Validate(serverID) != nil {
		return migrations.Server{}, ErrServerNotFound
	}
	var row migrations.Server
	err := service.db.WithContext(ctx).First(&row, "id = ?", serverID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return migrations.Server{}, ErrServerNotFound
	}
	if err != nil {
		return migrations.Server{}, fmt.Errorf("find server: %w", err)
	}
	return row, nil
}

func validateCreate(input CreateInput) (CreateInput, *ValidationError) {
	input.Name = strings.TrimSpace(input.Name)
	input.Hostname = strings.TrimSpace(input.Hostname)
	input.IPAddress = strings.TrimSpace(input.IPAddress)
	input.SSHUser = strings.TrimSpace(input.SSHUser)
	if input.SSHUser == "" {
		input.SSHUser = "root"
	}
	if input.SSHPort == 0 {
		input.SSHPort = 22
	}
	violations := validateValues(input.Name, input.Hostname, input.IPAddress, input.SSHUser, input.SSHPort)
	if len(violations) > 0 {
		return input, &ValidationError{Fields: violations}
	}
	return input, nil
}

func validateUpdate(input UpdateInput) (map[string]any, *ValidationError) {
	if input.Name == nil && input.Hostname == nil && input.IPAddress == nil &&
		input.SSHUser == nil && input.SSHPort == nil && input.SSHKeyID == nil {
		return nil, &ValidationError{Fields: []FieldViolation{{Field: "body", Code: "required", Message: "provide at least one field"}}}
	}
	values := map[string]any{}
	var violations []FieldViolation
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if length := utf8.RuneCountInString(name); length == 0 || length > 100 {
			violations = append(violations, FieldViolation{Field: "name", Code: "invalid", Message: "must be between 1 and 100 characters"})
		}
		values["name"] = name
	}
	if input.Hostname != nil {
		values["hostname"] = strings.TrimSpace(*input.Hostname)
	}
	if input.IPAddress != nil {
		address := strings.TrimSpace(*input.IPAddress)
		if address != "" && net.ParseIP(address) == nil {
			violations = append(violations, FieldViolation{Field: "ipAddress", Code: "invalid", Message: "must be an IP address"})
		}
		values["ip_address"] = address
	}
	if input.SSHUser != nil {
		user := strings.TrimSpace(*input.SSHUser)
		if user == "" {
			violations = append(violations, FieldViolation{Field: "sshUser", Code: "required", Message: "must not be empty"})
		}
		values["ssh_user"] = user
	}
	if input.SSHPort != nil {
		if *input.SSHPort < 1 || *input.SSHPort > 65535 {
			violations = append(violations, FieldViolation{Field: "sshPort", Code: "invalid", Message: "must be between 1 and 65535"})
		}
		values["ssh_port"] = *input.SSHPort
	}
	if input.SSHKeyID != nil {
		values["ssh_key_id"] = *input.SSHKeyID
	}
	if len(violations) > 0 {
		return nil, &ValidationError{Fields: violations}
	}
	return values, nil
}

func validateValues(name, hostname, ipAddress, sshUser string, sshPort int) []FieldViolation {
	var violations []FieldViolation
	if length := utf8.RuneCountInString(name); length == 0 || length > 100 {
		violations = append(violations, FieldViolation{Field: "name", Code: "invalid", Message: "must be between 1 and 100 characters"})
	}
	if hostname == "" && ipAddress == "" {
		violations = append(violations, FieldViolation{Field: "ipAddress", Code: "required", Message: "provide a hostname or an IP address"})
	}
	if ipAddress != "" && net.ParseIP(ipAddress) == nil {
		violations = append(violations, FieldViolation{Field: "ipAddress", Code: "invalid", Message: "must be an IP address"})
	}
	if sshUser == "" {
		violations = append(violations, FieldViolation{Field: "sshUser", Code: "required", Message: "must not be empty"})
	}
	if sshPort < 1 || sshPort > 65535 {
		violations = append(violations, FieldViolation{Field: "sshPort", Code: "invalid", Message: "must be between 1 and 65535"})
	}
	return violations
}

func response(row migrations.Server) ServerResource {
	resources := Resources{}
	_ = json.Unmarshal([]byte(row.Resources), &resources)
	return ServerResource{
		ID: row.ID, Name: row.Name, Hostname: row.Hostname, IPAddress: row.IPAddress,
		SSHUser: row.SSHUser, SSHPort: row.SSHPort, SSHKeyID: row.SSHKeyID,
		Architecture: row.Architecture, OS: row.OS, Status: row.Status,
		Resources: resources, HostKeySaved: row.HostKey != "",
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}

func uniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Server) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "server", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP, RequestID: requestContext.RequestID,
		Metadata: map[string]any{"name": row.Name},
	})
}
