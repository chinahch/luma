package httpapi

import (
	"context"
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
	ctx := c.Request.Context()
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT config FROM nodes WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&raw)
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
	if !a.lockConfiguredExits(c, tx, r) {
		return
	}
	if len(r.ExitIDs()) > 0 {
		if !a.requireBox(c) {
			return
		}
		if _, e = a.compileNodeExitsWith(ctx, tx, r, cfg); e != nil {
			if !c.IsAborted() {
				fail(c, 400, e.Error())
			}
			return
		}
	}
	raw, _ = json.Marshal(r)
	if _, e = tx.Exec(ctx, `INSERT INTO node_exit_routes(node_id,config) VALUES($1,$2) ON CONFLICT(node_id) DO UPDATE SET config=excluded.config,updated_at=now()`, id, raw); e != nil {
		dbFailure(c, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	c.JSON(200, gin.H{"ok": true, "notice": "requires landing-v1 node; applied on next successful config refresh"})
}
func (a *API) compileNodeExits(c *gin.Context, r proxy.Routes, cfg map[string]any) ([]proxy.M, error) {
	return a.compileNodeExitsWith(c.Request.Context(), a.s.Pool, r, cfg)
}

type exitQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (a *API) compileNodeExitsWith(ctx context.Context, q exitQueryer, r proxy.Routes, cfg map[string]any) ([]proxy.M, error) {
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
		var sid string
		var cipher []byte
		e := q.QueryRow(ctx, `SELECT active,secret_id::text,encrypted_profile FROM exits WHERE id=$1 FOR SHARE`, id).Scan(&active, &sid, &cipher)
		if e == pgx.ErrNoRows || e == nil && !active {
			return nil, fmt.Errorf("exit #%d missing or disabled", id)
		}
		if e != nil {
			return nil, errors.New("cannot read exit")
		}
		if a.box == nil {
			return nil, errors.New("configure LUMA_MASTER_KEY before assigning exits")
		}
		raw, e := a.box.Open(cipher, "exit:"+sid)
		var p proxy.Profile
		if e != nil || json.Unmarshal(raw, &p) != nil {
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

func (a *API) saveNodeRouting(c *gin.Context, tx pgx.Tx, id int64, r *proxy.Routes, config []byte) bool {
	if r == nil {
		return true
	}
	if _, err := r.Compile(); err != nil {
		fail(c, 400, err.Error())
		return false
	}
	if !a.lockConfiguredExits(c, tx, *r) {
		return false
	}
	if len(r.ExitIDs()) > 0 {
		if !a.requireBox(c) {
			return false
		}
		var cfg map[string]any
		if err := json.Unmarshal(config, &cfg); err != nil {
			fail(c, 400, "invalid node configuration")
			return false
		}
		if _, err := a.compileNodeExitsWith(c.Request.Context(), tx, *r, cfg); err != nil {
			fail(c, 400, err.Error())
			return false
		}
	}
	raw, _ := json.Marshal(r)
	if _, err := tx.Exec(c.Request.Context(), `INSERT INTO node_exit_routes(node_id,config) VALUES($1,$2) ON CONFLICT(node_id) DO UPDATE SET config=excluded.config,updated_at=now()`, id, raw); err != nil {
		dbFailure(c, err)
		return false
	}
	return true
}

func (a *API) lockConfiguredExits(c *gin.Context, tx pgx.Tx, r proxy.Routes) bool {
	for _, id := range r.ConfiguredExitIDs() {
		var locked int64
		err := tx.QueryRow(c.Request.Context(), `SELECT id FROM exits WHERE id=$1 FOR SHARE`, id).Scan(&locked)
		if err == pgx.ErrNoRows {
			fail(c, 400, fmt.Sprintf("落地 #%d 已不存在，请重新选择。", id))
			return false
		}
		if err != nil {
			internal(c, err)
			return false
		}
	}
	return true
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
	if r.Global != nil {
		merged := []any{}
		for _, rule := range rules {
			merged = append(merged, rule)
		}
		if !*r.Global {
			if old, ok := cfg["custom_route_rules"].([]any); ok {
				merged = append(merged, old...)
			}
		}
		cfg["custom_route_rules"] = merged
		return true
	}
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
