package webhooks

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/configuration"
	"github.com/Jonath-z/ship/server/internal/deployments"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	shipcrypto "github.com/Jonath-z/ship/server/internal/platform/crypto"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/jobs"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestGitHubWebhookQueuesDeployments(t *testing.T) {
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	redisURL := os.Getenv("SHIP_TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL and SHIP_TEST_REDIS_URL are required")
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
	if err := connection.ORM.Exec("TRUNCATE TABLE projects, vault_entries CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	defer connection.ORM.Exec("TRUNCATE TABLE projects, vault_entries CASCADE")

	options, err := redisclient.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	redis := redisclient.NewClient(options)
	defer redis.Close()

	vault := shipcrypto.NewVault(connection.ORM, shipcrypto.StaticKeyProvider{Key: strings.Repeat("11", 32)}, nil)
	project := migrations.Project{ID: uuid.NewString(), Name: "Acme", Slug: "acme"}
	environment := migrations.Environment{ID: uuid.NewString(), ProjectID: project.ID, Name: "Production", Slug: "production"}
	port := 3000
	webapp := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID,
		Name: "webapp", Type: "web", Repository: "github.com/acme/webapp", Branch: "main", Port: &port,
	}
	imageBacked := migrations.Service{
		ID: uuid.NewString(), EnvironmentID: environment.ID,
		Name: "api", Type: "web", Image: "acme/api:v1", Port: &port,
	}
	webhookSecret := migrations.Secret{ID: uuid.NewString(), EnvironmentID: environment.ID, Name: configuration.WebhookSecretKey}
	for _, value := range []any{&project, &environment, &webapp, &imageBacked, &webhookSecret} {
		if err := connection.ORM.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := vault.Store(ctx, shipcrypto.StoreInput{
		SecretID: &webhookSecret.ID, Kind: shipcrypto.KindApplicationSecret,
		ScopeType: "environment", ScopeID: environment.ID, Name: webhookSecret.Name,
		Plaintext: []byte("hook-secret"),
	}); err != nil {
		t.Fatal(err)
	}

	deploymentService := deployments.NewService(connection.ORM, redis, jobs.NewQueue(redis, slog.Default()), nil)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	routes := httpx.NewRouter(engine, func(access.Permission) gin.HandlerFunc {
		return func(c *gin.Context) { c.AbortWithStatus(500) } // public route must never hit authorize
	})
	RegisterRoutes(routes, config.Config{}, NewService(connection.ORM, vault, deploymentService))

	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"acme/webapp","default_branch":"main"}}`)
	post := func(signature, event string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
		request.Header.Set("X-GitHub-Event", event)
		if signature != "" {
			request.Header.Set("X-Hub-Signature-256", signature)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder
	}

	// Ping never touches the database.
	if response := post("", "ping"); response.Code != 200 {
		t.Fatalf("ping = %d: %s", response.Code, response.Body)
	}
	// A verified push queues exactly the matching repository-backed service.
	if response := post(sign(body, "hook-secret"), "push"); response.Code != 202 ||
		!strings.Contains(response.Body.String(), `"queued":1`) {
		t.Fatalf("verified push = %d: %s", response.Code, response.Body)
	}
	var queued []migrations.Deployment
	if err := connection.ORM.Find(&queued, "environment_id = ?", environment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].ServiceID != webapp.ID || queued[0].Status != string(deployments.StatusQueued) {
		t.Fatalf("deployments = %#v", queued)
	}
	// A bad signature queues nothing and reveals nothing.
	if response := post(sign(body, "wrong"), "push"); response.Code != 202 ||
		!strings.Contains(response.Body.String(), `"queued":0`) {
		t.Fatalf("unverified push = %d: %s", response.Code, response.Body)
	}
	var count int64
	connection.ORM.Model(&migrations.Deployment{}).Where("environment_id = ?", environment.ID).Count(&count)
	if count != 1 {
		t.Fatalf("deployment count after bad signature = %d", count)
	}
}
