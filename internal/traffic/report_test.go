package traffic

import (
	"math"
	"testing"
	"time"
)

func TestBillIntegerRoundingAndOverflow(t *testing.T) {
	for _, tc := range []struct{ up, down, milli, want int64 }{{100, 200, 1500, 450}, {1, 0, 1001, 2}, {0, 0, 1500, 0}, {1048576, 1048576, 1000, 2097152}} {
		got, err := Bill(tc.up, tc.down, tc.milli)
		if err != nil || got != tc.want {
			t.Fatalf("Bill(%d,%d,%d)=%d,%v want %d", tc.up, tc.down, tc.milli, got, err, tc.want)
		}
	}
	for _, tc := range []struct{ u, d, m int64 }{{-1, 0, 1000}, {0, 0, 0}, {math.MaxInt64, 1, 1000}, {math.MaxInt64, 0, 100000}} {
		if _, err := Bill(tc.u, tc.d, tc.m); err == nil {
			t.Fatal("invalid or overflowing bill accepted", tc)
		}
	}
}

func TestReportValidation(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	valid := func() Report {
		return Report{ReportID: "stable-batch-1", PeriodStart: now.Add(-time.Minute), PeriodEnd: now, Entries: []Entry{{UserID: 1, Up: 100, Down: 200}}}
	}
	if err := valid().Validate(now); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Report){
		"missing-id":      func(r *Report) { r.ReportID = "" },
		"negative":        func(r *Report) { r.Entries[0].Up = -1 },
		"duplicate-user":  func(r *Report) { r.Entries = append(r.Entries, r.Entries[0]) },
		"future":          func(r *Report) { r.PeriodEnd = now.Add(6 * time.Minute) },
		"window-too-long": func(r *Report) { r.PeriodStart = now.Add(-25 * time.Hour) },
		"invalid-window":  func(r *Report) { r.PeriodStart = r.PeriodEnd },
		"too-large":       func(r *Report) { r.Entries[0].Down = 1 << 51 },
	} {
		t.Run(name, func(t *testing.T) {
			r := valid()
			mutate(&r)
			if r.Validate(now) == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
}
