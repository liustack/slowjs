package intl

import (
	"errors"
	"fmt"
	"strings"

	"github.com/liustack/quickjs/internal/gointl/internal/tzdata"
)

// Time zones as ICU reckons them, which is what Node formats with: the
// offsets of ICU's zoneinfo64 (data/tz; see internal/tzgen), the tz database
// as ICU compiles it, rather than the host's or Go's copy of it.
//
// A zone is OlsonTimeZone: offsets in force between transitions, each a raw
// offset and a daylight saving, and, from the year it governs, a final rule
// that is SimpleTimeZone's. The daylight saving is kept apart from the raw
// offset, as ICU keeps it, because a zone's names depend on it: Ireland's
// summer is its daylight time in ICU's data, where Go's has its winter
// negative.

// zoneOffset is the raw offset and the daylight saving in force, in
// seconds.
type zoneOffset struct {
	raw, dst int
}

func (o zoneOffset) total() int { return o.raw + o.dst }

// A timeZone is a zone ICU knows by name, or an offset from UTC.
type timeZone struct {
	name       string // as ICU spells it, empty for an offset
	id         string // the canonical name, empty for an offset
	types      []zoneOffset
	trans      []int64 // seconds
	transTypes []uint8
	final      *finalZone
	finalStart int64 // milliseconds: 1 January of the final rule's first year
	finalYear  int
	// finalRaw and finalRule are the final rule as zoneinfo64 has it: the
	// raw offset and ICU's eleven numbers.
	finalRaw  int
	finalRule []int

	// firstTrans is OlsonTimeZone's firstTZTransitionIdx: the first
	// transition to a type other than the initial one.
	firstTrans int
}

// fixedZone is a zone of one offset.
func fixedZone(seconds int) *timeZone {
	return &timeZone{types: []zoneOffset{{raw: seconds}}}
}

// errNoZone is a name that is not a zone.
var errNoZone = errors.New("not a time zone")

// loadTimeZone reads a zone by a name as ECMA-402 gives it, in any case.
func loadTimeZone(src Source, name string) (*timeZone, error) {
	if !zoneNameShaped(name) {
		return nil, fmt.Errorf("intl: %q is %w", name, errNoZone)
	}
	b, err := src.Open(MarkerTimeZones+Marker("/"+strings.ToLower(name)), DataLocale{})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("intl: %q is %w", name, errNoZone)
		}
		return nil, fmt.Errorf("intl: the time zone %q: %w", name, err)
	}
	d, err := tzdata.Decode(b)
	if err != nil {
		return nil, fmt.Errorf("intl: the time zone %q: %w", name, err)
	}
	z := &timeZone{name: d.Name, id: d.Canonical}
	if d.Link != "" {
		if b, err = src.Open(MarkerTimeZones+Marker("/"+d.Link), DataLocale{}); err != nil {
			return nil, fmt.Errorf("intl: the time zone %s: %w", d.Link, err)
		}
		if d, err = tzdata.Decode(b); err != nil {
			return nil, fmt.Errorf("intl: the time zone %s: %w", z.name, err)
		}
	}
	if err := z.load(d); err != nil {
		return nil, fmt.Errorf("intl: the time zone %s: %w", z.name, err)
	}
	return z, nil
}

// zoneNameShaped reports whether a name is made as the tz database's are,
// of letters, digits, '_', '-' and '+' in parts separated by '/', and so
// may be looked for among the zone files.
func zoneNameShaped(name string) bool {
	part := 0
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '/':
			if part == 0 {
				return false
			}
			part = 0
			continue
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '_' || c == '-' || c == '+':
		default:
			return false
		}
		part++
	}
	return part > 0
}

// resolvedID is the name resolvedOptions reports: the canonical name, as
// V8's Intl::TimeZoneIdToString has it, Etc/UTC and Etc/GMT being "UTC".
func (z *timeZone) resolvedID() string {
	if z.id == "Etc/UTC" || z.id == "Etc/GMT" {
		return "UTC"
	}
	return z.id
}

