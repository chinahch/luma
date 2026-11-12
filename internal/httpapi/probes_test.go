package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/secretbox"
	"github.com/chinahch/luma/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

func TestProbeQueueMachineLeaseAndSecrets(t *testing.T) {
	h, s := setup(t)
	h = New(s, nil, Config{MasterKey: strings.Repeat("ab", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	var mid, nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('probe',$1) RETURNING id`, store.Hash("machine-probe-token")).Scan(&mid)
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash,public_host,config) VALUES('probe',$1,$2,'127.0.0.1','{"protocol":"vless","server_port":19443,"network":"tcp","tls":0,"kernel_type":"xray"}') RETURNING id`, mid, store.Hash("node-token")).Scan(&nid)
	poll := fmt.Sprintf("/api/v3/server/probes?machine_id=%d", mid)
	if w := request(h, "GET", poll, nil, nil); w.Code != 401 {
		t.Fatal("unauth poll", w.Code, w.Body.String())
	}
	poll += "&token=machine-probe-token"
	if w := request(h, "GET", poll, nil, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, user)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	id := result["id"].(string)
	if result["state"] != "queued" {
		t.Fatal(result)
	}
	var cipher []byte
	s.Pool.QueryRow(ctx, `SELECT encrypted_payload FROM proxy_probes WHERE id=$1`, id).Scan(&cipher)
	var uuid string
	s.Pool.QueryRow(ctx, `SELECT uuid::text FROM users WHERE id=2`).Scan(&uuid)
	if strings.Contains(string(cipher), uuid) {
		t.Fatal("credentials plaintext")
	}
	w = request(h, "GET", poll, nil, nil)
	var jobs struct {
		Tasks []struct {
			ID      string          `json:"id"`
			Lease   string          `json:"lease"`
			Payload json.RawMessage `json:"payload"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &jobs)
	if w.Code != 200 || len(jobs.Tasks) != 1 || jobs.Tasks[0].ID != id || !strings.Contains(string(jobs.Tasks[0].Payload), uuid) {
		t.Fatal(w.Code, w.Body.String())
	}
	body := map[string]any{"machine_id": mid, "token": "machine-probe-token", "lease": "wrong", "state": "succeeded", "latency_ms": 4}
	path := "/api/v3/server/probes/" + id + "/result"
	if w = request(h, "POST", path, body, nil); w.Code != 409 {
		t.Fatal("wrong lease", w.Code, w.Body.String())
	}
	body["lease"] = jobs.Tasks[0].Lease
	body["state"] = "failed"
	body["error_code"] = "password-secret-remote-error"
	if w = request(h, "POST", path, body, nil); w.Code != 400 {
		t.Fatal("raw secret error accepted", w.Code)
	}
	body["error_code"] = "proxy_failed"
	if w = request(h, "POST", path, body, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(h, "POST", path, body, nil); w.Code != 409 {
		t.Fatal("replayed result", w.Code)
	}
	w = request(h, "GET", "/api/me/probes/"+id, nil, user)
	if w.Code != 200 || strings.Contains(w.Body.String(), uuid) || strings.Contains(w.Body.String(), jobs.Tasks[0].Lease) {
		t.Fatal("result leak", w.Code, w.Body.String())
	}
	if w = request(h, "GET", "/api/me/probes/"+id, nil, admin); w.Code != 404 {
		t.Fatal("other account result", w.Code)
	}
	if w = request(h, "GET", "/api/admin/probes/"+id, nil, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestProbeUnsupportedExpiryEntitlementAndRate(t *testing.T) {
	h, s := setup(t)
	h = New(s, nil, Config{MasterKey: strings.Repeat("ab", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	var mid, nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('old',$1) RETURNING id`, store.Hash("old-token")).Scan(&mid)
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash,public_host) VALUES('node',$1,$2,'127.0.0.1') RETURNING id`, mid, store.Hash("node-old")).Scan(&nid)
	w := request(h, "POST", "/api/admin/exits/0/probes", map[string]any{"machine_id": mid, "kernel_type": "xray"}, admin)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "unsupported") {
		t.Fatal(w.Code, w.Body.String())
	}
	request(h, "GET", fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=old-token", mid), nil, nil)
	w = request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, user)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	id := result["id"].(string)
	s.Pool.Exec(ctx, `UPDATE proxy_probes SET expires_at=$2 WHERE id=$1`, id, time.Now().Add(-time.Second))
	w = request(h, "GET", "/api/me/probes/"+id, nil, user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "expired") {
		t.Fatal(w.Code, w.Body.String())
	}
	s.Pool.Exec(ctx, `UPDATE users SET active=false WHERE id=2`)
	if w = request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, user); w.Code != 401 && w.Code != 403 && w.Code != 404 {
		t.Fatal("disabled account allowed", w.Code)
	}
	for i := 0; i < 5; i++ {
		w = request(h, "POST", "/api/admin/exits/0/probes", map[string]any{"machine_id": mid, "kernel_type": "xray"}, admin)
	}
	if w.Code != 429 {
		t.Fatal("rate limit absent", w.Code, w.Body.String())
	}

}

func TestProbePrivateConfigSingleConnectionAndMachineBoundary(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	cfg := s.Pool.Config()
	cfg.MaxConns = 1
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	s.Pool.Close()
	s.Pool = pool
	h = New(s, nil, Config{MasterKey: strings.Repeat("ab", 32)})
	user := login(t, h, "user@example.test")
	var mid, other, nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('one',$1) RETURNING id`, store.Hash("one-token")).Scan(&mid)
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('two',$1) RETURNING id`, store.Hash("two-token")).Scan(&other)
	public := `{"protocol":"shadowsocks","cipher":"2022-blake3-aes-128-gcm","server_port":19443,"kernel_type":"singbox"}`
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash,public_host,config) VALUES('private',$1,$2,'127.0.0.1',$3) RETURNING id`, mid, store.Hash("private-node"), public).Scan(&nid)
	box, _ := secretbox.New(strings.Repeat("ab", 32))
	full := strings.TrimSuffix(public, "}") + `,"server_key":"MDEyMzQ1Njc4OWFiY2RlZg=="}`
	cipher, _ := box.Seal([]byte(full), nodeContext(nid))
	s.Pool.Exec(ctx, `INSERT INTO node_private(node_id,encrypted_config) VALUES($1,$2)`, nid, cipher)
	poll := fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=one-token", mid)
	request(h, "GET", poll, nil, nil)
	w := request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, user)
	if w.Code != 201 {
		t.Fatal("private client config missing", w.Code, w.Body.String())
	}
	var created probeResult
	json.Unmarshal(w.Body.Bytes(), &created)
	w = request(h, "GET", fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=two-token", other), nil, nil)
	if w.Code != 200 || w.Body.String() != `{"tasks":[]}` {
		t.Fatal("wrong machine got task", w.Code, w.Body.String())
	}
	w = request(h, "GET", poll, nil, nil)
	var jobs struct {
		Tasks []struct {
			ID      string          `json:"id"`
			Lease   string          `json:"lease"`
			Payload json.RawMessage `json:"payload"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &jobs)
	if w.Code != 200 || len(jobs.Tasks) != 1 || !strings.Contains(string(jobs.Tasks[0].Payload), "MDEyMzQ1Njc4OWFiY2RlZg==") {
		t.Fatal("private client credentials absent or tx blocked", w.Code, w.Body.String())
	}
	body := map[string]any{"machine_id": other, "token": "two-token", "lease": jobs.Tasks[0].Lease, "state": "succeeded", "latency_ms": 5}
	if w = request(h, "POST", "/api/v3/server/probes/"+created.ID+"/result", body, nil); w.Code != 409 {
		t.Fatal("cross-machine accepted", w.Code)
	}
	s.Pool.Exec(ctx, `UPDATE proxy_probes SET expires_at=now()-interval '1 second' WHERE id=$1`, created.ID)
	body["machine_id"] = mid
	body["token"] = "one-token"
	if w = request(h, "POST", "/api/v3/server/probes/"+created.ID+"/result", body, nil); w.Code != 409 {
		t.Fatal("expired lease accepted", w.Code)
	}
	w = request(h, "GET", "/api/me/probes/"+created.ID, nil, user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "expired") {
		t.Fatal(w.Code, w.Body.String())
	}
	var hasSecret bool
	s.Pool.QueryRow(ctx, `SELECT encrypted_payload IS NOT NULL OR lease_hash IS NOT NULL FROM proxy_probes WHERE id=$1`, created.ID).Scan(&hasSecret)
	if hasSecret {
		t.Fatal("expired credential retention")
	}
	// Recheck permissions at claim, even if an already queued task was authorized earlier.
	w = request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, user)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	s.Pool.Exec(ctx, `UPDATE users SET used_bytes=quota_bytes WHERE id=2`)
	w = request(h, "GET", poll, nil, nil)
	if w.Code != 200 || w.Body.String() != `{"tasks":[]}` {
		t.Fatal("quota revoked user got lease", w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/me/probes/"+created.ID, nil, user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "account_unavailable") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestProbeMTProxyReportsConnectorBoundaryOnly(t *testing.T) {
	h, s := setup(t)
	h = New(s, nil, Config{MasterKey: strings.Repeat("ab", 32)})
	ctx := context.Background()
	user := login(t, h, "user@example.test")
	admin := login(t, h, "admin@example.test")
	var mid, nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('mtg',$1) RETURNING id`, store.Hash("mtg-token")).Scan(&mid)
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash,public_host,config,status) VALUES('telegram',$1,$2,'tg.example','{"protocol":"mtproxy","connector":{"kind":"mtg","user_id":2}}','{"connector":{"state":"ready","kind":"mtg","seen_at":"2026-11-12T00:00:00Z","secret":"never-export"}}') RETURNING id`, mid, store.Hash("mtg-node")).Scan(&nid)
	w := request(h, "GET", "/api/me/nodes", nil, user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"connector_status":{"kind":"mtg","state":"ready"`) || strings.Contains(w.Body.String(), "never-export") {
		t.Fatal("actual sanitized connector status absent", w.Code, w.Body.String())
	}
	w = request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, user)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "connector_only") {
		t.Fatal("MTProxy fake HTTP success", w.Code, w.Body.String())
	}
	w = request(h, "POST", fmt.Sprintf("/api/me/nodes/%d/probes", nid), map[string]any{}, admin)
	if w.Code != 404 {
		t.Fatal("other connector owner allowed", w.Code, w.Body.String())
	}
}

