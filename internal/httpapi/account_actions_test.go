package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/chinahch/luma/internal/traffic"
)

func TestPersistentSubscriptionConcurrentRecoveryAndIdentity(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	var wg sync.WaitGroup
	paths := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request(h, "GET", "/api/me/subscription", nil, user)
			if w.Code != 200 {
				t.Errorf("stable link: %d %s", w.Code, w.Body.String())
				return
			}
			var v struct{ Path string }
			json.Unmarshal(w.Body.Bytes(), &v)
			paths <- v.Path
		}()
	}
	wg.Wait()
	close(paths)
	first := ""
	for p := range paths {
		if p == "" || first != "" && first != p {
			t.Fatal("concurrent read rotated subscription")
		}
		first = p
	}
	if first == "" {
		t.Fatal("no subscription")
	}
	var cipher, hash []byte
	s.Pool.QueryRow(context.Background(), `SELECT subscription_cipher,subscription_hash FROM users WHERE id=2`).Scan(&cipher, &hash)
	if bytes.Contains(cipher, []byte(strings.TrimPrefix(first, "/sub/"))) || !bytes.Equal(hash, store.Hash(strings.TrimPrefix(first, "/sub/"))) {
		t.Fatal("plaintext or mismatched subscription storage")
	}
	if w := request(h, "GET", "/api/admin/users/1/subscription", nil, user); w.Code != 403 {
		t.Fatal("cross account link", w.Code)
	}
	old := store.NewToken()
	s.Pool.Exec(context.Background(), `UPDATE users SET subscription_hash=$1,subscription_cipher=NULL WHERE id=2`, store.Hash(old))
	w := request(h, "GET", "/api/me/subscription", nil, user)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"needs_recovery":true`) {
		t.Fatal("old link silently revoked", w.Code, w.Body.String())
	}
	if w = request(h, "POST", "/api/me/subscription/recover", map[string]string{"token": store.NewToken()}, user); w.Code != 400 {
		t.Fatal("unrelated token recovered", w.Code)
	}
	if w = request(h, "POST", "/api/me/subscription/recover", map[string]string{"token": old}, user); w.Code != 200 {
		t.Fatal("old token recovery", w.Code, w.Body.String())
	}
	w = request(h, "GET", "/api/me/subscription", nil, user)
	if !strings.Contains(w.Body.String(), "/sub/"+old) {
		t.Fatal("recovery not persistent")
	}
	var uuid string
	s.Pool.QueryRow(context.Background(), `SELECT uuid FROM users WHERE id=2`).Scan(&uuid)
	w = request(h, "POST", "/api/admin/users/2/identity", map[string]any{}, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var next string
	var nextHash []byte
	s.Pool.QueryRow(context.Background(), `SELECT uuid,subscription_hash FROM users WHERE id=2`).Scan(&next, &nextHash)
	if next == uuid || bytes.Equal(nextHash, store.Hash(old)) {
		t.Fatal("identity reset did not revoke both credentials")
	}
}

func TestSubscriptionWrongKeyAndDeletedUserKeepHistory(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	first := request(h, "GET", "/api/me/subscription", nil, user)
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	wrong := New(s, nil, Config{MasterKey: strings.Repeat("02", 32)})
	if w := request(wrong, "GET", "/api/me/subscription", nil, user); w.Code != 503 {
		t.Fatal("bad key silently replaced link", w.Code)
	}
	if w := request(h, "GET", "/api/me/subscription", nil, user); w.Body.String() != first.Body.String() {
		t.Fatal("failed read changed old subscription")
	}
	var nodeID int64
	ctx := context.Background()
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash) VALUES('history',$1) RETURNING id`, store.Hash("history-node")).Scan(&nodeID)
	now := time.Now().UTC()
	report := traffic.Report{ReportID: "keep-history", PeriodStart: now.Add(-time.Minute), PeriodEnd: now, Entries: []traffic.Entry{{UserID: 2, Up: 12, Down: 34}}}
	if _, e := s.Ingest(ctx, nodeID, report, "reliable"); e != nil {
		t.Fatal(e)
	}
	if w := request(h, "POST", "/api/admin/users/2/reset-usage", map[string]any{}, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(h, "DELETE", "/api/admin/users/2", nil, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var entries, ledger, resets int
	var used int64
	if e := s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM usage_entries WHERE user_id=2),(SELECT count(*) FROM billing_ledger WHERE user_id=2),(SELECT count(*) FROM usage_resets WHERE user_id=2),used_bytes FROM users WHERE id=2`).Scan(&entries, &ledger, &resets, &used); e != nil {
		t.Fatal(e)
	}
	if entries != 1 || ledger != 1 || resets != 1 || used != 0 {
		t.Fatal("reset/delete destroyed immutable history", entries, ledger, resets, used)
	}
}

func TestInvitationRegistrationThrottlesAndCannotChooseRole(t *testing.T) {
	h, _ := setup(t)
	u := login(t, h, "user@example.test")
	w := request(h, "GET", "/api/me/invite", nil, u)
	var inv struct{ Code string }
	json.Unmarshal(w.Body.Bytes(), &inv)
	if w = request(h, "POST", "/api/register", map[string]any{"email": "elevated@example.test", "password": "new-password-123", "invite_code": inv.Code, "role": "admin"}, nil); w.Code != 400 {
		t.Fatal("public role accepted", w.Code)
	}
	for i := 0; i < 5; i++ {
		w = request(h, "POST", "/api/register", map[string]any{"email": "invalid", "password": "x", "invite_code": inv.Code}, nil)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w = request(h, "POST", "/api/register", map[string]any{"email": "valid@example.test", "password": "new-password-123", "invite_code": inv.Code}, nil)
	if w.Code != 429 {
		t.Fatal("public registration not rate limited", w.Code)
	}
}

func TestInvitationRegistrationAndUserDeletion(t *testing.T) {
	_, s := setup(t)
	h := New(s, nil, Config{MasterKey: strings.Repeat("01", 32)})
	admin := login(t, h, "admin@example.test")
	user := login(t, h, "user@example.test")
	w := request(h, "GET", "/api/me/invite", nil, user)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var inv struct{ Code, Path string }
	json.Unmarshal(w.Body.Bytes(), &inv)
	if inv.Code == "" || !strings.Contains(inv.Path, inv.Code) {
		t.Fatal("no invite")
	}
	w = request(h, "POST", "/api/register", map[string]string{"email": "invited@example.test", "password": "new-password-123", "invite_code": inv.Code}, nil)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var id, inviter, quota int64
	var role string
	s.Pool.QueryRow(context.Background(), `SELECT id,inviter_id,quota_bytes,role FROM users WHERE email='invited@example.test'`).Scan(&id, &inviter, &quota, &role)
	if inviter != 2 || quota != 0 || role != "user" {
		t.Fatal("untrusted signup privileges", id, inviter, quota, role)
	}
	w = request(h, "GET", "/api/admin/users/2/invites", nil, admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "invited@example.test") {
		t.Fatal("missing real invitation record", w.Code, w.Body.String())
	}
	if w = request(h, "GET", "/api/admin/users/2/invites", nil, user); w.Code != 403 {
		t.Fatal("invites crossed permission", w.Code)
	}
	if w = request(h, "DELETE", "/api/admin/users/1", nil, admin); w.Code != 409 {
		t.Fatal("self deletion", w.Code)
	}
	w = request(h, "DELETE", fmt.Sprint("/api/admin/users/", id), nil, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var deleted, active bool
	s.Pool.QueryRow(context.Background(), `SELECT deleted_at IS NOT NULL,active FROM users WHERE id=$1`, id).Scan(&deleted, &active)
	if !deleted || active {
		t.Fatal("not soft deleted")
	}
	// Revoking an inviter also invalidates outstanding public invitation links.
	w = request(h, "DELETE", "/api/admin/users/2", nil, admin)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(h, "GET", "/api/me", nil, user); w.Code != http.StatusUnauthorized {
		t.Fatal("deleted session retained", w.Code)
	}
	if w = request(h, "POST", "/api/register", map[string]string{"email": "late@example.test", "password": "new-password-123", "invite_code": inv.Code}, nil); w.Code != 400 {
		t.Fatal("deleted inviter accepted", w.Code)
	}
}
