package store

import (
	"context"
	"github.com/chinahch/luma/internal/traffic"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCalendarMonthlyClampAndRestore(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	for _, tt := range []struct{ anchor, now, want string }{
		{"2024-01-31", "2024-02-01", "2024-02-29"},
		{"2024-01-31", "2024-02-29", "2024-03-31"},
		{"2025-01-31", "2025-02-01", "2025-02-28"},
		{"2025-01-30", "2025-02-28", "2025-03-30"},
		{"2025-01-29", "2025-02-28", "2025-03-29"},
	} {
		a, _ := time.ParseInLocation("2006-01-02", tt.anchor, loc)
		n, _ := time.ParseInLocation("2006-01-02", tt.now, loc)
		if got := nextAnniversary(a, n, loc).In(loc).Format("2006-01-02 15:04"); got != tt.want+" 00:00" {
			t.Fatalf("%+v got %s", tt, got)
		}
	}
}

func TestCycleCatchupLateAndManualResetPreserveLedger(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	uid, nid := seed(t, s)
	if err := s.SetTimezone("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	old := now.AddDate(0, -5, 0)
	_, err := s.Pool.Exec(ctx, `UPDATE users SET billing_anchor_at=$2,used_bytes=700,next_reset_at=$3 WHERE id=$1`, uid, old, now.AddDate(0, -3, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RefreshCycles(ctx, now); err != nil {
		t.Fatal(err)
	}
	var used int64
	var start, next time.Time
	if err = s.Pool.QueryRow(ctx, `SELECT used_bytes,cycle_started_at,next_reset_at FROM users WHERE id=$1`, uid).Scan(&used, &start, &next); err != nil {
		t.Fatal(err)
	}
	if used != 0 || next.Before(now) || start.After(now) {
		t.Fatal(used, start, next)
	}
	report := traffic.Report{ReportID: "late-prior-cycle", PeriodStart: start.Add(-time.Minute), PeriodEnd: start.Add(-time.Second), Entries: []traffic.Entry{{UserID: uid, Up: 100}}}
	if _, err = s.Ingest(ctx, nid, report, "reliable"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Ingest(ctx, nid, report, "reliable"); err != nil {
		t.Fatal(err)
	}
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, uid).Scan(&used)
	if used != 0 {
		t.Fatal("late report consumed new quota", used)
	}
	_, err = s.Pool.Exec(ctx, `UPDATE users SET used_bytes=123 WHERE id=$1`, uid)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetUserUsage(ctx, uid, uid); err != nil {
		t.Fatal(err)
	}
	report.ReportID = "late-before-manual"
	report.PeriodStart = now.Add(-time.Minute)
	report.PeriodEnd = now.Add(-time.Second)
	if _, err = s.Ingest(ctx, nid, report, "reliable"); err != nil {
		t.Fatal(err)
	}
	var ledger, audits int
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, uid).Scan(&used)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM billing_ledger`).Scan(&ledger)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_resets WHERE user_id=$1 AND old_used_bytes=123 AND actor_id=$1`, uid).Scan(&audits)
	if used != 0 || ledger != 2 || audits != 1 {
		t.Fatal("reset lost history or charged late report", used, ledger, audits)
	}
}

func TestOrderIdempotencyAndRenewalPreserveCycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	uid, _ := seed(t, s)
	var pid, other int64
	s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('Plan',1000,30,100) RETURNING id`).Scan(&pid)
	s.Pool.QueryRow(ctx, `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES('Other',2000,30,200) RETURNING id`).Scan(&other)
	first, err := s.AllocateOrder(ctx, uid, uid, pid, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, e := s.AllocateOrder(ctx, uid, uid, pid, "request-1")
			if e == nil && o.ID != first.ID {
				t.Errorf("retry created another order")
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var anchor time.Time
	s.Pool.QueryRow(ctx, `UPDATE users SET used_bytes=77 WHERE id=$1 RETURNING billing_anchor_at`, uid).Scan(&anchor)
	if _, err = s.AllocateOrder(ctx, uid, uid, pid, "request-2"); err != nil {
		t.Fatal(err)
	}
	var used int64
	var after, expiry time.Time
	s.Pool.QueryRow(ctx, `SELECT used_bytes,billing_anchor_at,expires_at FROM users WHERE id=$1`, uid).Scan(&used, &after, &expiry)
	if used != 77 || !anchor.Equal(after) || expiry.Sub(first.ExpiresAt) != 30*24*time.Hour {
		t.Fatal("renewal reset cycle", used, anchor, after, expiry)
	}
	if _, err = s.AllocateOrder(ctx, uid, uid, other, "request-3"); err != nil {
		t.Fatal(err)
	}
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, uid).Scan(&used)
	if used != 0 {
		t.Fatal("new plan retained usage", used)
	}
	if _, err = s.AllocateOrder(ctx, uid, uid, other, "request-1"); err == nil {
		t.Fatal("request identity reused for different plan")
	}
	var count int
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM orders`).Scan(&count)
	if count != 3 {
		t.Fatal(count)
	}
}

