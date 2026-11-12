package proxy

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseLinksPreservesCredentialsIPv6AndTransport(t *testing.T) {
	cases := []struct {
		uri, protocol, host, password, network string
		port                                   int
	}{
		{"socks5://alice:p%40ss%3Aword@[2001:db8::1]:1080#exit", "socks", "2001:db8::1", "p@ss:word", "tcp", 1080},
		{"vless://00000000-0000-4000-8000-000000000001@exit.example:443?security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=abcd&type=xhttp&path=%2Fx&encryption=none#Edge", "vless", "exit.example", "", "xhttp", 443},
		{"hysteria2://pass%3Aword@exit.example:8443?sni=cert.example&obfs=salamander&obfs-password=obfs-key", "hysteria2", "exit.example", "pass:word", "tcp", 8443},
		{"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:p@ss:word")) + "@exit.example:8388#SS", "shadowsocks", "exit.example", "p@ss:word", "tcp", 8388},
	}
	for _, tc := range cases {
		p, err := Parse(tc.uri)
		if err != nil {
			t.Fatal(err)
		}
		if p.Protocol != tc.protocol || p.Server != tc.host || p.Port != tc.port || p.Password != tc.password || p.Transport.Network != tc.network {
			t.Fatalf("incorrect parsed profile: %+v", p)
		}
		if tc.protocol == "vless" && (p.TLS.RealityPublicKey != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" || p.Transport.Path != "/x") {
			t.Fatal("reality/path lost")
		}
	}
}
func TestVMessAndLegacySSFormats(t *testing.T) {
	vmess := base64.StdEncoding.EncodeToString([]byte(`{"v":"2","ps":"VMess","add":"exit.example","port":"443","id":"00000000-0000-4000-8000-000000000001","aid":"0","net":"ws","host":"edge.example","path":"/ws","tls":"tls","sni":"cert.example"}`))
	p, err := Parse("vmess://" + vmess)
	if err != nil || p.Transport.Host != "edge.example" || p.TLS.ServerName != "cert.example" {
		t.Fatal(p, err)
	}
	p, err = Parse("ss://" + base64.RawStdEncoding.EncodeToString([]byte("aes-256-gcm:p@ss@exit.example:8388")))
	if err != nil || p.Password != "p@ss" || p.Port != 8388 {
		t.Fatal(p, err)
	}
}
func TestInvalidLinksAndBatchNeverEchoPasswords(t *testing.T) {
	for _, uri := range []string{"https://example.com/sub/private-token", "socks5://u:very-secret@host:99999", "vless://id@host:443?type=unsupported", "ss://broken", "file:///etc/passwd"} {
		if _, err := Parse(uri); err == nil {
			t.Fatal("invalid link accepted")
		} else if strings.Contains(err.Error(), "very-secret") || strings.Contains(err.Error(), "private-token") {
			t.Fatal("credential leaked in error")
		}
	}
	profiles, issues, err := ParseBatch("socks5://host:1080#one\ninvalid\nsocks5://host:1080#one")
	if err != nil || len(profiles) != 1 || len(issues) != 2 || issues[0].Line != 2 {
		t.Fatal(len(profiles), issues, err)
	}
}

func TestEditedProfilesCannotBypassProtocolValidation(t *testing.T) {
	base := Profile{Name: "exit", Protocol: "socks", Server: "host", Port: 1080, Transport: Transport{Network: "tcp"}}
	for _, mutate := range []func(*Profile){func(p *Profile) { p.Protocol = "exec" }, func(p *Profile) { p.Transport.Network = "unknown" }, func(p *Profile) { p.Server = "host\ninvalid" }, func(p *Profile) { p.Name = "" }, func(p *Profile) { p.Flow = "invented" }, func(p *Profile) { p.Obfs = "salamander" }} {
		p := base
		mutate(&p)
		if e := p.Validate(); e == nil {
			t.Fatal("invalid edited profile accepted")
		}
	}
	for _, uri := range []string{"hysteria2://pass@host:443?type=ws", "anytls://pass@host:443?type=grpc", "socks5://host:1080?flow=xtls-rprx-vision", "vless://00000000-0000-4000-8000-000000000001@host:443?flow=xtls-rprx-vision&type=xhttp&security=tls"} {
		if _, e := Parse(uri); e == nil {
			t.Fatal("incompatible parameters accepted")
		}
	}
}

func TestMalformedCredentialsRejectedBeforeSaving(t *testing.T) {
	for _, uri := range []string{"ss://" + base64.RawURLEncoding.EncodeToString([]byte("invented-cipher:password")) + "@host:443", "vless://00000000-0000-4000-8000-000000000001@host:443?security=reality&pbk=invalid", "vless://00000000-0000-4000-8000-000000000001@host:443?security=tls&pcs=invalid"} {
		if _, e := Parse(uri); e == nil {
			t.Fatal("malformed cryptographic setting accepted")
		}
	}
}
