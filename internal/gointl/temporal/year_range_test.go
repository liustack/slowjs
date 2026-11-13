//go:build gointl_all

package temporal

import (
	"strings"
	"testing"
)

// A year outside the i32 temporal_rs holds it in is no ISO date: it had
// wrapped, 4294969316 becoming 2020. A year inside it but outside the
// limits keeps its own message. Node's errors (ISSUES.md API-4).
func TestYearOutsideInt32(t *testing.T) {
	iso, err := NewCalendar("iso8601")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		year            int64
		date, yearMonth string
	}{
		{4294969316, "Invalid ISO date.", "Invalid ISO date."},
		{1 << 31, "Invalid ISO date.", "Invalid ISO date."},
		{-(1 << 31) - 1, "Invalid ISO date.", "Invalid ISO date."},
		{1<<31 - 1, "Date is not within ISO date time limits.", "Exceeded valid range."},
		{275761, "Date is not within ISO date time limits.", "Exceeded valid range."},
	} {
		year := int(c.year)
		if int64(year) != c.year {
			// A year a 32-bit int cannot be given.
			continue
		}
		if _, err := NewPlainDate(year, 1, 1, iso, Reject); err == nil || !strings.Contains(err.Error(), c.date) {
			t.Errorf("PlainDate %d: %v, want %q", c.year, err, c.date)
		}
		if _, err := NewPlainYearMonth(year, 1, nil, iso, Reject); err == nil || !strings.Contains(err.Error(), c.yearMonth) {
			t.Errorf("PlainYearMonth %d: %v, want %q", c.year, err, c.yearMonth)
		}
	}
	if d, err := NewPlainDate(275760, 1, 1, iso, Reject); err != nil || d.String(CalendarAuto) != "+275760-01-01" {
		t.Errorf("275760: %v %v", d, err)
	}
}
