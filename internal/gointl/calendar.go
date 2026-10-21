package intl

import (
	"fmt"
	"strings"
	"time"

	"github.com/liustack/quickjs/internal/gointl/internal/eastasian"
)

// The calendars a date may be reckoned in.
//
// A calendar is two things: an arithmetic, which says what year and month an
// instant falls in, and a set of names and patterns, which the locale supplies
// separately for each one. The names live in the date data; the arithmetic is
// here and in the cal*.go files beside it, each ICU 78's.
//
// All of CLDR's are implemented. The Gregorian one needs no arithmetic of its
// own, since that is what Go's time package already reckons in; the others
// are reckoned from the Julian day, or, for the lunar ones, from the sun and
// the moon.

// A CalendarSystem is a way of reckoning dates.
type CalendarSystem string

const (
	// Gregory is the Gregorian calendar, and the default nearly everywhere.
	Gregory CalendarSystem = "gregory"
	// Buddhist counts its years from 543 BC, and is what Thailand reckons in.
	Buddhist CalendarSystem = "buddhist"
	// Persian is the solar hijri calendar of Iran and Afghanistan.
	Persian CalendarSystem = "persian"
	// Coptic, Ethiopic and EthiopicAmeteAlem count thirteen months, the
	// last of five or six days.
	Coptic            CalendarSystem = "coptic"
	Ethiopic          CalendarSystem = "ethiopic"
	EthiopicAmeteAlem CalendarSystem = "ethioaa"
	// Indian is the Indian national calendar, the Saka era.
	Indian CalendarSystem = "indian"
	// IslamicCivil and IslamicTabular are the arithmetic Islamic calendars,
	// counted from one day apart.
	IslamicCivil   CalendarSystem = "islamic-civil"
	IslamicTabular CalendarSystem = "islamic-tbla"
	// Islamic and IslamicRGSA are the astronomical Islamic calendar, whose
	// months start when the moon is past new at midnight, reckoned alike;
	// IslamicUmmAlQura is Saudi Arabia's, from its published table.
	Islamic          CalendarSystem = "islamic"
	IslamicRGSA      CalendarSystem = "islamic-rgsa"
	IslamicUmmAlQura CalendarSystem = "islamic-umalqura"
	// ROC counts the Gregorian years from 1912, the Republic of China.
	ROC CalendarSystem = "roc"
	// Hebrew is the lunisolar Hebrew calendar.
	Hebrew CalendarSystem = "hebrew"
	// Japanese counts the Gregorian years by the eras of Japan's reigns.
	Japanese CalendarSystem = "japanese"
	// ISO8601 is the Gregorian calendar written in ISO 8601's order, with
	// weeks from Monday.
	ISO8601 CalendarSystem = "iso8601"
	// Chinese and Dangi are the lunisolar calendars of China and Korea,
	// reckoned alike in their own time zones.
	Chinese CalendarSystem = "chinese"
	Dangi   CalendarSystem = "dangi"
)

// implemented lists the calendars this can reckon in, which is all of CLDR's.
// A name not among them is passed over when a formatter is built, as
// ECMA-402 passes over a calendar a locale does not support.
var implemented = map[CalendarSystem]bool{
	Gregory:           true,
	Buddhist:          true,
	Persian:           true,
	Coptic:            true,
	Ethiopic:          true,
	EthiopicAmeteAlem: true,
	Indian:            true,
	IslamicCivil:      true,
	IslamicTabular:    true,
	Islamic:           true,
	IslamicRGSA:       true,
	IslamicUmmAlQura:  true,
	ROC:               true,
	Hebrew:            true,
	Japanese:          true,
	ISO8601:           true,
	Chinese:           true,
	Dangi:             true,
}

// calendarPreferences maps a region to the calendar it reckons in, as lines of
// "region calendar". It is one table for every locale, since which calendar a
// place uses is not a matter of language.
type calendarPreferences string

func (p calendarPreferences) of(region string) (CalendarSystem, bool) {
	if list := p.all(region); len(list) > 0 {
		return CalendarSystem(list[0]), true
	}
	return "", false
}

// all is every calendar the region reckons in, most preferred first.
func (p calendarPreferences) all(region string) []string {
	if region == "" {
		return nil
	}
	// Only the region's own line is taken apart.
	for line := range strings.SplitSeq(strings.TrimRight(string(p), "\n"), "\n") {
		if name, rest, ok := strings.Cut(line, " "); ok && name == region {
			if fields := strings.Fields(rest); len(fields) > 0 {
				return fields
			}
		}
	}
	return nil
}

