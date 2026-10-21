package temporal

import (
	"math"
)

// Durations, as temporal_rs 0.2.3 has them with the
// float64_representable_durations feature Node builds it with.

// A timeDuration is TimeDuration: a duration's time part in nanoseconds,
// at most maxTimeDuration either way.
type timeDuration struct{ int128 }

// maxTimeDuration is 2^53 seconds less a nanosecond.
var maxTimeDuration = i128(1<<53 - 1).mul64(1_000_000_000).add(i128(999_999_999))

func (d timeDuration) inRange() bool { return d.abs().cmp(maxTimeDuration) <= 0 }

// timeDurationFromComponents is TimeDurationFromComponents.
func timeDurationFromComponents(hours, minutes, seconds, milliseconds int64, microseconds, nanoseconds int128) timeDuration {
	t := i128(hours).mul64(3_600_000_000_000)
	t = t.add(i128(minutes).mul64(60_000_000_000))
	t = t.add(i128(seconds).mul64(1_000_000_000))
	t = t.add(i128(milliseconds).mul64(1_000_000))
	t = t.add(microseconds.mul64(1000))
	return timeDuration{t.add(nanoseconds)}
}

// timeDurationFromDifference is TimeDurationFromEpochNanosecondsDifference.
func timeDurationFromDifference(one, two int128) (timeDuration, error) {
	d := timeDuration{one.sub(two)}
	if !d.inRange() {
		return timeDuration{}, rangeError("TimeDuration exceeds maxTimeDuration.")
	}
	return d, nil
}

// addDays is Add24HourDaysToTimeDuration.
func (d timeDuration) addDays(days int64) (timeDuration, error) {
	r := timeDuration{d.add(i128(days).mul64(nsPerDay))}
	if !r.inRange() {
		return timeDuration{}, rangeError("SubtractTimeDuration exceeded a valid Duration range.")
	}
	return r, nil
}

// plus is AddTimeDuration.
func (d timeDuration) plus(o timeDuration) (timeDuration, error) {
	r := timeDuration{d.add(o.int128)}
	if !r.inRange() {
		return timeDuration{}, rangeError("TimeDuration exceeds maxTimeDuration.")
	}
	return r, nil
}

func (d timeDuration) minus(o timeDuration) (timeDuration, error) {
	r := timeDuration{d.sub(o.int128)}
	if !r.inRange() {
		return timeDuration{}, rangeError("SubtractTimeDuration exceeded a valid TimeDuration range.")
	}
	return r, nil
}

// seconds is TimeDurationSeconds, truncated toward zero.
func (d timeDuration) seconds() int64 { return d.quo(i128(1_000_000_000)).int64() }

// subseconds is TimeDurationSubseconds, the remainder with the sign.
func (d timeDuration) subseconds() int32 { return int32(d.rem(i128(1_000_000_000)).int64()) }

// round is RoundTimeDuration.
func (d timeDuration) round(o resolvedRounding) (timeDuration, error) {
	inc := i128(int64(o.increment.get())).mul64(o.smallest.nanoseconds())
	return d.roundInner(inc, o.mode)
}

// roundInner is RoundTimeDurationToIncrement.
func (d timeDuration) roundInner(increment int128, mode RoundingMode) (timeDuration, error) {
	r := timeDuration{roundIncrement(d.int128, increment, mode)}
	if !r.inRange() {
		return timeDuration{}, rangeError("TimeDuration exceeds maxTimeDuration.")
	}
	return r, nil
}

// roundToFractionalDays is the day rounding of Duration's round.
func (d timeDuration) roundToFractionalDays(increment RoundingIncrement, mode RoundingMode) int64 {
	inc := mulSaturating(i128(int64(increment.get())), i128(nsPerDay))
	return roundIncrement(d.int128, inc, mode).quo(i128(nsPerDay)).int64()
}

// total is TotalTimeDuration.
func (d timeDuration) total(u Unit) float64 {
	return fractionToFloat64(d.int128, float64(u.nanoseconds()))
}

