package temporal

// An Instant is Temporal.Instant's value: nanoseconds since 1970, within
// 10^8 days either way.
type Instant struct{ ns int128 }

// EpochNanoseconds is the instant's nanoseconds as a high and low word, the
// high word signed, as temporal_capi's I128Nanoseconds holds them.
func (i Instant) EpochNanoseconds() (hi int64, lo uint64) { return i.ns.hi, i.ns.lo }

// NewInstant is Instant::try_new, from nanoseconds as a high and low word.
func NewInstant(hi int64, lo uint64) (Instant, error) {
	ns := int128{hi, lo}
	if !validEpochNanoseconds(ns) {
		return Instant{}, rangeError("Instant nanoseconds are not within a valid epoch range.")
	}
	return Instant{ns}, nil
}

func newInstant(ns int128) (Instant, error) {
	if !validEpochNanoseconds(ns) {
		return Instant{}, rangeError("Instant nanoseconds are not within a valid epoch range.")
	}
	return Instant{ns}, nil
}

// InstantFromEpochMilliseconds is Instant::from_epoch_milliseconds.
func InstantFromEpochMilliseconds(ms int64) (Instant, error) {
	return newInstant(i128(ms).mul64(1_000_000))
}

// EpochMilliseconds is the instant's milliseconds, floored.
func (i Instant) EpochMilliseconds() int64 { return i.ns.divEuclid(i128(1_000_000)).int64() }

// ParseInstant is Instant::from_utf8.
func ParseInstant(s []byte) (Instant, error) {
	r, err := parseIXDTF(s, variantDateTime)
	if err != nil {
		return Instant{}, err
	}
	if r.date == nil || r.time == nil || !r.z && r.offset == nil {
		return Instant{}, rangeError("Required fields missing from Instant string.")
	}
	var offset int64
	if r.offset != nil {
		o := r.offset
		var ns uint32
		if o.fraction != nil {
			v, ok := o.fraction.nanoseconds()
			if !ok {
				return Instant{}, rangeError("Fractional time exceeds nine digits.")
			}
			ns = v
		}
		offset = int64(o.hour)*3_600_000_000_000 + int64(o.minute)*60_000_000_000 +
			int64(o.second)*1_000_000_000 + int64(ns)
		if o.negative {
			offset = -offset
		}
	}
	var frac uint32
	if r.time.fraction != nil {
		v, ok := r.time.fraction.nanoseconds()
		if !ok {
			return Instant{}, rangeError("Fractional time exceeds nine digits.")
		}
		frac = v
	}
	second := int64(r.time.second)
	if second > 59 {
		second = 59
	}
	dt := balanceISODateTime(r.date.year, int32(r.date.month), int32(r.date.day), int64(r.time.hour),
		int64(r.time.minute), second, int64(frac/1_000_000), i128(int64(frac%1_000_000/1000)),
		i128(int64(frac%1000)-offset))
	return newInstant(dt.epochNanoseconds())
}

// addTimeDuration is AddInstant.
func (i Instant) addTimeDuration(d timeDuration) (Instant, error) {
	return newInstant(i.ns.add(d.int128))
}

// Add is Temporal.Instant.prototype.add.
func (i Instant) Add(d Duration) (Instant, error) {
	if d.defaultLargestUnit().isDateUnit() {
		return Instant{}, rangeError("Largest unit cannot be a date unit")
	}
	id, err := d.internalWith24HourDays()
	if err != nil {
		return Instant{}, err
	}
	return i.addTimeDuration(id.time)
}

// Subtract is Temporal.Instant.prototype.subtract.
func (i Instant) Subtract(d Duration) (Instant, error) { return i.Add(d.Negated()) }

// diffInternal is DifferenceInstant.
func (i Instant) diffInternal(o Instant, r resolvedRounding) (internalDuration, error) {
	diff, err := timeDurationFromDifference(o.ns, i.ns)
	if err != nil {
		return internalDuration{}, err
	}
	t, err := diff.round(r)
	if err != nil {
		return internalDuration{}, err
	}
	return newInternalDuration(DateDuration{}, t)
}

