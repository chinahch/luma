package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chinahch/luma/internal/proxy"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (a *API) nodeExits(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var raw []byte
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT COALESCE((SELECT config FROM node_exit_routes WHERE node_id=n.id),'{"default_exit":0,"rules":[]}') FROM nodes n WHERE id=$1`, id).Scan(&raw)
	if e == pgx.ErrNoRows {
		fail(c, 404, "node not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	c.Data(200, "application/json", raw)
}
func (a *API) saveNodeExits(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var r proxy.Routes
	if !decode(c, &r) {
		return
	}
	if _, e := r.Compile(); e != nil {
		fail(c, 400, e.Error())
		return
	}
	var raw []byte
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT config FROM nodes WHERE id=$1`, id).Scan(&raw)
	if e == pgx.ErrNoRows {
		fail(c, 404, "node not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	var cfg map[string]any
	json.Unmarshal(raw, &cfg)
	if len(r.ExitIDs()) > 0 {
		if !a.requireBox(c) {
			return
		}
		if _, e = a.compileNodeExits(c, r, cfg); e != nil {
			if !c.IsAborted() {
				fail(c, 400, e.Error())
			}
			return
		}
	}
	raw, _ = json.Marshal(r)
	if _, e = a.s.Pool.Exec(c.Request.Context(), `INSERT INTO node_exit_routes(node_id,config) VALUES($1,$2) ON CONFLICT(node_id) DO UPDATE SET config=excluded.config,updated_at=now()`, id, raw); e != nil {
		dbFailure(c, e)
		return
	}
	c.JSON(200, gin.H{"ok": true, "notice": "requires landing-v1 node; applied on next successful config refresh"})
}
func (a *API) compileNodeExits(c *gin.Context, r proxy.Routes, cfg map[string]any) ([]proxy.M, error) {
	kernel, _ := cfg["kernel_type"].(string)
	mtg := cfg["protocol"] == "mtproxy"
	if mtg {
		if len(r.Rules) > 0 {
			return nil, errors.New("MTG supports only a default SOCKS5 exit")
		}
		kernel = "singbox"
	}
	if kernel == "" {
		kernel = "singbox"
	}
	out := []proxy.M{}
	for _, id := range r.ExitIDs() {
		var active bool
		e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT active FROM exits WHERE id=$1`, id).Scan(&active)
		if e == pgx.ErrNoRows || e == nil && !active {
			return nil, fmt.Errorf("exit #%d missing or disabled", id)
		}
		if e != nil {
			return nil, errors.New("cannot read exit")
		}
		p, ok := a.loadExit(c, id)
		if !ok {
			return nil, errors.New("cannot decrypt exit")
		}
		if mtg && (p.Protocol != "socks" || p.TLS.Enabled) {
			return nil, errors.New("MTG requires a plain SOCKS5 landing")
		}
		o, e := p.Outbound(kernel, proxy.ExitTag(id))
		if e != nil {
			return nil, fmt.Errorf("exit #%d incompatible with %s: %s", id, kernel, e)
		}
		out = append(out, o)
	}
	return out, nil
}
func (a *API) applyNodeExits(c *gin.Context, id int64, cfg map[string]any) bool {
	var raw []byte
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT config FROM node_exit_routes WHERE node_id=$1`, id).Scan(&raw)
	if e == pgx.ErrNoRows {
		return true
	}
	if e != nil {
		internal(c, e)
		return false
	}
	var r proxy.Routes
	if json.Unmarshal(raw, &r) != nil {
		fail(c, 500, "invalid stored exit routes")
		return false
	}
	if len(r.ExitIDs()) == 0 {
		return true
	}
	capable := false
	for _, v := range strings.Split(c.GetHeader("X-Luma-Capabilities"), ",") {
		if strings.TrimSpace(v) == "landing-v1" {
			capable = true
		}
	}
	if !capable {
		fail(c, 409, "node requires landing-v1 capability; upgrade the node before enabling exit routing")
		return false
	}
	if !a.requireBox(c) {
		return false
	}
	out, e := a.compileNodeExits(c, r, cfg)
	if e != nil {
		if !c.IsAborted() {
			fail(c, 409, e.Error())
		}
		return false
	}
	rules, e := r.Compile()
	if e != nil {
		fail(c, 409, "invalid stored exit routes")
		return false
	}
	existing, _ := cfg["custom_outbounds"].([]any)
	for _, o := range existing {
		if m, ok := o.(map[string]any); ok {
			if tag, ok := m["tag"].(string); ok && strings.HasPrefix(tag, "luma-exit-") {
				fail(c, 409, "reserved Luma exit tag in custom outbounds")
				return false
			}
		}
	}
	for _, o := range out {
		existing = append(existing, o)
	}
	cfg["custom_outbounds"] = existing
	// Keep existing custom rules after the private guard but ahead of the
	// generated default rule, which would otherwise shadow their matches.
	old, _ := cfg["custom_route_rules"].([]any)
	merged := []any{}
	if len(rules) > 0 {
		merged = append(merged, rules[0])
	}
	for _, rule := range old {
		merged = append(merged, rule)
	}
	for _, rule := range rules[1:] {
		merged = append(merged, rule)
	}
	cfg["custom_route_rules"] = merged
	return true
}
