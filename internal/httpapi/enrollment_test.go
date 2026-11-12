package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/chinahch/luma/internal/store"
)

func TestAutomaticMachineEnrollmentLifecycle(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "auto"}, admin)
	var m struct {
		ID    int64  `json:"id"`
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &m)
	path := fmt.Sprintf("/api/admin/machines/%d/enrollment", m.ID)
	if w = request(h, "POST", path, map[string]any{}, user); w.Code != 403 {
		t.Fatal("user generated enrollment", w.Code)
	}
	if w = request(h, "POST", path, map[string]any{}, nil); w.Code != 401 {
		t.Fatal("unauthenticated enrollment", w.Code)
	}
	r := httptest.NewRequest("POST", path, strings.NewReader("{}"))
	r.AddCookie(admin)
	r.Header.Set("Origin", "https://other.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign origin enrollment", w.Code)
	}
	var code struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expires_at"`
	}
	w = request(h, "POST", path, map[string]any{}, admin)
	if w.Code != 200 {
		t.Fatalf("generate %d %s", w.Code, w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &code)
	if len(code.Code) != 43 || code.ExpiresAt == "" {
		t.Fatal("invalid generated command metadata")
	}
	// Merely viewing a new installation command must not revoke a running server.
	authPath := "/api/v2/server/machine/nodes"
	if w = request(h, "POST", authPath, map[string]any{"machine_id": m.ID, "token": m.Token}, nil); w.Code != 200 {
		t.Fatal("generation revoked current client")
	}
	candidate := store.NewToken()
	claim := map[string]any{"machine_id": m.ID, "code": code.Code, "token": candidate}
	for i := 0; i < 2; i++ {
		if w = request(h, "POST", "/api/server/enroll", claim, nil); w.Code != 200 {
			t.Fatalf("claim/replay %d %s", w.Code, w.Body.String())
		}
	}
	for _, item := range []struct {
		token string
		want  int
	}{{m.Token, 401}, {candidate, 200}} {
		if w = request(h, "POST", authPath, map[string]any{"machine_id": m.ID, "token": item.token}, nil); w.Code != item.want {
			t.Fatalf("auth %d want%d", w.Code, item.want)
		}
	}
	claim["token"] = store.NewToken()
	if w = request(h, "POST", "/api/server/enroll", claim, nil); w.Code != 409 {
		t.Fatal("different candidate replay accepted", w.Code)
	}
	claim["token"] = candidate
	claim["machine_id"] = m.ID + 1
	if w = request(h, "POST", "/api/server/enroll", claim, nil); w.Code != 401 {
		t.Fatal("cross machine accepted", w.Code)
	}
	claim["machine_id"] = m.ID
	var raw string
	s.Pool.QueryRow(context.Background(), `SELECT row_to_json(e)::text FROM machine_enrollments e WHERE machine_id=$1`, m.ID).Scan(&raw)
	if strings.Contains(raw, code.Code) || strings.Contains(raw, candidate) {
		t.Fatal("plaintext capability or credential stored")
	}
	w = request(h, "POST", path, map[string]any{}, admin)
	var next struct {
		Code string `json:"code"`
	}
	json.Unmarshal(w.Body.Bytes(), &next)
	if w = request(h, "POST", "/api/server/enroll", claim, nil); w.Code != 401 {
		t.Fatal("superseded code accepted", w.Code)
	}
	claim["code"] = next.Code
	s.Pool.Exec(context.Background(), `UPDATE machine_enrollments SET expires_at=now()-interval '1 minute' WHERE machine_id=$1`, m.ID)
	if w = request(h, "POST", "/api/server/enroll", claim, nil); w.Code != 401 {
		t.Fatal("expired command accepted", w.Code)
	}
	if w = request(h, "PATCH", fmt.Sprintf("/api/admin/machines/%d", m.ID), map[string]any{"name": "renamed"}, admin); w.Code != 200 {
		t.Fatal("rename", w.Code)
	}
	w = request(h, "GET", "/api/admin/machines", nil, admin)
	if !strings.Contains(w.Body.String(), "renamed") || strings.Contains(w.Body.String(), candidate) {
		t.Fatal("machine list")
	}
	if w = request(h, "DELETE", fmt.Sprintf("/api/admin/machines/%d", m.ID), nil, admin); w.Code != 200 {
		t.Fatal("delete enrollment machine", w.Code)
	}
}

func TestConcurrentEnrollmentHasOneWinnerAndCannotRestoreRotatedCredential(t *testing.T) {
	h, _ := setup(t)
	admin := login(t, h, "admin@example.test")
	w := request(h, "POST", "/api/admin/machines", map[string]any{"name": "race"}, admin)
	var m struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &m)
	path := fmt.Sprintf("/api/admin/machines/%d", m.ID)
	w = request(h, "POST", path+"/enrollment", map[string]any{}, admin)
	var code struct {
		Code string `json:"code"`
	}
	json.Unmarshal(w.Body.Bytes(), &code)
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	winner := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token := store.NewToken()
			r := request(h, "POST", "/api/server/enroll", map[string]any{"machine_id": m.ID, "code": code.Code, "token": token}, nil)
			codes <- r.Code
			if r.Code == 200 {
				winner <- token
			}
		}()
	}
	wg.Wait()
	close(codes)
	wins := 0
	for status := range codes {
		if status == 200 {
			wins++
		} else if status != 409 {
			t.Errorf("unexpected claim status%d", status)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d", wins)
	}
	token := <-winner
	if w = request(h, "POST", path+"/token", map[string]any{}, admin); w.Code != 200 {
		t.Fatal("rotate", w.Code)
	}
	if w = request(h, "POST", "/api/server/enroll", map[string]any{"machine_id": m.ID, "code": code.Code, "token": token}, nil); w.Code != 409 {
		t.Fatal("old claim restored revoked credential", w.Code)
	}
}
