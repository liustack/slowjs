package temporal

import (
	"math"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Temporal's options, as temporal_rs 0.2.3 takes them once the engine has
// read them: each is its zero value where the option was not given.

// unitTable is Table 21's units, from the largest.
var unitTable = [10]Unit{Year, Month, Week, Day, Hour, Minute, Second, Millisecond, Microsecond, Nanosecond}

var unitNames = map[string]Unit{
	"auto": UnitAuto, "year": Year, "years": Year, "month": Month, "months": Month, "week": Week,
	"weeks": Week, "day": Day, "days": Day, "hour": Hour, "hours": Hour, "minute": Minute,
	"minutes": Minute, "second": Second, "seconds": Second, "millisecond": Millisecond,
	"milliseconds": Millisecond, "microsecond": Microsecond, "microseconds": Microsecond,
	"nanosecond": Nanosecond, "nanoseconds": Nanosecond,
}

// ParseUnit is a unit option's value: its singular or plural name, or
// "auto".
func ParseUnit(s string) (Unit, bool) {
	u, ok := unitNames[s]
	return u, ok
}

func (u Unit) isCalendarUnit() bool { return u == Year || u == Month || u == Week }
func (u Unit) isDateUnit() bool     { return u >= Day }
func (u Unit) isTimeUnit() bool     { return u >= Nanosecond && u <= Hour }

// IsDateUnit reports whether the unit is a day or longer, which an engine
// asks of a unit it validates (ValidateTemporalUnitValue).
func (u Unit) IsDateUnit() bool { return u.isDateUnit() }

// nanoseconds is a time unit's length, 0 for the calendar units.
func (u Unit) nanoseconds() int64 {
	switch u {
	case Day:
		return nsPerDay
	case Hour:
		return 3_600_000_000_000
	case Minute:
		return 60_000_000_000
	case Second:
		return 1_000_000_000
	case Millisecond:
		return 1_000_000
	case Microsecond:
		return 1_000
	case Nanosecond:
		return 1
	}
	return 0
}

// maximumIncrement is MaximumTemporalDurationRoundingIncrement, 0 for none.
func (u Unit) maximumIncrement() uint64 {
	switch u {
	case Hour:
		return 24
	case Minute, Second:
		return 60
	case Millisecond, Microsecond, Nanosecond:
		return 1000
	}
	return 0
}

// tableIndex is the unit's row in Table 21.
func (u Unit) tableIndex() (int, error) {
	for i, v := range unitTable {
		if v == u {
			return i, nil
		}
	}
	return 0, internalError("'auto' units are not allowed during comparison")
}

// largerUnit is LargerOfTwoTemporalUnits.
func largerUnit(a, b Unit) (Unit, error) {
	for _, u := range unitTable {
		if a == u || b == u {
			return u, nil
		}
	}
	return 0, internalError("'auto' units are not allowed during comparison")
}

func maxUnit(a, b Unit) Unit {
	if a > b {
		return a
	}
	return b
}

// A unitGroup is the units an option may name.
type unitGroup int

const (
	groupDate unitGroup = iota
	groupTime
	groupDateTime
)

func (g unitGroup) validate(u, extra Unit) error {
	if u == extra {
		return nil
	}
	// A value no Unit names, which a Go caller can give where JavaScript
	// cannot: a unit past Year had divided by zero.
	if u != NoUnit && (u < UnitAuto || u > Year) {
		return rangeError("Unit was not a valid unit.")
	}
	switch g {
	case groupDate:
		if u == NoUnit || u.isDateUnit() {
			return nil
		}
		return rangeError("Unit was not part of the date unit group.")
	case groupTime:
		if u == NoUnit || u.isTimeUnit() {
			return nil
		}
		return rangeError("Unit was not part of the time unit group.")
	}
	if u != UnitAuto {
		return nil
	}
	return rangeError("'auto' units are not allowed during comparison")
}

func (g unitGroup) validateRequired(u, extra Unit) (Unit, error) {
	if u == NoUnit {
		return 0, rangeError("Unit is required")
	}
	return u, g.validate(u, extra)
}

// A RoundingMode is how to round, RoundingModeUnset where none was given.
type RoundingMode int

const (
	RoundingModeUnset RoundingMode = iota
	Ceil
	Floor
	Expand
	Trunc
	HalfCeil
	HalfFloor
	HalfExpand
	HalfTrunc
	HalfEven
)

var roundingModeNames = map[string]RoundingMode{
	"ceil": Ceil, "floor": Floor, "expand": Expand, "trunc": Trunc, "halfCeil": HalfCeil,
	"halfFloor": HalfFloor, "halfExpand": HalfExpand, "halfTrunc": HalfTrunc, "halfEven": HalfEven,
}

// ParseRoundingMode is a roundingMode option's value.
func ParseRoundingMode(s string) (RoundingMode, bool) {
	m, ok := roundingModeNames[s]
	return m, ok
}

func (m RoundingMode) or(def RoundingMode) RoundingMode {
	if m == RoundingModeUnset {
		return def
	}
	return m
}

// negate is NegateRoundingMode.
func (m RoundingMode) negate() RoundingMode {
	switch m {
	case Ceil:
		return Floor
	case Floor:
		return Ceil
	case HalfCeil:
		return HalfFloor
	case HalfFloor:
		return HalfCeil
	}
	return m
}

// unsignedRoundingMode is GetUnsignedRoundingMode's result.
type unsignedRoundingMode int

const (
	roundInfinity unsignedRoundingMode = iota
	roundZero
	roundHalfInfinity
	roundHalfZero
	roundHalfEven
)

func (m RoundingMode) unsigned(positive bool) unsignedRoundingMode {
	switch m {
	case Ceil:
		if positive {
			return roundInfinity
		}
		return roundZero
	case Trunc:
		return roundZero
	case Floor:
		if positive {
			return roundZero
		}
		return roundInfinity
	case Expand:
		return roundInfinity
	case HalfCeil:
		if positive {
			return roundHalfInfinity
		}
		return roundHalfZero
	case HalfTrunc:
		return roundHalfZero
	case HalfFloor:
		if positive {
			return roundHalfZero
		}
		return roundHalfInfinity
	case HalfExpand:
		return roundHalfInfinity
	}
	return roundHalfEven
}

// Overflow's names.
func ParseOverflow(s string) (Overflow, bool) {
	switch s {
	case "constrain":
		return Constrain, true
	case "reject":
		return Reject, true
	}
	return 0, false
}

// A Disambiguation is how a local time a transition skips or repeats is
// read.
type Disambiguation int

const (
	Compatible Disambiguation = iota
	Earlier
	Later
	DisambiguationReject
)

// ParseDisambiguation is a disambiguation option's value.
func ParseDisambiguation(s string) (Disambiguation, bool) {
	switch s {
	case "compatible":
		return Compatible, true
	case "earlier":
		return Earlier, true
	case "later":
		return Later, true
	case "reject":
		return DisambiguationReject, true
	}
	return 0, false
}

// An OffsetDisambiguation is what to do with an offset a local time gives
// that the zone does not have then, OffsetUnset where none was given.
type OffsetDisambiguation int

const (
	OffsetUnset OffsetDisambiguation = iota
	OffsetUse
	OffsetPrefer
	OffsetIgnore
	OffsetReject
)

// ParseOffsetDisambiguation is an offset option's value.
func ParseOffsetDisambiguation(s string) (OffsetDisambiguation, bool) {
	switch s {
	case "use":
		return OffsetUse, true
	case "prefer":
		return OffsetPrefer, true
	case "ignore":
		return OffsetIgnore, true
	case "reject":
		return OffsetReject, true
	}
	return 0, false
}

func (o OffsetDisambiguation) or(def OffsetDisambiguation) OffsetDisambiguation {
	if o == OffsetUnset {
		return def
	}
	return o
}

// DisplayCalendar is the calendarName option of toString.
type DisplayCalendar int

const (
	CalendarAuto DisplayCalendar = iota
	CalendarAlways
	CalendarNever
	CalendarCritical
)

// ParseDisplayCalendar is a calendarName option's value.
func ParseDisplayCalendar(s string) (DisplayCalendar, error) {
	switch s {
	case "auto":
		return CalendarAuto, nil
	case "always":
		return CalendarAlways, nil
	case "never":
		return CalendarNever, nil
	case "critical":
		return CalendarCritical, nil
	}
	return 0, rangeError("Invalid calendarName option provided")
}

// DisplayOffset is the offset option of toString.
type DisplayOffset int

const (
	OffsetAuto DisplayOffset = iota
	OffsetNever
)

// ParseDisplayOffset is an offset option's value for toString.
func ParseDisplayOffset(s string) (DisplayOffset, error) {
	switch s {
	case "auto":
		return OffsetAuto, nil
	case "never":
		return OffsetNever, nil
	}
	return 0, rangeError("Invalid offsetOption option provided")
}

// DisplayTimeZone is the timeZoneName option of toString.
type DisplayTimeZone int

const (
	TimeZoneAuto DisplayTimeZone = iota
	TimeZoneNever
	TimeZoneCritical
)

// ParseDisplayTimeZone is a timeZoneName option's value.
func ParseDisplayTimeZone(s string) (DisplayTimeZone, error) {
	switch s {
	case "auto":
		return TimeZoneAuto, nil
	case "never":
		return TimeZoneNever, nil
	case "critical":
		return TimeZoneCritical, nil
	}
	return 0, rangeError("Invalid timeZoneName option provided")
}

// A Precision is how many digits of fractional seconds toString writes:
// PrecisionAuto for as many as there are, PrecisionMinute for none and no
// seconds, or 0 to 9.
type Precision int

const (
	PrecisionAuto   Precision = -1
	PrecisionMinute Precision = -2
)

// A RoundingIncrement is a roundingIncrement option: 1 to 10⁹, 0 where
// none was given, which is 1.
type RoundingIncrement uint32

// NewRoundingIncrement is RoundingIncrement::try_from(f64), the option read
// from a number.
func NewRoundingIncrement(f float64) (RoundingIncrement, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, rangeError("roundingIncrement must be finite")
	}
	i := math.Trunc(f)
	if i < 1 || i > 1e9 {
		return 0, rangeError("roundingIncrement cannot be less that 1 or bigger than 10**9")
	}
	return RoundingIncrement(i), nil
}

