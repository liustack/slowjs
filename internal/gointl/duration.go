package intl

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// DurationFormat writes how long something took, as ECMA-402's
// Intl.DurationFormat does: "1 hr, 46 min, 40 sec" in English, "1 h, 46 min
// et 40 s" in French, "1:46:40" where a clock is wanted.
//
// It is made of the other formatters. Each part is a measurement, written by
// a NumberFormat in unit style; the parts are joined by a ListFormat of
// units; and the parts written as numbers run together with the mark the
// locale puts between hours and minutes, which is ICU's DateFormatSymbols'
// time separator. The options resolve as V8 resolves them, which reports a
// sub-second unit that joins the seconds as "numeric" where the proposal
// says "fractional".

// DurationStyle is how a duration is written as a whole.
type DurationStyle int

const (
	// DurationShort is "1 hr, 46 min", and the default.
	DurationShort DurationStyle = iota
	DurationLong
	DurationNarrow
	// DurationDigital is "1:46:40".
	DurationDigital
)

// DurationUnitStyle is how one unit of a duration is written. The zero
// value leaves it to the duration's style.
type DurationUnitStyle int

const (
	DurationUnitDefault DurationUnitStyle = iota
	DurationUnitLong
	DurationUnitShort
	DurationUnitNarrow
	// DurationUnitNumeric writes the unit as a bare number, joined to the
	// units beside it by the time separator.
	DurationUnitNumeric
	// DurationUnitTwoDigit is a bare number of at least two digits.
	DurationUnitTwoDigit
)

// DurationDisplay is whether a unit that is zero is written. The zero value
// leaves it to the unit's style.
type DurationDisplay int

const (
	DurationDisplayDefault DurationDisplay = iota
	// DurationDisplayAuto leaves a zero out.
	DurationDisplayAuto
	// DurationDisplayAlways writes it.
	DurationDisplayAlways
)

// The units of a duration, largest first, which index Duration's fields in
// DurationFormatOptions' Units and Display.
const (
	DurationYears = iota
	DurationMonths
	DurationWeeks
	DurationDays
	DurationHours
	DurationMinutes
	DurationSeconds
	DurationMilliseconds
	DurationMicroseconds
	DurationNanoseconds
	DurationUnits
)

// durationUnitNames are ECMA-402's names for the units, and the units a
// NumberFormat writes each in.
var durationUnitNames = [DurationUnits]struct{ plural, singular string }{
	{"years", "year"}, {"months", "month"}, {"weeks", "week"}, {"days", "day"},
	{"hours", "hour"}, {"minutes", "minute"}, {"seconds", "second"},
	{"milliseconds", "millisecond"}, {"microseconds", "microsecond"},
	{"nanoseconds", "nanosecond"},
}

// A Duration is an amount of each unit. ECMA-402 takes each as a whole
// number, all of one sign; see Valid.
type Duration [DurationUnits]float64

// DurationFormatOptions mirrors the option bag of Intl.DurationFormat. Its
// zero value is ECMA-402's default in every field.
type DurationFormatOptions struct {
	// NumberingSystem names the digits to write, as for NumberFormat.
	NumberingSystem string
	Style           DurationStyle
	// Units and Display are the per-unit options, "hours" and
	// "hoursDisplay", indexed by DurationYears and the rest.
	Units   [DurationUnits]DurationUnitStyle
	Display [DurationUnits]DurationDisplay
	// FractionalDigits is how many digits a fraction of a second keeps,
	// from 0 to 9; nil keeps as many as there are, up to nine.
	FractionalDigits *int

	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A DurationFormat writes durations in one locale. It never changes after it
// is built and is safe for any number of goroutines to share.
type DurationFormat struct {
	locale  Locale
	opts    DurationFormatOptions
	styles  [DurationUnits]durationStyle
	display [DurationUnits]DurationDisplay
	// formats write each unit. The one whose smaller units join it as a
	// fraction keeps the fraction. A unit written without its sign, which
	// ECMA-402 writes with signDisplay "never", is given its magnitude.
	formats   [DurationUnits]*NumberFormat
	list      *ListFormat
	separator string
	system    string
}

// durationStyle is a unit's resolved style, which, unlike the option, can
// be fractional: a sub-second unit that joins the seconds.
type durationStyle int

const (
	styleLong durationStyle = iota
	styleShort
	styleNarrow
	styleNumeric
	styleTwoDigit
	styleFractional
)

func (s durationStyle) numeric() bool {
	return s == styleNumeric || s == styleTwoDigit || s == styleFractional
}

// NewDurationFormat builds a formatter from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewDurationFormat(loc Locale, opts DurationFormatOptions) (*DurationFormat, error) {
	return NewDurationFormatFrom(Embedded, loc, opts)
}

