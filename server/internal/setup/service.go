// Package setup owns Ship's single-use owner bootstrap flow.
package setup

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/auth"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrAlreadyComplete = errors.New("setup is already complete")
	ErrInvalidToken    = errors.New("first-run token is invalid")
)

type Service struct {
	db        *gorm.DB
	tokenHash string
	hostname  string
}

type Status struct {
	Required        bool   `json:"required"`
	TokenConfigured bool   `json:"tokenConfigured"`
	Hostname        string `json:"hostname"`
}

type Owner struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func NewService(db *gorm.DB, tokenHash, hostname string) *Service {
	return &Service{db: db, tokenHash: tokenHash, hostname: hostname}
}

func (service *Service) Status(ctx context.Context) (Status, error) {
	var count int64
	if err := service.db.WithContext(ctx).Model(&migrations.User{}).Count(&count).Error; err != nil {
		return Status{}, fmt.Errorf("count users: %w", err)
	}
	return Status{
		Required:        count == 0,
		TokenConfigured: service.tokenHash != "",
		Hostname:        service.hostname,
	}, nil
}

func (service *Service) CreateOwner(ctx context.Context, token, email, password string) (Owner, error) {
	if !validToken(token, service.tokenHash) {
		return Owner{}, ErrInvalidToken
	}

	email = strings.ToLower(strings.TrimSpace(email))
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return Owner{}, err
	}
	userID, err := identity.New()
	if err != nil {
		return Owner{}, err
	}
	user := migrations.User{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         "owner",
	}

	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "ship-first-run-setup").Error; err != nil {
			return fmt.Errorf("lock setup: %w", err)
		}
		var count int64
		if err := tx.Model(&migrations.User{}).Count(&count).Error; err != nil {
			return fmt.Errorf("count users: %w", err)
		}
		if count > 0 {
			return ErrAlreadyComplete
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("create owner: %w", err)
		}
		return nil
	})
	if err != nil {
		return Owner{}, err
	}
	return Owner{ID: user.ID, Email: user.Email, Role: user.Role}, nil
}

func validToken(token, expectedHash string) bool {
	if token == "" || len(expectedHash) != sha256.Size*2 {
		return false
	}
	actual := sha256.Sum256([]byte(token))
	expected, err := hex.DecodeString(expectedHash)
	if err != nil || len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare(actual[:], expected) == 1
}