func (i Instant) diff(op differenceOp, o Instant, s DifferenceSettings) (Duration, error) {
	r, err := fromDiffSettings(s, op, groupTime, Second, Nanosecond)
	if err != nil {
		return Duration{}, err
	}
	id, err := i.diffInternal(o, r)
	if err != nil {
		return Duration{}, err
	}
	d, err := durationFromInternal(id, r.largest)
	if err != nil {
		return Duration{}, err
	}
	if op == opSince {
		d = d.Negated()
	}
	return d, nil
}

// Until is Temporal.Instant.prototype.until.
func (i Instant) Until(o Instant, s DifferenceSettings) (Duration, error) {
	return i.diff(opUntil, o, s)
}

// Since is Temporal.Instant.prototype.since.
func (i Instant) Since(o Instant, s DifferenceSettings) (Duration, error) {
	return i.diff(opSince, o, s)
}

// round is Instant::round_instant: the nanoseconds rounded as if positive.
func (i Instant) round(r resolvedRounding) (int128, error) {
	var unit int64
	switch r.smallest {
	case Hour:
		unit = 3_600_000_000_000
	case Minute:
		unit = 60_000_000_000
	case Second:
		unit = 1_000_000_000
	case Millisecond:
		unit = 1_000_000
	case Microsecond:
		unit = 1000
	case Nanosecond:
		unit = 1
	default:
		return int128{}, rangeError("Invalid unit provided for Instant::round.")
	}
	inc := i128(int64(r.increment.get())).mul64(unit)
	return roundIncrementAsIfPositive(i.ns, inc, r.mode), nil
}

// Round is Temporal.Instant.prototype.round.
func (i Instant) Round(o RoundingOptions) (Instant, error) {
	r, err := fromInstantOptions(o)
	if err != nil {
		return Instant{}, err
	}
	ns, err := i.round(r)
	if err != nil {
		return Instant{}, err
	}
	return newInstant(ns)
}

// Compare is Temporal.Instant.compare.
func (i Instant) Compare(o Instant) int { return i.ns.cmp(o.ns) }

// Equals is Temporal.Instant.prototype.equals.
func (i Instant) Equals(o Instant) bool { return i.ns == o.ns }

// ToZonedDateTimeISO is Temporal.Instant.prototype.toZonedDateTimeISO.
func (i Instant) ToZonedDateTimeISO(tz TimeZone) (ZonedDateTime, error) {
	return newZonedDateTimeWithOffset(i, tz, isoCalendar)
}

// String is Temporal.Instant.prototype.toString; a nil zone writes UTC
// with Z.
func (i Instant) String(zones *Zones, tz *TimeZone, o ToStringRoundingOptions) (string, error) {
	r, err := o.resolve()
	if err != nil {
		return "", err
	}
	ns, err := i.round(fromToStringOptions(r))
	if err != nil {
		return "", err
	}
	rounded, err := newInstant(ns)
	if err != nil {
		return "", err
	}
	var b ixdtfBuilder
	var dt ISODateTime
	if tz != nil {
		if dt, err = tz.isoDateTimeFor(rounded.ns); err != nil {
			return "", err
		}
		off, err := tz.offsetNanosFor(rounded.ns)
		if err != nil {
			return "", err
		}
		b.date(dt.Date)
		b.time(dt.Time, r.precision)
		neg, h, m := offsetMinutes(i128(off))
		b.minuteOffset(neg, h, m, OffsetAuto)
	} else {
		utc := zones.UTC()
		if dt, err = utc.isoDateTimeFor(rounded.ns); err != nil {
			return "", err
		}
		b.date(dt.Date)
		b.time(dt.Time, r.precision)
		b.z(OffsetAuto)
	}
	return b.String(), nil
}

// offsetMinutes is nanoseconds_to_formattable_offset_minutes: the offset
// rounded to the minute, half away from zero.
func offsetMinutes(ns int128) (negative bool, hour, minute int) {
	r := roundIncrement(ns, i128(60_000_000_000), HalfExpand)
	m := int32(r.quo(i128(60_000_000_000)).int64())
	negative = m < 0
	if m < 0 {
		m = -m
	}
	return negative, int(m / 60), int(m % 60)
}