// NewDurationFormatFrom builds a formatter from a source of the caller's own.
func NewDurationFormatFrom(src Source, loc Locale, opts DurationFormatOptions) (*DurationFormat, error) {
	if opts.Style < DurationShort || opts.Style > DurationDigital {
		return nil, fmt.Errorf("intl: %d is not a duration style", opts.Style)
	}
	if d := opts.FractionalDigits; d != nil && (*d < 0 || *d > 9) {
		return nil, fmt.Errorf("intl: fractionalDigits %d is out of range", *d)
	}
	f := &DurationFormat{opts: opts}
	if err := f.resolveUnits(); err != nil {
		return nil, err
	}

	// A plain number, which settles the locale, the numbering system and
	// the time separator.
	sources, err := loadNumberSources(src, loc, opts.NumberingSystem)
	if err != nil {
		return nil, err
	}
	plain, err := sources.numberFormat(NumberFormatOptions{})
	if err != nil {
		return nil, err
	}
	f.locale = plain.locale
	f.system = plain.data.NumberingSystem
	f.separator = plain.data.TimeSeparator(f.system)

	// Each unit that can be written; the units after the one that takes the
	// smaller ones as its fraction are never written.
	for i := 0; i < DurationUnits; i++ {
		var o NumberFormatOptions
		switch f.styles[i] {
		case styleNumeric, styleTwoDigit, styleFractional:
			o.UseGrouping = GroupingNever
			if f.styles[i] == styleTwoDigit {
				o.MinimumIntegerDigits = 2
			}
		default:
			o.Style, o.Unit = StyleUnit, durationUnitNames[i].singular
			o.UnitDisplay = [...]UnitDisplay{UnitLong, UnitShort, UnitNarrow}[f.styles[i]]
		}
		if f.takesFraction(i) {
			maxFrac, minFrac := 9, 0
			if d := opts.FractionalDigits; d != nil {
				maxFrac, minFrac = *d, *d
			}
			o.MaximumFractionDigits, o.MinimumFractionDigits = &maxFrac, &minFrac
			o.RoundingMode = Trunc
		}
		if f.formats[i], err = sources.numberFormat(o); err != nil {
			return nil, err
		}
		if f.takesFraction(i) {
			break
		}
	}

	listStyle := ListShort
	switch opts.Style {
	case DurationLong:
		listStyle = ListLong
	case DurationNarrow:
		listStyle = ListNarrow
	}
	if f.list, err = NewListFormatFrom(src, loc, ListFormatOptions{Type: UnitList, Style: listStyle}); err != nil {
		return nil, err
	}
	return f, nil
}

// resolveUnits is GetDurationUnitOptions for each unit in turn.
func (f *DurationFormat) resolveUnits() error {
	base := [...]durationStyle{DurationShort: styleShort, DurationLong: styleLong,
		DurationNarrow: styleNarrow, DurationDigital: styleNumeric}[f.opts.Style]
	var prev durationStyle
	havePrev := false
	for i := 0; i < DurationUnits; i++ {
		name := durationUnitNames[i].plural
		asked := f.opts.Units[i]
		var style durationStyle
		displayDefault := DurationDisplayAlways
		switch asked {
		case DurationUnitLong, DurationUnitShort, DurationUnitNarrow:
			style = durationStyle(asked - DurationUnitLong)
		case DurationUnitNumeric:
			if i < DurationHours {
				return fmt.Errorf("intl: %s cannot be numeric", name)
			}
			style = styleNumeric
		case DurationUnitTwoDigit:
			if i < DurationHours || i > DurationSeconds {
				return fmt.Errorf("intl: %s cannot be 2-digit", name)
			}
			style = styleTwoDigit
		case DurationUnitDefault:
			hms := i >= DurationHours && i <= DurationSeconds
			switch {
			case f.opts.Style == DurationDigital:
				if !hms {
					displayDefault = DurationDisplayAuto
				}
				style = styleNumeric
				if i < DurationHours {
					style = styleShort
				}
			case havePrev && prev.numeric():
				if i != DurationMinutes && i != DurationSeconds {
					displayDefault = DurationDisplayAuto
				}
				style = styleNumeric
			default:
				displayDefault = DurationDisplayAuto
				style = base
				if base == styleNumeric {
					style = styleShort
				}
			}
		default:
			return fmt.Errorf("intl: %d is not a duration unit style", asked)
		}
		if style == styleNumeric && i >= DurationMilliseconds {
			style, displayDefault = styleFractional, DurationDisplayAuto
		}
		display := f.opts.Display[i]
		switch display {
		case DurationDisplayDefault:
			display = displayDefault
		case DurationDisplayAuto, DurationDisplayAlways:
		default:
			return fmt.Errorf("intl: %d is not a duration display", display)
		}
		if display == DurationDisplayAlways && style == styleFractional {
			return fmt.Errorf("intl: %s joins the seconds and cannot always be displayed", name)
		}
		if havePrev && prev == styleFractional && style != styleFractional {
			return fmt.Errorf("intl: %s must be numeric after a numeric sub-second unit", name)
		}
		if havePrev && (prev == styleNumeric || prev == styleTwoDigit) {
			if !style.numeric() {
				return fmt.Errorf("intl: %s must be numeric after a numeric unit", name)
			}
			if i == DurationMinutes || i == DurationSeconds {
				style = styleTwoDigit
			}
		}
		f.styles[i], f.display[i] = style, display
		prev, havePrev = style, true
	}
	return nil
}

