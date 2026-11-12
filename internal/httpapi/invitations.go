package httpapi

import (
	"encoding/json"
	"strings"

	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (a *API) myInvite(c *gin.Context) {
	ctx := c.Request.Context()
	id := current(c).ID
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	var code *string
	e = tx.QueryRow(ctx, `SELECT invite_code FROM users WHERE id=$1 AND active AND deleted_at IS NULL FOR UPDATE`, id).Scan(&code)
	if e != nil {
		internal(c, e)
		return
	}
	if code == nil {
		v := store.NewToken()
		code = &v
		if _, e = tx.Exec(ctx, `UPDATE users SET invite_code=$2 WHERE id=$1`, id, v); e != nil {
			internal(c, e)
			return
		}
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	var raw []byte
	e = a.s.Pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT id,email,created_at FROM users WHERE inviter_id=$1 AND deleted_at IS NULL ORDER BY id DESC LIMIT 2000)t`, id).Scan(&raw)
	if e != nil {
		internal(c, e)
		return
	}
	// Raw JSON is decoded as a list; no sessions or subscription data are exposed.
	var invited []map[string]any
	if e = json.Unmarshal(raw, &invited); e != nil {
		internal(c, e)
		return
	}
	c.JSON(200, gin.H{"path": "/?invite=" + *code, "code": *code, "invited_count": len(invited), "invites": invited})
}
func (a *API) checkInvite(c *gin.Context) {
	var exists bool
	e := a.s.Pool.QueryRow(c.Request.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE invite_code=$1 AND active AND deleted_at IS NULL)`, c.Param("code")).Scan(&exists)
	if e != nil {
		internal(c, e)
		return
	}
	if !exists {
		fail(c, 404, "invitation unavailable")
		return
	}
	c.JSON(200, gin.H{"valid": true})
}
func (a *API) registerInvitedUser(c *gin.Context) {
	var in struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if !decode(c, &in) {
		return
	}
	ctx := c.Request.Context()
	var attempts int
	e := a.s.Pool.QueryRow(ctx, `INSERT INTO login_attempts(key_hash,attempts,window_start) VALUES($1,1,now()) ON CONFLICT(key_hash) DO UPDATE SET attempts=CASE WHEN login_attempts.window_start<now()-interval '15 minutes' THEN 1 ELSE login_attempts.attempts+1 END,window_start=CASE WHEN login_attempts.window_start<now()-interval '15 minutes' THEN now() ELSE login_attempts.window_start END RETURNING attempts`, store.Hash("registration|"+c.ClientIP())).Scan(&attempts)
	if e != nil {
		internal(c, e)
		return
	}
	if attempts > 5 {
		fail(c, 429, "too many registration attempts; retry in fifteen minutes")
		return
	}
	email, valid := normalizeEmail(in.Email)
	if !valid || len(in.Password) < 12 || len(in.Password) > 72 || len(in.InviteCode) != 43 || strings.ContainsAny(in.InviteCode, " \r\n\t") {
		fail(c, 400, "valid email, invitation and 12..72 byte password required")
		return
	}
	// Check a valid inviter before expensive bcrypt work; recheck under a lock
	// at insertion so disabling/deleting the inviter cannot race registration.
	var owner int64
	e = a.s.Pool.QueryRow(ctx, `SELECT id FROM users WHERE invite_code=$1 AND active AND deleted_at IS NULL`, in.InviteCode).Scan(&owner)
	if e == pgx.ErrNoRows {
		fail(c, 400, "registration unavailable; verify invitation and email")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		internal(c, e)
		return
	}
	tx, e := a.s.Pool.Begin(ctx)
	if e != nil {
		internal(c, e)
		return
	}
	defer tx.Rollback(ctx)
	e = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 AND invite_code=$2 AND active AND deleted_at IS NULL FOR SHARE`, owner, in.InviteCode).Scan(&owner)
	if e == pgx.ErrNoRows {
		fail(c, 400, "registration unavailable; verify invitation and email")
		return
	}
	if e != nil {
		internal(c, e)
		return
	}
	var id int64
	e = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash,role,uuid,quota_bytes,inviter_id) VALUES($1,$2,'user',$3,0,$4) RETURNING id`, email, string(hash), store.NewID(), owner).Scan(&id)
	if e != nil {
		fail(c, 400, "registration unavailable; verify invitation and email")
		return
	}
	if e = tx.Commit(ctx); e != nil {
		internal(c, e)
		return
	}
	c.JSON(201, gin.H{"ok": true, "id": id, "notice": "account created; an administrator must allocate a plan before node use"})
}
func (a *API) userInvites(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	a.jsonQuery(c, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]') FROM (SELECT id,email,created_at,deleted_at FROM users WHERE inviter_id=$1 ORDER BY id DESC LIMIT 2000)t`, id)
}
