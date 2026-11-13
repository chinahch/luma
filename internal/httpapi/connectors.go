package httpapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chinahch/luma/internal/proxy"
	"github.com/chinahch/luma/internal/traffic"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

type connectorConfig struct {
	Kind        string `json:"kind"`
	UserID      int64  `json:"user_id"`
	Secret      string `json:"secret"`
	Token       string `json:"token"`
	MetricsPort int    `json:"metrics_port"`
	Hostname    string `json:"hostname"`
}

func connectorFrom(raw []byte) (connectorConfig, error) {
	var in struct {
		Protocol  string           `json:"protocol"`
		Port      int              `json:"server_port"`
		Listen    string           `json:"listen_ip"`
		Network   string           `json:"network"`
		TLS       int              `json:"tls"`
		Connector *connectorConfig `json:"connector"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return connectorConfig{}, errors.New("invalid config")
	}
	if in.Connector == nil {
		return connectorConfig{}, nil
	}
	c := *in.Connector
	if c.MetricsPort < 1024 || c.MetricsPort > 65535 || c.MetricsPort == in.Port {
		return c, errors.New("invalid connector metrics port")
	}
	switch c.Kind {
	case "cloudflared":
		if in.Protocol != "vless" && in.Protocol != "vmess" || in.Network != "ws" || in.TLS != 0 || in.Listen != "127.0.0.1" || len(c.Token) < 16 || len(c.Token) > 8192 || strings.ContainsAny(c.Token, " \r\n\t") {
			return c, errors.New("Cloudflare Tunnel requires a loopback WS origin without local TLS and a named tunnel token")
		}
	case "mtg":
		b, e := hex.DecodeString(c.Secret)
		if in.Protocol != "mtproxy" || c.UserID <= 0 || e != nil || len(b) < 18 || len(b) > 270 || b[0] != 0xee {
			return c, errors.New("MTG requires a user and valid ee secret")
		}
	default:
		return c, errors.New("unsupported connector")
	}
	return c, nil
}
func (a *API) validateConnectorOwner(c *gin.Context, raw []byte) bool {
	v, e := connectorFrom(raw)
	if e != nil {
		fail(c, 400, e.Error())
		return false
	}
	if v.Kind != "mtg" {
		return true
	}
	var exists bool
	e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND deleted_at IS NULL)`, v.UserID).Scan(&exists)
	if e != nil {
		internal(c, e)
		return false
	}
	if !exists {
		fail(c, 400, "MTProxy owner must be an existing account")
		return false
	}
	return true
}

type desiredConnector struct {
	NodeID      int64  `json:"node_id"`
	Kind        string `json:"kind"`
	UserID      int64  `json:"user_id,omitempty"`
	Port        int    `json:"port"`
	MetricsPort int    `json:"metrics_port"`
	Credential  string `json:"credential"`
	SOCKSProxy  string `json:"socks_proxy,omitempty"`
}

