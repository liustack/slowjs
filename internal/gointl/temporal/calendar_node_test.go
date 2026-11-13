//go:build gointl_all

package temporal

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestCalendarsMatchNode replays testdata/temporal_calendars_node.js: every
// year of every Temporal calendar from ISO -3000 to 3000, and at the ends of
// Temporal's range, as Node's Temporal lays it out; and the Japanese era of
// every day from 1868 to 2030.
func TestCalendarsMatchNode(t *testing.T) {
	f, err := os.Open("../testdata/temporal_calendars_node.txt.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	s := bufio.NewScanner(z)
	s.Buffer(nil, 1<<22)
	calendars := map[string]*Calendar{}
	years, bad := 0, 0
	fail := func(format string, args ...any) {
		bad++
		if bad <= 40 {
			t.Errorf(format, args...)
		}
	}
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			if !strings.Contains(line, "icu_calendar 2.2.1") {
				t.Fatalf("the expectations come from %q, not icu_calendar 2.2.1", line)
			}
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		var id string
		json.Unmarshal(c[0], &id)
		if id == "japanese-day" {
			var start int64
			var eras []any
			json.Unmarshal(c[1], &start)
			json.Unmarshal(c[2], &eras)
			cal, _ := NewCalendar("japanese")
			for i := 0; i+1 < len(eras); i += 2 {
				d := ISODateFromEpochDays(start + int64(i/2))
				got := cal.date(d)
				if got.Era != eras[i] || float64(got.EraYear) != eras[i+1] {
					fail("japanese %v: %s %d, want %v %v", d, got.Era, got.EraYear, eras[i], eras[i+1])
				}
			}
			continue
		}
		cal := calendars[id]
		if cal == nil {
			if cal, err = NewCalendar(id); err != nil {
				t.Fatal(err)
			}
			calendars[id] = cal
		}
		var (
			year, eraYear *int
			era           *string
			start         int64
			months, days  int
			leap          bool
			codes         string
			lengths       []int
		)
		json.Unmarshal(c[1], &year)
		json.Unmarshal(c[2], &era)
		json.Unmarshal(c[3], &eraYear)
		json.Unmarshal(c[4], &start)
		json.Unmarshal(c[5], &months)
		json.Unmarshal(c[6], &days)
		json.Unmarshal(c[7], &leap)
		json.Unmarshal(c[8], &codes)
		json.Unmarshal(c[9], &lengths)
		years++
		first := cal.date(ISODateFromEpochDays(start))
		wantEra, wantEraYear := "", 0
		if era != nil {
			wantEra, wantEraYear = *era, *eraYear
		}
		if first.Year != *year || first.Month != 1 || first.Day != 1 || first.Era != wantEra ||
			first.EraYear != wantEraYear || first.MonthsInYear != months || first.DaysInYear != days ||
			first.InLeapYear != leap {
			fail("%s %d starting %v: %+v; want era %s %d, %d months, %d days, leap %v", id, *year,
				ISODateFromEpochDays(start), first, wantEra, wantEraYear, months, days, leap)
			continue
		}
		var codeList []string
		if codes != "" {
			codeList = strings.Split(codes, ",")
		}
		day := start
		for m, n := range lengths {
			if n == 0 {
				break
			}
			code := codeList
			want := ""
			if code != nil {
				want = code[m]
			} else {
				want = month{number: m + 1}.code()
			}
			d := cal.date(ISODateFromEpochDays(day))
			last := cal.date(ISODateFromEpochDays(day + int64(n) - 1))
			if d.Year != *year || d.Month != m+1 || d.Day != 1 || d.MonthCode != want || d.DaysInMonth != n ||
				last.Month != m+1 || last.Day != n {
				fail("%s %d month %d from %v: %+v to %d/%d; want %s of %d days", id, *year, m+1,
					ISODateFromEpochDays(day), d, last.Month, last.Day, want, n)
				break
			}
			day += int64(n)
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if years < 90000 {
		t.Fatalf("%d years: the recording is short", years)
	}
	t.Logf("%d years, %d differ", years, bad)
}

// TestCalendarEpochs are the epochs as Calendrical Calculations gives them.
func TestCalendarEpochs(t *testing.T) {
	for name, c := range map[string]struct{ got, want int64 }{
		"coptic":         {copticEpoch, 103605},
		"persian":        {persianEpoch, 226896},
		"islamic friday": {islamicEpochFriday, 227015},
		"hebrew":         {hebrewEpoch, -1373427},
		"1970-01-01":     {ISODate{1970, 1, 1}.rataDie(), rdEpoch},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", name, c.got, c.want)
		}
	}
}
