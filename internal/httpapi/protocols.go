package httpapi

import (
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

var privateFields = map[string]bool{"private_key": true, "privateKey": true, "decryption": true, "server_key": true, "key_content": true, "dns_env": true, "password": true, "obfs_password": true, "obfsPassword": true, "token": true, "secret": true}

func redactConfig(value any) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		private := false
		for k, item := range v {
			if privateFields[k] {
				if item != nil {
					if text, ok := item.(string); !ok || text != "" {
						private = true
					}
				}
				continue
			}
			safe, found := redactConfig(item)
			private = private || found
			out[k] = safe
		}
		return out, private
	case []any:
		out := make([]any, len(v))
		private := false
		for i, item := range v {
			safe, found := redactConfig(item)
			out[i] = safe
			private = private || found
		}
		return out, private
	default:
		return value, false
	}
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
		return public, true
	}
	if e != nil {
		internal(c, e)
		return nil, false
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
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT config FROM nodes WHERE id=$1`, id).Scan(&raw)
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
		c.Data(200, "application/json", raw)
	}
}

// Encrypt legacy node JSON once a master key is supplied. Conversion is atomic
// and existing encrypted records are not rewritten or read with a new key.
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
	rows, e := tx.Query(ctx, `SELECT n.id,n.config FROM nodes n LEFT JOIN node_private p ON p.node_id=n.id WHERE p.node_id IS NULL FOR UPDATE OF n`)
	if e != nil {
		return e
	}
	type entry struct {
		id          int64
		raw, public []byte
	}
	entries := []entry{}
	for rows.Next() {
		var id int64
		var raw []byte
		if e = rows.Scan(&id, &raw); e != nil {
			rows.Close()
			return e
		}
		public, private := publicNodeConfig(raw)
		if private {
			entries = append(entries, entry{id, raw, public})
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range entries {
		cipher, e := box.Seal(v.raw, nodeContext(v.id))
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO node_private(node_id,encrypted_config) VALUES($1,$2)`, v.id, cipher); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE nodes SET config=$2 WHERE id=$1`, v.id, v.public); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
