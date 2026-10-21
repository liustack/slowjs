package temporal

// Fields as Temporal hands them to a calendar, and the ISO calendar's own
// reading of them, as temporal_rs 0.2.3 has it.

// Overflow is what to do with a field out of range: constrain it to the
// nearest valid value, or reject it.
type Overflow int

const (
	Constrain Overflow = iota
	Reject
)

// CalendarFields are the fields a date is made from, temporal_rs's
// CalendarFields. A field not given is nil.
type CalendarFields struct {
	Era     *string
	EraYear *int
	// Year is the calendar's arithmetic year.
	Year *int
	// Month is the month's position in the year, and MonthCode its code.
	Month     *int
	MonthCode *string
	Day       *int
}

// Int is a pointer to v, for a field of CalendarFields.
func Int(v int) *int { return &v }

// String is a pointer to v, for a field of CalendarFields.
func String(v string) *string { return &v }

// ParseMonthCode checks a month code's syntax as temporal_rs's
// MonthCode::try_from_utf8 does: "M", two digits, and "L" for a leap
// month. Whether the calendar has the month is a later question.
func ParseMonthCode(s string) error {
	switch {
	case len(s) != 3 && len(s) != 4:
		return rangeError("Month codes must have 3 or 4 characters.")
	case s[0] != 'M':
		return rangeError("First month code character must be 'M'.")
	case s[1] < '0' || s[1] > '9' || s[2] < '0' || s[2] > '9':
		return rangeError("Invalid month code digit.")
	case len(s) == 4 && s[3] != 'L':
		return rangeError("Leap month code must end with 'L'.")
	}
	return nil
}

// validMonthCode is MonthCode::validate: the codes a calendar can have at
// all, M01 to M12 everywhere, and M13, M05L or any leap month where the
// calendar has them.
func validMonthCode(calendar, code string) bool {
	m, ok := parseMonthCode(code)
	if !ok {
		return false
	}
	if !m.leap && m.number >= 1 && m.number <= 12 {
		return true
	}
	switch calendar {
	case "chinese", "dangi":
		return m.leap && m.number >= 1 && m.number <= 12
	case "coptic", "ethiopic", "ethioaa":
		return !m.leap && m.number == 13
	case "hebrew":
		return m.leap && m.number == 5
	}
	return false
}

// checkYearRange is check_year_in_safe_arithmetical_range: no calendar's
// years are more than a few thousand from the ISO year, so a year beyond
// ±300,000 is out of Temporal's range whatever the calendar.
func (f *CalendarFields) checkYearRange() error {
	if f.Year != nil && (*f.Year < -300000 || *f.Year >= 300000) {
		return rangeError("Date is not within ISO date time limits.")
	}
	if f.EraYear != nil && (*f.EraYear < -300000 || *f.EraYear >= 300000) {
		return rangeError("Date is not within ISO date time limits.")
	}
	return nil
}

// resolution is what an ISO date is resolved for: a date, a year and
// month, or a month and day, with or without a year.
type resolution int

const (
	resolveDate resolution = iota
	resolveYearMonth
	resolveMonthDay
	resolveMonthDayWithYear
)

// resolveISOFields is ResolvedIsoFields::try_from_fields.
func resolveISOFields(f *CalendarFields, overflow Overflow, kind resolution) (year, month, day int, err error) {
	if kind != resolveMonthDayWithYear {
		if err := f.checkYearRange(); err != nil {
			return 0, 0, 0, err
		}
	}
	year = 1972
	if kind != resolveMonthDay {
		if f.Year == nil {
			return 0, 0, 0, typeError("Required year field is empty.")
		}
		year = *f.Year
	}
	day = 1
	if kind != resolveYearMonth {
		if f.Day == nil {
			return 0, 0, 0, typeError("Required day field is empty.")
		}
		day = *f.Day
	}
	switch {
	case f.MonthCode != nil:
		// MonthCode::validate refuses every leap month in ISO, so its own
		// check for one after is never reached.
		if !validMonthCode("iso8601", *f.MonthCode) {
			return 0, 0, 0, rangeError("MonthCode was not valid for the current calendar.")
		}
		m, _ := parseMonthCode(*f.MonthCode)
		if f.Month != nil && *f.Month != m.number {
			return 0, 0, 0, rangeError("Month does not match monthCode.")
		}
		month = m.number
	case f.Month != nil:
		month = *f.Month
	default:
		return 0, 0, 0, typeError("Required month/monthCode field is empty.")
	}
	if month < 1 || month > 12 {
		if overflow == Reject {
			return 0, 0, 0, rangeError("Month out of range.")
		}
		if month > 12 {
			month = 12
		} else {
			month = 1
		}
	}
	n := gregorianMonthLength(year, month)
	if overflow == Constrain {
		day = clamp(day, 1, n)
	} else if day < 1 || day > n {
		return 0, 0, 0, rangeError("day value is not in a valid range.")
	}
	return year, month, day, nil
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
