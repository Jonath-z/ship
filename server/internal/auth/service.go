// Package auth owns local password authentication and server-side sessions.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	redisclient "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/audit"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	"github.com/Jonath-z/ship/server/internal/platform/ratelimit"
	"github.com/Jonath-z/ship/server/migrations"
)

const (
	secureCookieName    = "__Host-ship_session"
	insecureCookieName  = "ship_session"
	sessionKeyPrefix    = "ship:session:"
	userSessionsPrefix  = "ship:user-sessions:"
	accountFailureLimit = int64(5)
	ipFailureLimit      = int64(20)
	loginFailureWindow  = 15 * time.Minute
)

var (
	ErrInvalidCredentials = errors.New("email or password is incorrect")
	ErrRateLimited        = errors.New("too many login attempts")
	ErrUnauthenticated    = errors.New("authentication is required")
)

type sessionRecord struct {
	UserID    string    `json:"userId"`
	CSRFToken string    `json:"csrfToken"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type issuedSession struct {
	Token  string
	Digest string
	Record sessionRecord
}

type sessionContext struct {
	Digest string
	Record sessionRecord
}

const sessionContextKey = "shipSession"

type Service struct {
	db        *gorm.DB
	redis     *redisclient.Client
	config    config.Config
	secret    []byte
	limiter   *ratelimit.Limiter
	audit     audit.Recorder
	dummyHash string
	now       func() time.Time
}

func NewService(db *gorm.DB, redis *redisclient.Client, cfg config.Config, recorder audit.Recorder) (*Service, error) {
	if len(cfg.SessionSecret) < 32 {
		return nil, errors.New("SHIP_SESSION_SECRET must contain at least 32 characters")
	}
	dummyHash, err := HashPassword("ship-invalid-account-password-placeholder")
	if err != nil {
		return nil, err
	}
	return &Service{
		db: db, redis: redis, config: cfg, secret: []byte(cfg.SessionSecret),
		limiter: ratelimit.New(redis, cfg.SessionSecret), audit: recorder,
		dummyHash: dummyHash, now: time.Now,
	}, nil
}

type User struct {
	ID    string      `json:"id"`
	Email string      `json:"email"`
	Role  access.Role `json:"role"`
}

type Session struct {
	User      User      `json:"user"`
	CSRFToken string    `json:"csrfToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func userResponse(user migrations.User) User {
	return User{ID: user.ID, Email: user.Email, Role: access.Role(user.Role)}
}

func (service *Service) Login(ctx context.Context, email, password, sourceIP, requestID string) (migrations.User, issuedSession, time.Duration, error) {
	email = normalizeEmail(email)
	accountKey := service.limiter.Key("login-account", email)
	ipKey := service.limiter.Key("login-ip", sourceIP)
	for _, check := range []struct {
		key   string
		limit int64
	}{{accountKey, accountFailureLimit}, {ipKey, ipFailureLimit}} {
		decision, err := service.limiter.Current(ctx, check.key, check.limit)
		if err != nil {
			return migrations.User{}, issuedSession{}, 0, err
		}
		if decision.Blocked() {
			return migrations.User{}, issuedSession{}, decision.RetryAfter, ErrRateLimited
		}
	}

	var user migrations.User
	databaseErr := service.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	hash := service.dummyHash
	accountExists := databaseErr == nil && user.DisabledAt == nil
	if accountExists {
		hash = user.PasswordHash
	} else if databaseErr != nil && !errors.Is(databaseErr, gorm.ErrRecordNotFound) {
		return migrations.User{}, issuedSession{}, 0, fmt.Errorf("find user: %w", databaseErr)
	}
	valid, verifyErr := VerifyPassword(password, hash)
	if verifyErr != nil && accountExists {
		return migrations.User{}, issuedSession{}, 0, fmt.Errorf("verify password: %w", verifyErr)
	}
	if !accountExists || !valid {
		retryAfter, rateErr := service.recordLoginFailure(ctx, accountKey, ipKey)
		service.record(ctx, audit.Event{
			Action: "auth.login", ResourceType: "session", Outcome: audit.OutcomeFailure,
			SourceIP: sourceIP, RequestID: requestID,
		})
		if rateErr != nil {
			return migrations.User{}, issuedSession{}, 0, rateErr
		}
		if retryAfter > 0 {
			return migrations.User{}, issuedSession{}, retryAfter, ErrRateLimited
		}
		return migrations.User{}, issuedSession{}, 0, ErrInvalidCredentials
	}

	if PasswordNeedsRehash(user.PasswordHash) {
		updated, err := HashPassword(password)
		if err != nil {
			return migrations.User{}, issuedSession{}, 0, err
		}
		if err := service.db.WithContext(ctx).Model(&user).Update("password_hash", updated).Error; err != nil {
			return migrations.User{}, issuedSession{}, 0, fmt.Errorf("rehash password: %w", err)
		}
		user.PasswordHash = updated
	}
	issued, err := service.issue(ctx, user.ID)
	if err != nil {
		return migrations.User{}, issuedSession{}, 0, err
	}
	_ = service.limiter.Reset(ctx, accountKey)
	service.record(ctx, audit.Event{
		ActorUserID: user.ID, ActorEmail: user.Email, Action: "auth.login",
		ResourceType: "session", ResourceID: issued.Digest,
		Outcome: audit.OutcomeSuccess, SourceIP: sourceIP, RequestID: requestID,
	})
	return user, issued, 0, nil
}

