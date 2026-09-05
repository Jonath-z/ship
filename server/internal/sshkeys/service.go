// Package sshkeys manages named SSH keypairs (SH-041). Public keys are stored
// for display and installation on servers; private keys live only in the
// encrypted vault and are never returned by the API.
package sshkeys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	cryptossh "golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

var (
	ErrKeyNotFound   = errors.New("SSH key was not found")
	ErrNameExists    = errors.New("an SSH key with this name already exists")
	ErrKeyInUse      = errors.New("SSH key is assigned to one or more servers")
	ErrInvalidImport = errors.New("private key could not be parsed")
)

type KeyResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	PublicKey string    `json:"publicKey"`
	CreatedAt time.Time `json:"createdAt"`
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

func (*ValidationError) Error() string { return "SSH key validation failed" }

type Service struct {
	db    *gorm.DB
	vault *shipcrypto.Vault
	audit audit.Recorder
}

func NewService(db *gorm.DB, vault *shipcrypto.Vault, recorder audit.Recorder) *Service {
	return &Service{db: db, vault: vault, audit: recorder}
}

func (service *Service) List(ctx context.Context) ([]KeyResource, error) {
	var rows []migrations.SSHKey
	if err := service.db.WithContext(ctx).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list SSH keys: %w", err)
	}
	keys := make([]KeyResource, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, response(row))
	}
	return keys, nil
}

func (service *Service) Get(ctx context.Context, keyID string) (KeyResource, error) {
	row, err := service.find(ctx, keyID)
	if err != nil {
		return KeyResource{}, err
	}
	return response(row), nil
}

// Create generates a new ed25519 keypair. The private key goes straight into
// the vault and is never part of the response.
func (service *Service) Create(ctx context.Context, requestContext RequestContext, name string) (KeyResource, error) {
	name, validationError := validateName(name)
	if validationError != nil {
		return KeyResource{}, validationError
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyResource{}, fmt.Errorf("generate keypair: %w", err)
	}
	block, err := cryptossh.MarshalPrivateKey(private, "")
	if err != nil {
		return KeyResource{}, fmt.Errorf("encode private key: %w", err)
	}
	sshPublic, err := cryptossh.NewPublicKey(public)
	if err != nil {
		return KeyResource{}, fmt.Errorf("encode public key: %w", err)
	}
	return service.store(ctx, requestContext, name, pem.EncodeToMemory(block), sshPublic)
}

// Import stores an operator-provided private key (PEM). The public key is
// derived, and the plaintext private key is discarded after encryption.
func (service *Service) Import(ctx context.Context, requestContext RequestContext, name, privatePEM string) (KeyResource, error) {
	name, validationError := validateName(name)
	if validationError != nil {
		return KeyResource{}, validationError
	}
	signer, err := cryptossh.ParsePrivateKey([]byte(privatePEM))
	if err != nil {
		return KeyResource{}, ErrInvalidImport
	}
	return service.store(ctx, requestContext, name, []byte(privatePEM), signer.PublicKey())
}

func (service *Service) store(ctx context.Context, requestContext RequestContext, name string, privatePEM []byte, publicKey cryptossh.PublicKey) (KeyResource, error) {
	id, err := identity.New()
	if err != nil {
		return KeyResource{}, err
	}
	publicLine := strings.TrimSpace(string(cryptossh.MarshalAuthorizedKey(publicKey))) + " ship-" + name

	row := migrations.SSHKey{ID: id, Name: name, PublicKey: publicLine}
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		vaultEntryID, err := service.vault.WithDB(tx).Store(ctx, shipcrypto.StoreInput{
			Kind: shipcrypto.KindSSHPrivateKey, ScopeType: "ssh_key", ScopeID: id, Name: name,
			Plaintext: privatePEM,
		})
		if err != nil {
			return fmt.Errorf("encrypt private key: %w", err)
		}
		row.VaultEntryID = vaultEntryID
		if err := tx.Create(&row).Error; err != nil {
			if uniqueViolation(err) {
				return ErrNameExists
			}
			return fmt.Errorf("create SSH key: %w", err)
		}
		return nil
	})
	if err != nil {
		return KeyResource{}, err
	}
	service.record(ctx, requestContext, "ssh_key.created", row)
	return response(row), nil
}

func (service *Service) Delete(ctx context.Context, requestContext RequestContext, keyID string) error {
	row, err := service.find(ctx, keyID)
	if err != nil {
		return err
	}
	var assigned int64
	if err := service.db.WithContext(ctx).Model(&migrations.Server{}).
		Where("ssh_key_id = ?", row.ID).Count(&assigned).Error; err != nil {
		return fmt.Errorf("count key assignments: %w", err)
	}
	if assigned > 0 {
		return ErrKeyInUse
	}
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&migrations.VaultEntry{}, "id = ?", row.VaultEntryID).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
	if err != nil {
		return fmt.Errorf("delete SSH key: %w", err)
	}
	service.record(ctx, requestContext, "ssh_key.deleted", row)
	return nil
}

// Signer decrypts the private key for the SSH transport. It is not exposed
// over HTTP — only the servers package calls it.
func (service *Service) Signer(ctx context.Context, keyID string) (cryptossh.Signer, error) {
	row, err := service.find(ctx, keyID)
	if err != nil {
		return nil, err
	}
	privatePEM, err := service.vault.Reveal(ctx, row.VaultEntryID)
	if err != nil {
		return nil, fmt.Errorf("decrypt private key: %w", err)
	}
	signer, err := cryptossh.ParsePrivateKey(privatePEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return signer, nil
}

func (service *Service) find(ctx context.Context, keyID string) (migrations.SSHKey, error) {
	if uuid.Validate(keyID) != nil {
		return migrations.SSHKey{}, ErrKeyNotFound
	}
	var row migrations.SSHKey
	err := service.db.WithContext(ctx).First(&row, "id = ?", keyID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return migrations.SSHKey{}, ErrKeyNotFound
	}
	if err != nil {
		return migrations.SSHKey{}, fmt.Errorf("find SSH key: %w", err)
	}
	return row, nil
}

func validateName(name string) (string, *ValidationError) {
	name = strings.TrimSpace(name)
	if length := utf8.RuneCountInString(name); length == 0 || length > 100 {
		return "", &ValidationError{Fields: []FieldViolation{{
			Field: "name", Code: "invalid", Message: "must be between 1 and 100 characters",
		}}}
	}
	return name, nil
}

func response(row migrations.SSHKey) KeyResource {
	return KeyResource{ID: row.ID, Name: row.Name, PublicKey: row.PublicKey, CreatedAt: row.CreatedAt.UTC()}
}

func uniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}

func (service *Service) record(ctx context.Context, requestContext RequestContext, action string, row migrations.SSHKey) {
	if service.audit == nil {
		return
	}
	_ = service.audit.Record(ctx, audit.Event{
		ActorUserID: requestContext.Actor.UserID, ActorEmail: requestContext.Actor.Email,
		Action: action, ResourceType: "ssh_key", ResourceID: row.ID,
		Outcome: audit.OutcomeSuccess, SourceIP: requestContext.SourceIP, RequestID: requestContext.RequestID,
		Metadata: map[string]any{"name": row.Name},
	})
}
