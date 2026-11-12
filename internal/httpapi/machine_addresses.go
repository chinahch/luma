package httpapi

import (
	"encoding/json"
	"errors"
	"net/netip"
	"time"
)

// Addresses are never inferred from the HTTP peer: that may be a WARP exit.
func physicalAddresses(raw json.RawMessage) (json.RawMessage, error) {
	var v struct {
		Source string   `json:"source"`
		IPv4   []string `json:"ipv4"`
		IPv6   []string `json:"ipv6"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Source != "physical_interface" || len(v.IPv4)+len(v.IPv6) > 32 {
		return nil, errors.New("invalid physical public address report")
	}
	out := struct {
		Source   string    `json:"source"`
		IPv4     []string  `json:"ipv4"`
		IPv6     []string  `json:"ipv6"`
		Detected time.Time `json:"detected_at"`
	}{Source: v.Source, IPv4: []string{}, IPv6: []string{}, Detected: time.Now().UTC()}
	seen := map[netip.Addr]bool{}
	for _, value := range append(v.IPv4, v.IPv6...) {
		ip, e := netip.ParseAddr(value)
		if e != nil || ip.Zone() != "" {
			continue
		}
		ip = ip.Unmap()
		if seen[ip] || !ip.IsGlobalUnicast() || ip.IsPrivate() || reservedPublicIP(ip) {
			continue
		}
		seen[ip] = true
		if ip.Is4() {
			out.IPv4 = append(out.IPv4, ip.String())
		} else {
			out.IPv6 = append(out.IPv6, ip.String())
		}
	}
	return json.Marshal(out)
}

var publicAddressExclusions = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func reservedPublicIP(ip netip.Addr) bool {
	for _, p := range publicAddressExclusions {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