func (r RoundingIncrement) get() uint64 {
	if r == 0 {
		return 1
	}
	return uint64(r)
}

// validate is ValidateTemporalRoundingIncrement.
func (r RoundingIncrement) validate(dividend uint64, inclusive bool) error {
	max := dividend
	if !inclusive {
		max--
	}
	inc := r.get()
	if inc > max {
		return rangeError("roundingIncrement exceeds maximum")
	}
	if dividend%inc != 0 {
		return rangeError("dividend is not divisible by roundingIncrement")
	}
	return nil
}

// ToStringRoundingOptions are toString's precision options.
type ToStringRoundingOptions struct {
	// Precision is fractionalSecondDigits: PrecisionAuto where not given.
	// Its zero value is not the default but 0, no fractional digits, as
	// fractionalSecondDigits: 0 is; start from DefaultToStringOptions().
	Precision    Precision
	SmallestUnit Unit // NoUnit where not given
	RoundingMode RoundingMode
}

// DefaultToStringOptions are toString's options where none are given.
func DefaultToStringOptions() ToStringRoundingOptions {
	return ToStringRoundingOptions{Precision: PrecisionAuto, SmallestUnit: NoUnit}
}

type resolvedToString struct {
	precision Precision
	smallest  Unit
	mode      RoundingMode
	increment RoundingIncrement
}

