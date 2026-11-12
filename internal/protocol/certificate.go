package protocol

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// CertificateConfig is the native cert_config wire format. UI-only saved flags
// and secret_action are resolved by the API and never sent to native workers.
type CertificateConfig struct {
	CertMode    string            `json:"cert_mode"`
	Domain      string            `json:"domain,omitempty"`
	Email       string            `json:"email,omitempty"`
	DNSProvider string            `json:"dns_provider,omitempty"`
	DNSEnv      map[string]string `json:"dns_env,omitempty"`
	HTTPPort    int               `json:"http_port,omitempty"`
	CertFile    string            `json:"cert_file,omitempty"`
	KeyFile     string            `json:"key_file,omitempty"`
	CertContent string            `json:"cert_content,omitempty"`
	KeyContent  string            `json:"key_content,omitempty"`
}

type DNSProvider struct {
	Name           string       `json:"name"`
	Aliases        []string     `json:"aliases"`
	Keys           []string     `json:"keys"`
	Required       [][]string   `json:"required"`
	CredentialSets [][][]string `json:"credential_sets,omitempty"`
}

// Generated from cedar2025/Xboard-Node 0a29338e1f102a462363ce3527417029f89bab28.
//
//go:embed dns_providers.json
var providerJSON []byte

func DNSProviders() []DNSProvider {
	var p []DNSProvider
	if json.Unmarshal(providerJSON, &p) != nil {
		panic("invalid embedded DNS provider catalogue")
	}
	return p
}
func FindDNSProvider(name string) (DNSProvider, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, p := range DNSProviders() {
		if p.Name == name {
			return p, true
		}
		for _, a := range p.Aliases {
			if a == name {
				return p, true
			}
		}
	}
	return DNSProvider{}, false
}

var envKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

func ParseDNSEnv(text string) (map[string]string, error) {
	if len(text) > 65536 {
		return nil, errors.New("DNS 凭据不能超过 64 KiB")
	}
	env := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !envKey.MatchString(key) || strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("DNS 凭据需要每行 KEY=VALUE，不要包含 export")
		}
		if _, duplicate := env[key]; duplicate {
			return nil, errors.New("DNS 凭据变量名重复")
		}
		env[key] = value
	}
	if len(env) > 64 {
		return nil, errors.New("DNS 凭据不能超过 64 项")
	}
	return env, nil
}

func validDNSName(name string, wildcard bool) bool {
	if wildcard && strings.HasPrefix(name, "*.") {
		name = name[2:]
	}
	if len(name) < 3 || len(name) > 253 || net.ParseIP(name) != nil || !strings.Contains(name, ".") {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
				return false
			}
		}
	}
	return true
}
func certText(c map[string]any, key string) string { s, _ := c[key].(string); return s }
func CertificateFromMap(value any) (CertificateConfig, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return CertificateConfig{}, errors.New("证书配置格式错误")
	}
	var c CertificateConfig
	if json.Unmarshal(raw, &c) != nil {
		return c, errors.New("证书配置字段类型错误")
	}
	return c, nil
}
func CertificateMap(c CertificateConfig) M {
	raw, _ := json.Marshal(c)
	var m M
	json.Unmarshal(raw, &m)
	return m
}

