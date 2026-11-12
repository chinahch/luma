package httpapi

import (
	"context"
	"encoding/json"
	"github.com/chinahch/luma/internal/store"
	"testing"
)

func TestMachinePhysicalAddressesRoundtripWithoutEgressFallback(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	var mid int64
	s.Pool.QueryRow(context.Background(), `INSERT INTO machines(name,token_hash) VALUES('address fixture',$1) RETURNING id`, store.Hash("address-token")).Scan(&mid)
	w := request(h, "POST", "/api/v2/server/machine/status", map[string]any{"token": "address-token", "machine_id": mid, "public_addresses": map[string]any{"source": "physical_interface", "ipv4": []string{"1.1.1.1", "10.0.0.2", "100.64.0.2"}, "ipv6": []string{"2606:4700:4700::1111", "fd00::1"}}}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var status struct {
		Public struct {
			Source     string `json:"source"`
			IPv4, IPv6 []string
			Detected   string `json:"detected_at"`
		} `json:"public_addresses"`
	}
	var raw []byte
	s.Pool.QueryRow(context.Background(), `SELECT status FROM machines WHERE id=$1`, mid).Scan(&raw)
	json.Unmarshal(raw, &status)
	if status.Public.Source != "physical_interface" || len(status.Public.IPv4) != 1 || len(status.Public.IPv6) != 1 || status.Public.Detected == "" {
		t.Fatal("confirmed addresses not safely preserved", string(raw))
	}
	if w = request(h, "GET", "/api/admin/machines", nil, admin); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = request(h, "POST", "/api/v2/server/machine/status", map[string]any{"token": "address-token", "machine_id": mid, "public_addresses": map[string]any{"source": "warp", "ipv4": []string{"1.1.1.1"}}}, nil)
	if w.Code != 400 {
		t.Fatal("unconfirmed address source accepted", w.Code)
	}
}
