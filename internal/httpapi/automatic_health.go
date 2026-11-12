package httpapi

import (
	"context"
	"encoding/json"
	"github.com/chinahch/luma/internal/protocol"
	"github.com/chinahch/luma/internal/store"
	"github.com/jackc/pgx/v5"
)

// Called by the authenticated native worker's existing five-second poll. No
// browser request or panel timer is needed. Its machine row is already locked.
func (a *API) scheduleMachineHealth(ctx context.Context, tx pgx.Tx, mid int64) error {
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM proxy_probes WHERE machine_id=$1 AND state IN ('queued','running') AND expires_at>now()`, mid).Scan(&pending); err != nil {
		return err
	}
	if pending >= 4 {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT n.id,n.name,n.public_host,n.config,p.encrypted_config,u.id,u.uuid::text FROM nodes n LEFT JOIN node_private p ON p.node_id=n.id LEFT JOIN LATERAL (SELECT u.id,u.uuid FROM users u WHERE `+entitlementSQL+` AND (n.config->>'protocol' IS DISTINCT FROM 'mtproxy' OR n.config->'connector'->>'user_id'=u.id::text) ORDER BY u.id LIMIT 1) u ON true WHERE n.machine_id=$1 AND n.active AND n.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM proxy_probes h WHERE h.node_id=n.id AND h.machine_id=n.machine_id AND h.automatic AND h.created_at>now()-interval '5 minutes') ORDER BY n.id LIMIT $2`, mid, 4-pending)
	if err != nil {
		return err
	}
	type candidate struct {
		id          int64
		name, host  string
		raw, cipher []byte
		uid         *int64
		uuid        *string
	}
	candidates := []candidate{}
	for rows.Next() {
		var n candidate
		if err = rows.Scan(&n.id, &n.name, &n.host, &n.raw, &n.cipher, &n.uid, &n.uuid); err != nil {
			break
		}
		candidates = append(candidates, n)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, n := range candidates {
		code := ""
		payload := probePayload{}
		if n.uid == nil || n.uuid == nil {
			code = "no_eligible_users"
		} else {
			raw := n.raw
			if len(n.cipher) > 0 {
				raw, err = a.box.Open(n.cipher, nodeContext(n.id))
				if err != nil {
					code = "configuration_unavailable"
				}
			}
			cfg := map[string]any{}
			if json.Unmarshal(raw, &cfg) != nil {
				code = "configuration_unavailable"
			}
			if code == "" {
				if cfg["protocol"] == "mtproxy" {
					code = "connector_only"
				} else {
					kernel, _ := cfg["kernel_type"].(string)
					if kernel == "" {
						kernel = "singbox"
					}
					client, e := protocol.Client(*n.uuid, n.name, n.host, cfg)
					if e != nil {
						code = "configuration_unavailable"
					} else {
						out, e := client.Outbound(kernel, "probe")
						if e != nil {
							code = "configuration_unavailable"
						} else {
							payload = probePayload{Kernel: kernel, Outbound: out, Target: probeTarget}
						}
					}
				}
			}
		}
		id := store.NewID()
		state := "queued"
		var cipher []byte
		if code != "" {
			state = "unsupported"
		} else {
			raw, _ := json.Marshal(payload)
			cipher, err = a.box.Seal(raw, "probe:"+id)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO proxy_probes(id,user_id,node_id,machine_id,kind,state,encrypted_payload,error_code,expires_at,finished_at,automatic,source) VALUES($1,$2,$3,$4,'node',$5,$6,$7,now()+interval '2 minutes',CASE WHEN $5='queued' THEN NULL ELSE now() END,true,'node machine · automatic')`, id, n.uid, n.id, mid, state, cipher, code)
		if err != nil {
			return err
		}
	}
	return nil
}
