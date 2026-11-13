package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

type Renewal struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	PlanID    int64     `json:"plan_id"`
	RequestID string    `json:"request_id"`
	PlanName  string    `json:"plan_name"`
	Quota     int64     `json:"quota_bytes"`
	Days      int       `json:"duration_days"`
	Price     int64     `json:"price_cents"`
	Status    string    `json:"status"`
	OrderID   *int64    `json:"order_id"`
	CreatedAt time.Time `json:"created_at"`
}

const renewalColumns = `id,user_id,plan_id,request_id,plan_name,quota_bytes,duration_days,price_cents,status,order_id,created_at`

func scanRenewal(row pgx.Row) (r Renewal, err error) {
	err = row.Scan(&r.ID, &r.UserID, &r.PlanID, &r.RequestID, &r.PlanName, &r.Quota, &r.Days, &r.Price, &r.Status, &r.OrderID, &r.CreatedAt)
	return
}

func (s *Store) RequestRenewal(ctx context.Context, userID int64, key string) (Renewal, error) {
	if len(key) < 1 || len(key) > 128 {
		return Renewal{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Renewal{}, err
	}
	defer tx.Rollback(ctx)
	var plan *int64
	var expiry *time.Time
	if err = tx.QueryRow(ctx, `SELECT plan_id,expires_at FROM users WHERE id=$1 AND active AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&plan, &expiry); err != nil {
		return Renewal{}, err
	}
	old, err := scanRenewal(tx.QueryRow(ctx, `SELECT `+renewalColumns+` FROM renewal_requests WHERE user_id=$1 AND request_id=$2`, userID, key))
	if err == nil {
		return old, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Renewal{}, err
	}
	if plan == nil || expiry == nil {
		return Renewal{}, ErrConflict
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM renewal_requests WHERE user_id=$1 AND status='pending')`, userID).Scan(&pending); err != nil {
		return Renewal{}, err
	}
	if pending {
		return Renewal{}, ErrConflict
	}
	r, err := scanRenewal(tx.QueryRow(ctx, `INSERT INTO renewal_requests(user_id,plan_id,request_id,plan_name,quota_bytes,duration_days,price_cents) SELECT $1,id,$3,name,quota_bytes,duration_days,price_cents FROM plans WHERE id=$2 RETURNING `+renewalColumns, userID, *plan, key))
	if err != nil {
		return Renewal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Renewal{}, err
	}
	return r, nil
}

// ResolveRenewal locks user before request, matching allocation and submission.
func (s *Store) ResolveRenewal(ctx context.Context, id, actorID int64, action string, ownerOnly bool) (Renewal, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Renewal{}, err
	}
	defer tx.Rollback(ctx)
	var uid int64
	if err = tx.QueryRow(ctx, `SELECT user_id FROM renewal_requests WHERE id=$1`, id).Scan(&uid); err != nil {
		return Renewal{}, err
	}
	if ownerOnly && uid != actorID {
		return Renewal{}, pgx.ErrNoRows
	}
	var plan *int64
	var expiry *time.Time
	var active bool
	var deleted *time.Time
	if err = tx.QueryRow(ctx, `SELECT plan_id,expires_at,active,deleted_at FROM users WHERE id=$1 FOR UPDATE`, uid).Scan(&plan, &expiry, &active, &deleted); err != nil {
		return Renewal{}, err
	}
	r, err := scanRenewal(tx.QueryRow(ctx, `SELECT `+renewalColumns+` FROM renewal_requests WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return Renewal{}, err
	}
	target := map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}[action]
	if target == "" || (ownerOnly && action != "cancel") {
		return Renewal{}, ErrInvalid
	}
	if r.Status == target {
		return r, nil
	}
	if r.Status != "pending" {
		return Renewal{}, ErrConflict
	}
	if action == "approve" {
		if !active || deleted != nil || plan == nil || *plan != r.PlanID || expiry == nil {
			return Renewal{}, ErrConflict
		}
		o := Order{UserID: uid, ActorID: actorID, PlanID: r.PlanID, RequestID: fmt.Sprintf("renewal:%d", id), PlanName: r.PlanName, Quota: r.Quota, Days: r.Days, Price: r.Price, Status: "allocated"}
		result, e := s.applyOrderTx(ctx, tx, o, plan, expiry)
		if e != nil {
			return Renewal{}, e
		}
		r.OrderID = &result.ID
	}
	_, err = tx.Exec(ctx, `UPDATE renewal_requests SET status=$2,order_id=$3,actor_id=$4,resolved_at=now() WHERE id=$1`, id, target, r.OrderID, actorID)
	if err != nil {
		return Renewal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Renewal{}, err
	}
	r.Status = target
	return r, nil
}
