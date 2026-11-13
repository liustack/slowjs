//go:build gointl_all

package temporal

import (
	"math"
	"strings"
	"unicode/utf8"
)

// V8's reading of property bags, as js-temporal-objects.cc has it.

type fieldFlags int

const (
	fieldsDay fieldFlags = 1 << iota
	fieldsMonth
	fieldsYear
	fieldsTime
	fieldsOffset
	fieldsTimeZone
	fieldsAllDate = fieldsDay | fieldsMonth | fieldsYear
)

type required int

const (
	requireNone required = iota
	requirePartial
	requireTimeZone
)

// combinedRecord is V8's CombinedRecord.
type combinedRecord struct {
	year, month, day, eraYear                                  *float64
	monthCode, era                                             *string
	hour, minute, second, millisecond, microsecond, nanosecond *float64
	offset                                                     *string
	timeZone                                                   *TimeZone
	cal                                                        *Calendar
}

// stdString is V8's String::ToStdString: UTF-8.
func stdString(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return strings.ToValidUTF8(s, "�")
}

// toMonthCode is V8's ToMonthCode.
func toMonthCode(v any) (string, error) {
	if isObject(v) {
		v = "[object Object]"
	}
	s, ok := v.(string)
	if !ok {
		return "", typeErr("Month code out of range.")
	}
	mc := stdString(s)
	switch {
	case len(mc) != 3 && len(mc) != 4, mc[0] != 'M', mc[1] < '0' || mc[1] > '9', mc[2] < '0' || mc[2] > '9',
		len(mc) == 4 && mc[3] != 'L', mc[1] == '0' && mc[2] == '0' && len(mc) != 4:
		return "", rangeErr("Month code out of range.")
	}
	return mc, nil
}

// toOffsetString is V8's ToOffsetString.
func toOffsetString(v any) (string, error) {
	if isObject(v) {
		v = "[object Object]"
	}
	s, ok := v.(string)
	if !ok {
		return "", typeErr("Offset must be string.")
	}
	s = stdString(s)
	if _, err := ParseOffset([]byte(s)); err != nil {
		return "", rustErr(err)
	}
	return s, nil
}

// toTimeZone is ToTemporalTimeZoneIdentifier.
func (h *harness) toTimeZone(v any) (TimeZone, error) {
	if z, ok := v.(ZonedDateTime); ok {
		return z.TimeZone(), nil
	}
	s, ok := v.(string)
	if !ok {
		return TimeZone{}, typeErr("Time zone must be string or ZonedDateTime object.")
	}
	tz, err := h.zones.TimeZoneFromString([]byte(stdString(s)))
	return tz, rustErr(err)
}

// calendarOf is the calendar of a calendared Temporal object.
func calendarOf(v any) (*Calendar, bool) {
	switch x := v.(type) {
	case PlainDate:
		return x.Calendar(), true
	case PlainDateTime:
		return x.Calendar(), true
	case PlainMonthDay:
		return x.Calendar(), true
	case PlainYearMonth:
		return x.Calendar(), true
	case ZonedDateTime:
		return x.Calendar(), true
	}
	return nil, false
}

// toCalendar is ToTemporalCalendarIdentifier.
func (h *harness) toCalendar(v any) (*Calendar, error) {
	if c, ok := calendarOf(v); ok {
		return c, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, typeErr("Calendar must be string or calendared Temporal object.")
	}
	id, ok := ParseCalendarString([]byte(stdString(s)))
	if !ok {
		return nil, rangeErr("Invalid calendar string")
	}
	return h.cal(id), nil
}

// canonicalizeCalendar is V8's CanonicalizeCalendar.
func (h *harness) canonicalizeCalendar(s string) (*Calendar, error) {
	id, ok := CalendarID(strings.ToLower(stdString(s)))
	if !ok {
		return nil, &jsError{"RangeError", "Temporal error: Unknown calendar type " + s + "."}
	}
	return h.cal(id), nil
}

// calendarWithISODefault is GetTemporalCalendarIdentifierWithISODefault.
func (h *harness) calendarWithISODefault(v any) (*Calendar, error) {
	if c, ok := calendarOf(v); ok {
		return c, nil
	}
	c := get(v, "calendar")
	if isUndefined(c) {
		return ISOCalendar(), nil
	}
	return h.toCalendar(c)
}

