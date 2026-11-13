package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGlobalExitSwitchPersistsAndControlsDeliveredConfig(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	profiles := []any{}
	for _, name := range []string{"global", "rule"} {
		profiles = append(profiles, map[string]any{"name": name, "protocol": "socks", "server": "exit.example", "port": 1080, "transport": map[string]any{"network": "tcp"}, "tls": map[string]any{"enabled": false}})
	}
	w := request(h, "POST", "/api/admin/exits", map[string]any{"profiles": profiles}, admin)
	var created struct {
		IDs []int64 `json:"ids"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &created) != nil || len(created.IDs) != 2 {
		t.Fatal(w.Code, w.Body.String())
	}
	token := store.NewToken()
	var nid int64
	config := map[string]any{"protocol": "vless", "server_port": 443, "kernel_type": "xray", "custom_route_rules": []any{map[string]any{"match": map[string]any{"networks": []string{"tcp", "udp"}}, "action": map[string]any{"type": "direct"}}}}
	raw, _ := json.Marshal(config)
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash,config) VALUES('global-switch',$1,$2) RETURNING id`, store.Hash(token), raw).Scan(&nid); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/admin/nodes/%d/exits", nid)
	for _, global := range []bool{true, false} {
		route := map[string]any{"global": global, "default_exit": created.IDs[0], "rules": []any{map[string]any{"exit_id": created.IDs[1], "domain_suffixes": []string{"example.com"}}}}
		w = request(h, "PUT", path, route, admin)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		w = request(h, "GET", path, nil, admin)
		var saved map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &saved) != nil || saved["global"] != global {
			t.Fatal("switch not persisted", w.Code, w.Body.String())
		}
		r := httptest.NewRequest("GET", fmt.Sprintf("/api/v2/server/config?node_id=%d&token=%s", nid, token), nil)
		r.Header.Set("X-Luma-Capabilities", "landing-v1")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var out struct {
			Outbounds []struct {
				Tag string `json:"tag"`
			} `json:"custom_outbounds"`
			Rules []struct {
				Action struct {
					Type   string `json:"type"`
					Target string `json:"target"`
				} `json:"action"`
			} `json:"custom_route_rules"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Outbounds) != 1 {
			t.Fatal("wrong effective config", w.Code, w.Body.String())
		}
		want := created.IDs[1]
		if global {
			want = created.IDs[0]
		}
		if out.Outbounds[0].Tag != fmt.Sprintf("luma-exit-%d", want) || len(out.Rules) < 2 || out.Rules[0].Action.Type != "block" || out.Rules[1].Action.Target != out.Outbounds[0].Tag {
			t.Fatal("route priority did not follow switch")
		}
		if global && len(out.Rules) != 2 {
			t.Fatal("global mode retained higher-priority direct/split overrides")
		}
	}
}
