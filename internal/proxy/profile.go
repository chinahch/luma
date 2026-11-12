// Package proxy parses share links locally and compiles explicit kernel outbounds.
package proxy

type Profile struct {
	Name         string    `json:"name"`
	Protocol     string    `json:"protocol"`
	Server       string    `json:"server"`
	Port         int       `json:"port"`
	Username     string    `json:"username,omitempty"`
	Password     string    `json:"password,omitempty"`
	UUID         string    `json:"uuid,omitempty"`
	Method       string    `json:"method,omitempty"`
	AlterID      int       `json:"alter_id,omitempty"`
	Encryption   string    `json:"encryption,omitempty"`
	Flow         string    `json:"flow,omitempty"`
	Obfs         string    `json:"obfs,omitempty"`
	ObfsPassword string    `json:"obfs_password,omitempty"`
	Transport    Transport `json:"transport"`
	TLS          TLS       `json:"tls"`
}
type Transport struct {
	Network     string `json:"network"`
	Path        string `json:"path,omitempty"`
	Host        string `json:"host,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	Mode        string `json:"mode,omitempty"`
}
type TLS struct {
	Enabled              bool     `json:"enabled"`
	ServerName           string   `json:"server_name,omitempty"`
	ALPN                 []string `json:"alpn,omitempty"`
	Insecure             bool     `json:"insecure,omitempty"`
	RealityPublicKey     string   `json:"reality_public_key,omitempty"`
	ShortID              string   `json:"short_id,omitempty"`
	Fingerprint          string   `json:"fingerprint,omitempty"`
	PinnedPeerCertSHA256 string   `json:"pinned_peer_cert_sha256,omitempty"`
	VerifyPeerCertByName string   `json:"verify_peer_cert_by_name,omitempty"`
}
