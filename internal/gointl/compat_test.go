//go:build gointl_all

package intl_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Each divergence's flag chooses Node's side of it alone, and every flag has
// an entry in Divergences.
func TestCompatFlags(t *testing.T) {
	var all intl.Compat
	for _, d := range intl.Divergences() {
		if d.Flag == 0 || all&d.Flag != 0 {
			t.Errorf("%s: flag %d is zero or another's", d.Name, d.Flag)
		}
		all |= d.Flag
		if !intl.NodeICU.Has(d.Flag) || intl.Standard.Has(d.Flag) {
			t.Errorf("%s: not in NodeICU, or in Standard", d.Name)
		}
		if d.Flag.String() != d.Name {
			t.Errorf("%s: String() = %q", d.Name, d.Flag.String())
		}
	}
	if all != intl.NodeICU {
		t.Errorf("the divergences make %d, NodeICU is %d", all, intl.NodeICU)
	}
	if got := (intl.NarrowSpace | intl.TwoLetterTags).String(); got != "NarrowSpace|TwoLetterTags" {
		t.Errorf("String() = %q", got)
	}
}

// hour12 in Japanese: ECMA-402 takes the locale's twelve-hour cycle, which
// Japan's time data gives as K, h11; V8 takes h12 for "ja", which names no
// region. Node's is its answer.
func TestTwelveHourCycle(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   string
		cycle  intl.HourCycle
	}{
		{intl.Standard, "午前0:00", intl.H11},
		{intl.TwelveHourCycle, "午前12:00", intl.H12},
	} {
		f := newDateTime(t, "ja", intl.DateTimeFormatOptions{TimeZone: "UTC", Hour: intl.WidthNumeric,
			Minute: intl.Width2Digit, Hour12: intl.Bool(true), Compat: c.compat})
		if got := f.Format(time.Date(2000, 2, 29, 0, 0, 0, 0, time.UTC)); got != c.want {
			t.Errorf("%v: %q, want %q", c.compat, got, c.want)
		}
		if got := f.ResolvedOptions().HourCycle; got != c.cycle {
			t.Errorf("%v: hour cycle %v, want %v", c.compat, got, c.cycle)
		}
	}
	// Where the region allows only one twelve-hour cycle, the two agree.
	for _, c := range []intl.Compat{intl.Standard, intl.NodeICU} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, Hour12: intl.Bool(false), Compat: c})
		if got := f.ResolvedOptions().HourCycle; got != intl.H23 {
			t.Errorf("%v: en with hour12 false is %v", c, got)
		}
	}
}

// A year before the Hijrah: CLDR 48.2's era for it, the year counted back,
// or ICU4C's negative year of the era after. Node's is ICU4C's.
func TestIslamicEras(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "127 Before Hijrah"},
		{intl.IslamicEras, "-126 Anno Hegirae"},
	} {
		for _, calendar := range []string{"islamic", "islamic-civil", "islamic-tbla", "islamic-umalqura"} {
			f := newDateTime(t, "en", intl.DateTimeFormatOptions{Calendar: calendar, TimeZone: "UTC",
				Era: intl.WidthLong, Year: intl.WidthNumeric, Compat: c.compat})
			if got := f.Format(time.Date(500, 1, 1, 0, 0, 0, 0, time.UTC)); got != c.want {
				t.Errorf("%v %s: %q, want %q", c.compat, calendar, got, c.want)
			}
		}
	}
	// After the Hijrah the two agree.
	for _, c := range []intl.Compat{intl.Standard, intl.NodeICU} {
		f := newDateTime(t, "en", intl.DateTimeFormatOptions{Calendar: "islamic-civil", TimeZone: "UTC",
			Era: intl.WidthShort, Year: intl.WidthNumeric, Compat: c})
		if got := f.Format(time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)); got != "1445 AH" {
			t.Errorf("%v: %q", c, got)
		}
	}
}

