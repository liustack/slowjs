//go:build gointl_all

package temporal

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// isoString is how Temporal writes an ISO date: four digits of year, or a
// sign and six outside 0 to 9999.
func isoString(d ISODate) string {
	if d.Year >= 0 && d.Year <= 9999 {
		return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
	}
	s := "+"
	y := d.Year
	if y < 0 {
		s, y = "-", -y
	}
	return fmt.Sprintf("%s%06d-%02d-%02d", s, y, d.Month, d.Day)
}

func errorName(err error) string {
	switch {
	case errors.Is(err, ErrRange):
		return "RangeError"
	case errors.Is(err, ErrType):
		return "TypeError"
	}
	return "Error"
}

// jsonFields reads a field bag as V8 hands it on.
func jsonFields(raw json.RawMessage) (CalendarFields, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return CalendarFields{}, err
	}
	var f CalendarFields
	for k, v := range m {
		switch k {
		case "era":
			f.Era = String(v.(string))
		case "monthCode":
			f.MonthCode = String(v.(string))
		case "eraYear":
			f.EraYear = Int(int(v.(float64)))
		case "year":
			f.Year = Int(int(v.(float64)))
		case "month":
			f.Month = Int(int(v.(float64)))
		case "day":
			f.Day = Int(int(v.(float64)))
		default:
			return f, fmt.Errorf("field %s", k)
		}
	}
	return f, nil
}

func parseISO(s string) (ISODate, error) {
	var d ISODate
	sign := 1
	if s[0] == '+' || s[0] == '-' {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	if _, err := fmt.Sscanf(s, "%d-%d-%d", &d.Year, &d.Month, &d.Day); err != nil {
		return d, err
	}
	d.Year *= sign
	return d, nil
}

// TestFieldsAndArithmeticMatchNode replays testdata/temporal_fields_node.js:
// dates, year-months and month-days from fields, and adding to dates and
// differencing them, in every calendar.
func TestFieldsAndArithmeticMatchNode(t *testing.T) {
	f, err := os.Open("../testdata/temporal_fields_node.txt.gz")
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
	calendars := map[string]*Calendar{}
	ran, bad := map[string]int{}, map[string]int{}
	shown := 0
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		var c []json.RawMessage
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		var kind, id string
		json.Unmarshal(c[0], &kind)
		json.Unmarshal(c[1], &id)
		cal := calendars[id]
		if cal == nil {
			if cal, err = NewCalendar(id); err != nil {
				t.Fatal(err)
			}
			calendars[id] = cal
		}
		overflowOf := func(raw json.RawMessage) Overflow {
			var o string
			json.Unmarshal(raw, &o)
			if o == "reject" {
				return Reject
			}
			return Constrain
		}
		var got, want string
		switch kind {
		case "date", "ym", "md":
			fields, err := jsonFields(c[2])
			if err != nil {
				t.Fatal(err)
			}
			overflow := overflowOf(c[3])
			json.Unmarshal(c[4], &want)
			var d ISODate
			switch kind {
			case "date":
				d, err = cal.DateFromFields(fields, overflow)
			case "ym":
				d, err = cal.YearMonthFromFields(fields, overflow)
			default:
				d, err = cal.MonthDayFromFields(fields, overflow)
			}
			switch {
			case err != nil:
				got = errorName(err)
			case kind == "ym" && id == "iso8601":
				got = isoString(d)[:len(isoString(d))-3]
			case kind == "md":
				got = isoString(d) + "[u-ca=" + id + "]"
			default:
				got = isoString(d)
			}
		case "add":
			var start string
			var dur map[string]int64
			json.Unmarshal(c[2], &start)
			json.Unmarshal(c[3], &dur)
			json.Unmarshal(c[5], &want)
			d0, err := parseISO(start)
			if err != nil {
				t.Fatal(err)
			}
			d, err := cal.DateAdd(d0, DateDuration{dur["years"], dur["months"], dur["weeks"], dur["days"]}, overflowOf(c[4]))
			if err != nil {
				got = errorName(err)
			} else {
				got = isoString(d)
			}
		case "until":
			var a, b, unit string
			json.Unmarshal(c[2], &a)
			json.Unmarshal(c[3], &b)
			json.Unmarshal(c[4], &unit)
			want = string(c[5])
			one, _ := parseISO(a)
			two, _ := parseISO(b)
			largest := map[string]Unit{"year": Year, "month": Month, "week": Week, "day": Day}[unit]
			d, err := cal.DateUntil(one, two, largest)
			if err != nil {
				got = fmt.Sprintf("%q", errorName(err))
			} else {
				got = fmt.Sprintf("[%d,%d,%d,%d]", d.Years, d.Months, d.Weeks, d.Days)
			}
		default:
			t.Fatalf("a line of kind %q", kind)
		}
		key := kind + " " + id
		ran[key]++
		if got != want {
			bad[key]++
			if shown < 40 {
				shown++
				t.Errorf("%s: got %s", line, got)
			}
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	total, failed := 0, 0
	for k, n := range ran {
		total += n
		failed += bad[k]
		if bad[k] > 0 {
			t.Logf("%s: %d of %d differ", k, bad[k], n)
		}
	}
	if total < 300000 {
		t.Fatalf("%d cases: the recording is short", total)
	}
	t.Logf("%d cases, %d differ", total, failed)
}
