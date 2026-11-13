package httpapi

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAdminRenewalQueuePagesAllRequests(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	admin := login(t, h, "admin@example.test")
	var pid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('Queue plan',100,30,100) RETURNING id`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO users(email,password_hash,uuid) SELECT 'queue'||i||'@example.test','unused',md5('queue-'||i::text)::uuid FROM generate_series(1,21)i`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO renewal_requests(user_id,plan_id,request_id,plan_name,quota_bytes,duration_days,price_cents) SELECT id,$1,'queue','Queue plan',100,30,100 FROM users WHERE email LIKE 'queue%'`, pid); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, path := range []string{"/api/admin/renewals?page=1", "/api/admin/renewals?page=2"} {
		w := request(h, "GET", path, nil, admin)
		var result struct {
			Total int `json:"total"`
			Rows  []struct {
				ID int64 `json:"id"`
			} `json:"rows"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Total != 21 {
			t.Fatal("queue pagination unavailable", w.Code, w.Body.String())
		}
		for _, r := range result.Rows {
			if seen[r.ID] {
				t.Fatal("request repeated across pages")
			}
			seen[r.ID] = true
		}
	}
	if len(seen) != 21 {
		t.Fatal("requests omitted", len(seen))
	}
}