// Temporal values: an era alone, and an hour cycle with no hour asked for.
// The Node side's expectations are Node's; the standard side's follow the
// Temporal proposal's GetDateTimeFormat.
func TestTemporalFormats(t *testing.T) {
	type value struct {
		kind intl.TemporalKind
		at   time.Time
	}
	instant := value{intl.TemporalInstant, time.UnixMilli(0)}
	date := value{intl.TemporalPlainDate, time.Date(2000, 5, 2, 0, 0, 0, 0, time.UTC)}
	dateTime := value{intl.TemporalPlainDateTime, time.Date(2000, 5, 2, 14, 46, 0, 0, time.UTC)}
	yearMonth := value{intl.TemporalPlainYearMonth, time.Date(2000, 5, 1, 0, 0, 0, 0, time.UTC)}
	clock := value{intl.TemporalPlainTime, time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)}
	any_ := [2]intl.DateTimeComponents{intl.ComponentsAny, intl.ComponentsAll}
	dates := [2]intl.DateTimeComponents{intl.ComponentsDate, intl.ComponentsDate}
	times := [2]intl.DateTimeComponents{intl.ComponentsTime, intl.ComponentsTime}
	for _, c := range []struct {
		name           string
		opts           intl.DateTimeFormatOptions
		method         [2]intl.DateTimeComponents
		value          value
		standard, node string
	}{
		{"era, Instant", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, any_, instant,
			"1/1/1970 A, 12:00:00 AM", "A"},
		{"era, PlainDate", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, dates, date, "5/2/2000 A", "A"},
		{"era, PlainDateTime", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, any_, dateTime,
			"5/2/2000 A, 2:46:00 PM", "A"},
		{"era, PlainYearMonth", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, dates, yearMonth, "5/2000 A", "A"},
		{"era, Intl.DateTimeFormat", intl.DateTimeFormatOptions{Era: intl.WidthNarrow}, [2]intl.DateTimeComponents{},
			date, "5/2/2000 A", "A"},
		{"hour12 false, PlainTime", intl.DateTimeFormatOptions{Hour12: intl.Bool(false)}, times, clock,
			"00:00:00", "12:00:00 AM"},
		{"hour12 false, Instant", intl.DateTimeFormatOptions{Hour12: intl.Bool(false)}, any_, instant,
			"1/1/1970, 00:00:00", "1/1/1970, 12:00:00 AM"},
		{"h24, PlainTime", intl.DateTimeFormatOptions{HourCycle: intl.H24}, times, clock,
			"24:00:00", "12:00:00 AM"},
		{"h11, PlainTime", intl.DateTimeFormatOptions{HourCycle: intl.H11}, times, clock,
			"0:00:00 AM", "12:00:00 AM"},
	} {
		for _, compat := range []intl.Compat{intl.Standard, intl.TemporalFormats} {
			opts := c.opts
			opts.TimeZone, opts.Compat = "UTC", compat
			opts.Required, opts.Defaults = c.method[0], c.method[1]
			f := newDateTime(t, "en", opts)
			k, err := f.ForTemporal(c.value.kind)
			if err != nil {
				t.Fatalf("%s %v: %v", c.name, compat, err)
			}
			want := c.standard
			if compat != intl.Standard {
				want = c.node
			}
			if got := k.Format(c.value.at); got != want {
				t.Errorf("%s %v: %+q, want %+q", c.name, compat, got, want)
			}
		}
	}
}

// "yes" as a keyword's value: UTS #35 makes it "true", and leaves that out,
// only for the keys whose BCP 47 data has the alias; ICU does it for every
// key. The expectations are test262's (unicode-ext-canonicalize-yes-to-true)
// and Node's.
func TestYesValues(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   []string
	}{
		{intl.Standard, []string{"und-u-kb", "und-u-kc", "und-u-kh", "und-u-kk", "und-u-kn",
			"und-u-ka-yes", "und-u-kf-yes", "und-u-kr-yes", "und-u-ks-yes", "und-u-kv-yes"}},
		{intl.YesValues, []string{"und-u-kb", "und-u-kc", "und-u-kh", "und-u-kk", "und-u-kn",
			"und-u-ka", "und-u-kf", "und-u-kr", "und-u-ks", "und-u-kv"}},
	} {
		canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		for i, key := range []string{"kb", "kc", "kh", "kk", "kn", "ka", "kf", "kr", "ks", "kv"} {
			l, err := canon.Canonicalize("und-u-" + key + "-yes")
			if err != nil {
				t.Fatal(err)
			}
			if got := l.String(); got != c.want[i] {
				t.Errorf("%v: und-u-%s-yes is %q, want %q", c.compat, key, got, c.want[i])
			}
		}
	}
}