// fractionToFloat64 is temporal_rs's Fraction::to_finite_f64: the ratio of
// an integer to a double, rounded once, as JavaScriptCore's
// fractionToDouble computes it.
func fractionToFloat64(numerator int128, denominator float64) float64 {
	if denominator == 1 {
		return numerator.float64()
	}
	if numerator.abs().cmp(i128(1<<53-1)) < 0 {
		return numerator.float64() / denominator
	}
	hi := numerator.float64()
	lo := numerator.sub(int128FromFloat(hi)).float64()
	q0 := hi / denominator
	// The product is rounded, and its error found by the one fused
	// multiply-add asked for; the conversion keeps an arm64 build from
	// fusing the product into the sums below as well, which Rust never does.
	product := float64(q0 * denominator)
	productLo := math.FMA(q0, denominator, -product)
	sum := hi - product
	one, two := hi, -product
	calcOne := sum - one
	calcTwo := sum - two
	sumLo := (one - calcTwo) + (two - calcOne)
	errTerm := sumLo + lo - productLo
	q1 := (sum + errTerm) / denominator
	return q0 + q1
}

// A Duration is Temporal.Duration's value: ten fields of one sign.
type Duration struct {
	sign                                        int
	years, months, weeks                        uint32
	days, hours, minutes, seconds, milliseconds uint64
	microseconds, nanoseconds                   int128 // magnitudes
}

// NewDuration is Duration::new: the fields, all of one sign and within
// Temporal's limits. Microseconds and nanoseconds past int64 come from
// DurationFromNumbers or ParseDuration.
func NewDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds int64) (Duration, error) {
	return newDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds, i128(microseconds), i128(nanoseconds))
}

// newDuration is NewDuration with microseconds and nanoseconds as i128.
func newDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds int64, microseconds, nanoseconds int128) (Duration, error) {
	if !validDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds) {
		return Duration{}, rangeError("Duration was not valid.")
	}
	s := durationSign(years, months, weeks, days, hours, minutes, seconds, milliseconds,
		int64(microseconds.sign()), int64(nanoseconds.sign()))
	return Duration{
		sign:   s,
		years:  uint32(saturatingAbs(years)),
		months: uint32(saturatingAbs(months)),
		weeks:  uint32(saturatingAbs(weeks)),
		days:   unsignedAbs(days), hours: unsignedAbs(hours), minutes: unsignedAbs(minutes),
		seconds: unsignedAbs(seconds),
		// float64_representable_durations: the smaller units are kept as a
		// double holds them.
		milliseconds: uint64FromFloat(float64(unsignedAbs(milliseconds))),
		microseconds: int128FromFloat(microseconds.abs().float64()),
		nanoseconds:  int128FromFloat(nanoseconds.abs().float64()),
	}, nil
}

func saturatingAbs(v int64) int64 {
	if v == math.MinInt64 {
		return math.MaxInt64
	}
	if v < 0 {
		return -v
	}
	return v
}

func unsignedAbs(v int64) uint64 {
	if v < 0 {
		return uint64(-v)
	}
	return uint64(v)
}

// durationSign is DurationSign over the fields, microseconds and
// nanoseconds by their sign.
func durationSign(fields ...int64) int {
	for _, v := range fields {
		if v < 0 {
			return -1
		}
		if v > 0 {
			return 1
		}
	}
	return 0
}

// validDuration is IsValidDuration, as temporal_rs checks it: the seconds
// and smaller units read as doubles first.
func validDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds int64, microseconds, nanoseconds int128) bool {
	set := [...]int64{years, months, weeks, days, hours, minutes, seconds, milliseconds,
		int64(microseconds.sign()), int64(nanoseconds.sign())}
	s := durationSign(set[:]...)
	for _, v := range set {
		if v < 0 && s > 0 || v > 0 && s < 0 {
			return false
		}
	}
	if saturatingAbs(years) > math.MaxUint32 || saturatingAbs(months) > math.MaxUint32 ||
		saturatingAbs(weeks) > math.MaxUint32 {
		return false
	}
	seconds = int64FromFloat(float64(seconds))
	milliseconds = int64FromFloat(float64(milliseconds))
	microseconds = int128FromFloat(microseconds.float64())
	nanoseconds = int128FromFloat(nanoseconds.float64())
	whole := i128(days).mul64(nsPerDay).add(i128(hours).mul64(3_600_000_000_000)).
		add(i128(minutes).mul64(60_000_000_000)).add(i128(seconds).mul64(1_000_000_000))
	sub := addSaturating(addSaturating(mulSaturating(i128(milliseconds), i128(1_000_000)),
		mulSaturating(microseconds, i128(1000))), nanoseconds)
	total := addSaturating(whole, sub)
	limit := i128(1 << 53).mul64(1_000_000_000)
	return saturatingAbs128(total).cmp(limit) < 0
}

