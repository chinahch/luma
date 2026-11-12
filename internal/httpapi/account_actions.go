package httpapi

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func (a *API) registerAccountRoutes(r *gin.Engine, admin *gin.RouterGroup) {
	r.GET("/api/me/subscription", a.auth(), func(c *gin.Context) { a.savedSubscription(c, current(c).ID, "read", "") })
	r.POST("/api/me/subscription/recover", a.auth(), a.recoverSubscription)
	r.GET("/api/me/invite", a.auth(), a.myInvite)
	r.GET("/api/invites/:code", a.checkInvite)
	r.POST("/api/register", a.registerInvitedUser)
	admin.GET("/users/:id/subscription", func(c *gin.Context) {
		if id, ok := positiveID(c); ok {
			a.savedSubscription(c, id, "read", "")
		}
	})
	admin.POST("/users/:id/identity", func(c *gin.Context) {
		if id, ok := positiveID(c); ok {
			a.savedSubscription(c, id, "identity", "")
		}
	})
	admin.POST("/users/:id/reset-usage", a.resetUserUsage)
	admin.DELETE("/users/:id", a.deleteUser)
	admin.GET("/users/:id/invites", a.userInvites)
}

// Lock the account before reading or changing either the token hash or its
// encrypted display copy. Refresh and simultaneous first visits never rotate it.
func (a *API) savedSubscription(c *gin.Context, id int64, mode, recoverToken string) {
	if !a.requireBox(c) {
		return
	}
	ctx := c.Request.Context()
	tx, err := a.s.Pool.Begin(ctx)
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(ctx)
	var hash, cipher []byte
	err = tx.QueryRow(ctx, `SELECT subscription_hash,subscription_cipher FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&hash, &cipher)
	if err == pgx.ErrNoRows {
		fail(c, 404, "user not found")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	token := ""
	write := false
	switch {
	case mode == "recover":
		if len(recoverToken) != 43 || !bytes.Equal(store.Hash(recoverToken), hash) {
			fail(c, 400, "link does not match this account's current subscription")
			return
		}
		token = recoverToken
		write = true
	case mode == "rotate" || mode == "identity" || len(hash) == 0:
		token = store.NewToken()
		write = true
	case len(cipher) == 0:
		if err = tx.Commit(ctx); err != nil {
			internal(c, err)
			return
		}
		c.JSON(200, gin.H{"path": "", "needs_recovery": true})
		return
	default:
		plain, e := a.box.Open(cipher, fmt.Sprintf("subscription:%d", id))
		if e != nil || len(plain) != 43 || !bytes.Equal(store.Hash(string(plain)), hash) {
			fail(c, 503, "cannot decrypt subscription; verify LUMA_MASTER_KEY")
			return
		}
		token = string(plain)
	}
	if write {
		cipher, err = a.box.Seal([]byte(token), fmt.Sprintf("subscription:%d", id))
		if err != nil {
			internal(c, err)
			return
		}
		_, err = tx.Exec(ctx, `UPDATE users SET subscription_hash=$2,subscription_cipher=$3 WHERE id=$1`, id, store.Hash(token), cipher)
		if err != nil {
			internal(c, err)
			return
		}
		if mode == "identity" {
			if _, err = tx.Exec(ctx, `UPDATE users SET uuid=$2 WHERE id=$1`, id, store.NewID()); err != nil {
				internal(c, err)
				return
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"path": "/sub/" + token, "needs_recovery": false})
}
func (a *API) recoverSubscription(c *gin.Context) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(c, &in) {
		return
	}
	token := strings.TrimSpace(in.Token)
	if u, e := url.Parse(token); e == nil && strings.HasPrefix(u.Path, "/sub/") && u.RawQuery == "" && u.Fragment == "" {
		token = strings.TrimPrefix(u.Path, "/sub/")
	}
	a.savedSubscription(c, current(c).ID, "recover", token)
}
func (a *API) resetUserUsage(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	if err := a.s.ResetUserUsage(c.Request.Context(), id, current(c).ID); err != nil {
		if err == pgx.ErrNoRows {
			fail(c, 404, "user not found")
		} else {
			internal(c, err)
		}
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) deleteUser(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	if id == current(c).ID {
		fail(c, 409, "cannot delete your own account")
		return
	}
	ctx := c.Request.Context()
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(74109914013)`); e != nil {
		internal(c, e)
		return
	}
	var role string
	e = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&role)
	if e == pgx.ErrNoRows {
		fail(c, 404, "user not found")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	if role == "admin" {
		var remaining int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='admin' AND active AND deleted_at IS NULL AND id<>$1`, id).Scan(&remaining); e != nil {
			internal(c, e)
			return
		}
		if remaining == 0 {
			fail(c, 409, "cannot delete the last active administrator")
			return
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE users SET active=false,deleted_at=now(),subscription_hash=NULL,subscription_cipher=NULL,invite_code=NULL,uuid=$2 WHERE id=$1`, id, store.NewID()); e != nil {
		internal(c, e)
		return
	}
	if _, e = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id); e != nil {
		internal(c, e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	c.JSON(200, gin.H{"ok": true, "history_preserved": true})
}
