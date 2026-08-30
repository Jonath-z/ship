// Package users manages local Ship accounts and install-scoped roles.
package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/auth"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrEmailExists       = errors.New("a user with this email already exists")
	ErrUserNotFound      = errors.New("user was not found")
	ErrOwnerRequired     = errors.New("the installation must retain an active owner")
	ErrOwnerOnly         = errors.New("only an owner can manage owner accounts")
	ErrCannotDisableSelf = errors.New("you cannot disable your own account")
)

type Service struct {
	db       *gorm.DB
	sessions *auth.Service
	audit    audit.Recorder
}

func NewService(db *gorm.DB, sessions *auth.Service, recorder audit.Recorder) *Service {
	return &Service{db: db, sessions: sessions, audit: recorder}
}

type User struct {
	ID        string      `json:"id"`
	Email     string      `json:"email"`
	Role      access.Role `json:"role"`
	Disabled  bool        `json:"disabled"`
	CreatedAt time.Time   `json:"createdAt"`
}

type RequestContext struct {
	Actor     access.Principal
	SourceIP  string
	RequestID string
}

func (service *Service) List(ctx context.Context) ([]User, error) {
	var rows []migrations.User
	if err := service.db.WithContext(ctx).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	result := make([]User, 0, len(rows))
	for _, row := range rows {
		result = append(result, response(row))
	}
	return result, nil
}

func (service *Service) Create(ctx context.Context, requestContext RequestContext, email, password string, role access.Role) (User, error) {
	if requestContext.Actor.Role != access.RoleOwner && role == access.RoleOwner {
		return User{}, ErrOwnerOnly
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	id, err := identity.New()
	if err != nil {
		return User{}, err
	}
	row := migrations.User{
		ID: id, Email: strings.ToLower(strings.TrimSpace(email)),
		PasswordHash: hash, Role: string(role),
	}
	if err := service.db.WithContext(ctx).Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailExists
		}
		return User{}, fmt.Errorf("create user: %w", err)
	}
	service.record(ctx, requestContext, "user.created", row, map[string]any{"role": role})
	return response(row), nil
}

func (service *Service) ChangeRole(ctx context.Context, requestContext RequestContext, userID string, role access.Role) (User, error) {
	var updated migrations.User
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user migrations.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if (access.Role(user.Role) == access.RoleOwner || role == access.RoleOwner) && requestContext.Actor.Role != access.RoleOwner {
			return ErrOwnerOnly
		}
		if access.Role(user.Role) == access.RoleOwner && role != access.RoleOwner {
			if err := ensureAnotherOwner(tx, user.ID); err != nil {
				return err
			}
		}
		if err := tx.Model(&user).Update("role", string(role)).Error; err != nil {
			return err
		}
		user.Role = string(role)
		updated = user
		return nil
	})
	if err != nil {
		return User{}, fmt.Errorf("change user role: %w", err)
	}
	if err := service.sessions.InvalidateUserSessions(ctx, updated.ID); err != nil {
		return User{}, err
	}
	service.record(ctx, requestContext, "user.role_changed", updated, map[string]any{"role": role})
	return response(updated), nil
}

func (service *Service) Disable(ctx context.Context, requestContext RequestContext, userID string) error {
	if requestContext.Actor.UserID == userID {
		return ErrCannotDisableSelf
	}
	var disabled migrations.User
	err := service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user migrations.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		if access.Role(user.Role) == access.RoleOwner {
			if requestContext.Actor.Role != access.RoleOwner {
				return ErrOwnerOnly
			}
			if err := ensureAnotherOwner(tx, user.ID); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		if err := tx.Model(&user).Update("disabled_at", now).Error; err != nil {
			return err
		}
		user.DisabledAt = &now
		disabled = user
		return nil
	})
	if err != nil {
		return fmt.Errorf("disable user: %w", err)
	}
	if err := service.sessions.InvalidateUserSessions(ctx, userID); err != nil {
		return err
	}
	service.record(ctx, requestContext, "user.disabled", disabled, nil)
	return nil
}

func ensureAnotherOwner(tx *gorm.DB, excludingID string) error {
	var owners []migrations.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
		"role = ? AND disabled_at IS NULL", access.RoleOwner,
	).Find(&owners).Error; err != nil {
		return err
	}
	for _, owner := range owners {
		if owner.ID != excludingID {
			return nil
		}
	}
	return ErrOwnerRequired
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, user migrations.User, metadata map[string]any) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "user", ResourceID: user.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP,
		RequestID: requestContext.RequestID, Metadata: metadata,
	})
}

func response(row migrations.User) User {
	return User{
		ID: row.ID, Email: row.Email, Role: access.Role(row.Role),
		Disabled: row.DisabledAt != nil, CreatedAt: row.CreatedAt.UTC(),
	}
}

func isUniqueViolation(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate key") ||
		strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
