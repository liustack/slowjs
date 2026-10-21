package intl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/liustack/quickjs/internal/gointl/internal/blob"
	"github.com/liustack/quickjs/internal/gointl/internal/layout"
)

// Choosing a pattern, as V8 asks ICU for one.
//
// A caller either asks for whole styles -- a long date, a short time -- or
// names fields. For fields, V8 spells the options as a skeleton, in a fixed
// order and with the hour letter its hour cycle calls for, and hands it to
// ICU's pattern generator (dtpg.go). For styles, ICU's own date formatter
// takes the locale's style patterns and glues them together, and V8
// regenerates the result when the hour cycle it settled on disagrees with
// the one the locale's time pattern counts in. Either way V8 then rewrites
// the hour letters for the cycle, which is where the answer is final.
//
// The code follows V8's js-date-time-format.cc and ICU's smpdtfmt.cpp; the
// comments name the functions it follows.

// hourLetters are the pattern letters of the hour cycles.
var hourLetters = map[HourCycle]byte{H11: 'K', H12: 'h', H23: 'H', H24: 'k'}

// choosePattern settles on the formatter's pattern and the hour cycle
// resolvedOptions reports, which is unset unless an hour or a time style was
// asked for.
func (f *DateTimeFormat) choosePattern(src Source, decimal string) (string, HourCycle, *dtpg, error) {
	hourChar, allowed, err := allowedHourFormats(src, f.locale)
	if err != nil {
		return "", HourCycleAuto, nil, err
	}
	g := newDTPG(f.patterns, f.calendar, f.data.FieldNames, decimal, hourChar, allowed)
	hc := f.resolveHourCycle(g.defaultHourCycle(), allowed)
	f.clock = hc

	o := &f.opts
	if o.DateStyle != LengthNone || o.TimeStyle != LengthNone {
		// DateTimeStylePattern.
		timePattern := ""
		if o.TimeStyle != LengthNone && f.timeStyleAfresh(src) {
			fresh := newDTPGWithoutStyles(f.patterns, f.calendar, f.data.FieldNames, decimal, hourChar, allowed)
			timePattern = fresh.bestPattern(timeStyleSkeletons[o.TimeStyle-1], 0)
		}
		pattern := f.stylePattern(timePattern)
		err := f.styleOverrides(src, timePattern != "")
		if errors.Is(err, ErrNotFound) {
			// DateFormat::create uses SimpleDateFormat's default pattern
			// when a style's algorithmic numbering rules are absent in ICU.
			// Chromium omits Hebrew RBNF. V8 regenerates the result below
			// only when a time style requests another hour cycle.
			pattern = "yMMdd hh:mm a"
			f.overrides = nil
		} else if err != nil {
			return "", HourCycleAuto, nil, err
		}
		if o.TimeStyle == LengthNone {
			return pattern, HourCycleAuto, g, nil
		}
		if hourCycleFromPattern(pattern) == hc {
			return pattern, hc, g, nil
		}
		// A regenerated pattern is a new one, with no overrides.
		f.overrides = nil
		pattern = g.bestPattern(replaceSkeleton(staticSkeleton(pattern), hc), matchHourFieldLength)
		return replaceHourCycleInPattern(pattern, hc), hc, g, nil
	}

	skeleton := v8Skeleton(o, hc)
	if skeleton == "" {
		return "", HourCycleAuto, nil, fmt.Errorf("intl: nothing to format: %w", ErrNotFound)
	}
	patternCycle := HourCycleAuto
	if o.Hour != WidthNone {
		patternCycle = hc
	}
	pattern := g.bestPattern(skeleton, matchHourFieldLength)
	return replaceHourCycleInPattern(pattern, patternCycle), patternCycle, g, nil
}

// resolveHourCycle is ECMA-402's: hour12 wins over hourCycle, which wins
// over the -u-hc keyword, which wins over the locale's default. hour12 picks
// the locale's own twelve- or twenty-four-hour cycle, [[HourCycle12]] and
// [[HourCycle24]] of its data: the first of each that CLDR's time data allows
// for the locale's region. V8 decides the twelve-hour one by whether the
// locale names Japan, where the clock counts from 0 (TwelveHourCycle).
func (f *DateTimeFormat) resolveHourCycle(def HourCycle, allowed []string) HourCycle {
	o := &f.opts
	if o.Hour12 != nil && !o.Compat.Has(TwelveHourCycle) {
		return allowedHourCycle(allowed, *o.Hour12)
	}
	if o.Hour12 != nil {
		if *o.Hour12 {
			if def == H11 || def == H12 {
				return def
			}
			if f.locale.Region.String() == "JP" {
				return H11
			}
			return H12
		}
		if def == H23 || def == H24 {
			return def
		}
		return H23
	}
	if o.HourCycle != HourCycleAuto {
		return o.HourCycle
	}
	if kw, ok := f.locale.keywordValue("hc"); ok {
		if hc, ok := parseHourCycle(kw); ok {
			return hc
		}
	}
	return def
}

