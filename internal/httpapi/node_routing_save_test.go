package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNodeAndSelectedLandingSaveAtomicallyWithSingleConnection(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	poolConfig := s.Pool.Config()
	poolConfig.MaxConns = 1
	s.Pool.Close()
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	s.Pool = pool
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	p := map[string]any{"name": "landing", "protocol": "socks", "server": "exit.example", "port": 1080, "password": "private-pass", "username": "u", "transport": map[string]any{"network": "tcp"}, "tls": map[string]any{"enabled": false}}
	w := request(h, "POST", "/api/admin/exits", map[string]any{"profiles": []any{p}}, admin)
	var exits struct {
		IDs []int64 `json:"ids"`
	}
	json.Unmarshal(w.Body.Bytes(), &exits)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	route := map[string]any{"default_exit": exits.IDs[0], "rules": []any{map[string]any{"exit_id": exits.IDs[0], "domain_suffixes": []string{"example.com"}, "ip_cidrs": []string{"203.0.113.0/24"}}}}
	body := map[string]any{"name": "node-with-landing", "config": map[string]any{"protocol": "vless", "server_port": 443, "kernel_type": "xray"}, "exit_routing": route}
	w = request(h, "POST", "/api/admin/nodes", body, admin)
	if w.Code != 201 {
		t.Fatalf("create node+route: %d %s", w.Code, w.Body.String())
	}
	var node struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &node)
	path := fmt.Sprintf("/api/admin/nodes/%d", node.ID)
	w = request(h, "GET", path+"/exits", nil, admin)
	if !strings.Contains(w.Body.String(), "example.com") {
		t.Fatal("saved route missing", w.Body.String())
	}
	body["name"] = "must-rollback"
	body["exit_routing"] = map[string]any{"default_exit": 999999, "rules": []any{}}
	w = request(h, "PATCH", path, body, admin)
	if w.Code != 400 {
		t.Fatalf("invalid route update %d %s", w.Code, w.Body.String())
	}
	var name string
	s.Pool.QueryRow(ctx, `SELECT name FROM nodes WHERE id=$1`, node.ID).Scan(&name)
	if name != "node-with-landing" {
		t.Fatal("node update not rolled back", name)
	}
	w = request(h, "POST", "/api/admin/nodes", body, admin)
	if w.Code != 400 {
		t.Fatal("invalid route create accepted", w.Code)
	}
	var count int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM nodes`).Scan(&count)
	if count != 1 {
		t.Fatal("invalid node persisted")
	}
	body["name"] = "cleared"
	body["exit_routing"] = map[string]any{"default_exit": 0, "rules": []any{}}
	w = request(h, "PATCH", path, body, admin)
	if w.Code != 200 {
		t.Fatal("clear route", w.Code, w.Body.String())
	}
	w = request(h, "GET", path+"/exits", nil, admin)
	var saved struct {
		DefaultExit int64 `json:"default_exit"`
	}
	json.Unmarshal(w.Body.Bytes(), &saved)
	if saved.DefaultExit != 0 {
		t.Fatal("default route not cleared")
	}
}
