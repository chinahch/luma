package protocol

import (
	"encoding/json"
	"os"
	"testing"
)

// Opt-in cross-repository fixtures contain newly generated test-only keys.
// Never point this at a deployed node configuration.
func TestExportPinnedCoreFixtures(t *testing.T) {
	path := os.Getenv("LUMA_PROTOCOL_FIXTURES")
	if path == "" {
		t.Skip("cross-repository fixture export not requested")
	}
	fixtures := []M{}
	for _, p := range Catalog() {
		if p.ID == "cf-tunnel" || p.ID == "mtg" {
			continue
		}
		auths, appearances := []string{"x25519"}, []string{"native"}
		if p.ENC {
			auths = []string{"x25519", "mlkem768"}
			appearances = []string{"native", "xorpub", "random"}
		}
		for _, a := range auths {
			for _, look := range appearances {
				c, e := Build(Input{Preset: p.ID, Port: 18443, ServerName: "cert.example", Path: "/edge", CertFile: "/test/cert.pem", KeyFile: "/test/key.pem", Authentication: a, Appearance: look})
				if e != nil {
					t.Fatal(e)
				}
				c["fixture_name"] = p.ID + "-" + a + "-" + look
				client, e := Client("00000000-0000-4000-8000-000000000001", "test", "node.example", c)
				if e != nil {
					t.Fatal(e)
				}
				out, e := client.Outbound(p.Kernel, "landing")
				if e != nil {
					t.Fatal(e)
				}
				c["fixture_outbound"] = out
				fixtures = append(fixtures, c)
			}
		}
	}
	data, e := json.MarshalIndent(fixtures, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
}
