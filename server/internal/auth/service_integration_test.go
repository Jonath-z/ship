package auth

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
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/identity"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
	"github.com/Jonath-z/ship/server/migrations"
)

func TestAuthenticationLifecycleIntegration(t *testing.T) {
	databaseURL, redisURL := integrationURLs(t)
	ctx := context.Background()
	if err := database.MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	redis, err := shipredis.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer redis.Close()
	cleanAuthenticationFixtures(t, db.ORM, redis)
	defer cleanAuthenticationFixtures(t, db.ORM, redis)

	recorder := audit.NewService(db.ORM)
	cfg := testConfig()
	service, err := NewService(db.ORM, redis, cfg, recorder)
	if err != nil {
		t.Fatal(err)
	}
	user := createTestUser(t, db.ORM, "owner@example.com", "original secure password", access.RoleOwner)

	loggedIn, issued, _, err := service.Login(ctx, user.Email, "original secure password", "192.0.2.10", "login-request")
	if err != nil {
		t.Fatal(err)
	}
	if loggedIn.ID != user.ID || issued.Token == "" || issued.Record.CSRFToken == "" {
		t.Fatalf("unexpected login result: %#v %#v", loggedIn, issued)
	}
	logoutSession, err := service.IssueForUser(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	logoutContext := newSessionRequest(service.CookieName(), logoutSession.Token)
	if _, _, err := service.Authenticate(logoutContext); err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(logoutContext); err != nil {
		t.Fatal(err)
	}
	if exists := redis.Exists(ctx, sessionKeyPrefix+logoutSession.Digest).Val(); exists != 0 {
		t.Fatal("logout did not destroy the server-side session")
	}

	requestContext := access.Principal{UserID: user.ID, Email: user.Email, Role: access.RoleOwner}
	replacement, err := service.ChangePassword(ctx, requestContext, "original secure password", "replacement secure password")
	if err != nil {
		t.Fatal(err)
	}
	if exists := redis.Exists(ctx, sessionKeyPrefix+issued.Digest).Val(); exists != 0 {
		t.Fatal("password change did not invalidate the prior session")
	}
	if replacement.Token == issued.Token {
		t.Fatal("password change did not rotate the session")
	}
	if _, _, _, err := service.Login(ctx, user.Email, "original secure password", "192.0.2.10", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, _, _, err := service.Login(ctx, user.Email, "replacement secure password", "192.0.2.10", "new-password"); err != nil {
		t.Fatalf("new password should succeed: %v", err)
	}

	service.now = func() time.Time { return replacement.Record.ExpiresAt.Add(time.Second) }
	gin.SetMode(gin.TestMode)
	request := newSessionRequest(service.CookieName(), replacement.Token)
	if _, _, err := service.Authenticate(request); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session should be rejected, got %v", err)
	}
}

func TestUnauthorizedRoleReturnsForbiddenIntegration(t *testing.T) {
	databaseURL, redisURL := integrationURLs(t)
	ctx := context.Background()
	if err := database.MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	redis, err := shipredis.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer redis.Close()
	cleanAuthenticationFixtures(t, db.ORM, redis)
	defer cleanAuthenticationFixtures(t, db.ORM, redis)
	service, err := NewService(db.ORM, redis, testConfig(), audit.NewService(db.ORM))
	if err != nil {
		t.Fatal(err)
	}
	viewer := createTestUser(t, db.ORM, "viewer@example.com", "viewer secure password", access.RoleViewer)
	issued, err := service.IssueForUser(ctx, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	routes := httpx.NewRouter(engine, service.Authorize)
	routes.GET("/owner-area", access.UsersManage, func(c *gin.Context) { c.Status(204) })
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://ship.test/owner-area", nil)
	request.Header.Set("Cookie", service.CookieName()+"="+issued.Token)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != 403 {
		t.Fatalf("viewer received %d, want 403", recorder.Code)
	}
}

func TestLoginLocksAccountAfterFailedAttemptsIntegration(t *testing.T) {
	databaseURL, redisURL := integrationURLs(t)
	ctx := context.Background()
	if err := database.MigrateUp(ctx, databaseURL); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	redis, err := shipredis.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer redis.Close()
	cleanAuthenticationFixtures(t, db.ORM, redis)
	defer cleanAuthenticationFixtures(t, db.ORM, redis)
	service, err := NewService(db.ORM, redis, testConfig(), audit.NewService(db.ORM))
	if err != nil {
		t.Fatal(err)
	}
	createTestUser(t, db.ORM, "locked@example.com", "correct secure password", access.RoleViewer)

	for attempt := int64(1); attempt <= accountFailureLimit; attempt++ {
		_, _, _, err := service.Login(ctx, "locked@example.com", "wrong password", "192.0.2.20", "failed-login")
		if attempt < accountFailureLimit && !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: expected invalid credentials, got %v", attempt, err)
		}
		if attempt == accountFailureLimit && !errors.Is(err, ErrRateLimited) {
			t.Fatalf("attempt %d: expected rate limit, got %v", attempt, err)
		}
	}
	if _, _, _, err := service.Login(ctx, "locked@example.com", "correct secure password", "192.0.2.20", "blocked-login"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("locked account should remain rate limited, got %v", err)
	}
}

func testConfig() config.Config {
	return config.Config{
		Environment: "test", PublicURL: "http://ship.test", PublicOrigin: "http://ship.test",
		SessionSecret: "0123456789abcdef0123456789abcdef", SessionIdleTTL: time.Hour,
		SessionAbsoluteTTL: 24 * time.Hour,
	}
}

func integrationURLs(t *testing.T) (string, string) {
	t.Helper()
	databaseURL := os.Getenv("SHIP_TEST_DATABASE_URL")
	redisURL := os.Getenv("SHIP_TEST_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("SHIP_TEST_DATABASE_URL and SHIP_TEST_REDIS_URL are required")
	}
	return databaseURL, redisURL
}

func cleanAuthenticationFixtures(t *testing.T, db *gorm.DB, redis *redisclient.Client) {
	t.Helper()
	if err := db.Exec("TRUNCATE TABLE audit_logs, users CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	if err := redis.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
}

func newSessionRequest(cookieName, token string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest("GET", "http://ship.test/auth/session", nil)
	request.Header.Set("Cookie", cookieName+"="+token)
	c.Request = request
	return c
}

func createTestUser(t *testing.T, db *gorm.DB, email, password string, role access.Role) migrations.User {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.New()
	if err != nil {
		t.Fatal(err)
	}
	user := migrations.User{ID: id, Email: email, PasswordHash: hash, Role: string(role)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}
