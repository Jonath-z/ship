// Package audit records append-only security and control-plane events.
package audit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

type Event struct {
	ActorUserID  string
	ActorEmail   string
	Action       string
	ResourceType string
	ResourceID   string
	Outcome      string
	SourceIP     string
	RequestID    string
	Metadata     map[string]any
}

type Recorder interface {
	Record(context.Context, Event) error
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (service *Service) Record(ctx context.Context, event Event) error {
	if strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.ResourceType) == "" {
		return errors.New("audit action and resource type are required")
	}
	if event.Outcome != OutcomeSuccess && event.Outcome != OutcomeFailure {
		return errors.New("audit outcome must be success or failure")
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	id, err := identity.New()
	if err != nil {
		return err
	}
	var actorUserID *string
	if event.ActorUserID != "" {
		value := event.ActorUserID
		actorUserID = &value
	}
	entry := migrations.AuditLog{
		ID:           id,
		ActorUserID:  actorUserID,
		ActorEmail:   event.ActorEmail,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Outcome:      event.Outcome,
		SourceIP:     event.SourceIP,
		RequestID:    event.RequestID,
		Metadata:     string(metadata),
	}
	if err := service.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

type Filters struct {
	Action       string
	ResourceType string
	ActorUserID  string
	From         *time.Time
	To           *time.Time
	Cursor       string
	Limit        int
}

type Entry struct {
	ID           string         `json:"id"`
	ActorUserID  string         `json:"actorUserId,omitempty"`
	ActorEmail   string         `json:"actorEmail,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId,omitempty"`
	Outcome      string         `json:"outcome"`
	SourceIP     string         `json:"sourceIp,omitempty"`
	RequestID    string         `json:"requestId,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
}

type Page struct {
	Items      []Entry `json:"items"`
	NextCursor string  `json:"nextCursor,omitempty"`
}

func (service *Service) List(ctx context.Context, filters Filters) (Page, error) {
	if filters.Limit < 1 || filters.Limit > 100 {
		filters.Limit = 20
	}
	query := service.db.WithContext(ctx).Model(&migrations.AuditLog{})
	if filters.Action != "" {
		query = query.Where("action = ?", filters.Action)
	}
	if filters.ResourceType != "" {
		query = query.Where("resource_type = ?", filters.ResourceType)
	}
	if filters.ActorUserID != "" {
		query = query.Where("actor_user_id = ?", filters.ActorUserID)
	}
	if filters.From != nil {
		query = query.Where("created_at >= ?", filters.From.UTC())
	}
	if filters.To != nil {
		query = query.Where("created_at <= ?", filters.To.UTC())
	}
	if filters.Cursor != "" {
		createdAt, id, err := decodeCursor(filters.Cursor)
		if err != nil {
			return Page{}, err
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", createdAt, createdAt, id)
	}

	var rows []migrations.AuditLog
	if err := query.Order("created_at DESC, id DESC").Limit(filters.Limit + 1).Find(&rows).Error; err != nil {
		return Page{}, fmt.Errorf("list audit events: %w", err)
	}
	page := Page{Items: make([]Entry, 0, min(len(rows), filters.Limit))}
	for index, row := range rows {
		if index == filters.Limit {
			last := rows[index-1]
			page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
			break
		}
		entry := Entry{
			ID: row.ID, ActorEmail: row.ActorEmail, Action: row.Action,
			ResourceType: row.ResourceType, ResourceID: row.ResourceID,
			Outcome: row.Outcome, SourceIP: row.SourceIP, RequestID: row.RequestID,
			CreatedAt: row.CreatedAt.UTC(), Metadata: map[string]any{},
		}
		if row.ActorUserID != nil {
			entry.ActorUserID = *row.ActorUserID
		}
		if err := json.Unmarshal([]byte(row.Metadata), &entry.Metadata); err != nil {
			return Page{}, fmt.Errorf("decode audit metadata: %w", err)
		}
		page.Items = append(page.Items, entry)
	}
	return page, nil
}

func encodeCursor(createdAt time.Time, id string) string {
	value := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursor(value string) (time.Time, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, "", errors.New("invalid audit cursor")
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[1] == "" {
		return time.Time{}, "", errors.New("invalid audit cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", errors.New("invalid audit cursor")
	}
	return createdAt, parts[1], nil
}