// prepareFields is PrepareCalendarFields.
func (h *harness) prepareFields(cal *Calendar, v any, which fieldFlags, req required) (*combinedRecord, error) {
	r := &combinedRecord{cal: cal}
	eras := cal.hasEras()
	found := false
	num := func(key string, conv func(any) (float64, error), out **float64) error {
		x := get(v, key)
		if isUndefined(x) {
			return nil
		}
		found = true
		f, err := conv(x)
		if err != nil {
			return err
		}
		*out = &f
		return nil
	}
	steps := []func() error{
		func() error {
			if which&fieldsDay != 0 {
				return num("day", toPositiveIntegerWithTruncation, &r.day)
			}
			return nil
		},
		func() error {
			if which&fieldsYear != 0 && eras {
				x := get(v, "era")
				if !isUndefined(x) {
					found = true
					s, err := toString(x)
					if err != nil {
						return err
					}
					s = stdString(s)
					r.era = &s
				}
			}
			return nil
		},
		func() error {
			if which&fieldsYear != 0 && eras {
				return num("eraYear", toIntegerWithTruncation, &r.eraYear)
			}
			return nil
		},
		func() error {
			if which&fieldsTime != 0 {
				return num("hour", toIntegerWithTruncation, &r.hour)
			}
			return nil
		},
		func() error {
			if which&fieldsTime != 0 {
				return num("microsecond", toIntegerWithTruncation, &r.microsecond)
			}
			return nil
		},
		func() error {
			if which&fieldsTime != 0 {
				return num("millisecond", toIntegerWithTruncation, &r.millisecond)
			}
			return nil
		},
		func() error {
			if which&fieldsTime != 0 {
				return num("minute", toIntegerWithTruncation, &r.minute)
			}
			return nil
		},
		func() error {
			if which&fieldsMonth != 0 {
				return num("month", toPositiveIntegerWithTruncation, &r.month)
			}
			return nil
		},
		func() error {
			if which&fieldsMonth != 0 {
				x := get(v, "monthCode")
				if !isUndefined(x) {
					found = true
					mc, err := toMonthCode(x)
					if err != nil {
						return err
					}
					r.monthCode = &mc
				}
			}
			return nil
		},
		func() error {
			if which&fieldsTime != 0 {
				return num("nanosecond", toIntegerWithTruncation, &r.nanosecond)
			}
			return nil
		},
		func() error {
			if which&fieldsOffset != 0 {
				x := get(v, "offset")
				if !isUndefined(x) {
					found = true
					o, err := toOffsetString(x)
					if err != nil {
						return err
					}
					r.offset = &o
				}
			}
			return nil
		},
		func() error {
			if which&fieldsTime != 0 {
				return num("second", toIntegerWithTruncation, &r.second)
			}
			return nil
		},
		func() error {
			if which&fieldsTimeZone != 0 {
				x := get(v, "timeZone")
				if !isUndefined(x) {
					found = true
					tz, err := h.toTimeZone(x)
					if err != nil {
						return err
					}
					r.timeZone = &tz
				} else if req == requireTimeZone {
					return typeErr("Must specify time zone.")
				}
			}
			return nil
		},
		func() error {
			if which&fieldsYear != 0 {
				return num("year", toIntegerWithTruncation, &r.year)
			}
			return nil
		},
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return nil, err
		}
	}
	if req == requirePartial && !found {
		return nil, typeErr("Must specify at least one calendar field.")
	}
	return r, nil
}

// clampInt is ClampIntegralDoubleToRange: the value within the type.
func clampInt(f, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, f)) }

