//go:build gointl_all

package intl_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

func TestDateTimeFormatMatchesICU(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatalf("loading the golden file: %v", err)
	}

	var ran, matched int
	var differences []string
	for _, c := range f.Cases {
		if c.Service != "DateTimeFormat" || c.Method != "format" {
			continue
		}
		opts, err := dateTimeOptions(c.Options)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		loc, err := intl.ParseLocale(c.Locale)
		if err != nil {
			t.Errorf("%s: %v", c.Source, err)
			continue
		}
		millis, ok := c.Number()
		if !ok {
			t.Errorf("%s: no instant", c.Source)
			continue
		}

		ran++
		dtf, err := intl.NewDateTimeFormat(loc, opts)
		if err != nil {
			differences = append(differences, c.Source+"\n   failed: "+err.Error())
			continue
		}
		got := dtf.Format(time.UnixMilli(int64(millis)).UTC())
		if got == c.Want {
			matched++
		} else {
			differences = append(differences, fmt.Sprintf("%s\n   got  %q\n   want %q",
				c.Source, got, c.Want))
		}
	}

	if ran == 0 {
		t.Fatal("no DateTimeFormat cases ran")
	}
	t.Logf("%d of %d DateTimeFormat cases match ICU exactly (%.2f%%)",
		matched, ran, 100*float64(matched)/float64(ran))
	if matched != ran {
		sort.Strings(differences)
		shown := differences
		if len(shown) > 20 {
			shown = shown[:20]
		}
		t.Errorf("%d of %d cases differ:\n%s", ran-matched, ran,
			strings.Join(shown, "\n"))
	}
}

// dateTimeOptions reads an option bag from the corpus or a node file. Both
// hold Node's answers, so the profile is Node's.
func dateTimeOptions(in map[string]any) (intl.DateTimeFormatOptions, error) {
	out := intl.DateTimeFormatOptions{Compat: intl.NodeICU}
	length := func(v any) (intl.DateTimeLength, error) {
		switch v {
		case "full":
			return intl.LengthFull, nil
		case "long":
			return intl.LengthLong, nil
		case "medium":
			return intl.LengthMedium, nil
		case "short":
			return intl.LengthShort, nil
		}
		return 0, fmt.Errorf("length %v", v)
	}
	width := func(v any) (intl.FieldWidth, error) {
		switch v {
		case "numeric":
			return intl.WidthNumeric, nil
		case "2-digit":
			return intl.Width2Digit, nil
		case "long":
			return intl.WidthLong, nil
		case "short":
			return intl.WidthShort, nil
		case "narrow":
			return intl.WidthNarrow, nil
		}
		return 0, fmt.Errorf("width %v", v)
	}

	var err error
	for key, value := range in {
		switch key {
		case "timeZone":
			out.TimeZone, _ = value.(string)
		case "dateStyle":
			out.DateStyle, err = length(value)
		case "timeStyle":
			out.TimeStyle, err = length(value)
		case "weekday":
			out.Weekday, err = width(value)
		case "era":
			out.Era, err = width(value)
		case "year":
			out.Year, err = width(value)
		case "month":
			out.Month, err = width(value)
		case "day":
			out.Day, err = width(value)
		case "hour":
			out.Hour, err = width(value)
		case "minute":
			out.Minute, err = width(value)
		case "second":
			out.Second, err = width(value)
		case "timeZoneName":
			switch value {
			case "short":
				out.TimeZoneName = intl.ZoneShort
			case "long":
				out.TimeZoneName = intl.ZoneLong
			case "shortOffset":
				out.TimeZoneName = intl.ZoneShortOffset
			case "longOffset":
				out.TimeZoneName = intl.ZoneLongOffset
			case "shortGeneric":
				out.TimeZoneName = intl.ZoneShortGeneric
			case "longGeneric":
				out.TimeZoneName = intl.ZoneLongGeneric
			default:
				err = fmt.Errorf("timeZoneName %v", value)
			}
		case "dayPeriod":
			out.DayPeriod, err = width(value)
		case "fractionalSecondDigits":
			if n, ok := value.(float64); ok {
				out.FractionalSecondDigits = int(n)
			}
		case "hour12":
			if on, ok := value.(bool); ok {
				out.Hour12 = intl.Bool(on)
			}
		case "calendar":
			out.Calendar, _ = value.(string)
		case "numberingSystem":
			out.NumberingSystem, _ = value.(string)
		case "hourCycle":
			switch value {
			case "h11":
				out.HourCycle = intl.H11
			case "h12":
				out.HourCycle = intl.H12
			case "h23":
				out.HourCycle = intl.H23
			case "h24":
				out.HourCycle = intl.H24
			default:
				return out, fmt.Errorf("hourCycle %v", value)
			}
		default:
			return out, fmt.Errorf("unknown option %q", key)
		}
		if err != nil {
			return out, fmt.Errorf("%s: %w", key, err)
		}
	}
	return out, nil
}