// isIslamic reports whether a calendar is one of the Islamic ones.
func isIslamic(c CalendarSystem) bool {
	switch c {
	case Islamic, IslamicCivil, IslamicTabular, IslamicRGSA, IslamicUmmAlQura:
		return true
	}
	return false
}

// chooseCalendar settles which calendar a formatter reckons in, as
// ECMA-402's ResolveLocale settles the "ca" key.
//
// The option wins if it names a calendar, then the locale's own "-u-ca-"
// extension if that does, then the calendar the locale's region uses, and
// failing all of that the Gregorian one. A name that is well formed but no
// calendar's is passed over, as Node passes over "foo"; one that is not a
// Unicode locale type at all is an error, ECMA-402's RangeError. A locale
// with no region of its own is given the one its likely subtags supply, so
// that "th" reckons as "th-TH" does.
//
// It also returns the "-u-ca-" value the resolved locale keeps: the
// keyword's, when that is the calendar chosen.
func chooseCalendar(src Source, loc Locale, asked string) (CalendarSystem, string, error) {
	if asked != "" {
		if !isUnicodeType(asked) {
			return "", "", fmt.Errorf("%w: %q is not a well-formed calendar name", ErrOption, asked)
		}
		// ResolveLocale's CanonicalizeUValue, as the -u-ca- keyword is
		// canonicalized: "islamicc" is islamic-civil.
		asked = canonicalUValue(src, "ca", strings.ToLower(asked))
	}
	keyword, _ := loc.Keyword("ca")
	switch {
	case asked != "" && asked != keyword && implemented[CalendarSystem(asked)]:
		return CalendarSystem(asked), "", nil
	case implemented[CalendarSystem(keyword)]:
		return CalendarSystem(keyword), keyword, nil
	}

	region := loc.Region.String()
	if region == "" {
		if f, err := NewFallbacker(src); err == nil {
			if full, ok := f.Maximize(loc.Data()); ok {
				region = full.Region.String()
			}
		}
	}
	if b, err := src.Open(MarkerCalendarPrefs, DataLocale{}); err == nil {
		if system, ok := calendarPreferences(b).of(region); ok && implemented[system] {
			return system, "", nil
		}
	}
	return Gregory, "", nil
}

// isUnicodeType reports whether a name is a Unicode locale type, UTS #35's
// "type": segments of three to eight letters or digits, joined by hyphens.
func isUnicodeType(s string) bool {
	for _, seg := range strings.Split(s, "-") {
		if len(seg) < 3 || len(seg) > 8 {
			return false
		}
		for i := 0; i < len(seg); i++ {
			if c := seg[i] | 0x20; !(c >= 'a' && c <= 'z') && !(seg[i] >= '0' && seg[i] <= '9') {
				return false
			}
		}
	}
	return true
}

// gregorianYearLength is the proleptic Gregorian year's length, as V8 sets
// ICU's Gregorian calendar to reckon with no Julian changeover.
func gregorianYearLength(year int) int {
	if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		return 366
	}
	return 365
}

