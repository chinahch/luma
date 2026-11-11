package httpapi

import (
	"encoding/csv"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type UsageRow struct {
	Bucket  string `json:"bucket"`
	UserID  int64  `json:"user_id"`
	Email   string `json:"user_email"`
	NodeID  int64  `json:"node_id"`
	Node    string `json:"node_name"`
	Up      string `json:"up_bytes"`
	Down    string `json:"down_bytes"`
	Billed  string `json:"billed_bytes"`
	Reports int64  `json:"reports"`
	Legacy  bool   `json:"legacy"`
}
type usageFilter struct {
	userID, nodeID int64
	from, to       time.Time
	zone, grain    string
}

func (a *API) usageFilter(c *gin.Context) (usageFilter, bool) {
	f := usageFilter{zone: c.DefaultQuery("timezone", a.cfg.Timezone), grain: c.DefaultQuery("grain", "day")}
	location, err := time.LoadLocation(f.zone)
	if err != nil {
		fail(c, 400, "invalid IANA timezone")
		return f, false
	}
	if f.grain != "hour" && f.grain != "day" && f.grain != "month" {
		fail(c, 400, "grain must be hour, day or month")
		return f, false
	}
	f.userID, err = parseOptionalID(c.Query("user_id"))
	if err != nil {
		fail(c, 400, "invalid user_id")
		return f, false
	}
	f.nodeID, err = parseOptionalID(c.Query("node_id"))
	if err != nil {
		fail(c, 400, "invalid node_id")
		return f, false
	}
	user := current(c)
	if user.Role != "admin" {
		if f.userID != 0 && f.userID != user.ID {
			fail(c, 403, "only your own usage is available")
			return f, false
		}
		f.userID = user.ID
	}
	now := time.Now().In(location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	f.from = today.AddDate(0, 0, -6)
	f.to = today.AddDate(0, 0, 1)
	if raw := c.Query("from"); raw != "" {
		f.from, err = time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			fail(c, 400, "from must be YYYY-MM-DD")
			return f, false
		}
	}
	if raw := c.Query("to"); raw != "" {
		var day time.Time
		day, err = time.ParseInLocation("2006-01-02", raw, location)
		if err != nil {
			fail(c, 400, "to must be YYYY-MM-DD")
			return f, false
		}
		f.to = day.AddDate(0, 0, 1)
	}
	if !f.to.After(f.from) || f.to.Sub(f.from) > 367*24*time.Hour {
		fail(c, 400, "date range must be 1..366 days")
		return f, false
	}
	return f, true
}
func (a *API) loadUsage(c *gin.Context, f usageFilter) ([]UsageRow, bool, error) {
	rows, err := a.s.Pool.Query(c.Request.Context(), `SELECT date_trunc($1::text,r.period_end AT TIME ZONE $2::text)::text,u.id,u.email,n.id,n.name,SUM(e.up_bytes)::text,SUM(e.down_bytes)::text,SUM(e.billed_bytes)::text,count(*),bool_or(r.quality='legacy') FROM usage_entries e JOIN usage_reports r ON r.id=e.report_id JOIN users u ON u.id=e.user_id JOIN nodes n ON n.id=r.node_id WHERE r.period_end >= $3 AND r.period_end < $4 AND ($5::bigint=0 OR u.id=$5) AND ($6::bigint=0 OR n.id=$6) GROUP BY 1,u.id,u.email,n.id,n.name ORDER BY 1 DESC,n.id,u.id LIMIT 2001`, f.grain, f.zone, f.from, f.to, f.userID, f.nodeID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := []UsageRow{}
	for rows.Next() {
		var r UsageRow
		if err := rows.Scan(&r.Bucket, &r.UserID, &r.Email, &r.NodeID, &r.Node, &r.Up, &r.Down, &r.Billed, &r.Reports, &r.Legacy); err != nil {
			return nil, false, err
		}
		result = append(result, r)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(result) > 2000
	if truncated {
		result = result[:2000]
	}
	return result, truncated, nil
}
func (a *API) usage(c *gin.Context) {
	f, ok := a.usageFilter(c)
	if !ok {
		return
	}
	rows, truncated, err := a.loadUsage(c, f)
	if err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"rows": rows, "timezone": f.zone, "grain": f.grain, "from": f.from, "to_exclusive": f.to, "updated_at": time.Now().UTC(), "truncated": truncated, "attribution": "report_period_end", "legacy_notice": "legacy windows use server receipt time and may contain retry duplicates"})
}
func (a *API) exportUsage(c *gin.Context) {
	f, ok := a.usageFilter(c)
	if !ok {
		return
	}
	rows, truncated, err := a.loadUsage(c, f)
	if err != nil {
		internal(c, err)
		return
	}
	if truncated {
		fail(c, 413, "more than 2000 groups; narrow the range before exporting")
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="luma-usage.csv"`)
	c.Writer.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(c.Writer)
	writer.Write([]string{"period_end_bucket", "timezone", "user_id", "email", "node_id", "node", "up_bytes", "down_bytes", "billed_bytes", "reports", "contains_legacy"})
	for _, row := range rows {
		writer.Write([]string{row.Bucket, f.zone, strconv.FormatInt(row.UserID, 10), csvCell(row.Email), strconv.FormatInt(row.NodeID, 10), csvCell(row.Node), row.Up, row.Down, row.Billed, strconv.FormatInt(row.Reports, 10), strconv.FormatBool(row.Legacy)})
	}
	writer.Flush()
}

// Spreadsheet apps must not execute user-controlled names as formulas.
func csvCell(value string) string {
	if len(value) > 0 {
		switch value[0] {
		case '=', '+', '-', '@', '\t', '\r', '\n':
			return "'" + value
		}
	}
	return value
}
func (a *API) dashboard(c *gin.Context) {
	user := current(c)
	filterID := int64(0)
	if user.Role != "admin" {
		filterID = user.ID
	}
	a.jsonQuery(c, `SELECT jsonb_build_object('total_users',CASE WHEN $1::bigint=0 THEN (SELECT count(*) FROM users) ELSE 1 END,'total_nodes',(SELECT count(*) FROM nodes WHERE active),'online_nodes',(SELECT count(*) FROM nodes WHERE active AND last_report_at>now()-make_interval(secs=>report_interval*3)),'up_bytes',(SELECT COALESCE(sum(e.up_bytes),0)::text FROM usage_entries e WHERE $1::bigint=0 OR e.user_id=$1),'down_bytes',(SELECT COALESCE(sum(e.down_bytes),0)::text FROM usage_entries e WHERE $1::bigint=0 OR e.user_id=$1),'billed_bytes',(SELECT COALESCE(sum(e.billed_bytes),0)::text FROM usage_entries e WHERE $1::bigint=0 OR e.user_id=$1),'legacy_reports',(SELECT count(*) FROM usage_reports r WHERE r.quality='legacy' AND ($1::bigint=0 OR EXISTS(SELECT 1 FROM usage_entries e WHERE e.report_id=r.id AND e.user_id=$1))),'updated_at',now())`, filterID)
}
