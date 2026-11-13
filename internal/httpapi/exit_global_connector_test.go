package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestMTGGlobalOffDoesNotDeliverDormantSOCKSProxy(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	var owner int64
	if err := s.Pool.QueryRow(context.Background(), `SELECT id FROM users WHERE role='user'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": "mtg", "port": 18443, "server_name": "front.example", "metrics_port": 21001, "owner_id": owner}, admin)
	var cfg map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &cfg) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "MTG toggle", "config": cfg}, admin)
	var node struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &node) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	profile := map[string]any{"name": "SOCKS", "protocol": "socks", "server": "exit.example", "port": 1080, "transport": map[string]any{"network": "tcp"}, "tls": map[string]any{"enabled": false}}
	w = request(h, "POST", "/api/admin/exits", map[string]any{"profiles": []any{profile}}, admin)
	var exits struct {
		IDs []int64 `json:"ids"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &exits) != nil || len(exits.IDs) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, mode := range []string{"on", "off", "legacy"} {
		routing := map[string]any{"default_exit": exits.IDs[0], "rules": []any{}}
		if mode != "legacy" {
			routing["global"] = mode == "on"
		}
		w = request(h, "PUT", fmt.Sprintf("/api/admin/nodes/%d/exits", node.ID), routing, admin)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		w = request(h, "GET", fmt.Sprintf("/api/v3/server/connectors?node_id=%d&token=%s", node.ID, node.Token), nil, nil)
		var result struct {
			Connectors []struct {
				SOCKS string `json:"socks_proxy"`
			} `json:"connectors"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Connectors) != 1 {
			t.Fatal(w.Code, w.Body.String())
		}
		if (result.Connectors[0].SOCKS != "") != (mode != "off") {
			t.Fatal("MTG connector ignored global toggle", mode)
		}
	}
}