// reckon turns an instant into the fields of a date in one calendar.
func reckon(t time.Time, system CalendarSystem, rules calendarRules) dateParts {
	p := partsOf(t)
	// The Gregorian calendar's own fields, which the Buddhist one keeps:
	// its extended year is the Gregorian one, counting on through 0 for
	// 1 BC.
	p.extYear, p.dayOfYear, p.relatedYear = t.Year(), t.YearDay(), t.Year()
	p.yearLength = gregorianYearLength
	switch system {
	case Buddhist, ROC, Japanese:
		// V8 makes ICU's Gregorian calendar proleptic, but only a calendar
		// that is exactly ICU's GregorianCalendar; the Buddhist and ROC
		// calendars are subclasses, and keep its Julian dates before the
		// changeover of October 1582.
		if jd := julianDay(t); jd < gregorianCutover {
			p.extYear, p.month, p.day, p.dayOfYear = julianDate(jd)
			p.relatedYear = p.extYear
		}
		p.yearLength = changeoverYearLength
	}
	switch system {
	case Persian:
		p.year, p.month, p.day = persianDate(julianDay(t))
		p.era = 0
		p.extYear = p.year
		p.dayOfYear = persianMonthStart[p.month-1] + p.day
		p.relatedYear = p.year + 622
		p.yearLength = persianYearLength
		return p
	case Coptic, Ethiopic, EthiopicAmeteAlem:
		// CECalendar::handleComputeFields, with each calendar's eras.
		epoch, related := copticEpoch, 284
		switch system {
		case Ethiopic:
			epoch, related = ethiopicEpoch, 8
		case EthiopicAmeteAlem:
			epoch, related = ameteAlemEpoch, 8
		}
		eyear, month, day, doy := ceDate(julianDay(t), epoch)
		p.extYear, p.month, p.day, p.dayOfYear = eyear, month, day, doy
		p.relatedYear = eyear + related
		p.yearLength = ceYearLength
		switch {
		case system == EthiopicAmeteAlem:
			p.year, p.era = eyear, 0
		case system == Ethiopic && eyear <= 0:
			p.year, p.era = eyear+ameteMihretDiff, 0
		case system == Coptic && eyear <= 0:
			p.year, p.era = 1-eyear, 0
		default:
			p.year, p.era = eyear, 1
		}
		return p
	case IslamicCivil, IslamicTabular:
		epoch := islamicCivilEpoch
		if system == IslamicTabular {
			epoch = islamicAstroEpoch
		}
		p.year, p.month, p.day, p.dayOfYear = islamicTabularDate(julianDay(t), epoch)
		p.era, p.extYear = 0, p.year
		p.relatedYear = islamicRelatedYear(p.year)
		p.yearLength = islamicTabularYearLength
		return p
	case Islamic, IslamicRGSA:
		p.year, p.month, p.day, p.dayOfYear = islamicAstroDate(julianDay(t), float64(t.UnixMilli()))
		p.era, p.extYear = 0, p.year
		p.relatedYear = islamicRelatedYear(p.year)
		p.yearLength = islamicAstroYearLength
		return p
	case IslamicUmmAlQura:
		if rules.ummAlQura == nil {
			break
		}
		p.year, p.month, p.day, p.dayOfYear = rules.ummAlQura.date(julianDay(t))
		p.era, p.extYear = 0, p.year
		p.relatedYear = islamicRelatedYear(p.year)
		p.yearLength = rules.ummAlQura.yearLength
		return p
	case Chinese, Dangi:
		if rules.eastAsian != nil {
			eastAsianDate(&p, t, rules.eastAsian)
			return p
		}
		// ChineseCalendar::handleComputeFields: the era is the sixty-year
		// cycle, the year the year within it, and the related year the
		// extended one.
		c := chineseReckoner{offset: chinaOffset}
		if system == Dangi {
			c.offset = koreaOffset
		}
		d := c.date(t.Year(), t.Month(), julianDay(t)-2440588)
		p.era, p.year, p.month, p.leapMonth = d.cycle, d.yearOfCycle, d.month, d.leap
		p.day, p.dayOfYear = d.day, d.dayOfYear
		p.extYear, p.relatedYear = d.extYear, d.extYear
		p.yearLength = c.yearLength
		p.failed, p.yearStartFails = d.failed, c.yearStartFails
		return p
	case Hebrew:
		p.year, p.month, p.day, p.dayOfYear = hebrewDate(julianDay(t))
		p.era, p.extYear = 0, p.year
		p.relatedYear = p.year - 3760
		p.yearLength = hebrewYearLength
		return p
	case Indian:
		p.year, p.month, p.day, p.dayOfYear = indianDate(t)
		p.era, p.extYear = 0, p.year
		p.relatedYear = p.year + 79
		p.yearLength = indianYearLength
		return p
	case ROC:
		// TaiwanCalendar::handleComputeFields: the extended year stays, and
		// the displayed year counts from 1912 or back from it.
		if y := p.extYear - 1911; y > 0 {
			p.year, p.era = y, 1
		} else {
			p.year, p.era = 1-y, 0
		}
		return p
	}
	if system == Japanese {
		// JapaneseCalendar::handleComputeFields: the era is the last to
		// start on or before the date, the first era counting back before
		// its own start, and the year counts from the era's first.
		p.era = 0
		eras := rules.eras
		for i := len(eras) - 1; i >= 0; i-- {
			e := eras[i]
			if e.year < p.extYear || e.year == p.extYear &&
				(e.month < p.month || e.month == p.month && e.day <= p.day) {
				p.era = e.era
				p.year = p.extYear - e.year + 1
				return p
			}
		}
		if len(eras) > 0 {
			p.era, p.year = eras[0].era, p.extYear-eras[0].year+1
		}
		return p
	}
	if system == Buddhist {
		// BuddhistCalendar::handleComputeFields: the extended year stays.
		p.year = p.extYear + 543
		p.era = 0
	}
	return p
}