// A key with no value, which stands for "true": Intl.Locale's getCalendars
// and the like answer the locale's value for it, the empty string (test262's
// preferred-from-unicode-extension-true-empty); V8 answers "true", and with
// YesValues ICU's "yes", which is what Node shows.
func TestTrueKeywords(t *testing.T) {
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, ""},
		{intl.YesValues, ""},
		{intl.TrueKeywords, "true"},
		{intl.TrueKeywords | intl.YesValues, "yes"},
	} {
		info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"ca", "co", "hc", "nu"} {
			l, err := intl.ParseLocale("en-u-" + key)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			switch key {
			case "ca":
				got = info.Calendars(l)
			case "co":
				got, err = info.Collations(l)
			case "hc":
				got, err = info.HourCycles(l)
			case "nu":
				got, err = info.NumberingSystems(l)
			}
			if err != nil || len(got) != 1 || got[0] != c.want {
				t.Errorf("%v: en-u-%s answers %q, %v, want [%q]", c.compat, key, got, err, c.want)
			}
		}
	}
}

// A currency Intl.supportedValuesOf does not list: named only on Node's
// side, as test262's currencies-accepted-by-DisplayNames requires of the
// standard. The Node side's expectation is Node's.
func TestCurrencyNames(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		compat    intl.Compat
		adp, name string
		found     bool
	}{
		{intl.Standard, "ADP", "", false},
		{intl.CurrencyNames, "ADP", "Andorran Peseta", true},
	} {
		d, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{Kind: intl.DisplayCurrency,
			Fallback: intl.FallbackNone, Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := d.Of(c.adp); got != c.name || ok != c.found {
			t.Errorf("%v: %s is %q %v, want %q %v", c.compat, c.adp, got, ok, c.name, c.found)
		}
		// Every listed currency is named on both sides.
		codes, err := intl.Currencies()
		if err != nil {
			t.Fatal(err)
		}
		for _, code := range codes {
			if _, ok := d.Of(code); !ok {
				t.Errorf("%v: listed %s has no name", c.compat, code)
			}
		}
	}
}

// Numeric minutes after numeric hours that are left out: a group of their
// own, as test262's digital-style-with-hours-display-auto-with-zero-hour
// requires of the standard, or joined to what came before, as Node writes.
func TestDurationSeparator(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		compat intl.Compat
		want   [3]string
	}{
		{intl.Standard, [3]string{"1 day, 01:02", "-1 day, 01:02", "01:02"}},
		{intl.DurationSeparator, [3]string{"1 day:01:02", "-1 day:01:02", "01:02"}},
	} {
		opts := intl.DurationFormatOptions{Style: intl.DurationDigital, Compat: c.compat}
		opts.Display[intl.DurationHours] = intl.DurationDisplayAuto
		f, err := intl.NewDurationFormat(loc, opts)
		if err != nil {
			t.Fatal(err)
		}
		for i, d := range []intl.Duration{
			{intl.DurationDays: 1, intl.DurationMinutes: 1, intl.DurationSeconds: 2},
			{intl.DurationDays: -1, intl.DurationMinutes: -1, intl.DurationSeconds: -2},
			{intl.DurationMinutes: 1, intl.DurationSeconds: 2},
		} {
			if got, err := f.Format(d); err != nil || got != c.want[i] {
				t.Errorf("%v %v: %q, %v; want %q", c.compat, d, got, err, c.want[i])
			}
		}
	}
}

