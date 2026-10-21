package temporal

// Dates from fields, and date arithmetic, in every calendar but the ISO, as
// icu_calendar 2.2.1's ArithmeticDate does them.

// The year ranges ArithmeticDate checks: a generous range of years that is
// wider than any calendar's valid days, checked early, and the valid days,
// ±999,999 years about the ISO epoch, checked at the end.
const (
	generousYearMin   = -1040000
	generousYearMax   = 1040000
	generousMaxYears  = generousYearMax - generousYearMin
	generousMaxMonths = generousMaxYears * 13
	generousMaxDays   = generousMaxMonths * 31
)

var (
	validRDMin = gregorianFixed(-999999, 1, 1)
	validRDMax = gregorianFixed(999999, 12, 31)
)

// icuFields are ICU4X's DateFields as temporal_rs fills them.
type icuFields struct {
	era          *string
	eraYear      *int
	extendedYear *int
	monthCode    *string
	ordinalMonth *int
	day          *int
}

func toICUFields(f *CalendarFields) icuFields {
	return icuFields{era: f.Era, eraYear: f.EraYear, extendedYear: f.Year, monthCode: f.MonthCode,
		ordinalMonth: f.Month, day: f.Day}
}

// ICU4X's missing-fields strategies: reject a date without a year or a
// day, or take ECMA-402's reference year and day 1.
const (
	missingReject = iota
	missingECMA
)

// An arithDate is ArithmeticDate: a year and a month and day in it.
type arithDate struct {
	y          calYear
	month, day int
}

func (d arithDate) rataDie() int64 { return d.y.rataDie(d.month, d.day) }

// fromFields is ArithmeticDate::from_fields.
func (c *Calendar) fromFields(f icuFields, overflow Overflow, missing int) (arithDate, error) {
	notEnough := typeError("Insufficient fields.")
	day := 0
	switch {
	case f.day != nil:
		day = *f.day
	case missing == missingECMA && (f.extendedYear != nil || f.eraYear != nil):
		day = 1
	default:
		return arithDate{}, notEnough
	}
	if f.monthCode == nil && f.ordinalMonth == nil {
		return arithDate{}, notEnough
	}
	var validMonth *month
	parseCode := func() (month, error) {
		if validMonth != nil {
			return *validMonth, nil
		}
		m, ok := parseMonthCode(*f.monthCode)
		if !ok {
			return month{}, rangeError("Invalid month code.")
		}
		return m, nil
	}
	var y calYear
	switch {
	case f.era == nil && f.eraYear == nil:
		switch {
		case f.extendedYear != nil:
			if *f.extendedYear < generousYearMin || *f.extendedYear > generousYearMax {
				return arithDate{}, rangeError("Date error.")
			}
			y = c.r.year(*f.extendedYear)
		case missing == missingReject:
			return arithDate{}, notEnough
		default:
			if f.monthCode == nil || f.ordinalMonth != nil {
				return arithDate{}, notEnough
			}
			m, err := parseCode()
			if err != nil {
				return arithDate{}, err
			}
			validMonth = &m
			ref, refErr := c.r.referenceYear(m, day)
			if refErr == referenceUseRegularIfConstrain && overflow == Constrain {
				regular := month{number: m.number}
				validMonth = &regular
				ref, refErr = c.r.referenceYear(regular, day)
			}
			if err := referenceError(refErr); err != nil {
				return arithDate{}, err
			}
			y = c.r.year(ref)
		}
	case f.era != nil && f.eraYear != nil:
		if *f.eraYear < generousYearMin || *f.eraYear > generousYearMax {
			return arithDate{}, rangeError("Date error.")
		}
		extended, ok := c.r.extendedFromEra(*f.era, *f.eraYear)
		if !ok {
			return arithDate{}, rangeError("Unknown era.")
		}
		y = c.r.year(extended)
		if f.extendedYear != nil && *f.extendedYear != y.extended {
			return arithDate{}, rangeError("Inconsistent year.")
		}
	default:
		// An era and an era year come together or not at all.
		return arithDate{}, notEnough
	}
	var ordinal int
	if f.monthCode != nil {
		m, err := parseCode()
		if err != nil {
			return arithDate{}, err
		}
		computed, merr := c.r.ordinalFromMonth(y, m, overflow == Constrain)
		if err := monthError(merr); err != nil {
			return arithDate{}, err
		}
		if f.ordinalMonth != nil && *f.ordinalMonth != computed {
			return arithDate{}, rangeError("Inconsistent month/monthCode.")
		}
		ordinal = computed
	} else {
		ordinal = *f.ordinalMonth
	}
	if overflow == Constrain {
		ordinal = clamp(ordinal, 1, y.count)
	} else if ordinal < 1 || ordinal > y.count {
		return arithDate{}, rangeError("Month out of range")
	}
	n := y.monthLength(ordinal)
	if overflow == Constrain {
		day = clamp(day, 1, n)
	} else if day < 1 || day > n {
		return arithDate{}, rangeError("Day out of range")
	}
	d := arithDate{y, ordinal, day}
	if rd := d.rataDie(); rd < validRDMin || rd > validRDMax {
		return arithDate{}, rangeError("Date error.")
	}
	return d, nil
}

