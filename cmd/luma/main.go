package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/chinahch/luma/internal/httpapi"
	"github.com/chinahch/luma/internal/secretbox"
	"github.com/chinahch/luma/internal/store"
	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil {
			return errors.New("healthcheck failed")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return errors.New("healthcheck not ready")
		}
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s, err := store.Open(startup, dsn)
	if err != nil {
		return errors.New("database connection failed; check DATABASE_URL and database availability")
	}
	defer s.Pool.Close()
	if err = s.Migrate(startup); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "migrate" {
		fmt.Println("Database schema is current")
		return nil
	}
	if len(args) == 2 && args[0] == "admin" && args[1] == "create" {
		if err = httpapi.CreateAdmin(startup, s, os.Getenv("LUMA_ADMIN_EMAIL"), os.Getenv("LUMA_ADMIN_PASSWORD")); err != nil {
			return errors.New("admin creation failed; use a new valid email and 12..72 byte password in LUMA_ADMIN_EMAIL/LUMA_ADMIN_PASSWORD")
		}
		fmt.Println("Administrator created; no password was printed")
		return nil
	}
	if len(args) > 0 && !(len(args) == 1 && args[0] == "serve") {
		return errors.New("usage: luma [serve|migrate|admin create]")
	}
	var cache *redis.Client
	if raw := os.Getenv("REDIS_URL"); raw != "" {
		options, err := redis.ParseURL(raw)
		if err != nil {
			return errors.New("invalid REDIS_URL")
		}
		options.DialTimeout = time.Second
		options.ReadTimeout = time.Second
		options.WriteTimeout = time.Second
		options.MaxRetries = 0
		cache = redis.NewClient(options)
		defer cache.Close()
	}
	origin := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("PUBLIC_URL must be an http(s) origin without a path")
		}
	}
	zone := os.Getenv("TZ")
	if zone == "" {
		zone = "UTC"
	}
	if _, err = time.LoadLocation(zone); err != nil {
		return errors.New("TZ must be a valid IANA timezone")
	}
	secure := os.Getenv("COOKIE_SECURE") != "false"
	if secure && strings.HasPrefix(origin, "http://") {
		return errors.New("HTTP preview requires COOKIE_SECURE=false; production should use HTTPS")
	}
	addr := os.Getenv("LISTEN_ADDR")
	masterKey := os.Getenv("LUMA_MASTER_KEY")
	if masterKey != "" {
		if _, err = secretbox.New(masterKey); err != nil {
			return err
		}
	}
	if err = httpapi.ProtectNodeSecrets(startup, s, masterKey); err != nil {
		return fmt.Errorf("node secret migration failed: %w", err)
	}
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	handler := httpapi.New(s, cache, httpapi.Config{SecureCookies: secure, AllowedOrigin: origin, Timezone: zone, MasterKey: masterKey, AutomaticHealth: true})
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			cycleCtx, cycleCancel := context.WithTimeout(ctx, 30*time.Second)
			err := s.RefreshCycles(cycleCtx, time.Now())
			cycleCancel()
			if err != nil && ctx.Err() == nil {
				slog.Error("monthly quota reset failed; will retry")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan error, 1)
	go func() { slog.Info("Luma preview listening", "address", addr); done <- server.ListenAndServe() }()
	select {
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer closeCancel()
		return server.Shutdown(shutdown)
	}
}
