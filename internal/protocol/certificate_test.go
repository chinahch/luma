package protocol

import (
	"testing"
)

func TestDNSCertificatePresetAndValidation(t *testing.T) {
	in := Input{Preset: "hysteria2", Port: 443, ServerName: "edge.example.com", Certificate: &CertificateConfig{CertMode: "dns", Domain: "*.example.com", Email: "admin@example.com", DNSProvider: "cf", DNSEnv: map[string]string{"CF_API_TOKEN": "test=value=="}}}
	cfg, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	cc := cfg["cert_config"].(M)
	if cc["cert_mode"] != "dns" || cc["dns_provider"] != "cloudflare" || cc["domain"] != "*.example.com" {
		t.Fatal(cc)
	}
	if err = ValidateCertificate(cfg); err != nil {
		t.Fatal(err)
	}
	cc["domain"] = "*.other.example.com"
	if ValidateCertificate(cfg) == nil {
		t.Fatal("uncovered SNI accepted")
	}
	cc["domain"] = "*.example.com"
	cc["cert_mode"] = "http"
	if ValidateCertificate(cfg) == nil {
		t.Fatal("HTTP wildcard accepted")
	}
}

func TestCertificateRejectsMalformedAndCrossProviderCredentials(t *testing.T) {
	for _, tc := range []struct {
		provider string
		env      map[string]string
	}{
		{"cloudflare", map[string]string{"CF_API_TOKEN": ""}},
		{"alidns", map[string]string{"CF_API_TOKEN": "secret"}},
		{"cloudflare", map[string]string{"CF_API_TOKEN": "secret", "PATH": "/bin"}},
		{"made-up", map[string]string{"TOKEN": "secret"}},
	} {
		cfg := M{"tls": 1, "server_name": "edge.example.com", "cert_config": M{"cert_mode": "dns", "domain": "edge.example.com", "email": "admin@example.com", "dns_provider": tc.provider, "dns_env": tc.env}}
		if ValidateCertificate(cfg) == nil {
			t.Fatal("invalid credential accepted", tc.provider)
		}
	}
}

func TestCertificatePEMMatchesSNIDomain(t *testing.T) {
	cfg := M{"tls": 1, "server_name": "edge.example.com", "cert_config": M{"cert_mode": "content", "domain": "edge.example.com", "cert_content": "not PEM", "key_content": "not PEM"}}
	if ValidateCertificate(cfg) == nil {
		t.Fatal("invalid PEM accepted")
	}
}

func TestDNSCredentialLinesAreData(t *testing.T) {
	env, err := ParseDNSEnv("CF_API_TOKEN=test==\r\n\n")
	if err != nil || env["CF_API_TOKEN"] != "test==" {
		t.Fatal(env, err)
	}
	for _, s := range []string{"CF_API_TOKEN=x\nCF_API_TOKEN=y", "export CF_API_TOKEN=x", "missing", "bad-name=x", "CF_API_TOKEN="} {
		if _, err = ParseDNSEnv(s); err == nil {
			t.Fatal("invalid lines accepted")
		}
	}
	env, err = ParseDNSEnv("CF_API_TOKEN=$(touch never-execute)")
	if err != nil || env["CF_API_TOKEN"] != "$(touch never-execute)" {
		t.Fatal("credential is data", err)
	}
}