// load takes a zone's offsets from its file, and checks them.
func (z *timeZone) load(d *tzdata.Zone) error {
	z.types = make([]zoneOffset, len(d.Types))
	for i, t := range d.Types {
		z.types[i] = zoneOffset{int(t.Raw), int(t.DST)}
	}
	z.trans, z.transTypes = d.Trans, d.TransTypes
	if len(d.Final) > 0 {
		n := make([]int, len(d.Final))
		for i, v := range d.Final {
			n[i] = int(v)
		}
		f, err := newFinalZone(n[0], n[2:])
		if err != nil {
			return fmt.Errorf("final %v: %w", n, err)
		}
		z.final = f
		z.finalYear = n[1]
		z.finalRaw, z.finalRule = n[0], n[2:]
		z.finalStart = gregoDay(n[1], 0, 1) * msPerDay
	}
	if len(z.types) == 0 {
		return fmt.Errorf("no offsets")
	}
	for _, t := range z.transTypes {
		if int(t) >= len(z.types) {
			return fmt.Errorf("a transition to type %d of %d", t, len(z.types))
		}
	}
	for z.firstTrans < len(z.trans) && z.transTypes[z.firstTrans] == 0 {
		z.firstTrans++
	}
	return nil
}

const msPerDay = 86400000

// The options getOffsetFromLocal takes for a local time a transition skips
// or repeats: to read it as standard or as daylight time, where the
// transition is between the two, else with the offset before the
// transition or the one after it.
const (
	tzLocalStandard = 0x01
	tzLocalDaylight = 0x03
	tzLocalFormer   = 0x04
	tzLocalLatter   = 0x0C

	tzStdDstMask       = 0x03
	tzFormerLatterMask = 0x0C
)

// offsetAt is OlsonTimeZone::getOffset for a UTC instant in milliseconds.
func (z *timeZone) offsetAt(ms int64) zoneOffset {
	if z.final != nil && ms >= z.finalStart {
		return z.final.offset(ms, false)
	}
	return z.historicOffset(ms, false, tzLocalFormer, tzLocalLatter)
}

// localOffset is OlsonTimeZone::getOffset for a local time, in
// milliseconds as if UTC.
func (z *timeZone) localOffset(ms int64) zoneOffset {
	if z.final != nil && ms >= z.finalStart {
		return z.final.offset(ms, true)
	}
	return z.historicOffset(ms, true, tzLocalFormer, tzLocalLatter)
}

// offsetFromLocal is OlsonTimeZone::getOffsetFromLocal: the offset of a
// local time, read by the options where a transition skips or repeats it.
func (z *timeZone) offsetFromLocal(ms int64, nonExisting, duplicated int) zoneOffset {
	if z.final != nil && ms >= z.finalStart {
		return z.final.offsetFromLocal(ms, nonExisting, duplicated)
	}
	return z.historicOffset(ms, true, nonExisting, duplicated)
}

// historicOffset is OlsonTimeZone::getHistoricalOffset.
func (z *timeZone) historicOffset(ms int64, local bool, nonExisting, duplicated int) zoneOffset {
	if len(z.trans) == 0 {
		return z.types[0]
	}
	sec := floorDiv64(ms, 1000)
	if !local && sec < z.trans[0] {
		return z.types[0]
	}
	i := len(z.trans) - 1
	for ; i >= 0; i-- {
		t := z.trans[i]
		if local && sec >= t-86400 {
			t += int64(localShift(z.typeAt(i-1), z.typeAt(i), nonExisting, duplicated))
		}
		if sec >= t {
			break
		}
	}
	return z.typeAt(i)
}

// localShift is the offset getHistoricalOffset adds to a transition to
// compare local times with it: the offset before it or the one after it,
// by the options.
func localShift(before, after zoneOffset, nonExisting, duplicated int) int {
	dstToStd := before.dst != 0 && after.dst == 0
	stdToDst := before.dst == 0 && after.dst != 0
	if after.total() >= before.total() {
		// A gap: by default, the offset before it.
		switch {
		case nonExisting&tzStdDstMask == tzLocalStandard && dstToStd,
			nonExisting&tzStdDstMask == tzLocalDaylight && stdToDst:
			return before.total()
		case nonExisting&tzStdDstMask == tzLocalStandard && stdToDst,
			nonExisting&tzStdDstMask == tzLocalDaylight && dstToStd:
			return after.total()
		case nonExisting&tzFormerLatterMask == tzLocalLatter:
			return before.total()
		}
		return after.total()
	}
	// An overlap: by default, the offset after it.
	switch {
	case duplicated&tzStdDstMask == tzLocalStandard && dstToStd,
		duplicated&tzStdDstMask == tzLocalDaylight && stdToDst:
		return after.total()
	case duplicated&tzStdDstMask == tzLocalStandard && stdToDst,
		duplicated&tzStdDstMask == tzLocalDaylight && dstToStd:
		return before.total()
	case duplicated&tzFormerLatterMask == tzLocalFormer:
		return before.total()
	}
	return after.total()
}

