package temporal

// A PlainTime is Temporal.PlainTime's value, a time of day.
type PlainTime struct{ iso ISOTime }

// ISO is the time's fields.
func (t PlainTime) ISO() ISOTime { return t.iso }

// NewPlainTime is PlainTime::new_with_overflow.
func NewPlainTime(hour, minute, second, millisecond, microsecond, nanosecond int, overflow Overflow) (PlainTime, error) {
	iso, err := newISOTime(hour, minute, second, millisecond, microsecond, nanosecond, overflow)
	return PlainTime{iso}, err
}

// PlainTimeFromPartial is PlainTime::from_partial.
func PlainTimeFromPartial(p PartialTime, overflow Overflow) (PlainTime, error) {
	if p.isEmpty() {
		return PlainTime{}, typeError("PartialTime cannot be empty.")
	}
	iso, err := ISOTime{}.with(p, overflow)
	return PlainTime{iso}, err
}

// With is Temporal.PlainTime.prototype.with.
func (t PlainTime) With(p PartialTime, overflow Overflow) (PlainTime, error) {
	if p.isEmpty() {
		return PlainTime{}, typeError("fields cannot be empty")
	}
	iso, err := t.iso.with(p, overflow)
	return PlainTime{iso}, err
}

// addTimeDuration is PlainTime::add_normalized_time_duration.
func (t PlainTime) addTimeDuration(d timeDuration) (int64, PlainTime) {
	days, iso := t.iso.add(d)
	return days, PlainTime{iso}
}

// Add is Temporal.PlainTime.prototype.add: the time moved by the
// duration's time fields, wrapping around the clock.
func (t PlainTime) Add(d Duration) PlainTime {
	sat := func(a, b int64) int64 {
		r := a + b
		if a > 0 && b > 0 && r < 0 {
			return 1<<63 - 1
		}
		if a < 0 && b < 0 && r >= 0 {
			return -1 << 63
		}
		return r
	}
	_, iso := balanceISOTime(sat(int64(t.iso.Hour), d.Hours()), sat(int64(t.iso.Minute), d.Minutes()),
		sat(int64(t.iso.Second), d.Seconds()), sat(int64(t.iso.Millisecond), d.Milliseconds()),
		addSaturating(i128(int64(t.iso.Microsecond)), d.microsecondsSigned()),
		addSaturating(i128(int64(t.iso.Nanosecond)), d.nanosecondsSigned()))
	return PlainTime{iso}
}

// Subtract is Temporal.PlainTime.prototype.subtract.
func (t PlainTime) Subtract(d Duration) PlainTime { return t.Add(d.Negated()) }

// Until is Temporal.PlainTime.prototype.until.
func (t PlainTime) Until(o PlainTime, s DifferenceSettings) (Duration, error) {
	return t.diffTime(opUntil, o, s)
}

// Since is Temporal.PlainTime.prototype.since.
func (t PlainTime) Since(o PlainTime, s DifferenceSettings) (Duration, error) {
	return t.diffTime(opSince, o, s)
}

func (t PlainTime) diffTime(op differenceOp, o PlainTime, s DifferenceSettings) (Duration, error) {
	r, err := fromDiffSettings(s, op, groupTime, Hour, Nanosecond)
	if err != nil {
		return Duration{}, err
	}
	td, err := t.iso.diff(o.iso).round(r)
	if err != nil {
		return Duration{}, err
	}
	d, err := durationFromInternal(internalDuration{time: td}, r.largest)
	if err != nil {
		return Duration{}, err
	}
	if op == opSince {
		d = d.Negated()
	}
	return d, nil
}

// Round is Temporal.PlainTime.prototype.round.
func (t PlainTime) Round(o RoundingOptions) (PlainTime, error) {
	r, err := fromTimeOptions(o)
	if err != nil {
		return PlainTime{}, err
	}
	_, iso, err := t.iso.round(r)
	return PlainTime{iso}, err
}

// Compare is Temporal.PlainTime.compare.
func (t PlainTime) Compare(o PlainTime) int { return t.iso.compare(o.iso) }

// String is Temporal.PlainTime.prototype.toString.
func (t PlainTime) String(o ToStringRoundingOptions) (string, error) {
	r, err := o.resolve()
	if err != nil {
		return "", err
	}
	_, iso, err := t.iso.round(fromToStringOptions(r))
	if err != nil {
		return "", err
	}
	var b ixdtfBuilder
	b.time(iso, r.precision)
	return b.String(), nil
}

// EpochNanosecondsForUTC is the time on 1970-01-01 read as UTC, as
// Intl.DateTimeFormat formats a PlainTime, as a high and low word as
// Instant.EpochNanoseconds gives them.
func (t PlainTime) EpochNanosecondsForUTC() (hi int64, lo uint64) {
	ns := epochNanoseconds(unixEpoch, t.iso)
	return ns.hi, ns.lo
}
