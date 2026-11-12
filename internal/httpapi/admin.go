package httpapi

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

func positiveID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, 400, "invalid id")
		return 0, false
	}
	return id, true
}
func validName(name string) bool { return len(strings.TrimSpace(name)) > 0 && len(name) <= 100 }
func dbFailure(c *gin.Context, err error) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			fail(c, 409, "record already exists")
			return
		}
		if strings.HasPrefix(pg.Code, "23") {
			fail(c, 400, "invalid record reference or value")
			return
		}
	}
	internal(c, err)
}
func (a *API) jsonQuery(c *gin.Context, query string, args ...any) {
	var data []byte
	if err := a.s.Pool.QueryRow(c.Request.Context(), query, args...).Scan(&data); err != nil {
		internal(c, err)
		return
	}
	c.Data(200, "application/json; charset=utf-8", data)
}
func (a *API) users(c *gin.Context) {
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT id,email,role,active,plan_id,quota_bytes,used_bytes,expires_at,created_at FROM users ORDER BY id DESC LIMIT 2000)t`)
}
func (a *API) plans(c *gin.Context) {
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT * FROM plans ORDER BY id)t`)
}
func (a *API) machines(c *gin.Context) {
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT m.id,m.name,m.last_report_at,m.status,m.created_at,(SELECT count(*) FROM nodes n WHERE n.machine_id=m.id) AS node_count FROM machines m ORDER BY m.id DESC)t`)
}
func (a *API) nodes(c *gin.Context) {
	var raw []byte
	if e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT id,name,machine_id,active,multiplier_milli,report_interval,config,public_host,last_report_at,last_period_end,last_quality,last_up_bytes,last_down_bytes,last_window_seconds,status,created_at FROM nodes ORDER BY id DESC)t`).Scan(&raw); e != nil {
		internal(c, e)
		return
	}
	var nodes []map[string]any
	json.Unmarshal(raw, &nodes)
	for _, n := range nodes {
		safe, _ := redactConfig(n["config"])
		n["config"] = safe
	}
	c.JSON(200, nodes)
}