func addSaturating(a, b int128) int128 {
	r := a.add(b)
	if a.isNeg() == b.isNeg() && r.isNeg() != a.isNeg() {
		if a.isNeg() {
			return int128{hi: math.MinInt64}
		}
		return int128{hi: math.MaxInt64, lo: math.MaxUint64}
	}
	return r
}

func saturatingAbs128(a int128) int128 {
	if a == (int128{hi: math.MinInt64}) {
		return int128{hi: math.MaxInt64, lo: math.MaxUint64}
	}
	return a.abs()
}

// multiplier is Sign::as_sign_multiplier: 1 for a zero duration.
func (d Duration) multiplier() int64 {
	if d.sign < 0 {
		return -1
	}
	return 1
}

// The fields, signed.
func (d Duration) Years() int64        { return int64(d.years) * d.multiplier() }
func (d Duration) Months() int64       { return int64(d.months) * d.multiplier() }
func (d Duration) Weeks() int64        { return int64(d.weeks) * d.multiplier() }
func (d Duration) Days() int64         { return int64(d.days) * d.multiplier() }
func (d Duration) Hours() int64        { return int64(d.hours) * d.multiplier() }
func (d Duration) Minutes() int64      { return int64(d.minutes) * d.multiplier() }
func (d Duration) Seconds() int64      { return int64(d.seconds) * d.multiplier() }
func (d Duration) Milliseconds() int64 { return int64(d.milliseconds) * d.multiplier() }

func (d Duration) microsecondsSigned() int128 { return d.microseconds.mul64(d.multiplier()) }
func (d Duration) nanosecondsSigned() int128  { return d.nanoseconds.mul64(d.multiplier()) }

// Microseconds and Nanoseconds are the fields as JavaScript reads them,
// doubles.
func (d Duration) Microseconds() float64 { return d.microsecondsSigned().float64() }
func (d Duration) Nanoseconds() float64  { return d.nanosecondsSigned().float64() }

// Sign is the duration's sign, -1, 0 or 1.
func (d Duration) Sign() int { return d.sign }

// IsZero is whether the duration is blank.
func (d Duration) IsZero() bool { return d.sign == 0 }

// Negated is the duration with its sign turned.
func (d Duration) Negated() Duration {
	d.sign = -d.sign
	return d
}

// Abs is the duration without its sign.
func (d Duration) Abs() Duration {
	if d.sign != 0 {
		d.sign = 1
	}
	return d
}

// IsTimeWithinRange is whether each time field is within its unit's range.
func (d Duration) IsTimeWithinRange() bool {
	return d.hours < 24 && d.minutes < 60 && d.seconds < 60 && d.milliseconds < 1000 &&
		d.microseconds.cmp(i128(1000)) < 0 && d.nanoseconds.cmp(i128(1000)) < 0
}

// date is the duration's date part.
func (d Duration) date() DateDuration {
	return DateDuration{d.Years(), d.Months(), d.Weeks(), d.Days()}
}

// timePart is TimeDuration::from_duration.
func (d Duration) timePart() timeDuration {
	m := d.multiplier()
	t := int128{lo: d.hours}.mul64(3_600_000_000_000).mul64(m)
	t = t.add(int128{lo: d.minutes}.mul64(60_000_000_000).mul64(m))
	t = t.add(int128{lo: d.seconds}.mul64(1_000_000_000).mul64(m))
	t = t.add(int128{lo: d.milliseconds}.mul64(1_000_000).mul64(m))
	t = t.add(d.microseconds.mul64(1000).mul64(m))
	t = t.add(d.nanoseconds.mul64(m))
	return timeDuration{t}
}

// defaultLargestUnit is DefaultTemporalLargestUnit.
func (d Duration) defaultLargestUnit() Unit {
	fields := [...]bool{d.years != 0, d.months != 0, d.weeks != 0, d.days != 0, d.hours != 0,
		d.minutes != 0, d.seconds != 0, d.milliseconds != 0, !d.microseconds.isZero(), !d.nanoseconds.isZero()}
	for i, nonzero := range fields {
		if nonzero {
			return unitTable[i]
		}
	}
	return Nanosecond
}