// A pattern whose quoted literal holds a field's letter: Portuguese writes a
// long month and a year "MMMM 'de' y". The standard reports the fields of
// the format chosen; Node finds a day in the "de".
func TestLiteralFields(t *testing.T) {
	loc, _ := intl.ParseLocale("pt")
	for _, c := range []struct {
		compat intl.Compat
		day    intl.FieldWidth
	}{
		{intl.Standard, intl.WidthNone},
		{intl.LiteralFields, intl.WidthNumeric},
	} {
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			Year: intl.WidthNumeric, Month: intl.WidthLong, Compat: c.compat,
		})
		if err != nil {
			t.Fatal(err)
		}
		r := f.ResolvedOptions()
		if r.Year != intl.WidthNumeric || r.Month != intl.WidthLong || r.Day != c.day {
			t.Errorf("%v: year %v month %v day %v, want %v %v %v", c.compat,
				r.Year, r.Month, r.Day, intl.WidthNumeric, intl.WidthLong, c.day)
		}
	}
}

// The calendars ECMA-402 deprecates: the standard settles them on one
// AvailableCalendars lists, as test262's
// constructor-options-calendar-islamic-fallback requires, and writes in it;
// Node keeps them.
func TestIslamicFallback(t *testing.T) {
	for _, c := range []struct {
		tag, calendar string
		compat        intl.Compat
		want          string
	}{
		{"en", "islamic", intl.Standard, "islamic-civil"},
		{"en", "islamic-rgsa", intl.Standard, "islamic-civil"},
		{"en-u-ca-islamic", "", intl.Standard, "islamic-civil"},
		{"en", "islamic", intl.IslamicFallback, "islamic"},
		{"en", "islamic-rgsa", intl.IslamicFallback, "islamic-rgsa"},
		{"en-u-ca-islamic", "", intl.IslamicFallback, "islamic"},
	} {
		loc, _ := intl.ParseLocale(c.tag)
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{Calendar: c.calendar, Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.ResolvedOptions().Calendar; got != c.want {
			t.Errorf("%s %q %v: %s, want %s", c.tag, c.calendar, c.compat, got, c.want)
		}
		// The formatter writes in the calendar it reports.
		loc, _ = intl.ParseLocale("en-u-ca-" + c.want)
		want, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{Compat: c.compat})
		if err != nil {
			t.Fatal(err)
		}
		at := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
		if got, w := f.Format(at), want.Format(at); got != w {
			t.Errorf("%s %q %v: wrote %q, want %q", c.tag, c.calendar, c.compat, got, w)
		}
	}
}

// A resolved locale's -u-hc keyword where hour12 or hourCycle was given:
// ResolveLocale drops it for any hour12 and for another hourCycle; V8
// drops it where the formatter's own hour cycle, none without an hour, is
// another.
func TestHourCycleKeyword(t *testing.T) {
	loc, _ := intl.ParseLocale("en-u-hc-h23")
	for _, c := range []struct {
		opts intl.DateTimeFormatOptions
		want [2]string // Standard, HourCycleKeyword
	}{
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, Hour12: intl.Bool(false)}, [2]string{"en", "en-u-hc-h23"}},
		{intl.DateTimeFormatOptions{Hour12: intl.Bool(false)}, [2]string{"en", "en"}},
		{intl.DateTimeFormatOptions{HourCycle: intl.H23}, [2]string{"en-u-hc-h23", "en"}},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, HourCycle: intl.H23}, [2]string{"en-u-hc-h23", "en-u-hc-h23"}},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric, HourCycle: intl.H11}, [2]string{"en", "en"}},
		{intl.DateTimeFormatOptions{Hour: intl.WidthNumeric}, [2]string{"en-u-hc-h23", "en-u-hc-h23"}},
	} {
		for i, compat := range []intl.Compat{intl.Standard, intl.HourCycleKeyword} {
			opts := c.opts
			opts.Compat = compat
			f, err := intl.NewDateTimeFormat(loc, opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := f.ResolvedOptions().Locale; got != c.want[i] {
				t.Errorf("%+v %v: %s, want %s", c.opts, compat, got, c.want[i])
			}
		}
	}
}

