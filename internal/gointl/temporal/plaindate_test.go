//go:build gointl_all

package temporal

import (
	"errors"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A date and time that is an instant outside Temporal's range is a
// RangeError, as Node throws it, in Node's words, where the error was lost
// and the epoch returned (test262's PlainDate/prototype/toZonedDateTime/
// get-epoch-nanoseconds-for-throws).
func TestPlainDateToZonedDateTimeOutOfRange(t *testing.T) {
	zs, err := LoadZones(intl.Embedded)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewPlainDate(-271821, 4, 19, ISOCalendar(), Reject)
	if err != nil {
		t.Fatal(err)
	}
	one, err := NewPlainTime(1, 0, 0, 0, 0, 0, Reject)
	if err != nil {
		t.Fatal(err)
	}
	for zone, want := range map[string]string{
		"UTC": "RangeError: Instant nanoseconds are not within a valid epoch range.",
		"+00": "RangeError: Not in a valid ISO day range.",
	} {
		tz, err := zs.TimeZoneFromString([]byte(zone))
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.ToZonedDateTime(tz, &one)
		if !errors.Is(err, ErrRange) || err.Error() != want {
			t.Errorf("%s: %v, want %s", zone, err, want)
		}
	}
}
