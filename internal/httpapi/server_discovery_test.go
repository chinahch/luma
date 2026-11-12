package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestServerAutomaticallyDiscoversAssignedNodesAndKernelEdits(t *testing.T) {
	h, _ := setup(t)
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "server"}, admin)
	var m struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &m)
	auth := map[string]any{"machine_id": m.ID, "token": m.Token}
	w = request(h, "POST", "/api/v2/server/machine/nodes", auth, nil)
	var discovery struct {
		Nodes []struct {
			ID         int64  `json:"id"`
			KernelType string `json:"kernel_type"`
		}
		BaseConfig struct {
			Pull int `json:"pull_interval"`
			Push int `json:"push_interval"`
		} `json:"base_config"`
	}
	json.Unmarshal(w.Body.Bytes(), &discovery)
	if w.Code != 200 || len(discovery.Nodes) != 0 || discovery.BaseConfig.Pull != 10 || discovery.BaseConfig.Push != 10 {
		t.Fatal("empty server discovery intervals", w.Code, w.Body.String())
	}
	w = request(h, "POST", "/api/admin/nodes", map[string]any{"name": "assigned", "machine_id": m.ID, "config": map[string]any{"protocol": "vless", "server_port": 443, "kernel_type": "xray"}}, admin)
	if w.Code != 201 {
		t.Fatal("node save", w.Code, w.Body.String())
	}
	w = request(h, "POST", "/api/v2/server/machine/nodes", auth, nil)
	json.Unmarshal(w.Body.Bytes(), &discovery)
	if len(discovery.Nodes) != 1 || discovery.Nodes[0].KernelType != "xray" {
		t.Fatal("server cannot discover saved node/kernel", w.Body.String())
	}
	path := fmt.Sprintf("/api/admin/nodes/%d", discovery.Nodes[0].ID)
	body := map[string]any{"name": "edited", "machine_id": m.ID, "config": map[string]any{"protocol": "vless", "server_port": 443, "kernel_type": "singbox"}}
	if w = request(h, "PATCH", path, body, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "POST", "/api/v2/server/machine/nodes", auth, nil)
	json.Unmarshal(w.Body.Bytes(), &discovery)
	if len(discovery.Nodes) != 1 || discovery.Nodes[0].KernelType != "singbox" {
		t.Fatal("edited kernel not rediscovered", w.Body.String())
	}
	body["machine_id"] = nil
	if w = request(h, "PATCH", path, body, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "POST", "/api/v2/server/machine/nodes", auth, nil)
	json.Unmarshal(w.Body.Bytes(), &discovery)
	if len(discovery.Nodes) != 0 {
		t.Fatal("unassigned node still dispatched")
	}
}
