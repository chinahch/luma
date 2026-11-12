package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/chinahch/luma/internal/protocol"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
)

func (a *API) rotateSubscription(c *gin.Context) {
	a.savedSubscription(c, current(c).ID, "rotate", "")
}
func (a *API) subscription(c *gin.Context) {
	if err := a.s.RefreshCycles(c.Request.Context(), time.Now()); err != nil {
		internal(c, err)
		return
	}
	token := c.Param("token")
	if len(token) != 43 {
		fail(c, 404, "subscription not found")
		return
	}
	var uuid string
	var used, quota, userID int64
	var expiry *time.Time
	err := a.s.Pool.QueryRow(c.Request.Context(), `SELECT uuid,used_bytes,quota_bytes,expires_at,id FROM users WHERE subscription_hash=$1 AND active AND deleted_at IS NULL AND used_bytes<quota_bytes AND (expires_at IS NULL OR expires_at>now())`, store.Hash(token)).Scan(&uuid, &used, &quota, &expiry, &userID)
	if err != nil {
		fail(c, 404, "subscription unavailable or quota expired")
		return
	}
	rows, err := a.s.Pool.Query(c.Request.Context(), `SELECT n.id,n.name,n.public_host,n.config,p.encrypted_config FROM nodes n LEFT JOIN node_private p ON p.node_id=n.id WHERE n.active AND n.public_host<>'' AND (NOT EXISTS(SELECT 1 FROM node_plans np WHERE np.node_id=n.id) OR EXISTS(SELECT 1 FROM node_plans np JOIN users u ON u.id=$1 WHERE np.node_id=n.id AND np.plan_id=u.plan_id)) ORDER BY n.id`, userID)
	if err != nil {
		internal(c, err)
		return
	}
	defer rows.Close()
	links := []string{}
	skipped := 0
	for rows.Next() {
		var id int64
		var name, host string
		var raw, cipher []byte
		if err = rows.Scan(&id, &name, &host, &raw, &cipher); err != nil {
			internal(c, err)
			return
		}
		var ok bool
		raw, ok = a.openNodeConfig(c, id, raw, cipher)
		if !ok {
			return
		}
		var cfg map[string]any
		json.Unmarshal(raw, &cfg)
		profile, e := protocol.Client(uuid, name, host, cfg)
		if e != nil {
			skipped++
			continue
		}
		link, e := profile.Link()
		if e != nil {
			skipped++
			continue
		}
		links = append(links, link)
	}
	if rows.Err() != nil {
		internal(c, rows.Err())
		return
	}
	if len(links) == 0 {
		fail(c, 503, "no supported subscription nodes configured")
		return
	}
	info := "upload=0; download=" + strconv.FormatInt(used, 10) + "; total=" + strconv.FormatInt(quota, 10)
	if expiry != nil {
		info += "; expire=" + strconv.FormatInt(expiry.Unix(), 10)
	}
	c.Header("Subscription-Userinfo", info)
	if skipped > 0 {
		c.Header("X-Luma-Skipped-Nodes", strconv.Itoa(skipped))
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="luma.txt"`)
	c.String(200, base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))))
}
func vlessLink(uuid, name, host string, raw []byte) string {
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		return ""
	}
	cfg["protocol"] = "vless"
	p, e := protocol.Client(uuid, name, host, cfg)
	if e != nil {
		return ""
	}
	link, e := p.Link()
	if e != nil {
		return ""
	}
	return link
}
