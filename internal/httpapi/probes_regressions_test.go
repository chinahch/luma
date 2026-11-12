package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"strings"
	"testing"
)

func TestDiagnosticSuccessIsNotConnectivitySuccess(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	var nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash) VALUES('kind boundary',$1) RETURNING id`, store.Hash("kind-node")).Scan(&nid)
	var mid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('kind machine',$1) RETURNING id`, store.Hash("kind-machine")).Scan(&mid)
	for _, kind := range []string{"node", "diagnostic"} {
		state := "failed"
		if kind == "diagnostic" {
			state = "succeeded"
		}
		_, e := s.Pool.Exec(ctx, `INSERT INTO proxy_probes(id,user_id,node_id,machine_id,kind,state,created_at,expires_at,finished_at) VALUES($1,1,$2,$3,$4,$5,CASE WHEN $4='node' THEN now()-interval '1 minute' ELSE now() END,now()+interval '1 minute',now())`, store.NewID(), nid, mid, kind, state)
		if e != nil {
			t.Fatal(e)
		}
	}
	w := request(h, "GET", "/api/me/nodes", nil, admin)
	var nodes []struct {
		Latest *probeResult `json:"latest_probe"`
	}
	json.Unmarshal(w.Body.Bytes(), &nodes)
	if w.Code != 200 || len(nodes) != 1 || nodes[0].Latest == nil || nodes[0].Latest.State != "failed" {
		t.Fatal("diagnostic passed off as real HTTP request", w.Code, w.Body.String())
	}
}
func TestH3DiagnosticInspectsUDP(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	var mid, nid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('h3',$1) RETURNING id`, store.Hash("h3-machine")).Scan(&mid)
	w := request(h, "POST", "/api/admin/nodes", map[string]any{"name": "h3", "machine_id": mid, "config": map[string]any{"protocol": "vless", "kernel_type": "xray", "network": "xhttp", "tls": 1, "server_port": 24443, "networkSettings": map[string]any{"alpn": "h3"}}}, admin)
	var created struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	nid = created.ID

	poll := fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=h3-machine", mid)
	request(h, "GET", poll, nil, nil)
	w = request(h, "POST", fmt.Sprintf("/api/admin/nodes/%d/diagnostics", nid), map[string]any{}, admin)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", poll, nil, nil)
	var jobs struct {
		Tasks []struct {
			Payload probePayload `json:"payload"`
		} `json:"tasks"`
	}
	json.Unmarshal(w.Body.Bytes(), &jobs)
	if w.Code != 200 || len(jobs.Tasks) != 1 || jobs.Tasks[0].Payload.Transport != "udp" {
		t.Fatal("H3 expected UDP socket inspection", w.Code, w.Body.String())
	}
}