// allowedHourCycle is the first twelve-hour cycle a region allows, or the
// first twenty-four-hour one: "H K h" in Japan allows h11 before h12.
func allowedHourCycle(allowed []string, twelve bool) HourCycle {
	for _, a := range allowed {
		if a == "" {
			continue
		}
		switch a[0] {
		case 'h':
			if twelve {
				return H12
			}
		case 'K':
			if twelve {
				return H11
			}
		case 'H':
			if !twelve {
				return H23
			}
		case 'k':
			if !twelve {
				return H24
			}
		}
	}
	if twelve {
		return H12
	}
	return H23
}

func parseHourCycle(s string) (HourCycle, bool) {
	switch s {
	case "h11":
		return H11, true
	case "h12":
		return H12, true
	case "h23":
		return H23, true
	case "h24":
		return H24, true
	}
	return HourCycleAuto, false
}

// v8Skeleton spells the options as a skeleton, in the order of V8's pattern
// data table: weekday, era, year, month, day, day period, hour, minute,
// second, the fraction, then the zone.
func v8Skeleton(o *DateTimeFormatOptions, hc HourCycle) string {
	var b strings.Builder
	name := func(w FieldWidth, letter string) {
		switch w {
		case WidthNarrow:
			b.WriteString(strings.Repeat(letter, 5))
		case WidthLong:
			b.WriteString(strings.Repeat(letter, 4))
		case WidthShort:
			b.WriteString(strings.Repeat(letter, 3))
		}
	}
	number := func(w FieldWidth, letter string) {
		switch w {
		case Width2Digit:
			b.WriteString(letter + letter)
		case WidthNumeric:
			b.WriteString(letter)
		}
	}
	name(o.Weekday, "E")
	name(o.Era, "G")
	number(o.Year, "y")
	if o.Month == WidthNumeric || o.Month == Width2Digit {
		number(o.Month, "M")
	} else {
		name(o.Month, "M")
	}
	number(o.Day, "d")
	switch o.DayPeriod {
	case WidthNarrow:
		b.WriteString("BBBBB")
	case WidthLong:
		b.WriteString("BBBB")
	case WidthShort:
		b.WriteString("B")
	}
	hour := "j"
	if letter, ok := hourLetters[hc]; ok {
		hour = string(letter)
	}
	number(o.Hour, hour)
	number(o.Minute, "m")
	number(o.Second, "s")
	// V8 reads the fraction just before the zone, so it goes there.
	for i := 0; i < o.FractionalSecondDigits && i < 3; i++ {
		b.WriteByte('S')
	}
	b.WriteString(map[ZoneStyle]string{
		ZoneShort: "z", ZoneLong: "zzzz",
		ZoneShortOffset: "O", ZoneLongOffset: "OOOO",
		ZoneShortGeneric: "v", ZoneLongGeneric: "vvvv",
	}[o.TimeZoneName])
	return b.String()
}

// stylePattern is what ICU's SimpleDateFormat builds for a date style, a time
// style or both: the locale's patterns, joined by the "atTime" glue for the
// date's length where there is one.
func (f *DateTimeFormat) stylePattern(timePattern string) string {
	cal := f.calendar
	date, clock := f.opts.DateStyle, f.opts.TimeStyle
	if clock != LengthNone && timePattern == "" {
		timePattern = cal.TimeFormats[clock-1]
	}
	switch {
	case date == LengthNone:
		return timePattern
	case clock == LengthNone:
		return cal.DateFormats[date-1]
	}
	glue := cal.AtTimeFormats[date-1]
	if glue == "" {
		glue = cal.DateTimeFormats[date-1]
	}
	if glue == "" {
		glue = "{1}, {0}"
	}
	return simpleFormat(glue, timePattern, cal.DateFormats[date-1])
}

