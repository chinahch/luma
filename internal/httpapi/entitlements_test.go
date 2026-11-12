package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"github.com/chinahch/luma/internal/traffic"
	"strings"
	"testing"
	"time"
)

func TestDeleteNodePreservesLedgerAndFreesMachine(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	var machineID, nodeID int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('delete-owner',$1) RETURNING id`, store.Hash("delete-machine")).Scan(&machineID); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash) VALUES('delete-history',$1,$2) RETURNING id`, machineID, store.Hash("delete-node")).Scan(&nodeID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	report := traffic.Report{ReportID: "before-delete", PeriodStart: now.Add(-time.Minute), PeriodEnd: now, Entries: []traffic.Entry{{UserID: 2, Up: 25}}}
	if _, err := s.Ingest(ctx, nodeID, report, "reliable"); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/admin/nodes/%d", nodeID)
	if w := request(h, "DELETE", path, nil, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var active bool
	var assigned *int64
	var deleted *time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT active,machine_id,deleted_at FROM nodes WHERE id=$1`, nodeID).Scan(&active, &assigned, &deleted); err != nil {
		t.Fatal(err)
	}
	if active || assigned != nil || deleted == nil {
		t.Fatal("deleted node remains available or assigned", active, assigned, deleted)
	}
	var reports, entries, ledger int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_reports WHERE node_id=$1`, nodeID).Scan(&reports)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_entries`).Scan(&entries)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM billing_ledger`).Scan(&ledger)
	if reports != 1 || entries != 1 || ledger != 1 {
		t.Fatal("delete destroyed accounting", reports, entries, ledger)
	}
	if w := request(h, "PATCH", path+"/active", map[string]any{"active": true}, admin); w.Code != 404 {
		t.Fatal("deleted node reactivated", w.Code, w.Body.String())
	}
	if w := request(h, "PATCH", path, map[string]any{"name": "resurrected"}, admin); w.Code != 404 {
		t.Fatal("deleted node edited", w.Code, w.Body.String())
	}
	if w := request(h, "GET", path, nil, admin); w.Code != 404 {
		t.Fatal("deleted node detail exposed", w.Code, w.Body.String())
	}
	if w := request(h, "GET", "/api/admin/nodes", nil, admin); w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatal("deleted node listed", w.Code, w.Body.String())
	}
	if w := request(h, "DELETE", fmt.Sprintf("/api/admin/machines/%d", machineID), nil, admin); w.Code != 200 {
		t.Fatal("deleted node blocks machine deletion", w.Code, w.Body.String())
	}
}