// takesFraction reports whether a unit is written with the smaller units as
// its fraction: the seconds, milliseconds or microseconds followed by a
// unit that joins them.
func (f *DurationFormat) takesFraction(i int) bool {
	return (i == DurationSeconds || i == DurationMilliseconds || i == DurationMicroseconds) &&
		f.styles[i+1] == styleFractional
}

// Valid reports whether a duration is one ECMA-402's IsValidDuration
// accepts: whole numbers, all of one sign, the years, months and weeks
// below 2**32, and the rest less than 2**53 seconds in all.
func (d Duration) Valid() bool {
	sign := 0
	for _, v := range d {
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) {
			return false
		}
		s := 0
		if v < 0 {
			s = -1
		} else if v > 0 {
			s = 1
		}
		if s != 0 && sign != 0 && s != sign {
			return false
		}
		if s != 0 {
			sign = s
		}
	}
	for _, v := range d[:DurationDays] {
		if math.Abs(v) >= 1<<32 {
			return false
		}
	}
	// The days to the nanoseconds, in nanoseconds, exactly.
	scale := [...]int64{86400e9, 3600e9, 60e9, 1e9, 1e6, 1e3, 1}
	total := new(big.Int)
	for i, v := range d[DurationDays:] {
		n, _ := new(big.Float).SetFloat64(math.Abs(v)).Int(nil)
		total.Add(total, n.Mul(n, big.NewInt(scale[i])))
	}
	limit := new(big.Int).Lsh(big.NewInt(1e9), 53)
	return total.Cmp(limit) < 0
}

// sign is -1 if any unit is negative.
func (d Duration) sign() int {
	for _, v := range d {
		if v < 0 {
			return -1
		}
	}
	return 1
}

// DurationPart is one piece of a written duration. Unit is the unit a
// number part belongs to, "hour", and empty for the list's own literals.
type DurationPart struct {
	Kind  PartKind
	Value string
	Unit  string
}

// Format writes a duration. It fails for a duration Valid rejects, where
// ECMA-402 throws a RangeError.
func (f *DurationFormat) Format(d Duration) (string, error) {
	parts, err := f.FormatToParts(d)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Value)
	}
	return b.String(), nil
}

