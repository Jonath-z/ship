// Package setup owns Ship's single-use owner bootstrap flow.
package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"

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
	passwordHash, err := hashPassword(password)
	if err != nil {
		return Owner{}, err
	}
	userID, err := newUUID()
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
	return Owner{Email: user.Email, Role: user.Role}, nil
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

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	const (
		memory      = 64 * 1024
		iterations  = 3
		parallelism = 4
		keyLength   = 32
	)
	key := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate user ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