func TestNativeUsersIncludeAdminAndRespectNodePlans(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	var nid, pid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash) VALUES('allowed',$1) RETURNING id`, store.Hash("entitlement-token")).Scan(&nid); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/v2/server/user?node_id=%d&token=entitlement-token", nid)
	read := func() []map[string]any {
		t.Helper()
		w := request(h, "GET", path, nil, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var body struct {
			Users []map[string]any `json:"users"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Users
	}
	if users := read(); len(users) != 2 {
		t.Fatal("admin missing from eligible users", users)
	}
	s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days) VALUES('restricted',10,30) RETURNING id`).Scan(&pid)
	s.Pool.Exec(ctx, `INSERT INTO node_plans(node_id,plan_id) VALUES($1,$2)`, nid, pid)
	if users := read(); len(users) != 0 {
		t.Fatal("users with no plan received restricted node", users)
	}
	s.Pool.Exec(ctx, `UPDATE users SET plan_id=$1 WHERE role='admin'`, pid)
	if users := read(); len(users) != 1 || users[0]["id"] != float64(1) {
		t.Fatal(users)
	}
	s.Pool.Exec(ctx, `UPDATE users SET used_bytes=quota_bytes WHERE role='admin'`)
	if users := read(); len(users) != 0 {
		t.Fatal("admin bypassed quota", users)
	}
}

func TestNodePlansAtomicSaveOmissionAndActiveOnly(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	ctx := context.Background()
	var pid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days) VALUES('restricted',10,30) RETURNING id`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"name": "original", "plan_ids": []int64{pid}, "config": map[string]any{"protocol": "vless", "server_port": 12345}}
	w := request(h, "POST", "/api/admin/nodes", body, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	path := fmt.Sprintf("/api/admin/nodes/%d", created.ID)
	body["name"] = "must-rollback"
	body["plan_ids"] = []int64{pid, 9999999}
	w = request(h, "PATCH", path, body, admin)
	if w.Code != 400 {
		t.Fatal("invalid plan accepted", w.Code, w.Body.String())
	}
	var name string
	var count int
	s.Pool.QueryRow(ctx, `SELECT name FROM nodes WHERE id=$1`, created.ID).Scan(&name)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_plans WHERE node_id=$1`, created.ID).Scan(&count)
	if name != "original" || count != 1 {
		t.Fatal("node save partially committed", name, count)
	}
	delete(body, "plan_ids")
	body["name"] = "renamed"
	w = request(h, "PATCH", path, body, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_plans WHERE node_id=$1`, created.ID).Scan(&count)
	if count != 1 {
		t.Fatal("omission cleared restriction")
	}
	w = request(h, "PATCH", path+"/active", map[string]any{"active": false}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var active bool
	s.Pool.QueryRow(ctx, `SELECT name,active FROM nodes WHERE id=$1`, created.ID).Scan(&name, &active)
	if name != "renamed" || active {
		t.Fatal("active patch replaced node", name, active)
	}
	w = request(h, "PATCH", path+"/active", map[string]any{}, admin)
	if w.Code != 400 {
		t.Fatal("missing active accepted", w.Code)
	}
	body["plan_ids"] = []int64{}
	w = request(h, "PATCH", path, body, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM node_plans WHERE node_id=$1`, created.ID).Scan(&count)
	if count != 0 {
		t.Fatal("explicit empty restriction retained")
	}
	s.Pool.QueryRow(ctx, `SELECT active FROM nodes WHERE id=$1`, created.ID).Scan(&active)
	if active {
		t.Fatal("unrelated update undid row active toggle")
	}
	w = request(h, "GET", "/api/admin/nodes", nil, admin)
	var nodes []map[string]any
	json.Unmarshal(w.Body.Bytes(), &nodes)
	if len(nodes) != 1 || nodes[0]["plan_ids"] == nil {
		t.Fatal(w.Body.String())
	}
}

func TestCreateUserWithPlanRecordsInitialOrder(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	ctx := context.Background()
	var pid int64
	s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('initial',1000,30,900) RETURNING id`).Scan(&pid)
	w := request(h, "POST", "/api/admin/users", map[string]any{"email": "new@example.test", "password": "a-long-password-123", "plan_id": pid}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var orders int
	var quota int64
	if err := s.Pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(o.quota_bytes),0) FROM orders o JOIN users u ON u.id=o.user_id WHERE u.email='new@example.test' AND o.price_cents=900 AND o.actor_id=1`).Scan(&orders, &quota); err != nil {
		t.Fatal(err)
	}
	if orders != 1 || quota != 1000 {
		t.Fatal("initial plan assignment missing immutable order", orders, quota)
	}
}

func TestManualOrderHTTPIdempotencyAndPrivacy(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	var pid int64
	s.Pool.QueryRow(context.Background(), `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('snapshot',1000,30,900) RETURNING id`).Scan(&pid)
	for i := 0; i < 2; i++ {
		w := request(h, "POST", "/api/admin/users/2/orders", map[string]any{"plan_id": pid, "request_id": "order-http-1"}, admin)
		if w.Code != 201 && w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := request(h, "GET", "/api/admin/users/1/orders", nil, user); w.Code != 403 {
		t.Fatal("user read admin orders", w.Code)
	}
	w := request(h, "GET", "/api/me/orders", nil, user)
	var orders []map[string]any
	json.Unmarshal(w.Body.Bytes(), &orders)
	if w.Code != 200 || len(orders) != 1 || orders[0]["price_cents"] != float64(900) {
		t.Fatal(w.Code, w.Body.String())
	}
}