// typeAt is the offset in force from transition i, the initial one before
// the first.
func (z *timeZone) typeAt(i int) zoneOffset {
	if i < 0 {
		return z.types[0]
	}
	return z.types[z.transTypes[i]]
}

// A zoneTransition is a change of offset: when, in milliseconds, and the
// offsets either side.
type zoneTransition struct {
	at       int64
	from, to zoneOffset
}

// finalTransition is OlsonTimeZone's firstFinalTZTransition: from the last
// historic offset to the final rule's first, which ICU reports whether or
// not the offset changes.
func (z *timeZone) finalTransition() zoneTransition {
	from := z.types[0]
	if len(z.trans) > 0 {
		from = z.typeAt(len(z.trans) - 1)
	}
	if !z.final.daylight {
		return zoneTransition{at: z.finalStart, from: from, to: zoneOffset{raw: z.final.raw}}
	}
	t, _ := z.final.next(z.finalStart, false, z.finalYear)
	t.from = from
	return t
}

// nextTransition is OlsonTimeZone::getNextTransition: the first
// transition after an instant, or at it when inclusive.
func (z *timeZone) nextTransition(base int64, inclusive bool) (zoneTransition, bool) {
	if z.final != nil {
		ft := z.finalTransition()
		if inclusive && base == ft.at {
			return ft, true
		} else if base >= ft.at {
			if z.final.daylight {
				return z.final.next(base, inclusive, z.finalYear)
			}
			return zoneTransition{}, false
		}
	}
	if len(z.trans) == 0 {
		if z.final != nil {
			return z.finalTransition(), true
		}
		return zoneTransition{}, false
	}
	for {
		i := len(z.trans) - 1
		for ; i >= z.firstTrans; i-- {
			t := z.trans[i] * 1000
			if base > t || !inclusive && base == t {
				break
			}
		}
		switch {
		case i == len(z.trans)-1:
			if z.final != nil {
				return z.finalTransition(), true
			}
			return zoneTransition{}, false
		case i < z.firstTrans:
			return zoneTransition{at: z.trans[z.firstTrans] * 1000, from: z.types[0], to: z.typeAt(z.firstTrans)}, true
		}
		from, to := z.typeAt(i), z.typeAt(i+1)
		at := z.trans[i+1] * 1000
		if from == to {
			// Not a transition: the next one after it.
			base, inclusive = at, false
			continue
		}
		return zoneTransition{at: at, from: from, to: to}, true
	}
}

// previousTransition is OlsonTimeZone::getPreviousTransition: the last
// transition before an instant, or at it when inclusive.
func (z *timeZone) previousTransition(base int64, inclusive bool) (zoneTransition, bool) {
	if z.final != nil {
		ft := z.finalTransition()
		if inclusive && base == ft.at {
			return ft, true
		} else if base > ft.at {
			if z.final.daylight {
				return z.final.previous(base, inclusive, z.finalYear)
			}
			return ft, true
		}
	}
	for len(z.trans) > 0 {
		i := len(z.trans) - 1
		for ; i >= z.firstTrans; i-- {
			t := z.trans[i] * 1000
			if base > t || inclusive && base == t {
				break
			}
		}
		switch {
		case i < z.firstTrans:
			return zoneTransition{}, false
		case i == z.firstTrans:
			return zoneTransition{at: z.trans[i] * 1000, from: z.types[0], to: z.typeAt(i)}, true
		}
		from, to := z.typeAt(i-1), z.typeAt(i)
		at := z.trans[i] * 1000
		if from == to {
			// Not a transition: the one before it.
			base, inclusive = at, false
			continue
		}
		return zoneTransition{at: at, from: from, to: to}, true
	}
	return zoneTransition{}, false
}

// finalZone is the SimpleTimeZone a zone ends in, its offsets in seconds.
type finalZone struct {
	raw, savings int
	start, end   zoneRule
	daylight     bool
}

