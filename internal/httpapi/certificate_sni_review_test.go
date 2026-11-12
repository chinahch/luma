package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRawNodeSaveRejectsConflictingNestedSNI(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/nodes", map[string]any{"name": "SNI conflict", "config": map[string]any{"protocol": "vless", "server_port": 443, "tls": 1, "server_name": "edge.example.com", "tls_settings": map[string]any{"server_name": "other.example.com"}, "cert_config": map[string]any{"cert_mode": "file", "domain": "edge.example.com", "cert_file": "/a/cert", "key_file": "/a/key"}}}, admin)
	if w.Code != 400 {
		t.Fatal("raw save accepted conflicting SNI", w.Code)
	}
}
func TestCloudflareSubscriptionHostMustUseTunnelHostname(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": "cf-tunnel", "port": 18001, "metrics_port": 21001, "server_name": "tunnel.example.com", "tunnel_token": "local-only-not-real-token"}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var cfg map[string]any
	json.Unmarshal(w.Body.Bytes(), &cfg)
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "CF wrong host", "public_host": "8.8.8.8", "config": cfg}, admin)
	if w.Code != 400 {
		t.Fatal("physical IP accepted for CF Tunnel subscription", w.Code)
	}
}