// FormatToParts writes a duration as pieces: ECMA-402's
// PartitionDurationFormatPattern, as V8 carries it out
// (js-duration-format.cc). Where the proposal leaves room, V8's choices are
// kept: numeric seconds join whatever part was written last, numeric minutes
// join it whenever the hours are numeric in style, and a zero numeric
// minute is written between hours and seconds only in two-digit style.
func (f *DurationFormat) FormatToParts(d Duration) ([]DurationPart, error) {
	if !d.Valid() {
		return nil, fmt.Errorf("intl: %v is not a valid duration", d)
	}
	w := durationWriter{f: f, negative: d.sign() < 0, signPending: true}
	auto := func(i int) bool { return f.display[i] == DurationDisplayAuto }

	for i := DurationYears; i <= DurationDays; i++ {
		if d[i] != 0 || !auto(i) {
			w.write(i, DecimalFromFloat(d[i]+0), false, true)
		}
	}
	// The hours, minutes and seconds, as OutputLongShortNarrowNumericOr2Digit.
	hoursNumeric := f.styles[DurationHours] == styleNumeric || f.styles[DurationHours] == styleTwoDigit
	clock := func(i int, maybeJoin, required bool) bool {
		if d[i] == 0 && auto(i) && !required {
			return false
		}
		if f.styles[i] == styleTwoDigit {
			w.write(i, DecimalFromFloat(d[i]+0), maybeJoin, true)
			return true
		}
		// A zero the display would leave out stays out, whatever was
		// required.
		if d[i] == 0 && auto(i) {
			return false
		}
		w.write(i, DecimalFromFloat(d[i]+0), maybeJoin && f.styles[i] == styleNumeric, true)
		return true
	}
	hoursWritten := clock(DurationHours, false, false)
	// DisplayRequired: numeric hours that are written, and a second or less
	// after them.
	required := hoursNumeric && (!auto(DurationHours) || d[DurationHours] != 0) &&
		(f.display[DurationSeconds] == DurationDisplayAlways || d[DurationSeconds] != 0 ||
			d[DurationMilliseconds] != 0 || d[DurationMicroseconds] != 0 || d[DurationNanoseconds] != 0)
	// The minutes join the hours by the time separator. V8 joins them to
	// whatever was written last when the hours are numeric but were left
	// out, "1 day:01:02" (DurationSeparator); the proposal starts a new
	// group, "1 day, 01:02".
	joinMinutes := hoursNumeric && (hoursWritten || f.opts.Compat.Has(DurationSeparator))
	clock(DurationMinutes, joinMinutes, required)

	for i := DurationSeconds; i < DurationUnits; i++ {
		if f.takesFraction(i) {
			// OutputFractional: the unit and those after it, in nanoseconds.
			value, zero := fractionOf(d, i, f.opts.Compat)
			if zero && auto(i) {
				break
			}
			join := f.styles[i] == styleNumeric || f.styles[i] == styleTwoDigit
			w.write(i, value, join, false)
			break
		}
		if i == DurationSeconds {
			clock(i, true, false)
			continue
		}
		if d[i] != 0 || !auto(i) {
			w.write(i, DecimalFromFloat(d[i]+0), false, true)
		}
	}

	items := make([]string, len(w.groups))
	for i, g := range w.groups {
		var b strings.Builder
		for _, p := range g {
			b.WriteString(p.Value)
		}
		items[i] = b.String()
	}
	out := []DurationPart{}
	at := 0
	for _, p := range f.list.FormatToParts(items) {
		if p.Kind == ListElement && at < len(w.groups) {
			out = append(out, w.groups[at]...)
			at++
			continue
		}
		out = append(out, DurationPart{Kind: PartLiteral, Value: p.Value})
	}
	return out, nil
}

// durationWriter gathers the parts of a duration as V8's Output does: each
// written unit a group of its own, or joined to the last one by the time
// separator, and the sign on the first alone.
type durationWriter struct {
	f           *DurationFormat
	groups      [][]DurationPart
	negative    bool
	signPending bool
}

// write writes one unit. A zero written first in a negative duration is
// negative zero, so that it carries the sign, unless it is the fraction
// V8 writes from its nanoseconds, which has no negative zero.
func (w *durationWriter) write(i int, value Decimal, join, negativeZero bool) {
	if w.signPending {
		w.signPending = false
		if negativeZero && w.negative && value.isZero() {
			value = ParseDecimal("-0")
		}
	} else {
		value = value.abs()
	}
	unit := durationUnitNames[i].singular
	var parts []DurationPart
	for _, p := range w.f.formats[i].FormatDecimalToParts(value) {
		parts = append(parts, DurationPart{Kind: p.Kind, Value: p.Value, Unit: unit})
	}
	if join && len(w.groups) > 0 {
		last := len(w.groups) - 1
		w.groups[last] = append(w.groups[last], DurationPart{Kind: PartLiteral, Value: w.f.separator})
		w.groups[last] = append(w.groups[last], parts...)
		return
	}
	w.groups = append(w.groups, parts)
}

// fractionOf is a unit with the smaller ones written as its fraction, summed
// exactly in nanoseconds, as the proposal sums them; NodeICU sums them as V8
// does (v8FractionOf). It reports whether the sum is zero.
func fractionOf(d Duration, from int, compat Compat) (Decimal, bool) {
	if compat.Has(DurationOverflow) {
		return v8FractionOf(d, from)
	}
	exponent := 9 - 3*(from-DurationSeconds)
	scale := [...]int64{1e9, 1e6, 1e3, 1}
	total := new(big.Int)
	for i := from; i < DurationUnits; i++ {
		n, _ := new(big.Float).SetFloat64(d[i]).Int(nil)
		total.Add(total, n.Mul(n, big.NewInt(scale[i-DurationSeconds])))
	}
	if total.Sign() == 0 {
		return DecimalFromFloat(0), true
	}
	digits := new(big.Int).Abs(total).String()
	if len(digits) <= exponent {
		digits = strings.Repeat("0", exponent-len(digits)+1) + digits
	}
	text := digits[:len(digits)-exponent] + "." + digits[len(digits)-exponent:]
	if total.Sign() < 0 {
		text = "-" + text
	}
	return ParseDecimal(text), false
}

