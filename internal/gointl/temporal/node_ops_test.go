//go:build gointl_all

package temporal

import (
	"math"
	"math/big"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The methods of Temporal's types other than Duration, as V8's
// js-temporal-objects.cc binds them.

// ==== Shared glue ====

// roundTo is round's reading of its argument: a string is the smallest
// unit, and V8 checks for an object itself except in Instant's round.
func roundTo(v any, optionsObject bool) (*jsObject, error) {
	switch x := v.(type) {
	case jsUndefined:
		return nil, typeErr("Must specify a roundTo parameter.")
	case string:
		return newObject("smallestUnit", x), nil
	case *jsObject:
		return x, nil
	}
	if !isObject(v) {
		if optionsObject {
			return nil, &jsError{"TypeError", "invalid_argument"}
		}
		return nil, typeErr("roundTo must be an object.")
	}
	return newObject(), nil
}

// roundingOptions reads round's options: the increment, the mode, and the
// required smallest unit, validated against the time units and extra.
func roundingOptions(o *jsObject, method string, extra Unit) (RoundingOptions, error) {
	inc, err := getRoundingIncrement(o)
	if err != nil {
		return RoundingOptions{}, err
	}
	mode, err := getRoundingMode(o, method, HalfExpand)
	if err != nil {
		return RoundingOptions{}, err
	}
	smallest, err := getUnitOption(o, "smallestUnit", method, true)
	if err != nil {
		return RoundingOptions{}, err
	}
	if err := validateUnit(smallest, groupTime, extra); err != nil {
		return RoundingOptions{}, err
	}
	return RoundingOptions{LargestUnit: NoUnit, SmallestUnit: smallest, RoundingMode: mode, Increment: inc, Compat: intl.NodeICU}, nil
}

// differenceSettings is GetDifferenceSettingsWithoutChecks.
func differenceSettings(opts any, method string) (DifferenceSettings, error) {
	o, err := getOptionsObject(opts)
	if err != nil {
		return DifferenceSettings{}, err
	}
	largest, err := getUnitOption(o, "largestUnit", method, false)
	if err != nil {
		return DifferenceSettings{}, err
	}
	inc, err := getRoundingIncrement(o)
	if err != nil {
		return DifferenceSettings{}, err
	}
	mode, err := getRoundingMode(o, method, Trunc)
	if err != nil {
		return DifferenceSettings{}, err
	}
	smallest, err := getUnitOption(o, "smallestUnit", method, false)
	if err != nil {
		return DifferenceSettings{}, err
	}
	return DifferenceSettings{LargestUnit: largest, SmallestUnit: smallest, RoundingMode: mode, Increment: inc, Compat: intl.NodeICU}, nil
}

func sameCalendar(a, b *Calendar) error {
	if !a.Equal(b) {
		return &jsError{"RangeError", "Mismatched calendars."}
	}
	return nil
}

// showCalendar is GetTemporalShowCalendarNameOption.
func showCalendar(o *jsObject, method string) (DisplayCalendar, error) {
	i, err := getStringOption(o, "calendarName", method, []string{"auto", "always", "never", "critical"}, 0)
	return []DisplayCalendar{CalendarAuto, CalendarAlways, CalendarNever, CalendarCritical}[i], err
}

// toStringOptions reads the fractional digits, the rounding mode and the
// smallest unit, validated as a time unit.
func toStringOptions(o *jsObject, method string, between func() error) (ToStringRoundingOptions, error) {
	digits, err := getFractionalSecondDigits(o)
	if err != nil {
		return ToStringRoundingOptions{}, err
	}
	if between != nil {
		if err := between(); err != nil {
			return ToStringRoundingOptions{}, err
		}
	}
	mode, err := getRoundingMode(o, method, Trunc)
	if err != nil {
		return ToStringRoundingOptions{}, err
	}
	smallest, err := getUnitOption(o, "smallestUnit", method, false)
	if err != nil {
		return ToStringRoundingOptions{}, err
	}
	return ToStringRoundingOptions{Precision: digits, SmallestUnit: smallest, RoundingMode: mode}, nil
}

// isPartialTemporalObject is IsPartialTemporalObject.
func isPartialTemporalObject(v any) bool {
	o, ok := v.(*jsObject)
	if !ok {
		return false
	}
	return isUndefined(o.get("calendar")) && isUndefined(o.get("timeZone"))
}

func withNoPartial() error { return typeErr("Argument to with() must contain some date/time fields.") }

// disambiguationOption is GetTemporalDisambiguationOptionHandleUndefined.
func disambiguationOption(opts any, method string) (Disambiguation, error) {
	if isUndefined(opts) {
		return Compatible, nil
	}
	o, ok := opts.(*jsObject)
	if !ok {
		if !isObject(opts) {
			return 0, typeErrWithArg("Option must be object:", "disambiguation")
		}
		o = newObject()
	}
	i, err := getStringOption(o, "disambiguation", method, []string{"compatible", "earlier", "later", "reject"}, 0)
	return Disambiguation(i), err
}

// offsetOption is GetTemporalOffsetOptionHandleUndefined.
func offsetOption(opts any, method string, fallback OffsetDisambiguation) (OffsetDisambiguation, error) {
	if isUndefined(opts) {
		return fallback, nil
	}
	o, ok := opts.(*jsObject)
	if !ok {
		if !isObject(opts) {
			return 0, typeErrWithArg("Option must be object:", "offset")
		}
		o = newObject()
	}
	offsets := []OffsetDisambiguation{OffsetPrefer, OffsetUse, OffsetIgnore, OffsetReject}
	def := 0
	for i, x := range offsets {
		if x == fallback {
			def = i
		}
	}
	i, err := getStringOption(o, "offset", method, []string{"prefer", "use", "ignore", "reject"}, def)
	return offsets[i], err
}

// calendarArg is a constructor's calendar: ISO where undefined, else a
// string CanonicalizeCalendar takes.
func (h *harness) calendarArg(v any) (*Calendar, error) {
	if isUndefined(v) {
		return ISOCalendar(), nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, typeErr("Calendar must be string.")
	}
	return h.canonicalizeCalendar(s)
}

// isValidISODate is V8's IsValidIsoDate.
func isValidISODate(y, m, d float64) bool {
	if m < 1 || m > 12 || !inRange(y, math.MinInt32, math.MaxInt32) {
		return false
	}
	return d >= 1 && d <= float64(gregorianMonthLength(int(y), int(m)))
}

func isValidTime(t [6]float64) bool {
	maxes := [6]float64{23, 59, 59, 999, 999, 999}
	for i, v := range t {
		if v < 0 || v > maxes[i] {
			return false
		}
	}
	return true
}

// timeArgs reads a constructor's time arguments, from index i.
func timeArgs(arg func(int) any, i int) ([6]float64, error) {
	var t [6]float64
	for j := range t {
		v := arg(i + j)
		if isUndefined(v) {
			continue
		}
		f, err := toIntegerWithTruncation(v)
		if err != nil {
			return t, err
		}
		t[j] = f
	}
	return t, nil
}

func isoTimeOf(t [6]float64) ISOTime {
	return ISOTime{int(t[0]), int(t[1]), int(t[2]), int(t[3]), int(t[4]), int(t[5])}
}

// toBigInt is BigInt::FromObject for the values the recording has.
func toBigInt(v any) (*big.Int, error) {
	switch x := v.(type) {
	case *big.Int:
		return x, nil
	case string:
		if b, ok := new(big.Int).SetString(jsTrim(x), 0); ok {
			return b, nil
		}
		return nil, &jsError{"SyntaxError", "Cannot convert " + x + " to a BigInt"}
	case bool:
		if x {
			return big.NewInt(1), nil
		}
		return big.NewInt(0), nil
	}
	s, _ := toString(v)
	return nil, &jsError{"TypeError", "Cannot convert " + s + " to a BigInt"}
}

// epochNanosecondsFromBigInt is GetI128FromBigInt, which checks the range
// itself.
func epochNanosecondsFromBigInt(b *big.Int) (int128, error) {
	if new(big.Int).Abs(b).BitLen() > 127 {
		return int128{}, rangeErr("Nanoseconds out of range.")
	}
	m := new(big.Int).And(b, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1)))
	lo := new(big.Int).And(m, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
	hi := new(big.Int).Rsh(m, 64).Uint64()
	ns := int128{int64(hi), lo}
	if !validEpochNanoseconds(ns) {
		return int128{}, rangeErr("Nanoseconds out of range.")
	}
	return ns, nil
}

