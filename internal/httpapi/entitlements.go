package httpapi

import (
	"context"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// entitlementSQL is shared by native user distribution and individual access checks.
// Callers bind users as u and nodes as n. Roles control management, never quota.
const entitlementSQL = `u.active AND u.deleted_at IS NULL AND u.used_bytes<u.quota_bytes AND (u.expires_at IS NULL OR u.expires_at>now()) AND n.active AND n.deleted_at IS NULL AND (NOT EXISTS(SELECT 1 FROM node_plans p WHERE p.node_id=n.id) OR EXISTS(SELECT 1 FROM node_plans p WHERE p.node_id=n.id AND p.plan_id=u.plan_id))`

func (a *API) nodeAllowed(ctx context.Context, userID, nodeID int64) (bool, error) {
	var allowed bool
	err := a.s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users u CROSS JOIN nodes n WHERE u.id=$1 AND n.id=$2 AND `+entitlementSQL+`)`, userID, nodeID).Scan(&allowed)
	return allowed, err
}

func (a *API) saveNodePlans(c *gin.Context, tx pgx.Tx, nodeID int64, planIDs *[]int64) bool {
	if planIDs == nil {
		return true
	}
	if len(*planIDs) > 2000 {
		fail(c, 400, "too many plans")
		return false
	}
	seen := map[int64]bool{}
	for _, id := range *planIDs {
		if id <= 0 || seen[id] {
			fail(c, 400, "invalid or duplicate plan")
			return false
		}
		seen[id] = true
	}
	ctx := c.Request.Context()
	if _, err := tx.Exec(ctx, `DELETE FROM node_plans WHERE node_id=$1`, nodeID); err != nil {
		internal(c, err)
		return false
	}
	for _, id := range *planIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO node_plans(node_id,plan_id) VALUES($1,$2)`, nodeID, id); err != nil {
			dbFailure(c, err)
			return false
		}
	}
	return true
}

func (a *API) setNodeActive(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct {
		Active *bool `json:"active"`
	}
	if !decode(c, &in) {
		return
	}
	if in.Active == nil {
		fail(c, 400, "active is required")
		return
	}
	tag, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE nodes SET active=$2 WHERE id=$1 AND deleted_at IS NULL`, id, *in.Active)
	if err != nil {
		dbFailure(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "node not found")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (a *API) deleteNode(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	// Keep foreign-key identities and all historical traffic; revoke distribution.
	tag, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE nodes SET active=false,machine_id=NULL,deleted_at=now(),token_hash=$2 WHERE id=$1 AND deleted_at IS NULL`, id, store.Hash(store.NewToken()))
	if err != nil {
		dbFailure(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "node not found")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
