package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAutomaticHealthDoesNotShowPreviousMachineResult(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32), AutomaticHealth: true})
	admin := login(t, h, "admin@example.test")
	ctx := context.Background()
	var a, b, nid, uid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('old',$1) RETURNING id`, []byte("local-old")).Scan(&a)
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('new',$1) RETURNING id`, []byte("local-new")).Scan(&b)
	w := request(h, "POST", "/api/admin/nodes", map[string]any{"name": "moved", "machine_id": b, "public_host": "127.0.0.1", "config": map[string]any{"protocol": "socks", "server_port": 25000}}, admin)
	var n struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &n)
	nid = n.ID
	s.Pool.QueryRow(ctx, `SELECT id FROM users WHERE email='admin@example.test'`).Scan(&uid)
	_, err := s.Pool.Exec(ctx, `INSERT INTO proxy_probes(id,user_id,node_id,machine_id,kind,state,automatic,expires_at,finished_at) VALUES('d2bc9795-3d56-44ba-a075-784a64ac7a79',$1,$2,$3,'node','succeeded',true,now()+interval '1 minute',now())`, uid, nid, a)
	if err != nil {
		t.Fatal(err)
	}
	w = request(h, "GET", "/api/me/nodes", nil, admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), "d2bc9795") {
		t.Fatal("health result from previous host was displayed", w.Code, w.Body.String())
	}
}
