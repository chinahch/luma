package httpapi

import (
	"encoding/json"
	"fmt"
	"github.com/chinahch/luma/internal/protocol"
	"github.com/chinahch/luma/internal/proxy"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

const probeTarget = "https://www.gstatic.com/generate_204"

type probePayload struct {
	Action    string  `json:"action,omitempty"`
	NodeID    int64   `json:"node_id,omitempty"`
	Port      int     `json:"expected_port,omitempty"`
	Transport string  `json:"transport,omitempty"`
	Users     int     `json:"eligible_users"`
	Active    bool    `json:"active"`
	Protocol  string  `json:"protocol,omitempty"`
	Kernel    string  `json:"kernel_type"`
	Outbound  proxy.M `json:"outbound"`
	Target    string  `json:"target"`
	PingHost  string  `json:"ping_host,omitempty"`
}
type probeResult struct {
	ID        string     `json:"id"`
	State     string     `json:"state"`
	Latency   *int       `json:"latency_ms"`
	Error     string     `json:"error_code"`
	Created   time.Time  `json:"created_at"`
	Finished  *time.Time `json:"finished_at"`
	Source    string     `json:"source"`
	Ping      *float64   `json:"ping_ms"`
	PingError string     `json:"ping_error_code"`
}

const probeColumns = `id::text,state,latency_ms,error_code,created_at,finished_at,source,ping_ms,ping_error_code`

func scanProbe(row pgx.Row) (p probeResult, e error) {
	e = row.Scan(&p.ID, &p.State, &p.Latency, &p.Error, &p.Created, &p.Finished, &p.Source, &p.Ping, &p.PingError)
	return
}
func (a *API) registerProbeRoutes(r *gin.Engine, admin *gin.RouterGroup) {
	r.GET("/api/me/nodes", a.auth(), a.probeNodes)
	r.POST("/api/me/nodes/:id/probes", a.auth(), a.createNodeProbe)
	r.GET("/api/me/probes/:id", a.auth(), a.getProbe)
	admin.POST("/exits/:id/probes", a.createExitProbe)
	admin.POST("/nodes/:id/diagnostics", a.createDiagnostic)
	admin.GET("/probes/:id", a.getAdminProbe)
	r.GET("/api/v3/server/probes", a.nodeAuth(), a.leaseProbes)
	r.POST("/api/v3/server/probes/:id/result", a.nodeAuth(), a.finishProbe)
}
func (a *API) expireProbes(c *gin.Context) error {
	_, e := a.s.Pool.Exec(c.Request.Context(), `UPDATE proxy_probes SET state='expired',error_code='expired',finished_at=now(),encrypted_payload=NULL,lease_hash=NULL WHERE state IN ('queued','running') AND expires_at<=now()`)
	return e
}
func (a *API) getProbe(c *gin.Context)      { a.readProbe(c, false) }
func (a *API) getAdminProbe(c *gin.Context) { a.readProbe(c, true) }
func (a *API) readProbe(c *gin.Context, admin bool) {
	if e := a.expireProbes(c); e != nil {
		internal(c, e)
		return
	}
	id := c.Param("id")
	if !validProbeID(id) {
		fail(c, 404, "probe not found")
		return
	}
	p, e := scanProbe(a.s.Pool.QueryRow(c.Request.Context(), `SELECT `+probeColumns+` FROM proxy_probes WHERE id=$1 AND ($2 OR user_id=$3)`, id, admin, current(c).ID))
	if e == pgx.ErrNoRows {
		fail(c, 404, "probe not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	if admin {
		var diagnostic json.RawMessage
		if e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT diagnostics FROM proxy_probes WHERE id=$1`, id).Scan(&diagnostic); e != nil {
			internal(c, e)
			return
		}
		if len(diagnostic) > 0 {
			c.JSON(200, struct {
				probeResult
				Diagnostics json.RawMessage `json:"diagnostics"`
			}{p, diagnostic})
			return
		}
	}
	c.JSON(200, p)
}
func validProbeID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, b := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if b != '-' {
				return false
			}
		} else if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
func (a *API) createNodeProbe(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct{}
	if !decode(c, &in) {
		return
	}
	allowed, e := a.nodeAllowed(c.Request.Context(), current(c).ID, id)
	if e != nil {
		internal(c, e)
		return
	}
	if !allowed {
		fail(c, 404, "node unavailable for this account")
		return
	}
	var mid *int64
	var raw []byte
	var host, name, uuid string
	e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT n.machine_id,n.config,n.public_host,n.name,u.uuid::text FROM nodes n CROSS JOIN users u WHERE n.id=$1 AND u.id=$2`, id, current(c).ID).Scan(&mid, &raw, &host, &name, &uuid)
	if e != nil {
		internal(c, e)
		return
	}
	if mid == nil {
		fail(c, 409, "node has no machine")
		return
	}
	raw, ok = a.fullNodeConfig(c, id, raw)
	if !ok {
		return
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		fail(c, 409, "invalid node configuration")
		return
	}
	kernel, _ := cfg["kernel_type"].(string)
	if kernel == "" {
		kernel = "singbox"
	}
	if cfg["protocol"] == "mtproxy" {
		con, _ := cfg["connector"].(map[string]any)
		owner, _ := con["user_id"].(float64)
		if int64(owner) != current(c).ID {
			fail(c, 404, "node unavailable for this account")
			return
		}
		a.enqueueProbe(c, *mid, &id, "node", probePayload{}, "connector_only")
		return
	}
	p, e := protocol.Client(uuid, name, host, cfg)
	if e != nil {
		fail(c, 409, "node client configuration unsupported")
		return
	}
	out, e := p.Outbound(kernel, "probe")
	if e != nil {
		fail(c, 409, "node client configuration unsupported")
		return
	}
	a.enqueueProbe(c, *mid, &id, "node", probePayload{Kernel: kernel, Outbound: out, Target: probeTarget}, "")
}
func (a *API) createExitProbe(c *gin.Context) {
	id, e := parseOptionalID(c.Param("id"))
	if e != nil {
		fail(c, 400, "invalid exit id")
		return
	}
	var in struct {
		Machine int64  `json:"machine_id"`
		Kernel  string `json:"kernel_type"`
	}
	if !decode(c, &in) {
		return
	}
	if in.Machine <= 0 || (in.Kernel != "xray" && in.Kernel != "singbox") {
		fail(c, 400, "machine_id and xray or singbox kernel required")
		return
	}
	out := proxy.M{"tag": "probe", "protocol": "direct", "settings": proxy.M{}}
	pingHost := ""
	if id > 0 {
		if !a.requireBox(c) {
			return
		}
		var active bool
		e = a.s.Pool.QueryRow(c.Request.Context(), `SELECT active FROM exits WHERE id=$1`, id).Scan(&active)
		if e == pgx.ErrNoRows || e == nil && !active {
			fail(c, 404, "exit unavailable")
			return
		}
		if e != nil {
			internal(c, e)
			return
		}
		p, ok := a.loadExit(c, id)
		if !ok {
			return
		}
		pingHost = p.Server
		out, e = p.Outbound(in.Kernel, "probe")
		if e != nil {
			fail(c, 400, "exit incompatible with selected kernel")
			return
		}
	}
	a.enqueueProbe(c, in.Machine, nil, "exit", probePayload{Kernel: in.Kernel, Outbound: out, Target: probeTarget, PingHost: pingHost}, "")
}
func (a *API) enqueueProbe(c *gin.Context, mid int64, nid *int64, kind string, payload probePayload, unsupported string) {
	if !a.requireBox(c) {
		return
	}
	ctx := c.Request.Context()
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	// Serialize both account and machine limits even across multiple panel processes.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, current(c).ID); e != nil {
		internal(c, e)
		return
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT true FROM machines WHERE id=$1 FOR UPDATE`, mid).Scan(&exists); e == pgx.ErrNoRows {
		fail(c, 404, "machine not found")
		return
	} else if e != nil {
		internal(c, e)
		return
	}
	var accountCount, machineCount, pending int
	e = tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE user_id=$1 AND created_at>now()-interval '1 minute'),count(*) FILTER(WHERE machine_id=$2 AND created_at>now()-interval '1 minute'),count(*) FILTER(WHERE machine_id=$2 AND state IN ('queued','running') AND expires_at>now()) FROM proxy_probes WHERE user_id=$1 OR machine_id=$2`, current(c).ID, mid).Scan(&accountCount, &machineCount, &pending)
	if e != nil {
		internal(c, e)
		return
	}
	if accountCount >= 4 || machineCount >= 20 || pending >= 4 {
		fail(c, 429, "probe rate or concurrency limit reached")
		return
	}
	var supported bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM machine_probe_capabilities WHERE machine_id=$1 AND last_seen_at>now()-interval '2 minutes')`, mid).Scan(&supported)
	if e != nil {
		internal(c, e)
		return
	}
	state := "queued"
	var finished *time.Time
	if !supported && unsupported == "" {
		var online bool
		if e = tx.QueryRow(ctx, `SELECT COALESCE(last_report_at>now()-interval '2 minutes',false) FROM machines WHERE id=$1`, mid).Scan(&online); e != nil {
			internal(c, e)
			return
		}
		unsupported = "machine_unavailable"
		if online {
			unsupported = "upgrade_required"
		}
	}
	if unsupported != "" {
		state = "unsupported"
		now := time.Now().UTC()
		finished = &now
	}
	id := store.NewID()
	raw, _ := json.Marshal(payload)
	cipher, e := a.box.Seal(raw, "probe:"+id)
	if e != nil {
		fail(c, 503, "probe encryption unavailable")
		return
	}
	if state != "queued" {
		cipher = nil
	}
	result, e := scanProbe(tx.QueryRow(ctx, `INSERT INTO proxy_probes(id,user_id,node_id,machine_id,kind,state,encrypted_payload,error_code,finished_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now()+interval '2 minutes') RETURNING `+probeColumns, id, current(c).ID, nid, mid, kind, state, cipher, unsupported, finished))
	if e != nil {
		internal(c, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	c.JSON(201, result)
}
func probeMachine(c *gin.Context) (int64, bool) {
	i := c.MustGet("node_identity").(nodeIdentity)
	if i.MachineID <= 0 || i.NodeID != 0 {
		fail(c, 403, "machine identity required")
		return 0, false
	}
	return i.MachineID, true
}
func (a *API) leaseProbes(c *gin.Context) {
	mid, ok := probeMachine(c)
	if !ok {
		return
	}
	if !a.requireBox(c) {
		return
	}
	ctx := c.Request.Context()
	if e := a.expireProbes(c); e != nil {
		internal(c, e)
		return
	}
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT id FROM machines WHERE id=$1 FOR UPDATE`, mid); e != nil {
		internal(c, e)
		return
	}
	if _, e = tx.Exec(ctx, `INSERT INTO machine_probe_capabilities(machine_id) VALUES($1) ON CONFLICT(machine_id) DO UPDATE SET last_seen_at=now()`, mid); e != nil {
		internal(c, e)
		return
	}
	if a.cfg.AutomaticHealth {
		if e = a.scheduleMachineHealth(ctx, tx, mid); e != nil {
			internal(c, e)
			return
		}
	}
	tasks := []gin.H{}
	var running bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM proxy_probes WHERE machine_id=$1 AND state='running' AND expires_at>now())`, mid).Scan(&running)
	if e != nil {
		internal(c, e)
		return
	}
	if !running {
		var id string
		var uid *int64
		var nid *int64
		var cipher []byte
		var expires time.Time
		var kind string
		e = tx.QueryRow(ctx, `SELECT id::text,user_id,node_id,encrypted_payload,expires_at,kind FROM proxy_probes WHERE machine_id=$1 AND state='queued' AND expires_at>now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`, mid).Scan(&id, &uid, &nid, &cipher, &expires, &kind)
		if e != nil && e != pgx.ErrNoRows {
			internal(c, e)
			return
		}
		if e == nil {
			allowed := true
			if nid != nil && kind != "diagnostic" {
				e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u CROSS JOIN nodes n WHERE u.id=$1 AND n.id=$2 AND `+entitlementSQL+`)`, uid, *nid).Scan(&allowed)
				if e != nil {
					internal(c, e)
					return
				}
				var same bool
				e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id=$1 AND machine_id=$2 AND active)`, *nid, mid).Scan(&same)
				if e != nil {
					internal(c, e)
					return
				}
				allowed = allowed && same
			}
			if kind == "diagnostic" {
				e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id=$1 AND machine_id=$2)`, nid, mid).Scan(&allowed)
				if e != nil {
					internal(c, e)
					return
				}
			}
			raw, decErr := a.box.Open(cipher, "probe:"+id)
			if !allowed || decErr != nil {
				code := "account_unavailable"
				if decErr != nil {
					code = "configuration_unavailable"
				}
				_, e = tx.Exec(ctx, `UPDATE proxy_probes SET state='failed',error_code=$2,encrypted_payload=NULL,finished_at=now() WHERE id=$1`, id, code)
			} else {
				lease := store.NewID()
				_, e = tx.Exec(ctx, `UPDATE proxy_probes SET state='running',lease_hash=$2 WHERE id=$1`, id, store.Hash(lease))
				tasks = append(tasks, gin.H{"id": id, "lease": lease, "expires_at": expires, "timeout_ms": 10000, "payload": json.RawMessage(raw)})
			}
			if e != nil {
				internal(c, e)
				return
			}
		}
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	if a.cfg.AutomaticHealth {
		a.refreshMachineCountry(ctx, mid)
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

var probeErrors = map[string]bool{"": true, "proxy_failed": true, "target_failed": true, "timeout": true, "configuration_unavailable": true, "unsupported": true, "expired": true, "connector_only": true}

func (a *API) finishProbe(c *gin.Context) {
	mid, ok := probeMachine(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validProbeID(id) {
		fail(c, 404, "probe not found")
		return
	}
	var in struct {
		Token       string          `json:"token"`
		Machine     int64           `json:"machine_id"`
		Lease       string          `json:"lease"`
		State       string          `json:"state"`
		Latency     int             `json:"latency_ms"`
		Error       string          `json:"error_code"`
		Diagnostics json.RawMessage `json:"diagnostics"`
		Ping        *float64        `json:"ping_ms"`
		PingError   string          `json:"ping_error_code"`
	}
	raw := c.MustGet("node_raw").([]byte)
	if json.Unmarshal(raw, &in) != nil || len(in.Lease) > 100 || in.Latency < 0 || in.Latency > 15000 || !probeErrors[in.Error] || (in.State != "succeeded" && in.State != "failed" && in.State != "unsupported" && in.State != "expired") || (in.State == "succeeded" && in.Error != "") {
		fail(c, 400, "invalid probe result")
		return
	}
	if (in.Ping != nil && (*in.Ping < 0 || *in.Ping > 15000 || in.State != "succeeded" || in.PingError != "")) || (in.PingError != "" && in.PingError != "icmp_timeout" && in.PingError != "icmp_unavailable" && in.PingError != "icmp_resolve_failed") {
		fail(c, 400, "invalid ICMP result")
		return
	}
	var diagnostic json.RawMessage
	if len(in.Diagnostics) > 0 {
		var nid *int64
		var kind string
		e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT node_id,kind FROM proxy_probes WHERE id=$1 AND machine_id=$2`, id, mid).Scan(&nid, &kind)
		if e != nil || nid == nil || kind != "diagnostic" {
			fail(c, 409, "probe lease unavailable")
			return
		}
		var valid bool
		diagnostic, valid = sanitizeDiagnostic(in.Diagnostics, *nid)
		if !valid {
			fail(c, 400, "invalid diagnostics")
			return
		}
	}
	tag, e := a.s.Pool.Exec(c.Request.Context(), `UPDATE proxy_probes SET state=$4,latency_ms=$5,error_code=$6,diagnostics=$7,ping_ms=$8,ping_error_code=$9,finished_at=now(),encrypted_payload=NULL,lease_hash=NULL WHERE id=$1 AND machine_id=$2 AND lease_hash=$3 AND state='running' AND expires_at>now()`, id, mid, store.Hash(in.Lease), in.State, in.Latency, in.Error, diagnostic, in.Ping, in.PingError)
	if e != nil {
		internal(c, e)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 409, "probe lease unavailable")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) probeNodes(c *gin.Context) {
	ctx := c.Request.Context()
	rows, e := a.s.Pool.Query(ctx, `SELECT n.id,n.name,n.active,n.last_report_at,EXISTS(SELECT 1 FROM machine_probe_capabilities m WHERE m.machine_id=n.machine_id AND m.last_seen_at>now()-interval '2 minutes'),n.config->>'protocol',COALESCE(n.status->'connector'->>'state',''),COALESCE(n.status->'connector'->>'kind',''),COALESCE(n.status->'connector'->>'seen_at','') FROM nodes n WHERE n.active AND (n.config->>'protocol' IS DISTINCT FROM 'mtproxy' OR n.config->'connector'->>'user_id'=$1) ORDER BY n.id`, fmt.Sprint(current(c).ID))
	if e != nil {
		internal(c, e)
		return
	}
	type connectorStatus struct {
		Kind  string `json:"kind"`
		State string `json:"state"`
		Seen  string `json:"seen_at"`
	}
	type node struct {
		ID          int64            `json:"id"`
		Name        string           `json:"name"`
		Active      bool             `json:"active"`
		Last        *time.Time       `json:"last_report_at"`
		MachineLast *time.Time       `json:"machine_last_report_at"`
		Supported   bool             `json:"probe_supported"`
		Latest      *probeResult     `json:"latest_probe"`
		Protocol    string           `json:"protocol"`
		Connector   *connectorStatus `json:"connector_status,omitempty"`
	}
	list := []node{}
	for rows.Next() {
		var n node
		var state, kind, seen string
		if e = rows.Scan(&n.ID, &n.Name, &n.Active, &n.Last, &n.Supported, &n.Protocol, &state, &kind, &seen); e != nil {
			rows.Close()
			internal(c, e)
			return
		}
		if kind == "mtg" && (state == "ready" || state == "starting" || state == "unhealthy" || state == "stopped" || state == "queue_blocked") {
			if _, err := time.Parse(time.RFC3339Nano, seen); err == nil {
				n.Connector = &connectorStatus{kind, state, seen}
			}
		}
		if n.Protocol == "mtproxy" {
			n.Supported = false
		}
		list = append(list, n)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		internal(c, e)
		return
	}
	if e = a.expireProbes(c); e != nil {
		internal(c, e)
		return
	}
	result := []node{}
	for _, n := range list {
		allowed, e := a.nodeAllowed(ctx, current(c).ID, n.ID)
		if e != nil {
			internal(c, e)
			return
		}
		if !allowed {
			continue
		}
		if e = a.s.Pool.QueryRow(ctx, `SELECT m.last_report_at FROM nodes n LEFT JOIN machines m ON m.id=n.machine_id WHERE n.id=$1`, n.ID).Scan(&n.MachineLast); e != nil {
			internal(c, e)
			return
		}
		query := `SELECT ` + probeColumns + ` FROM proxy_probes WHERE node_id=$1 AND user_id=$2 AND kind='node' ORDER BY created_at DESC LIMIT 1`
		if a.cfg.AutomaticHealth {
			query = `SELECT ` + probeColumns + ` FROM proxy_probes WHERE node_id=$1 AND machine_id=(SELECT machine_id FROM nodes WHERE id=$1) AND automatic AND kind='node' AND $2::bigint>0 ORDER BY created_at DESC LIMIT 1`
		}
		p, e := scanProbe(a.s.Pool.QueryRow(ctx, query, n.ID, current(c).ID))
		if e != nil && e != pgx.ErrNoRows {
			internal(c, e)
			return
		}
		if e == nil {
			n.Latest = &p
		}
		result = append(result, n)
	}
	c.JSON(200, result)
}
