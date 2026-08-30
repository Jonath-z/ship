// Package crypto provides envelope encryption for Ship's sensitive values.
package crypto

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Jonath-z/ship/server/internal/platform/config"
)

type Keyring struct {
	Active string
	Keys   map[string][]byte
}

func (keyring Keyring) ActiveKey() ([]byte, error) {
	key, ok := keyring.Keys[keyring.Active]
	if !ok {
		return nil, errors.New("active encryption key is missing from keyring")
	}
	return key, nil
}

type KeyProvider interface {
	Load(context.Context) (Keyring, error)
}

func ProviderFromConfig(cfg config.Config) (KeyProvider, error) {
	var provider KeyProvider
	if cfg.KeyringPath != "" {
		provider = FileKeyProvider{Path: cfg.KeyringPath}
	} else if cfg.MasterKey != "" {
		provider = StaticKeyProvider{Key: cfg.MasterKey}
	} else {
		return nil, errors.New("no encryption master key is configured")
	}
	if _, err := provider.Load(context.Background()); err != nil {
		return nil, err
	}
	return provider, nil
}

type FileKeyProvider struct {
	Path string
}

// Load reparses the small keyring file on every operation. This allows the host
// CLI to stage old and new keys atomically while the API stays online.
func (provider FileKeyProvider) Load(_ context.Context) (Keyring, error) {
	file, err := os.Open(provider.Path)
	if err != nil {
		return Keyring{}, fmt.Errorf("open encryption keyring: %w", err)
	}
	defer file.Close()
	keyring := Keyring{Keys: map[string][]byte{}}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		switch {
		case len(parts) == 2 && parts[0] == "active":
			keyring.Active = parts[1]
		case len(parts) == 3 && parts[0] == "key":
			if !validKeyID(parts[1]) {
				return Keyring{}, fmt.Errorf("invalid key ID on keyring line %d", lineNumber)
			}
			key, decodeErr := decodeKey(parts[2])
			if decodeErr != nil {
				return Keyring{}, fmt.Errorf("invalid key on keyring line %d: %w", lineNumber, decodeErr)
			}
			if _, exists := keyring.Keys[parts[1]]; exists {
				return Keyring{}, fmt.Errorf("duplicate key ID %q", parts[1])
			}
			keyring.Keys[parts[1]] = key
		default:
			return Keyring{}, fmt.Errorf("invalid encryption keyring line %d", lineNumber)
		}
	}
	if err := scanner.Err(); err != nil {
		return Keyring{}, fmt.Errorf("read encryption keyring: %w", err)
	}
	if !validKeyID(keyring.Active) {
		return Keyring{}, errors.New("encryption keyring has no valid active key")
	}
	if _, err := keyring.ActiveKey(); err != nil {
		return Keyring{}, err
	}
	return keyring, nil
}

type StaticKeyProvider struct {
	Key string
}

func (provider StaticKeyProvider) Load(_ context.Context) (Keyring, error) {
	key, err := decodeKey(provider.Key)
	if err != nil {
		return Keyring{}, err
	}
	id := KeyID(key)
	return Keyring{Active: id, Keys: map[string][]byte{id: key}}, nil
}

func KeyID(key []byte) string {
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:8])
}

func decodeKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	return nil, errors.New("master key must encode exactly 32 bytes")
}

func validKeyID(value string) bool {
	if len(value) < 8 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}
