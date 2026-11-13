package proxy

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestSOCKSBase64CredentialsReachBothKernels(t *testing.T) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		for _, scheme := range []string{"socks", "socks5"} {
			u := url.URL{Scheme: scheme, Host: "exit.example:52000", User: url.User(enc.EncodeToString([]byte("alice:pass:@+=中文"))), Fragment: "Landing"}
			p, err := Parse(u.String())
			if err != nil {
				t.Fatal(err)
			}
			if p.Username != "alice" || p.Password != "pass:@+=中文" {
				t.Fatal("encoded SOCKS credentials were not decoded")
			}
			for _, kernel := range []string{"xray", "singbox"} {
				o, err := p.Outbound(kernel, "landing")
				if err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(o)
				if !strings.Contains(string(b), `"alice"`) || !strings.Contains(string(b), `"pass:@+=中文"`) {
					t.Fatal("credentials lost in outbound")
				}
			}
			link, err := p.Link()
			if err != nil {
				t.Fatal(err)
			}
			again, err := Parse(link)
			if err != nil || again.Username != "alice" || again.Password != "pass:@+=中文" {
				t.Fatal("credentials lost in link round trip", err)
			}
		}
	}
}

func TestSOCKSUnescapedStandardBase64Slash(t *testing.T) {
	for _, link := range []string{"socks://dTo/@exit.example:1080#Raw", "SOCKS5://dTo/@[2001:db8::1]:1080#Raw"} {
		p, err := Parse(link)
		if err != nil || p.Username != "u" || p.Password != "?" {
			t.Fatal("raw standard Base64 slash was treated as URI path", err)
		}
	}
}

func TestSOCKSPlainCredentialsRemainLiteral(t *testing.T) {
	for _, tc := range []struct{ userinfo, username, password string }{
		{"YWxpY2U6cGFzcw:", "YWxpY2U6cGFzcw", ""},
		{"YWxpY2U", "YWxpY2U", ""},
		{"alice:p%40ss%3Aword", "alice", "p@ss:word"},
		{"", "", ""},
	} {
		authority := tc.userinfo
		if authority != "" {
			authority += "@"
		}
		p, err := Parse("socks5://" + authority + "exit.example:1080")
		if err != nil || p.Username != tc.username || p.Password != tc.password {
			t.Fatal("literal credentials changed", err)
		}
	}
}

const encRealityLink = "vless://00000000-0000-4000-8000-000000000001@exit.example:20200?encryption=mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&flow=xtls-rprx-vision&security=reality&sni=addons.mozilla.org&fp=chrome&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=7d460428&spx=%2Fprobe%3Fx%3D1&type=tcp&headerType=none#Landing"

func TestVLESSENCRealityShareOptionsReachXray(t *testing.T) {
	p, err := Parse(encRealityLink)
	if err != nil {
		t.Fatal(err)
	}
	o, err := p.Outbound("xray", "landing")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(o)
	for _, want := range []string{`"spiderX":"/probe?x=1"`, `"encryption":"mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"`, `"flow":"xtls-rprx-vision"`, `"serverName":"addons.mozilla.org"`, `"shortId":"7d460428"`, `"security":"reality"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("outbound missing %s", want)
		}
	}
	link, err := p.Link()
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(again)
	want, _ := json.Marshal(p)
	if string(got) != string(want) {
		t.Fatal("VLESS parameters lost in round trip")
	}
}

func TestUnsupportedShareOptionsStillFailClosed(t *testing.T) {
	base := "vless://00000000-0000-4000-8000-000000000001@exit.example:443?type=tcp"
	for _, suffix := range []string{"&headerType=http", "&headerType=secret", "&spx=%2F", "&unknown=secret", "&security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&spx=bad", "&security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&spx=%2F%25zz"} {
		if _, err := Parse(base + suffix); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("unsupported option accepted or reflected")
		}
	}
}

func TestSingboxDoesNotDiscardRealitySpiderPath(t *testing.T) {
	p, err := Parse("vless://00000000-0000-4000-8000-000000000001@exit.example:443?security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&spx=%2Fprobe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Outbound("singbox", "landing"); err == nil {
		t.Fatal("unsupported spider path silently discarded")
	}
}