// A zoneRule is one of SimpleTimeZone's rules, decoded as decodeStartRule
// decodes it: a month from zero, a day and a weekday by mode, and a time of
// day in milliseconds, of the kind timeMode says.
type zoneRule struct {
	mode, month, day, dayOfWeek int
	millis                      int64
	timeMode                    int
}

// SimpleTimeZone's modes and time modes.
const (
	domMode = iota + 1
	dowInMonthMode
	dowGEDomMode
	dowLEDomMode

	wallTime     = 0
	standardTime = 1
	utcTime      = 2
)

// newFinalZone is the SimpleTimeZone OlsonTimeZone makes of a final rule:
// the raw offset and the rule's eleven numbers, in seconds.
func newFinalZone(raw int, rule []int) (*finalZone, error) {
	f := &finalZone{raw: raw, savings: rule[10]}
	f.start = decodeZoneRule(rule[0], rule[1], rule[2], rule[3], rule[4])
	f.end = decodeZoneRule(rule[5], rule[6], rule[7], rule[8], rule[9])
	f.daylight = rule[1] != 0 && rule[6] != 0
	if f.daylight && f.savings == 0 {
		f.savings = 3600
	}
	for _, r := range []zoneRule{f.start, f.end} {
		if f.daylight && (r.month < 0 || r.month > 11 || r.dayOfWeek > 7 ||
			r.millis < 0 || r.millis > msPerDay || r.timeMode < wallTime || r.timeMode > utcTime) {
			return nil, fmt.Errorf("a rule out of range")
		}
	}
	return f, nil
}

// decodeZoneRule is decodeStartRule and decodeEndRule: the mode is in the
// signs of the day and the weekday.
func decodeZoneRule(month, day, dayOfWeek, seconds, timeMode int) zoneRule {
	r := zoneRule{month: month, day: day, dayOfWeek: dayOfWeek, millis: int64(seconds) * 1000, timeMode: timeMode}
	switch {
	case day == 0:
		// No daylight saving.
	case dayOfWeek == 0:
		r.mode = domMode
	case dayOfWeek > 0:
		r.mode = dowInMonthMode
	default:
		r.dayOfWeek = -dayOfWeek
		if day > 0 {
			r.mode = dowGEDomMode
		} else {
			r.day = -day
			r.mode = dowLEDomMode
		}
	}
	return r
}

// offset is TimeZone::getOffset, which SimpleTimeZone inherits: the date
// in local standard time, compared with the rules. A local time is read
// twice where the first reading is daylight time, so that a time the rules
// skip is daylight time and one they repeat standard time.
func (f *finalZone) offset(ms int64, local bool) zoneOffset {
	date := ms
	if !local {
		date += int64(f.raw) * 1000
	}
	for pass := 0; ; pass++ {
		dst := f.dstAt(date)
		if pass != 0 || !local || dst == 0 {
			return zoneOffset{raw: f.raw, dst: dst}
		}
		date -= int64(dst) * 1000
	}
}

// offsetFromLocal is SimpleTimeZone::getOffsetFromLocal.
func (f *finalZone) offsetFromLocal(date int64, nonExisting, duplicated int) zoneOffset {
	dst := f.dstAt(date)
	recalc := false
	if dst > 0 {
		if nonExisting&tzStdDstMask == tzLocalStandard ||
			nonExisting&tzStdDstMask != tzLocalDaylight && nonExisting&tzFormerLatterMask != tzLocalLatter {
			date -= int64(f.savings) * 1000
			recalc = true
		}
	} else if duplicated&tzStdDstMask == tzLocalDaylight ||
		duplicated&tzStdDstMask != tzLocalStandard && duplicated&tzFormerLatterMask == tzLocalFormer {
		date -= int64(f.savings) * 1000
		recalc = true
	}
	if recalc {
		dst = f.dstAt(date)
	}
	return zoneOffset{raw: f.raw, dst: dst}
}

