// Parts of this file are ported to Go from calendrical_calculations 0.2.4
// (Apache-2.0, Copyright 2023 The Unicode Consortium) and changed. See
// internal/gointl/qjsdata/NOTICE-ICU4X.

package temporal

import (
	"sort"

	intl "github.com/liustack/slowjs/internal/gointl"
)

// Time zones as Node's Temporal reckons them: ICU4X's zoneinfo64 crate
// 0.3.0 over ICU's zoneinfo64 data, which is go-intl's data too but read
// by other rules than ICU's OlsonTimeZone. Everything here is in seconds.

// A zoneOffset is zoneinfo64's Offset: the total offset, and whether a
// daylight rule is in force.
type zoneOffset struct {
	offset      int32
	ruleApplies bool
}

// A zoneTransition is zoneinfo64's Transition.
type zoneTransition struct {
	since       int64
	offset      int32
	ruleApplies bool
}

func (t zoneTransition) zoneOffset() zoneOffset { return zoneOffset{t.offset, t.ruleApplies} }

// zoneRuleDate is TzRuleDate.
type zoneRuleDate struct {
	day, weekday, month uint8
	time                uint32
	timeMode            uint8 // 0 wall, 1 standard, 2 UTC
	mode                uint8 // ruleDOWInMonth, ruleDOM, ruleDOWGEQDOM, ruleDOWLEQDOM
}

const (
	ruleDOWInMonth = iota
	ruleDOM
	ruleDOWGEQDOM
	ruleDOWLEQDOM
)

// zoneRule is TzRule, with the Rule's standard offset and first year.
type zoneRule struct {
	additional int32
	start, end zoneRuleDate
	standard   int32
	startYear  int32
}

// newZoneRuleDate is TzRuleDate::new.
func newZoneRuleDate(day, weekday int8, zeroBasedMonth uint8, time uint32, timeMode int8) (zoneRuleDate, bool) {
	month := zeroBasedMonth + 1
	if day == 0 || month > 12 || int64(time) > 86400 {
		return zoneRuleDate{}, false
	}
	if timeMode < 0 || timeMode > 2 {
		return zoneRuleDate{}, false
	}
	var mode uint8
	if weekday == 0 {
		mode = ruleDOM
	} else {
		if weekday > 0 {
			mode = ruleDOWInMonth
		} else {
			weekday = -weekday
			if day > 0 {
				mode = ruleDOWGEQDOM
			} else {
				day = -day
				mode = ruleDOWLEQDOM
			}
		}
		if weekday > 7 {
			return zoneRuleDate{}, false
		}
	}
	if mode == ruleDOWInMonth {
		if day < -5 || day > 5 {
			return zoneRuleDate{}, false
		}
	} else {
		max := int8(30 | (month ^ (month >> 3)))
		if month == 2 {
			max = 29
		}
		if day < 1 || day > max {
			return zoneRuleDate{}, false
		}
	}
	d := uint8(0)
	if day >= 0 {
		d = uint8(day)
	}
	w := uint8(0)
	if weekday-1 >= 0 {
		w = uint8(weekday - 1)
	}
	return zoneRuleDate{day: d, weekday: w, month: month, time: time, timeMode: uint8(timeMode), mode: mode}, true
}

// dayBeforeYear is calendrical_calculations' day_before_year.
func dayBeforeYear(year int32) int64 {
	prev := int64(year) - 1
	const shift = (-(int64(-1<<31)-1)/400 + 1) * 400
	fixed := 365*prev + (prev+shift)/4 - (prev+shift)/100 + (prev+shift)/400 - (shift/4 - shift/100 + shift/400)
	return fixed
}

func gregorianLeap32(year int32) bool {
	if year%25 != 0 {
		return year%4 == 0
	}
	return year%16 == 0
}