// regulateDate is DateRecord::Regulate, and temporal_capi's conversion
// of the result.
func (r *combinedRecord) regulateDate(overflow Overflow) (CalendarFields, error) {
	var f CalendarFields
	if overflow == Constrain {
		if r.year != nil {
			f.Year = Int(int(int32(clampInt(*r.year, math.MinInt32, math.MaxInt32))))
		}
		// V8 clamps the month and day to an int8, and hands them to Rust
		// as a u8.
		if r.month != nil {
			f.Month = Int(int(uint8(int8(clampInt(*r.month, -128, 127)))))
		}
		if r.day != nil {
			f.Day = Int(int(uint8(int8(clampInt(*r.day, -128, 127)))))
		}
		if r.eraYear != nil {
			f.EraYear = Int(int(int32(clampInt(*r.eraYear, math.MinInt32, math.MaxInt32))))
		}
	} else {
		check := func(v *float64, lo, hi float64) (*int, error) {
			if v == nil {
				return nil, nil
			}
			if !inRange(*v, lo, hi) {
				return nil, rangeErr("Integer out of range.")
			}
			return Int(int(*v)), nil
		}
		var err error
		if f.Year, err = check(r.year, math.MinInt32, math.MaxInt32); err != nil {
			return f, err
		}
		if f.Month, err = check(r.month, 0, 255); err != nil {
			return f, err
		}
		if f.Day, err = check(r.day, 0, 255); err != nil {
			return f, err
		}
		if f.EraYear, err = check(r.eraYear, math.MinInt32, math.MaxInt32); err != nil {
			return f, err
		}
	}
	if r.monthCode != nil && *r.monthCode != "" {
		if err := ParseMonthCode(*r.monthCode); err != nil {
			return f, rustErr(err)
		}
		f.MonthCode = String(*r.monthCode)
	}
	if r.era != nil && *r.era != "" {
		if len(*r.era) > 19 || !isASCII(*r.era) {
			return f, rangeErr("Invalid era code.")
		}
		f.Era = String(*r.era)
	}
	return f, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// regulateTime is TimeRecord::Regulate.
func regulateTime(t [6]*float64, overflow Overflow) (PartialTime, error) {
	var p PartialTime
	out := [6]**int{&p.Hour, &p.Minute, &p.Second, &p.Millisecond, &p.Microsecond, &p.Nanosecond}
	maxes := [6]float64{23, 59, 59, 999, 999, 999}
	if overflow == Constrain {
		for i, v := range t {
			if v != nil {
				*out[i] = Int(int(clampInt(*v, 0, maxes[i])))
			}
		}
		return p, nil
	}
	for i, v := range t {
		x := 0.0
		if v != nil {
			x = *v
		}
		if x < 0 || x > maxes[i] {
			return p, rangeErr("Invalid time provided")
		}
	}
	for i, v := range t {
		if v != nil {
			*out[i] = Int(int(*v))
		}
	}
	return p, nil
}

func (r *combinedRecord) timeFields() [6]*float64 {
	return [6]*float64{r.hour, r.minute, r.second, r.millisecond, r.microsecond, r.nanosecond}
}

// regulateZoned is CombinedRecord::Regulate for a PartialZonedDateTime, and
// temporal_capi's conversion of it.
func (r *combinedRecord) regulateZoned(overflow Overflow) (PartialZonedDateTime, error) {
	var p PartialZonedDateTime
	var err error
	if p.Date, err = r.regulateDate(overflow); err != nil {
		return p, err
	}
	if p.Time, err = regulateTime(r.timeFields(), overflow); err != nil {
		return p, err
	}
	if r.offset != nil {
		ns, err := ParseOffset([]byte(*r.offset))
		if err != nil {
			return p, rustErr(err)
		}
		p.Offset = &ns
	}
	return p, nil
}

// plainDateFromFields is PlainDate::from_partial.
func (h *harness) plainDateFromFields(f CalendarFields, cal *Calendar, overflow Overflow, _ bool) (PlainDate, error) {
	d, err := PlainDateFromFields(f, cal, overflow)
	return d, rustErr(err)
}

// plainDateFromTemporal is PlainDate::from_partial of the partial date V8
// takes from a Temporal object: its year, month and day in its calendar.
func (h *harness) plainDateFromTemporal(d PlainDate, overflow *Overflow) (PlainDate, error) {
	o := Constrain
	if overflow != nil {
		o = *overflow
	}
	return h.plainDateFromFields(CalendarFields{Year: Int(d.Year()), Month: Int(d.Month()), Day: Int(d.Day())}, d.Calendar(), o, false)
}

// ==== ToTemporalX ====

func (h *harness) readDiscardOverflow(opts any, method string) error {
	_, err := getOverflow(opts, method)
	return err
}

func (h *harness) toPlainTime(v any, opts any, method string) (PlainTime, error) {
	switch x := v.(type) {
	case PlainTime:
		return x, h.readDiscardOverflow(opts, method)
	case PlainDateTime:
		if _, err := getOverflow(opts, method); err != nil {
			return PlainTime{}, err
		}
		return x.ToPlainTime(), nil
	case ZonedDateTime:
		if _, err := getOverflow(opts, method); err != nil {
			return PlainTime{}, err
		}
		return x.ToPlainTime(), nil
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return PlainTime{}, err
		}
		t, err := ParsePlainTime(b)
		if err != nil {
			return PlainTime{}, rustErr(err)
		}
		return t, h.readDiscardOverflow(opts, method)
	}
	if !isObject(v) {
		return PlainTime{}, typeErr("Time-like argument must be object or string")
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
			return PlainTime{}, err
		}
		found = true
		t[[6]int{0, 4, 3, 1, 5, 2}[i]] = &f
	}
	if !found {
		return PlainTime{}, typeErr("Must specify at least one time field.")
	}
	zero := 0.0
	for i := range t {
		if t[i] == nil {
			t[i] = &zero
		}
	}
	overflow, err := getOverflow(opts, method)
	if err != nil {
		return PlainTime{}, err
	}
	p, err := regulateTime(t, overflow)
	if err != nil {
		return PlainTime{}, err
	}
	pt, err := PlainTimeFromPartial(p, overflow)
	return pt, rustErr(err)
}

