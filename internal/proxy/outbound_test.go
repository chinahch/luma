package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOutboundPreservesRealityAndTransport(t *testing.T) {
	p, e := Parse("vless://00000000-0000-4000-8000-000000000001@exit.example:443?security=reality&sni=cert.example&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=abcd&fp=chrome&type=xhttp&path=%2Fedge")
	if e != nil {
		t.Fatal(e)
	}
	o, e := p.Outbound("xray", "landing")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(o)
	s := string(b)
	for _, v := range []string{`"stream_settings"`, `"realitySettings"`, `"serverName":"cert.example"`, `"xhttpSettings"`, `"path":"/edge"`, `"vnext"`} {
		if !strings.Contains(s, v) {
			t.Fatalf("missing %s in %s", v, s)
		}
	}
	if _, e = p.Outbound("singbox", "landing"); e == nil {
		t.Fatal("sing-box cannot silently discard XHTTP")
	}
}
func TestOutboundKernelDialectAndUnsupported(t *testing.T) {
	for _, tc := range []struct{ uri, kernel, want string }{
		{"socks5://u:p@host:1080", "xray", `"servers":[{"address":"host","port":1080,"users":[{"pass":"p","user":"u"}]}]`},
		{"socks5://u:p@host:1080", "singbox", `"server_port":1080`},
		{"hysteria2://secret@host:443?obfs=salamander&obfs-password=key", "singbox", `"obfs":{"password":"key","type":"salamander"}`},
		{"anytls://secret@host:443?sni=cert.example", "singbox", `"tls":{"enabled":true,"server_name":"cert.example"}`},
	} {
		p, e := Parse(tc.uri)
		if e != nil {
			t.Fatal(e)
		}
		o, e := p.Outbound(tc.kernel, "exit-1")
		if e != nil {
			t.Fatal(e)
		}
		b, _ := json.Marshal(o)
		if !strings.Contains(string(b), tc.want) {
			t.Fatal(string(b))
		}
	}
	p, _ := Parse("anytls://secret@host:443")
	if _, e := p.Outbound("xray", "exit"); e == nil {
		t.Fatal("AnyTLS unsupported in Xray")
	}
	p, _ = Parse("vless://00000000-0000-4000-8000-000000000001@host:443?encryption=mlkem768x25519plus.native.0rtt.key")
	if _, e := p.Outbound("singbox", "exit"); e == nil {
		t.Fatal("native encryption unsupported in sing-box")
	}
}

func TestXrayUsesCertificatePinningAndRejectsRemovedInsecure(t *testing.T) {
	p, _ := Parse("vless://00000000-0000-4000-8000-000000000001@host:443?security=tls&allowInsecure=1")
	if _, e := p.Outbound("xray", "exit"); e == nil {
		t.Fatal("removed Xray allowInsecure accepted")
	}
	pin := strings.Repeat("01", 32)
	p, e := Parse("vless://00000000-0000-4000-8000-000000000001@host:443?security=tls&pcs=" + pin + "&vcn=cert.example")
	if e != nil {
		t.Fatal(e)
	}
	o, e := p.Outbound("xray", "exit")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(o)
	if !strings.Contains(string(b), `"pinnedPeerCertSha256":"`+pin+`"`) {
		t.Fatal("pin lost")
	}
	if _, e = p.Outbound("singbox", "exit"); e == nil {
		t.Fatal("Xray certificate pin silently discarded")
	}
}
