//go:build gointl_all

package temporal_test

import (
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/gointl/temporal"
)

// Every exported function can be called from outside the package: they had
// taken or returned the unexported int128, so that NewZonedDateTime,
// NewDuration, TimeZone.OffsetNanosecondsFor and EpochNanosecondsForUTC
// could not be. An offset zone is whole minutes less than a day, as
// temporal_rs's are; a day's offset had written "+24:00", which no parser
// reads (ISSUES.md API-10).
func TestExportedAPIIsCallable(t *testing.T) {
	iso, err := temporal.NewCalendar("iso8601")
	if err != nil {
		t.Fatal(err)
	}
	date, err := temporal.NewPlainDate(2020, 1, 1, iso, temporal.Reject)
	if err != nil {
		t.Fatal(err)
	}
	// 2020-01-01T12:00Z.
	noon, err := temporal.NewInstant(date.EpochNanosecondsForUTC())
	if err != nil {
		t.Fatal(err)
	}
	if ms := noon.EpochMilliseconds(); ms != 1577880000000 {
		t.Errorf("PlainDate: %d ms, want 1577880000000", ms)
	}
	ym, err := temporal.NewPlainYearMonth(2020, 1, nil, iso, temporal.Reject)
	if err != nil {
		t.Fatal(err)
	}
	if hi, lo := ym.EpochNanosecondsForUTC(); hi != 0 || lo != 1577880000000_000000 {
		t.Errorf("PlainYearMonth: %d %d", hi, lo)
	}
	tm, err := temporal.NewPlainTime(1, 2, 3, 0, 0, 0, temporal.Reject)
	if err != nil {
		t.Fatal(err)
	}
	if hi, lo := tm.EpochNanosecondsForUTC(); hi != 0 || lo != 3723_000000000 {
		t.Errorf("PlainTime: %d %d", hi, lo)
	}
	old, err := temporal.NewPlainDate(-271821, 4, 19, iso, temporal.Reject)
	if err != nil {
		t.Fatal(err)
	}
	// -8.64e21 less 12 hours, the 20th being day -10^8: past int64.
	if hi, lo := old.EpochNanosecondsForUTC(); hi != -469 || lo != 0x9fe9_a964_4858_8000 {
		t.Errorf("-271821-04-19: %d %#x", hi, lo)
	}

	d, err := temporal.NewDuration(0, 0, 0, 0, 1, 0, 0, 0, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := d.String(temporal.DefaultToStringOptions()); err != nil || s != "PT1H0.000002003S" {
		t.Errorf("NewDuration: %q %v", s, err)
	}
	if _, err := temporal.NewDuration(0, 0, 0, 0, 1, 0, 0, 0, -2, 0); err == nil {
		t.Error("NewDuration of mixed signs: no error")
	}

	for _, ns := range []int64{86400_000000000, -86400_000000000, 30_000000000, 1} {
		if _, err := temporal.OffsetTimeZone(ns); err == nil ||
			!strings.Contains(err.Error(), "Offset time zones are whole minutes less than a day.") {
			t.Errorf("OffsetTimeZone(%d): %v", ns, err)
		}
	}
	for ns, id := range map[int64]string{
		19800_000000000:  "+05:30",
		-86340_000000000: "-23:59",
		0:                "+00:00",
	} {
		tz, err := temporal.OffsetTimeZone(ns)
		if err != nil {
			t.Errorf("OffsetTimeZone(%d): %v", ns, err)
			continue
		}
		if got := tz.Identifier(); got != id {
			t.Errorf("OffsetTimeZone(%d): %s, want %s", ns, got, id)
		}
		if off, err := tz.OffsetNanosecondsFor(noon); err != nil || off != ns {
			t.Errorf("OffsetNanosecondsFor %s: %d %v", id, off, err)
		}
	}
	tz, _ := temporal.OffsetTimeZone(19800_000000000)
	z, err := temporal.NewZonedDateTimeFromInstant(noon, tz, iso)
	if err != nil {
		t.Fatal(err)
	}
	if off := z.OffsetNanoseconds(); off != 19800_000000000 {
		t.Errorf("ZonedDateTime offset: %d", off)
	}
}