func (h *harness) toPlainDate(v any, opts any, method string) (PlainDate, error) {
	switch x := v.(type) {
	case PlainDate:
		return x, h.readDiscardOverflow(opts, method)
	case ZonedDateTime, PlainDateTime:
		var d PlainDate
		if z, ok := x.(ZonedDateTime); ok {
			d = z.ToPlainDate()
		} else {
			d = x.(PlainDateTime).ToPlainDate()
		}
		if err := h.readDiscardOverflow(opts, method); err != nil {
			return PlainDate{}, err
		}
		return h.plainDateFromTemporal(d, nil)
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return PlainDate{}, err
		}
		p, err := ParseDate(b)
		if err != nil {
			return PlainDate{}, rustErr(err)
		}
		if err := h.readDiscardOverflow(opts, method); err != nil {
			return PlainDate{}, err
		}
		d, err := PlainDateFromParsed(p, h.cal(p.Calendar()))
		return d, rustErr(err)
	}
	if !isObject(v) {
		return PlainDate{}, typeErr("Date argument must be object or string.")
	}
	cal, err := h.calendarWithISODefault(v)
	if err != nil {
		return PlainDate{}, err
	}
	r, err := h.prepareFields(cal, v, fieldsAllDate, requireNone)
	if err != nil {
		return PlainDate{}, err
	}
	overflow, err := getOverflow(opts, method)
	if err != nil {
		return PlainDate{}, err
	}
	f, err := r.regulateDate(overflow)
	if err != nil {
		return PlainDate{}, err
	}
	return h.plainDateFromFields(f, cal, overflow, true)
}