func monthError(e errMonth) error {
	switch e {
	case monthNotInCalendar:
		return rangeError("Month code not in calendar.")
	case monthNotInYear:
		return rangeError("Month code not in year.")
	}
	return nil
}

func referenceError(e errReference) error {
	switch e {
	case referenceNotInCalendar:
		return rangeError("Month code not in calendar.")
	case referenceUseRegularIfConstrain:
		return rangeError("Month code not in year.")
	}
	return nil
}

// balance is ArithmeticDate::new_balanced, BalanceNonISODate: excess
// months carried into the year, and excess days into the month.
func (c *Calendar) balance(y calYear, ordinal, day int) arithDate {
	for ordinal <= 0 {
		y = c.r.year(y.extended - 1)
		ordinal += y.count
	}
	for ordinal > y.count {
		ordinal -= y.count
		y = c.r.year(y.extended + 1)
	}
	for day <= 0 {
		ordinal--
		if ordinal == 0 {
			y = c.r.year(y.extended - 1)
			ordinal = y.count
		}
		day += y.monthLength(ordinal)
	}
	for day > y.monthLength(ordinal) {
		day -= y.monthLength(ordinal)
		ordinal++
		if ordinal > y.count {
			y = c.r.year(y.extended + 1)
			ordinal = 1
		}
	}
	return arithDate{y, ordinal, day}
}

// An icuDuration is ICU4X's DateDuration: a sign and magnitudes.
type icuDuration struct {
	negative                   bool
	years, months, weeks, days int64
}

func (d icuDuration) addYearsTo(year int) int {
	if d.negative {
		return year - int(d.years)
	}
	return year + int(d.years)
}

func (d icuDuration) addMonthsTo(month int) int {
	if d.negative {
		return month - int(d.months)
	}
	return month + int(d.months)
}

func (d icuDuration) addWeeksAndDaysTo(day int) int {
	if d.negative {
		return day - int(d.weeks)*7 - int(d.days)
	}
	return day + int(d.weeks)*7 + int(d.days)
}

// added is ArithmeticDate::added, NonISODateAdd.
func (c *Calendar) added(d arithDate, dur icuDuration, overflow Overflow) (arithDate, error) {
	if dur.years > generousMaxYears || dur.months > generousMaxMonths ||
		dur.weeks*7+dur.days > generousMaxDays {
		return arithDate{}, rangeError("Overflow during addition.")
	}
	extended := dur.addYearsTo(d.y.extended)
	if extended < generousYearMin || extended > generousYearMax {
		return arithDate{}, rangeError("Overflow during addition.")
	}
	y0 := c.r.year(extended)
	base := c.r.monthFromOrdinal(d.y, d.month)
	m0, merr := c.r.ordinalFromMonth(y0, base, overflow == Constrain)
	if merr != monthOK {
		return arithDate{}, rangeError("Month code not in year.")
	}
	endOfMonth := c.balance(y0, dur.addMonthsTo(m0)+1, 0)
	regulated := d.day
	if d.day > endOfMonth.day {
		if overflow == Reject {
			return arithDate{}, rangeError("Day out of range")
		}
		regulated = endOfMonth.day
	}
	out := c.balance(endOfMonth.y, endOfMonth.month, dur.addWeeksAndDaysTo(regulated))
	if rd := out.rataDie(); rd < validRDMin || rd > validRDMax {
		return arithDate{}, rangeError("Overflow during addition.")
	}
	return out, nil
}

// compare is the order of two dates of one calendar.
func (d arithDate) compare(o arithDate) int {
	switch {
	case d.y.extended != o.y.extended:
		return sign(d.y.extended - o.y.extended)
	case d.month != o.month:
		return sign(d.month - o.month)
	}
	return sign(d.day - o.day)
}