func TestAdminDiagnosticsLeaseSanitizationAndIsolation(t *testing.T) {
	h, s := setup(t)
	h = New(s, nil, Config{MasterKey: strings.Repeat("ab", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	var mid, nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('diag',$1) RETURNING id`, store.Hash("diag-token")).Scan(&mid)
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash,public_host) VALUES('diag',$1,$2,'127.0.0.1') RETURNING id`, mid, store.Hash("diag-node")).Scan(&nid)
	poll := fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=diag-token", mid)
	request(h, "GET", poll, nil, nil)
	path := fmt.Sprintf("/api/admin/nodes/%d/diagnostics", nid)
	if w := request(h, "POST", path, map[string]any{}, user); w.Code != 403 {
		t.Fatal(w.Code)
	}
	s.Pool.Exec(ctx, `UPDATE users SET quota_bytes=0 WHERE id=1`)
	w := request(h, "POST", path, map[string]any{}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created probeResult
	json.Unmarshal(w.Body.Bytes(), &created)
	w = request(h, "GET", poll, nil, nil)
	var jobs struct {
		Tasks []struct {
			Lease   string         `json:"lease"`
			Payload map[string]any `json:"payload"`
		}
	}
	json.Unmarshal(w.Body.Bytes(), &jobs)
	if len(jobs.Tasks) != 1 || jobs.Tasks[0].Payload["action"] != "diagnostic" {
		t.Fatal(w.Code, w.Body.String())
	}
	body := map[string]any{"machine_id": mid, "token": "diag-token", "lease": jobs.Tasks[0].Lease, "state": "succeeded", "latency_ms": 5, "diagnostics": map[string]any{"version": 1, "node_id": nid, "expected_port": 443, "transport": "tcp", "listening": false, "eligible_users": 1, "runtime_state": "stopped", "findings": []map[string]any{{"code": "no_users", "message": "private uuid secret", "severity": "error"}}, "logs": []map[string]any{{"code": "port_in_use", "message": "PRIVATEKEY-secret", "count": 2, "scope": "other-node"}}}}
	w = request(h, "POST", "/api/v3/server/probes/"+created.ID+"/result", body, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/admin/probes/"+created.ID, nil, admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "diagnostics") || strings.Contains(w.Body.String(), "PRIVATEKEY") || strings.Contains(w.Body.String(), "private uuid") || strings.Contains(w.Body.String(), "other-node") {
		t.Fatal("unsanitized diagnostics", w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/me/probes/"+created.ID, nil, admin)
	if strings.Contains(w.Body.String(), "diagnostics") {
		t.Fatal("diagnostics outside admin endpoint", w.Body.String())
	}
}

func TestProbeOfflineAndOnlineOldVersionAreDistinct(t *testing.T) {
	h, s := setup(t)
	h = New(s, nil, Config{MasterKey: strings.Repeat("ab", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	var mid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('offline',$1) RETURNING id`, store.Hash("offline-token")).Scan(&mid)
	body := map[string]any{"machine_id": mid, "kernel_type": "xray"}
	w := request(h, "POST", "/api/admin/exits/0/probes", body, admin)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "machine_unavailable") {
		t.Fatal(w.Code, w.Body.String())
	}
	s.Pool.Exec(ctx, `UPDATE machines SET last_report_at=now() WHERE id=$1`, mid)
	w = request(h, "POST", "/api/admin/exits/0/probes", body, admin)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "upgrade_required") {
		t.Fatal(w.Code, w.Body.String())
	}
}