func (h *harness) toPlainDateTime(v any, opts any, method string) (PlainDateTime, error) {
	switch x := v.(type) {
	case PlainDateTime:
		return x, h.readDiscardOverflow(opts, method)
	case ZonedDateTime, PlainDate:
		var d PlainDate
		var t PartialTime
		if z, ok := x.(ZonedDateTime); ok {
			d = z.ToPlainDate()
			it := z.ToPlainTime().ISO()
			t = PartialTime{Int(it.Hour), Int(it.Minute), Int(it.Second), Int(it.Millisecond), Int(it.Microsecond), Int(it.Nanosecond)}
		} else {
			d = x.(PlainDate)
		}
		overflow, err := getOverflow(opts, method)
		if err != nil {
			return PlainDateTime{}, err
		}
		dt, err := PlainDateTimeFromFields(CalendarFields{Year: Int(d.Year()), Month: Int(d.Month()), Day: Int(d.Day())},
			t, d.Calendar(), overflow)
		return dt, rustErr(err)
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return PlainDateTime{}, err
		}
		p, err := ParseDateTime(b)
		if err != nil {
			return PlainDateTime{}, rustErr(err)
		}
		if err := h.readDiscardOverflow(opts, method); err != nil {
			return PlainDateTime{}, err
		}
		dt, err := PlainDateTimeFromParsed(p, h.cal(p.Calendar()))
		return dt, rustErr(err)
	}
	if !isObject(v) {
		return PlainDateTime{}, typeErr("DateTime argument must be object or string.")
	}
	cal, err := h.calendarWithISODefault(v)
	if err != nil {
		return PlainDateTime{}, err
	}
	r, err := h.prepareFields(cal, v, fieldsAllDate|fieldsTime, requireNone)
	if err != nil {
		return PlainDateTime{}, err
	}
	overflow, err := getOverflow(opts, method)
	if err != nil {
		return PlainDateTime{}, err
	}
	f, err := r.regulateDate(overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	t, err := regulateTime(r.timeFields(), overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	dt, err := PlainDateTimeFromFields(f, t, cal, overflow)
	return dt, rustErr(err)
}

func (h *harness) toPlainYearMonth(v any, opts any, method string) (PlainYearMonth, error) {
	switch x := v.(type) {
	case PlainYearMonth:
		return x, h.readDiscardOverflow(opts, method)
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return PlainYearMonth{}, err
		}
		p, err := ParseYearMonth(b)
		if err != nil {
			return PlainYearMonth{}, rustErr(err)
		}
		if err := h.readDiscardOverflow(opts, method); err != nil {
			return PlainYearMonth{}, err
		}
		ym, err := PlainYearMonthFromParsed(p, h.cal(p.Calendar()))
		return ym, rustErr(err)
	}
	if !isObject(v) {
		return PlainYearMonth{}, typeErr("YearMonth argument must be object or string.")
	}
	cal, err := h.calendarWithISODefault(v)
	if err != nil {
		return PlainYearMonth{}, err
	}
	r, err := h.prepareFields(cal, v, fieldsYear|fieldsMonth, requireNone)
	if err != nil {
		return PlainYearMonth{}, err
	}
	overflow, err := getOverflow(opts, method)
	if err != nil {
		return PlainYearMonth{}, err
	}
	f, err := r.regulateDate(overflow)
	if err != nil {
		return PlainYearMonth{}, err
	}
	ym, err := PlainYearMonthFromFields(f, cal, overflow)
	return ym, rustErr(err)
}

func (h *harness) toPlainMonthDay(v any, opts any, method string) (PlainMonthDay, error) {
	switch x := v.(type) {
	case PlainMonthDay:
		return x, h.readDiscardOverflow(opts, method)
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return PlainMonthDay{}, err
		}
		p, err := ParseMonthDay(b)
		if err != nil {
			return PlainMonthDay{}, rustErr(err)
		}
		if err := h.readDiscardOverflow(opts, method); err != nil {
			return PlainMonthDay{}, err
		}
		md, err := PlainMonthDayFromParsed(p, h.cal(p.Calendar()))
		return md, rustErr(err)
	}
	if !isObject(v) {
		return PlainMonthDay{}, typeErr("MonthDay argument must be object or string.")
	}
	cal, err := h.calendarWithISODefault(v)
	if err != nil {
		return PlainMonthDay{}, err
	}
	r, err := h.prepareFields(cal, v, fieldsYear|fieldsMonth|fieldsDay, requireNone)
	if err != nil {
		return PlainMonthDay{}, err
	}
	overflow, err := getOverflow(opts, method)
	if err != nil {
		return PlainMonthDay{}, err
	}
	f, err := r.regulateDate(overflow)
	if err != nil {
		return PlainMonthDay{}, err
	}
	md, err := PlainMonthDayFromFields(f, cal, overflow)
	return md, rustErr(err)
}

func (h *harness) toInstant(v any) (Instant, error) {
	switch x := v.(type) {
	case Instant:
		return x, nil
	case ZonedDateTime:
		return x.Instant(), nil
	case *jsObject:
		v = "[object Object]"
	}
	s, ok := v.(string)
	if !ok {
		return Instant{}, typeErr("Instant argument must be Instant or string.")
	}
	b, err := jsBytes(s)
	if err != nil {
		return Instant{}, err
	}
	i, err := ParseInstant(b)
	return i, rustErr(err)
}