// A plain Temporal value whose wall-clock time the formatter's zone skips:
// the standard writes its fields as they are, as test262's
// PlainDate/prototype/toLocaleString/ignore-timezone and its PlainDateTime
// counterpart require; Node writes the instant they name in the zone.
func TestPlainValueZone(t *testing.T) {
	loc, _ := intl.ParseLocale("en-US")
	for _, c := range []struct {
		zone   string
		kind   intl.TemporalKind
		fields [6]int
		want   [2]string // Standard, PlainValueZone
	}{
		{"Pacific/Apia", intl.TemporalPlainDate, [6]int{2011, 12, 30},
			[2]string{"12/30/2011", "12/31/2011"}},
		{"America/Los_Angeles", intl.TemporalPlainDateTime, [6]int{2026, 3, 8, 2, 30},
			[2]string{"3/8/2026, 2:30:00 AM", "3/8/2026, 3:30:00 AM"}},
		{"America/Los_Angeles", intl.TemporalPlainDateTime, [6]int{2026, 7, 8, 2, 30},
			[2]string{"7/8/2026, 2:30:00 AM", "7/8/2026, 2:30:00 AM"}},
	} {
		for i, compat := range []intl.Compat{intl.Standard, intl.PlainValueZone} {
			opts := intl.DateTimeFormatOptions{TimeZone: c.zone, Compat: compat}
			if c.kind == intl.TemporalPlainDate {
				opts.Required, opts.Defaults = intl.ComponentsDate, intl.ComponentsDate
			} else {
				opts.Required, opts.Defaults = intl.ComponentsAny, intl.ComponentsAll
			}
			f, err := intl.NewDateTimeFormat(loc, opts)
			if err != nil {
				t.Fatal(err)
			}
			kf, err := f.ForTemporal(c.kind)
			if err != nil {
				t.Fatal(err)
			}
			v := c.fields
			if got := kf.Format(f.PlainInstant(v[0], time.Month(v[1]), v[2], v[3], v[4], v[5], 0)); got != c.want[i] {
				t.Errorf("%s %v %v: %q, want %q", c.zone, v, compat, got, c.want[i])
			}
		}
	}
}

// The time zone names a DateTimeFormat takes and reports: the IANA
// database's, as given, as test262's timezone-not-canonicalized,
// canonicalize-timezone and timezone-legacy-non-iana require; Node takes
// any ICU knows, and reports ICU's canonical name.
func TestZoneIdentifiers(t *testing.T) {
	for _, c := range []struct {
		zone string
		want [2]string // Standard, ZoneIdentifiers; empty for an error
	}{
		{"Asia/Kolkata", [2]string{"Asia/Kolkata", "Asia/Calcutta"}},
		{"asia/calcutta", [2]string{"Asia/Calcutta", "Asia/Calcutta"}},
		{"Australia/Canberra", [2]string{"Australia/Canberra", "Australia/Sydney"}},
		{"US/PACIFIC", [2]string{"US/Pacific", "America/Los_Angeles"}},
		{"Etc/GMT", [2]string{"Etc/GMT", "UTC"}},
		{"GMT", [2]string{"GMT", "UTC"}},
		{"UTC", [2]string{"UTC", "UTC"}},
		{"+05:30", [2]string{"+05:30", "+05:30"}},
		{"ACT", [2]string{"", "Australia/Darwin"}},
		{"SystemV/AST4ADT", [2]string{"", "SystemV/AST4ADT"}},
		{"Mars/Base", [2]string{"", ""}},
	} {
		for i, compat := range []intl.Compat{intl.Standard, intl.ZoneIdentifiers} {
			got, err := intl.ResolveTimeZone(intl.Embedded, c.zone, compat)
			f, ferr := intl.NewDateTimeFormat(intl.Locale{}, intl.DateTimeFormatOptions{TimeZone: c.zone, Compat: compat})
			if c.want[i] == "" {
				if err == nil || ferr == nil {
					t.Errorf("%s %v: %q, %v, want an error", c.zone, compat, got, ferr)
				}
				continue
			}
			if err != nil || ferr != nil {
				t.Errorf("%s %v: %v, %v", c.zone, compat, err, ferr)
				continue
			}
			if got != c.want[i] || f.ResolvedOptions().TimeZone != c.want[i] {
				t.Errorf("%s %v: %q and %q, want %q", c.zone, compat, got, f.ResolvedOptions().TimeZone, c.want[i])
			}
		}
	}
	// A timeZone given as "" is no zone, on either side.
	for _, compat := range []intl.Compat{intl.Standard, intl.ZoneIdentifiers} {
		if got, err := intl.ResolveTimeZone(intl.Embedded, "", compat); err == nil {
			t.Errorf("%v: \"\" is %q, want an error", compat, got)
		}
	}
	// A link is written as the zone it links to.
	at := time.UnixMilli(0)
	for _, compat := range []intl.Compat{intl.Standard, intl.ZoneIdentifiers} {
		var out []string
		for _, zone := range []string{"Asia/Calcutta", "Asia/Kolkata"} {
			f, err := intl.NewDateTimeFormat(intl.Locale{}, intl.DateTimeFormatOptions{
				TimeZone: zone, Hour: intl.WidthNumeric, TimeZoneName: intl.ZoneLong, Compat: compat,
			})
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, f.Format(at))
		}
		if out[0] != out[1] {
			t.Errorf("%v: %q and %q differ", compat, out[0], out[1])
		}
	}
}

