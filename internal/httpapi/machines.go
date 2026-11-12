package httpapi

import (
	"errors"
	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
)

func (a *API) rotateMachineToken(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	token := store.NewToken()
	tag, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE machines SET token_hash=$2 WHERE id=$1`, id, store.Hash(token))
	if err != nil {
		internal(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "machine not found")
		return
	}
	c.JSON(200, gin.H{"id": id, "token": token, "notice": "shown once; old credential revoked"})
}

func (a *API) deleteMachine(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	// The FK additionally prevents a concurrent association from being orphaned.
	tag, err := a.s.Pool.Exec(c.Request.Context(), `DELETE FROM machines m WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM nodes n WHERE n.machine_id=m.id)`, id)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" {
			fail(c, 409, "机器仍有关联节点或正在被使用，请先迁移关联节点后重试")
		} else {
			internal(c, err)
		}
		return
	}
	if tag.RowsAffected() != 1 {
		var exists bool
		if err = a.s.Pool.QueryRow(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM machines WHERE id=$1)`, id).Scan(&exists); err != nil {
			internal(c, err)
			return
		}
		if exists {
			fail(c, 409, "机器仍有关联节点，请先在节点管理中修改归属机器")
		} else {
			fail(c, 404, "machine not found")
		}
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
