package traffic

import (
	"errors"
	"math/big"
	"regexp"
	"time"
)

type Entry struct {
	UserID int64 `json:"user_id"`
	Up     int64 `json:"up_bytes"`
	Down   int64 `json:"down_bytes"`
}
type Report struct {
	ReportID    string    `json:"report_id"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	Entries     []Entry   `json:"entries"`
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func (r Report) Validate(now time.Time) error {
	if !idPattern.MatchString(r.ReportID) || r.PeriodStart.IsZero() || !r.PeriodEnd.After(r.PeriodStart) || r.PeriodEnd.Sub(r.PeriodStart) > 24*time.Hour || r.PeriodEnd.After(now.Add(5*time.Minute)) {
		return errors.New("invalid report identity or period (maximum 24 hours)")
	}
	if len(r.Entries) > 10000 {
		return errors.New("too many entries")
	}
	seen := map[int64]bool{}
	for _, e := range r.Entries {
		if e.UserID <= 0 || e.Up < 0 || e.Down < 0 || e.Up > 1<<50 || e.Down > 1<<50 || seen[e.UserID] {
			return errors.New("invalid or duplicate traffic entry")
		}
		seen[e.UserID] = true
	}
	return nil
}

// Bill rounds up once per user and report, without floating point or overflow.
func Bill(up, down, milli int64) (int64, error) {
	if up < 0 || down < 0 || milli < 1 || milli > 100000 {
		return 0, errors.New("invalid traffic or multiplier")
	}
	total := new(big.Int).Add(big.NewInt(up), big.NewInt(down))
	// Raw totals must also fit the PostgreSQL bigint accounting model.
	if !total.IsInt64() {
		return 0, errors.New("traffic total overflow")
	}
	total.Mul(total, big.NewInt(milli))
	total.Add(total, big.NewInt(999))
	total.Div(total, big.NewInt(1000))
	if !total.IsInt64() {
		return 0, errors.New("billed traffic overflow")
	}
	return total.Int64(), nil
}
