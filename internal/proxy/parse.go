package proxy

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func invalid(reason string) error { return errors.New("invalid proxy link: " + reason) }
func unbase64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, e := enc.DecodeString(s); e == nil {
			return b, nil
		}
	}
	return nil, invalid("base64 encoding")
}
func Parse(raw string) (Profile, error) {
	p := Profile{Transport: Transport{Network: "tcp"}}
	raw = strings.TrimSpace(raw)
	if len(raw) > 16384 {
		return p, invalid("link exceeds 16 KiB")
	}
	if strings.HasPrefix(raw, "vmess://") {
		return parseVMess(strings.TrimPrefix(raw, "vmess://"))
	}
	if strings.HasPrefix(raw, "ss://") {
		// SIP002 userinfo and the older whole-authority Base64 form.
		s := strings.TrimPrefix(raw, "ss://")
		authority, fragment, _ := strings.Cut(s, "#")
		if !strings.Contains(authority, "@") {
			b, e := unbase64(authority)
			if e != nil {
				return p, e
			}
			at := strings.LastIndexByte(string(b), '@')
			if at < 1 {
				return p, invalid("shadowsocks authority")
			}
			cred := string(b[:at])
			method, password, ok := strings.Cut(cred, ":")
			if !ok {
				return p, invalid("shadowsocks credentials")
			}
			u := url.URL{Scheme: "ss", Host: string(b[at+1:]), User: url.UserPassword(method, password)}
			if fragment != "" {
				f, e := url.PathUnescape(fragment)
				if e != nil {
					return p, invalid("name encoding")
				}
				u.Fragment = f
			}
			raw = u.String()
		}
	}
	// Standard Base64 can contain a raw slash. Normalize credentials before
	// URI parsing so it cannot mistake that slash for the authority's end.
	if scheme, authority, ok := strings.Cut(raw, "://"); ok && (strings.EqualFold(scheme, "socks") || strings.EqualFold(scheme, "socks5")) {
		if at := strings.IndexByte(authority, '@'); at > 0 && !strings.ContainsAny(authority[:at], ":%") {
			if decoded, err := unbase64(authority[:at]); err == nil {
				if user, pass, ok := strings.Cut(string(decoded), ":"); ok {
					if !utf8.Valid(decoded) {
						return p, invalid("SOCKS credentials encoding")
					}
					raw = scheme + "://" + url.UserPassword(user, pass).String() + authority[at:]
				}
			}
		}
	}
	u, e := url.Parse(raw)
	if e != nil {
		return p, invalid("URI syntax")
	}
	if u.Opaque != "" || u.Host == "" || u.RawPath != "" || u.Path != "" && u.Path != "/" {
		return p, invalid("expected proxy authority without subscription path")
	}
	p.Name = u.Fragment
	p.Server = u.Hostname()
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 || p.Server == "" || strings.ContainsAny(p.Server, " \r\n\t") {
		return p, invalid("server or port")
	}
	p.Port = port
	if strings.Contains(p.Server, ":") && net.ParseIP(p.Server) == nil {
		return p, invalid("IPv6 address")
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return p, invalid("query encoding")
	}
	for _, v := range q {
		if len(v) != 1 {
			return p, invalid("duplicate parameter")
		}
	}
	username, password := "", ""
	hasPassword := false
	if u.User != nil {
		username = u.User.Username()
		password, hasPassword = u.User.Password()
	}
	switch strings.ToLower(u.Scheme) {
	case "socks", "socks5":
		p.Protocol = "socks"
		// Some clients encode the complete user:password pair in userinfo.
		// An explicit colon always denotes literal URI credentials, even if empty.
		if !hasPassword {
			if decoded, err := unbase64(username); err == nil {
				if user, pass, ok := strings.Cut(string(decoded), ":"); ok {
					if !utf8.Valid(decoded) {
						return p, invalid("SOCKS credentials encoding")
					}
					username, password = user, pass
				}
			}
		}
		p.Username = username
		p.Password = password
	case "http", "https":
		p.Protocol = "http"
		p.Username = username
		p.Password = password
		p.TLS.Enabled = u.Scheme == "https"
	case "vless":
		p.Protocol = "vless"
		p.UUID = username
		p.Encryption = q.Get("encryption")
		if p.Encryption == "" {
			p.Encryption = "none"
		}
		p.Flow = q.Get("flow")
	case "trojan", "anytls", "hysteria2", "hy2":
		p.Protocol = u.Scheme
		if p.Protocol == "hy2" {
			p.Protocol = "hysteria2"
		}
		p.Password = username
		if password != "" {
			p.Password = username + ":" + password
		}
		p.TLS.Enabled = true
	case "ss":
		p.Protocol = "shadowsocks"
		if password == "" {
			b, e := unbase64(username)
			if e != nil {
				return p, e
			}
			var ok bool
			username, password, ok = strings.Cut(string(b), ":")
			if !ok {
				return p, invalid("shadowsocks credentials")
			}
		}
		p.Method = username
		p.Password = password
	default:
		return p, invalid("unsupported scheme")
	}
	if q.Get("plugin") != "" {
		return p, invalid("shadowsocks plugins are unsupported")
	}
	if p.Protocol != "vless" && (q.Get("flow") != "" || q.Get("encryption") != "") {
		return p, invalid("VLESS-only parameter")
	}
	if q.Get("pbk") != "" && q.Get("security") != "reality" {
		return p, invalid("REALITY key without REALITY security")
	}
	known := map[string]bool{}
	for _, k := range strings.Fields("type security sni peer alpn allowInsecure insecure pbk sid fp spx headerType path host serviceName mode encryption flow obfs obfs-password pcs vcn") {
		known[k] = true
	}
	for k := range q {
		if !known[k] {
			return p, invalid("unsupported parameter")
		}
	}
	if n := q.Get("type"); n != "" {
		p.Transport.Network = n
	}
	switch p.Transport.Network {
	case "raw":
		p.Transport.Network = "tcp"
	case "splithttp":
		p.Transport.Network = "xhttp"
	case "tcp", "ws", "grpc", "xhttp":
	default:
		return p, invalid("unsupported transport")
	}
	if header := q.Get("headerType"); header != "" && (header != "none" || p.Protocol != "vless" || p.Transport.Network != "tcp") {
		return p, invalid("unsupported header type")
	}
	p.Transport.Path = q.Get("path")
	p.Transport.Host = q.Get("host")
	p.Transport.ServiceName = q.Get("serviceName")
	p.Transport.Mode = q.Get("mode")
	switch q.Get("security") {
	case "", "none":
	case "tls":
		p.TLS.Enabled = true
	case "reality":
		p.TLS.Enabled = true
		p.TLS.RealityPublicKey = q.Get("pbk")
		if p.TLS.RealityPublicKey == "" {
			return p, invalid("REALITY public key required")
		}
	default:
		return p, invalid("unsupported security")
	}
	p.TLS.ServerName = q.Get("sni")
	if p.TLS.ServerName == "" {
		p.TLS.ServerName = q.Get("peer")
	}
	p.TLS.ShortID = q.Get("sid")
	p.TLS.RealitySpiderX = q.Get("spx")
	p.TLS.Fingerprint = q.Get("fp")
	p.TLS.PinnedPeerCertSHA256 = q.Get("pcs")
	p.TLS.VerifyPeerCertByName = q.Get("vcn")
	if a := q.Get("alpn"); a != "" {
		p.TLS.ALPN = strings.Split(a, ",")
	}
	for _, k := range []string{"insecure", "allowInsecure"} {
		if v := q.Get(k); v != "" {
			if v != "0" && v != "1" && v != "true" && v != "false" {
				return p, invalid("TLS verification flag")
			}
			p.TLS.Insecure = v == "1" || v == "true"
		}
	}
	p.Obfs = q.Get("obfs")
	p.ObfsPassword = q.Get("obfs-password")
	if p.Name == "" {
		p.Name = p.Server
	}
	if e := p.Validate(); e != nil {
		return Profile{}, e
	}
	return p, nil
}

