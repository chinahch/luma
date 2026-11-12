package protocol

import "testing"

func TestGoDaddyAcceptsBothNativeCredentialAlternatives(t *testing.T) {
	for _, env := range []map[string]string{{"GODADDY_API_TOKEN": "key:secret"}, {"GODADDY_API_KEY": "key", "GODADDY_API_SECRET": "secret"}} {
		cfg := M{"tls": 1, "server_name": "edge.example.com", "cert_config": M{"cert_mode": "dns", "domain": "edge.example.com", "email": "admin@example.com", "dns_provider": "godaddy", "dns_env": env}}
		if err := ValidateCertificate(cfg); err != nil {
			t.Fatal("native credential form rejected", err)
		}
	}
}
func TestCertificateRejectsConflictingEffectiveSNIs(t *testing.T) {
	for _, nested := range []string{"other.example.com", "*.example.com"} {
		cfg := M{"tls": 1, "server_name": "edge.example.com", "tls_settings": M{"server_name": nested}, "cert_config": M{"cert_mode": "file", "domain": "edge.example.com", "cert_file": "/a/cert", "key_file": "/a/key"}}
		if ValidateCertificate(cfg) == nil {
			t.Fatal("conflicting nested SNI accepted", nested)
		}
	}
}
func TestProviderCatalogueOnlyReferencesAcceptedVariableNames(t *testing.T) {
	for _, p := range DNSProviders() {
		for _, g := range p.Required {
			for _, key := range g {
				found := false
				for _, accepted := range p.Keys {
					if key == accepted {
						found = true
					}
				}
				if !found {
					t.Fatal(p.Name, "required variable is not accepted", key)
				}
			}
		}
	}
}
func TestProviderCredentialMatrixAcceptsDocumentedMinimum(t *testing.T) {
	for _, p := range DNSProviders() {
		t.Run(p.Name, func(t *testing.T) {
			env := map[string]string{}
			groups := p.Required
			if len(p.CredentialSets) > 0 {
				groups = p.CredentialSets[0]
			}
			for _, g := range groups {
				env[g[0]] = "unit-test-placeholder"
			}
			cfg := M{"tls": 1, "server_name": "edge.example.com", "cert_config": M{"cert_mode": "dns", "domain": "edge.example.com", "email": "admin@example.com", "dns_provider": p.Name, "dns_env": env}}
			if err := ValidateCertificate(cfg); err != nil {
				t.Fatal(err)
			}
		})
	}
}
