package store

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// SetTimezone selects the panel's civil calendar. Reads remain safe while APIs initialize.
func (s *Store) SetTimezone(zone string) error {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return err
	}
	s.cycleLocation.Store(loc)
	return nil
}
func (s *Store) calendarLocation() *time.Location {
	if loc := s.cycleLocation.Load(); loc != nil {
		return loc
	}
	return time.UTC
}
func anniversary(anchor time.Time, year int, month time.Month, loc *time.Location) time.Time {
	day := anchor.In(loc).Day()
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}
func nextAnniversary(anchor, now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	next := anniversary(anchor, local.Year(), local.Month(), loc)
	if !next.After(now) {
		next = anniversary(anchor, local.Year(), local.Month()+1, loc)
	}
	return next
}

// refreshUserCycle must share the user row lock with reports, resets and orders.
func (s *Store) refreshUserCycle(ctx context.Context, tx pgx.Tx, id int64, now time.Time) error {
	var anchor time.Time
	var next *time.Time
	var used int64
	if err := tx.QueryRow(ctx, `SELECT billing_anchor_at,next_reset_at,used_bytes FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&anchor, &next, &used); err != nil {
		return err
	}
	loc := s.calendarLocation()
	if next == nil {
		_, err := tx.Exec(ctx, `UPDATE users SET next_reset_at=$2 WHERE id=$1`, id, nextAnniversary(anchor, now, loc))
		return err
	}
	if next.After(now) {
		return nil
	}
	upcoming := nextAnniversary(anchor, now, loc)
	// AddDate from day 31 can overflow; derive the preceding month from day one.
	monthBefore := time.Date(upcoming.In(loc).Year(), upcoming.In(loc).Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
	started := anniversary(anchor, monthBefore.Year(), monthBefore.Month(), loc)
	if _, err := tx.Exec(ctx, `INSERT INTO usage_resets(user_id,old_used_bytes,reason,created_at) VALUES($1,$2,'calendar',$3)`, id, used, started); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE users SET used_bytes=0,cycle_started_at=$2,next_reset_at=$3 WHERE id=$1`, id, started, upcoming)
	return err
}

func (s *Store) RefreshCycles(ctx context.Context, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE deleted_at IS NULL AND (next_reset_at IS NULL OR next_reset_at<=$1) ORDER BY id FOR UPDATE`, now)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.refreshUserCycle(ctx, tx, id, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ResetUserUsage(ctx context.Context, userID, actorID int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Define the manual boundary only after earlier report/order writers finish.
	var lockedID int64
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&lockedID); err != nil {
		return err
	}
	now := time.Now().UTC()
	if err = s.refreshUserCycle(ctx, tx, userID, now); err != nil {
		return err
	}
	var used int64
	if err = tx.QueryRow(ctx, `SELECT used_bytes FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).Scan(&used); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO usage_resets(user_id,actor_id,old_used_bytes,reason,created_at) VALUES($1,$2,$3,'manual',$4)`, userID, actorID, used, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET used_bytes=0,cycle_started_at=$2 WHERE id=$1`, userID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
