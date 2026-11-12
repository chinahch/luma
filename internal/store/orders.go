package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type Order struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ActorID   int64     `json:"actor_id"`
	PlanID    int64     `json:"plan_id"`
	RequestID string    `json:"request_id"`
	PlanName  string    `json:"plan_name"`
	Quota     int64     `json:"quota_bytes"`
	Days      int       `json:"duration_days"`
	Price     int64     `json:"price_cents"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

const orderColumns = `id,user_id,actor_id,plan_id,request_id,plan_name,quota_bytes,duration_days,price_cents,status,expires_at,created_at`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.UserID, &o.ActorID, &o.PlanID, &o.RequestID, &o.PlanName, &o.Quota, &o.Days, &o.Price, &o.Status, &o.ExpiresAt, &o.CreatedAt)
	return o, err
}

// AllocateOrder records a manual allocation and its immutable plan snapshot atomically.
func (s *Store) AllocateOrder(ctx context.Context, userID, actorID, planID int64, requestID string) (Order, error) {
	if userID <= 0 || actorID <= 0 || planID <= 0 || len(requestID) < 1 || len(requestID) > 128 {
		return Order{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(ctx)
	o, err := s.AllocateOrderTx(ctx, tx, userID, actorID, planID, requestID)
	if err != nil {
		return Order{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return o, nil
}

// AllocateOrderTx lets account creation commit the initial allocation in the same transaction.
func (s *Store) AllocateOrderTx(ctx context.Context, tx pgx.Tx, userID, actorID, planID int64, requestID string) (Order, error) {
	if userID <= 0 || actorID <= 0 || planID <= 0 || len(requestID) < 1 || len(requestID) > 128 {
		return Order{}, ErrInvalid
	}
	var previous *int64
	var expiry *time.Time
	if err := tx.QueryRow(ctx, `SELECT plan_id,expires_at FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&previous, &expiry); err != nil {
		return Order{}, err
	}
	old, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE user_id=$1 AND request_id=$2`, userID, requestID))
	if err == nil {
		if old.PlanID != planID {
			return Order{}, ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Order{}, err
	}
	o := Order{UserID: userID, ActorID: actorID, PlanID: planID, RequestID: requestID, Status: "allocated"}
	if err = tx.QueryRow(ctx, `SELECT name,quota_bytes,duration_days,price_cents FROM plans WHERE id=$1 FOR SHARE`, planID).Scan(&o.PlanName, &o.Quota, &o.Days, &o.Price); err != nil {
		return Order{}, err
	}
	now := time.Now().UTC()
	if err = s.refreshUserCycle(ctx, tx, userID, now); err != nil {
		return Order{}, err
	}
	renew := previous != nil && *previous == planID && expiry != nil && expiry.After(now)
	base := now
	if renew {
		base = *expiry
	}
	o.ExpiresAt = base.AddDate(0, 0, o.Days)
	if renew {
		_, err = tx.Exec(ctx, `UPDATE users SET plan_id=$2,quota_bytes=$3,expires_at=$4 WHERE id=$1`, userID, planID, o.Quota, o.ExpiresAt)
	} else {
		if _, err = tx.Exec(ctx, `INSERT INTO usage_resets(user_id,actor_id,old_used_bytes,reason,created_at) SELECT id,$2,used_bytes,'order',$3 FROM users WHERE id=$1`, userID, actorID, now); err != nil {
			return Order{}, err
		}
		_, err = tx.Exec(ctx, `UPDATE users SET plan_id=$2,quota_bytes=$3,expires_at=$4,used_bytes=0,billing_anchor_at=$5,cycle_started_at=$5,next_reset_at=$6 WHERE id=$1`, userID, planID, o.Quota, o.ExpiresAt, now, nextAnniversary(now, now, s.calendarLocation()))
	}
	if err != nil {
		return Order{}, err
	}
	o, err = scanOrder(tx.QueryRow(ctx, `INSERT INTO orders(user_id,actor_id,plan_id,request_id,plan_name,quota_bytes,duration_days,price_cents,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+orderColumns, userID, actorID, planID, requestID, o.PlanName, o.Quota, o.Days, o.Price, o.ExpiresAt))
	if err != nil {
		return Order{}, err
	}
	return o, nil
}
