package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRenewalRequiresAdminApprovalAndAppliesOnce(t *testing.T) {
	h, s := setup(t)
	admin, user := login(t, h, "admin@example.test"), login(t, h, "user@example.test")
	ctx := context.Background()
	var plan, uid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('Monthly',5000,30,1200) RETURNING id`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().AddDate(0, 0, 10).Truncate(time.Microsecond)
	if err := s.Pool.QueryRow(ctx, `UPDATE users SET plan_id=$1,expires_at=$2,used_bytes=123 WHERE role='user' RETURNING id`, plan, expiry).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	create := func(key string) int64 {
		t.Helper()
		w := request(h, "POST", "/api/me/renewals", map[string]any{"request_id": key}, user)
		var result struct {
			ID int64 `json:"id"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ID <= 0 {
			t.Fatal("renewal submission failed", w.Code, w.Body.String())
		}
		return result.ID
	}
	id := create("first")
	if got := create("first"); got != id {
		t.Fatal("submission retry created another request")
	}
	if w := request(h, "POST", "/api/me/renewals", map[string]any{"request_id": "second"}, user); w.Code != 409 {
		t.Fatal("duplicate pending renewal accepted", w.Code)
	}
	var got time.Time
	s.Pool.QueryRow(ctx, `SELECT expires_at FROM users WHERE id=$1`, uid).Scan(&got)
	if !got.Equal(expiry) {
		t.Fatal("submission granted entitlement before approval")
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE plans SET price_cents=9000,duration_days=99 WHERE id=$1`, plan); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/admin/renewals/%d/approve", id)
	if w := request(h, "POST", path, map[string]any{}, user); w.Code != 403 {
		t.Fatal("user approved own order", w.Code)
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- request(h, "POST", path, map[string]any{}, admin).Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatal("approval retry failed", code)
		}
	}
	var used, price, days, count int64
	if err := s.Pool.QueryRow(ctx, `SELECT expires_at,used_bytes FROM users WHERE id=$1`, uid).Scan(&got, &used); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(expiry.AddDate(0, 0, 30)) || used != 123 {
		t.Fatal("renewal extension or cycle usage incorrect", got, used)
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*),min(price_cents),min(duration_days) FROM orders WHERE user_id=$1`, uid).Scan(&count, &price, &days); err != nil {
		t.Fatal(err)
	}
	if count != 1 || price != 1200 || days != 30 {
		t.Fatal("approval lost quote snapshot or duplicated grant", count, price, days)
	}
	for _, path := range []string{"/api/me/renewals", "/api/me/orders", "/api/me/renewal-plan"} {
		if w := request(h, "GET", path, nil, user); w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	if w := request(h, "GET", "/api/admin/renewals", nil, user); w.Code != 403 {
		t.Fatal("user read all requests", w.Code)
	}
}

func TestRenewalCancellationAndPlanChangesDoNotGrantService(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	admin, user := login(t, h, "admin@example.test"), login(t, h, "user@example.test")
	var plan int64
	s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('Monthly',5000,30,1200) RETURNING id`).Scan(&plan)
	s.Pool.Exec(ctx, `UPDATE users SET plan_id=$1 WHERE role='user'`, plan)
	create := func(key string) int64 {
		t.Helper()
		w := request(h, "POST", "/api/me/renewals", map[string]any{"request_id": key}, user)
		var r struct {
			ID int64 `json:"id"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &r) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		return r.ID
	}
	id := create("cancel")
	if w := request(h, "POST", fmt.Sprintf("/api/me/renewals/%d/cancel", id), map[string]any{}, admin); w.Code != 404 {
		t.Fatal("other account canceled renewal", w.Code)
	}
	if w := request(h, "POST", fmt.Sprintf("/api/me/renewals/%d/cancel", id), map[string]any{}, user); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", fmt.Sprintf("/api/admin/renewals/%d/approve", id), map[string]any{}, admin); w.Code != 409 {
		t.Fatal("canceled request approved", w.Code)
	}
	id = create("changed")
	s.Pool.Exec(ctx, `UPDATE users SET plan_id=NULL WHERE role='user'`)
	if w := request(h, "POST", fmt.Sprintf("/api/admin/renewals/%d/approve", id), map[string]any{}, admin); w.Code != 409 {
		t.Fatal("changed plan silently replaced", w.Code)
	}
	if w := request(h, "POST", fmt.Sprintf("/api/admin/renewals/%d/reject", id), map[string]any{}, admin); w.Code != 200 {
		t.Fatal("cannot reject obsolete request", w.Code)
	}
	if w := request(h, "POST", "/api/me/renewals", map[string]any{"request_id": "no-plan"}, user); w.Code != 409 {
		t.Fatal("unassigned user created renewal", w.Code)
	}
}