// internal is ToInternalDurationRecord.
func (d Duration) internal() internalDuration {
	return internalDuration{date: d.date(), time: timeDurationFromComponents(d.Hours(), d.Minutes(),
		d.Seconds(), d.Milliseconds(), d.microsecondsSigned(), d.nanosecondsSigned())}
}

// internalWith24HourDays is ToInternalDurationRecordWith24HourDays.
func (d Duration) internalWith24HourDays() (internalDuration, error) {
	t, err := d.timePart().addDays(d.Days())
	if err != nil {
		return internalDuration{}, err
	}
	return newInternalDuration(DateDuration{d.Years(), d.Months(), d.Weeks(), 0}, t)
}

// dateDurationWithoutTime is ToDateDurationRecordWithoutTime.
func (d Duration) dateDurationWithoutTime() (DateDuration, error) {
	i, err := d.internalWith24HourDays()
	if err != nil {
		return DateDuration{}, err
	}
	days := i.time.quo(i128(nsPerDay)).int64()
	return newDateDuration(i.date.Years, i.date.Months, i.date.Weeks, days)
}

// newDateDuration is DateDuration::new: the fields, valid as a duration.
func newDateDuration(years, months, weeks, days int64) (DateDuration, error) {
	if !validDuration(years, months, weeks, days, 0, 0, 0, 0, int128{}, int128{}) {
		return DateDuration{}, rangeError("Invalid DateDuration.")
	}
	return DateDuration{years, months, weeks, days}, nil
}

// adjust is AdjustDateDurationRecord.
func (d DateDuration) adjust(days int64, weeks, months *int64) DateDuration {
	out := DateDuration{d.Years, d.Months, d.Weeks, days}
	if weeks != nil {
		out.Weeks = *weeks
	}
	if months != nil {
		out.Months = *months
	}
	return out
}

func (d DateDuration) negated() DateDuration {
	neg := func(v int64) int64 {
		if v == math.MinInt64 {
			return math.MaxInt64
		}
		return -v
	}
	return DateDuration{neg(d.Years), neg(d.Months), neg(d.Weeks), neg(d.Days)}
}

// durationFromDate is Duration::from(DateDuration).
func durationFromDate(d DateDuration) Duration {
	return Duration{sign: d.sign(), years: uint32(unsignedAbs(d.Years)), months: uint32(unsignedAbs(d.Months)),
		weeks: uint32(unsignedAbs(d.Weeks)), days: unsignedAbs(d.Days)}
}

// An internalDuration is the Internal Duration Record: a date duration and
// a time duration, not of opposite signs.
type internalDuration struct {
	date DateDuration
	time timeDuration
}

// newInternalDuration is InternalDurationRecord::new.
func newInternalDuration(date DateDuration, t timeDuration) (internalDuration, error) {
	if ds, ts := date.sign(), t.sign(); ds != 0 && ts != 0 && ds != ts {
		return internalDuration{}, rangeError("DateDuration and TimeDuration must agree if both are not zero.")
	}
	return internalDuration{date, t}, nil
}

func (i internalDuration) sign() int {
	if s := i.date.sign(); s != 0 {
		return s
	}
	return i.time.sign()
}

// durationFromInternal is TemporalDurationFromInternal: the time part
// balanced up to the largest unit.
func durationFromInternal(i internalDuration, largest Unit) (Duration, error) {
	s := i.time.sign()
	if s == 0 {
		s = 1
	}
	ns := i.time.abs()
	thousand := i128(1000)
	var days, hours, minutes, seconds, milliseconds, microseconds int128
	divRem := func(v, d int128) (int128, int128) { return v.divEuclid(d), v.remEuclid(d) }
	switch largest {
	case Year, Month, Week, Day, Hour, Minute, Second, Millisecond, Microsecond:
		microseconds, ns = divRem(ns, thousand)
		if largest == Microsecond {
			break
		}
		milliseconds, microseconds = divRem(microseconds, thousand)
		if largest == Millisecond {
			break
		}
		seconds, milliseconds = divRem(milliseconds, thousand)
		if largest == Second {
			break
		}
		minutes, seconds = divRem(seconds, i128(60))
		if largest == Minute {
			break
		}
		hours, minutes = divRem(minutes, i128(60))
		if largest == Hour {
			break
		}
		days, hours = divRem(hours, i128(24))
	case Nanosecond:
	default:
		return Duration{}, assertError()
	}
	sg := int64(s)
	return newDuration(i.date.Years, i.date.Months, i.date.Weeks, i.date.Days+days.int64()*sg,
		hours.int64()*sg, minutes.int64()*sg, seconds.int64()*sg, milliseconds.int64()*sg,
		microseconds.mul64(sg), ns.mul64(sg))
}

