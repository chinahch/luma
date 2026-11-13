package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/chinahch/luma/internal/proxy"
	"github.com/chinahch/luma/internal/store"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionInfoNodesCloneFirstNodeForEachAccount(t *testing.T) {
	for _, proto := range []string{"vless", "vmess"} {
		t.Run(proto, func(t *testing.T) {
			_, s := setup(t)
			h := New(s, nil, Config{Timezone: "Asia/Shanghai"})
			ctx := context.Background()
			var id int64
			cfg := `{"protocol":"` + proto + `","server_port":443,"network":"ws","tls":1,"tls_settings":{"server_name":"cert.example"},"networkSettings":{"path":"/ws","headers":{"Host":"front.example"}}}`
			if err := s.Pool.QueryRow(ctx, `INSERT INTO nodes(name,token_hash,public_host,config) VALUES('First',$1,'node.example',$2) RETURNING id`, store.Hash(store.NewToken()), cfg).Scan(&id); err != nil {
				t.Fatal(err)
			}
			for i, role := range []string{"admin", "user"} {
				token := store.NewToken()
				var expiry *time.Time
				if i == 1 {
					v := time.Now().UTC().AddDate(0, 0, 20)
					expiry = &v
				}
				quota := int64((i + 1) * 1000000000)
				if _, err := s.Pool.Exec(ctx, `UPDATE users SET subscription_hash=$1,quota_bytes=$2,used_bytes=100000000,expires_at=$3 WHERE role=$4`, store.Hash(token), quota, expiry, role); err != nil {
					t.Fatal(err)
				}
				w := request(h, "GET", "/sub/"+token, nil, nil)
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				decoded, err := base64.StdEncoding.DecodeString(w.Body.String())
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(string(decoded), "\n")
				if len(lines) != 3 {
					t.Fatalf("want two info nodes plus actual node, got %d", len(lines))
				}
				profiles := make([]proxy.Profile, 3)
				for j, line := range lines {
					p, e := proxy.Parse(line)
					if e != nil {
						t.Fatal(e)
					}
					profiles[j] = p
				}
				wantFlow := "剩余流量：0.90 GB"
				if i == 1 {
					wantFlow = "剩余流量：1.90 GB"
				}
				if profiles[0].Name != wantFlow {
					t.Fatal("account remaining traffic incorrect", profiles[0].Name)
				}
				wantExpiry := "有效期：长期有效"
				if expiry != nil {
					loc, _ := time.LoadLocation("Asia/Shanghai")
					wantExpiry = "有效期：" + expiry.In(loc).Format("2006-01-02 15:04")
				}
				if profiles[1].Name != wantExpiry {
					t.Fatal("account expiry incorrect", profiles[1].Name)
				}
				profiles[0].Name = "First"
				profiles[1].Name = "First"
				first, _ := json.Marshal(profiles[2])
				for _, p := range profiles[:2] {
					b, _ := json.Marshal(p)
					if string(first) != string(b) {
						t.Fatal("info node changed first node connection parameters")
					}
				}
				if !strings.Contains(w.Header().Get("Subscription-Userinfo"), "download=100000000") {
					t.Fatal("standard subscription usage header lost")
				}
			}
		})
	}
}

func TestExpiredSubscriptionReplacesAllNodesWithLoopbackNotice(t *testing.T) {
	h, s := setup(t)
	ctx := context.Background()
	token := store.NewToken()
	for _, tc := range []struct {
		label  string
		expiry time.Time
		quota  int64
	}{
		{"订阅已过期，需要续费", time.Now().Add(-time.Hour), 1000},
		{"流量已用尽，请续费", time.Now().Add(time.Hour), 0},
	} {
		if _, err := s.Pool.Exec(ctx, `UPDATE users SET subscription_hash=$1,expires_at=$2,quota_bytes=$3 WHERE role='user'`, store.Hash(token), tc.expiry, tc.quota); err != nil {
			t.Fatal(err)
		}
		w := request(h, "GET", "/sub/"+token, nil, nil)
		if w.Code != 200 {
			t.Fatal("expired subscription must refresh client nodes", w.Code)
		}
		b, err := base64.StdEncoding.DecodeString(w.Body.String())
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		if len(lines) != 1 {
			t.Fatal("expired subscription retained real nodes")
		}
		p, err := proxy.Parse(lines[0])
		if err != nil {
			t.Fatal(err)
		}
		if p.Protocol != "shadowsocks" || p.Server != "127.0.0.1" || p.Port != 65535 || p.Name != tc.label {
			t.Fatal("invalid unavailable-service notice")
		}
	}
	s.Pool.Exec(ctx, `UPDATE users SET active=false WHERE role='user'`)
	if w := request(h, "GET", "/sub/"+token, nil, nil); w.Code != 404 {
		t.Fatal("disabled account still received subscription", w.Code)
	}
}