// until is ArithmeticDate::until, NonISODateUntil.
func (c *Calendar) until(one, two arithDate, largest Unit) icuDuration {
	if largest == Day || largest == Week {
		diff := two.rataDie() - one.rataDie()
		out := icuDuration{negative: diff < 0}
		if largest == Week {
			out.weeks, out.days = abs64(diff/7), abs64(diff%7)
		} else {
			out.days = abs64(diff)
		}
		return out
	}
	sgn := two.compare(one)
	if sgn == 0 {
		return icuDuration{}
	}
	s := surpassesChecker{c: c, parts: one, target: two, sign: sgn}
	yearDiff := two.y.extended - one.y.extended
	minYears := 0
	if yearDiff != 0 {
		minYears = yearDiff - sgn
	}
	years := 0
	if largest == Year {
		candidate := sgn
		if minYears != 0 {
			candidate = minYears
		}
		for !s.surpassesYears(candidate) {
			years = candidate
			candidate += sgn
		}
	}
	s.surpassesYears(years)
	months := 0
	if largest == Year || largest == Month {
		candidate := sgn
		if largest == Month && minYears != 0 {
			candidate = c.r.minMonthsFrom(one.y, minYears)
		}
		for !s.surpassesMonths(candidate) {
			months = candidate
			candidate += sgn
		}
	}
	s.setMonths(months)
	days := 0
	candidate := sgn
	for !s.surpassesDays(candidate) {
		days = candidate
		candidate += sgn
	}
	return icuDuration{negative: sgn < 0, years: abs64(int64(years)), months: abs64(int64(months)),
		days: abs64(int64(days))}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// surpassesChecker is ICU4X's SurpassesChecker, NonISODateSurpasses kept
// between one field and the next.
type surpassesChecker struct {
	c            *Calendar
	parts        arithDate
	target       arithDate
	sign         int
	y0           calYear
	m0           int
	endOfMonth   arithDate
	regulatedDay int
	// lastMonths and last are the months surpassesMonths last balanced and
	// where they came to, which the next, a month on, starts from rather
	// than walking the years from y0 again: until in months over thousands
	// of Chinese years had been quadratic.
	lastMonths int
	last       arithDate
	haveLast   bool
}

func (s *surpassesChecker) surpassesYears(years int) bool {
	s.y0 = s.c.r.year(s.parts.y.extended + years)
	base := s.c.r.monthFromOrdinal(s.parts.y, s.parts.month)
	surpasses := s.compareLexicographic(s.y0, base, s.parts.day)
	m0, merr := s.c.r.ordinalFromMonth(s.y0, base, true)
	if merr != monthOK {
		m0 = 1
	}
	s.m0 = m0
	s.haveLast = false
	return surpasses || s.surpassesMonths(0)
}

func (s *surpassesChecker) surpassesMonths(months int) bool {
	added := s.monthsOn(months)
	return s.compareOrdinal(added.y, added.month, s.parts.day)
}

// monthsOn is the first of the month months after y0's m0: balance(y0,
// months+m0, 1), balanced from the month last asked for, which gives the
// same day, a month's first day being reached from any other by its
// months alone.
func (s *surpassesChecker) monthsOn(months int) arithDate {
	var added arithDate
	if s.haveLast {
		added = s.c.balance(s.last.y, s.last.month+months-s.lastMonths, 1)
	} else {
		added = s.c.balance(s.y0, months+s.m0, 1)
	}
	s.lastMonths, s.last, s.haveLast = months, added, true
	return added
}

func (s *surpassesChecker) setMonths(months int) {
	added := s.monthsOn(months)
	s.endOfMonth = s.c.balance(added.y, added.month+1, 0)
	s.regulatedDay = s.parts.day
	if s.parts.day >= s.endOfMonth.day {
		s.regulatedDay = s.endOfMonth.day
	}
}

func (s *surpassesChecker) surpassesDays(days int) bool {
	if days == 0 {
		return false
	}
	d := s.c.balance(s.endOfMonth.y, s.endOfMonth.month, days+s.regulatedDay)
	return s.compareOrdinal(d.y, d.month, d.day)
}

// compareLexicographic is CompareSurpasses by month code.
func (s *surpassesChecker) compareLexicographic(y calYear, m month, day int) bool {
	t := s.target
	if y.extended != t.y.extended {
		return s.sign*(y.extended-t.y.extended) > 0
	}
	tm := s.c.r.monthFromOrdinal(t.y, t.month)
	if m != tm {
		if s.sign > 0 {
			return tm.less(m)
		}
		return !tm.less(m)
	}
	if day != t.day {
		return s.sign*(day-t.day) > 0
	}
	return false
}

// compareOrdinal is CompareSurpasses by ordinal month.
func (s *surpassesChecker) compareOrdinal(y calYear, month, day int) bool {
	t := s.target
	switch {
	case y.extended != t.y.extended:
		return s.sign*(y.extended-t.y.extended) > 0
	case month != t.month:
		return s.sign*(month-t.month) > 0
	case day != t.day:
		return s.sign*(day-t.day) > 0
	}
	return false
}