// A reset waiting for another writer takes effect when it owns the row, not when
// its HTTP request began; backdating it could charge traffic already cleared.
func TestManualResetBoundaryAfterContendedLock(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	uid, _ := seed(t, s)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, uid); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.ResetUserUsage(ctx, uid, uid) }()
	time.Sleep(100 * time.Millisecond)
	beforeRelease := time.Now().UTC().Truncate(time.Microsecond)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	var boundary time.Time
	if err = s.Pool.QueryRow(ctx, `SELECT cycle_started_at FROM users WHERE id=$1`, uid).Scan(&boundary); err != nil {
		t.Fatal(err)
	}
	if boundary.Before(beforeRelease) {
		t.Fatalf("reset boundary was backdated while waiting: %s < %s", boundary, beforeRelease)
	}
}

func TestCalendarCatchupUsesOriginalAnchorAfterOfflineMonths(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	uid, _ := seed(t, s)
	if err := s.SetTimezone("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Pool.Exec(ctx, `UPDATE users SET billing_anchor_at='2024-01-31T15:00:00+08:00',used_bytes=999,next_reset_at='2024-02-29T00:00:00+08:00' WHERE id=$1`, uid)
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2025-02-28T00:00:00+08:00")
	if err = s.RefreshCycles(ctx, now); err != nil {
		t.Fatal(err)
	}
	var used int64
	var started, next time.Time
	if err = s.Pool.QueryRow(ctx, `SELECT used_bytes,cycle_started_at,next_reset_at FROM users WHERE id=$1`, uid).Scan(&used, &started, &next); err != nil {
		t.Fatal(err)
	}
	wantStart, _ := time.Parse(time.RFC3339, "2025-02-28T00:00:00+08:00")
	wantNext, _ := time.Parse(time.RFC3339, "2025-03-31T00:00:00+08:00")
	if used != 0 || !started.Equal(wantStart) || !next.Equal(wantNext) {
		t.Fatal("offline catchup drifted", used, started, next)
	}
	// Repeated lazy checks at the same instant must not clear newly arrived use.
	s.Pool.Exec(ctx, `UPDATE users SET used_bytes=42 WHERE id=$1`, uid)
	if err = s.RefreshCycles(ctx, now); err != nil {
		t.Fatal(err)
	}
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, uid).Scan(&used)
	if used != 42 {
		t.Fatal("same cycle reset twice", used)
	}
}

func TestMigrationPreservesExistingUsageAndInitializesNextCivilBoundary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	if err := s.SetTimezone("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// A second schema in the same transaction exercises the actual v4 -> v5 SQL.
	legacySchema := pgx.Identifier{"legacy_business_" + strings.ReplaceAll(NewID(), "-", "")}.Sanitize()
	if _, err = tx.Exec(ctx, `CREATE SCHEMA `+legacySchema); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `SET LOCAL search_path TO `+legacySchema); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{schema, migration2, migration3, migration4} {
		if _, err = tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var uid int64
	if err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash,uuid,used_bytes,created_at) VALUES('legacy@example.test','fixture',$1,99,'2024-01-31T23:30:00+08:00') RETURNING id`, NewID()).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, migration5); err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, "2025-02-01T15:00:00+08:00")
	if err = s.refreshUserCycle(ctx, tx, uid, now); err != nil {
		t.Fatal(err)
	}
	var used int64
	var anchor, next time.Time
	if err = tx.QueryRow(ctx, `SELECT used_bytes,billing_anchor_at,next_reset_at FROM users WHERE id=$1`, uid).Scan(&used, &anchor, &next); err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339, "2025-02-28T00:00:00+08:00")
	if used != 99 || anchor.In(s.calendarLocation()).Day() != 31 || !next.Equal(want) {
		t.Fatal("migration cleared usage or lost local anchor", used, anchor, next)
	}
}
