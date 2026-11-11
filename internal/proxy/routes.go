package proxy

import (
	"fmt"
	"net"
	"strings"
)

type Routes struct {
	DefaultExit int64   `json:"default_exit"`
	Rules       []Route `json:"rules"`
}
type Route struct {
	ExitID         int64    `json:"exit_id"`
	DomainSuffixes []string `json:"domain_suffixes,omitempty"`
	IPCIDRs        []string `json:"ip_cidrs,omitempty"`
}

func ExitTag(id int64) string { return fmt.Sprintf("luma-exit-%d", id) }
func (r Routes) ExitIDs() []int64 {
	ids := []int64{}
	seen := map[int64]bool{}
	add := func(id int64) {
		if id > 0 && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	for _, v := range r.Rules {
		add(v.ExitID)
	}
	add(r.DefaultExit)
	return ids
}
func (r Routes) Compile() ([]M, error) {
	if r.DefaultExit < 0 || len(r.Rules) > 100 {
		return nil, invalid("invalid exit or too many routes")
	}
	rules := []M{}
	if len(r.ExitIDs()) == 0 && len(r.Rules) == 0 {
		return rules, nil
	}
	// An explicit first rule is necessary: upstream structured rules precede
	// its implicit private-network blocks. A catch-all must never bypass them.
	rules = append(rules, M{"name": "Luma private-network guard", "match": M{"ip_cidrs": []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.168.0.0/16", "192.0.0.0/24", "198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4", "::/128", "::1/128", "::ffff:0:0/96", "fc00::/7", "fe80::/10", "ff00::/8"}}, "action": M{"type": "block"}})
	for _, v := range r.Rules {
		if v.ExitID <= 0 || len(v.DomainSuffixes)+len(v.IPCIDRs) == 0 || len(v.DomainSuffixes)+len(v.IPCIDRs) > 100 {
			return nil, invalid("route requires an exit and domain suffix or CIDR")
		}
		for _, d := range v.DomainSuffixes {
			if len(d) == 0 || len(d) > 253 || strings.ContainsAny(d, " /:@?#*\t\r\n") || strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") {
				return nil, invalid("domain suffix")
			}
		}
		for _, cidr := range v.IPCIDRs {
			if _, _, e := net.ParseCIDR(cidr); e != nil {
				return nil, invalid("CIDR")
			}
		}
		// Split domain and IP lists into separate rules so their meaning is OR
		// on both kernels rather than an impossible domain AND IP match.
		for _, match := range []M{{"domain_suffixes": v.DomainSuffixes}, {"ip_cidrs": v.IPCIDRs}} {
			has := false
			for _, a := range match {
				has = len(a.([]string)) > 0
			}
			if has {
				rules = append(rules, M{"match": match, "action": M{"type": "route", "target": ExitTag(v.ExitID)}})
			}
		}
	}
	if r.DefaultExit > 0 {
		rules = append(rules, M{"name": "Luma default exit", "match": M{"networks": []string{"tcp", "udp"}}, "action": M{"type": "route", "target": ExitTag(r.DefaultExit)}})
	}
	return rules, nil
}