// Add is Temporal.Duration.prototype.add.
func (d Duration) Add(o Duration) (Duration, error) {
	largest := maxUnit(d.defaultLargestUnit(), o.defaultLargestUnit())
	if largest.isCalendarUnit() {
		return Duration{}, rangeError("Largest unit cannot be a calendar unit when adding two durations.")
	}
	d1, err := d.internalWith24HourDays()
	if err != nil {
		return Duration{}, err
	}
	d2, err := o.internalWith24HourDays()
	if err != nil {
		return Duration{}, err
	}
	t, err := d1.time.plus(d2.time)
	if err != nil {
		return Duration{}, err
	}
	return durationFromInternal(internalDuration{time: t}, largest)
}

// Subtract is Temporal.Duration.prototype.subtract.
func (d Duration) Subtract(o Duration) (Duration, error) { return d.Add(o.Negated()) }

// A PartialDuration is a duration's fields as Temporal.Duration.from and
// with take them, each nil where not given.
type PartialDuration struct {
	Years, Months, Weeks, Days, Hours, Minutes, Seconds, Milliseconds *int64
	Microseconds, Nanoseconds                                         *float64
}

// DurationFromPartial is Duration::from_partial_duration, from the fields
// as V8 reads them.
func DurationFromPartial(p PartialDuration) (Duration, error) {
	get := func(v *int64) int64 {
		if v == nil {
			return 0
		}
		return *v
	}
	var us, ns int128
	var err error
	if p.Microseconds != nil {
		if us, err = int128FromIntegralFloat(*p.Microseconds, "μs out of range"); err != nil {
			return Duration{}, err
		}
	}
	if p.Nanoseconds != nil {
		if ns, err = int128FromIntegralFloat(*p.Nanoseconds, "ns out of range"); err != nil {
			return Duration{}, err
		}
	}
	if p == (PartialDuration{}) {
		return Duration{}, typeError("PartialDuration cannot have all empty fields.")
	}
	return newDuration(get(p.Years), get(p.Months), get(p.Weeks), get(p.Days), get(p.Hours), get(p.Minutes),
		get(p.Seconds), get(p.Milliseconds), us, ns)
}

// int128FromIntegralFloat is num-traits' i128::from_f64: truncated, and
// an error beyond i128's range.
func int128FromIntegralFloat(f float64, msg string) (int128, error) {
	if math.IsNaN(f) || f >= 0x1p127 || f < -0x1p127 {
		return int128{}, rangeError("%s", msg)
	}
	return int128FromFloat(f), nil
}

// DurationFromNumbers is Temporal.Duration's constructor as V8 takes its
// arguments, each already ToIntegerIfIntegral: the fields to microseconds
// must be within i64, and microseconds and nanoseconds within i128.
func DurationFromNumbers(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds float64) (Duration, error) {
	var ints [8]int64
	for i, f := range [...]float64{years, months, weeks, days, hours, minutes, seconds, milliseconds} {
		v, err := IntegerInRange64(f)
		if err != nil {
			return Duration{}, err
		}
		ints[i] = v
	}
	us, err := int128FromIntegralFloat(microseconds, "μs out of range")
	if err != nil {
		return Duration{}, err
	}
	ns, err := int128FromIntegralFloat(nanoseconds, "ms out of range")
	if err != nil {
		return Duration{}, err
	}
	return newDuration(ints[0], ints[1], ints[2], ints[3], ints[4], ints[5], ints[6], ints[7], us, ns)
}

// IntegerInRange64 is V8's check that an integral double is within int64:
// -2^63 ≤ f < 2^63.
func IntegerInRange64(f float64) (int64, error) {
	if !(f >= -0x1p63 && f < 0x1p63) {
		return 0, rangeError("Integer out of range.")
	}
	return int64(f), nil
}
