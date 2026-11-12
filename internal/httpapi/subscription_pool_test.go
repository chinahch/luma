package httpapi

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSubscriptionsAndTelegramDoNotNestPoolConnections(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	var owner int64
	s.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE role='user'`).Scan(&owner)
	for _, kind := range []string{"ss2022", "mtg"} {
		w := request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": kind, "port": 18443, "server_name": "front.example", "metrics_port": 21001, "owner_id": owner}, admin)
		var cfg map[string]any
		json.Unmarshal(w.Body.Bytes(), &cfg)
		w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": kind, "public_host": "node.example", "config": cfg}, admin)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := request(h, "POST", "/api/me/subscription-token", nil, user)
	var sub struct {
		Path string `json:"path"`
	}
	json.Unmarshal(w.Body.Bytes(), &sub)
	config := s.Pool.Config()
	config.MaxConns = 1
	s.Pool.Close()
	pool, e := pgxpool.NewWithConfig(context.Background(), config)
	if e != nil {
		t.Fatal(e)
	}
	s.Pool = pool
	var wg sync.WaitGroup
	results := make(chan int, 4)
	for _, path := range []string{sub.Path, "/api/me/telegram", sub.Path, "/api/me/telegram"} {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r := httptest.NewRequest("GET", path, nil).WithContext(ctx)
			if path == "/api/me/telegram" {
				r.AddCookie(user)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			results <- w.Code
		}(path)
	}
	wg.Wait()
	close(results)
	for code := range results {
		if code != http.StatusOK {
			t.Fatal("nested pool request failed", code)
		}
	}
}
