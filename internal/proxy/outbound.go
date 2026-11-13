package proxy

import "errors"

type M = map[string]any

// Outbound returns the node's structured outbound envelope. StreamSettings is
// deliberately separate: Xray transport options are not protocol settings.
func (p Profile) Outbound(kernel, tag string) (M, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	if tag == "" {
		return nil, errors.New("outbound tag required")
	}
	if kernel != "xray" && kernel != "singbox" {
		return nil, errors.New("choose xray or singbox")
	}
	o := M{"tag": tag, "protocol": p.Protocol}
	s := M{}
	o["settings"] = s
	if kernel == "singbox" {
		if p.TLS.RealitySpiderX != "" {
			return nil, errors.New("REALITY spider path requires Xray")
		}
		if p.TLS.PinnedPeerCertSHA256 != "" || p.TLS.VerifyPeerCertByName != "" {
			return nil, errors.New("Xray certificate pin parameters require Xray")
		}
		if p.Protocol == "socks" && p.TLS.Enabled {
			return nil, errors.New("TLS-wrapped SOCKS requires Xray")
		}
		if p.Transport.Network == "xhttp" || p.Encryption != "" && p.Encryption != "none" && p.Protocol == "vless" {
			return nil, errors.New("XHTTP and VLESS native encryption require Xray")
		}
		s["server"] = p.Server
		s["server_port"] = p.Port
		switch p.Protocol {
		case "socks", "http":
			if p.Username != "" {
				s["username"] = p.Username
				s["password"] = p.Password
			}
			if p.Protocol == "socks" {
				s["version"] = "5"
			}
		case "vless", "vmess":
			s["uuid"] = p.UUID
			if p.Flow != "" {
				s["flow"] = p.Flow
			}
			if p.Protocol == "vmess" {
				s["security"] = p.Encryption
				s["alter_id"] = p.AlterID
			}
		case "shadowsocks":
			s["method"] = p.Method
			s["password"] = p.Password
		case "trojan", "hysteria2", "anytls":
			s["password"] = p.Password
		default:
			return nil, errors.New("unsupported sing-box outbound")
		}
		if p.Obfs != "" {
			s["obfs"] = M{"type": p.Obfs, "password": p.ObfsPassword}
		}
		if p.TLS.Enabled {
			t := M{"enabled": true}
			if p.TLS.ServerName != "" {
				t["server_name"] = p.TLS.ServerName
			}
			if p.TLS.Insecure {
				t["insecure"] = true
			}
			if len(p.TLS.ALPN) > 0 {
				t["alpn"] = p.TLS.ALPN
			}
			if p.TLS.Fingerprint != "" || p.TLS.RealityPublicKey != "" {
				fp := p.TLS.Fingerprint
				if fp == "" {
					fp = "chrome"
				}
				t["utls"] = M{"enabled": true, "fingerprint": fp}
			}
			if p.TLS.RealityPublicKey != "" {
				t["reality"] = M{"enabled": true, "public_key": p.TLS.RealityPublicKey, "short_id": p.TLS.ShortID}
			}
			s["tls"] = t
		}
		switch p.Transport.Network {
		case "ws":
			t := M{"type": "ws", "path": p.Transport.Path}
			if p.Transport.Host != "" {
				t["headers"] = M{"Host": p.Transport.Host}
			}
			s["transport"] = t
		case "grpc":
			s["transport"] = M{"type": "grpc", "service_name": p.Transport.ServiceName}
		}
		return o, nil
	}
	if p.TLS.Insecure {
		return nil, errors.New("pinned Xray removed allowInsecure; use a valid certificate or pcs certificate SHA256 pin")
	}
	switch p.Protocol {
	case "socks", "http":
		server := M{"address": p.Server, "port": p.Port}
		if p.Username != "" {
			server["users"] = []M{{"user": p.Username, "pass": p.Password}}
		}
		s["servers"] = []M{server}
	case "vless", "vmess":
		user := M{"id": p.UUID}
		if p.Protocol == "vless" {
			user["encryption"] = p.Encryption
			if p.Flow != "" {
				user["flow"] = p.Flow
			}
		} else {
			user["security"] = p.Encryption
			user["alterId"] = p.AlterID
		}
		s["vnext"] = []M{{"address": p.Server, "port": p.Port, "users": []M{user}}}
	case "trojan", "shadowsocks":
		server := M{"address": p.Server, "port": p.Port, "password": p.Password}
		if p.Protocol == "shadowsocks" {
			server["method"] = p.Method
		}
		s["servers"] = []M{server}
	default:
		return nil, errors.New("this outbound protocol requires sing-box")
	}
	stream := M{"network": p.Transport.Network, "security": "none"}
	if p.TLS.Enabled {
		stream["security"] = "tls"
		t := M{"serverName": p.TLS.ServerName}
		if p.TLS.PinnedPeerCertSHA256 != "" {
			t["pinnedPeerCertSha256"] = p.TLS.PinnedPeerCertSHA256
		}
		if p.TLS.VerifyPeerCertByName != "" {
			t["verifyPeerCertByName"] = p.TLS.VerifyPeerCertByName
		}
		if len(p.TLS.ALPN) > 0 {
			t["alpn"] = p.TLS.ALPN
		}
		if p.TLS.Fingerprint != "" {
			t["fingerprint"] = p.TLS.Fingerprint
		}
		stream["tlsSettings"] = t
		if p.TLS.RealityPublicKey != "" {
			stream["security"] = "reality"
			delete(stream, "tlsSettings")
			fp := p.TLS.Fingerprint
			if fp == "" {
				fp = "chrome"
			}
			stream["realitySettings"] = M{"serverName": p.TLS.ServerName, "publicKey": p.TLS.RealityPublicKey, "shortId": p.TLS.ShortID, "fingerprint": fp}
			if p.TLS.RealitySpiderX != "" {
				stream["realitySettings"].(M)["spiderX"] = p.TLS.RealitySpiderX
			}
		}
	}
	switch p.Transport.Network {
	case "ws":
		t := M{"path": p.Transport.Path}
		if p.Transport.Host != "" {
			t["headers"] = M{"Host": p.Transport.Host}
		}
		stream["wsSettings"] = t
	case "grpc":
		stream["grpcSettings"] = M{"serviceName": p.Transport.ServiceName}
	case "xhttp":
		t := M{"path": p.Transport.Path}
		if p.Transport.Host != "" {
			t["host"] = p.Transport.Host
		}
		if p.Transport.Mode != "" {
			t["mode"] = p.Transport.Mode
		}
		stream["xhttpSettings"] = t
	}
	o["stream_settings"] = stream
	return o, nil
}
