// Package webhooks turns GitHub push events into deployments. No GitHub App
// or OAuth is involved: the operator adds a webhook to the repository by hand
// and stores the same shared secret in Ship as the environment secret
// GITHUB_WEBHOOK_SECRET. A push then deploys every repository-backed service
// in environments whose secret verifies the payload signature — the HMAC is
// the authentication, which is why the route is public.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/configuration"
	"github.com/Jonath-z/ship/server/internal/deployments"
	"github.com/Jonath-z/ship/server/internal/gitsource"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/migrations"
)

type Service struct {
	db          *gorm.DB
	vault       *shipcrypto.Vault
	deployments *deployments.Service
}

func NewService(db *gorm.DB, vault *shipcrypto.Vault, deploymentService *deployments.Service) *Service {
	return &Service{db: db, vault: vault, deployments: deploymentService}
}

// pushEvent is the slice of GitHub's push payload Ship needs.
type pushEvent struct {
	Ref        string `json:"ref"`
	Deleted    bool   `json:"deleted"`
	Repository struct {
		FullName      string `json:"full_name"`
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
}

var errInvalidPayload = errors.New("payload is not a GitHub push event")

// HandlePush matches a push event against repository-backed services and
// queues a deployment for each one whose environment's webhook secret
// verifies the signature. It reports how many were queued; verification
// failures are skipped silently — an unauthenticated caller learns nothing
// about what exists.
func (service *Service) HandlePush(ctx context.Context, body []byte, signature string, requestContext deployments.RequestContext) (int, error) {
	var event pushEvent
	if err := json.Unmarshal(body, &event); err != nil || event.Repository.FullName == "" {
		return 0, errInvalidPayload
	}
	branch, isBranch := strings.CutPrefix(event.Ref, "refs/heads/")
	if !isBranch || event.Deleted {
		return 0, nil // tag pushes and branch deletions never deploy
	}

	var candidates []migrations.Service
	err := service.db.WithContext(ctx).
		Where("repository <> '' AND image = ''").Order("name ASC").Find(&candidates).Error
	if err != nil {
		return 0, fmt.Errorf("list services: %w", err)
	}

	verified := map[string]bool{} // environment id -> signature outcome
	queued := 0
	for _, candidate := range candidates {
		if !repositoryMatches(candidate.Repository, event.Repository.FullName) {
			continue
		}
		serviceBranch := candidate.Branch
		if serviceBranch == "" {
			serviceBranch = event.Repository.DefaultBranch
		}
		if serviceBranch != branch {
			continue
		}
		environmentOK, checked := verified[candidate.EnvironmentID]
		if !checked {
			secret, found, err := service.webhookSecret(ctx, candidate.EnvironmentID)
			if err != nil {
				return queued, err
			}
			environmentOK = found && signatureValid(body, signature, secret)
			verified[candidate.EnvironmentID] = environmentOK
		}
		if !environmentOK {
			continue
		}
		var environment migrations.Environment
		if err := service.db.WithContext(ctx).First(&environment, "id = ?", candidate.EnvironmentID).Error; err != nil {
			return queued, fmt.Errorf("load environment: %w", err)
		}
		if _, err := service.deployments.Create(ctx, requestContext, environment.ProjectID, environment.ID, candidate.ID); err != nil {
			return queued, fmt.Errorf("queue deployment for service %s: %w", candidate.Name, err)
		}
		queued++
	}
	return queued, nil
}

// webhookSecret reveals the environment's GITHUB_WEBHOOK_SECRET value.
func (service *Service) webhookSecret(ctx context.Context, environmentID string) (string, bool, error) {
	var secret migrations.Secret
	err := service.db.WithContext(ctx).
		First(&secret, "environment_id = ? AND service_id IS NULL AND name = ?",
			environmentID, configuration.WebhookSecretKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var entry migrations.VaultEntry
	err = service.db.WithContext(ctx).First(&entry, "secret_id = ?", secret.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plaintext, err := service.vault.Reveal(ctx, entry.ID)
	if err != nil {
		return "", false, fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return string(plaintext), true, nil
}

// signatureValid checks GitHub's X-Hub-Signature-256 header:
// "sha256=" + hex(HMAC-SHA256(secret, body)).
func signatureValid(body []byte, signature, secret string) bool {
	provided, hasPrefix := strings.CutPrefix(signature, "sha256=")
	if !hasPrefix || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(provided)))
}

// repositoryMatches compares a service's stored repository against the
// pushed repository's owner/name. Only github.com repositories can match —
// the event came from GitHub.
func repositoryMatches(stored, fullName string) bool {
	normalized, err := gitsource.NormalizeURL(stored)
	if err != nil {
		return false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), "github.com") {
		return false
	}
	path := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	return strings.EqualFold(path, fullName)
}
