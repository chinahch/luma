package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestServerPollAutomaticallyQueuesHealthWithoutBrowserAction(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32), AutomaticHealth: true})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "auto-health"}, admin)
	var m struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &m)
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "auto node", "machine_id": m.ID, "public_host": "127.0.0.1", "config": map[string]any{"protocol": "socks", "kernel_type": "singbox", "server_port": 25501}}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	path := fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=%s", m.ID, m.Token)
	w = request(h, "GET", path, nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"tasks":[{`) {
		t.Fatal("server poll did not schedule self-test", w.Code, w.Body.String())
	}
	w = request(h, "GET", path, nil, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"tasks":[{`) {
		t.Fatal("unchanged node requeued before interval", w.Code)
	}
	var count int
	s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM proxy_probes WHERE automatic`).Scan(&count)
	if count != 1 {
		t.Fatal("automatic job duplication", count)
	}
	user := login(t, h, "user@example.test")
	w = request(h, "GET", "/api/me/nodes", nil, user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "automatic") {
		t.Fatal("user cannot see server self-test result", w.Code, w.Body.String())
	}
}
