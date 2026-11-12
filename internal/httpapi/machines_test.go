package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/chinahch/luma/internal/store"
)

func TestMachineRotationAndSafeRemoval(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "edge"}, admin)
	var m struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &m)
	path := fmt.Sprintf("/api/admin/machines/%d", m.ID)
	for _, method := range []string{"POST", "DELETE"} {
		p := path
		if method == "POST" {
			p += "/token"
		}
		if w = request(h, method, p, map[string]any{}, user); w.Code != 403 {
			t.Fatalf("role %s: %d", method, w.Code)
		}
	}
	w = request(h, "POST", path+"/token", map[string]any{}, admin)
	if w.Code != 200 {
		t.Fatalf("rotate: %d %s", w.Code, w.Body.String())
	}
	var rotated struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &rotated)
	if rotated.Token == "" || rotated.Token == m.Token {
		t.Fatal("missing fresh credential")
	}
	for _, token := range []string{m.Token, rotated.Token} {
		w = request(h, "POST", "/api/v2/server/machine/nodes", map[string]any{"machine_id": m.ID, "token": token}, nil)
		want := 200
		if token == m.Token {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("token auth: %d want %d", w.Code, want)
		}
	}
	w = request(h, "GET", "/api/admin/machines", nil, admin)
	if strings.Contains(w.Body.String(), rotated.Token) || strings.Contains(w.Body.String(), "token_hash") {
		t.Fatal("credential leaked")
	}
	ctx := context.Background()
	var node int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash) VALUES('keep',$1,$2) RETURNING id`, m.ID, store.Hash("node-safe-removal")).Scan(&node); err != nil {
		t.Fatal(err)
	}
	if w = request(h, "DELETE", path, nil, admin); w.Code != 409 {
		t.Fatalf("attached machine removed: %d %s", w.Code, w.Body.String())
	}
	var count int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM nodes WHERE id=$1`, node).Scan(&count)
	if count != 1 {
		t.Fatal("node removed")
	}
	s.Pool.Exec(ctx, `UPDATE nodes SET machine_id=NULL WHERE id=$1`, node)
	if w = request(h, "DELETE", path, nil, admin); w.Code != 200 {
		t.Fatalf("empty removal: %d %s", w.Code, w.Body.String())
	}
	if w = request(h, "POST", "/api/v2/server/machine/nodes", map[string]any{"machine_id": m.ID, "token": rotated.Token}, nil); w.Code != 401 {
		t.Fatal("deleted identity authenticates")
	}
	for _, method := range []string{"DELETE", "POST"} {
		p := path
		if method == "POST" {
			p += "/token"
		}
		if w = request(h, method, p, map[string]any{}, admin); w.Code != 404 {
			t.Fatalf("missing: %d", w.Code)
		}
	}
}
