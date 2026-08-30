package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	"github.com/Jonath-z/ship/server/migrations"
)

const envelopeFormatVersion = 1

const (
	KindApplicationSecret = "application_secret"
	KindSSHPrivateKey     = "ssh_private_key"
	KindGitToken          = "git_token"
)

var ErrVaultEntryNotFound = errors.New("encrypted value was not found")

type Vault struct {
	db       *gorm.DB
	provider KeyProvider
	audit    audit.Recorder
}

func NewVault(db *gorm.DB, provider KeyProvider, recorder audit.Recorder) *Vault {
	return &Vault{db: db, provider: provider, audit: recorder}
}

type StoreInput struct {
	SecretID  *string
	Kind      string
	ScopeType string
	ScopeID   string
	Name      string
	Plaintext []byte
}

func (vault *Vault) Store(ctx context.Context, input StoreInput) (string, error) {
	if !validKind(input.Kind) || input.ScopeType == "" || input.ScopeID == "" || input.Name == "" {
		return "", errors.New("encrypted value kind, scope, and name are required")
	}
	id, err := identity.New()
	if err != nil {
		return "", err
	}
	keyring, err := vault.provider.Load(ctx)
	if err != nil {
		return "", err
	}
	masterKey, err := keyring.ActiveKey()
	if err != nil {
		return "", err
	}
	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return "", fmt.Errorf("generate data encryption key: %w", err)
	}
	dataNonce, ciphertext, err := seal(dataKey, input.Plaintext, associatedData(id, input.Kind, "data"))
	if err != nil {
		return "", err
	}
	wrapNonce, wrappedKey, err := seal(masterKey, dataKey, associatedData(id, input.Kind, "dek"))
	if err != nil {
		return "", err
	}
	row := migrations.VaultEntry{
		ID: id, SecretID: input.SecretID, Kind: input.Kind, ScopeType: input.ScopeType, ScopeID: input.ScopeID,
		Name: input.Name, KeyID: keyring.Active, FormatVersion: envelopeFormatVersion,
		Ciphertext: ciphertext, DataNonce: dataNonce, WrappedDEK: wrappedKey, WrapNonce: wrapNonce,
	}
	if err := vault.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", fmt.Errorf("store encrypted value: %w", err)
	}
	return id, nil
}

// WithDB returns a vault bound to db. It lets a feature store metadata and its
// encrypted value in the same database transaction.
func (vault *Vault) WithDB(db *gorm.DB) *Vault {
	return &Vault{db: db, provider: vault.provider, audit: vault.audit}
}

// Replace encrypts a new plaintext value for an existing vault entry while
// preserving its identity and metadata.
func (vault *Vault) Replace(ctx context.Context, id string, plaintext []byte) error {
	var row migrations.VaultEntry
	if err := vault.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrVaultEntryNotFound
		}
		return fmt.Errorf("find encrypted value: %w", err)
	}
	keyring, err := vault.provider.Load(ctx)
	if err != nil {
		return err
	}
	masterKey, err := keyring.ActiveKey()
	if err != nil {
		return err
	}
	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return fmt.Errorf("generate data encryption key: %w", err)
	}
	dataNonce, ciphertext, err := seal(dataKey, plaintext, associatedData(row.ID, row.Kind, "data"))
	if err != nil {
		return err
	}
	wrapNonce, wrappedKey, err := seal(masterKey, dataKey, associatedData(row.ID, row.Kind, "dek"))
	if err != nil {
		return err
	}
	result := vault.db.WithContext(ctx).Model(&migrations.VaultEntry{}).Where("id = ?", row.ID).Updates(map[string]any{
		"key_id": keyring.Active, "format_version": envelopeFormatVersion,
		"ciphertext": ciphertext, "data_nonce": dataNonce,
		"wrapped_dek": wrappedKey, "wrap_nonce": wrapNonce,
		"updated_at": time.Now().UTC(),
	})
	if result.Error != nil {
		return fmt.Errorf("replace encrypted value: %w", result.Error)
	}
	return nil
}

