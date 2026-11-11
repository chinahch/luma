package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
)

func (a *API) rotateSubscription(c *gin.Context) {
	token := store.NewToken()
	if _, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE users SET subscription_hash=$2 WHERE id=$1`, current(c).ID, store.Hash(token)); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"path": "/sub/" + token, "notice": "previous subscription link is now revoked"})
}
func (a *API) subscription(c *gin.Context) {
	token := c.Param("token")
	if len(token) != 43 {
		fail(c, 404, "subscription not found")
		return
	}
	var uuid string
	var used, quota int64
	var expiry *time.Time
	err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT uuid,used_bytes,quota_bytes,expires_at FROM users WHERE subscription_hash=$1 AND active AND used_bytes<quota_bytes AND (expires_at IS NULL OR expires_at>now())`, store.Hash(token)).Scan(&uuid, &used, &quota, &expiry)
	if err != nil {
		fail(c, 404, "subscription unavailable or quota expired")
		return
	}
	rows, err := a.s.Pool.Query(c.Request.Context(), `SELECT name,public_host,config FROM nodes WHERE active AND public_host<>'' AND config->>'protocol'='vless' ORDER BY id`)
	if err != nil {
		internal(c, err)
		return
	}
	defer rows.Close()
	links := []string{}
	for rows.Next() {
		var name, host string
		var raw []byte
		if err = rows.Scan(&name, &host, &raw); err != nil {
			internal(c, err)
			return
		}
		if link := vlessLink(uuid, name, host, raw); link != "" {
			links = append(links, link)
		}
	}
	if rows.Err() != nil {
		internal(c, rows.Err())
		return
	}
	if len(links) == 0 {
		fail(c, 503, "no supported VLESS nodes configured; other subscription formats are not implemented in this preview")
		return
	}
	info := "upload=0; download=" + strconv.FormatInt(used, 10) + "; total=" + strconv.FormatInt(quota, 10)
	if expiry != nil {
		info += "; expire=" + strconv.FormatInt(expiry.Unix(), 10)
	}
	c.Header("Subscription-Userinfo", info)
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="luma.txt"`)
	c.String(200, base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))))
}
func vlessLink(uuid, name, host string, raw []byte) string {
	var cfg struct {
		Port            int            `json:"server_port"`
		Network         string         `json:"network"`
		TLS             int            `json:"tls"`
		ServerName      string         `json:"server_name"`
		Host            string         `json:"host"`
		TLSSettings     map[string]any `json:"tls_settings"`
		Flow            string         `json:"flow"`
		NetworkSettings map[string]any `json:"networkSettings"`
	}
	if json.Unmarshal(raw, &cfg) != nil || cfg.Port < 1 || cfg.TLS == 2 {
		return ""
	}
	if cfg.Network == "" {
		cfg.Network = "tcp"
	}
	if cfg.Network != "tcp" && cfg.Network != "ws" && cfg.Network != "xhttp" && cfg.Network != "grpc" {
		return ""
	}
	q := url.Values{"encryption": {"none"}, "type": {cfg.Network}, "security": {"none"}}
	if cfg.TLS == 1 {
		q.Set("security", "tls")
		sni := cfg.ServerName
		if sni == "" {
			sni = cfg.Host
		}
		if value, ok := cfg.TLSSettings["server_name"].(string); ok && value != "" {
			sni = value
		}
		if sni == "" {
			sni = host
		}
		q.Set("sni", sni)
	}
	if cfg.Flow != "" {
		q.Set("flow", cfg.Flow)
	}
	for _, key := range []string{"path", "host", "mode", "serviceName", "alpn"} {
		if val, ok := cfg.NetworkSettings[key].(string); ok && val != "" {
			q.Set(key, val)
		}
	}
	if q.Get("serviceName") == "" {
		if name, ok := cfg.NetworkSettings["service_name"].(string); ok {
			q.Set("serviceName", name)
		}
	}
	if q.Get("host") == "" {
		if headers, ok := cfg.NetworkSettings["headers"].(map[string]any); ok {
			if value, ok := headers["Host"].(string); ok {
				q.Set("host", value)
			}
		}
	}
	u := url.URL{Scheme: "vless", User: url.User(uuid), Host: net.JoinHostPort(host, strconv.Itoa(cfg.Port)), RawQuery: q.Encode(), Fragment: name}
	return u.String()
}
