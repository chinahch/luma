package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestMachineCardLayoutPersistsForAdminAndRejectsInvalidIDs(t *testing.T) {
	h, _ := setup(t)
	admin := login(t, h, "admin@example.test")
	ids := []int64{}
	for i := 0; i < 2; i++ {
		w := request(h, "POST", "/api/admin/machines", map[string]any{"name": fmt.Sprint("machine ", i)}, admin)
		var m struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal(w.Body.Bytes(), &m)
		ids = append(ids, m.ID)
	}
	w := request(h, "PUT", "/api/admin/machines/layout", map[string]any{"machine_ids": []int64{ids[1], ids[0]}}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/admin/machines/layout", nil, admin)
	var got []int64
	json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || len(got) != 2 || got[0] != ids[1] || got[1] != ids[0] {
		t.Fatal("layout not persisted", w.Code, w.Body.String())
	}
	for _, bad := range [][]int64{{ids[0], ids[0]}, {999999}, {0}} {
		w = request(h, "PUT", "/api/admin/machines/layout", map[string]any{"machine_ids": bad}, admin)
		if w.Code != 400 {
			t.Fatal("invalid layout accepted", w.Code)
		}
	}
}
func TestCountryResponseMustMatchRequestedPhysicalIP(t *testing.T) {
	if country, err := parseCountryResponse([]byte(`{"ip":"8.8.8.8","country":"US"}`), "8.8.8.8"); err != nil || country != "US" {
		t.Fatal(country, err)
	}
	for _, raw := range []string{`{"ip":"1.1.1.1","country":"AU"}`, `{"ip":"8.8.8.8","country":"bad"}`, `invalid`} {
		if _, err := parseCountryResponse([]byte(raw), "8.8.8.8"); err == nil {
			t.Fatal("invalid country evidence accepted")
		}
	}
}