func int128ToBigInt(v int128) *big.Int {
	b := new(big.Int).Lsh(big.NewInt(v.hi), 64)
	return b.Add(b, new(big.Int).SetUint64(v.lo))
}

// timeOrMidnight is ToTimeRecordOrMidnight.
func (h *harness) timeOrMidnight(v any, method string) (*PlainTime, error) {
	if isUndefined(v) {
		return nil, nil
	}
	t, err := h.toPlainTime(v, jsUndefined{}, method)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullableString(s string, ok bool) any {
	if !ok || s == "" {
		return jsUndefined{}
	}
	return s
}

func nullableInt(v int, ok bool) any {
	if !ok {
		return jsUndefined{}
	}
	return float64(v)
}

// dateGetters are the getters of a calendar date, for every type with one.
func dateGetters(m map[string]opFunc, typ string, date func(any) PlainDate) {
	g := map[string]func(PlainDate) any{
		"era":          func(d PlainDate) any { return nullableString(d.Era()) },
		"eraYear":      func(d PlainDate) any { return nullableInt(d.EraYear()) },
		"year":         func(d PlainDate) any { return float64(d.Year()) },
		"month":        func(d PlainDate) any { return float64(d.Month()) },
		"monthCode":    func(d PlainDate) any { return d.MonthCode() },
		"day":          func(d PlainDate) any { return float64(d.Day()) },
		"dayOfWeek":    func(d PlainDate) any { return float64(d.DayOfWeek()) },
		"dayOfYear":    func(d PlainDate) any { return float64(d.DayOfYear()) },
		"weekOfYear":   func(d PlainDate) any { return nullableInt(d.WeekOfYear()) },
		"yearOfWeek":   func(d PlainDate) any { return nullableInt(d.YearOfWeek()) },
		"daysInWeek":   func(d PlainDate) any { return float64(d.DaysInWeek()) },
		"daysInMonth":  func(d PlainDate) any { return float64(d.DaysInMonth()) },
		"daysInYear":   func(d PlainDate) any { return float64(d.DaysInYear()) },
		"monthsInYear": func(d PlainDate) any { return float64(d.MonthsInYear()) },
		"inLeapYear":   func(d PlainDate) any { return d.InLeapYear() },
		"calendarId":   func(d PlainDate) any { return d.Calendar().ID() },
	}
	for name, f := range g {
		f := f
		m["get:"+typ+"."+name] = func(this any, _ func(int) any) (any, error) { return f(date(this)), nil }
	}
}

func timeGetters(m map[string]opFunc, typ string, time func(any) ISOTime) {
	g := map[string]func(ISOTime) int{
		"hour":        func(t ISOTime) int { return t.Hour },
		"minute":      func(t ISOTime) int { return t.Minute },
		"second":      func(t ISOTime) int { return t.Second },
		"millisecond": func(t ISOTime) int { return t.Millisecond },
		"microsecond": func(t ISOTime) int { return t.Microsecond },
		"nanosecond":  func(t ISOTime) int { return t.Nanosecond },
	}
	for name, f := range g {
		f := f
		m["get:"+typ+"."+name] = func(this any, _ func(int) any) (any, error) { return float64(f(time(this))), nil }
	}
}

// jsonOps are toJSON: toString with its defaults.
func (h *harness) jsonOps(m map[string]opFunc) {
	m["method:PlainDate.toJSON"] = func(this any, _ func(int) any) (any, error) { return this.(PlainDate).String(CalendarAuto), nil }
	m["method:PlainYearMonth.toJSON"] = func(this any, _ func(int) any) (any, error) {
		return this.(PlainYearMonth).String(CalendarAuto), nil
	}
	m["method:PlainMonthDay.toJSON"] = func(this any, _ func(int) any) (any, error) {
		return this.(PlainMonthDay).String(CalendarAuto), nil
	}
	m["method:PlainTime.toJSON"] = func(this any, _ func(int) any) (any, error) {
		s, err := this.(PlainTime).String(DefaultToStringOptions())
		return s, rustErr(err)
	}
	m["method:PlainDateTime.toJSON"] = func(this any, _ func(int) any) (any, error) {
		s, err := this.(PlainDateTime).String(DefaultToStringOptions(), CalendarAuto)
		return s, rustErr(err)
	}
	m["method:Instant.toJSON"] = func(this any, _ func(int) any) (any, error) {
		s, err := this.(Instant).String(h.zones, nil, DefaultToStringOptions())
		return s, rustErr(err)
	}
	m["method:ZonedDateTime.toJSON"] = func(this any, _ func(int) any) (any, error) {
		s, err := this.(ZonedDateTime).String(OffsetAuto, TimeZoneAuto, CalendarAuto, DefaultToStringOptions())
		return s, rustErr(err)
	}
}

// ==== PlainDate ====

func (h *harness) plainDateOps(m map[string]opFunc) {
	pd := func(v any) PlainDate { return v.(PlainDate) }
	dateGetters(m, "PlainDate", pd)
	m["new:PlainDate"] = func(_ any, arg func(int) any) (any, error) {
		var f [3]float64
		for i := range f {
			n, err := toIntegerWithTruncation(arg(i))
			if err != nil {
				return nil, err
			}
			f[i] = n
		}
		cal, err := h.calendarArg(arg(3))
		if err != nil {
			return nil, err
		}
		if !isValidISODate(f[0], f[1], f[2]) {
			return nil, rangeErr("Invalid ISO date.")
		}
		d, err := NewPlainDate(int(f[0]), int(f[1]), int(f[2]), cal, Reject)
		return d, rustErr(err)
	}
	m["static:PlainDate.compare"] = func(_ any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDate.compare"
		a, err := h.toPlainDate(arg(0), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		b, err := h.toPlainDate(arg(1), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		return float64(a.Compare(b)), nil
	}
	m["method:PlainDate.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toPlainDate(arg(0), jsUndefined{}, "Temporal.PlainDate.prototype.equals")
		if err != nil {
			return nil, err
		}
		return pd(this).Equals(o), nil
	}
	m["method:PlainDate.toPlainYearMonth"] = func(this any, _ func(int) any) (any, error) {
		ym, err := pd(this).ToPlainYearMonth()
		return ym, rustErr(err)
	}
	m["method:PlainDate.toPlainMonthDay"] = func(this any, _ func(int) any) (any, error) {
		md, err := pd(this).ToPlainMonthDay()
		return md, rustErr(err)
	}
	m["method:PlainDate.toPlainDateTime"] = func(this any, arg func(int) any) (any, error) {
		t, err := h.timeOrMidnight(arg(0), "Temporal.PlainDate.toPlainDateTime")
		if err != nil {
			return nil, err
		}
		dt, err := pd(this).ToPlainDateTime(t)
		return dt, rustErr(err)
	}
	m["method:PlainDate.with"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDate.prototype.with"
		d := pd(this)
		if !isPartialTemporalObject(arg(0)) {
			return nil, withNoPartial()
		}
		r, err := h.prepareFields(d.Calendar(), arg(0), fieldsAllDate, requirePartial)
		if err != nil {
			return nil, err
		}
		ov, err := getOverflow(arg(1), method)
		if err != nil {
			return nil, err
		}
		f, err := r.regulateDate(ov)
		if err != nil {
			return nil, err
		}
		out, err := d.With(f, ov)
		return out, rustErr(err)
	}
	m["method:PlainDate.withCalendar"] = func(this any, arg func(int) any) (any, error) {
		c, err := h.toCalendar(arg(0))
		if err != nil {
			return nil, err
		}
		return pd(this).WithCalendar(c), nil
	}
	m["method:PlainDate.toZonedDateTime"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDate.toZonedDateTime"
		item := arg(0)
		var tz TimeZone
		var timeLike any = jsUndefined{}
		var err error
		if isObject(item) {
			tzLike := get(item, "timeZone")
			if isUndefined(tzLike) {
				tz, err = h.toTimeZone(item)
			} else {
				tz, err = h.toTimeZone(tzLike)
				timeLike = get(item, "plainTime")
			}
		} else {
			tz, err = h.toTimeZone(item)
		}
		if err != nil {
			return nil, err
		}
		var t *PlainTime
		if !isUndefined(timeLike) {
			pt, err := h.toPlainTime(timeLike, jsUndefined{}, method)
			if err != nil {
				return nil, err
			}
			t = &pt
		}
		z, err := pd(this).ToZonedDateTime(tz, t)
		return z, rustErr(err)
	}
	for _, sub := range []bool{false, true} {
		sub := sub
		name := "add"
		if sub {
			name = "subtract"
		}
		m["method:PlainDate."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainDate.prototype." + name
			dur, err := h.toDuration(arg(0))
			if err != nil {
				return nil, err
			}
			ov, err := getOverflow(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out PlainDate
			if sub {
				out, err = pd(this).Subtract(dur, ov)
			} else {
				out, err = pd(this).Add(dur, ov)
			}
			return out, rustErr(err)
		}
	}
	for _, since := range []bool{false, true} {
		since := since
		name := "until"
		if since {
			name = "since"
		}
		m["method:PlainDate."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainDate.prototype." + name
			o, err := h.toPlainDate(arg(0), jsUndefined{}, method)
			if err != nil {
				return nil, err
			}
			if err := sameCalendar(pd(this).Calendar(), o.Calendar()); err != nil {
				return nil, err
			}
			s, err := differenceSettings(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out Duration
			if since {
				out, err = pd(this).Since(o, s)
			} else {
				out, err = pd(this).Until(o, s)
			}
			return out, rustErr(err)
		}
	}
	m["method:PlainDate.toString"] = func(this any, arg func(int) any) (any, error) {
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		show, err := showCalendar(o, "Temporal.PlainDate.prototype.toString")
		if err != nil {
			return nil, err
		}
		return pd(this).String(show), nil
	}
}

// ==== PlainTime ====

func (h *harness) plainTimeOps(m map[string]opFunc) {
	pt := func(v any) PlainTime { return v.(PlainTime) }
	timeGetters(m, "PlainTime", func(v any) ISOTime { return pt(v).ISO() })
	m["new:PlainTime"] = func(_ any, arg func(int) any) (any, error) {
		t, err := timeArgs(arg, 0)
		if err != nil {
			return nil, err
		}
		if !isValidTime(t) {
			return nil, rangeErr("Invalid time")
		}
		out, err := NewPlainTime(int(t[0]), int(t[1]), int(t[2]), int(t[3]), int(t[4]), int(t[5]), Reject)
		return out, rustErr(err)
	}
	m["static:PlainTime.compare"] = func(_ any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainTime.compare"
		a, err := h.toPlainTime(arg(0), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		b, err := h.toPlainTime(arg(1), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		return float64(a.Compare(b)), nil
	}
	m["method:PlainTime.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toPlainTime(arg(0), jsUndefined{}, "Temporal.PlainTime.prototype.equals")
		if err != nil {
			return nil, err
		}
		return pt(this).Compare(o) == 0, nil
	}
	m["method:PlainTime.round"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDateTime.prototype.round"
		o, err := roundTo(arg(0), false)
		if err != nil {
			return nil, err
		}
		r, err := roundingOptions(o, method, NoUnit)
		if err != nil {
			return nil, err
		}
		out, err := pt(this).Round(r)
		return out, rustErr(err)
	}
	m["method:PlainTime.with"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainTime.prototype.with"
		v := arg(0)
		if !isPartialTemporalObject(v) {
			return nil, withNoPartial()
		}
		var t [6]*float64
		found := false
		for i, key := range []string{"hour", "microsecond", "millisecond", "minute", "nanosecond", "second"} {
			x := get(v, key)
			if isUndefined(x) {
				continue
			}
			f, err := toIntegerWithTruncation(x)
			if err != nil {
				return nil, err
			}
			found = true
			t[[6]int{0, 4, 3, 1, 5, 2}[i]] = &f
		}
		if !found {
			return nil, typeErr("Must specify at least one time field.")
		}
		ov, err := getOverflow(arg(1), method)
		if err != nil {
			return nil, err
		}
		p, err := regulateTime(t, ov)
		if err != nil {
			return nil, err
		}
		out, err := pt(this).With(p, ov)
		return out, rustErr(err)
	}
	for _, sub := range []bool{false, true} {
		sub := sub
		name := "add"
		if sub {
			name = "subtract"
		}
		m["method:PlainTime."+name] = func(this any, arg func(int) any) (any, error) {
			dur, err := h.toDuration(arg(0))
			if err != nil {
				return nil, err
			}
			if sub {
				return pt(this).Subtract(dur), nil
			}
			return pt(this).Add(dur), nil
		}
	}
	for _, since := range []bool{false, true} {
		since := since
		name := "until"
		if since {
			name = "since"
		}
		m["method:PlainTime."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainTime.prototype." + name
			o, err := h.toPlainTime(arg(0), jsUndefined{}, method)
			if err != nil {
				return nil, err
			}
			s, err := differenceSettings(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out Duration
			if since {
				out, err = pt(this).Since(o, s)
			} else {
				out, err = pt(this).Until(o, s)
			}
			return out, rustErr(err)
		}
	}
	m["method:PlainTime.toString"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainTime.prototype.toString"
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		opts, err := toStringOptions(o, method, nil)
		if err != nil {
			return nil, err
		}
		if err := validateUnit(opts.SmallestUnit, groupTime, NoUnit); err != nil {
			return nil, err
		}
		s, err := pt(this).String(opts)
		return s, rustErr(err)
	}
}

// ==== PlainDateTime ====

func (h *harness) plainDateTimeOps(m map[string]opFunc) {
	pdt := func(v any) PlainDateTime { return v.(PlainDateTime) }
	dateGetters(m, "PlainDateTime", func(v any) PlainDate { return pdt(v).ToPlainDate() })
	timeGetters(m, "PlainDateTime", func(v any) ISOTime { return pdt(v).ISO().Time })
	m["new:PlainDateTime"] = func(_ any, arg func(int) any) (any, error) {
		var f [3]float64
		for i := range f {
			n, err := toIntegerWithTruncation(arg(i))
			if err != nil {
				return nil, err
			}
			f[i] = n
		}
		t, err := timeArgs(arg, 3)
		if err != nil {
			return nil, err
		}
		cal, err := h.calendarArg(arg(9))
		if err != nil {
			return nil, err
		}
		if !isValidISODate(f[0], f[1], f[2]) {
			return nil, rangeErr("Invalid ISO date.")
		}
		if !isValidTime(t) {
			return nil, rangeErr("Invalid time")
		}
		out, err := NewPlainDateTime(ISODate{int(f[0]), int(f[1]), int(f[2])}, isoTimeOf(t), cal, Reject)
		return out, rustErr(err)
	}
	m["static:PlainDateTime.compare"] = func(_ any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDateTime.compare"
		a, err := h.toPlainDateTime(arg(0), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		b, err := h.toPlainDateTime(arg(1), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		return float64(a.Compare(b)), nil
	}
	m["method:PlainDateTime.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toPlainDateTime(arg(0), jsUndefined{}, "Temporal.PlainDateTime.prototype.equals")
		if err != nil {
			return nil, err
		}
		return pdt(this).Equals(o), nil
	}
	m["method:PlainDateTime.with"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDateTime.prototype.with"
		dt := pdt(this)
		if !isPartialTemporalObject(arg(0)) {
			return nil, withNoPartial()
		}
		r, err := h.prepareFields(dt.Calendar(), arg(0), fieldsAllDate|fieldsTime, requirePartial)
		if err != nil {
			return nil, err
		}
		ov, err := getOverflow(arg(1), method)
		if err != nil {
			return nil, err
		}
		f, err := r.regulateDate(ov)
		if err != nil {
			return nil, err
		}
		t, err := regulateTime(r.timeFields(), ov)
		if err != nil {
			return nil, err
		}
		out, err := dt.With(f, t, ov)
		return out, rustErr(err)
	}
	m["method:PlainDateTime.withCalendar"] = func(this any, arg func(int) any) (any, error) {
		c, err := h.toCalendar(arg(0))
		if err != nil {
			return nil, err
		}
		return pdt(this).WithCalendar(c), nil
	}
	m["method:PlainDateTime.withPlainTime"] = func(this any, arg func(int) any) (any, error) {
		t, err := h.timeOrMidnight(arg(0), "Temporal.PlainDateTime.prototype.withPlainTime")
		if err != nil {
			return nil, err
		}
		out, err := pdt(this).WithPlainTime(t)
		return out, rustErr(err)
	}
	m["method:PlainDateTime.toZonedDateTime"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDateTime.prototype.toZonedDateTime"
		tz, err := h.toTimeZone(arg(0))
		if err != nil {
			return nil, err
		}
		d, err := disambiguationOption(arg(1), method)
		if err != nil {
			return nil, err
		}
		z, err := pdt(this).ToZonedDateTime(tz, d)
		return z, rustErr(err)
	}
	m["method:PlainDateTime.toString"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.DateTime.prototype.toString"
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		show, err := showCalendar(o, method)
		if err != nil {
			return nil, err
		}
		opts, err := toStringOptions(o, method, nil)
		if err != nil {
			return nil, err
		}
		if err := validateUnit(opts.SmallestUnit, groupTime, NoUnit); err != nil {
			return nil, err
		}
		s, err := pdt(this).String(opts, show)
		return s, rustErr(err)
	}
	m["method:PlainDateTime.round"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDateTime.prototype.round"
		o, err := roundTo(arg(0), false)
		if err != nil {
			return nil, err
		}
		r, err := roundingOptions(o, method, Day)
		if err != nil {
			return nil, err
		}
		out, err := pdt(this).Round(r)
		return out, rustErr(err)
	}
	for _, sub := range []bool{false, true} {
		sub := sub
		name := "add"
		if sub {
			name = "subtract"
		}
		m["method:PlainDateTime."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainDateTime.prototype." + name
			dur, err := h.toDuration(arg(0))
			if err != nil {
				return nil, err
			}
			ov, err := getOverflow(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out PlainDateTime
			if sub {
				out, err = pdt(this).Subtract(dur, ov)
			} else {
				out, err = pdt(this).Add(dur, ov)
			}
			return out, rustErr(err)
		}
	}
	for _, since := range []bool{false, true} {
		since := since
		name := "until"
		if since {
			name = "since"
		}
		m["method:PlainDateTime."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainDateTime.prototype." + name
			o, err := h.toPlainDateTime(arg(0), jsUndefined{}, method)
			if err != nil {
				return nil, err
			}
			if err := sameCalendar(pdt(this).Calendar(), o.Calendar()); err != nil {
				return nil, err
			}
			s, err := differenceSettings(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out Duration
			if since {
				out, err = pdt(this).Since(o, s)
			} else {
				out, err = pdt(this).Until(o, s)
			}
			return out, rustErr(err)
		}
	}
	m["method:PlainDateTime.toPlainDate"] = func(this any, _ func(int) any) (any, error) { return pdt(this).ToPlainDate(), nil }
	m["method:PlainDateTime.toPlainTime"] = func(this any, _ func(int) any) (any, error) { return pdt(this).ToPlainTime(), nil }
}

// ==== PlainYearMonth and PlainMonthDay ====

func (h *harness) yearMonthOps(m map[string]opFunc) {
	ymOf := func(v any) PlainYearMonth { return v.(PlainYearMonth) }
	getters := map[string]func(PlainYearMonth) any{
		"era":          func(ym PlainYearMonth) any { return nullableString(ym.Era()) },
		"eraYear":      func(ym PlainYearMonth) any { return nullableInt(ym.EraYear()) },
		"year":         func(ym PlainYearMonth) any { return float64(ym.Year()) },
		"month":        func(ym PlainYearMonth) any { return float64(ym.Month()) },
		"monthCode":    func(ym PlainYearMonth) any { return ym.MonthCode() },
		"daysInMonth":  func(ym PlainYearMonth) any { return float64(ym.DaysInMonth()) },
		"daysInYear":   func(ym PlainYearMonth) any { return float64(ym.DaysInYear()) },
		"monthsInYear": func(ym PlainYearMonth) any { return float64(ym.MonthsInYear()) },
		"inLeapYear":   func(ym PlainYearMonth) any { return ym.InLeapYear() },
		"calendarId":   func(ym PlainYearMonth) any { return ym.Calendar().ID() },
	}
	for name, g := range getters {
		g := g
		m["get:PlainYearMonth."+name] = func(this any, _ func(int) any) (any, error) { return g(ymOf(this)), nil }
	}
	m["new:PlainYearMonth"] = func(_ any, arg func(int) any) (any, error) {
		y, err := toIntegerWithTruncation(arg(0))
		if err != nil {
			return nil, err
		}
		mo, err := toIntegerWithTruncation(arg(1))
		if err != nil {
			return nil, err
		}
		cal, err := h.calendarArg(arg(2))
		if err != nil {
			return nil, err
		}
		ref := 1.0
		if !isUndefined(arg(3)) {
			if ref, err = toIntegerWithTruncation(arg(3)); err != nil {
				return nil, err
			}
		}
		if !isValidISODate(y, mo, ref) {
			return nil, rangeErr("Invalid ISO date.")
		}
		day := int(ref)
		out, err := NewPlainYearMonth(int(y), int(mo), &day, cal, Reject)
		return out, rustErr(err)
	}
	m["static:PlainYearMonth.compare"] = func(_ any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainYearMonth.compare"
		a, err := h.toPlainYearMonth(arg(0), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		b, err := h.toPlainYearMonth(arg(1), jsUndefined{}, method)
		if err != nil {
			return nil, err
		}
		return float64(a.Compare(b)), nil
	}
	m["method:PlainYearMonth.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toPlainYearMonth(arg(0), jsUndefined{}, "Temporal.PlainYearMonth.prototype.equals")
		if err != nil {
			return nil, err
		}
		return ymOf(this).Equals(o), nil
	}
	for _, sub := range []bool{false, true} {
		sub := sub
		name := "add"
		if sub {
			name = "subtract"
		}
		m["method:PlainYearMonth."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainYearMonth.prototype." + name
			dur, err := h.toDuration(arg(0))
			if err != nil {
				return nil, err
			}
			ov, err := getOverflow(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out PlainYearMonth
			if sub {
				out, err = ymOf(this).Subtract(dur, ov)
			} else {
				out, err = ymOf(this).Add(dur, ov)
			}
			return out, rustErr(err)
		}
	}
	for _, since := range []bool{false, true} {
		since := since
		name := "until"
		if since {
			name = "since"
		}
		m["method:PlainYearMonth."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.PlainYearMonth.prototype." + name
			o, err := h.toPlainYearMonth(arg(0), jsUndefined{}, method)
			if err != nil {
				return nil, err
			}
			if err := sameCalendar(ymOf(this).Calendar(), o.Calendar()); err != nil {
				return nil, err
			}
			s, err := differenceSettings(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out Duration
			if since {
				out, err = ymOf(this).Since(o, s)
			} else {
				out, err = ymOf(this).Until(o, s)
			}
			return out, rustErr(err)
		}
	}
	m["method:PlainYearMonth.with"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainYearMonth.prototype.with"
		ym := ymOf(this)
		if !isPartialTemporalObject(arg(0)) {
			return nil, withNoPartial()
		}
		r, err := h.prepareFields(ym.Calendar(), arg(0), fieldsYear|fieldsMonth, requirePartial)
		if err != nil {
			return nil, err
		}
		ov, err := getOverflow(arg(1), method)
		if err != nil {
			return nil, err
		}
		f, err := r.regulateDate(ov)
		if err != nil {
			return nil, err
		}
		out, err := ym.With(f, ov)
		return out, rustErr(err)
	}
	m["method:PlainYearMonth.toPlainDate"] = func(this any, arg func(int) any) (any, error) {
		ym := ymOf(this)
		if !isObject(arg(0)) {
			return nil, typeErr("year argument must be an object.")
		}
		r, err := h.prepareFields(ym.Calendar(), arg(0), fieldsDay, requireNone)
		if err != nil {
			return nil, err
		}
		f, err := r.regulateDate(Constrain)
		if err != nil {
			return nil, err
		}
		out, err := ym.ToPlainDate(f.Day)
		return out, rustErr(err)
	}
	m["method:PlainYearMonth.toString"] = func(this any, arg func(int) any) (any, error) {
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		show, err := showCalendar(o, "Temporal.PlainYearMonth.prototype.toString")
		if err != nil {
			return nil, err
		}
		return ymOf(this).String(show), nil
	}

	mdOf := func(v any) PlainMonthDay { return v.(PlainMonthDay) }
	m["get:PlainMonthDay.monthCode"] = func(this any, _ func(int) any) (any, error) { return mdOf(this).MonthCode(), nil }
	m["get:PlainMonthDay.day"] = func(this any, _ func(int) any) (any, error) { return float64(mdOf(this).Day()), nil }
	m["get:PlainMonthDay.calendarId"] = func(this any, _ func(int) any) (any, error) { return mdOf(this).Calendar().ID(), nil }
	m["new:PlainMonthDay"] = func(_ any, arg func(int) any) (any, error) {
		mo, err := toIntegerWithTruncation(arg(0))
		if err != nil {
			return nil, err
		}
		d, err := toIntegerWithTruncation(arg(1))
		if err != nil {
			return nil, err
		}
		cal, err := h.calendarArg(arg(2))
		if err != nil {
			return nil, err
		}
		ref := 1972.0
		if !isUndefined(arg(3)) {
			if ref, err = toIntegerWithTruncation(arg(3)); err != nil {
				return nil, err
			}
		}
		if !isValidISODate(ref, mo, d) {
			return nil, rangeErr("Invalid ISO date.")
		}
		year := int(ref)
		out, err := NewPlainMonthDay(int(mo), int(d), cal, Reject, &year)
		return out, rustErr(err)
	}
	m["method:PlainMonthDay.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toPlainMonthDay(arg(0), jsUndefined{}, "Temporal.PlainMonthDay.prototype.equals")
		if err != nil {
			return nil, err
		}
		return mdOf(this).Equals(o), nil
	}
	m["method:PlainMonthDay.with"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainYearMonth.prototype.with"
		md := mdOf(this)
		if !isPartialTemporalObject(arg(0)) {
			return nil, withNoPartial()
		}
		r, err := h.prepareFields(md.Calendar(), arg(0), fieldsYear|fieldsMonth|fieldsDay, requirePartial)
		if err != nil {
			return nil, err
		}
		ov, err := getOverflow(arg(1), method)
		if err != nil {
			return nil, err
		}
		f, err := r.regulateDate(ov)
		if err != nil {
			return nil, err
		}
		out, err := md.With(f, ov)
		return out, rustErr(err)
	}
	m["method:PlainMonthDay.toPlainDate"] = func(this any, arg func(int) any) (any, error) {
		md := mdOf(this)
		if !isObject(arg(0)) {
			return nil, typeErr("year argument must be an object.")
		}
		r, err := h.prepareFields(md.Calendar(), arg(0), fieldsYear, requireNone)
		if err != nil {
			return nil, err
		}
		f, err := r.regulateDate(Constrain)
		if err != nil {
			return nil, err
		}
		out, err := md.ToPlainDate(f.Year, f.Era, f.EraYear)
		return out, rustErr(err)
	}
	m["method:PlainMonthDay.toString"] = func(this any, arg func(int) any) (any, error) {
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		show, err := showCalendar(o, "Temporal.PlainMonthDay.prototype.toString")
		if err != nil {
			return nil, err
		}
		return mdOf(this).String(show), nil
	}
}

// ==== Instant ====

func (h *harness) instantOps(m map[string]opFunc) {
	in := func(v any) Instant { return v.(Instant) }
	fromBigInt := func(v any) (any, error) {
		b, err := toBigInt(v)
		if err != nil {
			return nil, err
		}
		ns, err := epochNanosecondsFromBigInt(b)
		if err != nil {
			return nil, err
		}
		i, err := NewInstant(ns.hi, ns.lo)
		return i, rustErr(err)
	}
	m["new:Instant"] = func(_ any, arg func(int) any) (any, error) { return fromBigInt(arg(0)) }
	m["static:Instant.fromEpochNanoseconds"] = func(_ any, arg func(int) any) (any, error) { return fromBigInt(arg(0)) }
	m["static:Instant.fromEpochMilliseconds"] = func(_ any, arg func(int) any) (any, error) {
		ms, err := toNumber(arg(0))
		if err != nil {
			return nil, err
		}
		if math.IsNaN(ms) || math.IsInf(ms, 0) || !(ms >= -0x1p63 && ms < 0x1p63) || math.RoundToEven(ms) != ms {
			return nil, rangeErr("Expected finite integer.")
		}
		i, err := InstantFromEpochMilliseconds(int64(ms))
		return i, rustErr(err)
	}
	m["get:Instant.epochMilliseconds"] = func(this any, _ func(int) any) (any, error) {
		return float64(in(this).EpochMilliseconds()), nil
	}
	m["get:Instant.epochNanoseconds"] = func(this any, _ func(int) any) (any, error) {
		return int128ToBigInt(in(this).ns), nil
	}
	m["static:Instant.compare"] = func(_ any, arg func(int) any) (any, error) {
		a, err := h.toInstant(arg(0))
		if err != nil {
			return nil, err
		}
		b, err := h.toInstant(arg(1))
		if err != nil {
			return nil, err
		}
		return float64(a.Compare(b)), nil
	}
	m["method:Instant.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toInstant(arg(0))
		if err != nil {
			return nil, err
		}
		return in(this).Equals(o), nil
	}
	m["method:Instant.round"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.Instant.prototype.round"
		o, err := roundTo(arg(0), true)
		if err != nil {
			return nil, err
		}
		r, err := roundingOptions(o, method, NoUnit)
		if err != nil {
			return nil, err
		}
		out, err := in(this).Round(r)
		return out, rustErr(err)
	}
	m["method:Instant.toZonedDateTimeISO"] = func(this any, arg func(int) any) (any, error) {
		tz, err := h.toTimeZone(arg(0))
		if err != nil {
			return nil, err
		}
		z, err := in(this).ToZonedDateTimeISO(tz)
		return z, rustErr(err)
	}
	m["method:Instant.toString"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.Instant.prototype.toString"
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		opts, err := toStringOptions(o, method, nil)
		if err != nil {
			return nil, err
		}
		tzLike := o.get("timeZone")
		if err := validateUnit(opts.SmallestUnit, groupTime, NoUnit); err != nil {
			return nil, err
		}
		if opts.SmallestUnit == Hour {
			return nil, &jsError{"RangeError", "smallestUnit value is out of range."}
		}
		var tz *TimeZone
		if !isUndefined(tzLike) {
			z, err := h.toTimeZone(tzLike)
			if err != nil {
				return nil, err
			}
			tz = &z
		}
		s, err := in(this).String(h.zones, tz, opts)
		return s, rustErr(err)
	}
	for _, sub := range []bool{false, true} {
		sub := sub
		name := "add"
		if sub {
			name = "subtract"
		}
		m["method:Instant."+name] = func(this any, arg func(int) any) (any, error) {
			dur, err := h.toDuration(arg(0))
			if err != nil {
				return nil, err
			}
			var out Instant
			if sub {
				out, err = in(this).Subtract(dur)
			} else {
				out, err = in(this).Add(dur)
			}
			return out, rustErr(err)
		}
	}
	for _, since := range []bool{false, true} {
		since := since
		name := "until"
		if since {
			name = "since"
		}
		m["method:Instant."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.Instant.prototype." + name
			o, err := h.toInstant(arg(0))
			if err != nil {
				return nil, err
			}
			s, err := differenceSettings(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out Duration
			if since {
				out, err = in(this).Since(o, s)
			} else {
				out, err = in(this).Until(o, s)
			}
			return out, rustErr(err)
		}
	}
}

// ==== ZonedDateTime ====

func (h *harness) zonedOps(m map[string]opFunc) {
	zdt := func(v any) ZonedDateTime { return v.(ZonedDateTime) }
	dateGetters(m, "ZonedDateTime", func(v any) PlainDate { return zdt(v).ToPlainDate() })
	timeGetters(m, "ZonedDateTime", func(v any) ISOTime { return zdt(v).ToPlainTime().ISO() })
	m["get:ZonedDateTime.epochMilliseconds"] = func(this any, _ func(int) any) (any, error) {
		return float64(zdt(this).Instant().EpochMilliseconds()), nil
	}
	m["get:ZonedDateTime.epochNanoseconds"] = func(this any, _ func(int) any) (any, error) {
		return int128ToBigInt(zdt(this).Instant().ns), nil
	}
	m["get:ZonedDateTime.offset"] = func(this any, _ func(int) any) (any, error) { return zdt(this).Offset(), nil }
	m["get:ZonedDateTime.offsetNanoseconds"] = func(this any, _ func(int) any) (any, error) {
		return float64(zdt(this).OffsetNanoseconds()), nil
	}
	m["get:ZonedDateTime.timeZoneId"] = func(this any, _ func(int) any) (any, error) {
		return zdt(this).TimeZone().Identifier(), nil
	}
	m["get:ZonedDateTime.hoursInDay"] = func(this any, _ func(int) any) (any, error) {
		f, err := zdt(this).HoursInDay()
		return f, rustErr(err)
	}
	m["new:ZonedDateTime"] = func(_ any, arg func(int) any) (any, error) {
		b, err := toBigInt(arg(0))
		if err != nil {
			return nil, err
		}
		ns, err := epochNanosecondsFromBigInt(b)
		if err != nil {
			return nil, err
		}
		s, ok := arg(1).(string)
		if !ok {
			return nil, typeErr("Time zone must be string")
		}
		tz, err := h.zones.TimeZoneFromIdentifier([]byte(stdString(s)))
		if err != nil {
			return nil, rustErr(err)
		}
		cal, err := h.calendarArg(arg(2))
		if err != nil {
			return nil, err
		}
		z, err := newZonedDateTime(ns, tz, cal)
		return z, rustErr(err)
	}
	m["static:ZonedDateTime.compare"] = func(_ any, arg func(int) any) (any, error) {
		const method = "Temporal.ZonedDateTime.compare"
		a, err := h.toZonedDateTime(arg(0), jsUndefined{}, method, false)
		if err != nil {
			return nil, err
		}
		b, err := h.toZonedDateTime(arg(1), jsUndefined{}, method, false)
		if err != nil {
			return nil, err
		}
		return float64(a.Compare(b)), nil
	}
	m["method:ZonedDateTime.equals"] = func(this any, arg func(int) any) (any, error) {
		o, err := h.toZonedDateTime(arg(0), jsUndefined{}, "Temporal.ZonedDateTime.prototype.equals", false)
		if err != nil {
			return nil, err
		}
		return zdt(this).Equals(o), nil
	}
	m["method:ZonedDateTime.with"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.ZonedDateTime.prototype.with"
		z := zdt(this)
		if !isPartialTemporalObject(arg(0)) {
			return nil, withNoPartial()
		}
		r, err := h.prepareFields(z.Calendar(), arg(0), fieldsAllDate|fieldsTime|fieldsOffset, requirePartial)
		if err != nil {
			return nil, err
		}
		d, err := disambiguationOption(arg(1), method)
		if err != nil {
			return nil, err
		}
		o, err := offsetOption(arg(1), method, OffsetPrefer)
		if err != nil {
			return nil, err
		}
		ov, err := getOverflow(arg(1), method)
		if err != nil {
			return nil, err
		}
		p, err := r.regulateZoned(ov)
		if err != nil {
			return nil, err
		}
		out, err := z.With(p, d, o, ov)
		return out, rustErr(err)
	}
	m["method:ZonedDateTime.withCalendar"] = func(this any, arg func(int) any) (any, error) {
		c, err := h.toCalendar(arg(0))
		if err != nil {
			return nil, err
		}
		return zdt(this).WithCalendar(c), nil
	}
	m["method:ZonedDateTime.withPlainTime"] = func(this any, arg func(int) any) (any, error) {
		t, err := h.timeOrMidnight(arg(0), "Temporal.ZonedDateTime.prototype.withPlainTime")
		if err != nil {
			return nil, err
		}
		out, err := zdt(this).WithPlainTime(t)
		return out, rustErr(err)
	}
	m["method:ZonedDateTime.withTimeZone"] = func(this any, arg func(int) any) (any, error) {
		tz, err := h.toTimeZone(arg(0))
		if err != nil {
			return nil, err
		}
		out, err := zdt(this).WithTimeZone(tz)
		return out, rustErr(err)
	}
	m["method:ZonedDateTime.toString"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.ZonedDateTime.prototype.toString"
		o, err := getOptionsObject(arg(0))
		if err != nil {
			return nil, err
		}
		showCal, err := showCalendar(o, method)
		if err != nil {
			return nil, err
		}
		var showOffset DisplayOffset
		opts, err := toStringOptions(o, method, func() error {
			i, err := getStringOption(o, "offset", method, []string{"auto", "never"}, 0)
			showOffset = []DisplayOffset{OffsetAuto, OffsetNever}[i]
			return err
		})
		if err != nil {
			return nil, err
		}
		i, err := getStringOption(o, "timeZoneName", method, []string{"auto", "never", "critical"}, 0)
		if err != nil {
			return nil, err
		}
		showZone := []DisplayTimeZone{TimeZoneAuto, TimeZoneNever, TimeZoneCritical}[i]
		if err := validateUnit(opts.SmallestUnit, groupTime, NoUnit); err != nil {
			return nil, err
		}
		if opts.SmallestUnit == Hour {
			return nil, rangeErr("smallestUnit cannot be Hour.")
		}
		s, err := zdt(this).String(showOffset, showZone, showCal, opts)
		return s, rustErr(err)
	}
	m["method:ZonedDateTime.round"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.PlainDateTime.prototype.round"
		o, err := roundTo(arg(0), false)
		if err != nil {
			return nil, err
		}
		r, err := roundingOptions(o, method, Day)
		if err != nil {
			return nil, err
		}
		out, err := zdt(this).Round(r)
		return out, rustErr(err)
	}
	for _, sub := range []bool{false, true} {
		sub := sub
		name := "add"
		if sub {
			name = "subtract"
		}
		m["method:ZonedDateTime."+name] = func(this any, arg func(int) any) (any, error) {
			method := "Temporal.ZonedDateTime.prototype." + name
			dur, err := h.toDuration(arg(0))
			if err != nil {
				return nil, err
			}
			ov, err := getOverflow(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out ZonedDateTime
			if sub {
				out, err = zdt(this).Subtract(dur, ov)
			} else {
				out, err = zdt(this).Add(dur, ov)
			}
			return out, rustErr(err)
		}
	}
	for _, since := range []bool{false, true} {
		since := since
		name := "until"
		if since {
			name = "since"
		}
		m["method:ZonedDateTime."+name] = func(this any, arg func(int) any) (any, error) {
			// V8 names until "since" too.
			const method = "Temporal.ZonedDateTime.prototype.since"
			o, err := h.toZonedDateTime(arg(0), jsUndefined{}, method, false)
			if err != nil {
				return nil, err
			}
			if err := sameCalendar(zdt(this).Calendar(), o.Calendar()); err != nil {
				return nil, err
			}
			s, err := differenceSettings(arg(1), method)
			if err != nil {
				return nil, err
			}
			var out Duration
			if since {
				out, err = zdt(this).Since(o, s)
			} else {
				out, err = zdt(this).Until(o, s)
			}
			return out, rustErr(err)
		}
	}
	m["method:ZonedDateTime.startOfDay"] = func(this any, _ func(int) any) (any, error) {
		out, err := zdt(this).StartOfDay()
		return out, rustErr(err)
	}
	m["method:ZonedDateTime.getTimeZoneTransition"] = func(this any, arg func(int) any) (any, error) {
		const method = "Temporal.ZonedDateTime.prototype.getTimeZoneTransition"
		var o *jsObject
		switch x := arg(0).(type) {
		case jsUndefined:
			return nil, typeErr("Must specify a direction parameter.")
		case string:
			o = newObject("direction", x)
		case *jsObject:
			o = x
		default:
			if !isObject(x) {
				return nil, typeErr("directionParam must be object or string.")
			}
			o = newObject()
		}
		i, err := getStringOption(o, "direction", method, []string{"next", "previous"}, -1)
		if err != nil {
			return nil, err
		}
		out, ok, err := zdt(this).TimeZoneTransition(i == 0)
		if err != nil {
			return nil, rustErr(err)
		}
		if !ok {
			return jsNull{}, nil
		}
		return out, nil
	}
	m["method:ZonedDateTime.toInstant"] = func(this any, _ func(int) any) (any, error) { return zdt(this).Instant(), nil }
	m["method:ZonedDateTime.toPlainDate"] = func(this any, _ func(int) any) (any, error) { return zdt(this).ToPlainDate(), nil }
	m["method:ZonedDateTime.toPlainTime"] = func(this any, _ func(int) any) (any, error) { return zdt(this).ToPlainTime(), nil }
	m["method:ZonedDateTime.toPlainDateTime"] = func(this any, _ func(int) any) (any, error) {
		return zdt(this).ToPlainDateTime(), nil
	}
}
