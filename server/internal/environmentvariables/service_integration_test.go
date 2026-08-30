package environmentvariables

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestVariablesAndSecretsIntegration(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	if err := database.MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	connection, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.ORM.Exec("TRUNCATE TABLE projects, audit_logs CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects, audit_logs CASCADE")

	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	otherEnvironment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Staging", Slug: "staging"}
	serviceRow := migrations.Service{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	otherService := migrations.Service{ID: uuid.NewString(), EnvironmentID: otherEnvironment.ID, Name: "api", Type: "web", Image: "acme/api:latest"}
	for _, value := range []any{&project, &environment, &otherEnvironment, &serviceRow, &otherService} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}

	auditService := audit.NewService(connection.ORM)
	vault := shipcrypto.NewVault(connection.ORM, shipcrypto.StaticKeyProvider{Key: strings.Repeat("11", 32)}, auditService)
	service := NewService(NewRepository(connection.ORM), vault, auditService)

	globalVariable, err := service.CreateVariable(ctx, RequestContext{}, project.ID, environment.ID, CreateVariableInput{Name: "NODE_ENV", Value: "production"})
	if err != nil {
		t.Fatal(err)
	}
	serviceVariable, err := service.CreateVariable(ctx, RequestContext{}, project.ID, environment.ID, CreateVariableInput{ServiceID: &serviceRow.ID, Name: "NODE_ENV", Value: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if globalVariable.ServiceID != nil || serviceVariable.ServiceID == nil {
		t.Fatalf("variable scopes = %#v / %#v", globalVariable, serviceVariable)
	}
	if _, err := service.CreateVariable(ctx, RequestContext{}, project.ID, environment.ID, CreateVariableInput{ServiceID: &otherService.ID, Name: "PORT", Value: "3000"}); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("cross-environment service error = %v", err)
	}

	secretValue := "postgres://ship:very-secret@postgres:5432/acme"
	secret, err := service.CreateSecret(ctx, RequestContext{}, project.ID, environment.ID, CreateSecretInput{Name: "DATABASE_URL", Value: secretValue})
	if err != nil {
		t.Fatal(err)
	}
	if !secret.HasValue || secret.Name != "DATABASE_URL" {
		t.Fatalf("secret metadata = %#v", secret)
	}
	encodedSecret, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encodedSecret, []byte(secretValue)) || bytes.Contains(encodedSecret, []byte(`"value"`)) {
		t.Fatalf("secret metadata response exposed a value: %s", encodedSecret)
	}
	serviceSecret, err := service.CreateSecret(ctx, RequestContext{}, project.ID, environment.ID, CreateSecretInput{
		ServiceID: &serviceRow.ID, Name: "DATABASE_URL", Value: "service-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if serviceSecret.ServiceID == nil || *serviceSecret.ServiceID != serviceRow.ID {
		t.Fatalf("service-scoped secret metadata = %#v", serviceSecret)
	}
	if _, err := service.CreateVariable(ctx, RequestContext{}, project.ID, environment.ID, CreateVariableInput{Name: "DATABASE_URL", Value: "plaintext"}); !errors.Is(err, ErrNameExists) {
		t.Fatalf("cross-tier duplicate error = %v", err)
	}
	var raw migrations.VaultEntry
	if err := connection.ORM.First(&raw, "secret_id = ?", secret.ID).Error; err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw.Ciphertext, []byte(secretValue)) || bytes.Contains(raw.Ciphertext, []byte("very-secret")) {
		t.Fatal("raw database ciphertext exposed the secret")
	}
	revealed, err := service.RevealSecret(ctx, RequestContext{}, project.ID, environment.ID, secret.ID)
	if err != nil || revealed != secretValue {
		t.Fatalf("revealed = %q, error = %v", revealed, err)
	}
	replacement := "postgres://ship:new-secret@postgres:5432/acme"
	if _, err := service.UpdateSecret(ctx, RequestContext{}, project.ID, environment.ID, secret.ID, UpdateSecretInput{Value: &replacement}); err != nil {
		t.Fatal(err)
	}
	revealed, err = service.RevealSecret(ctx, RequestContext{}, project.ID, environment.ID, secret.ID)
	if err != nil || revealed != replacement {
		t.Fatalf("replacement reveal = %q, error = %v", revealed, err)
	}

	variableImport, err := service.ImportVariables(ctx, RequestContext{}, project.ID, environment.ID, ImportInput{Content: "NODE_ENV=staging\nPORT=3000\n"})
	if err != nil || variableImport.Created != 1 || variableImport.Updated != 1 {
		t.Fatalf("variable import = %#v, error = %v", variableImport, err)
	}
	secretImport, err := service.ImportSecrets(ctx, RequestContext{}, project.ID, environment.ID, ImportInput{Content: "DATABASE_URL=rotated\nJWT_SECRET=token\n"})
	if err != nil || secretImport.Created != 1 || secretImport.Updated != 1 {
		t.Fatalf("secret import = %#v, error = %v", secretImport, err)
	}
	revealed, err = service.RevealSecret(ctx, RequestContext{}, project.ID, environment.ID, secret.ID)
	if err != nil || revealed != "rotated" {
		t.Fatalf("imported reveal = %q, error = %v", revealed, err)
	}

	if err := service.DeleteSecret(ctx, RequestContext{}, project.ID, environment.ID, secret.ID); err != nil {
		t.Fatal(err)
	}
	var vaultCount int64
	if err := connection.ORM.Model(&migrations.VaultEntry{}).Where("secret_id = ?", secret.ID).Count(&vaultCount).Error; err != nil {
		t.Fatal(err)
	}
	if vaultCount != 0 {
		t.Fatalf("vault entries after secret deletion = %d", vaultCount)
	}
	var revealAuditCount int64
	if err := connection.ORM.Model(&migrations.AuditLog{}).Where("action = ?", "secret.revealed").Count(&revealAuditCount).Error; err != nil {
		t.Fatal(err)
	}
	if revealAuditCount != 3 {
		t.Fatalf("secret reveal audit count = %d", revealAuditCount)
	}
}

