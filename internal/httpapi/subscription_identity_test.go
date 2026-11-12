package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestDifferentAccountsHaveDifferentNodeUUIDsAndURLRotationKeepsIdentity(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	w := request(h, "POST", "/api/admin/nodes", map[string]any{"name": "identity", "public_host": "node.example.test", "config": map[string]any{"protocol": "vless", "server_port": 25000}}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	var a, b string
	ctx := context.Background()
	s.Pool.QueryRow(ctx, `SELECT uuid::text FROM users WHERE email='admin@example.test'`).Scan(&a)
	s.Pool.QueryRow(ctx, `SELECT uuid::text FROM users WHERE email='user@example.test'`).Scan(&b)
	if a == b {
		t.Fatal("different database accounts share UUID")
	}
	path := func(body []byte) string {
		var r struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(body, &r) != nil || r.Path == "" {
			t.Fatal("missing subscription path")
		}
		return r.Path
	}
	assertUUID := func(link, want string) {
		r := request(h, "GET", link, nil, nil)
		if r.Code != 200 {
			t.Fatal(r.Code)
		}
		raw, e := base64.StdEncoding.DecodeString(r.Body.String())
		if e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(strings.Split(string(raw), "\n")[0])
		if e != nil || u.User.Username() != want {
			t.Fatal("subscription UUID differs from owning account")
		}
	}
	wa := request(h, "GET", "/api/me/subscription", nil, admin)
	wb := request(h, "GET", "/api/me/subscription", nil, user)
	pa, pb := path(wa.Body.Bytes()), path(wb.Body.Bytes())
	if pa == pb {
		t.Fatal("two accounts share subscription URL")
	}
	assertUUID(pa, a)
	assertUUID(pb, b)
	rotated := request(h, "POST", "/api/me/subscription-token", map[string]any{}, user)
	pr := path(rotated.Body.Bytes())
	if pr == pb {
		t.Fatal("URL not rotated")
	}
	assertUUID(pr, b)
	var uid int64
	s.Pool.QueryRow(ctx, `SELECT id FROM users WHERE email='user@example.test'`).Scan(&uid)
	reset := request(h, "POST", fmt.Sprintf("/api/admin/users/%d/identity", uid), map[string]any{}, admin)
	if reset.Code != 200 {
		t.Fatal(reset.Code)
	}
	var next string
	s.Pool.QueryRow(ctx, `SELECT uuid::text FROM users WHERE id=$1`, uid).Scan(&next)
	if next == b || next == a {
		t.Fatal("identity reset reused UUID")
	}
	assertUUID(path(reset.Body.Bytes()), next)
	assertUUID(pa, a)
}