// zdtOptions is GetZDTOptions.
func zdtOptions(opts any, method string, present bool) (Disambiguation, OffsetDisambiguation, Overflow, error) {
	if !present || isUndefined(opts) {
		return Compatible, OffsetReject, Constrain, nil
	}
	o, ok := opts.(*jsObject)
	if !ok {
		if !isObject(opts) {
			return 0, 0, 0, typeErrWithArg("Option must be object:", "disambiguation")
		}
		o = newObject()
	}
	d, err := getStringOption(o, "disambiguation", method, []string{"compatible", "earlier", "later", "reject"}, 0)
	if err != nil {
		return 0, 0, 0, err
	}
	od, err := getStringOption(o, "offset", method, []string{"prefer", "use", "ignore", "reject"}, 3)
	if err != nil {
		return 0, 0, 0, err
	}
	ov, err := getStringOption(o, "overflow", method, []string{"constrain", "reject"}, 0)
	if err != nil {
		return 0, 0, 0, err
	}
	offsets := []OffsetDisambiguation{OffsetPrefer, OffsetUse, OffsetIgnore, OffsetReject}
	return Disambiguation(d), offsets[od], Overflow(ov), nil
}

func (h *harness) toZonedDateTime(v any, opts any, method string, present bool) (ZonedDateTime, error) {
	switch x := v.(type) {
	case ZonedDateTime:
		if _, _, _, err := zdtOptions(opts, method, present); err != nil {
			return ZonedDateTime{}, err
		}
		return x, nil
	case string:
		b, err := jsBytes(x)
		if err != nil {
			return ZonedDateTime{}, err
		}
		p, err := h.zones.ParseZonedDateTime(b)
		if err != nil {
			return ZonedDateTime{}, rustErr(err)
		}
		d, o, _, err := zdtOptions(opts, method, present)
		if err != nil {
			return ZonedDateTime{}, err
		}
		z, err := ZonedDateTimeFromParsed(p, h.cal(p.Calendar()), d, o)
		return z, rustErr(err)
	}
	if !isObject(v) {
		return ZonedDateTime{}, typeErr("ZonedDateTime argument must be object or string.")
	}
	cal, err := h.calendarWithISODefault(v)
	if err != nil {
		return ZonedDateTime{}, err
	}
	r, err := h.prepareFields(cal, v, fieldsAllDate|fieldsTime|fieldsOffset|fieldsTimeZone, requireTimeZone)
	if err != nil {
		return ZonedDateTime{}, err
	}
	d, o, ov, err := zdtOptions(opts, method, present)
	if err != nil {
		return ZonedDateTime{}, err
	}
	p, err := r.regulateZoned(ov)
	if err != nil {
		return ZonedDateTime{}, err
	}
	z, err := h.zones.ZonedDateTimeFromFields(p, r.timeZone, cal, ov, d, o)
	return z, rustErr(err)
}

func (h *harness) fromOps(m map[string]opFunc) {
	m["static:PlainTime.from"] = func(_ any, arg func(int) any) (any, error) {
		return h.toPlainTime(arg(0), arg(1), "Temporal.PlainTime.from")
	}
	m["static:PlainDate.from"] = func(_ any, arg func(int) any) (any, error) {
		return h.toPlainDate(arg(0), arg(1), "Temporal.PlainDate.from")
	}
	m["static:PlainDateTime.from"] = func(_ any, arg func(int) any) (any, error) {
		return h.toPlainDateTime(arg(0), arg(1), "Temporal.PlainDateTime.from")
	}
	m["static:PlainYearMonth.from"] = func(_ any, arg func(int) any) (any, error) {
		return h.toPlainYearMonth(arg(0), arg(1), "Temporal.PlainYearMonth.from")
	}
	m["static:PlainMonthDay.from"] = func(_ any, arg func(int) any) (any, error) {
		return h.toPlainMonthDay(arg(0), arg(1), "Temporal.PlainMonthDay.from")
	}
	m["static:Instant.from"] = func(_ any, arg func(int) any) (any, error) { return h.toInstant(arg(0)) }
	m["static:ZonedDateTime.from"] = func(_ any, arg func(int) any) (any, error) {
		return h.toZonedDateTime(arg(0), arg(1), "Temporal.ZonedDateTime.from", true)
	}
}