// ResolvedDurationFormat is what a DurationFormat settled on.
type ResolvedDurationFormat struct {
	Locale           string
	NumberingSystem  string
	Style            DurationStyle
	Units            [DurationUnits]DurationUnitStyle
	Display          [DurationUnits]DurationDisplay
	FractionalDigits *int
}

// ResolvedOptions returns what the formatter settled on. A sub-second unit
// that joins the seconds is reported as numeric, as V8 reports it.
func (f *DurationFormat) ResolvedOptions() ResolvedDurationFormat {
	r := ResolvedDurationFormat{
		Locale: f.locale.String(), NumberingSystem: f.system,
		Style: f.opts.Style, Display: f.display,
	}
	for i, s := range f.styles {
		r.Units[i] = [...]DurationUnitStyle{
			styleLong: DurationUnitLong, styleShort: DurationUnitShort, styleNarrow: DurationUnitNarrow,
			styleNumeric: DurationUnitNumeric, styleTwoDigit: DurationUnitTwoDigit,
			styleFractional: DurationUnitNumeric,
		}[s]
	}
	if d := f.opts.FractionalDigits; d != nil {
		v := *d
		r.FractionalDigits = &v
	}
	return r
}

// v8FractionOf is V8's OutputFractional and the sums before it
// (js-duration-format.cc): the smaller units summed in a double, converted
// to an int64 of nanoseconds and carried into the unit, then written as
// the unit's integer scaled by a power of ten plus the nanoseconds.
//
// A sum past 2**63 nanoseconds does not fit. C++ leaves converting it
// undefined; x86-64, where Node runs, gives INT64_MIN, so Node writes 1e20
// nanoseconds as 9223372036.854775808 seconds, negative.
func v8FractionOf(d Duration, from int) (Decimal, bool) {
	var integer, nanos int64
	power := 3
	switch from {
	case DurationSeconds:
		// Each product is rounded before it is added, as C++ rounds it,
		// rather than fused.
		ns := x86Int64(d[DurationNanoseconds] + float64(d[DurationMicroseconds]*1000) +
			float64(d[DurationMilliseconds]*1000000))
		integer = x86Int64(d[DurationSeconds] + float64(ns/1000000000))
		nanos, power = ns%1000000000, 9
	case DurationMilliseconds:
		ns := x86Int64(d[DurationNanoseconds] + float64(d[DurationMicroseconds]*1000))
		integer = x86Int64(d[DurationMilliseconds] + float64(ns/1000000))
		nanos, power = ns%1000000, 6
	default:
		ns := x86Int64(d[DurationNanoseconds])
		integer = x86Int64(d[DurationMicroseconds] + float64(ns/1000))
		nanos = ns % 1000
	}
	if integer == 0 && nanos == 0 {
		return DecimalFromFloat(0), true
	}
	factor := int64(1)
	for i := 0; i < power; i++ {
		factor *= 10
	}
	var digits string
	// llabs(INT64_MIN) is INT64_MIN on x86-64, which is below the bound.
	abs := integer
	if abs < 0 {
		abs = -abs
	}
	if abs < math.MaxInt64/factor-1 {
		// formatInt, in int64 arithmetic, which wraps as x86-64's does.
		digits = strconv.FormatInt(nanos+integer*factor, 10)
	} else {
		// formatDecimal of the integer and the nanoseconds' magnitude,
		// padded to the power.
		n := nanos
		if n < 0 {
			n = -n
		}
		fraction := strconv.FormatInt(n, 10)
		digits = strconv.FormatInt(integer, 10)
		if len(fraction) < power {
			digits += strings.Repeat("0", power-len(fraction))
		}
		digits += fraction
	}
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign, digits = "-", digits[1:]
	}
	if len(digits) <= power {
		digits = strings.Repeat("0", power-len(digits)+1) + digits
	}
	return ParseDecimal(sign + digits[:len(digits)-power] + "." + digits[len(digits)-power:]), false
}

// x86Int64 converts a double to an int64 as x86-64's cvttsd2si does:
// truncated, and INT64_MIN for NaN and anything out of range, which C++
// leaves undefined.
func x86Int64(f float64) int64 {
	if math.IsNaN(f) || f >= 0x1p63 || f < -0x1p63 {
		return math.MinInt64
	}
	return int64(f)
}
