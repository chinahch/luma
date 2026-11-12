package protocol

import (
	"github.com/chinahch/luma/internal/proxy"
	"strings"
	"testing"
)

func TestNativeSubscriptionMatchesUserAndSecrets(t *testing.T) {
	for _, preset := range Catalog() {
		if preset.ID == "cf-tunnel" || preset.ID == "mtg" {
			continue
		}
		c, e := Build(Input{Preset: preset.ID, Port: 18443, ServerName: "cert.example", Path: "/edge", CertFile: "/test/cert.pem", KeyFile: "/test/key.pem"})
		if e != nil {
			t.Fatal(e)
		}
		p, e := Client("00000000-0000-4000-8000-000000000001", "Node", "2001:db8::1", c)
		if e != nil {
			t.Fatal(preset.ID, e)
		}
		link, e := p.Link()
		if e != nil {
			t.Fatal(preset.ID, e)
		}
		parsed, e := proxy.Parse(link)
		if e != nil {
			t.Fatal(preset.ID, e)
		}
		if parsed.Server != "2001:db8::1" || parsed.Port != 18443 || parsed.Protocol != p.Protocol || parsed.Password != p.Password || parsed.UUID != p.UUID || parsed.Encryption != p.Encryption || parsed.TLS.RealityPublicKey != p.TLS.RealityPublicKey {
			t.Fatal("client settings changed", preset.ID)
		}
		if secret, ok := c["decryption"].(string); ok && strings.Contains(link, secret) {
			t.Fatal("private ENC key in link")
		}
		if preset.ID == "ss2022" && !strings.Contains(parsed.Password, ":") {
			t.Fatal("SS2022 user key missing")
		}
	}
}

func TestCloudflareClientUsesEdgeTLSAnd443(t *testing.T) {
	c, e := Build(Input{Preset: "cf-tunnel", Port: 18001, ServerName: "edge.example", Path: "/ws", MetricsPort: 21001, TunnelToken: "private-tunnel-token"})
	if e != nil {
		t.Fatal(e)
	}
	p, e := Client("00000000-0000-4000-8000-000000000001", "CF", "edge.example", c)
	if e != nil || !p.TLS.Enabled || p.Port != 443 || p.TLS.ServerName != "edge.example" {
		t.Fatal("origin listener exposed in client", e)
	}
}
