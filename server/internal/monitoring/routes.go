package monitoring

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/Jonath-z/ship/server/internal/access"
	"github.com/Jonath-z/ship/server/internal/platform/config"
	"github.com/Jonath-z/ship/server/internal/platform/database"
	"github.com/Jonath-z/ship/server/internal/platform/httpx"
	shipredis "github.com/Jonath-z/ship/server/internal/platform/redis"
)

func RegisterRoutes(router *httpx.Router, cfg config.Config, db *database.Connection, redisClient *redisclient.Client) {
	collector := New(cfg, Dependencies{
		Postgres: db.Ping,
		Redis: func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		},
		WorkerLastSeen: func(ctx context.Context) (time.Time, error) {
			return shipredis.WorkerLastSeen(ctx, redisClient)
		},
	})

	router.GET("/system", access.SystemRead, func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		c.JSON(200, collector.Collect(ctx))
	})
}
