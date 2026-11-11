package store

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chinahch/luma/internal/traffic"
	"github.com/jackc/pgx/v5"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a dedicated PostgreSQL *_test database")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("refuse non-test database")
	}
	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "luma_test_" + strings.ReplaceAll(NewID(), "-", "")
	if _, err = s.Pool.Exec(context.Background(), "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	s.Pool.Close()
	testURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := testURL.Query()
	query.Set("search_path", schema)
	testURL.RawQuery = query.Encode()
	s, err = Open(context.Background(), testURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Pool.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		s.Pool.Close()
	})
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func seed(t *testing.T, s *Store) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	var uid, nid int64
	if err := s.Pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,uuid) VALUES('user@example.test','not-a-login-hash',$1) RETURNING id`, NewID()).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash,multiplier_milli) VALUES('node',$1,1500) RETURNING id`, []byte("fixture-token")).Scan(&nid); err != nil {
		t.Fatal(err)
	}
	return uid, nid
}

func TestIngestIdempotentAndConflict(t *testing.T) {
	s := testStore(t)
	uid, nid := seed(t, s)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	r := traffic.Report{ReportID: "persistent-batch-1", PeriodStart: now.Add(-time.Minute), PeriodEnd: now, Entries: []traffic.Entry{{UserID: uid, Up: 100, Down: 200}}}
	duplicate, err := s.Ingest(ctx, nid, r, "reliable")
	if err != nil || duplicate {
		t.Fatal(duplicate, err)
	}
	s.Pool.Exec(ctx, `UPDATE nodes SET multiplier_milli=3000 WHERE id=$1`, nid)
	duplicate, err = s.Ingest(ctx, nid, r, "reliable")
	if err != nil || !duplicate {
		t.Fatal(duplicate, err)
	}
	var used, count int64
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, uid).Scan(&used)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM billing_ledger`).Scan(&count)
	if used != 450 || count != 1 {
		t.Fatal("duplicate billed again", used, count)
	}
	r.Entries[0].Up = 101
	_, err = s.Ingest(ctx, nid, r, "reliable")
	if !errors.Is(err, ErrConflict) {
		t.Fatal("changed payload must conflict", err)
	}
}

func TestIngestUnknownUserRollsBackWholeReport(t *testing.T) {
	s := testStore(t)
	uid, nid := seed(t, s)
	ctx := context.Background()
	now := time.Now().UTC()
	r := traffic.Report{ReportID: "batch-bad-user", PeriodStart: now.Add(-time.Minute), PeriodEnd: now, Entries: []traffic.Entry{{UserID: uid, Up: 10}, {UserID: 999999, Up: 10}}}
	if _, err := s.Ingest(ctx, nid, r, "reliable"); err == nil {
		t.Fatal("unknown user accepted")
	}
	var used, count int64
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1`, uid).Scan(&used)
	s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_reports`).Scan(&count)
	if used != 0 || count != 0 {
		t.Fatal("partial accounting committed", used, count)
	}
}

func TestLateReportDoesNotReplaceCurrentRateWindow(t *testing.T) {
	s := testStore(t)
	uid, nid := seed(t, s)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	fresh := traffic.Report{ReportID: "fresh", PeriodStart: now.Add(-time.Minute), PeriodEnd: now, Entries: []traffic.Entry{{UserID: uid, Up: 100}}}
	if _, err := s.Ingest(ctx, nid, fresh, "reliable"); err != nil {
		t.Fatal(err)
	}
	old := traffic.Report{ReportID: "late", PeriodStart: now.Add(-2 * time.Hour), PeriodEnd: now.Add(-time.Hour), Entries: []traffic.Entry{{UserID: uid, Up: 9000}}}
	if _, err := s.Ingest(ctx, nid, old, "reliable"); err != nil {
		t.Fatal(err)
	}
	var up int64
	if err := s.Pool.QueryRow(ctx, `SELECT last_up_bytes FROM nodes WHERE id=$1`, nid).Scan(&up); err != nil {
		t.Fatal(err)
	}
	if up != 100 {
		t.Fatalf("old window replaced recent rate: %d", up)
	}
}