// timeStyleSkeletons are the skeletons SimpleDateFormat::construct makes a
// time style from, full to short.
var timeStyleSkeletons = [...]string{"jmmsszzzz", "jmmssz", "jmmss", "jmm"}

// timeStyleAfresh is SimpleDateFormat::construct's test for making a time
// style with the pattern generator rather than taking the locale's: the
// locale ICU is given has a -u-hc keyword, or the bundle ICU's ures_open
// settles on for it, the valid locale of its style patterns, is of another
// language, or of another region where the locale names one. So British
// English with -u-hc-h12 writes "7:05:09 am" from its "hms", where the
// stored "HH:mm:ss" with its hours made twelve would be "07:05:09 am". ICU
// has a bundle for "en_US", which cldr-json leaves to "en", so American
// English keeps its own.
//
// The keyword is the one the locale resolves with, which hour12 or
// hourCycle overriding it leaves out; V8 gives ICU the locale's keyword
// whatever the options say (HourCycleStyles).
func (f *DateTimeFormat) timeStyleAfresh(src Source) bool {
	if kw, ok := f.locale.keywordValue("hc"); ok {
		if f.opts.Compat.Has(HourCycleStyles) {
			return true
		}
		hc, valid := parseHourCycle(kw)
		o := &f.opts
		if valid && o.Hour12 == nil && (o.HourCycle == HourCycleAuto || o.HourCycle == hc) {
			return true
		}
	}
	language := f.locale.Language.String()
	if language == "" || language == "und" {
		return false
	}
	fb, err := NewFallbacker(src)
	if err != nil {
		return false
	}
	// The root, where ICU has no bundle for the locale, is a language of
	// its own.
	valid := fb.ChainIn(treeLocales, f.locale.Data())[0]
	if valid.IsRoot() {
		return true
	}
	region := f.locale.Region.String()
	return valid.Language.String() != language || region != "" && region != valid.Region.String()
}

