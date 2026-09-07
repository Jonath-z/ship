package deployments

import (
	"context"
	"encoding/json"
	"fmt"

	redisclient "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

// Event is one item on a deployment's stream (SH-080): a log line or a status
// transition. Events are persisted for replay and published for live clients;
// Sequence is the replay cursor.
type Event struct {
	Sequence int64  `json:"sequence"`
	Type     string `json:"type"` // log | status
	Stream   string `json:"stream,omitempty"`
	Message  string `json:"message,omitempty"`
	Status   Status `json:"status,omitempty"`
}

func channelFor(deploymentID string) string {
	return "ship:deployment:" + deploymentID
}

// eventLog persists and fans out one deployment's events. The pipeline is the
// only writer for a given deployment, so a local sequence counter is safe.
type eventLog struct {
	db           *gorm.DB
	redis        *redisclient.Client
	deploymentID string
	sequence     int64
}

func newEventLog(ctx context.Context, db *gorm.DB, redis *redisclient.Client, deploymentID string) (*eventLog, error) {
	var last int64
	err := db.WithContext(ctx).Model(&migrations.DeploymentLog{}).
		Where("deployment_id = ?", deploymentID).
		Select("COALESCE(MAX(sequence), 0)").Scan(&last).Error
	if err != nil {
		return nil, fmt.Errorf("read log cursor: %w", err)
	}
	return &eventLog{db: db, redis: redis, deploymentID: deploymentID, sequence: last}, nil
}

func (log *eventLog) append(ctx context.Context, event Event) {
	log.sequence++
	event.Sequence = log.sequence
	stream := event.Stream
	message := event.Message
	if event.Type == "status" {
		stream = "system"
		message = "deployment is " + string(event.Status)
	}
	id, err := identity.New()
	if err == nil {
		_ = log.db.WithContext(ctx).Create(&migrations.DeploymentLog{
			ID: id, DeploymentID: log.deploymentID, Sequence: event.Sequence,
			Stream: stream, Message: message,
		}).Error
	}
	if encoded, err := json.Marshal(event); err == nil {
		_ = log.redis.Publish(ctx, channelFor(log.deploymentID), encoded).Err()
	}
}

func (log *eventLog) line(ctx context.Context, stream, message string) {
	log.append(ctx, Event{Type: "log", Stream: stream, Message: message})
}

func (log *eventLog) status(ctx context.Context, status Status) {
	log.append(ctx, Event{Type: "status", Status: status})
}
