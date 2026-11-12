package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte(store.NewToken()), bcrypt.DefaultCost)

func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(email)
	return email, err == nil && len(email) <= 254 && parsed.Address == email
}
func (a *API) auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := a.s.RefreshCycles(c.Request.Context(), time.Now()); err != nil {
			internal(c, err)
			return
		}
		token, err := c.Cookie("luma_session")
		if err != nil || len(token) != 43 {
			fail(c, 401, "login required")
			return
		}
		var u User
		err = a.s.Pool.QueryRow(c.Request.Context(), `SELECT u.id,u.email,u.role,u.active,u.quota_bytes,u.used_bytes,u.expires_at,u.plan_id,u.next_reset_at,u.billing_anchor_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND u.active AND u.deleted_at IS NULL`, store.Hash(token)).Scan(&u.ID, &u.Email, &u.Role, &u.Active, &u.Quota, &u.Used, &u.ExpiresAt, &u.PlanID, &u.NextResetAt, &u.OpenedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(c, 401, "session expired")
			return
		}
		if err != nil {
			internal(c, err)
			return
		}
		c.Set("user", u)
		c.Next()
	}
}
func (a *API) login(c *gin.Context) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	email, valid := normalizeEmail(in.Email)
	key := store.Hash(email + "|" + c.ClientIP())
	var attempts int
	err := a.s.Pool.QueryRow(c.Request.Context(), `INSERT INTO login_attempts(key_hash,attempts,window_start) VALUES($1,1,now()) ON CONFLICT(key_hash) DO UPDATE SET attempts=CASE WHEN login_attempts.window_start<now()-interval '5 minutes' THEN 1 ELSE login_attempts.attempts+1 END,window_start=CASE WHEN login_attempts.window_start<now()-interval '5 minutes' THEN now() ELSE login_attempts.window_start END RETURNING attempts`, key).Scan(&attempts)
	if err != nil {
		fail(c, 503, "login temporarily unavailable")
		return
	}
	if attempts > 10 {
		fail(c, 429, "too many login attempts; retry in five minutes")
		return
	}
	var id int64
	var hash string
	var active bool
	err = a.s.Pool.QueryRow(c.Request.Context(), `SELECT id,password_hash,active FROM users WHERE email=$1 AND deleted_at IS NULL`, email).Scan(&id, &hash, &active)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		internal(c, err)
		return
	}
	if hash == "" {
		hash = string(dummyHash)
	}
	passwordOK := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil
	if !valid || err != nil || !active || !passwordOK {
		fail(c, 401, "invalid email or password")
		return
	}
	token := store.NewToken()
	_, err = a.s.Pool.Exec(c.Request.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')`, store.Hash(token), id)
	if err != nil {
		internal(c, err)
		return
	}
	a.s.Pool.Exec(c.Request.Context(), `DELETE FROM sessions WHERE expires_at<now()`)
	a.s.Pool.Exec(c.Request.Context(), `DELETE FROM login_attempts WHERE window_start<now()-interval '1 day'`)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("luma_session", token, 7*24*3600, "/", "", a.cfg.SecureCookies, true)
	c.JSON(200, gin.H{"ok": true})
}
func (a *API) logout(c *gin.Context) {
	token, _ := c.Cookie("luma_session")
	if _, err := a.s.Pool.Exec(c.Request.Context(), `DELETE FROM sessions WHERE token_hash=$1`, store.Hash(token)); err != nil {
		internal(c, err)
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("luma_session", "", -1, "/", "", a.cfg.SecureCookies, true)
	c.JSON(200, gin.H{"ok": true})
}

// CreateAdmin is shared by the explicit bootstrap CLI and uses no default password.
func CreateAdmin(ctx context.Context, s *store.Store, email, password string) error {
	email, valid := normalizeEmail(email)
	if !valid || len(password) < 12 || len(password) > 72 {
		return errors.New("valid email and a password of 12..72 bytes required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO users(email,password_hash,role,uuid) VALUES($1,$2,'admin',$3)`, email, string(hash), store.NewID())
	return err
}
