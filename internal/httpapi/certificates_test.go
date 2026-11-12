package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCertificateSecretsMaskedPreservedAndDeliveredToOwner(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "TLS owner"}, admin)
	var machine struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &machine)
	cfg := map[string]any{"preset": "hysteria2", "kernel_type": "singbox", "protocol": "hysteria", "server_port": 24443, "tls": 1, "server_name": "edge.example.com", "cert_config": map[string]any{"cert_mode": "dns", "domain": "edge.example.com", "email": "admin@example.com", "dns_provider": "cf", "dns_env": map[string]string{"CF_API_TOKEN": "sentinel-dns-api-key=="}}}
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "TLS", "machine_id": machine.ID, "config": cfg}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var node struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &node)
	path := fmt.Sprintf("/api/admin/nodes/%d", node.ID)
	w = request(h, "GET", path, nil, admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), "sentinel-dns-api-key") || !strings.Contains(w.Body.String(), `"dns_env_saved":true`) {
		t.Fatal("DNS credential must be masked with saved indicator", w.Code, w.Body.String())
	}
	var detail map[string]any
	json.Unmarshal(w.Body.Bytes(), &detail)
	delete(detail, "plan_ids")
	w = request(h, "PATCH", path, map[string]any{"name": "TLS renamed", "machine_id": machine.ID, "config": detail}, admin)
	if w.Code != 200 {
		t.Fatal("unchanged certificate must retain credential", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/v2/server/config?machine_id=%d&node_id=%d", machine.ID, node.ID), nil)
	r.Header.Set("Authorization", "Bearer "+machine.Token)
	r.Header.Set("X-Luma-Capabilities", "protocols-v1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "sentinel-dns-api-key==") || strings.Contains(rec.Body.String(), "secret_action") || strings.Contains(rec.Body.String(), "dns_env_saved") {
		t.Fatal("assigned node did not receive clean native wire config", rec.Code, rec.Body.String())
	}
	for _, query := range []string{`SELECT config FROM nodes WHERE id=$1`, `SELECT encrypted_config FROM node_private WHERE node_id=$1`} {
		var b []byte
		if err := s.Pool.QueryRow(context.Background(), query, node.ID).Scan(&b); err != nil || strings.Contains(string(b), "sentinel-dns-api-key") {
			t.Fatal("plaintext DNS credential in database", err)
		}
	}
	cc := detail["cert_config"].(map[string]any)
	cc["dns_provider"] = "alidns"
	w = request(h, "PATCH", path, map[string]any{"name": "should rollback", "machine_id": machine.ID, "config": detail}, admin)
	if w.Code != 400 {
		t.Fatal("provider changed with saved credential", w.Code, w.Body.String())
	}
	var name string
	s.Pool.QueryRow(context.Background(), `SELECT name FROM nodes WHERE id=$1`, node.ID).Scan(&name)
	if name != "TLS renamed" {
		t.Fatal("failed certificate update partly saved")
	}
	user := login(t, h, "user@example.test")
	w = request(h, "GET", "/api/admin/certificates/providers", nil, user)
	if w.Code != 403 {
		t.Fatal("ordinary user fetched certificate administration", w.Code)
	}
}

func TestInvalidRawCertificateRejectedBeforeSaving(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/nodes", map[string]any{"name": "Bad TLS", "config": map[string]any{"protocol": "vless", "server_port": 443, "tls": 1, "server_name": "edge.example.com", "cert_config": map[string]any{"cert_mode": "dns", "domain": "wrong.example.com", "email": "admin@example.com", "dns_provider": "cloudflare", "dns_env": map[string]string{"CF_API_TOKEN": "not-a-real-token"}}}}, admin)
	if w.Code != 400 {
		t.Fatal("raw JSON bypassed certificate validation", w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/admin/certificates/providers", nil, admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "CF_API_TOKEN") {
		t.Fatal("provider catalogue missing", w.Code)
	}
}

func TestOneClickChoosesFreeMachinePortAndDefaultRealityTarget(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "generator"}, admin)
	var machine struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &machine)
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "occupied", "machine_id": machine.ID, "config": map[string]any{"protocol": "socks", "server_port": 25000}}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	w = request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": "vless-reality", "port": 25000, "auto_port": true, "machine_id": machine.ID}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var cfg map[string]any
	json.Unmarshal(w.Body.Bytes(), &cfg)
	if cfg["server_port"] == float64(25000) || cfg["tls_settings"].(map[string]any)["server_name"] != "addons.mozilla.org" {
		t.Fatal("one click produced occupied or non-default config", cfg)
	}
}
