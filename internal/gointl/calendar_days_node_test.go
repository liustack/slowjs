//go:build gointl_all

package intl_test

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/gointl"
)

// calendarDayGaps are the calendars testdata/calendar_days_node.txt.gz holds
// that go-intl does not reckon in yet. One that starts to pass is reported,
// so the entry goes.
var calendarDayGaps = map[string]string{}

type calendarRun struct {
	calendar string
	offset   int64
	day      int64
	fields   string
	dayOf    int
}

// TestCalendarDaysMatchNode checks every day from 1600 to 2400, in every
// calendar whose months are not the Gregorian ones, against the dates Node
// gives them: testdata/calendar_days_node.js.
func TestCalendarDaysMatchNode(t *testing.T) {
	if testing.Short() {
		t.Skip("reckons some three million days")
	}
	runs := readCalendarRuns(t, "testdata/calendar_days_node.txt.gz")
	end := runs[len(runs)-1].day
	runs = runs[:len(runs)-1]

	type result struct{ days, wrong int }
	results := map[string]*result{}
	examples := map[string][]string{}
	formats := map[string]*intl.DateTimeFormat{}
	loc, err := intl.ParseLocale("en-u-nu-latn")
	if err != nil {
		t.Fatal(err)
	}
	for i, run := range runs {
		f, ok := formats[run.calendar]
		if !ok {
			f, err = intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
				Calendar: run.calendar, TimeZone: "UTC", Era: intl.WidthShort,
				Year: intl.WidthNumeric, Month: intl.WidthNumeric, Day: intl.WidthNumeric,
				// The days of Node's islamic and islamic-rgsa, which the
				// standard would settle on islamic-civil, and of ICU4C's
				// Chinese astronomy, where the standard's are Temporal's.
				Compat: intl.IslamicFallback | intl.ChineseAstronomy,
			})
			if err != nil {
				if _, gap := calendarDayGaps[run.calendar]; !gap {
					t.Errorf("%s: %v", run.calendar, err)
				}
				f = nil
			}
			formats[run.calendar] = f
		}
		if f == nil {
			continue
		}
		stop := end
		if i+1 < len(runs) && runs[i+1].calendar == run.calendar && runs[i+1].offset == run.offset {
			stop = runs[i+1].day
		}
		key := run.calendar
		if run.offset != 0 {
			key += " late"
		}
		r := results[key]
		if r == nil {
			r = &result{}
			results[key] = r
		}
		for d := run.day; d < stop; d++ {
			want := run.fields + " day=" + strconv.Itoa(run.dayOf+int(d-run.day))
			got := calendarFields(f.FormatToParts(time.UnixMilli(d*86400000 + run.offset).UTC()))
			r.days++
			if got != want {
				r.wrong++
				if len(examples[key]) < 5 {
					examples[key] = append(examples[key], time.UnixMilli(d*86400000).UTC().Format("2006-01-02")+
						": got "+got+", want "+want)
				}
			}
		}
	}

	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := results[k]
		name := strings.TrimSuffix(k, " late")
		_, gap := calendarDayGaps[name]
		switch {
		case r.wrong == 0 && gap:
			t.Errorf("%s: all %d days match; remove the gap", k, r.days)
		case r.wrong > 0 && !gap:
			t.Errorf("%s: %d of %d days differ:\n\t%s", k, r.wrong, r.days, strings.Join(examples[k], "\n\t"))
		default:
			t.Logf("%s: %d of %d days match", k, r.days-r.wrong, r.days)
		}
	}
}

// calendarFields writes a date's parts as the recording does: every part
// but the literals, the day last.
func calendarFields(parts []intl.Part) string {
	var fields []string
	day := ""
	for _, p := range parts {
		switch p.Kind {
		case intl.PartLiteral:
		case intl.PartDay:
			day = p.Value
		default:
			fields = append(fields, string(p.Kind)+"="+p.Value)
		}
	}
	sort.Strings(fields)
	return strings.Join(fields, " ") + " day=" + day
}

func readCalendarRuns(t *testing.T, file string) []calendarRun {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	var runs []calendarRun
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.HasSuffix(line, "ICU 78.3") {
				t.Fatalf("the expectations come from %q, not ICU 78.3", line)
			}
			continue
		}
		var fields []json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil || len(fields) != 5 {
			t.Fatalf("a line that is not five fields: %s", line)
		}
		var r calendarRun
		for i, dst := range []any{&r.calendar, &r.offset, &r.day, &r.fields, &r.dayOf} {
			if err := json.Unmarshal(fields[i], dst); err != nil {
				t.Fatalf("%s: %v", line, err)
			}
		}
		runs = append(runs, r)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 || runs[len(runs)-1].calendar != "" {
		t.Fatal("the recording has no end")
	}
	return runs
}
