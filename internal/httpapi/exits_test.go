package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/proxy"
	"github.com/chinahch/luma/internal/store"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEncodedSOCKSAndENCRealityImportPersistWithoutLosingOptions(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	links := "socks://YWxpY2U6cGFzcw@exit.example:52000#SOCKS\n" +
		"vless://00000000-0000-4000-8000-000000000001@exit.example:20200?encryption=mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&flow=xtls-rprx-vision&security=reality&sni=addons.mozilla.org&fp=chrome&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=7d460428&spx=%2F&type=tcp&headerType=none#VLESS"
	w := request(h, "POST", "/api/admin/exits/parse", map[string]any{"links": links}, admin)
	var preview struct {
		Profiles []proxy.Profile   `json:"profiles"`
		Issues   []json.RawMessage `json:"issues"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &preview) != nil || len(preview.Profiles) != 2 || len(preview.Issues) != 0 {
		t.Fatal("import preview failed", w.Code, w.Body.String())
	}
	if preview.Profiles[0].Username != "alice" || preview.Profiles[0].Password != "pass" {
		t.Fatal("encoded SOCKS credentials lost")
	}
	w = request(h, "POST", "/api/admin/exits", map[string]any{"profiles": preview.Profiles}, admin)
	var saved struct {
		IDs []int64 `json:"ids"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &saved) != nil || len(saved.IDs) != 2 {
		t.Fatal("save failed", w.Code, w.Body.String())
	}
	for i, id := range saved.IDs {
		w = request(h, "GET", fmt.Sprintf("/api/admin/exits/%d", id), nil, admin)
		var p proxy.Profile
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
			t.Fatal("saved profile could not be read")
		}
		before, _ := json.Marshal(preview.Profiles[i])
		after, _ := json.Marshal(p)
		if string(before) != string(after) {
			t.Fatal("profile changed during encrypted persistence")
		}
		o, err := p.Outbound("xray", "landing")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(o)
		if i == 0 && !strings.Contains(string(b), `"users":[{"pass":"pass","user":"alice"}]`) {
			t.Fatal("SOCKS authentication lost")
		}
		if i == 1 && (!strings.Contains(string(b), `"spiderX":"/"`) || !strings.Contains(string(b), `"flow":"xtls-rprx-vision"`) || !strings.Contains(string(b), "mlkem768x25519plus.native.0rtt.")) {
			t.Fatal("ENC/REALITY outbound options lost")
		}
	}
}

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
