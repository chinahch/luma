package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"strings"
	"testing"
	"time"
)

func TestExitDeleteWaitsForConcurrentRouteSave(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	var eid, nid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO exits(secret_id,name,protocol,server,port,encrypted_profile) VALUES($1,'dormant','socks','exit.example',1080,'unused') RETURNING id`, store.NewID()).Scan(&eid); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash,config) VALUES('concurrent',$1,'{"protocol":"vless","server_port":443,"kernel_type":"xray"}') RETURNING id`, store.Hash(store.NewToken())).Scan(&nid); err != nil {
		t.Fatal(err)
	}
	const gate int64 = 66715432
	if _, err := s.Pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION pause_route_save() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock(%d); RETURN NEW; END $$; CREATE TRIGGER pause_route BEFORE INSERT ON node_exit_routes FOR EACH ROW EXECUTE FUNCTION pause_route_save()`, gate)); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, gate); err != nil {
		t.Fatal(err)
	}
	saved := make(chan int, 1)
	deleted := make(chan int, 1)
	go func() {
		saved <- request(h, "PUT", fmt.Sprintf("/api/admin/nodes/%d/exits", nid), map[string]any{"global": false, "default_exit": eid, "rules": []any{}}, admin).Code
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=$1 AND NOT granted)`, gate).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("route save did not reach insert gate")
		}
		time.Sleep(10 * time.Millisecond)
	}
	go func() { deleted <- request(h, "DELETE", fmt.Sprintf("/api/admin/exits/%d", eid), nil, admin).Code }()
	select {
	case code := <-deleted:
		t.Fatalf("delete bypassed in-flight route lock: %d", code)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-saved:
		if code != 200 {
			t.Fatal("route save failed", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("route save stalled")
	}
	select {
	case code := <-deleted:
		if code != 409 {
			t.Fatal("referenced exit deleted", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delete stalled")
	}
}

func TestDeleteExitRequiresAdminAndRejectsNodeReferences(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	ctx := context.Background()
	admin, user := login(t, h, "admin@example.test"), login(t, h, "user@example.test")
	w := request(h, "POST", "/api/admin/exits", map[string]any{"profiles": []any{map[string]any{"name": "Delete test", "protocol": "socks", "server": "exit.example", "port": 1080, "username": "u", "password": "sentinel-exit-secret", "transport": map[string]any{"network": "tcp"}, "tls": map[string]any{"enabled": false}}}}, admin)
	var result struct {
		IDs []int64 `json:"ids"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.IDs) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	eid := result.IDs[0]
	path := fmt.Sprintf("/api/admin/exits/%d", eid)
	if w = request(h, "DELETE", path, nil, user); w.Code != 403 {
		t.Fatal("user deleted exit", w.Code)
	}
	var nid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash,active,config) VALUES('Bound node',$1,false,'{"protocol":"vless","server_port":443,"kernel_type":"xray"}') RETURNING id`, store.Hash(store.NewToken())).Scan(&nid); err != nil {
		t.Fatal(err)
	}
	for _, config := range []string{fmt.Sprintf(`{"default_exit":%d,"rules":[]}`, eid), fmt.Sprintf(`{"default_exit":0,"rules":[{"exit_id":%d,"domain_suffixes":["example.com"]}]}`, eid)} {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO node_exit_routes(node_id,config) VALUES($1,$2) ON CONFLICT(node_id) DO UPDATE SET config=excluded.config`, nid, config); err != nil {
			t.Fatal(err)
		}
		w = request(h, "DELETE", path, nil, admin)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "Bound node") || strings.Contains(w.Body.String(), "sentinel-exit-secret") {
			t.Fatal("referenced exit not protected", w.Code, w.Body.String())
		}
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE node_exit_routes SET config='{"default_exit":0,"rules":[]}' WHERE node_id=$1`, nid); err != nil {
		t.Fatal(err)
	}
	if w = request(h, "DELETE", path, nil, admin); w.Code != 200 {
		t.Fatal("unused exit deletion failed", w.Code, w.Body.String())
	}
	if w = request(h, "GET", path, nil, admin); w.Code != 404 {
		t.Fatal("deleted secret still readable", w.Code)
	}
	if w = request(h, "DELETE", path, nil, admin); w.Code != 404 {
		t.Fatal("nonexistent exit deletion not rejected", w.Code)
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM exits WHERE id=$1`, eid).Scan(&count); err != nil || count != 0 {
		t.Fatal("encrypted profile not removed", err)
	}
}