// calendarRules are the tables a calendar reckons with beyond its arithmetic,
// read when a formatter is built for it.
type calendarRules struct {
	// eras are the Japanese calendar's.
	eras []eraStart
	// ummAlQura is the Umm al-Qura calendar's month lengths.
	ummAlQura *ummAlQura
	// eastAsian are ICU4X's rules for the Chinese calendar or Dangi, which
	// Temporal reckons with; nil where ICU4C's astronomy reckons them
	// (ChineseAstronomy).
	eastAsian *eastasian.Rules
}

// loadEastAsian reads ICU4X's rules for the Chinese calendar or Dangi.
func loadEastAsian(src Source, system CalendarSystem) (*eastasian.Rules, error) {
	b, err := src.Open(MarkerTemporalCalendars, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the calendar tables: %w", err)
	}
	country := "china"
	if system == Dangi {
		country = "korea"
	}
	tables, err := eastasian.ParseTables(b, country, "qing")
	if err != nil {
		return nil, fmt.Errorf("intl: %w", err)
	}
	rules := eastasian.China(tables)
	if system == Dangi {
		rules = eastasian.Korea(tables)
	}
	return &rules, nil
}

// eastAsianDate reckons a local date as ICU4X does, in the fields ICU4C's
// ChineseCalendar gives it: the sixty-year cycle as the era, the year of
// it as the year, and the Gregorian year the year starts in as the related
// and extended year.
func eastAsianDate(p *dateParts, t time.Time, rules *eastasian.Rules) {
	rd := eastasian.GregorianFixed(t.Year(), int(t.Month()), t.Day())
	y := rules.YearOfRD(rd)
	p.dayOfYear = int(rd-y.Start) + 1
	ordinal, day := 1, p.dayOfYear
	for ordinal < y.Count && day > int(y.Lengths[ordinal-1]) {
		day -= int(y.Lengths[ordinal-1])
		ordinal++
	}
	// A leap month has the number of the month before it.
	p.month, p.leapMonth, p.day = ordinal, false, day
	if y.Leap != 0 && ordinal >= y.Leap {
		p.month, p.leapMonth = ordinal-1, ordinal == y.Leap
	}
	cycleYear := y.Related + 2637
	cycle := floorDiv(cycleYear-1, 60)
	p.era, p.year = cycle+1, cycleYear-cycle*60
	p.extYear, p.relatedYear = y.Related, y.Related
	p.yearLength = func(extended int) int {
		return int(rules.Year(extended+1).Start - rules.Year(extended).Start)
	}
}

// eraStart is the first day of one of the Japanese calendar's eras.
type eraStart struct {
	era, year, month, day int
}

// loadJapaneseEras reads the Japanese calendar's eras, in order.
func loadJapaneseEras(src Source) ([]eraStart, error) {
	b, err := src.Open(MarkerJapaneseEras, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the Japanese eras: %w", err)
	}
	var out []eraStart
	for _, line := range strings.Split(string(b), "\n") {
		var e eraStart
		if n, _ := fmt.Sscan(line, &e.era, &e.year, &e.month, &e.day); n == 4 {
			out = append(out, e)
		}
	}
	return out, nil
}

// gregorianCutover is the Julian day ICU's Gregorian calendar changes over
// from the Julian one on, 15 October 1582.
const gregorianCutover = 2299161

// julianDate is the Julian-calendar half of GregorianCalendar's
// handleComputeFields: the extended year, month from one, day and day of the
// year.
func julianDate(jd int) (year, month, day, dayOfYear int) {
	epochDay := jd - (1721426 - 2)
	year = floorDiv(4*epochDay+1464, 1461)
	jan1 := 365*(year-1) + floorDiv(year-1, 4)
	doy := epochDay - jan1
	leap := mod(year, 4) == 0
	correction, march1 := 0, 59
	if leap {
		march1 = 60
	}
	if doy >= march1 {
		correction = 2
		if leap {
			correction = 1
		}
	}
	m := (12*(doy+correction) + 6) / 367
	before := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	day = doy - before[m] + 1
	if leap && m > 1 {
		day--
	}
	return year, m + 1, day, doy + 1
}

// changeoverYearLength is GregorianCalendar's year length with the
// changeover: Julian leap years before 1582.
func changeoverYearLength(year int) int {
	if year < 1582 {
		if mod(year, 4) == 0 {
			return 366
		}
		return 365
	}
	return gregorianYearLength(year)
}
