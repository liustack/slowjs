//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// In Japanese, in the Japanese calendar, a pattern with a 年 in it writes
// the first year of an era as 元, as ICU's SimpleDateFormat does with its
// jpanyear rules wherever the pattern has no override of its own. go-intl
// had done so only for the date styles: {year, month, day} was
// "令和1年7月23日". Node's answers (ISSUES.md DT-2).
func TestGannen(t *testing.T) {
	d := time.Date(2019, 7, 23, 3, 0, 0, 0, time.UTC)
	e := time.Date(2019, 9, 2, 3, 0, 0, 0, time.UTC)
	z := time.Date(2020, 1, 5, 3, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		tag                string
		opts               intl.DateTimeFormatOptions
		want, near, across string
	}{
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{Year: intl.WidthNumeric, Month: intl.WidthLong,
			Day: intl.WidthNumeric}, "令和元年7月23日", "R1/07/23～1/09/02", "R1/07/23～2/01/05"},
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{Year: intl.WidthNumeric}, "令和元年", "令和元年", "令和元年～2年"},
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{Era: intl.WidthLong, Year: intl.WidthNumeric},
			"令和元年", "令和元年", "令和元年～2年"},
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{Year: intl.Width2Digit, Month: intl.WidthNumeric},
			"R01/7", "R01/07～01/09", "R01/07～02/01"},
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{DateStyle: intl.LengthLong},
			"令和元年7月23日", "R1/07/23～1/09/02", "R1/07/23～2/01/05"},
		{"ja-u-ca-japanese", intl.DateTimeFormatOptions{DateStyle: intl.LengthShort},
			"R1/7/23", "R1/07/23～1/09/02", "R1/07/23～2/01/05"},
		{"ja-JP-u-ca-japanese", intl.DateTimeFormatOptions{Year: intl.WidthNumeric, Month: intl.WidthShort},
			"令和元年7月", "R1/07～1/09", "R1/07～2/01"},
		{"ja-u-ca-japanese-nu-hanidec", intl.DateTimeFormatOptions{Year: intl.WidthNumeric,
			Month: intl.WidthLong}, "令和元年七月", "R一/〇七～一/〇九", "R一/〇七～二/〇一"},
		{"en-u-ca-japanese", intl.DateTimeFormatOptions{Year: intl.WidthNumeric, Month: intl.WidthLong},
			"July 1 Reiwa", "July – September 1 Reiwa", "July 1 – January 2 Reiwa"},
	} {
		c.opts.TimeZone = "UTC"
		f := newDateTime(t, c.tag, c.opts)
		if got := f.Format(d); got != c.want {
			t.Errorf("%s %+v: %q, want %q", c.tag, c.opts, got, c.want)
		}
		for _, r := range []struct {
			to   time.Time
			want string
		}{{e, c.near}, {z, c.across}} {
			if got := f.FormatRange(d, r.to); got != r.want {
				t.Errorf("%s %+v to %v: %q, want %q", c.tag, c.opts, r.to.Format("2006-01-02"), got, r.want)
			}
		}
	}
	// A Temporal value's pattern is made from the skeleton of the
	// formatter's, so {year, month: "long", day}, whose Japanese pattern
	// writes the month as a number before 月, comes out as "R1/7/23", with
	// no 年; where the pattern keeps one, 元 is written.
	date := time.Date(2019, 7, 23, 0, 0, 0, 0, time.UTC)
	until := time.Date(2020, 1, 5, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		opts              intl.DateTimeFormatOptions
		date, month, span string
	}{
		{intl.DateTimeFormatOptions{Year: intl.WidthNumeric}, "令和元年", "令和元年", "令和元年～2年"},
		{intl.DateTimeFormatOptions{DateStyle: intl.LengthFull}, "令和元年7月23日火曜日", "R1/7",
			"R1/07/23(火曜日)～2/01/05(日曜日)"},
		{intl.DateTimeFormatOptions{Year: intl.WidthNumeric, Month: intl.WidthLong, Day: intl.WidthNumeric},
			"R1/7/23", "R1/7", "R1/07/23～2/01/05"},
	} {
		c.opts.Compat = intl.NodeICU
		f := newDateTime(t, "ja-u-ca-japanese", c.opts)
		pd, err := f.ForTemporal(intl.TemporalPlainDate)
		if err != nil {
			t.Fatal(err)
		}
		ym, err := f.ForTemporal(intl.TemporalPlainYearMonth)
		if err != nil {
			t.Fatal(err)
		}
		if got := pd.Format(date); got != c.date {
			t.Errorf("Temporal %+v: %q, want %q", c.opts, got, c.date)
		}
		if got := ym.Format(date); got != c.month {
			t.Errorf("Temporal year and month %+v: %q, want %q", c.opts, got, c.month)
		}
		if got := pd.FormatRange(date, until); got != c.span {
			t.Errorf("Temporal range %+v: %q, want %q", c.opts, got, c.span)
		}
	}

	f := newDateTime(t, "ja-u-ca-japanese", intl.DateTimeFormatOptions{TimeZone: "UTC", Year: intl.WidthNumeric})
	parts := f.FormatToParts(d)
	if len(parts) < 2 || parts[1].Kind != intl.PartYear || parts[1].Value != "元" {
		t.Errorf("parts: %q, want the year 元", parts)
	}
}
