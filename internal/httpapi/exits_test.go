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

func TestExitImportEncryptedMaskedAndRoleIsolated(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	input := map[string]any{"links": "socks5://alice:secret-landing-password@exit.example:1080#Landing"}
	if w := request(h, "POST", "/api/admin/exits/parse", input, user); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w := request(h, "POST", "/api/admin/exits/parse", input, admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "secret-landing-password") {
		t.Fatal(w.Code, w.Body.String())
	}
	var preview struct {
		Profiles []json.RawMessage `json:"profiles"`
	}
	json.Unmarshal(w.Body.Bytes(), &preview)
	w = request(h, "POST", "/api/admin/exits", map[string]any{"profiles": preview.Profiles}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/admin/exits", nil, admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-landing-password") {
		t.Fatal(w.Code, w.Body.String())
	}
	var cipher []byte
	if e := s.Pool.QueryRow(context.Background(), "SELECT encrypted_profile FROM exits LIMIT 1").Scan(&cipher); e != nil || strings.Contains(string(cipher), "secret-landing-password") {
		t.Fatal("plaintext in database", e)
	}
}

func TestLandingDeliveryRequiresCapabilityAndNeverFallsBack(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	ctx := context.Background()
	token := store.NewToken()
	var node int64
	e := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash,config) VALUES('test',$1,'{"protocol":"vless","server_port":443,"kernel_type":"xray"}') RETURNING id`, store.Hash(token)).Scan(&node)
	if e != nil {
		t.Fatal(e)
	}
	p := map[string]any{"name": "landing", "protocol": "socks", "server": "exit.example", "port": 1080, "password": "private-pass", "username": "u", "transport": map[string]any{"network": "tcp"}, "tls": map[string]any{"enabled": false}}
	w := request(h, "POST", "/api/admin/exits", map[string]any{"profiles": []any{p}}, admin)
	var saved struct {
		IDs []int64 `json:"ids"`
	}
	json.Unmarshal(w.Body.Bytes(), &saved)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	path := fmt.Sprintf("/api/admin/nodes/%d/exits", node)
	w = request(h, "PUT", path, map[string]any{"default_exit": saved.IDs[0], "rules": []any{}}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	configPath := fmt.Sprintf("/api/v2/server/config?node_id=%d&token=%s", node, token)
	w = request(h, "GET", configPath, nil, nil)
	if w.Code != 409 {
		t.Fatal("old node accepted landing", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", configPath, nil)
	r.Header.Set("X-Luma-Capabilities", "landing-v1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "luma-exit-") || !strings.Contains(w.Body.String(), "private-pass") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "PATCH", fmt.Sprintf("/api/admin/exits/%d", saved.IDs[0]), map[string]bool{"active": false}, admin)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 409 || strings.Contains(w.Body.String(), "private-pass") {
		t.Fatal("disabled landing silently bypassed", w.Code, w.Body.String())
	}
}
func TestExitStorageRequiresStableMasterKey(t *testing.T) {
	h, _ := setup(t)
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/exits", map[string]any{"profiles": []any{}}, admin)
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
}
