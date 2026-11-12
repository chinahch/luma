package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"strings"
	"testing"
	"time"
)

func TestConnectorDiscoveryScopedAndQuotaEnforced(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	ctx := context.Background()
	var user int64
	s.Pool.QueryRow(ctx, `SELECT id FROM users WHERE role='user'`).Scan(&user)
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "test"}, admin)
	var machine struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &machine)
	w = request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": "mtg", "port": 18443, "server_name": "front.example", "metrics_port": 21001, "owner_id": user}, admin)
	var cfg map[string]any
	json.Unmarshal(w.Body.Bytes(), &cfg)
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "telegram", "public_host": "node.example", "machine_id": machine.ID, "config": cfg}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var node struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &node)
	userCookie := login(t, h, "user@example.test")
	w = request(h, "GET", "/api/me/telegram", nil, userCookie)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "secret") {
		t.Fatal("owner link missing")
	}
	report := map[string]any{"node_id": node.ID, "machine_id": machine.ID, "token": machine.Token, "report_id": "connector-test-1", "period_start": time.Now().Add(-time.Minute), "period_end": time.Now(), "entries": []any{map[string]any{"user_id": user, "up_bytes": 101, "down_bytes": 202}}}
	for i := 0; i < 2; i++ {
		w = request(h, "POST", "/api/v3/server/connector/report", report, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var used int64
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, user).Scan(&used)
	if used != 303 {
		t.Fatal("duplicate connector charge", used)
	}
	report["entries"] = []any{map[string]any{"user_id": user + 100, "up_bytes": 1, "down_bytes": 1}}
	w = request(h, "POST", "/api/v3/server/connector/report", report, nil)
	if w.Code != 400 {
		t.Fatal("foreign user report accepted")
	}
	var otherUser int64
	s.Pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,uuid,quota_bytes) VALUES('other@example.test','test',$1,1000000) RETURNING id`, store.NewID()).Scan(&otherUser)
	cfg["connector"].(map[string]any)["user_id"] = otherUser
	w = request(h, "PATCH", fmt.Sprintf("/api/admin/nodes/%d", node.ID), map[string]any{"name": "telegram", "machine_id": machine.ID, "config": cfg}, admin)
	if w.Code != 409 {
		t.Fatal("MTG owner changed under pending reports", w.Code)
	}
	path := fmt.Sprintf("/api/v3/server/connectors?machine_id=%d&token=%s", machine.ID, machine.Token)
	w = request(h, "GET", path, nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "credential") || !strings.Contains(w.Body.String(), "mtg") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "POST", "/api/v2/server/machine/nodes", map[string]any{"machine_id": machine.ID, "token": machine.Token}, nil)
	if strings.Contains(w.Body.String(), "telegram") {
		t.Fatal("native worker received MTG")
	}
	s.Pool.Exec(ctx, `UPDATE users SET used_bytes=quota_bytes WHERE id=$1`, user)
	w = request(h, "GET", path, nil, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), "credential") {
		t.Fatal("exhausted owner kept running", w.Code)
	}
	// Another Machine's token must not read this node's desired state.
	other := store.NewToken()
	var otherID int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('other',$1) RETURNING id`, store.Hash(other)).Scan(&otherID)
	w = request(h, "GET", fmt.Sprintf("/api/v3/server/connectors?machine_id=%d&token=%s", otherID, machine.Token), nil, nil)
	if w.Code != 401 {
		t.Fatal("machine scope leaked", w.Code)
	}
}

func TestNativeStatusCannotEraseConnectorReadiness(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	token := store.NewToken()
	var id int64
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash) VALUES('CF',$1) RETURNING id`, store.Hash(token)).Scan(&id)
	auth := map[string]any{"node_id": id, "token": token, "kind": "cloudflared", "state": "ready"}
	w := request(h, "POST", "/api/v3/server/connector/status", auth, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = request(h, "POST", "/api/v1/server/UniProxy/status", map[string]any{"node_id": id, "token": token, "cpu": 1}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var state string
	s.Pool.QueryRow(ctx, `SELECT COALESCE(status->'connector'->>'state','') FROM nodes WHERE id=$1`, id).Scan(&state)
	if state != "ready" {
		t.Fatal("native status erased connector readiness")
	}
}

func TestMTGRouteReadFailuresNeverRenewDirectLease(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	var owner int64
	s.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE role='user'`).Scan(&owner)
	w := request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": "mtg", "port": 18443, "server_name": "front.example", "metrics_port": 21001, "owner_id": owner}, admin)
	var cfg map[string]any
	json.Unmarshal(w.Body.Bytes(), &cfg)
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "MTG", "config": cfg}, admin)
	var node struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &node)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	path := fmt.Sprintf("/api/v3/server/connectors?node_id=%d&token=%s", node.ID, node.Token)
	if _, e := s.Pool.Exec(context.Background(), `ALTER TABLE node_exit_routes RENAME TO unavailable_routes`); e != nil {
		t.Fatal(e)
	}
	w = request(h, "GET", path, nil, nil)
	if w.Code < 400 || strings.Contains(w.Body.String(), "lease_until") {
		t.Fatal("database failure became a direct lease", w.Code)
	}
	s.Pool.Exec(context.Background(), `ALTER TABLE unavailable_routes RENAME TO node_exit_routes`)
	s.Pool.Exec(context.Background(), `INSERT INTO node_exit_routes(node_id,config) VALUES($1,'"broken"')`, node.ID)
	w = request(h, "GET", path, nil, nil)
	if w.Code < 400 || strings.Contains(w.Body.String(), "lease_until") {
		t.Fatal("malformed route became a direct lease", w.Code)
	}
}
