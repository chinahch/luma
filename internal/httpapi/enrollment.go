package httpapi

import (
	"bytes"
	"regexp"
	"time"

	"github.com/chinahch/luma/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var enrollmentValue = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func (a *API) createEnrollment(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	code := store.NewToken()
	var expires time.Time
	ctx := c.Request.Context()
	tx, err := a.s.Pool.Begin(ctx)
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(ctx)
	var machine int64
	err = tx.QueryRow(ctx, `SELECT id FROM machines WHERE id=$1 FOR UPDATE`, id).Scan(&machine)
	if err == pgx.ErrNoRows {
		fail(c, 404, "server not found")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	err = tx.QueryRow(ctx, `INSERT INTO machine_enrollments(machine_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '15 minutes') ON CONFLICT(machine_id) DO UPDATE SET code_hash=excluded.code_hash,claimed_token_hash=NULL,expires_at=excluded.expires_at,created_at=now() RETURNING expires_at`, id, store.Hash(code)).Scan(&expires)
	if err != nil {
		internal(c, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"machine_id": id, "code": code, "expires_at": expires})
}

func (a *API) enrollMachine(c *gin.Context) {
	var in struct {
		MachineID int64  `json:"machine_id"`
		Code      string `json:"code"`
		Token     string `json:"token"`
	}
	if !decode(c, &in) {
		return
	}
	if in.MachineID <= 0 || !enrollmentValue.MatchString(in.Code) || !enrollmentValue.MatchString(in.Token) {
		fail(c, 401, "installation command is invalid or expired")
		return
	}
	ctx := c.Request.Context()
	tx, err := a.s.Pool.Begin(ctx)
	if err != nil {
		internal(c, err)
		return
	}
	defer tx.Rollback(ctx)
	// Lock in the same machine→enrollment order as deletion and regeneration.
	var current []byte
	err = tx.QueryRow(ctx, `SELECT token_hash FROM machines WHERE id=$1 FOR UPDATE`, in.MachineID).Scan(&current)
	if err == pgx.ErrNoRows {
		fail(c, 401, "installation command is invalid or expired")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	var claimed []byte
	var valid bool
	err = tx.QueryRow(ctx, `SELECT claimed_token_hash,expires_at>clock_timestamp() FROM machine_enrollments WHERE machine_id=$1 AND code_hash=$2 FOR UPDATE`, in.MachineID, store.Hash(in.Code)).Scan(&claimed, &valid)
	if err == pgx.ErrNoRows || err == nil && !valid {
		fail(c, 401, "installation command is invalid or expired")
		return
	}
	if err != nil {
		internal(c, err)
		return
	}
	hash := store.Hash(in.Token)
	if claimed != nil {
		if !bytes.Equal(claimed, hash) || !bytes.Equal(current, hash) {
			fail(c, 409, "installation command has already been used; generate a new command")
			return
		}
	} else {
		if _, err = tx.Exec(ctx, `UPDATE machines SET token_hash=$2 WHERE id=$1`, in.MachineID, hash); err != nil {
			internal(c, err)
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE machine_enrollments SET claimed_token_hash=$2 WHERE machine_id=$1`, in.MachineID, hash); err != nil {
			internal(c, err)
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		internal(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true, "machine_id": in.MachineID})
}

func (a *API) renameMachine(c *gin.Context) {
	id, ok := positiveID(c)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(c, &in) {
		return
	}
	if !validName(in.Name) {
		fail(c, 400, "invalid server name")
		return
	}
	tag, err := a.s.Pool.Exec(c.Request.Context(), `UPDATE machines SET name=$2 WHERE id=$1`, id, in.Name)
	if err != nil {
		internal(c, err)
		return
	}
	if tag.RowsAffected() != 1 {
		fail(c, 404, "server not found")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
