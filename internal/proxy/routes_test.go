package proxy

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestLandingGuardUsesKernelCompatibleCIDRs(t *testing.T) {
	on, off := true, false
	for _, r := range []Routes{
		{DefaultExit: 7},
		{Global: &on, DefaultExit: 7},
		{Global: &off, Rules: []Route{{ExitID: 7, DomainSuffixes: []string{"example.com"}}}},
	} {
		rules, err := r.Compile()
		if err != nil {
			t.Fatal(err)
		}
		var networks []*net.IPNet
		for _, cidr := range rules[0]["match"].(M)["ip_cidrs"].([]string) {
			ip, network, err := net.ParseCIDR(cidr)
			if err != nil {
				t.Fatal(err)
			}
			// Xray normalizes mapped IPv6 to IPv4 before checking the mask.
			// A mapped /96 is valid CIDR syntax but is rejected by its router.
			bits, _ := network.Mask.Size()
			if ip.To4() != nil && bits > 32 {
				t.Fatalf("Xray rejects mapped IPv4 CIDR %s after normalization", cidr)
			}
			networks = append(networks, network)
		}
		for _, tc := range []struct {
			address string
			blocked bool
		}{
			{"10.1.2.3", true}, {"127.0.0.1", true}, {"192.168.1.1", true},
			{"::ffff:10.1.2.3", true}, {"::ffff:127.0.0.1", true}, {"::ffff:192.168.1.1", true},
			{"::1", true}, {"fc00::1", true}, {"fe80::1", true},
			{"1.1.1.1", false}, {"::ffff:1.1.1.1", false}, {"2606:4700:4700::1111", false},
		} {
			blocked := false
			for _, network := range networks {
				blocked = blocked || network.Contains(net.ParseIP(tc.address))
			}
			if blocked != tc.blocked {
				t.Fatalf("guard for %s: blocked=%t, want %t", tc.address, blocked, tc.blocked)
			}
		}
	}
}

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