// A coptic year before the era: the standard's one era, as test262's
// formatToParts/era requires; ICU4C's era no data names.
func TestCopticEra(t *testing.T) {
	loc, _ := intl.ParseLocale("en")
	for _, c := range []struct {
		compat intl.Compat
		want   [2]string
	}{
		{intl.Standard, [2]string{"-34 Anno Martyrum", "1741 Anno Martyrum"}},
		{intl.CopticEra, [2]string{"35", "1741 Anno Martyrum"}},
	} {
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			Calendar: "coptic", Era: intl.WidthLong, Year: intl.WidthNumeric, TimeZone: "UTC", Compat: c.compat,
		})
		if err != nil {
			t.Fatal(err)
		}
		for i, at := range []time.Time{time.Date(250, 6, 15, 0, 0, 0, 0, time.UTC), time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)} {
			if got := strings.TrimSpace(f.Format(at)); got != c.want[i] {
				t.Errorf("%v %d: %q, want %q", c.compat, at.Year(), got, c.want[i])
			}
		}
	}
}

// A style whose fields a Temporal value has every one of: the standard keeps
// the style's format, as a Date is written, as test262's
// datestyle-not-adjusted-when-no-conflicting-options requires; Node makes
// one again from its skeleton.
func TestTemporalStyleKept(t *testing.T) {
	loc, _ := intl.ParseLocale("ja")
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "1970年1月1日木曜日"},
		{intl.TemporalFormats, "1970/1/1木曜日"},
	} {
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			DateStyle: intl.LengthFull, TimeZone: "UTC", Compat: c.compat,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(time.UnixMilli(0)); got != "1970年1月1日木曜日" {
			t.Errorf("%v: a Date is %q", c.compat, got)
		}
		for _, kind := range []intl.TemporalKind{intl.TemporalPlainDate, intl.TemporalPlainDateTime, intl.TemporalInstant} {
			kf, err := f.ForTemporal(kind)
			if err != nil {
				t.Fatal(err)
			}
			if got := kf.Format(f.PlainInstant(1970, 1, 1, 0, 0, 0, 0)); got != c.want {
				t.Errorf("%v kind %d: %q, want %q", c.compat, kind, got, c.want)
			}
		}
	}
}

