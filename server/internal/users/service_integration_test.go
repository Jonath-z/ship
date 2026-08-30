package users

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	redisclient "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/auth"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestRoleGuardsAndSessionInvalidationIntegration(t *testing.T) {
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
	redis, err := shipredis.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer redis.Close()
	cleanUserFixtures(t, connection.ORM, redis)
	defer cleanUserFixtures(t, connection.ORM, redis)

	recorder := audit.NewService(connection.ORM)
	authService, err := auth.NewService(connection.ORM, redis, config.Config{
		Environment: "test", PublicURL: "http://ship.test", PublicOrigin: "http://ship.test",
		SessionSecret: "0123456789abcdef0123456789abcdef", SessionIdleTTL: time.Hour,
		SessionAbsoluteTTL: 24 * time.Hour,
	}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(connection.ORM, authService, recorder)
	owner := createFixtureUser(t, connection.ORM, "owner@example.com", access.RoleOwner)
	admin := createFixtureUser(t, connection.ORM, "admin@example.com", access.RoleAdmin)
	viewer := createFixtureUser(t, connection.ORM, "viewer@example.com", access.RoleViewer)

	ownerContext := RequestContext{Actor: access.Principal{UserID: owner.ID, Email: owner.Email, Role: access.RoleOwner}}
	if _, err := service.ChangeRole(ctx, ownerContext, owner.ID, access.RoleViewer); !errors.Is(err, ErrOwnerRequired) {
		t.Fatalf("last owner demotion should fail with ErrOwnerRequired, got %v", err)
	}
	adminContext := RequestContext{Actor: access.Principal{UserID: admin.ID, Email: admin.Email, Role: access.RoleAdmin}}
	if _, err := service.ChangeRole(ctx, adminContext, owner.ID, access.RoleAdmin); !errors.Is(err, ErrOwnerOnly) {
		t.Fatalf("admin should not manage an owner, got %v", err)
	}

	issued, err := authService.IssueForUser(ctx, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := authService.Authenticate(sessionContext(authService.CookieName(), issued.Token)); err != nil {
		t.Fatalf("fixture session should authenticate: %v", err)
	}
	updated, err := service.ChangeRole(ctx, ownerContext, viewer.ID, access.RoleDeployer)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != access.RoleDeployer {
		t.Fatalf("updated role = %q, want deployer", updated.Role)
	}
	if _, _, err := authService.Authenticate(sessionContext(authService.CookieName(), issued.Token)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("role change should invalidate existing sessions, got %v", err)
	}
}

func cleanUserFixtures(t *testing.T, db *gorm.DB, redis *redisclient.Client) {
	t.Helper()
	if err := db.Exec("TRUNCATE TABLE audit_logs, users CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	if err := redis.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
}

func createFixtureUser(t *testing.T, db *gorm.DB, email string, role access.Role) migrations.User {
	t.Helper()
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("fixture secure password")
	if err != nil {
		t.Fatal(err)
	}
	user := migrations.User{ID: id, Email: email, PasswordHash: hash, Role: string(role)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func sessionContext(cookieName, token string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest("GET", "http://ship.test/auth/session", nil)
	request.Header.Set("Cookie", cookieName+"="+token)
	c.Request = request
	return c
}
