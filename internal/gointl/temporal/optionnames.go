package temporal

import "strconv"

// The names of the option values, as JavaScript spells them: what String
// returns for each, which is the string the option takes and the matching
// Parse function reads. A value that stands for an option not given is
// "undefined", as the option is in JavaScript; a value no constant names is
// its type and number, "Overflow(5)".

// optionName is the name at v's place in names, or the type and number.
func optionName(typ string, v int, names ...string) string {
	if v >= 0 && v < len(names) {
		return names[v]
	}
	return typ + "(" + strconv.Itoa(v) + ")"
}

// String is the unit's singular name, "day", or "auto", which ParseUnit
// reads back.
func (u Unit) String() string {
	return optionName("Unit", int(u), "undefined", "auto", "nanosecond", "microsecond", "millisecond", "second",
		"minute", "hour", "day", "week", "month", "year")
}

func (o Overflow) String() string { return optionName("Overflow", int(o), "constrain", "reject") }

func (m RoundingMode) String() string {
	return optionName("RoundingMode", int(m), "undefined", "ceil", "floor", "expand", "trunc",
		"halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven")
}

func (d Disambiguation) String() string {
	return optionName("Disambiguation", int(d), "compatible", "earlier", "later", "reject")
}

func (o OffsetDisambiguation) String() string {
	return optionName("OffsetDisambiguation", int(o), "undefined", "use", "prefer", "ignore", "reject")
}

func (d DisplayCalendar) String() string {
	return optionName("DisplayCalendar", int(d), "auto", "always", "never", "critical")
}

func (d DisplayOffset) String() string {
	return optionName("DisplayOffset", int(d), "auto", "never")
}

func (d DisplayTimeZone) String() string {
	return optionName("DisplayTimeZone", int(d), "auto", "never", "critical")
}

// String is the fractionalSecondDigits option's value, "auto" or a digit
// count, or "minute" for the precision a smallestUnit of minutes gives,
// which the option cannot say.
func (p Precision) String() string {
	switch {
	case p == PrecisionAuto:
		return "auto"
	case p == PrecisionMinute:
		return "minute"
	case p >= 0 && p <= 9:
		return strconv.Itoa(int(p))
	}
	return "Precision(" + strconv.Itoa(int(p)) + ")"
}