// daysBeforeMonth is calendrical_calculations' days_before_month.
func daysBeforeMonth(year int32, month uint8) uint16 {
	if month < 3 {
		if month == 1 {
			return 0
		}
		return 31
	}
	leap := uint16(0)
	if gregorianLeap32(year) {
		leap = 1
	}
	return 31 + 28 + leap + uint16((979*uint32(month)-2919)>>5)
}

// yearFromFixed is calendrical_calculations' year_from_fixed, false beyond
// an i32.
func yearFromFixed(rd int64) (int32, bool) {
	date := rd - 1
	n400, date := floorDiv(date, 146097), floorMod(date, 146097)
	n100, date := date/36524, date%36524
	n4, date := date/1461, date%1461
	n1 := date / 365
	year := 400*n400 + 100*n100 + 4*n4 + n1
	if n100 != 4 && n1 != 4 {
		year++
	}
	if year < -1<<31 || year > 1<<31-1 {
		return 0, false
	}
	return int32(year), true
}

// rdEpoch1970 is the Rata Die of 1970-01-01.
const rdEpoch1970 = 719163

func (d zoneRuleDate) dayInYear(year int32, before int64) uint16 {
	dbm := daysBeforeMonth(year, d.month)
	if d.mode == ruleDOM {
		return dbm + uint16(d.day)
	}
	// weekday of a Rata Die, 0 being Sunday
	const sunday = 0 // fixed_from_gregorian(0, 12, 31)
	weekdayBefore := uint8((before + int64(dbm) - sunday) % 7)
	var dom uint8
	switch d.mode {
	case ruleDOWInMonth:
		first := uint8(7)
		if d.weekday > weekdayBefore {
			first = 0
		}
		first += d.weekday - weekdayBefore
		dom = first + (d.day-1)*7
	case ruleDOWGEQDOM:
		anchor := (weekdayBefore + d.day) % 7
		add := uint8(7)
		if d.weekday >= anchor {
			add = 0
		}
		add += d.weekday - anchor
		dom = d.day + add
	default:
		anchor := (weekdayBefore + d.day) % 7
		sub := uint8(7)
		if d.weekday <= anchor {
			sub = 0
		}
		sub += anchor - d.weekday
		dom = d.day - sub
	}
	return dbm + uint16(dom)
}

func (d zoneRuleDate) timestampForYear(year int32, before int64, standard, additional int32) int64 {
	day := before + int64(d.dayInYear(year, before))
	secs := int32(d.time)
	switch d.timeMode {
	case 1:
		secs -= standard
	case 0:
		secs -= standard + additional
	}
	return (day-rdEpoch1970)*86400 + int64(secs)
}

func (r *zoneRule) endBeforeStart() bool {
	return r.start.month > r.end.month || r.start.month == r.end.month && r.start.day > r.end.day
}

// transition is Rule::transition: the offset before the year's first or
// second transition, and the transition.
func (r *zoneRule) transition(year int32, before int64, second bool) (zoneOffset, zoneTransition) {
	sel, selAdd := r.start, r.additional
	otherAdd := int32(0)
	if r.endBeforeStart() != second {
		sel, selAdd = r.end, 0
		otherAdd = r.additional
	}
	return zoneOffset{r.standard + otherAdd, otherAdd != 0},
		zoneTransition{sel.timestampForYear(year, before, r.standard, otherAdd), r.standard + selAdd, selAdd != 0}
}

func (r *zoneRule) localYear(s int64) (int32, bool) {
	year, ok := yearFromFixed(rdEpoch1970 + s/86400)
	if !ok || year < r.startYear {
		return 0, false
	}
	return year, true
}

func (r *zoneRule) forTimestamp(s int64) (zoneOffset, bool) {
	year, ok := r.localYear(s)
	if !ok {
		return zoneOffset{}, false
	}
	before := dayBeforeYear(year)
	b, first := r.transition(year, before, false)
	if s < first.since {
		if year == r.startYear {
			return zoneOffset{}, false
		}
		return b, true
	}
	_, second := r.transition(year, before, true)
	if s < second.since {
		return first.zoneOffset(), true
	}
	return second.zoneOffset(), true
}