func (o ToStringRoundingOptions) resolve() (resolvedToString, error) {
	mode := o.RoundingMode.or(Trunc)
	r := resolvedToString{mode: mode, increment: 1}
	switch o.SmallestUnit {
	case Minute:
		r.precision, r.smallest = PrecisionMinute, Minute
	case Second:
		r.precision, r.smallest = 0, Second
	case Millisecond:
		r.precision, r.smallest = 3, Millisecond
	case Microsecond:
		r.precision, r.smallest = 6, Microsecond
	case Nanosecond:
		r.precision, r.smallest = 9, Nanosecond
	case NoUnit:
		switch p := o.Precision; {
		case p == PrecisionAuto:
			r.precision, r.smallest = PrecisionAuto, Nanosecond
		case p == 0:
			r.precision, r.smallest = 0, Second
		case p >= 1 && p <= 3:
			r.precision, r.smallest, r.increment = p, Millisecond, RoundingIncrement(pow10(3-int(p)))
		case p >= 4 && p <= 6:
			r.precision, r.smallest, r.increment = p, Microsecond, RoundingIncrement(pow10(6-int(p)))
		case p >= 7 && p <= 9:
			r.precision, r.smallest, r.increment = p, Nanosecond, RoundingIncrement(pow10(9-int(p)))
		default:
			return r, rangeError("Invalid fractionalDigits precision value")
		}
	default:
		return r, rangeError("smallestUnit must be a valid time unit.")
	}
	return r, nil
}

func pow10(n int) uint32 {
	v := uint32(1)
	for ; n > 0; n-- {
		v *= 10
	}
	return v
}

// DifferenceSettings are the options of until and since.
type DifferenceSettings struct {
	LargestUnit, SmallestUnit Unit // NoUnit where not given
	RoundingMode              RoundingMode
	Increment                 RoundingIncrement
	// Compat chooses Node's side of intl.RoundingWindow.
	Compat intl.Compat
}

// RoundingOptions are the options of round.
type RoundingOptions struct {
	LargestUnit, SmallestUnit Unit // NoUnit where not given
	RoundingMode              RoundingMode
	Increment                 RoundingIncrement
	// Compat chooses Node's side of intl.RoundingWindow, for a duration,
	// and of intl.RepeatedMidnight, for a ZonedDateTime.
	Compat intl.Compat
}