// The days of the Chinese calendar: Temporal's on the standard side, as
// test262's compare-to-temporal-lunisolar requires, where ICU4C's astronomy
// makes ISO 2030-03-03 the 30th of the first month.
func TestChineseAstronomy(t *testing.T) {
	loc, _ := intl.ParseLocale("en-u-ca-chinese")
	at := time.Date(2030, 3, 3, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		compat intl.Compat
		want   string
	}{
		{intl.Standard, "1/29"},
		{intl.ChineseAstronomy, "1/30"},
	} {
		f, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{
			Month: intl.WidthNumeric, Day: intl.WidthNumeric, TimeZone: "UTC", Compat: c.compat,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(at); got != c.want {
			t.Errorf("%v: %q, want %q", c.compat, got, c.want)
		}
	}
}

// The region of a locale's hour cycles: ECMA-402's RegionPreference, the
// "-u-rg-" override, the region subtag, the "-u-sd-" subdivision, the
// likely region, then the world's, as test262's getHourCycles/region-priority
// requires; ICU's pattern generator, and so Node, passes over the
// subdivision.
func TestSubdivisionHourCycles(t *testing.T) {
	for _, c := range []struct {
		tag  string
		want [2]string // Standard, SubdivisionHourCycles
	}{
		{"en-US-u-sd-gbeng-rg-gbzzzz", [2]string{"h23", "h23"}},
		{"en-US-u-sd-gbeng", [2]string{"h12", "h12"}},
		{"en-u-sd-gbeng", [2]string{"h23", "h12"}},
		{"en", [2]string{"h12", "h12"}},
		{"eo", [2]string{"h23", "h23"}},
	} {
		for i, compat := range []intl.Compat{intl.Standard, intl.SubdivisionHourCycles} {
			info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{Compat: compat})
			if err != nil {
				t.Fatal(err)
			}
			l, err := intl.ParseLocale(c.tag)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := info.HourCycles(l); err != nil || len(got) != 1 || got[0] != c.want[i] {
				t.Errorf("%s %v: %v, %v, want [%s]", c.tag, compat, got, err, c.want[i])
			}
		}
	}
}

// The lists Intl.supportedValuesOf answers with where they differ: the
// calendars a DateTimeFormat resolves to themselves, and the time zones in
// no region, on the standard side, as test262's
// calendars-accepted-by-DateTimeFormat and timeZones-include-non-continental
// require; Node's, which list islamic and islamic-rgsa and leave out UTC and
// Etc/GMT's.
func TestSupportedValues(t *testing.T) {
	for _, c := range []struct {
		compat             intl.Compat
		calendars, zones   int
		islamic, nonRegion bool
	}{
		{intl.Standard, 16, 445, false, true},
		{intl.IslamicFallback | intl.RegionZones, 18, 418, true, false},
	} {
		calendars := intl.Calendars(c.compat)
		zones, err := intl.TimeZones(c.compat)
		if err != nil {
			t.Fatal(err)
		}
		if len(calendars) != c.calendars || slices.Contains(calendars, "islamic") != c.islamic ||
			slices.Contains(calendars, "islamic-rgsa") != c.islamic {
			t.Errorf("%v: %d calendars %v", c.compat, len(calendars), calendars)
		}
		nonRegion := slices.Contains(zones, "UTC") && slices.Contains(zones, "Etc/GMT+12") &&
			slices.Contains(zones, "Etc/GMT-14")
		if len(zones) != c.zones || nonRegion != c.nonRegion || !slices.IsSorted(zones) {
			t.Errorf("%v: %d zones, UTC and Etc/GMT's %v", c.compat, len(zones), nonRegion)
		}
		// Each zone listed is one a DateTimeFormat resolves to itself.
		for _, zone := range zones {
			if got, err := intl.ResolveTimeZone(intl.Embedded, zone, c.compat); err != nil || got != zone {
				t.Errorf("%v: %s resolves to %q, %v", c.compat, zone, got, err)
			}
		}
		for _, cal := range calendars {
			f, err := intl.NewDateTimeFormat(intl.Locale{}, intl.DateTimeFormatOptions{Calendar: cal, Compat: c.compat})
			if err != nil || f.ResolvedOptions().Calendar != cal {
				t.Errorf("%v: %s resolves otherwise, %v", c.compat, cal, err)
			}
		}
	}
}