func (p Profile) Validate() error {
	if p.Port < 1 || p.Port > 65535 || p.Server == "" || len(p.Server) > 253 || strings.ContainsAny(p.Server, " /@?#\t\r\n") || strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 {
		return invalid("server, port or name")
	}
	switch p.Protocol {
	case "socks", "http", "vless", "vmess", "trojan", "shadowsocks", "hysteria2", "anytls":
	default:
		return invalid("unsupported protocol")
	}
	switch p.Transport.Network {
	case "tcp", "ws", "grpc", "xhttp":
	default:
		return invalid("unsupported transport")
	}
	if p.Transport.Network != "tcp" && p.Protocol != "vless" && p.Protocol != "vmess" && p.Protocol != "trojan" {
		return invalid("transport is incompatible with protocol")
	}
	if p.Flow != "" && (p.Protocol != "vless" || p.Flow != "xtls-rprx-vision") {
		return invalid("unsupported flow")
	}
	if p.Flow != "" && (p.Encryption == "" || p.Encryption == "none") && (p.Transport.Network != "tcp" || !p.TLS.Enabled) {
		return invalid("Vision requires TCP TLS/REALITY or VLESS native encryption")
	}
	if p.Obfs != "" && (p.Protocol != "hysteria2" || p.ObfsPassword == "") {
		return invalid("obfuscation requires Hysteria2 and password")
	}
	if p.TLS.RealityPublicKey != "" && !p.TLS.Enabled {
		return invalid("REALITY requires TLS")
	}
	if p.TLS.RealityPublicKey != "" {
		raw, e := base64.RawURLEncoding.DecodeString(p.TLS.RealityPublicKey)
		if e != nil || len(raw) != 32 {
			return invalid("REALITY public key")
		}
		short, e := hex.DecodeString(p.TLS.ShortID)
		if e != nil || len(short) > 8 {
			return invalid("REALITY short ID")
		}
	}
	if path := p.TLS.RealitySpiderX; path != "" {
		if p.TLS.RealityPublicKey == "" || !strings.HasPrefix(path, "/") || len(path) > 4096 || strings.ContainsAny(path, "\r\n\x00") {
			return invalid("REALITY spider path")
		}
		if _, err := url.Parse(path); err != nil {
			return invalid("REALITY spider path")
		}
	}
	if p.TLS.VerifyPeerCertByName != "" && (!p.TLS.Enabled || p.TLS.RealityPublicKey != "" || len(p.TLS.VerifyPeerCertByName) > 1024) {
		return invalid("certificate verification names require ordinary TLS")
	}
	if p.TLS.PinnedPeerCertSHA256 != "" {
		if !p.TLS.Enabled || p.TLS.RealityPublicKey != "" {
			return invalid("certificate pins require ordinary TLS")
		}
		for _, pin := range strings.Split(p.TLS.PinnedPeerCertSHA256, ",") {
			raw, e := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(pin), ":", ""))
			if e != nil || len(raw) != 32 {
				return invalid("certificate SHA256 pin")
			}
		}
	}
	if p.Protocol == "trojan" || p.Protocol == "anytls" || p.Protocol == "hysteria2" {
		if !p.TLS.Enabled {
			return invalid("protocol requires TLS")
		}
	}
	if len(p.Password)+len(p.UUID)+len(p.Encryption)+len(p.ObfsPassword)+len(p.Transport.Path) > 16384 {
		return invalid("profile exceeds size limit")
	}
	if p.AlterID < 0 || p.AlterID > 65535 {
		return invalid("VMess alter ID")
	}
	if p.Protocol == "vless" || p.Protocol == "vmess" {
		if !validUUID(p.UUID) {
			return invalid("UUID")
		}
	}
	switch p.Protocol {
	case "shadowsocks":
		if p.Method == "" || p.Password == "" {
			return invalid("shadowsocks credentials")
		}
		methods := map[string]int{"aes-128-gcm": 0, "aes-256-gcm": 0, "chacha20-ietf-poly1305": 0, "2022-blake3-aes-128-gcm": 16, "2022-blake3-aes-256-gcm": 32, "2022-blake3-chacha20-poly1305": 32}
		size, ok := methods[p.Method]
		if !ok {
			return invalid("unsupported Shadowsocks cipher")
		}
		if size > 0 {
			for _, key := range strings.Split(p.Password, ":") {
				raw, e := base64.StdEncoding.DecodeString(key)
				if e != nil || len(raw) != size {
					return invalid("SS2022 key length or encoding")
				}
			}
		}
	case "trojan", "hysteria2", "anytls":
		if p.Password == "" {
			return invalid("password required")
		}
	}
	if p.Obfs != "" && p.Obfs != "salamander" {
		return invalid("unsupported obfuscation")
	}
	return nil
}
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func parseVMess(encoded string) (Profile, error) {
	p := Profile{Protocol: "vmess", Transport: Transport{Network: "tcp"}, Encryption: "auto"}
	b, e := unbase64(encoded)
	if e != nil {
		return p, e
	}
	var v map[string]json.RawMessage
	if json.Unmarshal(b, &v) != nil {
		return p, invalid("VMess JSON")
	}
	get := func(k string) string {
		var s string
		if json.Unmarshal(v[k], &s) == nil {
			return s
		}
		return string(v[k])
	}
	p.Name = get("ps")
	p.Server = get("add")
	p.Port, _ = strconv.Atoi(get("port"))
	p.UUID = get("id")
	p.AlterID, _ = strconv.Atoi(get("aid"))
	p.Transport.Network = get("net")
	if p.Transport.Network == "" || p.Transport.Network == "raw" {
		p.Transport.Network = "tcp"
	}
	switch p.Transport.Network {
	case "tcp", "ws", "grpc":
	default:
		return p, invalid("unsupported VMess transport")
	}
	if t := get("type"); t != "" && t != "none" {
		return p, invalid("unsupported VMess header")
	}
	p.Transport.Host = get("host")
	p.Transport.Path = get("path")
	if p.Transport.Network == "grpc" {
		p.Transport.ServiceName = p.Transport.Path
	}
	p.TLS.Enabled = get("tls") == "tls"
	p.TLS.ServerName = get("sni")
	if a := get("alpn"); a != "" {
		p.TLS.ALPN = strings.Split(a, ",")
	}
	p.TLS.Fingerprint = get("fp")
	if s := get("scy"); s != "" {
		p.Encryption = s
	}
	if p.Name == "" {
		p.Name = p.Server
	}
	return p, p.Validate()
}

type Issue struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func ParseBatch(text string) ([]Profile, []Issue, error) {
	if len(text) > 1<<20 {
		return nil, nil, invalid("batch exceeds 1 MiB")
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > 200 {
		return nil, nil, invalid("batch exceeds 200 lines")
	}
	profiles := []Profile{}
	issues := []Issue{}
	seen := map[[32]byte]bool{}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p, e := Parse(line)
		if e != nil {
			issues = append(issues, Issue{i + 1, e.Error()})
			continue
		}
		canonical := p
		canonical.Name = ""
		b, _ := json.Marshal(canonical)
		key := sha256.Sum256(b)
		if seen[key] {
			issues = append(issues, Issue{i + 1, "duplicate proxy"})
			continue
		}
		seen[key] = true
		profiles = append(profiles, p)
	}
	return profiles, issues, nil
}