func TestProvisionPostgresConnectionIntegration(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	if err := database.MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	connection, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.ORM.Exec("TRUNCATE TABLE projects, audit_logs CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects, audit_logs CASCADE")
	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	if err := connection.ORM.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := connection.ORM.Create(&environment).Error; err != nil {
		t.Fatal(err)
	}
	vault := shipcrypto.NewVault(connection.ORM, shipcrypto.StaticKeyProvider{Key: strings.Repeat("22", 32)}, nil)
	service := NewService(NewRepository(connection.ORM), vault, nil)
	name, err := service.ProvisionPostgresConnection(ctx, access.Principal{}, "", "", project.ID, environment.ID, "Primary PostgreSQL", 5432)
	if err != nil || name != "DATABASE_URL" {
		t.Fatalf("provisioned name = %q, error = %v", name, err)
	}
	var secret migrations.Secret
	if err := connection.ORM.First(&secret, "environment_id = ? AND name = ?", environment.ID, name).Error; err != nil {
		t.Fatal(err)
	}
	revealed, err := service.RevealSecret(ctx, RequestContext{}, project.ID, environment.ID, secret.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(revealed, "postgres://ship:") || !strings.Contains(revealed, "@primary-postgresql:5432/primary_postgresql?sslmode=disable") {
		t.Fatalf("generated connection URL has unexpected safe shape: %q", revealed)
	}
	secondName, err := service.ProvisionPostgresConnection(ctx, access.Principal{}, "", "", project.ID, environment.ID, "Analytics PostgreSQL", 5432)
	if err != nil || secondName != "ANALYTICS_POSTGRESQL_DATABASE_URL" {
		t.Fatalf("second provisioned name = %q, error = %v", secondName, err)
	}
}
