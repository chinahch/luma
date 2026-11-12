package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPresetNodeSecretsEncryptedAndNotInLists(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": "vless-enc-reality-vision", "port": 18443, "server_name": "cert.example", "authentication": "mlkem768", "appearance": "xorpub"}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var cfg map[string]any
	json.Unmarshal(w.Body.Bytes(), &cfg)
	secret := cfg["decryption"].(string)
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "ENC", "config": cfg}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var node struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &node)
	w = request(h, "GET", "/api/admin/nodes", nil, admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), secret) {
		t.Fatal("private key in list", w.Code)
	}
	var stored []byte
	if e := s.Pool.QueryRow(context.Background(), `SELECT config FROM nodes WHERE id=$1`, node.ID).Scan(&stored); e != nil || strings.Contains(string(stored), secret) {
		t.Fatal("private key stored in plaintext", e)
	}
	w = request(h, "GET", fmt.Sprintf("/api/admin/nodes/%d", node.ID), nil, admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), secret) {
		t.Fatal("administrator cannot edit full config", w.Code)
	}
	w = request(h, "GET", fmt.Sprintf("/api/v2/server/config?node_id=%d&token=%s", node.ID, node.Token), nil, nil)
	if w.Code != 409 {
		t.Fatal("old node accepted a protocol preset", w.Code)
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/v2/server/config?node_id=%d&token=%s", node.ID, node.Token), nil)
	r.Header.Set("X-Luma-Capabilities", "protocols-v1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), secret) {
		t.Fatal("scoped node did not receive secret", w.Code)
	}
}

func TestSubscriptionIncludesEncryptedSS2022AndAnyTLS(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	for _, preset := range []string{"ss2022", "anytls-reality"} {
		w := request(h, "POST", "/api/admin/protocols/preset", map[string]any{"preset": preset, "port": 18443, "server_name": "cert.example"}, admin)
		var cfg map[string]any
		json.Unmarshal(w.Body.Bytes(), &cfg)
		w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": preset, "public_host": "node.example", "config": cfg}, admin)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := request(h, "POST", "/api/me/subscription-token", nil, user)
	var link struct {
		Path string `json:"path"`
	}
	json.Unmarshal(w.Body.Bytes(), &link)
	w = request(h, "GET", link.Path, nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	raw, e := base64.StdEncoding.DecodeString(w.Body.String())
	if e != nil || !strings.Contains(string(raw), "ss://") || !strings.Contains(string(raw), "anytls://") {
		t.Fatal("native subscription omitted nodes")
	}
	if strings.Contains(string(raw), "private_key") {
		t.Fatal("inbound key leaked")
	}
}

func TestCustomOutboundSecretsUseEncryptedCompleteConfig(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	cfg := map[string]any{"protocol": "vless", "server_port": 443, "custom_outbounds": []any{map[string]any{"tag": "proxy", "protocol": "socks", "settings": map[string]any{"servers": []any{map[string]any{"address": "exit.example", "port": 1080, "users": []any{map[string]any{"user": "test", "pass": "sentinel-private-pass", "id": "sentinel-private-id", "uuid": "sentinel-private-uuid", "secret_key": "sentinel-private-wireguard"}}}}}}}}
	w := request(h, "POST", "/api/admin/nodes", map[string]any{"name": "custom", "config": cfg}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	var raw []byte
	s.Pool.QueryRow(context.Background(), `SELECT config FROM nodes WHERE id=$1`, created.ID).Scan(&raw)
	if strings.Contains(string(raw), "sentinel-private") {
		t.Fatal("custom credentials stored as plaintext")
	}
	w = request(h, "GET", "/api/admin/nodes", nil, admin)
	if strings.Contains(w.Body.String(), "sentinel-private") {
		t.Fatal("list exposed custom credentials")
	}
	w = request(h, "GET", fmt.Sprintf("/api/admin/nodes/%d", created.ID), nil, admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "sentinel-private-pass") {
		t.Fatal("complete config not restored")
	}
	// Legacy conversion must cover arbitrary credential structures too.
	legacy, _ := json.Marshal(cfg)
	s.Pool.Exec(context.Background(), `UPDATE nodes SET config=$2 WHERE id=$1`, created.ID, legacy)
	s.Pool.Exec(context.Background(), `INSERT INTO nodes(name,token_hash,config) VALUES('legacy',$1,$2)`, store.Hash("legacy-test-token"), legacy)
	if e := ProtectNodeSecrets(context.Background(), s, strings.Repeat("01", 32)); e != nil {
		t.Fatal(e)
	}
	var leaks int
	s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM nodes WHERE config::text LIKE '%sentinel-private%'`).Scan(&leaks)
	if leaks != 0 {
		t.Fatal("legacy credentials remained plaintext")
	}
}