// ValidateCertificate accepts legacy configs without a cert_config. Explicit
// certificate configs must be complete; invalid values never reach a worker.
func ValidateCertificate(cfg M) error {
	value, exists := cfg["cert_config"]
	if !exists {
		return nil
	}
	if value == nil {
		return errors.New("证书配置不能为空")
	}
	c, e := CertificateFromMap(value)
	if e != nil {
		return e
	}
	if n := cfg["tls"]; n == 2 || n == float64(2) {
		return errors.New("REALITY 不使用普通 TLS 证书配置")
	}
	sni := certText(cfg, "server_name")
	if settings, ok := cfg["tls_settings"].(map[string]any); ok {
		nested := certText(settings, "server_name")
		if sni != "" && nested != "" && !strings.EqualFold(sni, nested) {
			return errors.New("顶层与 TLS 设置中的服务器名称不一致，请使用相同的具体 SNI")
		}
		if nested != "" {
			sni = nested
		}
	}
	if sni == "" {
		sni = c.Domain
	}
	if strings.Contains(sni, "*") || (!validDNSName(sni, false) && net.ParseIP(sni) == nil) {
		return errors.New("TLS 服务器名称必须是具体域名或 IP，不能使用泛域名")
	}
	if c.Domain != "" {
		if !validDNSName(c.Domain, true) && net.ParseIP(c.Domain) == nil {
			return errors.New("证书域名格式错误")
		}
		cert := x509.Certificate{DNSNames: []string{c.Domain}}
		if net.ParseIP(c.Domain) != nil {
			cert.IPAddresses = []net.IP{net.ParseIP(c.Domain)}
		}
		if cert.VerifyHostname(sni) != nil {
			return errors.New("证书域名未覆盖 TLS 服务器名称")
		}
	}
	switch c.CertMode {
	case "dns", "http":
		if !validDNSName(c.Domain, c.CertMode == "dns") {
			return errors.New("ACME 证书需要合法域名；HTTP-01 不支持泛域名")
		}
		addr, err := mail.ParseAddress(c.Email)
		if err != nil || addr.Address != c.Email || len(c.Email) > 254 {
			return errors.New("请填写有效的证书通知邮箱")
		}
		if c.CertFile != "" || c.KeyFile != "" || c.CertContent != "" || c.KeyContent != "" {
			return errors.New("ACME 模式不能同时提交文件或 PEM 证书")
		}
		if c.CertMode == "http" {
			if c.DNSProvider != "" || len(c.DNSEnv) > 0 {
				return errors.New("HTTP-01 不能携带 DNS 凭据")
			}
			if c.HTTPPort != 0 && c.HTTPPort != 80 {
				return errors.New("HTTP-01 当前仅支持节点公网 TCP 80")
			}
			break
		}
		p, ok := FindDNSProvider(c.DNSProvider)
		if !ok {
			return errors.New("DNS 提供商不在当前原生版本支持范围内")
		}
		if len(c.DNSEnv) > 64 {
			return errors.New("DNS 凭据项目过多")
		}
		total := 0
		for key, v := range c.DNSEnv {
			allowed := false
			for _, k := range p.Keys {
				if k == key {
					allowed = true
					break
				}
			}
			if !allowed || !envKey.MatchString(key) {
				return errors.New("DNS 凭据包含当前提供商不支持的变量名")
			}
			total += len(key) + len(v)
			if strings.TrimSpace(v) == "" || strings.ContainsAny(v, "\x00\r\n") || len(v) > 16384 {
				return errors.New("DNS 凭据值无效或过长")
			}
		}
		if total > 65536 {
			return errors.New("DNS 凭据不能超过 64 KiB")
		}
		for _, group := range p.Required {
			found := false
			for _, key := range group {
				if c.DNSEnv[key] != "" {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("缺少 DNS 凭据：%s", strings.Join(group, " / "))
			}
		}
		if len(p.CredentialSets) > 0 {
			valid := false
			for _, set := range p.CredentialSets {
				complete := true
				for _, group := range set {
					found := false
					for _, key := range group {
						if c.DNSEnv[key] != "" {
							found = true
						}
					}
					complete = complete && found
				}
				valid = valid || complete
			}
			if !valid {
				return errors.New("缺少 DNS 凭据，请完整填写该提供商支持的一组凭据")
			}
		}
	case "file":
		if !strings.HasPrefix(c.CertFile, "/") || !strings.HasPrefix(c.KeyFile, "/") || strings.ContainsAny(c.CertFile+c.KeyFile, "\x00\r\n") {
			return errors.New("请填写节点上的证书和私钥绝对路径")
		}
		if len(c.DNSEnv) > 0 || c.DNSProvider != "" || c.CertContent != "" || c.KeyContent != "" {
			return errors.New("文件证书模式不能携带 DNS 凭据或 PEM 内容")
		}
	case "content":
		if len(c.DNSEnv) > 0 || c.DNSProvider != "" || c.CertFile != "" || c.KeyFile != "" {
			return errors.New("PEM 模式不能携带 DNS 凭据或证书路径")
		}
		if len(c.CertContent) > 131072 || len(c.KeyContent) > 32768 {
			return errors.New("PEM 证书内容过长")
		}
		pair, err := tls.X509KeyPair([]byte(c.CertContent), []byte(c.KeyContent))
		if err != nil || len(pair.Certificate) == 0 {
			return errors.New("PEM 证书与私钥无效或不匹配")
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil || leaf.VerifyHostname(sni) != nil {
			return errors.New("PEM 证书没有覆盖 TLS 服务器名称")
		}
		if time.Now().After(leaf.NotAfter) || time.Now().Before(leaf.NotBefore) {
			return errors.New("PEM 证书已过期或尚未生效")
		}
	default:
		return errors.New("请选择 DNS-01、HTTP-01、节点文件或 PEM 证书模式")
	}
	return nil
}
