package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/chinahch/luma/internal/traffic"
	"github.com/gin-gonic/gin"
)

type nodeIdentity struct {
	NodeID    int64
	MachineID int64
}

func (a *API) nodeAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string
		var ident nodeIdentity
		if c.Request.Method == "GET" {
			token = c.Query("token")
			var err error
			ident.NodeID, err = parseOptionalID(c.Query("node_id"))
			if err != nil {
				fail(c, 400, "invalid node_id")
				return
			}
			ident.MachineID, err = parseOptionalID(c.Query("machine_id"))
			if err != nil {
				fail(c, 400, "invalid machine_id")
				return
			}
		} else {
			raw, err := io.ReadAll(c.Request.Body)
			if err != nil {
				fail(c, 413, "request too large or unreadable")
				return
			}
			var body map[string]json.RawMessage
			if json.Unmarshal(raw, &body) != nil || body == nil {
				fail(c, 400, "invalid report object")
				return
			}
			var auth struct {
				Token     string `json:"token"`
				NodeID    int64  `json:"node_id"`
				MachineID int64  `json:"machine_id"`
			}
			if json.Unmarshal(raw, &auth) != nil {
				fail(c, 400, "invalid node identity")
				return
			}
			token = auth.Token
			ident = nodeIdentity{auth.NodeID, auth.MachineID}
			c.Set("node_body", body)
			c.Set("node_raw", raw)
		}
		if token == "" {
			token = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}
		if token == "" || len(token) > 256 || ident.NodeID < 0 || ident.MachineID < 0 {
			fail(c, 401, "invalid node credentials")
			return
		}
		ctx := c.Request.Context()
		var exists bool
		if ident.MachineID > 0 {
			err := a.s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM machines WHERE id=$1 AND token_hash=$2)`, ident.MachineID, store.Hash(token)).Scan(&exists)
			if err != nil {
				internal(c, err)
				return
			}
			if !exists {
				fail(c, 401, "invalid machine credentials")
				return
			}
			if ident.NodeID > 0 {
				if err = a.s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id=$1 AND machine_id=$2 AND active)`, ident.NodeID, ident.MachineID).Scan(&exists); err != nil {
					internal(c, err)
					return
				}
				if !exists {
					fail(c, 403, "node does not belong to this machine")
					return
				}
			}
		} else {
			err := a.s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id=$1 AND token_hash=$2 AND active)`, ident.NodeID, store.Hash(token)).Scan(&exists)
			if err != nil {
				internal(c, err)
				return
			}
			if !exists {
				fail(c, 401, "invalid node credentials")
				return
			}
		}
		c.Set("node_identity", ident)
		c.Next()
	}
}
func parseOptionalID(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}
func nodeID(c *gin.Context) (int64, bool) {
	id := c.MustGet("node_identity").(nodeIdentity).NodeID
	if id <= 0 {
		fail(c, 400, "node_id required")
		return 0, false
	}
	return id, true
}
func (a *API) handshake(c *gin.Context) {
	ident := c.MustGet("node_identity").(nodeIdentity)
	interval := 60
	if ident.NodeID > 0 {
		if err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT report_interval FROM nodes WHERE id=$1`, ident.NodeID).Scan(&interval); err != nil {
			internal(c, err)
			return
		}
	}
	c.JSON(200, gin.H{"websocket": gin.H{"enabled": false}, "settings": gin.H{"push_interval": interval, "pull_interval": 60}})
}
func (a *API) nodeConfig(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	var raw []byte
	var interval int
	if err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT config,report_interval FROM nodes WHERE id=$1`, id).Scan(&raw, &interval); err != nil {
		internal(c, err)
		return
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		fail(c, 500, "invalid stored node configuration")
		return
	}
	cfg["base_config"] = gin.H{"push_interval": interval, "pull_interval": 60}
	c.JSON(200, cfg)
}
func (a *API) nodeUsers(c *gin.Context) {
	if _, ok := nodeID(c); !ok {
		return
	}
	a.jsonQuery(c, `SELECT jsonb_build_object('users',COALESCE(jsonb_agg(to_jsonb(t)),'[]')) FROM (SELECT id,uuid,0 AS speed_limit,0 AS device_limit FROM users WHERE active AND role='user' AND used_bytes<quota_bytes AND (expires_at IS NULL OR expires_at>now()) ORDER BY id)t`)
}
func (a *API) machineNodes(c *gin.Context) {
	ident := c.MustGet("node_identity").(nodeIdentity)
	if ident.MachineID <= 0 {
		fail(c, 403, "machine credentials required")
		return
	}
	a.jsonQuery(c, `SELECT jsonb_build_object('nodes',COALESCE(jsonb_agg(to_jsonb(t)),'[]'),'base_config',jsonb_build_object('push_interval',60,'pull_interval',60)) FROM (SELECT id,config->>'protocol' AS type,name FROM nodes WHERE machine_id=$1 AND active ORDER BY id)t`, ident.MachineID)
}
func (a *API) machineStatus(c *gin.Context) {
	ident := c.MustGet("node_identity").(nodeIdentity)
	if ident.MachineID <= 0 {
		fail(c, 403, "machine credentials required")
		return
	}
	body := c.MustGet("node_body").(map[string]json.RawMessage)
	status := map[string]json.RawMessage{}
	for _, key := range []string{"cpu", "mem", "swap", "disk", "net"} {
		if val, ok := body[key]; ok {
			status[key] = val
		}
	}
	raw, _ := json.Marshal(status)
	if _, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE machines SET last_report_at=now(),status=$2 WHERE id=$1`, ident.MachineID, raw); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) reliableReport(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	var report traffic.Report
	if json.Unmarshal(c.MustGet("node_raw").([]byte), &report) != nil {
		fail(c, 400, "invalid report")
		return
	}
	a.ingest(c, id, report, "reliable")
}
func (a *API) legacyReport(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	body := c.MustGet("node_body").(map[string]json.RawMessage)
	a.ingestLegacy(c, id, body["traffic"])
}
func (a *API) legacyPush(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	body := c.MustGet("node_body").(map[string]json.RawMessage)
	for _, key := range []string{"token", "node_id", "machine_id", "node_type"} {
		delete(body, key)
	}
	raw, _ := json.Marshal(body)
	a.ingestLegacy(c, id, raw)
}
func (a *API) ingestLegacy(c *gin.Context, id int64, raw json.RawMessage) {
	values := map[string][]int64{}
	if len(raw) > 0 && string(raw) != "null" {
		if json.Unmarshal(raw, &values) != nil {
			fail(c, 400, "invalid traffic map")
			return
		}
	}
	entries := make([]traffic.Entry, 0, len(values))
	for user, pair := range values {
		uid, err := strconv.ParseInt(user, 10, 64)
		if err != nil || len(pair) != 2 {
			fail(c, 400, "traffic entries require [upload,download]")
			return
		}
		entries = append(entries, traffic.Entry{UserID: uid, Up: pair[0], Down: pair[1]})
	}
	var interval int
	if err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT report_interval FROM nodes WHERE id=$1`, id).Scan(&interval); err != nil {
		internal(c, err)
		return
	}
	now := time.Now().UTC()
	report := traffic.Report{ReportID: "legacy-" + store.NewID(), PeriodStart: now.Add(-time.Duration(interval) * time.Second), PeriodEnd: now, Entries: entries}
	a.ingest(c, id, report, "legacy")
}
func (a *API) ingest(c *gin.Context, id int64, report traffic.Report, quality string) {
	duplicate, err := a.s.Ingest(c.Request.Context(), id, report, quality)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			fail(c, 409, err.Error())
		case errors.Is(err, store.ErrInvalid):
			fail(c, 400, "invalid traffic entries or accounting bounds")
		case errors.Is(err, store.ErrForbidden):
			fail(c, 403, "node not available")
		default:
			internal(c, err)
		}
		return
	}
	if !duplicate {
		body, _ := c.Get("node_body")
		if m, ok := body.(map[string]json.RawMessage); ok {
			if status := m["status"]; len(status) > 0 {
				a.s.Pool.Exec(c.Request.Context(), `UPDATE nodes SET status=$2 WHERE id=$1`, id, status)
			}
		}
		if a.cache != nil {
			raw, _ := json.Marshal(gin.H{"node_id": id, "received_at": time.Now().UTC(), "quality": quality})
			a.cache.Set(c.Request.Context(), "luma:node:"+strconv.FormatInt(id, 10), raw, 5*time.Minute)
		}
	}
	c.JSON(200, gin.H{"ok": true, "duplicate": duplicate, "quality": quality})
}
func (a *API) legacyStatus(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	body := c.MustGet("node_body").(map[string]json.RawMessage)
	status := map[string]json.RawMessage{}
	for _, key := range []string{"cpu", "mem", "swap", "disk"} {
		if val, ok := body[key]; ok {
			status[key] = val
		}
	}
	raw, _ := json.Marshal(status)
	if _, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE nodes SET status=$2 WHERE id=$1`, id, raw); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
