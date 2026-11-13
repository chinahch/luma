package proxy

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func (p Profile) Link() (string, error) {
	if e := p.Validate(); e != nil {
		return "", e
	}
	if p.Protocol == "vmess" {
		tls := ""
		if p.TLS.Enabled {
			tls = "tls"
		}
		v := map[string]any{"v": "2", "ps": p.Name, "add": p.Server, "port": strconv.Itoa(p.Port), "id": p.UUID, "aid": strconv.Itoa(p.AlterID), "net": p.Transport.Network, "host": p.Transport.Host, "path": p.Transport.Path, "tls": tls, "sni": p.TLS.ServerName, "scy": p.Encryption, "type": "none"}
		if p.Transport.Network == "grpc" {
			v["path"] = p.Transport.ServiceName
		}
		if len(p.TLS.ALPN) > 0 {
			v["alpn"] = strings.Join(p.TLS.ALPN, ",")
		}
		b, _ := json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b), nil
	}
	scheme := p.Protocol
	u := url.URL{Host: net.JoinHostPort(p.Server, strconv.Itoa(p.Port)), Fragment: p.Name}
	q := url.Values{}
	switch p.Protocol {
	case "socks":
		scheme = "socks5"
		if p.Username != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		}
	case "http":
		if p.TLS.Enabled {
			scheme = "https"
		}
		if p.Username != "" {
			u.User = url.UserPassword(p.Username, p.Password)
		}
	case "vless":
		u.User = url.User(p.UUID)
		q.Set("encryption", p.Encryption)
		if p.Flow != "" {
			q.Set("flow", p.Flow)
		}
	case "shadowsocks":
		scheme = "ss"
		u.User = url.User(base64.RawURLEncoding.EncodeToString([]byte(p.Method + ":" + p.Password)))
	default:
		u.User = url.User(p.Password)
	}
	if p.Protocol == "vless" || p.Protocol == "trojan" {
		q.Set("type", p.Transport.Network)
		q.Set("security", "none")
	}
	if p.TLS.Enabled {
		if p.Protocol == "vless" || p.Protocol == "trojan" {
			q.Set("security", "tls")
		}
		if p.TLS.RealityPublicKey != "" {
			q.Set("security", "reality")
			q.Set("pbk", p.TLS.RealityPublicKey)
			q.Set("sid", p.TLS.ShortID)
			if p.TLS.RealitySpiderX != "" {
				q.Set("spx", p.TLS.RealitySpiderX)
			}
		}
		if p.TLS.ServerName != "" {
			q.Set("sni", p.TLS.ServerName)
		}
		if p.TLS.Fingerprint != "" {
			q.Set("fp", p.TLS.Fingerprint)
		}
		if len(p.TLS.ALPN) > 0 {
			q.Set("alpn", strings.Join(p.TLS.ALPN, ","))
		}
		if p.TLS.Insecure {
			q.Set("allowInsecure", "1")
		}
		if p.TLS.PinnedPeerCertSHA256 != "" {
			q.Set("pcs", p.TLS.PinnedPeerCertSHA256)
		}
		if p.TLS.VerifyPeerCertByName != "" {
			q.Set("vcn", p.TLS.VerifyPeerCertByName)
		}
	}
	for k, v := range map[string]string{"path": p.Transport.Path, "host": p.Transport.Host, "serviceName": p.Transport.ServiceName, "mode": p.Transport.Mode, "obfs": p.Obfs, "obfs-password": p.ObfsPassword} {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.Scheme = scheme
	u.RawQuery = q.Encode()
	return u.String(), nil
}
