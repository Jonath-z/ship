package deployments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/internal/platform/jobs"
	"github.com/Jonath-z/ship/server/internal/platform/pagecursor"
	"github.com/Jonath-z/ship/server/migrations"
)

const JobTypeDeploy = "deploy_service"

var (
	ErrEnvironmentNotFound = errors.New("environment was not found")
	ErrServiceNotFound     = errors.New("service was not found in this environment")
	ErrDeploymentNotFound  = errors.New("deployment was not found")
	ErrRollbackUnavailable = errors.New("only successful deployments can be rolled back")
)

type DeploymentResource struct {
	ID                     string     `json:"id"`
	EnvironmentID          string     `json:"environmentId"`
	ServiceID              string     `json:"serviceId"`
	ServiceName            string     `json:"serviceName,omitempty"`
	ConfigurationVersionID string     `json:"configurationVersionId,omitempty"`
	SourceDeploymentID     *string    `json:"sourceDeploymentId,omitempty"`
	Image                  string     `json:"image,omitempty"`
	Status                 Status     `json:"status"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	FinishedAt             *time.Time `json:"finishedAt,omitempty"`
	CreatedAt              time.Time  `json:"createdAt"`
}

type Page struct {
	Items      []DeploymentResource `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

type LogEntry struct {
	Sequence int64     `json:"sequence"`
	Stream   string    `json:"stream"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

type RequestContext struct {
	Actor     access.Principal
	SourceIP  string
	RequestID string
}

type Service struct {
	db    *gorm.DB
	redis *redisclient.Client
	queue *jobs.Queue
	audit audit.Recorder
}

func NewService(db *gorm.DB, redis *redisclient.Client, queue *jobs.Queue, recorder audit.Recorder) *Service {
	return &Service{db: db, redis: redis, queue: queue, audit: recorder}
}

// Create records a QUEUED deployment and enqueues the job. It returns
// immediately — no HTTP request ever waits on Kamal (spec §24).
func (service *Service) Create(ctx context.Context, requestContext RequestContext, projectID, environmentID, serviceID string) (DeploymentResource, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return DeploymentResource{}, err
	}
	var serviceRow migrations.Service
	err := service.db.WithContext(ctx).First(&serviceRow, "id = ? AND environment_id = ?", serviceID, environmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || uuid.Validate(serviceID) != nil {
		return DeploymentResource{}, ErrServiceNotFound
	}
	if err != nil {
		return DeploymentResource{}, fmt.Errorf("find service: %w", err)
	}
	return service.enqueue(ctx, requestContext, environmentID, serviceRow, nil)
}

// Rollback creates a new deployment linked to a successful source deployment;
// the pipeline reproduces the source's configuration version (SH-076).
func (service *Service) Rollback(ctx context.Context, requestContext RequestContext, projectID, environmentID, deploymentID string) (DeploymentResource, error) {
	source, err := service.findRow(ctx, projectID, environmentID, deploymentID)
	if err != nil {
		return DeploymentResource{}, err
	}
	if Status(source.Status) != StatusSuccess {
		return DeploymentResource{}, ErrRollbackUnavailable
	}
	var serviceRow migrations.Service
	if err := service.db.WithContext(ctx).First(&serviceRow, "id = ?", source.ServiceID).Error; err != nil {
		return DeploymentResource{}, ErrServiceNotFound
	}
	return service.enqueue(ctx, requestContext, environmentID, serviceRow, &source.ID)
}

func (service *Service) enqueue(ctx context.Context, requestContext RequestContext, environmentID string, serviceRow migrations.Service, sourceID *string) (DeploymentResource, error) {
	id, err := identity.New()
	if err != nil {
		return DeploymentResource{}, err
	}
	// The configuration version is snapshotted by the pipeline during
	// VALIDATING; a queued record does not reference one yet.
	row := migrations.Deployment{
		ID: id, EnvironmentID: environmentID, ServiceID: serviceRow.ID,
		SourceDeploymentID: sourceID,
		Image:              serviceRow.Image,
		Status:             string(StatusQueued),
	}
	if err := service.db.WithContext(ctx).Create(&row).Error; err != nil {
		return DeploymentResource{}, fmt.Errorf("create deployment: %w", err)
	}
	if err := service.queue.Enqueue(ctx, jobs.Job{Type: JobTypeDeploy, Payload: id}); err != nil {
		return DeploymentResource{}, fmt.Errorf("enqueue deployment: %w", err)
	}
	action := "deployment.queued"
	if sourceID != nil {
		action = "deployment.rollback_queued"
	}
	service.record(ctx, requestContext, action, row)
	return service.response(ctx, row), nil
}

// List returns history, filterable by service and status (SH-074).
func (service *Service) List(ctx context.Context, projectID, environmentID, serviceID, status, cursor string, limit int) (Page, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return Page{}, err
	}
	query := service.db.WithContext(ctx).Where("environment_id = ?", environmentID).Order("created_at DESC, id DESC")
	if serviceID != "" {
		query = query.Where("service_id = ?", serviceID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if cursor != "" {
		value, err := pagecursor.Decode(cursor)
		if err != nil {
			return Page{}, pagecursor.ErrInvalid
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", value.CreatedAt, value.CreatedAt, value.ID)
	}
	var rows []migrations.Deployment
	if err := query.Limit(limit + 1).Find(&rows).Error; err != nil {
		return Page{}, fmt.Errorf("list deployments: %w", err)
	}
	page := Page{Items: make([]DeploymentResource, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.NextCursor = pagecursor.Encode(last.CreatedAt, last.ID)
	}
	for _, row := range rows {
		page.Items = append(page.Items, service.response(ctx, row))
	}
	return page, nil
}

func (service *Service) Get(ctx context.Context, projectID, environmentID, deploymentID string) (DeploymentResource, error) {
	row, err := service.findRow(ctx, projectID, environmentID, deploymentID)
	if err != nil {
		return DeploymentResource{}, err
	}
	return service.response(ctx, row), nil
}

// Logs returns persisted output after the given sequence cursor (SH-090).
func (service *Service) Logs(ctx context.Context, projectID, environmentID, deploymentID string, after int64, limit int) ([]LogEntry, error) {
	if limit < 1 || limit > 5000 {
		limit = 1000
	}
	if _, err := service.findRow(ctx, projectID, environmentID, deploymentID); err != nil {
		return nil, err
	}
	var rows []migrations.DeploymentLog
	err := service.db.WithContext(ctx).
		Where("deployment_id = ? AND sequence > ?", deploymentID, after).
		Order("sequence ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("read deployment logs: %w", err)
	}
	entries := make([]LogEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, LogEntry{
			Sequence: row.Sequence, Stream: row.Stream, Message: row.Message, At: row.CreatedAt.UTC(),
		})
	}
	return entries, nil
}

func (service *Service) findRow(ctx context.Context, projectID, environmentID, deploymentID string) (migrations.Deployment, error) {
	if err := service.requireEnvironment(ctx, projectID, environmentID); err != nil {
		return migrations.Deployment{}, err
	}
	if uuid.Validate(deploymentID) != nil {
		return migrations.Deployment{}, ErrDeploymentNotFound
	}
	var row migrations.Deployment
	err := service.db.WithContext(ctx).First(&row, "id = ? AND environment_id = ?", deploymentID, environmentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return migrations.Deployment{}, ErrDeploymentNotFound
	}
	if err != nil {
		return migrations.Deployment{}, fmt.Errorf("find deployment: %w", err)
	}
	return row, nil
}

func (service *Service) requireEnvironment(ctx context.Context, projectID, environmentID string) error {
	if uuid.Validate(projectID) != nil || uuid.Validate(environmentID) != nil {
		return ErrEnvironmentNotFound
	}
	var count int64
	err := service.db.WithContext(ctx).Model(&migrations.Environment{}).
		Where("id = ? AND project_id = ?", environmentID, projectID).Count(&count).Error
	if err != nil {
		return fmt.Errorf("find environment: %w", err)
	}
	if count != 1 {
		return ErrEnvironmentNotFound
	}
	return nil
}

func (service *Service) response(ctx context.Context, row migrations.Deployment) DeploymentResource {
	resource := DeploymentResource{
		ID: row.ID, EnvironmentID: row.EnvironmentID, ServiceID: row.ServiceID,
		SourceDeploymentID: row.SourceDeploymentID, Image: row.Image, Status: Status(row.Status),
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, CreatedAt: row.CreatedAt.UTC(),
	}
	if row.ConfigurationVersionID != nil {
		resource.ConfigurationVersionID = *row.ConfigurationVersionID
	}
	var name string
	if err := service.db.WithContext(ctx).Model(&migrations.Service{}).
		Where("id = ?", row.ServiceID).Pluck("name", &name).Error; err == nil {
		resource.ServiceName = name
	}
	return resource
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.Deployment) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "deployment", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP, RequestID: requestContext.RequestID,
		Metadata: map[string]any{"environmentId": row.EnvironmentID, "serviceId": row.ServiceID},
	})
}
