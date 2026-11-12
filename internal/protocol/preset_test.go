package protocol

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNativePresetsHaveExplicitKernelAndTransport(t *testing.T) {
	for _, tc := range []struct {
		id, kernel, protocol, network string
		tls                           int
	}{
		{"socks5", "singbox", "socks", "tcp", 0}, {"vless-reality", "xray", "vless", "tcp", 2}, {"hysteria2", "singbox", "hysteria", "tcp", 1}, {"shadowsocks", "singbox", "shadowsocks", "tcp", 0}, {"ss2022", "singbox", "shadowsocks", "tcp", 0}, {"vless-enc-reality-vision", "xray", "vless", "tcp", 2}, {"vless-enc", "xray", "vless", "tcp", 0}, {"vmess-ws", "xray", "vmess", "ws", 1}, {"vless-ws", "xray", "vless", "ws", 1}, {"anytls-reality", "singbox", "anytls", "tcp", 2}, {"anytls-tls", "singbox", "anytls", "tcp", 1}, {"vless-xhttp-reality", "xray", "vless", "xhttp", 2}, {"vless-xhttp-reality-enc", "xray", "vless", "xhttp", 2}, {"vless-xhttp-tls-h3", "xray", "vless", "xhttp", 1},
	} {
		c, e := Build(Input{Preset: tc.id, Port: 18443, ServerName: "cert.example", Destination: "cert.example:443", Path: "/edge", CertFile: "/etc/caddy/cert.pem", KeyFile: "/etc/caddy/key.pem", Authentication: "x25519", Appearance: "native"})
		if e != nil {
			t.Fatal(tc.id, e)
		}
		if c["kernel_type"] != tc.kernel || c["protocol"] != tc.protocol || c["network"] != tc.network || c["tls"] != tc.tls {
			t.Fatal(tc.id, c)
		}
		if tc.id == "vless-xhttp-tls-h3" && c["networkSettings"].(M)["alpn"] != "h3" {
			t.Fatal("H3 is not enabled")
		}
	}
}
func TestAllSixENCKeypairs(t *testing.T) {
	for _, auth := range []string{"x25519", "mlkem768"} {
		for _, appearance := range []string{"native", "xorpub", "random"} {
			c, e := Build(Input{Preset: "vless-enc", Port: 443, Authentication: auth, Appearance: appearance})
			if e != nil {
				t.Fatal(e)
			}
			secret := strings.Split(c["decryption"].(string), ".")
			public := strings.Split(c["encryption"].(string), ".")
			s, _ := base64.RawURLEncoding.DecodeString(secret[len(secret)-1])
			p, _ := base64.RawURLEncoding.DecodeString(public[len(public)-1])
			wantS, wantP := 32, 32
			if auth == "mlkem768" {
				wantS, wantP = 64, 1184
			}
			if len(s) != wantS || len(p) != wantP || secret[1] != appearance || public[1] != appearance {
				t.Fatal("incorrect auth keypair", auth, appearance)
			}
		}
	}
}
func TestPresetRejectsInvalidAndConnectorAliases(t *testing.T) {
	for _, in := range []Input{{Preset: "cloudflare", Port: 443}, {Preset: "mtproxy", Port: 443}, {Preset: "vless-enc", Port: 443, Authentication: "invented"}, {Preset: "anytls-tls", Port: 443}, {Preset: "vless-reality", Port: 443, ServerName: "https://bad"}} {
		if _, e := Build(in); e == nil {
			t.Fatal("invalid preset accepted")
		}
	}
}

func TestConnectorPresetsKeepAccountingAttributable(t *testing.T) {
	cf, e := Build(Input{Preset: "cf-tunnel", Port: 18001, ServerName: "edge.example", Path: "/edge", MetricsPort: 21001, TunnelToken: "private-tunnel-token"})
	if e != nil {
		t.Fatal(e)
	}
	if cf["listen_ip"] != "127.0.0.1" || cf["tls"] != 0 || cf["network"] != "ws" {
		t.Fatal("unsafe CF origin")
	}
	mtg, e := Build(Input{Preset: "mtg", Port: 18443, ServerName: "front.example", MetricsPort: 21002, OwnerID: 5})
	if e != nil {
		t.Fatal(e)
	}
	conn := mtg["connector"].(M)
	if conn["user_id"] != int64(5) || !strings.HasPrefix(conn["secret"].(string), "ee") {
		t.Fatal("MTG must have one attributable user")
	}
	if _, e = Build(Input{Preset: "mtg", Port: 18443, ServerName: "front.example", MetricsPort: 21002}); e == nil {
		t.Fatal("shared billing accepted")
	}
}
