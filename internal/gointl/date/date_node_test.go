//go:build gointl_all

package date

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestDateMatchesNode replays testdata/date_node.js: toString,
// toDateString, toTimeString and getTimezoneOffset about every transition
// of every zone Node knows and at the ends of time, dates made from local
// fields across every transition, Date.parse, toUTCString and toISOString.
func TestDateMatchesNode(t *testing.T) {
	f, err := os.Open("../testdata/date_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(nil, 1<<20)
	var now time.Time
	var loc intl.Locale
	var env *Environment
	// Setting process.env.TZ empties V8's offset cache, so each zone line
	// starts a new Environment.
	envFor := func(zone string) *Environment {
		tz, err := intl.LoadTimeZone(intl.Embedded, zone)
		if err != nil {
			t.Fatalf("%s: %v", zone, err)
		}
		e, err := New(Options{Locale: &loc, TimeZone: tz, Now: func() time.Time { return now }})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	zone := ""
	parseEnvs := map[string]*Environment{}
	ran, bad := map[string]int{}, map[string]int{}
	fail := func(kind, format string, args ...any) {
		bad[kind]++
		if bad[kind] <= 15 {
			t.Errorf("%s %s: "+format, append([]any{kind, zone}, args...)...)
		}
	}
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			var ms int64
			var tag string
			i := strings.Index(line, "now ")
			if i < 0 || !strings.Contains(line, "ICU 78.3, tz 2026c") {
				t.Fatalf("the expectations come from %q", line)
			}
			fmt.Sscanf(line[i:], "now %d, locale %s", &ms, &tag)
			now = time.UnixMilli(ms)
			if loc, err = intl.ParseLocale(tag); err != nil {
				t.Fatal(err)
			}
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		var kind string
		json.Unmarshal(c[0], &kind)
		ran[kind]++
		switch kind {
		case "zone":
			json.Unmarshal(c[1], &zone)
			env = envFor(zone)
		case "str":
			var tv float64
			var str, ds, ts string
			var offset float64
			json.Unmarshal(c[1], &tv)
			json.Unmarshal(c[2], &str)
			json.Unmarshal(c[3], &ds)
			json.Unmarshal(c[4], &ts)
			json.Unmarshal(c[5], &offset)
			// new Date(t) reads its local time through the cache before
			// any of the four do.
			tv = env.NewDate(tv).Value()
			if got := env.String(tv); got != str {
				fail(kind, "String(%v) = %q, want %q", tv, got, str)
			}
			if got := env.DateString(tv); got != ds {
				fail(kind, "DateString(%v) = %q, want %q", tv, got, ds)
			}
			if got := env.TimeString(tv); got != ts {
				fail(kind, "TimeString(%v) = %q, want %q", tv, got, ts)
			}
			if got := env.TimezoneOffset(tv); got != offset {
				fail(kind, "TimezoneOffset(%v) = %v, want %v", tv, got, offset)
			}
		case "local":
			var fields []float64
			var want *float64
			json.Unmarshal(c[1], &fields)
			json.Unmarshal(c[2], &want)
			for len(fields) < 7 {
				v := 0.0
				if len(fields) == 2 {
					v = 1
				}
				fields = append(fields, v)
			}
			year := fields[0]
			// new Date(y, ...) takes a year of 0 to 99 as 1900 on.
			if y := math.Trunc(year); y >= 0 && y <= 99 {
				year = 1900 + y
			}
			local := MakeDate(MakeDay(year, fields[1], fields[2]), MakeTime(fields[3], fields[4], fields[5], fields[6]))
			// new Date(y, m, ...) reads the local time of what it made.
			got := env.NewDate(env.UTC(local)).Value()
			if want == nil && !math.IsNaN(got) || want != nil && got != *want {
				fail(kind, "UTC of %v = %v, want %v", fields, got, want)
			}
		case "parse":
			var pz string
			var str []uint16
			var want *float64
			json.Unmarshal(c[1], &pz)
			json.Unmarshal(c[2], &str)
			json.Unmarshal(c[3], &want)
			if parseEnvs[pz] == nil {
				parseEnvs[pz] = envFor(pz)
			}
			got := parseEnvs[pz].Parse(str)
			if want == nil && !math.IsNaN(got) || want != nil && got != *want {
				fail(kind, "Parse(%q) in %s = %v, want %v", string(utf16Runes(str)), pz, got, want)
			}
		case "utc":
			var tv *float64
			var utc string
			var iso *string
			json.Unmarshal(c[1], &tv)
			json.Unmarshal(c[2], &utc)
			json.Unmarshal(c[3], &iso)
			v := math.NaN()
			if tv != nil {
				v = TimeClip(*tv)
			}
			if got := UTCString(v); got != utc {
				fail(kind, "UTCString(%v) = %q, want %q", v, got, utc)
			}
			gotISO, ok := ISOString(v)
			if iso == nil && ok || iso != nil && gotISO != *iso {
				fail(kind, "ISOString(%v) = %q %v, want %v", v, gotISO, ok, iso)
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	total := 0
	for k, n := range ran {
		total += n
		if bad[k] > 0 {
			t.Logf("%s: %d of %d differ", k, bad[k], n)
		}
	}
	if total < 100000 {
		t.Fatalf("%d cases: the recording is short", total)
	}
	t.Logf("%d cases", total)
}

func utf16Runes(u []uint16) []rune {
	var out []rune
	for _, c := range u {
		out = append(out, rune(c))
	}
	return out
}
