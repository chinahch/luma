package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExplicitGlobalExitToggleControlsEffectiveRoutes(t *testing.T) {
	for _, tc := range []struct {
		raw    string
		ids    []int64
		count  int
		target string
	}{
		{`{"global":true,"default_exit":7,"rules":[{"exit_id":8,"domain_suffixes":["example.com"]}]}`, []int64{7}, 2, "luma-exit-7"},
		{`{"global":false,"default_exit":7,"rules":[{"exit_id":8,"domain_suffixes":["example.com"]}]}`, []int64{8}, 2, "luma-exit-8"},
		{`{"global":false,"default_exit":7,"rules":[]}`, []int64{}, 0, ""},
	} {
		var r Routes
		if e := json.Unmarshal([]byte(tc.raw), &r); e != nil {
			t.Fatal(e)
		}
		ids := r.ExitIDs()
		if len(ids) != len(tc.ids) {
			t.Fatal("wrong effective exits", ids)
		}
		for i, id := range ids {
			if id != tc.ids[i] {
				t.Fatal("inactive exit used", ids)
			}
		}
		rules, e := r.Compile()
		if e != nil || len(rules) != tc.count {
			t.Fatal("global mode not applied", len(rules), e)
		}
		if tc.count > 0 && rules[len(rules)-1]["action"].(M)["target"] != tc.target {
			t.Fatal("wrong exit target")
		}
	}
	var r Routes
	json.Unmarshal([]byte(`{"global":true,"default_exit":0,"rules":[]}`), &r)
	if _, e := r.Compile(); e == nil {
		t.Fatal("global mode accepted without a landing")
	}
}

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
