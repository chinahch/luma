package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/chinahch/luma/internal/protocol"
	"github.com/chinahch/luma/internal/secretbox"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	SecureCookies bool
	AllowedOrigin string
	Timezone      string
	MasterKey     string
}
type API struct {
	s     *store.Store
	cache *redis.Client
	cfg   Config
	box   *secretbox.Box
}
type User struct {
	ID        int64      `json:"id"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	Active    bool       `json:"active"`
	Quota     int64      `json:"quota_bytes"`
	Used      int64      `json:"used_bytes"`
	ExpiresAt *time.Time `json:"expires_at"`
	PlanID    *int64     `json:"plan_id"`
}

func New(s *store.Store, cache *redis.Client, cfg Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.SetTrustedProxies(nil)
	if cfg.Timezone == "" {
		cfg.Timezone = "UTC"
	}
	a := &API{s: s, cache: cache, cfg: cfg}
	if cfg.MasterKey != "" {
		var err error
		a.box, err = secretbox.New(cfg.MasterKey)
		if err != nil {
			panic(err)
		}
	}
	r.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				slog.Error("request panic", "path", c.Request.URL.Path)
				c.AbortWithStatusJSON(500, gin.H{"error": "internal server error"})
			}
		}()
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") && c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			origin := c.GetHeader("Origin")
			expected := cfg.AllowedOrigin
			if expected == "" {
				scheme := "http"
				if c.Request.TLS != nil {
					scheme = "https"
				}
				expected = scheme + "://" + c.Request.Host
			}
			if origin != "" && origin != expected {
				fail(c, 403, "cross-origin write rejected")
				return
			}
		}
		c.Next()
	})
	r.GET("/healthz", a.health)
	r.POST("/api/login", a.login)
	r.POST("/api/logout", a.auth(), a.logout)
	r.GET("/api/me", a.auth(), func(c *gin.Context) {
		c.JSON(200, gin.H{"user": current(c), "timezone": cfg.Timezone, "version": "0.1.0-preview"})
	})
	r.GET("/api/me/telegram", a.auth(), a.telegramLinks)
	r.GET("/api/dashboard", a.auth(), a.dashboard)
	r.GET("/api/usage", a.auth(), a.usage)
	r.GET("/api/usage/export", a.auth(), a.exportUsage)
	r.POST("/api/me/subscription-token", a.auth(), a.rotateSubscription)
	r.GET("/sub/:token", a.subscription)
	admin := r.Group("/api/admin", a.auth(), func(c *gin.Context) {
		if current(c).Role != "admin" {
			fail(c, 403, "administrator required")
			return
		}
		c.Next()
	})
	admin.GET("/users", a.users)
	admin.POST("/users", a.createUser)
	admin.PATCH("/users/:id", a.updateUser)
	admin.GET("/plans", a.plans)
	admin.POST("/plans", a.createPlan)
	admin.PATCH("/plans/:id", a.updatePlan)
	admin.GET("/machines", a.machines)
	admin.POST("/machines", a.createMachine)
	admin.PATCH("/machines/:id", a.renameMachine)
	admin.POST("/machines/:id/enrollment", a.createEnrollment)
	admin.POST("/machines/:id/token", a.rotateMachineToken)
	admin.DELETE("/machines/:id", a.deleteMachine)
	admin.GET("/nodes", a.nodes)
	admin.GET("/nodes/:id", a.nodeDetail)
	admin.POST("/nodes", a.createNode)
	admin.PATCH("/nodes/:id", a.updateNode)
	admin.GET("/exits", a.exits)
	admin.POST("/exits/parse", a.parseExits)
	admin.POST("/exits", a.createExits)
	admin.GET("/exits/:id", a.exitDetail)
	admin.PATCH("/exits/:id", a.updateExit)
	admin.GET("/nodes/:id/exits", a.nodeExits)
	admin.PUT("/nodes/:id/exits", a.saveNodeExits)
	admin.GET("/protocols", func(c *gin.Context) { c.JSON(200, protocol.Catalog()) })
	admin.POST("/protocols/preset", a.protocolPreset)
	r.POST("/api/server/enroll", a.enrollMachine)
	for _, prefix := range []string{"/api/v1/server/UniProxy", "/api/v2/server"} {
		r.GET(prefix+"/config", a.nodeAuth(), a.nodeConfig)
		r.GET(prefix+"/user", a.nodeAuth(), a.nodeUsers)
	}
	r.POST("/api/v2/server/handshake", a.nodeAuth(), a.handshake)
	r.POST("/api/v2/server/machine/nodes", a.nodeAuth(), a.machineNodes)
	r.POST("/api/v2/server/machine/status", a.nodeAuth(), a.machineStatus)
	r.POST("/api/v2/server/report", a.nodeAuth(), a.legacyReport)
	r.POST("/api/v1/server/UniProxy/push", a.nodeAuth(), a.legacyPush)
	r.POST("/api/v1/server/UniProxy/status", a.nodeAuth(), a.legacyStatus)
	r.POST("/api/v1/server/UniProxy/alive", a.nodeAuth(), func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.POST("/api/v3/server/report", a.nodeAuth(), a.reliableReport)
	r.GET("/api/v3/server/connectors", a.nodeAuth(), a.connectors)
	r.POST("/api/v3/server/connector/status", a.nodeAuth(), a.connectorStatus)
	r.POST("/api/v3/server/connector/report", a.nodeAuth(), a.connectorReport)
	return r
}
func fail(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
func internal(c *gin.Context, err error) {
	slog.Error("database operation failed", "path", c.FullPath(), "error", err)
	fail(c, 500, "operation failed")
}
func decode(c *gin.Context, target any) bool {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		fail(c, 400, "invalid JSON input")
		return false
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		fail(c, 400, "expected a single JSON object")
		return false
	}
	return true
}
func current(c *gin.Context) User { return c.MustGet("user").(User) }
func (a *API) health(c *gin.Context) {
	if err := a.s.Pool.Ping(c.Request.Context()); err != nil {
		fail(c, 503, "database unavailable")
		return
	}
	cache := "not configured"
	if a.cache != nil {
		cache = "unavailable"
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if a.cache.Ping(ctx).Err() == nil {
			cache = "available"
		}
	}
	c.JSON(200, gin.H{"ok": true, "cache": cache, "version": "0.1.0-preview"})
}