// dstAt is SimpleTimeZone's getOffset by fields less the raw offset, in
// seconds, for a local standard time in milliseconds.
func (f *finalZone) dstAt(date int64) int {
	if !f.daylight {
		return 0
	}
	day := floorDiv64(date, msPerDay)
	millis := date - day*msPerDay
	year, month, dom, dow := gregoFields(day)
	monthLen := gregoMonthLength(year, month)
	prevLen := 31
	if month > 0 {
		prevLen = gregoMonthLength(year, month-1)
	}
	raw := int64(f.raw) * 1000

	southern := f.start.month > f.end.month
	startDelta := int64(0)
	if f.start.timeMode == utcTime {
		startDelta = -raw
	}
	startCompare := compareToRule(month, monthLen, prevLen, dom, dow, millis, startDelta, f.start)
	endCompare := 0
	if southern != (startCompare >= 0) {
		endDelta := int64(0)
		switch f.end.timeMode {
		case wallTime:
			endDelta = int64(f.savings) * 1000
		case utcTime:
			endDelta = -raw
		}
		endCompare = compareToRule(month, monthLen, prevLen, dom, dow, millis, endDelta, f.end)
	}
	if !southern && startCompare >= 0 && endCompare < 0 || southern && (startCompare >= 0 || endCompare < 0) {
		return f.savings
	}
	return 0
}

// compareToRule is SimpleTimeZone::compareToRule: whether a date and time
// come before (-1), at (0) or after (1) a rule's in the same year.
func compareToRule(month, monthLen, prevMonthLen, dayOfMonth, dayOfWeek int, millis, millisDelta int64, r zoneRule) int {
	millis += millisDelta
	for millis >= msPerDay {
		millis -= msPerDay
		dayOfMonth++
		dayOfWeek = 1 + dayOfWeek%7
		if dayOfMonth > monthLen {
			dayOfMonth = 1
			month++
		}
	}
	for millis < 0 {
		millis += msPerDay
		dayOfMonth--
		dayOfWeek = 1 + (dayOfWeek+5)%7
		if dayOfMonth < 1 {
			dayOfMonth = prevMonthLen
			month--
		}
	}
	if month < r.month {
		return -1
	}
	if month > r.month {
		return 1
	}
	ruleDay := r.day
	if ruleDay > monthLen {
		ruleDay = monthLen
	}
	ruleDayOfMonth := 0
	switch r.mode {
	case domMode:
		ruleDayOfMonth = ruleDay
	case dowInMonthMode:
		if ruleDay > 0 {
			ruleDayOfMonth = 1 + (ruleDay-1)*7 + (7+r.dayOfWeek-(dayOfWeek-dayOfMonth+1))%7
		} else {
			ruleDayOfMonth = monthLen + (ruleDay+1)*7 - (7+(dayOfWeek+monthLen-dayOfMonth)-r.dayOfWeek)%7
		}
	case dowGEDomMode:
		ruleDayOfMonth = ruleDay + (49+r.dayOfWeek-ruleDay-dayOfWeek+dayOfMonth)%7
	case dowLEDomMode:
		ruleDayOfMonth = ruleDay - (49-r.dayOfWeek+ruleDay+dayOfWeek-dayOfMonth)%7
	}
	switch {
	case dayOfMonth < ruleDayOfMonth:
		return -1
	case dayOfMonth > ruleDayOfMonth:
		return 1
	case millis < r.millis:
		return -1
	case millis > r.millis:
		return 1
	}
	return 0
}

// startInYear is AnnualTimeZoneRule::getStartInYear over the DateTimeRule
// SimpleTimeZone::initTransitionRules makes of a rule: the instant, in
// milliseconds, the rule takes effect in a year, given the offsets before
// it.
func (r zoneRule) startInYear(year int, prev zoneOffset) int64 {
	var ruleDay int64
	if r.mode == domMode {
		ruleDay = gregoDay(year, r.month, r.day)
	} else {
		after := true
		switch r.mode {
		case dowInMonthMode:
			if r.day > 0 {
				ruleDay = gregoDay(year, r.month, 1) + int64(7*(r.day-1))
			} else {
				after = false
				ruleDay = gregoDay(year, r.month, gregoMonthLength(year, r.month)) + int64(7*(r.day+1))
			}
		default:
			dom := r.day
			if r.mode == dowLEDomMode {
				after = false
				if r.month == 1 && dom == 29 && !gregoLeap(year) {
					dom--
				}
			}
			ruleDay = gregoDay(year, r.month, dom)
		}
		delta := r.dayOfWeek - gregoDayOfWeek(ruleDay)
		if after && delta < 0 {
			delta += 7
		} else if !after && delta > 0 {
			delta -= 7
		}
		ruleDay += int64(delta)
	}
	at := ruleDay*msPerDay + r.millis
	if r.timeMode != utcTime {
		at -= int64(prev.raw) * 1000
	}
	if r.timeMode == wallTime {
		at -= int64(prev.dst) * 1000
	}
	return at
}

