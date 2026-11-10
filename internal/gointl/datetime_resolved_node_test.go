//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestDateTimeResolvedMatchesNode holds ResolvedOptions to what Node's
// resolvedOptions reports, the fields read from the pattern among it. The
// expectations are written by testdata/datetime_resolved_node.js.
func TestDateTimeResolvedMatchesNode(t *testing.T) {
	f, err := os.Open("testdata/datetime_resolved_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(make([]byte, 1<<20), 1<<20)

	var ran, matched int
	var differences []string
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var c struct {
			tag  string
			opts map[string]any
			want map[string]any
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 3 {
			t.Fatalf("a line that is not three fields: %s", line)
		}
		for i, dst := range []any{&c.tag, &c.opts, &c.want} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: field %d: %v", line, i, err)
			}
		}
		ran++

		opts, err := dateTimeOptions(c.opts)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		var got any
		if dtf, err := intl.NewDateTimeFormat(loc, opts); err != nil {
			got = err.Error()
		} else {
			got = normalizeJSON(resolvedAsNode(dtf.ResolvedOptions()))
		}
		if reflect.DeepEqual(got, any(c.want)) {
			matched++
			continue
		}
		gotText, _ := json.Marshal(got)
		differences = append(differences, fmt.Sprintf("%s %s\n   got  %s\n   want %s",
			c.tag, fields[1], gotText, fields[2]))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d of %d match Node exactly", matched, ran)
	if len(differences) > 0 {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 25 && os.Getenv("ALLDIFF") == "" {
			shown = shown[:25]
		}
		t.Errorf("%d differences:\n%s", len(differences), strings.Join(shown, "\n"))
	}
}

// resolvedAsNode is what resolvedOptions makes of a formatter's resolved
// options, but for the time zone.
func resolvedAsNode(r intl.ResolvedDateTimeFormat) map[string]any {
	out := map[string]any{
		"locale": r.Locale, "calendar": r.Calendar, "numberingSystem": r.NumberingSystem,
	}
	if r.HourCycle != intl.HourCycleAuto {
		out["hourCycle"] = [...]string{"", "h11", "h12", "h23", "h24"}[r.HourCycle]
		out["hour12"] = r.HourCycle == intl.H11 || r.HourCycle == intl.H12
	}
	widths := [...]string{"", "numeric", "2-digit", "long", "short", "narrow"}
	for name, w := range map[string]intl.FieldWidth{
		"weekday": r.Weekday, "era": r.Era, "year": r.Year, "month": r.Month, "day": r.Day,
		"dayPeriod": r.DayPeriod, "hour": r.Hour, "minute": r.Minute, "second": r.Second,
	} {
		if w != intl.WidthNone {
			out[name] = widths[w]
		}
	}
	if r.FractionalSecondDigits > 0 {
		out["fractionalSecondDigits"] = r.FractionalSecondDigits
	}
	if r.TimeZoneName != intl.ZoneNone {
		out["timeZoneName"] = [...]string{"", "short", "long", "shortOffset", "longOffset",
			"shortGeneric", "longGeneric"}[r.TimeZoneName]
	}
	lengths := [...]string{"", "full", "long", "medium", "short"}
	if r.DateStyle != intl.LengthNone {
		out["dateStyle"] = lengths[r.DateStyle]
	}
	if r.TimeStyle != intl.LengthNone {
		out["timeStyle"] = lengths[r.TimeStyle]
	}
	return out
}
