package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/protocol"
	"github.com/chinahch/luma/internal/secretbox"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (a *API) protocolPreset(c *gin.Context) {
	var in protocol.Input
	if !decode(c, &in) {
		return
	}
	cfg, e := protocol.Build(in)
	if e != nil {
		fail(c, 400, e.Error())
		return
	}
	c.JSON(200, cfg)
}

func redactConfig(value any) (any, bool) {
	// Lists need only this stable display/discovery projection. Unknown nested
	// kernel dialects (including future credential names) stay private in full.
	v, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}, true
	}
	out := map[string]any{}
	copyPrimitive := func(dst, src map[string]any, keys []string) {
		for _, key := range keys {
			switch x := src[key].(type) {
			case string, bool, float64, json.Number:
				dst[key] = x
			}
		}
	}
	copyPrimitive(out, v, []string{"protocol", "server_port", "listen_ip", "network", "tls", "kernel_type", "kernel_log_level", "preset", "flow", "cipher", "version", "up_mbps", "down_mbps"})
	if connector, ok := v["connector"].(map[string]any); ok {
		safe := map[string]any{}
		copyPrimitive(safe, connector, []string{"kind", "user_id", "metrics_port", "hostname"})
		out["connector"] = safe
	}
	original, _ := json.Marshal(v)
	public, _ := json.Marshal(out)
	return out, !bytes.Equal(original, public)
}
func publicNodeConfig(raw []byte) ([]byte, bool) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return raw, false
	}
	safe, private := redactConfig(value)
	out, _ := json.Marshal(safe)
	return out, private
}
func nodeContext(id int64) string { return fmt.Sprintf("node:%d", id) }
func (a *API) savePrivateNode(c *gin.Context, tx pgx.Tx, id int64, raw []byte) error {
	_, private := publicNodeConfig(raw)
	if !private {
		_, e := tx.Exec(c.Request.Context(), `DELETE FROM node_private WHERE node_id=$1`, id)
		return e
	}
	encrypted, e := a.box.Seal(raw, nodeContext(id))
	if e != nil {
		return e
	}
	_, e = tx.Exec(c.Request.Context(), `INSERT INTO node_private(node_id,encrypted_config) VALUES($1,$2) ON CONFLICT(node_id) DO UPDATE SET encrypted_config=excluded.encrypted_config,updated_at=now()`, id, encrypted)
	return e
}
func (a *API) fullNodeConfig(c *gin.Context, id int64, public []byte) ([]byte, bool) {
	var cipher []byte
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT encrypted_config FROM node_private WHERE node_id=$1`, id).Scan(&cipher)
	if e == pgx.ErrNoRows {
		return a.openNodeConfig(c, id, public, nil)
	}
	if e != nil {
		internal(c, e)
		return nil, false
	}
	return a.openNodeConfig(c, id, public, cipher)
}
func (a *API) openNodeConfig(c *gin.Context, id int64, public, cipher []byte) ([]byte, bool) {
	if len(cipher) == 0 {
		return public, true
	}
	if !a.requireBox(c) {
		return nil, false
	}
	raw, e := a.box.Open(cipher, nodeContext(id))
	if e != nil {
		fail(c, 503, "cannot decrypt node configuration; verify LUMA_MASTER_KEY")
		return nil, false
	}
	return raw, true
}
func (a *API) nodeDetail(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var raw []byte
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT config FROM nodes WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&raw)
	if e == pgx.ErrNoRows {
		fail(c, 404, "node not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	raw, ok = a.fullNodeConfig(c, id, raw)
	if ok {
		var config map[string]any
		if err := json.Unmarshal(raw, &config); err != nil {
			internal(c, err)
			return
		}
		var planIDs []int64 = []int64{}
		rows, err := a.s.Pool.Query(c.Request.Context(), `SELECT plan_id FROM node_plans WHERE node_id=$1 ORDER BY plan_id`, id)
		if err != nil {
			internal(c, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var planID int64
			if err = rows.Scan(&planID); err != nil {
				internal(c, err)
				return
			}
			planIDs = append(planIDs, planID)
		}
		if err = rows.Err(); err != nil {
			internal(c, err)
			return
		}
		config["plan_ids"] = planIDs
		c.JSON(200, config)
	}
}

// Encrypt legacy node JSON once a master key is supplied. Conversion is atomic
// and existing ciphertext is preserved while stale public projections are scrubbed.
func ProtectNodeSecrets(ctx context.Context, s *store.Store, key string) error {
	if key == "" {
		return nil
	}
	box, e := secretbox.New(key)
	if e != nil {
		return e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT n.id,n.config,p.encrypted_config FROM nodes n LEFT JOIN node_private p ON p.node_id=n.id FOR UPDATE OF n`)
	if e != nil {
		return e
	}
	type entry struct {
		id          int64
		raw, public []byte
		existing    bool
	}
	entries := []entry{}
	for rows.Next() {
		var id int64
		var raw, cipher []byte
		if e = rows.Scan(&id, &raw, &cipher); e != nil {
			rows.Close()
			return e
		}
		if len(cipher) > 0 {
			raw, e = box.Open(cipher, nodeContext(id))
			if e != nil {
				rows.Close()
				return e
			}
		}
		public, private := publicNodeConfig(raw)
		if private {
			entries = append(entries, entry{id, raw, public, len(cipher) > 0})
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range entries {
		if !v.existing {
			cipher, e := box.Seal(v.raw, nodeContext(v.id))
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO node_private(node_id,encrypted_config) VALUES($1,$2)`, v.id, cipher); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE nodes SET config=$2 WHERE id=$1`, v.id, v.public); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
