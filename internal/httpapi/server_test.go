package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func setup(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	cfg, e := pgx.ParseConfig(dsn)
	if e != nil || !strings.HasSuffix(cfg.Database, "_test") {
		t.Fatal("dedicated *_test database required")
	}
	ctx := context.Background()
	s, e := store.Open(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	schema := "luma_http_" + strings.ReplaceAll(store.NewID(), "-", "")
	if _, e = s.Pool.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	s.Pool.Close()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	s, e = store.Open(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Pool.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); s.Pool.Close() })
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.MinCost)
	for _, role := range []string{"admin", "user"} {
		if _, e = s.Pool.Exec(ctx, `INSERT INTO users(email,password_hash,role,uuid,quota_bytes,expires_at) VALUES($1,$2,$3,$4,1000000000,now()+interval '1 day')`, role+"@example.test", string(hash), role, store.NewID()); e != nil {
			t.Fatal(e)
		}
	}
	return New(s, nil, Config{}), s
}

func request(h http.Handler, method, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, h http.Handler, email string) *http.Cookie {
	t.Helper()
	w := request(h, "POST", "/api/login", map[string]string{"email": email, "password": "test-password-123"}, nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "luma_session" {
			return c
		}
	}
	t.Fatal("no session")
	return nil
}
func TestAuthenticationAndRoleIsolation(t *testing.T) {
	h, _ := setup(t)
	if w := request(h, "GET", "/api/me", nil, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	user := login(t, h, "user@example.test")
	if w := request(h, "GET", "/api/admin/users", nil, user); w.Code != 403 {
		t.Fatal("user read admin data", w.Code)
	}
	if w := request(h, "GET", "/api/usage?user_id=1", nil, user); w.Code != 403 {
		t.Fatal("user read other account", w.Code, w.Body.String())
	}
	admin := login(t, h, "admin@example.test")
	if w := request(h, "GET", "/api/admin/users", nil, admin); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("POST", "/api/logout", nil)
	r.AddCookie(admin)
	r.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign origin accepted", w.Code)
	}
}
func TestReliableNodeReportAndMachineBoundary(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	var machineID, nodeID int64
	s.Pool.QueryRow(ctx, `INSERT INTO machines(name,token_hash) VALUES('m1',$1) RETURNING id`, store.Hash("machine-token")).Scan(&machineID)
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,machine_id,token_hash) VALUES('n1',$1,$2) RETURNING id`, machineID, store.Hash("node-token")).Scan(&nodeID)
	now := time.Now().UTC()
	body := map[string]any{"token": "node-token", "node_id": nodeID, "report_id": "batch-1", "period_start": now.Add(-time.Minute), "period_end": now, "entries": []map[string]any{{"user_id": 2, "up_bytes": 100, "down_bytes": 200}}}
	for i := 0; i < 2; i++ {
		w := request(h, "POST", "/api/v3/server/report", body, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	var used int64
	s.Pool.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=2`).Scan(&used)
	if used != 300 {
		t.Fatal("duplicate billing", used)
	}
	body["token"] = "invalid"
	if w := request(h, "POST", "/api/v3/server/report", body, nil); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	var otherID int64
	s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash) VALUES('foreign',$1) RETURNING id`, store.Hash("other")).Scan(&otherID)
	if w := request(h, "POST", "/api/v2/server/report", map[string]any{"token": "machine-token", "machine_id": machineID, "node_id": otherID, "traffic": map[string]any{"2": []int{1, 2}}}, nil); w.Code != 403 {
		t.Fatal("cross-machine report accepted", w.Code, w.Body.String())
	}
}

func TestUserExpiryCanBeClearedAndUnrelatedEditsPreserveIt(t *testing.T) {
	h, s := setup(t)
	admin := login(t, h, "admin@example.test")
	for _, payload := range []map[string]any{{"expires_at": nil}, {"quota_bytes": int64(2000000000)}} {
		w := request(h, "PATCH", "/api/admin/users/2", payload, admin)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var expiry *time.Time
		if err := s.Pool.QueryRow(context.Background(), `SELECT expires_at FROM users WHERE id=2`).Scan(&expiry); err != nil {
			t.Fatal(err)
		}
		if expiry != nil {
			t.Fatal("expiry was not cleared/preserved", expiry)
		}
	}
}
