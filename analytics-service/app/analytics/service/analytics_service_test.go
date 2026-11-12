package service

import (
	"errors"
	"testing"

	"github.com/quickbite/analytics-service/app/analytics"
)

// dayKey must bucket by the UTC calendar day, so events from different shards /
// timezones agree on the same day boundary.
func TestDayKey(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"utc midnight", "2026-06-06T00:00:00Z", "2026-06-06", false},
		{"end of day utc", "2026-12-31T23:59:59Z", "2026-12-31", false},
		// 23:30 at -05:00 is 04:30Z the NEXT day — must bucket to the UTC day.
		{"rolls to next utc day", "2026-06-06T23:30:00-05:00", "2026-06-07", false},
		{"unparseable", "not-a-timestamp", "", true},
	}
	for _, c := range cases {
		got, err := dayKey(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error, got none", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: dayKey(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// validateRange guards the read endpoints against bad/unbounded ranges.
func TestValidateRange(t *testing.T) {
	if err := validateRange("2026-01-01", "2026-01-31"); err != nil {
		t.Errorf("valid 1-month range rejected: %v", err)
	}
	if err := validateRange("2026-01-01", "2026-01-01"); err != nil {
		t.Errorf("single-day range rejected: %v", err)
	}
	// exactly one (leap) year is allowed (boundary inclusive)
	if err := validateRange("2024-01-01", "2025-01-01"); err != nil {
		t.Errorf("exactly-366-days range rejected: %v", err)
	}

	bad := map[string][2]string{
		"reversed":     {"2026-02-01", "2026-01-01"},
		"over one year": {"2024-01-01", "2025-06-01"},
		"bad from":      {"01-01-2026", "2026-01-31"},
		"bad to":        {"2026-01-01", ""},
	}
	for name, r := range bad {
		if err := validateRange(r[0], r[1]); !errors.Is(err, analytics.ErrInvalidDateRange) {
			t.Errorf("%s: expected ErrInvalidDateRange, got %v", name, err)
		}
	}
}

// avg must never divide by zero (a zero-order day yields 0, not a panic).
func TestAvg(t *testing.T) {
	cases := []struct {
		sum, count, want int64
	}{
		{1000, 4, 250},
		{100, 0, 0}, // division-by-zero guard
		{0, 0, 0},
		{10, 3, 3}, // integer division (minor units, no fractional cents)
	}
	for _, c := range cases {
		if got := avg(c.sum, c.count); got != c.want {
			t.Errorf("avg(%d, %d) = %d, want %d", c.sum, c.count, got, c.want)
		}
	}
}

// failureField maps the failure discriminator to its Mongo counter field, and
// rejects unknown kinds rather than silently picking a column.
func TestFailureField(t *testing.T) {
	if f, err := failureField(analytics.FailureRejected); err != nil || f != "rejected_count" {
		t.Errorf("rejected: got (%q, %v), want (\"rejected_count\", nil)", f, err)
	}
	if f, err := failureField(analytics.FailureCancelled); err != nil || f != "cancelled_count" {
		t.Errorf("cancelled: got (%q, %v), want (\"cancelled_count\", nil)", f, err)
	}
	if _, err := failureField(analytics.FailureKind("bogus")); err == nil {
		t.Error("unknown failure kind: expected error, got none")
	}
}