func (service *Service) recordLoginFailure(ctx context.Context, accountKey, ipKey string) (time.Duration, error) {
	account, err := service.limiter.Hit(ctx, accountKey, accountFailureLimit, loginFailureWindow)
	if err != nil {
		return 0, err
	}
	ip, err := service.limiter.Hit(ctx, ipKey, ipFailureLimit, loginFailureWindow)
	if err != nil {
		return 0, err
	}
	if account.Blocked() {
		return account.RetryAfter, nil
	}
	if ip.Blocked() {
		return ip.RetryAfter, nil
	}
	return 0, nil
}

func (service *Service) IssueForUser(ctx context.Context, userID string) (issuedSession, error) {
	return service.issue(ctx, userID)
}

func (service *Service) issue(ctx context.Context, userID string) (issuedSession, error) {
	token, err := randomToken(32)
	if err != nil {
		return issuedSession{}, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return issuedSession{}, err
	}
	now := service.now().UTC()
	record := sessionRecord{
		UserID: userID, CSRFToken: csrfToken, CreatedAt: now,
		ExpiresAt: now.Add(service.config.SessionAbsoluteTTL),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return issuedSession{}, fmt.Errorf("encode session: %w", err)
	}
	digest := service.sessionDigest(token)
	key := sessionKeyPrefix + digest
	indexKey := userSessionsPrefix + userID
	_, err = service.redis.Pipelined(ctx, func(pipe redisclient.Pipeliner) error {
		pipe.Set(ctx, key, payload, service.config.SessionIdleTTL)
		pipe.SAdd(ctx, indexKey, digest)
		pipe.ExpireAt(ctx, indexKey, record.ExpiresAt)
		return nil
	})
	if err != nil {
		return issuedSession{}, fmt.Errorf("store session: %w", err)
	}
	return issuedSession{Token: token, Digest: digest, Record: record}, nil
}