// nextStart is AnnualTimeZoneRule::getNextStart for a rule in force from
// a year.
func (r zoneRule) nextStart(base int64, prev zoneOffset, inclusive bool, startYear int) int64 {
	year, _, _, _ := gregoFields(floorDiv64(base, msPerDay))
	if year < startYear {
		return r.startInYear(startYear, prev)
	}
	t := r.startInYear(year, prev)
	if t < base || !inclusive && t == base {
		return r.startInYear(year+1, prev)
	}
	return t
}

// previousStart is AnnualTimeZoneRule::getPreviousStart.
func (r zoneRule) previousStart(base int64, prev zoneOffset, inclusive bool, startYear int) (int64, bool) {
	year, _, _, _ := gregoFields(floorDiv64(base, msPerDay))
	if year < startYear {
		return 0, false
	}
	t := r.startInYear(year, prev)
	if t > base || !inclusive && t == base {
		if year-1 < startYear {
			return 0, false
		}
		return r.startInYear(year-1, prev), true
	}
	return t, true
}

// next is SimpleTimeZone::getNextTransition, its rules in force from a
// year.
func (f *finalZone) next(base int64, inclusive bool, startYear int) (zoneTransition, bool) {
	std := zoneOffset{raw: f.raw}
	dst := zoneOffset{raw: f.raw, dst: f.savings}
	stdDate := f.end.nextStart(base, dst, inclusive, startYear)
	dstDate := f.start.nextStart(base, std, inclusive, startYear)
	if stdDate < dstDate {
		return zoneTransition{at: stdDate, from: dst, to: std}, true
	}
	if dstDate < stdDate {
		return zoneTransition{at: dstDate, from: std, to: dst}, true
	}
	return zoneTransition{}, false
}

// previous is SimpleTimeZone::getPreviousTransition.
func (f *finalZone) previous(base int64, inclusive bool, startYear int) (zoneTransition, bool) {
	std := zoneOffset{raw: f.raw}
	dst := zoneOffset{raw: f.raw, dst: f.savings}
	// The first transition is the earlier of the rules' first starts.
	first := f.end.startInYear(startYear, dst)
	if s := f.start.startInYear(startYear, std); s < first {
		first = s
	}
	if base < first || !inclusive && base == first {
		return zoneTransition{}, false
	}
	stdDate, stdOK := f.end.previousStart(base, dst, inclusive, startYear)
	dstDate, dstOK := f.start.previousStart(base, std, inclusive, startYear)
	if stdOK && (!dstOK || stdDate > dstDate) {
		return zoneTransition{at: stdDate, from: dst, to: std}, true
	}
	if dstOK && (!stdOK || dstDate > stdDate) {
		return zoneTransition{at: dstDate, from: std, to: dst}, true
	}
	return zoneTransition{}, false
}

// gregoDay is Grego::fieldsToDay: the days from 1970 to a date of the
// proleptic Gregorian calendar, its month from zero.
func gregoDay(year, month, dom int) int64 {
	y := int64(year)
	m := int64(month) + 1
	if m <= 2 {
		y--
	}
	era := floorDiv64(y, 400)
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + int64(dom) - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// gregoFields is Grego::dayToFields: the year, the month from zero, the
// day of the month and the weekday, Sunday being one, of a day from 1970.
func gregoFields(day int64) (year, month, dom, dow int) {
	z := day + 719468
	era := floorDiv64(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	m := mp + 3
	if m > 12 {
		m -= 12
	}
	y := yoe + era*400
	if m <= 2 {
		y++
	}
	return int(y), int(m) - 1, int(d), gregoDayOfWeek(day)
}

// gregoDayOfWeek is the weekday of a day from 1970, Sunday being one.
func gregoDayOfWeek(day int64) int {
	return int((day+4)%7+7)%7 + 1
}

func gregoLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// gregoMonthLength is Grego::monthLength, the month from zero.
func gregoMonthLength(year, month int) int {
	switch month {
	case 1:
		if gregoLeap(year) {
			return 29
		}
		return 28
	case 3, 5, 8, 10:
		return 30
	}
	return 31
}
