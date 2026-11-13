//go:build gointl_all

package date

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

func elAaiun(t *testing.T) *Environment {
	t.Helper()
	tz, err := intl.LoadTimeZone(intl.Embedded, "Africa/El_Aaiun")
	if err != nil {
		t.Fatal(err)
	}
	loc, err := intl.ParseLocale("en-US")
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(Options{Locale: &loc, TimeZone: tz})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// El_Aaiun went from UTC-1 to UTC at 1976-04-14T01:00Z and to UTC+1 17 days
// later, less than the 19 days V8's offset cache assumes. Node, asked about
// the instant before the first change, calls the instant of it UTC+1:
//
//	process.env.TZ = "Africa/El_Aaiun"
//	new Date(198291599999).getTimezoneOffset()  // 60
//	new Date(198291600000).getTimezoneOffset()  // -60
//
// and, asked first, UTC:
//
//	new Date(198291600000).getTimezoneOffset()  // 0
//
// Making the second Date asks the cache once, finding the change and keeping
// the segment after it, whose offset getTimezoneOffset then reads.
func TestCacheAnswersAsNodeAcrossShortSegment(t *testing.T) {
	const at = 198291600000
	e := elAaiun(t)
	if got := e.TimezoneOffset(e.NewDate(at - 1).Value()); got != 60 {
		t.Errorf("before the change: %v, want 60", got)
	}
	if got := e.TimezoneOffset(e.NewDate(at).Value()); got != -60 {
		t.Errorf("at the change, after the instant before it: %v, want -60", got)
	}
	e = elAaiun(t)
	if got := e.TimezoneOffset(e.NewDate(at).Value()); got != 0 {
		t.Errorf("at the change, asked first: %v, want 0", got)
	}
}

// A Date keeps the local fields it read when its value was set, as V8's
// JSDate does: in Node, after the two Dates above, the second's getHours()
// is 1, read when it was made, though toString then writes 02:00:00.
func TestDateKeepsLocalFields(t *testing.T) {
	const at = 198291600000
	e := elAaiun(t)
	e.NewDate(at - 1)
	d := e.NewDate(at)
	f, ok := e.Local(d)
	if !ok || f.Hour != 1 || f.Day != 14 {
		t.Errorf("Local = %+v %v, want 01:00 on the 14th", f, ok)
	}
	if got, want := e.String(d.Value()), "Wed Apr 14 1976 02:00:00 GMT+0100 (Western European Standard Time)"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
	if _, ok := e.Local(e.NewDate(8.64e15 + 1)); ok {
		t.Error("Local of an invalid date is ok")
	}
}
