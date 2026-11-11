package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoutesProtectPrivateAddressesBeforeCatchAll(t *testing.T) {
	r := Routes{DefaultExit: 7, Rules: []Route{{ExitID: 8, DomainSuffixes: []string{"example.com"}}}}
	rules, e := r.Compile()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(rules)
	s := string(b)
	if !strings.Contains(s, `"ip_cidrs"`) || !strings.Contains(s, `"target":"luma-exit-7"`) {
		t.Fatal(s)
	}
	if !strings.Contains(s, "192.0.0.0/24") || !strings.Contains(s, "198.18.0.0/15") {
		t.Fatal("catch-all bypasses inherited private/reserved blocks")
	}
	if rules[0]["action"].(M)["type"] != "block" || rules[len(rules)-1]["action"].(M)["target"] != "luma-exit-7" {
		t.Fatal("unsafe priority")
	}
	for _, r := range []Routes{{Rules: []Route{{ExitID: 1}}}, {Rules: []Route{{ExitID: 1, IPCIDRs: []string{"not-a-network"}}}}, {Rules: []Route{{ExitID: 1, DomainSuffixes: []string{"https://example.com"}}}}} {
		if _, e := r.Compile(); e == nil {
			t.Fatal("invalid matcher accepted")
		}
	}
}
