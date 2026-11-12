package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/store"
	"strings"
	"testing"
)

func TestLandingPingHostAndRTTRoundTrip(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	ctx := context.Background()
	var mid int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('ping fixture',$1) RETURNING id`, store.Hash("ping-machine")).Scan(&mid)
	poll := fmt.Sprintf("/api/v3/server/probes?machine_id=%d&token=ping-machine", mid)
	request(h, "GET", poll, nil, nil)
	profile := map[string]any{"name": "ping exit", "protocol": "socks", "server": "exit.example", "port": 1080, "transport": map[string]any{"network": "tcp"}, "tls": map[string]any{"enabled": false}}
	w := request(h, "POST", "/api/admin/exits", map[string]any{"profiles": []any{profile}}, admin)
	var exits struct {
		IDs []int64 `json:"ids"`
	}
	json.Unmarshal(w.Body.Bytes(), &exits)
	if w.Code != 201 || len(exits.IDs) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "POST", fmt.Sprintf("/api/admin/exits/%d/probes", exits.IDs[0]), map[string]any{"machine_id": mid, "kernel_type": "singbox"}, admin)
	var p probeResult
	json.Unmarshal(w.Body.Bytes(), &p)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", poll, nil, nil)
	var jobs struct {
		Tasks []struct {
			Lease   string         `json:"lease"`
			Payload map[string]any `json:"payload"`
		} `json:"tasks"`
	}
	json.Unmarshal(w.Body.Bytes(), &jobs)
	if len(jobs.Tasks) != 1 || jobs.Tasks[0].Payload["ping_host"] != "exit.example" {
		t.Fatal("wrong node-to-landing ping destination", w.Body.String())
	}
	body := map[string]any{"machine_id": mid, "token": "ping-machine", "lease": jobs.Tasks[0].Lease, "state": "succeeded", "ping_ms": 12.5, "ping_error_code": ""}
	if w = request(h, "POST", "/api/v3/server/probes/"+p.ID+"/result", body, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/admin/probes/"+p.ID, nil, admin)
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["ping_ms"] != 12.5 {
		t.Fatal("ICMP RTT lost", w.Body.String())
	}
}
