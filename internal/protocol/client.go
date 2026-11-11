package protocol

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/chinahch/luma/internal/proxy"
	"strings"
)

func number(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
func text(m map[string]any, key string) string { v, _ := m[key].(string); return v }

// Client projects a node config to the authenticated user's client parameters.
// It never exports inbound private keys, certificates or ENC decryption keys.
func Client(uuid, name, host string, c map[string]any) (proxy.Profile, error) {
	p := proxy.Profile{Name: name, Protocol: text(c, "protocol"), Server: host, Port: number(c["server_port"]), Transport: proxy.Transport{Network: text(c, "network")}}
	if p.Transport.Network == "" {
		p.Transport.Network = "tcp"
	}
	p.TLS.Enabled = number(c["tls"]) > 0
	p.TLS.ServerName = text(c, "server_name")
	if p.TLS.ServerName == "" {
		p.TLS.ServerName = text(c, "host")
	}
	ts, _ := c["tls_settings"].(map[string]any)
	if s := text(ts, "server_name"); s != "" {
		p.TLS.ServerName = s
	}
	if p.TLS.Enabled && p.TLS.ServerName == "" {
		p.TLS.ServerName = host
	}
	if connector, ok := c["connector"].(map[string]any); ok && text(connector, "kind") == "cloudflared" {
		p.TLS.Enabled = true
		p.TLS.ServerName = host
		p.Port = 443
	}
	if number(c["tls"]) == 2 {
		p.TLS.RealityPublicKey = text(ts, "public_key")
		if p.TLS.RealityPublicKey == "" {
			raw, e := base64.RawURLEncoding.DecodeString(text(ts, "private_key"))
			if e != nil {
				return p, errors.New("invalid REALITY private key")
			}
			key, e := ecdh.X25519().NewPrivateKey(raw)
			if e != nil {
				return p, errors.New("invalid REALITY private key")
			}
			p.TLS.RealityPublicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
		}
		p.TLS.Fingerprint = "chrome"
		switch v := ts["short_id"].(type) {
		case string:
			p.TLS.ShortID = v
		case []any:
			if len(v) > 0 {
				p.TLS.ShortID, _ = v[0].(string)
			}
		}
	}
	ns, _ := c["networkSettings"].(map[string]any)
	p.Transport.Path = text(ns, "path")
	p.Transport.Host = text(ns, "host")
	p.Transport.Mode = text(ns, "mode")
	p.Transport.ServiceName = text(ns, "serviceName")
	if p.Transport.ServiceName == "" {
		p.Transport.ServiceName = text(ns, "service_name")
	}
	if p.Transport.Host == "" {
		headers, _ := ns["headers"].(map[string]any)
		p.Transport.Host = text(headers, "Host")
	}
	if a := text(ns, "alpn"); a != "" {
		p.TLS.ALPN = strings.Split(a, ",")
	} else {
		switch a := ts["alpn"].(type) {
		case []any:
			for _, v := range a {
				if s, ok := v.(string); ok {
					p.TLS.ALPN = append(p.TLS.ALPN, s)
				}
			}
		case []string:
			p.TLS.ALPN = a
		}
	}
	switch p.Protocol {
	case "socks", "http":
		p.Username = uuid
		p.Password = uuid
	case "vless", "vmess":
		p.UUID = uuid
		p.Flow = text(c, "flow")
		p.Encryption = text(c, "encryption")
		if p.Encryption == "" {
			p.Encryption = "none"
			if p.Protocol == "vmess" {
				p.Encryption = "auto"
			}
		}
	case "shadowsocks":
		p.Method = text(c, "cipher")
		p.Password = uuid
		if strings.HasPrefix(p.Method, "2022-") {
			size := 32
			if p.Method == "2022-blake3-aes-128-gcm" {
				size = 16
			}
			raw := make([]byte, size)
			copy(raw, uuid)
			serverKey := text(c, "server_key")
			if serverKey == "" {
				return p, errors.New("SS2022 server key required")
			}
			p.Password = serverKey + ":" + base64.StdEncoding.EncodeToString(raw)
		}
	case "hysteria":
		if number(c["version"]) != 2 {
			return p, errors.New("only Hysteria2 client export is supported")
		}
		p.Protocol = "hysteria2"
		p.Password = uuid
		p.Obfs = text(c, "obfs")
		p.ObfsPassword = text(c, "obfs_password")
		if len(p.TLS.ALPN) == 0 {
			p.TLS.ALPN = []string{"h3"}
		}
	case "anytls", "trojan":
		p.Password = uuid
	default:
		return p, fmt.Errorf("unsupported subscription protocol")
	}
	return p, p.Validate()
}