func (service *Service) Authenticate(c *gin.Context) (migrations.User, sessionContext, error) {
	token, err := c.Cookie(service.CookieName())
	if err != nil || token == "" {
		return migrations.User{}, sessionContext{}, ErrUnauthenticated
	}
	digest := service.sessionDigest(token)
	key := sessionKeyPrefix + digest
	payload, err := service.redis.Get(c.Request.Context(), key).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return migrations.User{}, sessionContext{}, ErrUnauthenticated
	}
	if err != nil {
		return migrations.User{}, sessionContext{}, fmt.Errorf("read session: %w", err)
	}
	var record sessionRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		_ = service.redis.Del(c.Request.Context(), key).Err()
		return migrations.User{}, sessionContext{}, ErrUnauthenticated
	}
	now := service.now().UTC()
	if !now.Before(record.ExpiresAt) {
		_ = service.removeSession(c.Request.Context(), record.UserID, digest)
		return migrations.User{}, sessionContext{}, ErrUnauthenticated
	}
	var user migrations.User
	if err := service.db.WithContext(c.Request.Context()).First(&user, "id = ?", record.UserID).Error; err != nil || user.DisabledAt != nil {
		_ = service.removeSession(c.Request.Context(), record.UserID, digest)
		return migrations.User{}, sessionContext{}, ErrUnauthenticated
	}
	ttl := service.config.SessionIdleTTL
	if remaining := record.ExpiresAt.Sub(now); remaining < ttl {
		ttl = remaining
	}
	if ttl <= 0 || service.redis.Expire(c.Request.Context(), key, ttl).Err() != nil {
		return migrations.User{}, sessionContext{}, ErrUnauthenticated
	}
	session := sessionContext{Digest: digest, Record: record}
	access.SetPrincipal(c, access.Principal{UserID: user.ID, Email: user.Email, Role: access.Role(user.Role)})
	c.Set(sessionContextKey, session)
	return user, session, nil
}

func (service *Service) Authorize(permission access.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, session, err := service.Authenticate(c)
		if err != nil {
			service.ClearCookie(c)
			httpx.WriteError(c, 401, "authentication_required", "authentication is required", nil)
			return
		}
		principal, _ := access.PrincipalFrom(c)
		if permission != access.Authenticated && !access.Allowed(principal.Role, permission) {
			httpx.WriteError(c, 403, "permission_denied", "you do not have permission to perform this action", nil)
			return
		}
		if isMutation(c.Request.Method) && !service.validMutation(c, session.Record.CSRFToken) {
			httpx.WriteError(c, 403, "csrf_rejected", "request origin or CSRF token is invalid", nil)
			return
		}
		c.Next()
	}
}

func (service *Service) SameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !service.validOrigin(c.GetHeader("Origin")) {
			httpx.WriteError(c, 403, "origin_rejected", "request origin is invalid", nil)
			return
		}
		c.Next()
	}
}

func (service *Service) validMutation(c *gin.Context, expectedToken string) bool {
	if !service.validOrigin(c.GetHeader("Origin")) {
		return false
	}
	actualToken := c.GetHeader("X-CSRF-Token")
	return len(actualToken) == len(expectedToken) &&
		subtle.ConstantTimeCompare([]byte(actualToken), []byte(expectedToken)) == 1
}

func (service *Service) validOrigin(origin string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	return origin != "" && origin == service.config.PublicOrigin
}

func (service *Service) CurrentSession(c *gin.Context) (Session, error) {
	if value, ok := c.Get(sessionContextKey); ok {
		stored := value.(sessionContext)
		principal, principalOK := access.PrincipalFrom(c)
		if principalOK {
			return Session{
				User:      User{ID: principal.UserID, Email: principal.Email, Role: principal.Role},
				CSRFToken: stored.Record.CSRFToken, ExpiresAt: stored.Record.ExpiresAt,
			}, nil
		}
	}
	user, session, err := service.Authenticate(c)
	if err != nil {
		return Session{}, err
	}
	return Session{User: userResponse(user), CSRFToken: session.Record.CSRFToken, ExpiresAt: session.Record.ExpiresAt}, nil
}

func (service *Service) LimitByIP(policy string, limit int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		sourceIP := httpx.ClientIP(c, service.config.TrustForwardedIP)
		decision, err := service.limiter.Hit(
			c.Request.Context(), service.limiter.Key(policy, sourceIP), limit, window,
		)
		if err != nil {
			httpx.WriteError(c, 503, "rate_limit_unavailable", "request protection is temporarily unavailable", nil)
			return
		}
		if decision.Count > decision.Limit {
			seconds := int(decision.RetryAfter.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", fmt.Sprintf("%d", seconds))
			httpx.WriteError(c, 429, "rate_limited", "too many requests; try again later", nil)
			return
		}
		c.Next()
	}
}

