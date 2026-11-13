//go:build gointl_all

package date

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The strings as they were written with printf, before they were appended
// into a buffer: what the new code must write, byte for byte.

func printfYear(year int64) string {
	if year < 0 {
		return fmt.Sprintf("%05d", year)
	}
	return fmt.Sprintf("%04d", year)
}

func printfOffset(minutes int64) string {
	sign := byte('+')
	if minutes < 0 {
		sign = '-'
		minutes = -minutes
	}
	return fmt.Sprintf("GMT%c%02d%02d", sign, minutes/60, minutes%60)
}

func printfString(e *Environment, t float64) string {
	f, offset, name := e.local(t)
	return fmt.Sprintf("%s %s %02d %s %02d:%02d:%02d %s (%s)", shortWeekdays[f.Weekday], shortMonths[f.Month],
		f.Day, printfYear(f.Year), f.Hour, f.Minute, f.Second, printfOffset(offset), name)
}

func printfDateString(e *Environment, t float64) string {
	f, _, _ := e.local(t)
	return fmt.Sprintf("%s %s %02d %s", shortWeekdays[f.Weekday], shortMonths[f.Month], f.Day, printfYear(f.Year))
}

func printfTimeString(e *Environment, t float64) string {
	f, offset, name := e.local(t)
	return fmt.Sprintf("%02d:%02d:%02d %s (%s)", f.Hour, f.Minute, f.Second, printfOffset(offset), name)
}

func printfUTCString(t float64) string {
	f := BreakDown(t)
	return fmt.Sprintf("%s, %02d %s %s %02d:%02d:%02d GMT", shortWeekdays[f.Weekday], f.Day, shortMonths[f.Month],
		printfYear(f.Year), f.Hour, f.Minute, f.Second)
}

func printfISOString(t float64) string {
	f := BreakDown(t)
	var year string
	switch {
	case f.Year >= 0 && f.Year <= 9999:
		year = fmt.Sprintf("%04d", f.Year)
	case f.Year < 0:
		year = fmt.Sprintf("-%06d", -f.Year)
	default:
		year = fmt.Sprintf("+%06d", f.Year)
	}
	return fmt.Sprintf("%s-%02d-%02dT%02d:%02d:%02d.%03dZ", year, f.Month+1, f.Day, f.Hour, f.Minute, f.Second,
		f.Millisecond)
}

// TestStringsMatchPrintf writes every string Date has for times across the
// whole range a Date holds -- the ends of it, years before 1 and past 9999,
// the first and last millisecond of days -- in zones east and west of UTC
// and with half-hour offsets, and compares each with what printf wrote.
func TestStringsMatchPrintf(t *testing.T) {
	const limit = 8.64e15
	times := []float64{0, -1, 1, limit, -limit, limit - 1, -limit + 1,
		-62198755200000, -62198755200001, -62135596800000, 253402300799999, 253402300800000,
		-2208988800000, 1e12, -1e12, 951782400000, 1709251199999}
	rng := rand.New(rand.NewSource(1))
	for range 20000 {
		x := (rng.Float64()*2 - 1) * limit
		if rng.Intn(3) == 0 {
			x = (rng.Float64()*2 - 1) * 1e13
		}
		times = append(times, math.Trunc(x))
	}
	// German writes its zone names with letters outside ASCII.
	for _, zone := range []string{"UTC", "America/New_York", "Asia/Kolkata", "Australia/Adelaide",
		"Pacific/Chatham", "America/St_Johns", "Europe/Dublin", "Asia/Kathmandu", "Europe/Berlin"} {
		tz, err := intl.LoadTimeZone(intl.Embedded, zone)
		if err != nil {
			t.Fatalf("%s: %v", zone, err)
		}
		tag := "en-US"
		if zone == "Europe/Berlin" {
			tag = "de-DE"
		}
		loc, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		e, err := New(Options{Locale: &loc, TimeZone: tz})
		if err != nil {
			t.Fatalf("%s: %v", zone, err)
		}
		for _, x := range times {
			if got, want := e.String(x), printfString(e, x); got != want {
				t.Fatalf("String(%v) in %s = %q, want %q", x, zone, got, want)
			}
			if got, want := e.DateString(x), printfDateString(e, x); got != want {
				t.Fatalf("DateString(%v) in %s = %q, want %q", x, zone, got, want)
			}
			if got, want := e.TimeString(x), printfTimeString(e, x); got != want {
				t.Fatalf("TimeString(%v) in %s = %q, want %q", x, zone, got, want)
			}
		}
	}
	for _, x := range times {
		if got, want := UTCString(x), printfUTCString(x); got != want {
			t.Fatalf("UTCString(%v) = %q, want %q", x, got, want)
		}
		if got, ok := ISOString(x); !ok || got != printfISOString(x) {
			t.Fatalf("ISOString(%v) = %q, want %q", x, got, printfISOString(x))
		}
	}
}