// styleOverrides takes the numbering overrides of the style patterns in use,
// as ICU's date formatter does when it builds one from styles; a time
// style made afresh has none.
func (f *DateTimeFormat) styleOverrides(src Source, freshTime bool) error {
	o := &f.opts
	overrides := map[byte]string{}
	if o.DateStyle != LengthNone {
		parseNumberingOverride(overrides, f.calendar.DateNumbers[o.DateStyle-1], true)
	}
	if o.TimeStyle != LengthNone && !freshTime {
		parseNumberingOverride(overrides, f.calendar.TimeNumbers[o.TimeStyle-1], false)
	}
	if len(overrides) == 0 {
		return nil
	}
	f.overrides = overrides
	f.systems, _ = loadNumberingSystems(src)
	// The systems not written with ten digits are written by rules.
	for _, system := range overrides {
		numeric := false
		for _, s := range f.systems {
			numeric = numeric || s.Name == system
		}
		if !numeric {
			if err := f.loadRules(src, system); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadRules loads a rule-based numbering system an override names.
func (f *DateTimeFormat) loadRules(src Source, system string) error {
	if f.rbnf[system].rules != nil {
		return nil
	}
	rules, set, err := loadRBNF(src, system)
	if err != nil {
		return err
	}
	if f.rbnf == nil {
		f.rbnf = map[string]rbnfSystem{}
	}
	f.rbnf[system] = rbnfSystem{rules, set}
	return nil
}

// gannen reports whether a Japanese year is written by ICU's jpanyear rules,
// which call the first year of an era 元: SimpleDateFormat does so in
// Japanese, in the Japanese calendar, for a pattern with a 年 in it,
// quoted or not, that has no override of its own (smpdtfmt.cpp).
func (f *DateTimeFormat) gannen() bool {
	return f.system == Japanese && f.locale.Language.String() == "ja"
}

// patternOverrides are the numbering overrides a pattern is written with:
// its own, or for one with a 年 in it, the jpanyear rules for the year
// where gannen says so.
func (f *DateTimeFormat) patternOverrides(pattern string, own map[byte]string) map[byte]string {
	if len(own) > 0 || !f.gannen() || !strings.Contains(pattern, "年") {
		return own
	}
	return map[byte]string{'y': "jpanyear"}
}

// hourCycleFromPattern is the cycle of the first hour letter outside quotes.
func hourCycleFromPattern(pattern string) HourCycle {
	inQuote := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\'':
			inQuote = !inQuote
		case 'K':
			if !inQuote {
				return H11
			}
		case 'h':
			if !inQuote {
				return H12
			}
		case 'H':
			if !inQuote {
				return H23
			}
		case 'k':
			if !inQuote {
				return H24
			}
		}
	}
	return HourCycleAuto
}

// replaceSkeleton is V8's ReplaceSkeleton: a skeleton with its hour letters
// changed to the cycle's and its day periods removed (ICU-20437).
func replaceSkeleton(skeleton string, hc HourCycle) string {
	to := hourLetters[hc]
	var b strings.Builder
	for i := 0; i < len(skeleton); i++ {
		switch c := skeleton[i]; c {
		case 'a', 'b', 'B':
		case 'h', 'H', 'K', 'k':
			b.WriteByte(to)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// replaceHourCycleInPattern is V8's ReplaceHourCycleInPattern: every hour
// letter outside quotes becomes the cycle's. V8 also puts a space before an
// hour that directly follows a day, and so does this.
func replaceHourCycleInPattern(pattern string, hc HourCycle) string {
	to, ok := hourLetters[hc]
	if !ok {
		return pattern
	}
	var b strings.Builder
	replace := true
	var last byte
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '\'':
			replace = !replace
			b.WriteByte(c)
		case 'H', 'h', 'K', 'k':
			if replace && last == 'd' {
				b.WriteByte(' ')
			}
			if replace {
				b.WriteByte(to)
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
		last = c
	}
	return b.String()
}

// allowedHourFormats is ICU's getAllowedHourFormats: the hour letter a "j"
// means in this locale and the cycles it allows, from CLDR's time data, keyed
// by the language and region or by the region alone. A -u-hc keyword sets
// the letter.
func allowedHourFormats(src Source, loc Locale) (byte, []string, error) {
	table, err := loadTimeData(src)
	if err != nil {
		return 0, nil, err
	}
	language := loc.Language.String()
	region := regionForSupplementalData(src, loc)
	if language == "" || language == "und" || region == "" {
		if fb, err := NewFallbacker(src); err == nil {
			if full, ok := fb.Maximize(loc.Data()); ok {
				language = full.Language.String()
				region = full.Region.String()
			}
		}
	}
	if language == "" {
		language = "und"
	}
	if region == "" {
		region = "001"
	}
	entry, ok := table.entry(language + "_" + region)
	if !ok {
		entry, ok = table.entry(region)
	}

	var hourChar byte
	if kw, has := loc.keywordValue("hc"); has {
		switch kw {
		case "h24":
			hourChar = 'k'
		case "h23":
			hourChar = 'H'
		case "h12":
			hourChar = 'h'
		case "h11":
			hourChar = 'K'
		}
	}
	if !ok {
		if hourChar == 0 {
			hourChar = 'H'
		}
		return hourChar, []string{"H"}, nil
	}
	if hourChar == 0 {
		switch entry.preferred {
		case "h":
			hourChar = 'h'
		case "K":
			hourChar = 'K'
		case "k":
			hourChar = 'k'
		default:
			hourChar = 'H'
		}
	}
	return hourChar, entry.allowed, nil
}

// regionForSupplementalData is the region a locale's preferences are looked
// up by: the -u-rg keyword's where it names a valid one, else the locale's
// own.
func regionForSupplementalData(src Source, loc Locale) string {
	if r := keywordRegion(src, loc, "rg", false); r != "" {
		return r
	}
	return loc.Region.String()
}

type timeDataEntry struct {
	preferred string
	allowed   []string
}

// timeData is CLDR's hour-cycle preferences, an index read where it lies:
// by "de_AT" or "AT", the preferred cycle and the allowed ones separated by
// commas.
type timeData struct{ index blob.Index }

func loadTimeData(src Source) (timeData, error) {
	b, err := src.Open(MarkerTimeData, DataLocale{})
	if err != nil {
		return timeData{}, fmt.Errorf("intl: the hour-cycle preferences: %w", err)
	}
	index, err := blob.ReadIndex(b, layout.TimeData)
	if err != nil {
		return timeData{}, fmt.Errorf("intl: the hour-cycle preferences: %w", err)
	}
	return timeData{index}, nil
}

func (t timeData) entry(key string) (timeDataEntry, bool) {
	v, ok := t.index.Find(key)
	if !ok {
		return timeDataEntry{}, false
	}
	preferred, allowed, _ := strings.Cut(string(v), " ")
	return timeDataEntry{preferred: preferred, allowed: strings.Split(allowed, ",")}, true
}