type userInput struct {
	Email     string     `json:"email"`
	Password  string     `json:"password"`
	Role      string     `json:"role"`
	PlanID    *int64     `json:"plan_id"`
	Quota     int64      `json:"quota_bytes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (a *API) createUser(c *gin.Context) {
	var in userInput
	if !decode(c, &in) {
		return
	}
	email, ok := normalizeEmail(in.Email)
	if in.Role == "" {
		in.Role = "user"
	}
	if !ok || len(in.Password) < 12 || len(in.Password) > 72 || (in.Role != "user" && in.Role != "admin") || in.Quota < 0 {
		fail(c, 400, "valid email, 12..72 byte password, role and nonnegative quota required")
		return
	}
	ctx := c.Request.Context()
	if in.PlanID != nil {
		var days int
		if err := a.s.Pool.QueryRow(ctx, `SELECT quota_bytes,duration_days FROM plans WHERE id=$1`, in.PlanID).Scan(&in.Quota, &days); err != nil {
			fail(c, 400, "plan not found")
			return
		}
		expiry := time.Now().UTC().AddDate(0, 0, days)
		in.ExpiresAt = &expiry
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		internal(c, err)
		return
	}
	var id int64
	err = a.s.Pool.QueryRow(ctx, `INSERT INTO users(email,password_hash,role,uuid,plan_id,quota_bytes,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, email, string(hash), in.Role, store.NewID(), in.PlanID, in.Quota, in.ExpiresAt).Scan(&id)
	if err != nil {
		dbFailure(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id})
}
func (a *API) updateUser(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct {
		Active    *bool        `json:"active"`
		Quota     *int64       `json:"quota_bytes"`
		ExpiresAt optionalTime `json:"expires_at"`
		Password  string       `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	if in.Quota != nil && *in.Quota < 0 {
		fail(c, 400, "quota must be nonnegative")
		return
	}
	ctx := c.Request.Context()
	tx, err := a.s.Pool.Begin(ctx)
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE users SET active=COALESCE($2,active),quota_bytes=COALESCE($3,quota_bytes),expires_at=CASE WHEN $4 THEN $5::timestamptz ELSE expires_at END WHERE id=$1`, id, in.Active, in.Quota, in.ExpiresAt.Set, in.ExpiresAt.Value)
	if err != nil {
		dbFailure(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "user not found")
		return
	}
	if in.Password != "" {
		if len(in.Password) < 12 || len(in.Password) > 72 {
			fail(c, 400, "password must be 12..72 bytes")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			internal(c, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, string(hash)); err != nil {
			internal(c, err)
			return
		}
		if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
			internal(c, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

type planInput struct {
	Name  string `json:"name"`
	Quota int64  `json:"quota_bytes"`
	Days  int    `json:"duration_days"`
	Price int64  `json:"price_cents"`
}

func validPlan(in planInput) bool {
	return validName(in.Name) && in.Quota >= 0 && in.Days >= 1 && in.Days <= 3650 && in.Price >= 0
}
func (a *API) createPlan(c *gin.Context) {
	var in planInput
	if !decode(c, &in) {
		return
	}
	if !validPlan(in) {
		fail(c, 400, "invalid plan")
		return
	}
	var id int64
	err := a.s.Pool.QueryRow(c.Request.Context(), `INSERT INTO plans(name,quota_bytes,duration_days,price_cents) VALUES($1,$2,$3,$4) RETURNING id`, in.Name, in.Quota, in.Days, in.Price).Scan(&id)
	if err != nil {
		dbFailure(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id})
}
func (a *API) updatePlan(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in planInput
	if !decode(c, &in) {
		return
	}
	if !validPlan(in) {
		fail(c, 400, "invalid plan")
		return
	}
	tag, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE plans SET name=$2,quota_bytes=$3,duration_days=$4,price_cents=$5 WHERE id=$1`, id, in.Name, in.Quota, in.Days, in.Price)
	if err != nil {
		dbFailure(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "plan not found")
		return
	}
	c.JSON(200, gin.H{"ok": true, "note": "existing user quotas are unchanged"})
}
func (a *API) createMachine(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(c, &in) {
		return
	}
	if !validName(in.Name) {
		fail(c, 400, "invalid name")
		return
	}
	token := store.NewToken()
	var id int64
	if err := a.s.Pool.QueryRow(c.Request.Context(), `INSERT INTO machines(name,token_hash) VALUES($1,$2) RETURNING id`, in.Name, store.Hash(token)).Scan(&id); err != nil {
		dbFailure(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id, "token": token, "notice": "shown once; store securely"})
}

type nodeInput struct {
	Name       string          `json:"name"`
	MachineID  *int64          `json:"machine_id"`
	Active     *bool           `json:"active"`
	Multiplier int             `json:"multiplier_milli"`
	Interval   int             `json:"report_interval"`
	Config     json.RawMessage `json:"config"`
	PublicHost string          `json:"public_host"`
}

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$`)

func validateNode(in *nodeInput) bool {
	if in.Multiplier == 0 {
		in.Multiplier = 1000
	}
	if in.Interval == 0 {
		in.Interval = 60
	}
	if in.Active == nil {
		active := true
		in.Active = &active
	}
	if len(in.Config) == 0 {
		in.Config = json.RawMessage(`{"protocol":"vless","listen_ip":"0.0.0.0","server_port":443,"network":"tcp","tls":0}`)
	}
	var cfg struct {
		Protocol string `json:"protocol"`
		Port     int    `json:"server_port"`
	}
	if json.Unmarshal(in.Config, &cfg) != nil {
		return false
	}
	protocols := map[string]bool{"mtproxy": true, "socks": true, "http": true, "vless": true, "vmess": true, "trojan": true, "shadowsocks": true, "hysteria": true, "tuic": true, "anytls": true}
	hostOK := in.PublicHost == "" || net.ParseIP(in.PublicHost) != nil || hostPattern.MatchString(in.PublicHost)
	return validName(in.Name) && in.Multiplier >= 1 && in.Multiplier <= 100000 && in.Interval >= 10 && in.Interval <= 3600 && cfg.Port >= 1 && cfg.Port <= 65535 && protocols[cfg.Protocol] && hostOK && len(in.PublicHost) <= 253
}
func (a *API) createNode(c *gin.Context) {
	var in nodeInput
	if !decode(c, &in) {
		return
	}
	if !validateNode(&in) {
		fail(c, 400, "invalid node configuration")
		return
	}
	if !a.validateConnectorOwner(c, in.Config) {
		return
	}
	token := store.NewToken()
	public, private := publicNodeConfig(in.Config)
	if private && !a.requireBox(c) {
		return
	}
	tx, err := a.s.Pool.Begin(c.Request.Context())
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(c.Request.Context())
	var id int64
	err = tx.QueryRow(c.Request.Context(), `INSERT INTO nodes(name,machine_id,active,token_hash,multiplier_milli,report_interval,config,public_host) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, in.Name, in.MachineID, in.Active, store.Hash(token), in.Multiplier, in.Interval, public, in.PublicHost).Scan(&id)
	if err != nil {
		dbFailure(c, err)
		return
	}
	if err = a.savePrivateNode(c, tx, id, in.Config); err != nil {
		internal(c, err)
		return
	}
	if err = tx.Commit(c.Request.Context()); err != nil {
		internal(c, err)
		return
	}
	c.JSON(201, gin.H{"id": id, "token": token, "notice": "shown once; store securely"})
}
func (a *API) updateNode(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in nodeInput
	if !decode(c, &in) {
		return
	}
	if !validateNode(&in) {
		fail(c, 400, "invalid node configuration")
		return
	}
	if !a.validateConnectorOwner(c, in.Config) {
		return
	}
	public, private := publicNodeConfig(in.Config)
	if private && !a.requireBox(c) {
		return
	}
	tx, err := a.s.Pool.Begin(c.Request.Context())
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(c.Request.Context())
	var oldConfig []byte
	if err = tx.QueryRow(c.Request.Context(), `SELECT config FROM nodes WHERE id=$1 FOR UPDATE`, id).Scan(&oldConfig); err != nil {
		fail(c, 404, "node not found")
		return
	}
	var old, next struct {
		Connector connectorConfig `json:"connector"`
	}
	json.Unmarshal(oldConfig, &old)
	json.Unmarshal(in.Config, &next)
	if old.Connector.Kind == "mtg" && (next.Connector.Kind != "mtg" || old.Connector.UserID != next.Connector.UserID) {
		fail(c, 409, "MTG owner is permanent for this node; create a new node for another user")
		return
	}
	tag, err := tx.Exec(c.Request.Context(), `UPDATE nodes SET name=$2,machine_id=$3,active=$4,multiplier_milli=$5,report_interval=$6,config=$7,public_host=$8 WHERE id=$1`, id, in.Name, in.MachineID, in.Active, in.Multiplier, in.Interval, public, in.PublicHost)
	if err != nil {
		dbFailure(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "node not found")
		return
	}
	if err = a.savePrivateNode(c, tx, id, in.Config); err != nil {
		internal(c, err)
		return
	}
	if err = tx.Commit(c.Request.Context()); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