func (a *API) connectors(c *gin.Context) {
	if !a.requireBox(c) {
		return
	}
	ident := c.MustGet("node_identity").(nodeIdentity)
	rows, e := a.s.Pool.Query(c.Request.Context(), `SELECT id,config FROM nodes WHERE active AND config ? 'connector' AND (($1::bigint>0 AND machine_id=$1) OR ($1=0 AND id=$2)) ORDER BY id LIMIT 65`, ident.MachineID, ident.NodeID)
	if e != nil {
		internal(c, e)
		return
	}
	type candidate struct {
		id  int64
		raw []byte
	}
	candidates := []candidate{}
	for rows.Next() {
		var n candidate
		if e = rows.Scan(&n.id, &n.raw); e != nil {
			rows.Close()
			internal(c, e)
			return
		}
		candidates = append(candidates, n)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		internal(c, e)
		return
	}
	if len(candidates) > 64 {
		fail(c, 409, "connector limit is 64 per worker")
		return
	}
	desired := []desiredConnector{}
	for _, n := range candidates {
		raw, ok := a.fullNodeConfig(c, n.id, n.raw)
		if !ok {
			return
		}
		con, e := connectorFrom(raw)
		if e != nil {
			fail(c, 409, "invalid connector configuration")
			return
		}
		var cfg map[string]any
		json.Unmarshal(raw, &cfg)
		port := int(cfg["server_port"].(float64))
		d := desiredConnector{NodeID: n.id, Kind: con.Kind, UserID: con.UserID, Port: port, MetricsPort: con.MetricsPort, Credential: con.Token}
		if con.Kind == "mtg" {
			var allowed bool
			allowed, e = a.nodeAllowed(c.Request.Context(), con.UserID, n.id)
			if e != nil {
				internal(c, e)
				return
			}
			if !allowed {
				continue
			}
			d.Credential = con.Secret
			var routeRaw []byte
			e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT config FROM node_exit_routes WHERE node_id=$1`, n.id).Scan(&routeRaw)
			if e != nil && e != pgx.ErrNoRows {
				internal(c, e)
				return
			}
			if e == nil {
				var r proxy.Routes
				if json.Unmarshal(routeRaw, &r) != nil {
					fail(c, 409, "invalid stored MTG exit routes")
					return
				}
				if _, e = r.Compile(); e != nil {
					fail(c, 409, "invalid stored MTG exit routes")
					return
				}
				if len(r.Rules) > 0 {
					fail(c, 409, "MTG supports only a default SOCKS5 exit")
					return
				}
				if r.DefaultExit > 0 && (r.Global == nil || *r.Global) {
					p, ok := a.loadExit(c, r.DefaultExit)
					if !ok {
						return
					}
					var active bool
					if e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT active FROM exits WHERE id=$1`, r.DefaultExit).Scan(&active); e != nil || !active || p.Protocol != "socks" || p.TLS.Enabled {
						fail(c, 409, "MTG exit must be an active plain SOCKS5 proxy")
						return
					}
					d.SOCKSProxy, e = p.Link()
					if e != nil {
						fail(c, 409, "invalid stored MTG SOCKS5 exit")
						return
					}
				}
			}
		}
		desired = append(desired, d)
	}
	c.JSON(200, gin.H{"connectors": desired, "lease_until": time.Now().UTC().Add(90 * time.Second), "poll_seconds": 15})
}
func (a *API) connectorStatus(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	var in struct {
		State string `json:"state"`
		Kind  string `json:"kind"`
	}
	if json.Unmarshal(c.MustGet("node_raw").([]byte), &in) != nil {
		fail(c, 400, "invalid status")
		return
	}
	allowed := map[string]bool{"starting": true, "ready": true, "unhealthy": true, "stopped": true, "queue_blocked": true}
	if !allowed[in.State] || in.Kind != "mtg" && in.Kind != "cloudflared" {
		fail(c, 400, "invalid connector state")
		return
	}
	raw, _ := json.Marshal(gin.H{"state": in.State, "kind": in.Kind, "seen_at": time.Now().UTC()})
	if _, e := a.s.Pool.Exec(c.Request.Context(), `UPDATE nodes SET status=jsonb_set(status,'{connector}',$2),last_report_at=now() WHERE id=$1`, id, raw); e != nil {
		internal(c, e)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) connectorReport(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	var cfg []byte
	if e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT config FROM nodes WHERE id=$1`, id).Scan(&cfg); e != nil {
		internal(c, e)
		return
	}
	var v struct {
		Connector connectorConfig `json:"connector"`
	}
	json.Unmarshal(cfg, &v)
	var report traffic.Report
	if json.Unmarshal(c.MustGet("node_raw").([]byte), &report) != nil || v.Connector.Kind != "mtg" || len(report.Entries) != 1 || report.Entries[0].UserID != v.Connector.UserID {
		fail(c, 400, "MTG report must contain exactly its assigned user")
		return
	}
	a.ingest(c, id, report, "reliable")
}
func (a *API) telegramLinks(c *gin.Context) {
	rows, e := a.s.Pool.Query(c.Request.Context(), `SELECT n.id,n.name,n.public_host,n.config,p.encrypted_config FROM nodes n LEFT JOIN node_private p ON p.node_id=n.id JOIN users u ON u.id=$1 WHERE n.active AND n.public_host<>'' AND n.config->'connector'->>'kind'='mtg' AND n.config->'connector'->>'user_id'=$2 AND u.deleted_at IS NULL AND u.active AND u.used_bytes<u.quota_bytes AND (u.expires_at IS NULL OR u.expires_at>now()) AND (NOT EXISTS(SELECT 1 FROM node_plans np WHERE np.node_id=n.id) OR EXISTS(SELECT 1 FROM node_plans np WHERE np.node_id=n.id AND np.plan_id=u.plan_id)) ORDER BY n.id`, current(c).ID, fmt.Sprint(current(c).ID))
	if e != nil {
		internal(c, e)
		return
	}
	defer rows.Close()
	links := []gin.H{}
	for rows.Next() {
		var id int64
		var name, host string
		var raw, cipher []byte
		if e = rows.Scan(&id, &name, &host, &raw, &cipher); e != nil {
			internal(c, e)
			return
		}
		full, ok := a.openNodeConfig(c, id, raw, cipher)
		if !ok {
			return
		}
		con, e := connectorFrom(full)
		if e != nil {
			continue
		}
		var cfg struct {
			Port int `json:"server_port"`
		}
		json.Unmarshal(full, &cfg)
		links = append(links, gin.H{"name": name, "server": host, "port": cfg.Port, "secret": con.Secret})
	}
	if e = rows.Err(); e != nil {
		internal(c, e)
		return
	}
	c.JSON(200, links)
}