func (r *zoneRule) prevTransition(s int64, exact bool) (zoneTransition, bool) {
	year, ok := r.localYear(s)
	if !ok {
		return zoneTransition{}, false
	}
	before := dayBeforeYear(year)
	_, first := r.transition(year, before, false)
	if exact && s <= first.since || !exact && s < first.since {
		if year == r.startYear {
			return zoneTransition{}, false
		}
		_, t := r.transition(year-1, dayBeforeYear(year-1), true)
		return t, true
	}
	_, second := r.transition(year, before, true)
	if exact && s <= second.since || !exact && s < second.since {
		return first, true
	}
	return second, true
}

func (r *zoneRule) nextTransition(s int64) zoneTransition {
	year, ok := r.localYear(s)
	if !ok {
		year = r.startYear
	}
	before := dayBeforeYear(year)
	_, first := r.transition(year, before, false)
	if s < first.since {
		return first
	}
	_, second := r.transition(year, before, true)
	if s < second.since {
		return second
	}
	_, t := r.transition(year+1, dayBeforeYear(year+1), false)
	return t
}

// zoneInfo is zoneinfo64's TzZoneData for one zone.
type zoneInfo struct {
	types   [][2]int32 // standard and rule-additional offsets
	trans   []int64
	typeMap []uint8
	rule    *zoneRule
}

func newZoneInfo(r *intl.ZoneRecord) (*zoneInfo, error) {
	z := &zoneInfo{trans: r.Transitions, typeMap: r.TransitionTypes}
	for _, t := range r.Types {
		z.types = append(z.types, [2]int32{int32(t[0]), int32(t[1])})
	}
	if len(z.types) == 0 || len(z.trans) != len(z.typeMap) {
		return nil, internalError("inconsistent offset data")
	}
	if r.FinalRule != nil {
		v := r.FinalRule
		if len(v) != 11 {
			return nil, internalError("Invalid rule bits")
		}
		start, ok1 := newZoneRuleDate(int8(v[1]), int8(v[2]), uint8(v[0]), uint32(v[3]), int8(v[4]))
		end, ok2 := newZoneRuleDate(int8(v[6]), int8(v[7]), uint8(v[5]), uint32(v[8]), int8(v[9]))
		if !ok1 || !ok2 {
			return nil, internalError("Invalid rule bits")
		}
		z.rule = &zoneRule{additional: int32(v[10]), start: start, end: end, standard: int32(r.FinalRaw),
			startYear: int32(r.FinalYear)}
	}
	return z, nil
}

// prevIndex is prev_transition_offset_idx: the last transition at or
// before the instant, -1 for none.
func (z *zoneInfo) prevIndex(s int64) int {
	return sort.Search(len(z.trans), func(i int) bool { return z.trans[i] > s }) - 1
}

func (z *zoneInfo) count() int { return len(z.typeMap) }

// at is transition_offset_at.
func (z *zoneInfo) at(i int) zoneTransition {
	if i < 0 || len(z.typeMap) == 0 {
		t := z.types[0]
		return zoneTransition{since: -1 << 63, offset: t[0] + t[1], ruleApplies: t[1] > 0}
	}
	if i > z.count()-1 {
		i = z.count() - 1
	}
	t := z.types[z.typeMap[i]]
	return zoneTransition{since: z.trans[i], offset: t[0] + t[1], ruleApplies: t[1] > 0}
}

// A possibleOffset is PossibleOffset: one offset, two for a repeated
// local time, none for a skipped one, with the offsets either side and the
// transition.
type possibleOffset struct {
	n             int // 0, 1 or 2
	single        zoneOffset
	before, after zoneOffset
	transition    int64
}