func (vault *Vault) Reveal(ctx context.Context, id string) ([]byte, error) {
	var row migrations.VaultEntry
	if err := vault.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrVaultEntryNotFound
		}
		return nil, fmt.Errorf("find encrypted value: %w", err)
	}
	return vault.decrypt(ctx, row)
}

func (vault *Vault) decrypt(ctx context.Context, row migrations.VaultEntry) ([]byte, error) {
	if row.FormatVersion != envelopeFormatVersion {
		return nil, fmt.Errorf("unsupported encryption format version %d", row.FormatVersion)
	}
	keyring, err := vault.provider.Load(ctx)
	if err != nil {
		return nil, err
	}
	masterKey, ok := keyring.Keys[row.KeyID]
	if !ok {
		return nil, fmt.Errorf("master key %q is not available", row.KeyID)
	}
	dataKey, err := open(masterKey, row.WrapNonce, row.WrappedDEK, associatedData(row.ID, row.Kind, "dek"))
	if err != nil {
		return nil, errors.New("unwrap encrypted data key: authentication failed")
	}
	plaintext, err := open(dataKey, row.DataNonce, row.Ciphertext, associatedData(row.ID, row.Kind, "data"))
	if err != nil {
		return nil, errors.New("decrypt value: authentication failed")
	}
	return plaintext, nil
}

type RotationResult struct {
	ActiveKeyID string
	Rewrapped   int64
}

// Rotate rewraps data keys in small transactions. The provider retains old and
// new master keys during the operation, so concurrent reads remain available.
func (vault *Vault) Rotate(ctx context.Context) (RotationResult, error) {
	keyring, err := vault.provider.Load(ctx)
	if err != nil {
		return RotationResult{}, err
	}
	activeKey, err := keyring.ActiveKey()
	if err != nil {
		return RotationResult{}, err
	}
	result := RotationResult{ActiveKeyID: keyring.Active}
	for {
		var rows []migrations.VaultEntry
		if err := vault.db.WithContext(ctx).Where("key_id <> ?", keyring.Active).Order("id").Limit(100).Find(&rows).Error; err != nil {
			return result, fmt.Errorf("list values for key rotation: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			oldKey, ok := keyring.Keys[row.KeyID]
			if !ok {
				return result, fmt.Errorf("master key %q is required to rotate value %s", row.KeyID, row.ID)
			}
			dataKey, err := open(oldKey, row.WrapNonce, row.WrappedDEK, associatedData(row.ID, row.Kind, "dek"))
			if err != nil {
				return result, fmt.Errorf("unwrap value %s during rotation: %w", row.ID, err)
			}
			nonce, wrapped, err := seal(activeKey, dataKey, associatedData(row.ID, row.Kind, "dek"))
			if err != nil {
				return result, err
			}
			update := vault.db.WithContext(ctx).Model(&migrations.VaultEntry{}).
				Where("id = ? AND key_id = ?", row.ID, row.KeyID).
				Updates(map[string]any{"key_id": keyring.Active, "wrap_nonce": nonce, "wrapped_dek": wrapped})
			if update.Error != nil {
				return result, fmt.Errorf("rewrap value %s: %w", row.ID, update.Error)
			}
			result.Rewrapped += update.RowsAffected
		}
	}
	if vault.audit != nil {
		_ = vault.audit.Record(ctx, audit.Event{
			Action: "encryption.rotated", ResourceType: "encryption_key",
			ResourceID: result.ActiveKeyID, Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{"rewrapped": result.Rewrapped},
		})
	}
	return result, nil
}

func seal(key, plaintext, additionalData []byte) ([]byte, []byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate encryption nonce: %w", err)
	}
	return nonce, aead.Seal(nil, nonce, plaintext, additionalData), nil
}

func open(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, errors.New("invalid encryption nonce")
	}
	return aead.Open(nil, nonce, ciphertext, additionalData)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("AES-256 key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func associatedData(id, kind, purpose string) []byte {
	return []byte(fmt.Sprintf("ship:v%d:%s:%s:%s", envelopeFormatVersion, id, kind, purpose))
}

func validKind(kind string) bool {
	return kind == KindApplicationSecret || kind == KindSSHPrivateKey || kind == KindGitToken
}