func (service *Service) Logout(c *gin.Context) error {
	value, ok := c.Get(sessionContextKey)
	if !ok {
		return ErrUnauthenticated
	}
	session := value.(sessionContext)
	if err := service.removeSession(c.Request.Context(), session.Record.UserID, session.Digest); err != nil {
		return err
	}
	principal, _ := access.PrincipalFrom(c)
	service.record(c.Request.Context(), audit.Event{
		ActorUserID: principal.UserID, ActorEmail: principal.Email, Action: "auth.logout",
		ResourceType: "session", ResourceID: session.Digest, Outcome: audit.OutcomeSuccess,
		SourceIP: httpx.ClientIP(c, service.config.TrustForwardedIP), RequestID: c.GetString("requestID"),
	})
	service.ClearCookie(c)
	return nil
}

func (service *Service) ChangePassword(ctx context.Context, principal access.Principal, currentPassword, newPassword string) (issuedSession, error) {
	var user migrations.User
	if err := service.db.WithContext(ctx).First(&user, "id = ?", principal.UserID).Error; err != nil {
		return issuedSession{}, ErrUnauthenticated
	}
	valid, err := VerifyPassword(currentPassword, user.PasswordHash)
	if err != nil || !valid {
		return issuedSession{}, ErrInvalidCredentials
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return issuedSession{}, err
	}
	if err := service.db.WithContext(ctx).Model(&user).Update("password_hash", hash).Error; err != nil {
		return issuedSession{}, fmt.Errorf("change password: %w", err)
	}
	if err := service.InvalidateUserSessions(ctx, user.ID); err != nil {
		return issuedSession{}, err
	}
	return service.issue(ctx, user.ID)
}

func (service *Service) InvalidateUserSessions(ctx context.Context, userID string) error {
	indexKey := userSessionsPrefix + userID
	digests, err := service.redis.SMembers(ctx, indexKey).Result()
	if err != nil && !errors.Is(err, redisclient.Nil) {
		return fmt.Errorf("list user sessions: %w", err)
	}
	keys := make([]string, 0, len(digests)+1)
	for _, digest := range digests {
		keys = append(keys, sessionKeyPrefix+digest)
	}
	keys = append(keys, indexKey)
	if err := service.redis.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("invalidate user sessions: %w", err)
	}
	return nil
}

func (service *Service) removeSession(ctx context.Context, userID, digest string) error {
	_, err := service.redis.Pipelined(ctx, func(pipe redisclient.Pipeliner) error {
		pipe.Del(ctx, sessionKeyPrefix+digest)
		pipe.SRem(ctx, userSessionsPrefix+userID, digest)
		return nil
	})
	if err != nil {
		return fmt.Errorf("remove session: %w", err)
	}
	return nil
}

func (service *Service) CookieName() string {
	if service.config.SecureCookies() {
		return secureCookieName
	}
	return insecureCookieName
}

func (service *Service) WriteCookie(c *gin.Context, token string) {
	parts := []string{service.CookieName() + "=" + token, "Path=/", "HttpOnly", "SameSite=Strict"}
	if service.config.SecureCookies() {
		parts = append(parts, "Secure")
	}
	c.Writer.Header().Add("Set-Cookie", strings.Join(parts, "; "))
}

func (service *Service) ClearCookie(c *gin.Context) {
	parts := []string{
		service.CookieName() + "=", "Path=/", "HttpOnly", "SameSite=Strict",
		"Max-Age=0", "Expires=Thu, 01 Jan 1970 00:00:00 GMT",
	}
	if service.config.SecureCookies() {
		parts = append(parts, "Secure")
	}
	c.Writer.Header().Add("Set-Cookie", strings.Join(parts, "; "))
}

func (service *Service) sessionDigest(token string) string {
	mac := hmac.New(sha256.New, service.secret)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (service *Service) record(ctx context.Context, event audit.Event) {
	if service.audit != nil {
		_ = service.audit.Record(ctx, event)
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate secure token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func isMutation(method string) bool {
	return method != "GET" && method != "HEAD" && method != "OPTIONS"
}
