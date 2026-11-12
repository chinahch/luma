// Package protocol defines presets for the exact kernels pinned by caddy.
package protocol

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"strings"
)

type M = map[string]any
type Input struct {
	Preset         string `json:"preset"`
	Port           int    `json:"port"`
	ServerName     string `json:"server_name"`
	Destination    string `json:"destination"`
	Path           string `json:"path"`
	CertFile       string `json:"cert_file"`
	KeyFile        string `json:"key_file"`
	Authentication string `json:"authentication"`
	Appearance     string `json:"appearance"`
	MetricsPort    int    `json:"metrics_port"`
	TunnelToken    string `json:"tunnel_token"`
	OwnerID        int64  `json:"owner_id"`
}
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kernel  string `json:"kernel"`
	TLS     bool   `json:"certificate"`
	Reality bool   `json:"reality"`
	ENC     bool   `json:"enc"`
	Path    bool   `json:"path"`
}

func Catalog() []Preset {
	return []Preset{
		{ID: "socks5", Name: "SOCKS5", Kernel: "singbox"},
		{ID: "vless-reality", Name: "VLESS + REALITY", Kernel: "xray", Reality: true},
		{ID: "hysteria2", Name: "Hysteria2", Kernel: "singbox", TLS: true},
		{ID: "shadowsocks", Name: "Shadowsocks (AES-128-GCM)", Kernel: "singbox"},
		{ID: "ss2022", Name: "Shadowsocks 2022 (AES-128-GCM)", Kernel: "singbox"},
		{ID: "vless-enc-reality-vision", Name: "VLESS ENC + REALITY + Vision", Kernel: "xray", Reality: true, ENC: true},
		{ID: "vless-enc", Name: "VLESS ENC", Kernel: "xray", ENC: true},
		{ID: "vmess-ws", Name: "VMess + WS + TLS", Kernel: "xray", TLS: true, Path: true},
		{ID: "vless-ws", Name: "VLESS + WS + TLS", Kernel: "xray", TLS: true, Path: true},
		{ID: "anytls-reality", Name: "AnyTLS + REALITY", Kernel: "singbox", Reality: true},
		{ID: "anytls-tls", Name: "AnyTLS + TLS", Kernel: "singbox", TLS: true},
		{ID: "vless-xhttp-reality", Name: "VLESS + XHTTP + REALITY", Kernel: "xray", Reality: true, Path: true},
		{ID: "vless-xhttp-reality-enc", Name: "VLESS + XHTTP + REALITY + ENC / Vision", Kernel: "xray", Reality: true, ENC: true, Path: true},
		{ID: "vless-xhttp-tls-h3", Name: "VLESS + XHTTP + TLS + H3 (UDP)", Kernel: "xray", TLS: true, Path: true},
		{ID: "cf-tunnel", Name: "Cloudflare Tunnel + VLESS WS", Kernel: "singbox", Path: true},
		{ID: "mtg", Name: "MTProto / MTProxy (mtg · 每用户独立实例)", Kernel: "connector"},
	}
}
func Build(in Input) (M, error) {
	var preset *Preset
	for _, p := range Catalog() {
		if p.ID == in.Preset {
			copy := p
			preset = &copy
			break
		}
	}
	if preset == nil {
		return nil, errors.New("unknown native protocol preset; connectors use separate configuration")
	}
	if in.Port < 1 || in.Port > 65535 {
		return nil, errors.New("port must be 1..65535")
	}
	if in.Preset == "cf-tunnel" || in.Preset == "mtg" {
		if in.MetricsPort < 1024 || in.MetricsPort > 65535 || in.MetricsPort == in.Port {
			return nil, errors.New("connector metrics port must be 1024..65535 and different from the listener")
		}
		if len(in.ServerName) == 0 || len(in.ServerName) > 253 || strings.ContainsAny(in.ServerName, " /:@?#*\r\n\t") {
			return nil, errors.New("connector hostname required")
		}
		if in.Preset == "mtg" {
			if in.OwnerID <= 0 {
				return nil, errors.New("MTProxy requires one user per instance for attributable accounting")
			}
			return M{"preset": in.Preset, "kernel_type": "connector", "protocol": "mtproxy", "server_port": in.Port, "network": "tcp", "tls": 0, "connector": M{"kind": "mtg", "user_id": in.OwnerID, "secret": "ee" + randomHex(16) + hex.EncodeToString([]byte(in.ServerName)), "metrics_port": in.MetricsPort}}, nil
		}
		if len(in.TunnelToken) < 16 || len(in.TunnelToken) > 8192 || strings.ContainsAny(in.TunnelToken, " \r\n\t") {
			return nil, errors.New("named Cloudflare Tunnel token required")
		}
	}
	if preset.TLS || preset.Reality {
		if len(in.ServerName) == 0 || len(in.ServerName) > 253 || strings.ContainsAny(in.ServerName, " /:@?#*\r\n\t") {
			return nil, errors.New("valid TLS server name required")
		}
	}
	c := M{"preset": in.Preset, "kernel_type": preset.Kernel, "protocol": "vless", "listen_ip": "0.0.0.0", "server_port": in.Port, "network": "tcp", "tls": 0}
	if preset.Path {
		if in.Path == "" {
			in.Path = "/" + randomHex(12)
		}
		if !strings.HasPrefix(in.Path, "/") || len(in.Path) > 1024 || strings.ContainsAny(in.Path, "\r\n") {
			return nil, errors.New("path must begin with / and be at most 1024 bytes")
		}
		network := "ws"
		if strings.Contains(in.Preset, "xhttp") {
			network = "xhttp"
		}
		c["network"] = network
		c["networkSettings"] = M{"path": in.Path}
		if network == "xhttp" {
			c["networkSettings"].(M)["mode"] = "auto"
		}
	}
	if preset.TLS {
		if !strings.HasPrefix(in.CertFile, "/") || !strings.HasPrefix(in.KeyFile, "/") || strings.ContainsAny(in.CertFile+in.KeyFile, "\r\n\x00") {
			return nil, errors.New("absolute certificate and key paths on the node are required")
		}
		c["tls"] = 1
		c["server_name"] = in.ServerName
		c["tls_settings"] = M{"server_name": in.ServerName}
		c["cert_config"] = M{"cert_mode": "file", "domain": in.ServerName, "cert_file": in.CertFile, "key_file": in.KeyFile}
	}
	if preset.Reality {
		dest := in.Destination
		if dest == "" {
			dest = net.JoinHostPort(in.ServerName, "443")
		}
		if host, port, e := net.SplitHostPort(dest); e != nil || host == "" || port == "" {
			return nil, errors.New("REALITY destination must be host:port")
		}
		key, e := ecdh.X25519().GenerateKey(rand.Reader)
		if e != nil {
			return nil, e
		}
		c["tls"] = 2
		c["tls_settings"] = M{"server_name": in.ServerName, "dest": dest, "private_key": base64.RawURLEncoding.EncodeToString(key.Bytes()), "public_key": base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), "short_id": randomHex(8)}
	}
	if preset.ENC {
		secret, public, e := encryptionKeys(in.Authentication, in.Appearance)
		if e != nil {
			return nil, e
		}
		c["decryption"] = secret
		c["encryption"] = public
	}
	switch in.Preset {
	case "socks5":
		c["protocol"] = "socks"
	case "shadowsocks", "ss2022":
		c["protocol"] = "shadowsocks"
		c["cipher"] = "aes-128-gcm"
		if in.Preset == "ss2022" {
			c["cipher"] = "2022-blake3-aes-128-gcm"
			key := make([]byte, 16)
			if _, e := rand.Read(key); e != nil {
				return nil, e
			}
			c["server_key"] = base64.StdEncoding.EncodeToString(key)
		}
	case "hysteria2":
		c["protocol"] = "hysteria"
		c["version"] = 2
	case "vmess-ws":
		c["protocol"] = "vmess"
	case "anytls-tls", "anytls-reality":
		c["protocol"] = "anytls"
	case "vless-reality", "vless-enc-reality-vision", "vless-xhttp-reality-enc":
		c["flow"] = "xtls-rprx-vision"
	case "vless-xhttp-tls-h3":
		c["networkSettings"].(M)["alpn"] = "h3"
	case "cf-tunnel":
		c["listen_ip"] = "127.0.0.1"
		c["connector"] = M{"kind": "cloudflared", "token": in.TunnelToken, "metrics_port": in.MetricsPort, "hostname": in.ServerName}
	}
	return c, nil
}
func encryptionKeys(auth, appearance string) (string, string, error) {
	if auth == "" {
		auth = "x25519"
	}
	if appearance == "" {
		appearance = "native"
	}
	if appearance != "native" && appearance != "xorpub" && appearance != "random" {
		return "", "", errors.New("choose native, xorpub or random appearance")
	}
	var secret, public []byte
	switch auth {
	case "x25519":
		k, e := ecdh.X25519().GenerateKey(rand.Reader)
		if e != nil {
			return "", "", e
		}
		secret = k.Bytes()
		public = k.PublicKey().Bytes()
	case "mlkem768":
		k, e := mlkem.GenerateKey768()
		if e != nil {
			return "", "", e
		}
		secret = k.Bytes()
		public = k.EncapsulationKey().Bytes()
	default:
		return "", "", errors.New("choose x25519 or mlkem768 authentication")
	}
	prefix := "mlkem768x25519plus." + appearance + "."
	return prefix + "600s." + base64.RawURLEncoding.EncodeToString(secret), prefix + "0rtt." + base64.RawURLEncoding.EncodeToString(public), nil
}
func randomHex(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