type differenceOp int

const (
	opUntil differenceOp = iota
	opSince
)

// resolvedRounding is ResolvedRoundingOptions.
type resolvedRounding struct {
	largest, smallest Unit
	increment         RoundingIncrement
	mode              RoundingMode
	compat            intl.Compat
}

func fromToStringOptions(o resolvedToString) resolvedRounding {
	return resolvedRounding{largest: UnitAuto, smallest: o.smallest, increment: o.increment, mode: o.mode}
}

// fromDiffSettings is GetDifferenceSettings.
func fromDiffSettings(o DifferenceSettings, op differenceOp, group unitGroup, fallbackLargest, fallbackSmallest Unit) (resolvedRounding, error) {
	if err := group.validate(o.LargestUnit, UnitAuto); err != nil {
		return resolvedRounding{}, err
	}
	mode := o.RoundingMode.or(Trunc)
	if op == opSince {
		mode = mode.negate()
	}
	smallest := o.SmallestUnit
	if smallest == NoUnit {
		smallest = fallbackSmallest
	}
	if err := group.validate(o.SmallestUnit, NoUnit); err != nil {
		return resolvedRounding{}, err
	}
	defLargest := maxUnit(smallest, fallbackLargest)
	largest := o.LargestUnit
	if largest == NoUnit || largest == UnitAuto {
		largest = defLargest
	}
	if largest < smallest {
		return resolvedRounding{}, rangeError("smallestUnit was larger than largestunit in DifferenceeSettings")
	}
	if max := smallest.maximumIncrement(); max != 0 {
		if err := o.Increment.validate(max, false); err != nil {
			return resolvedRounding{}, err
		}
	}
	return resolvedRounding{largest: largest, smallest: smallest, increment: o.Increment, mode: mode, compat: o.Compat}, nil
}

// fromDateTimeOptions is the resolution of PlainDateTime's and
// ZonedDateTime's round.
func fromDateTimeOptions(o RoundingOptions) (resolvedRounding, error) {
	mode := o.RoundingMode.or(HalfExpand)
	smallest, err := groupTime.validateRequired(o.SmallestUnit, Day)
	if err != nil {
		return resolvedRounding{}, err
	}
	max, inclusive := uint64(1), true
	if smallest != Day {
		max, inclusive = smallest.maximumIncrement(), false
		if max == 0 {
			return resolvedRounding{}, rangeError("smallestUnit must be a valid time unit.")
		}
	}
	if err := o.Increment.validate(max, inclusive); err != nil {
		return resolvedRounding{}, err
	}
	return resolvedRounding{largest: UnitAuto, smallest: smallest, increment: o.Increment, mode: mode}, nil
}

// fromTimeOptions is the resolution of PlainTime's round.
func fromTimeOptions(o RoundingOptions) (resolvedRounding, error) {
	if o.SmallestUnit == NoUnit {
		return resolvedRounding{}, rangeError("smallestUnit is required")
	}
	mode := o.RoundingMode.or(HalfExpand)
	max := o.SmallestUnit.maximumIncrement()
	if max == 0 {
		return resolvedRounding{}, rangeError("smallestUnit must be a valid time unit.")
	}
	if err := o.Increment.validate(max, false); err != nil {
		return resolvedRounding{}, err
	}
	return resolvedRounding{largest: UnitAuto, smallest: o.SmallestUnit, increment: o.Increment, mode: mode}, nil
}

// fromInstantOptions is the resolution of Instant's round.
func fromInstantOptions(o RoundingOptions) (resolvedRounding, error) {
	mode := o.RoundingMode.or(HalfExpand)
	smallest, err := groupTime.validateRequired(o.SmallestUnit, NoUnit)
	if err != nil {
		return resolvedRounding{}, err
	}
	var max uint64
	switch smallest {
	case Hour:
		max = 24
	case Minute:
		max = 24 * 60
	case Second:
		max = 24 * 3600
	case Millisecond:
		max = msPerDay
	case Microsecond:
		max = msPerDay * 1000
	case Nanosecond:
		max = nsPerDay
	default:
		return resolvedRounding{}, rangeError("Invalid roundTo unit provided.")
	}
	if err := o.Increment.validate(max, true); err != nil {
		return resolvedRounding{}, err
	}
	return resolvedRounding{largest: UnitAuto, smallest: smallest, increment: o.Increment, mode: mode}, nil
}

func (r resolvedRounding) isNoop() bool {
	return r.smallest == Nanosecond && r.increment.get() == 1
}

const nsPerDay = 86_400_000_000_000
