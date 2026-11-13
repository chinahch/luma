package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/proxy"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (a *API) deleteExit(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	tx, err := a.s.Pool.Begin(ctx)
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(ctx)
	var locked int64
	if err = tx.QueryRow(ctx, `SELECT id FROM exits WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err == pgx.ErrNoRows {
		fail(c, 404, "exit not found")
		return
	} else if err != nil {
		internal(c, err)
		return
	}
	rows, err := tx.Query(ctx, `SELECT n.id,n.name FROM node_exit_routes r JOIN nodes n ON n.id=r.node_id WHERE n.deleted_at IS NULL AND (r.config @> jsonb_build_object('default_exit',$1::bigint) OR r.config @> jsonb_build_object('rules',jsonb_build_array(jsonb_build_object('exit_id',$1::bigint)))) ORDER BY n.id LIMIT 6`, id)
	if err != nil {
		internal(c, err)
		return
	}
	names := []string{}
	for rows.Next() {
		var nid int64
		var name string
		if err = rows.Scan(&nid, &name); err != nil {
			rows.Close()
			internal(c, err)
			return
		}
		names = append(names, fmt.Sprintf("%s（#%d）", name, nid))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		internal(c, err)
		return
	}
	if len(names) > 0 {
		suffix := ""
		if len(names) > 5 {
			names = names[:5]
			suffix = "等节点"
		}
		fail(c, 409, "该落地仍被 "+strings.Join(names, "、")+suffix+" 引用，请先在节点的落地与分流中替换或移除。")
		return
	}
	if _, err = tx.Exec(ctx, `DELETE FROM exits WHERE id=$1`, id); err != nil {
		dbFailure(c, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) exits(c *gin.Context) {
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT id,name,protocol,server,port,active,created_at,updated_at FROM exits ORDER BY id DESC)t`)
}
func (a *API) parseExits(c *gin.Context) {
	var in struct {
		Links string `json:"links"`
	}
	if !decode(c, &in) {
		return
	}
	p, issues, e := proxy.ParseBatch(in.Links)
	if e != nil {
		fail(c, 400, e.Error())
		return
	}
	c.JSON(200, gin.H{"profiles": p, "issues": issues, "storage_ready": a.box != nil})
}
func (a *API) requireBox(c *gin.Context) bool {
	if a.box == nil {
		fail(c, 503, "configure LUMA_MASTER_KEY before storing or reading credentials")
		return false
	}
	return true
}
func validateExit(p proxy.Profile) error {
	if e := p.Validate(); e != nil {
		return e
	}
	_, x := p.Outbound("xray", "validation")
	_, s := p.Outbound("singbox", "validation")
	if x != nil && s != nil {
		return x
	}
	return nil
}
func (a *API) createExits(c *gin.Context) {
	if !a.requireBox(c) {
		return
	}
	var in struct {
		Profiles []proxy.Profile `json:"profiles"`
	}
	if !decode(c, &in) {
		return
	}
	if len(in.Profiles) == 0 || len(in.Profiles) > 200 {
		fail(c, 400, "provide 1..200 profiles")
		return
	}
	for _, p := range in.Profiles {
		if e := validateExit(p); e != nil {
			fail(c, 400, e.Error())
			return
		}
	}
	ctx := c.Request.Context()
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	ids := []int64{}
	for _, p := range in.Profiles {
		sid := store.NewID()
		raw, _ := json.Marshal(p)
		encrypted, e := a.box.Seal(raw, "exit:"+sid)
		if e != nil {
			internal(c, e)
			return
		}
		var id int64
		e = tx.QueryRow(ctx, `INSERT INTO exits(secret_id,name,protocol,server,port,encrypted_profile) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, sid, p.Name, p.Protocol, p.Server, p.Port, encrypted).Scan(&id)
		if e != nil {
			dbFailure(c, e)
			return
		}
		ids = append(ids, id)
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	c.JSON(201, gin.H{"ids": ids})
}
func (a *API) loadExit(c *gin.Context, id int64) (proxy.Profile, bool) {
	var p proxy.Profile
	var sid string
	var cipher []byte
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT secret_id::text,encrypted_profile FROM exits WHERE id=$1`, id).Scan(&sid, &cipher)
	if e == pgx.ErrNoRows {
		fail(c, 404, "exit not found")
		return p, false
	}
	if e != nil {
		internal(c, e)
		return p, false
	}
	raw, e := a.box.Open(cipher, "exit:"+sid)
	if e != nil {
		fail(c, 503, "cannot decrypt exit; verify LUMA_MASTER_KEY")
		return p, false
	}
	if json.Unmarshal(raw, &p) != nil {
		fail(c, 500, "invalid stored exit")
		return p, false
	}
	return p, true
}
func (a *API) exitDetail(c *gin.Context) {
	if !a.requireBox(c) {
		return
	}
	id, ok := positiveID(c)
	if !ok {
		return
	}
	p, ok := a.loadExit(c, id)
	if ok {
		c.JSON(200, p)
	}
}
func (a *API) updateExit(c *gin.Context) {
	if !a.requireBox(c) {
		return
	}
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct {
		Profile *proxy.Profile `json:"profile"`
		Active  *bool          `json:"active"`
	}
	if !decode(c, &in) {
		return
	}
	if in.Profile == nil && in.Active == nil {
		fail(c, 400, "profile or active required")
		return
	}
	if in.Profile != nil {
		if e := validateExit(*in.Profile); e != nil {
			fail(c, 400, e.Error())
			return
		}
	}
	ctx := c.Request.Context()
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	var sid string
	e = tx.QueryRow(ctx, `SELECT secret_id::text FROM exits WHERE id=$1 FOR UPDATE`, id).Scan(&sid)
	if e == pgx.ErrNoRows {
		fail(c, 404, "exit not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	if in.Profile != nil {
		p := in.Profile
		raw, _ := json.Marshal(p)
		cipher, e := a.box.Seal(raw, "exit:"+sid)
		if e != nil {
			internal(c, e)
			return
		}
		if _, e = tx.Exec(ctx, `UPDATE exits SET name=$2,protocol=$3,server=$4,port=$5,encrypted_profile=$6,updated_at=now() WHERE id=$1`, id, p.Name, p.Protocol, p.Server, p.Port, cipher); e != nil {
			dbFailure(c, e)
			return
		}
	}
	if in.Active != nil {
		if _, e = tx.Exec(ctx, `UPDATE exits SET active=$2,updated_at=now() WHERE id=$1`, id, *in.Active); e != nil {
			internal(c, e)
			return
		}
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
