// Package date is JavaScript's Date apart from any engine, as Node's V8
// has it: time values and ECMA-262's arithmetic on them, local time in a
// time zone as ICU reckons it, the strings Date writes, and Date.parse.
//
// Time values are milliseconds from 1970 as float64s, NaN being an invalid
// date, as JavaScript holds them.
package date

import (
	"math"
	"sync"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Options say where an Environment reckons: the data, the locale the zone
// names are written in, the zone, and the clock. Each left out is the
// host's, as Node's are.
type Options struct {
	// Source is the data; nil is the intl package's embedded data.
	Source intl.Source
	// Locale is the locale Date writes zone names in; nil is the host's,
	// ICU's default locale.
	Locale *intl.Locale
	// TimeZone is the zone local time is in; nil is the host's.
	TimeZone *intl.TimeZone
	// Now is the clock; nil is the system's.
	Now func() time.Time
	// Compat chooses Node's side of intl.HostAbbreviations, for the host's
	// zone where TimeZone is nil.
	Compat intl.Compat
}

// An Environment is local time as a JavaScript realm sees it, V8's
// DateCache: a zone, the names of its standard and daylight time, a clock,
// and the cache of offsets V8 reads local time through. It is safe for any
// number of goroutines to share, but, as V8's, what it says a time's offset
// is can depend on the times it was asked about before (see cache.go).
type Environment struct {
	tz  *intl.TimeZone
	now func() time.Time
	// The names V8 writes in brackets. V8 asks ICU for each once, at the
	// current time, and keeps it; this asks for both when it is built.
	standardName, daylightName string

	mu    sync.Mutex
	cache dateCache
}

// New builds an Environment.
func New(opts Options) (*Environment, error) {
	src := opts.Source
	if src == nil {
		src = intl.Embedded
	}
	loc := intl.HostLocale()
	if opts.Locale != nil {
		loc = *opts.Locale
	}
	tz := opts.TimeZone
	if tz == nil {
		tz = intl.HostTimeZone(src, opts.Compat)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	e := &Environment{tz: tz, now: now}
	e.cache.reset()
	at := now()
	var err error
	if e.standardName, err = intl.TimeZoneDisplayName(src, loc, tz, false, at); err != nil {
		return nil, err
	}
	if e.daylightName, err = intl.TimeZoneDisplayName(src, loc, tz, true, at); err != nil {
		return nil, err
	}
	return e, nil
}

// TimeZone is the zone local time is in.
func (e *Environment) TimeZone() *intl.TimeZone { return e.tz }

// Now is Date.now(): the current time value.
func (e *Environment) Now() float64 { return float64(e.now().UnixMilli()) }

const (
	msPerSecond = 1000
	msPerMinute = 60000
	msPerHour   = 3600000
	msPerDay    = 86400000

	// maxTime is the largest time value, 8.64e15.
	maxTime = 864000000 * 10000000
	// maxTimeBeforeUTC is the largest local time V8 converts to UTC: a
	// month past the largest time value.
	maxTimeBeforeUTC = maxTime + 30*msPerDay
	// maxEpochTime is the last time value V8 reads the zone's daylight
	// saving at as it is: 2038-01-19, the end of 32-bit seconds.
	maxEpochTime = math.MaxInt32 * 1000
)

// TimeClip is ECMA-262's TimeClip: NaN beyond ±8.64e15, else the value
// truncated toward zero, negative zero made zero.
func TimeClip(t float64) float64 {
	if math.IsNaN(t) || t < -maxTime || t > maxTime {
		return math.NaN()
	}
	return math.Trunc(t) + 0
}

// zoneOffset is the offset in force at an instant, in milliseconds, as ICU
// gives it.
func (e *Environment) zoneOffset(t int64) int64 {
	return int64(e.tz.Offset(t).Total()) * msPerSecond
}

// toLocal is DateCache::ToLocal: an instant as local time, through the
// cache.
func (e *Environment) toLocal(t int64) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return t + e.cache.localOffset(t, e.zoneOffset)
}

// timezoneOffset is DateCache::TimezoneOffset: UTC less local time, in
// minutes, truncated.
func (e *Environment) timezoneOffset(t int64) int64 {
	return (t - e.toLocal(t)) / msPerMinute
}

// LocalTime is ECMA-262's LocalTime, V8's DateCache::ToLocal: the time
// value read as local time. NaN stays NaN.
func (e *Environment) LocalTime(t float64) float64 {
	if math.IsNaN(t) {
		return t
	}
	return float64(e.toLocal(int64(t)))
}

// UTC is ECMA-262's UTC, as V8 sets a date from local fields
// (SetLocalDateValue): a local time value made a time value, the local time
// a transition skips or repeats read with the offset before it, and the
// result clipped. It is NaN for a local time more than a month beyond the
// range of time values.
func (e *Environment) UTC(local float64) float64 {
	if math.IsNaN(local) || local < -maxTimeBeforeUTC || local > maxTimeBeforeUTC {
		return math.NaN()
	}
	ms := int64(local)
	offset := int64(e.tz.OffsetFromLocal(ms, intl.Former, intl.Former).Total()) * msPerSecond
	return TimeClip(float64(ms - offset))
}

// TimezoneOffset is Date.prototype.getTimezoneOffset: the minutes UTC is
// ahead of local time, truncated. NaN for NaN.
func (e *Environment) TimezoneOffset(t float64) float64 {
	if math.IsNaN(t) {
		return t
	}
	return float64(e.timezoneOffset(int64(t)))
}

// zoneName is V8's DateCache::LocalTimezone: the name of standard or
// daylight time, by the daylight saving at the instant, read in a year of
// the same shape where the instant is outside 1970 to 2038.
func (e *Environment) zoneName(t int64) string {
	if t < 0 || t > maxEpochTime {
		t = equivalentTime(t)
	}
	// V8 asks at the whole second.
	if e.tz.Offset(t/msPerSecond*msPerSecond).DST != 0 {
		return e.daylightName
	}
	return e.standardName
}

// daysFromTime is DateCache::DaysFromTime: days from 1970, floored.
func daysFromTime(t int64) int64 {
	if t < 0 {
		t -= msPerDay - 1
	}
	return t / msPerDay
}

func weekday(days int64) int {
	r := int((days + 4) % 7)
	if r < 0 {
		r += 7
	}
	return r
}

func isLeap(year int64) bool { return year%4 == 0 && (year%100 != 0 || year%400 == 0) }

var (
	daysBeforeMonth     = [12]int64{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	daysBeforeMonthLeap = [12]int64{0, 31, 60, 91, 121, 152, 182, 213, 244, 274, 305, 335}
	daysInMonth         = [12]int64{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
)

// daysFromYearMonth is DateCache::DaysFromYearMonth: the days from 1970 to
// the first of a month from zero, which may be out of 0 to 11.
func daysFromYearMonth(year, month int64) int64 {
	year += month / 12
	month %= 12
	if month < 0 {
		year--
		month += 12
	}
	const yearDelta = 399999
	const baseDay = 365*(1970+yearDelta) + (1970+yearDelta)/4 - (1970+yearDelta)/100 + (1970+yearDelta)/400
	y1 := year + yearDelta
	days := 365*y1 + y1/4 - y1/100 + y1/400 - baseDay
	if isLeap(year) {
		return days + daysBeforeMonthLeap[month]
	}
	return days + daysBeforeMonth[month]
}

// yearMonthDay is DateCache::YearMonthDayFromDays: the year, the month
// from zero and the day of a day from 1970.
func yearMonthDay(days int64) (year, month, day int64) {
	const (
		in4Years   = 4*365 + 1
		in100Years = 25*in4Years - 1
		in400Years = 4*in100Years + 1
		from1970   = 365*30 + 7 // to 2000
		offset     = 1000*in400Years + 5*in400Years - from1970
		yearOffset = 400000
	)
	days += offset
	year = 400*(days/in400Years) - yearOffset
	days %= in400Years
	days--
	yd1 := days / in100Years
	days %= in100Years
	year += 100 * yd1
	days++
	yd2 := days / in4Years
	days %= in4Years
	year += 4 * yd2
	days--
	yd3 := days / 365
	days %= 365
	year += yd3
	leap := (yd1 == 0 || yd2 != 0) && yd3 == 0
	if leap {
		days++
	}
	feb := int64(28)
	if leap {
		feb = 29
	}
	if days >= 31+feb {
		days -= 31 + feb
		for m := int64(2); m < 12; m++ {
			if days < daysInMonth[m] {
				return year, m, days + 1
			}
			days -= daysInMonth[m]
		}
	}
	if days < 31 {
		return year, 0, days + 1
	}
	return year, 1, days - 31 + 1
}

// equivalentTime is DateCache::EquivalentTime: the time in the year of
// 2008 to 2035 that is leap as the year is and starts on its weekday.
func equivalentTime(t int64) int64 {
	days := daysFromTime(t)
	within := t - days*msPerDay
	year, month, day := yearMonthDay(days)
	return (daysFromYearMonth(equivalentYear(year), month)+day-1)*msPerDay + within
}

func equivalentYear(year int64) int64 {
	wd := int64(weekday(daysFromYearMonth(year, 0)))
	recent := int64(1967)
	if isLeap(year) {
		recent = 1956
	}
	recent += (wd * 12) % 28
	return 2008 + (recent+3*28-2008)%28
}

// Fields are a time value taken apart, DateCache::BreakDownTime.
type Fields struct {
	Year                              int64
	Month                             int // from zero
	Day, Weekday                      int // Weekday from Sunday, zero
	Hour, Minute, Second, Millisecond int
}

// BreakDown takes a time value apart, in UTC. For local fields, take apart
// LocalTime(t).
func BreakDown(t float64) Fields {
	ms := int64(t)
	days := daysFromTime(ms)
	within := ms - days*msPerDay
	y, m, d := yearMonthDay(days)
	return Fields{Year: y, Month: int(m), Day: int(d), Weekday: weekday(days),
		Hour: int(within / msPerHour), Minute: int(within / msPerMinute % 60),
		Second: int(within / msPerSecond % 60), Millisecond: int(within % 1000)}
}

// MakeTime is ECMA-262's MakeTime, NaN unless all are finite.
//
// Each product is rounded before it is added, as the specification's
// arithmetic and V8's are: the conversions keep an arm64 build from fusing a
// multiply and an add into one rounding, which at these magnitudes gives
// another answer (Go's spec, "Arithmetic operators").
func MakeTime(hour, min, sec, ms float64) float64 {
	if !finite(hour) || !finite(min) || !finite(sec) || !finite(ms) {
		return math.NaN()
	}
	return float64(float64(float64(math.Trunc(hour)*msPerHour)+float64(math.Trunc(min)*msPerMinute))+
		float64(math.Trunc(sec)*msPerSecond)) + math.Trunc(ms)
}

// MakeDay is ECMA-262's MakeDay as V8 bounds it: NaN for a year beyond a
// million either way, a month beyond ten million, or a day not finite.
func MakeDay(year, month, date float64) float64 {
	if !(year >= -1000000 && year <= 1000000) || !(month >= -10000000 && month <= 10000000) || !finite(date) {
		return math.NaN()
	}
	y, m := int64(year), int64(month)
	return float64(daysFromYearMonth(y, m)) + math.Trunc(date) - 1
}

// MakeDate is ECMA-262's MakeDate, its product rounded before the sum as
// MakeTime's are.
func MakeDate(day, t float64) float64 {
	if !finite(day) || !finite(t) {
		return math.NaN()
	}
	return t + float64(day*msPerDay)
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
