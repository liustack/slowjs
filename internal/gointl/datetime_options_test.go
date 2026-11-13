//go:build gointl_all

package intl_test

import (
	"errors"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// DateTimeFormat refuses what InitializeDateTimeFormat refuses, with
// ErrOption: a field in a width its table does not give it, more than three
// fractional digits, a value no option has, and a calendar name that is not
// a Unicode type. DateStyle 9 had panicked, an hour "long" written no hour,
// and an era "2-digit" been dropped. A calendar is canonicalized as the
// -u-ca- keyword is: "islamicc" is islamic-civil, where go-intl had fallen
// back to the Gregorian. Node's answers for the calendars (ISSUES.md API-2).
func TestDateTimeOptionsRefused(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for name, opts := range map[string]intl.DateTimeFormatOptions{
		"date style 9":        {DateStyle: intl.DateTimeLength(9)},
		"hour long":           {Hour: intl.WidthLong},
		"era 2-digit":         {Era: intl.Width2Digit},
		"weekday numeric":     {Weekday: intl.WidthNumeric},
		"day short":           {Day: intl.WidthShort},
		"width 9":             {Month: intl.FieldWidth(9)},
		"fractional 4":        {FractionalSecondDigits: 4},
		"fractional -1":       {FractionalSecondDigits: -1},
		"hour cycle 9":        {HourCycle: intl.HourCycle(9), Hour: intl.WidthNumeric},
		"zone name 99":        {TimeZoneName: intl.ZoneStyle(99)},
		"calendar gregorian":  {Calendar: "gregorian"},
		"calendar ab":         {Calendar: "ab"},
		"required components": {Required: intl.DateTimeComponents(9)},
	} {
		if _, err := intl.NewDateTimeFormat(en, opts); !errors.Is(err, intl.ErrOption) {
			t.Errorf("%s: %v, want ErrOption", name, err)
		}
	}
	for _, c := range []struct{ calendar, want string }{
		{"islamicc", "islamic-civil"},
		{"ethiopic-amete-alem", "ethioaa"},
		{"islamic-civil", "islamic-civil"},
		{"abc", "gregory"},
		{"GREGORY", "gregory"},
	} {
		f, err := intl.NewDateTimeFormat(en, intl.DateTimeFormatOptions{Calendar: c.calendar})
		if err != nil {
			t.Errorf("calendar %s: %v", c.calendar, err)
			continue
		}
		if got := f.ResolvedOptions().Calendar; got != c.want {
			t.Errorf("calendar %s: %s, want %s", c.calendar, got, c.want)
		}
	}
	for name, opts := range map[string]intl.DateTimeFormatOptions{
		"month narrow":  {Month: intl.WidthNarrow},
		"month numeric": {Month: intl.WidthNumeric},
		"fractional 3":  {FractionalSecondDigits: 3},
		"weekday long":  {Weekday: intl.WidthLong},
	} {
		if _, err := intl.NewDateTimeFormat(en, opts); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
