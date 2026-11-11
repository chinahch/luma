package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/chinahch/luma/internal/traffic"
	"github.com/jackc/pgx/v5"
)

// Ingest atomically records raw use, the multiplier snapshot and one debit.
func (s *Store) Ingest(ctx context.Context, nodeID int64, report traffic.Report, quality string) (bool, error) {
	if quality != "reliable" && quality != "legacy" {
		return false, ErrInvalid
	}
	if err := report.Validate(time.Now()); err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	report.PeriodStart = report.PeriodStart.UTC()
	report.PeriodEnd = report.PeriodEnd.UTC()
	report.Entries = append([]traffic.Entry(nil), report.Entries...)
	sort.Slice(report.Entries, func(i, j int) bool { return report.Entries[i].UserID < report.Entries[j].UserID })
	canonical, err := json.Marshal(report)
	if err != nil {
		return false, err
	}
	digest := Hash(string(canonical))
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var multiplier int64
	var active bool
	if err = tx.QueryRow(ctx, `SELECT multiplier_milli,active FROM nodes WHERE id=$1 FOR UPDATE`, nodeID).Scan(&multiplier, &active); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrForbidden
		}
		return false, err
	}
	if !active {
		return false, ErrForbidden
	}
	var old []byte
	err = tx.QueryRow(ctx, `SELECT payload_hash FROM usage_reports WHERE node_id=$1 AND report_id=$2`, nodeID, report.ReportID).Scan(&old)
	if err == nil {
		if !bytes.Equal(old, digest) {
			return false, ErrConflict
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO usage_reports(node_id,report_id,payload_hash,quality,period_start,period_end,multiplier_milli) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, nodeID, report.ReportID, digest, quality, report.PeriodStart, report.PeriodEnd, multiplier).Scan(&id)
	if err != nil {
		return false, err
	}
	var totalUp, totalDown int64
	for _, entry := range report.Entries {
		billed, err := traffic.Bill(entry.Up, entry.Down, multiplier)
		if err != nil {
			return false, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if totalUp > math.MaxInt64-entry.Up || totalDown > math.MaxInt64-entry.Down {
			return false, ErrInvalid
		}
		totalUp += entry.Up
		totalDown += entry.Down
		tag, err := tx.Exec(ctx, `UPDATE users SET used_bytes=used_bytes+$1 WHERE id=$2 AND used_bytes<=$3`, billed, entry.UserID, math.MaxInt64-billed)
		if err != nil {
			return false, err
		}
		if tag.RowsAffected() != 1 {
			return false, ErrInvalid
		}
		if _, err = tx.Exec(ctx, `INSERT INTO usage_entries(report_id,user_id,up_bytes,down_bytes,billed_bytes) VALUES($1,$2,$3,$4,$5)`, id, entry.UserID, entry.Up, entry.Down, billed); err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO billing_ledger(report_id,user_id,billed_bytes) VALUES($1,$2,$3)`, id, entry.UserID, billed); err != nil {
			return false, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE nodes SET last_report_at=now(),
 last_quality=CASE WHEN last_period_end IS NULL OR last_period_end<=$6 THEN $2 ELSE last_quality END,
 last_up_bytes=CASE WHEN last_period_end IS NULL OR last_period_end<=$6 THEN $3 ELSE last_up_bytes END,
 last_down_bytes=CASE WHEN last_period_end IS NULL OR last_period_end<=$6 THEN $4 ELSE last_down_bytes END,
 last_window_seconds=CASE WHEN last_period_end IS NULL OR last_period_end<=$6 THEN $5 ELSE last_window_seconds END,
 last_period_end=GREATEST(last_period_end,$6) WHERE id=$1`, nodeID, quality, totalUp, totalDown, report.PeriodEnd.Sub(report.PeriodStart).Seconds(), report.PeriodEnd)
	if err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}
