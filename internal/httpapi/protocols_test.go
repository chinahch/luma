package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