// forDateTime is Zone::for_date_time: the offsets a local time can have.
func (z *zoneInfo) forDateTime(year int32, month, day, hour, minute, second uint8) possibleOffset {
	before := dayBeforeYear(year)
	local := (before+int64(daysBeforeMonth(year, month))+int64(day)-rdEpoch1970)*86400 +
		(int64(hour)*60+int64(minute))*60 + int64(second)
	rule := z.rule
	if rule != nil && year < rule.startYear {
		rule = nil
	}
	idx := 0
	var beforeFirst *zoneOffset
	var first zoneTransition
	if rule != nil {
		b, t := rule.transition(year, before, false)
		beforeFirst, first = &b, t
	} else {
		idx = z.prevIndex(local)
		if idx >= 0 {
			b := z.at(idx - 1).zoneOffset()
			beforeFirst = &b
		}
		first = z.at(idx)
	}
	if beforeFirst != nil {
		wallBefore := first.since + int64(beforeFirst.offset)
		wallAfter := first.since + int64(first.offset)
		switch a, b := local < wallBefore, local < wallAfter; {
		case a && b:
			return possibleOffset{n: 1, single: *beforeFirst}
		case a && !b:
			return possibleOffset{n: 2, before: *beforeFirst, after: first.zoneOffset(), transition: first.since}
		case !a && b:
			return possibleOffset{n: 0, before: *beforeFirst, after: first.zoneOffset(), transition: first.since}
		}
	}
	var next *zoneTransition
	if rule != nil {
		_, t := rule.transition(year, before, true)
		next = &t
	} else if idx+1 < z.count() {
		t := z.at(idx + 1)
		next = &t
	}
	if next != nil {
		wallBefore := next.since + int64(first.offset)
		wallAfter := next.since + int64(next.offset)
		switch a, b := local < wallBefore, local < wallAfter; {
		case a && b:
			return possibleOffset{n: 1, single: first.zoneOffset()}
		case a && !b:
			return possibleOffset{n: 2, before: first.zoneOffset(), after: next.zoneOffset(), transition: next.since}
		case !a && b:
			return possibleOffset{n: 0, before: first.zoneOffset(), after: next.zoneOffset(), transition: next.since}
		}
		return possibleOffset{n: 1, single: next.zoneOffset()}
	}
	return possibleOffset{n: 1, single: first.zoneOffset()}
}

// forTimestamp is Zone::for_timestamp.
func (z *zoneInfo) forTimestamp(s int64) zoneOffset {
	idx := z.prevIndex(s)
	if idx == z.count()-1 && z.rule != nil {
		if o, ok := z.rule.forTimestamp(s); ok {
			return o
		}
	}
	return z.at(idx).zoneOffset()
}

// prevTransition is Zone::prev_transition.
func (z *zoneInfo) prevTransition(s int64, exact, requireChange bool) (zoneTransition, bool) {
	idx := z.prevIndex(s)
	if idx == -1 {
		return zoneTransition{}, false
	}
	if idx == z.count()-1 && z.rule != nil {
		if t, ok := z.rule.prevTransition(s, exact); ok {
			return t, true
		}
	}
	c := z.at(idx)
	if c.since == s && exact {
		if idx <= 0 {
			return zoneTransition{}, false
		}
		idx--
		c = z.at(idx)
	}
	for requireChange && idx > 0 {
		p := z.at(idx - 1)
		if p.offset != c.offset {
			break
		}
		c = p
		idx--
	}
	return c, true
}

// nextTransition is Zone::next_transition.
func (z *zoneInfo) nextTransition(s int64, requireChange bool) (zoneTransition, bool) {
	idx := z.prevIndex(s)
	for requireChange && idx < z.count()-1 && z.at(idx).offset == z.at(idx+1).offset {
		idx++
	}
	if idx == z.count()-1 {
		if z.rule == nil {
			return zoneTransition{}, false
		}
		return z.rule.nextTransition(s), true
	}
	return z.at(idx + 1), true
}
