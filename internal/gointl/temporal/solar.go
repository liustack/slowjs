package temporal

import "sort"

// The calendars whose years are arithmetic: the Gregorian ones, the Coptic
// and Ethiopian, the Indian, the Persian and the tabular Islamic, as
// icu_calendar 2.2.1 and calendrical_calculations 0.2.4 reckon them.

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

// gregorianLeap is calendrical_calculations::gregorian::is_leap_year.
func gregorianLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// gregorianDayBeforeYear is day_before_year: the Rata Die of the last day
// of the year before.
func gregorianDayBeforeYear(year int) int64 {
	p := int64(year) - 1
	return 365*p + floorDiv(p, 4) - floorDiv(p, 100) + floorDiv(p, 400)
}

// gregorianDaysBeforeMonth is days_before_month.
func gregorianDaysBeforeMonth(year, month int) int {
	if month < 3 {
		if month == 1 {
			return 0
		}
		return 31
	}
	leap := 0
	if gregorianLeap(year) {
		leap = 1
	}
	return 31 + 28 + leap + (979*month-2919)>>5
}

// gregorianFixed is fixed_from_gregorian.
func gregorianFixed(year, month, day int) int64 {
	return gregorianDayBeforeYear(year) + int64(gregorianDaysBeforeMonth(year, month)) + int64(day)
}

// gregorianYearFromFixed is year_from_fixed.
func gregorianYearFromFixed(rd int64) int {
	d := rd - 1
	n400, d := floorDiv(d, 146097), floorMod(d, 146097)
	n100, d := d/36524, d%36524
	n4, d := d/1461, d%1461
	n1 := d / 365
	year := 400*n400 + 100*n100 + 4*n4 + n1
	if n100 != 4 && n1 != 4 {
		year++
	}
	return int(year)
}

func gregorianFromFixed(rd int64) (year, month, day int) {
	year = gregorianYearFromFixed(rd)
	doy := int(rd - gregorianDayBeforeYear(year))
	month = 12
	for month > 1 && gregorianDaysBeforeMonth(year, month) >= doy {
		month--
	}
	return year, month, doy - gregorianDaysBeforeMonth(year, month)
}

func gregorianMonthLength(year, month int) int {
	switch month {
	case 2:
		if gregorianLeap(year) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// julianFixed is fixed_from_julian.
func julianFixed(year, month, day int) int64 {
	p := int64(year) - 1
	leap := floorMod(int64(year), 4) == 0
	before := int64((367*month - 362) / 12)
	if month > 2 {
		if leap {
			before--
		} else {
			before -= 2
		}
	}
	// The Julian calendar's epoch, its 0001-01-01, is 0000-12-30 in the
	// Gregorian, Rata Die -1.
	return -2 + 365*p + floorDiv(p, 4) + before + int64(day)
}

// gregorianRules are AbstractGregorian: ISO years, counted from an offset,
// with a calendar's eras.
type gregorianRules struct {
	solarMonths
	// offset is the ISO year less the calendar's.
	offset int
	eras   gregorianEras
}

type gregorianEras interface {
	eraYear(extended, month, day int) (string, int, bool)
	extendedFromEra(era string, year int) (int, bool)
}

func (r gregorianRules) year(extended int) calYear {
	iso := extended + r.offset
	y := calYear{extended: extended, start: gregorianFixed(iso, 1, 1), count: 12, leapYear: gregorianLeap(iso)}
	for m := 1; m <= 12; m++ {
		y.lengths[m-1] = uint8(gregorianMonthLength(iso, m))
	}
	return y
}

func (r gregorianRules) yearOfRD(rd int64) calYear {
	return r.year(gregorianYearFromFixed(rd) - r.offset)
}

func (r gregorianRules) eraYear(y calYear, month, day int) (string, int, bool) {
	return r.eras.eraYear(y.extended, month, day)
}

func (r gregorianRules) extendedFromEra(era string, year int) (int, bool) {
	return r.eras.extendedFromEra(era, year)
}

// referenceYear is ISO 1972 for every month and day; a month the calendar
// does not have is found out later.
func (r gregorianRules) referenceYear(m month, day int) (int, errReference) {
	return 1972 - r.offset, referenceOK
}

type isoEraRules struct{}

var isoEras isoEraRules

// The ISO calendar's era is ICU4X's "default", which Temporal does not
// report.
func (isoEraRules) eraYear(extended, month, day int) (string, int, bool) { return "", 0, false }
func (isoEraRules) extendedFromEra(era string, year int) (int, bool) {
	if era == "default" {
		return year, true
	}
	return 0, false
}

type ceBCERules struct{}

var ceBCE ceBCERules

func (ceBCERules) eraYear(extended, month, day int) (string, int, bool) {
	if extended > 0 {
		return "ce", extended, true
	}
	return "bce", 1 - extended, true
}

func (ceBCERules) extendedFromEra(era string, year int) (int, bool) {
	switch era {
	case "ad", "ce":
		return year, true
	case "bce", "bc":
		return 1 - year, true
	}
	return 0, false
}

type buddhistEraRules struct{}

var buddhistEras buddhistEraRules

func (buddhistEraRules) eraYear(extended, month, day int) (string, int, bool) {
	return "be", extended, true
}

func (buddhistEraRules) extendedFromEra(era string, year int) (int, bool) {
	return year, era == "be"
}

type rocEraRules struct{}

var rocEras rocEraRules

func (rocEraRules) eraYear(extended, month, day int) (string, int, bool) {
	if extended > 0 {
		return "roc", extended, true
	}
	return "broc", 1 - extended, true
}

func (rocEraRules) extendedFromEra(era string, year int) (int, bool) {
	switch era {
	case "roc":
		return year, true
	case "broc":
		return 1 - year, true
	}
	return 0, false
}

// japaneseEraRules are ICU4X's Japanese eras, the modern ones it builds in,
// latest first. Before Meiji 6, when Japan took up the Gregorian calendar,
// and before Meiji, a year is counted CE or BCE.
type japaneseEraRules struct{}

var japaneseEras japaneseEraRules

var japaneseEraStarts = []struct {
	year, month, day int
	name             string
}{
	{2019, 5, 1, "reiwa"},
	{1989, 1, 8, "heisei"},
	{1926, 12, 25, "showa"},
	{1912, 7, 30, "taisho"},
	{1868, 10, 23, "meiji"},
}

func (japaneseEraRules) eraYear(extended, month, day int) (string, int, bool) {
	for _, e := range japaneseEraStarts {
		if extended > e.year || extended == e.year && (month > e.month || month == e.month && day >= e.day) {
			if e.name == "meiji" && extended-e.year+1 < 6 {
				break
			}
			return e.name, extended - e.year + 1, true
		}
	}
	return ceBCE.eraYear(extended, month, day)
}

func (japaneseEraRules) extendedFromEra(era string, year int) (int, bool) {
	if y, ok := ceBCE.extendedFromEra(era, year); ok {
		return y, true
	}
	for _, e := range japaneseEraStarts {
		if e.name == era {
			return year - 1 + e.year, true
		}
	}
	return 0, false
}

// The Coptic calendar's epoch, and the Ethiopian eras' years less the
// Coptic.
var copticEpoch = julianFixed(284, 8, 29)

const (
	copticYears = 0
	ameteMihret = -276
	ameteAlem   = -5776
)

// copticRules are the Coptic calendar and the Ethiopian, which is the
// Coptic counted from another year: twelve months of thirty days and five
// or six days over.
type copticRules struct {
	solarMonths
	// style is the Coptic year less the calendar's.
	style int
}

func copticLeap(year int) bool { return floorMod(int64(year)+1, 4) == 0 }

func copticFixed(year, month, day int) int64 {
	return copticEpoch - 1 + 365*(int64(year)-1) + floorDiv(int64(year), 4) + 30*int64(month-1) + int64(day)
}

func (r copticRules) year(extended int) calYear {
	coptic := extended + r.style
	y := calYear{extended: extended, start: copticFixed(coptic, 1, 1), count: 13, leapYear: copticLeap(coptic)}
	for m := 0; m < 12; m++ {
		y.lengths[m] = 30
	}
	y.lengths[12] = 5
	if y.leapYear {
		y.lengths[12] = 6
	}
	return y
}

func (r copticRules) yearOfRD(rd int64) calYear {
	coptic := int(floorDiv(4*(rd-copticEpoch)+1463, 1461))
	return r.year(coptic - r.style)
}

func (copticRules) minMonthsFrom(y calYear, years int) int { return 13 * years }

func (r copticRules) eraYear(y calYear, month, day int) (string, int, bool) {
	coptic := y.extended + r.style
	switch {
	case r.style == copticYears:
		return "am", y.extended, true
	case r.style == ameteAlem || y.extended <= 0:
		return "aa", coptic - ameteAlem, true
	}
	return "am", coptic - ameteMihret, true
}

func (r copticRules) extendedFromEra(era string, year int) (int, bool) {
	switch {
	case r.style == copticYears && era == "am":
		return year, true
	case r.style == ameteMihret && era == "am":
		return year, true
	case r.style == ameteMihret && era == "aa":
		return year - ameteMihret + ameteAlem, true
	case r.style == ameteAlem && era == "aa":
		return year, true
	}
	return 0, false
}

// referenceYear is Coptic::reference_year_from_month_day, in the
// calendar's years.
func (r copticRules) referenceYear(m month, day int) (int, errReference) {
	if m.leap {
		return 0, referenceNotInCalendar
	}
	coptic := 1688
	switch {
	case m.number < 4 || m.number == 4 && day <= 22:
		coptic = 1689
	case m.number == 13 && day >= 6:
		coptic = 1687
	}
	return coptic - r.style, referenceOK
}

// indianRules are the Indian national calendar: its year starts on the
// eighty-first day of the ISO year 78 later.
type indianRules struct{ solarMonths }

const (
	indianDayOffset  = 80
	indianYearOffset = 78
)

func (indianRules) year(extended int) calYear {
	leap := gregorianLeap(extended + indianYearOffset)
	y := calYear{extended: extended, start: gregorianDayBeforeYear(extended+indianYearOffset) + indianDayOffset + 1,
		count: 12, leapYear: leap}
	for m := 1; m <= 12; m++ {
		n := 30
		if m <= 6 {
			n++
		}
		if m == 1 && !leap {
			n--
		}
		y.lengths[m-1] = uint8(n)
	}
	return y
}

func (r indianRules) yearOfRD(rd int64) calYear {
	iso := gregorianYearFromFixed(rd)
	if rd-gregorianDayBeforeYear(iso) <= indianDayOffset {
		return r.year(iso - indianYearOffset - 1)
	}
	return r.year(iso - indianYearOffset)
}

func (indianRules) eraYear(y calYear, month, day int) (string, int, bool) {
	return "shaka", y.extended, true
}

func (indianRules) extendedFromEra(era string, year int) (int, bool) { return year, era == "shaka" }

func (indianRules) referenceYear(m month, day int) (int, errReference) {
	if m.leap {
		return 0, referenceNotInCalendar
	}
	if m.number < 10 || m.number == 10 && day <= 10 {
		return 1894, referenceOK
	}
	return 1893, referenceOK
}

// The Persian calendar, as calendrical_calculations' "fast" Persian: a
// 33-year cycle corrected, year by year, to the astronomical calendar where
// the two part.
var persianEpoch = julianFixed(622, 3, 19)

var persianNonLeapCorrection = []int{
	1502, 1601, 1634, 1667, 1700, 1733, 1766, 1799, 1832, 1865, 1898, 1931, 1964, 1997, 2030, 2059,
	2063, 2096, 2129, 2158, 2162, 2191, 2195, 2224, 2228, 2257, 2261, 2290, 2294, 2323, 2327, 2356,
	2360, 2389, 2393, 2422, 2426, 2455, 2459, 2488, 2492, 2521, 2525, 2554, 2558, 2587, 2591, 2620,
	2624, 2653, 2657, 2686, 2690, 2719, 2723, 2748, 2752, 2756, 2781, 2785, 2789, 2818, 2822, 2847,
	2851, 2855, 2880, 2884, 2888, 2913, 2917, 2921, 2946, 2950, 2954, 2979, 2983, 2987,
}

func persianCorrected(year int) bool {
	if year < persianNonLeapCorrection[0] {
		return false
	}
	i := sort.SearchInts(persianNonLeapCorrection, year)
	return i < len(persianNonLeapCorrection) && persianNonLeapCorrection[i] == year
}

// persianLeap is calendrical_calculations::persian::is_leap_year.
func persianLeap(year int) bool {
	switch {
	case persianCorrected(year):
		return false
	case year > persianNonLeapCorrection[0] && persianCorrected(year-1):
		return true
	}
	return floorMod(25*int64(year)+11, 33) < 8
}

// persianNewYear is fixed_from_fast_persian(year, 1, 1).
func persianNewYear(year int) int64 {
	y := int64(year)
	ny := persianEpoch - 1 + 365*(y-1) + floorDiv(8*y+21, 33)
	if year > persianNonLeapCorrection[0] && persianCorrected(year-1) {
		ny--
	}
	return ny
}

type persianRules struct{ solarMonths }

func (persianRules) year(extended int) calYear {
	y := calYear{extended: extended, start: persianNewYear(extended), count: 12, leapYear: persianLeap(extended)}
	for m := 1; m <= 12; m++ {
		n := 30
		if m <= 6 {
			n = 31
		}
		if m == 12 && !y.leapYear {
			n = 29
		}
		y.lengths[m-1] = uint8(n)
	}
	return y
}

// yearOfRD is fast_persian_from_fixed's year: the 33-year estimate, and the
// next year for a day the correction moves into it.
func (r persianRules) yearOfRD(rd int64) calYear {
	year := int(1 + floorDiv(33*(rd-persianEpoch+1)+3, 12053))
	if rd-persianNewYear(year) == 365 && persianCorrected(year) {
		year++
	}
	return r.year(year)
}

func (persianRules) eraYear(y calYear, month, day int) (string, int, bool) {
	return "ap", y.extended, true
}

func (persianRules) extendedFromEra(era string, year int) (int, bool) { return year, era == "ap" }

func (persianRules) referenceYear(m month, day int) (int, errReference) {
	if m.leap {
		return 0, referenceNotInCalendar
	}
	if m.number < 10 || m.number == 10 && day <= 10 {
		return 1351, referenceOK
	}
	return 1350, referenceOK
}

// The Islamic calendars' epochs: the Hijra, reckoned from a Friday for the
// civil calendar and Umm al-Qura, and from the Thursday before for the
// astronomical tabular one.
var (
	islamicEpochFriday   = julianFixed(622, 7, 16)
	islamicEpochThursday = julianFixed(622, 7, 15)
)

// tabularIslamicFixed is fixed_from_tabular_islamic.
func tabularIslamicFixed(year, month, day int, epoch int64) int64 {
	y, m := int64(year), int64(month)
	return epoch - 1 + (y-1)*354 + floorDiv(3+y*11, 30) + 29*(m-1) + floorDiv(m, 2) + int64(day)
}

// tabularYearFromFixed is tabular_year_from_fixed, whose division truncates.
func tabularYearFromFixed(rd, epoch int64) int {
	y := (rd - epoch) * 30 / (354*30 + 11)
	if rd >= epoch {
		y++
	}
	return int(y)
}

// hijriRules are ICU4X's Hijri calendars: the tabular ones, of Type II leap
// years from either epoch, and Umm al-Qura, which reads a table and is the
// civil calendar outside it.
type hijriRules struct {
	solarMonths
	epoch     int64
	table     []tableYear
	reference func(m month, day int) (int, errReference)
}

func (r hijriRules) year(extended int) calYear {
	if t, ok := lookup(r.table, extended); ok {
		y := calYear{extended: extended, start: t.start, count: 12}
		long := 0
		for m := 0; m < 12; m++ {
			y.lengths[m] = 29
			if t.long&(1<<m) != 0 {
				y.lengths[m] = 30
				long++
			}
		}
		y.leapYear = long == 7
		return y
	}
	y := calYear{extended: extended, start: tabularIslamicFixed(extended, 1, 1, r.epoch), count: 12}
	long := 0
	for m := 0; m < 12; m++ {
		y.lengths[m] = 29
		if m%2 == 0 || m == 11 && floorMod(14+11*int64(extended), 30) < 11 {
			y.lengths[m] = 30
			long++
		}
	}
	y.leapYear = long == 7
	return y
}

// yearOfRD is Rules::year_containing_rd: the tabular year of five days
// before, or the year after it.
func (r hijriRules) yearOfRD(rd int64) calYear {
	y := r.year(tabularYearFromFixed(rd-5, islamicEpochFriday))
	if rd >= y.start+int64(y.daysInYear()) {
		y = r.year(y.extended + 1)
	}
	return y
}

func (hijriRules) eraYear(y calYear, month, day int) (string, int, bool) {
	if y.extended > 0 {
		return "ah", y.extended, true
	}
	return "bh", 1 - y.extended, true
}

func (hijriRules) extendedFromEra(era string, year int) (int, bool) {
	switch era {
	case "ah":
		return year, true
	case "bh":
		return 1 - year, true
	}
	return 0, false
}

func (r hijriRules) referenceYear(m month, day int) (int, errReference) { return r.reference(m, day) }

func civilReference(m month, day int) (int, errReference) { return tabularReference(m, day, 26) }
func tblaReference(m month, day int) (int, errReference)  { return tabularReference(m, day, 27) }

// tabularReference is TabularAlgorithm::ecma_reference_year: 1392, but for
// the end of the eleventh month and the twelfth.
func tabularReference(m month, day, lastOf1392 int) (int, errReference) {
	if m.leap || m.number < 1 || m.number > 12 {
		return 0, referenceNotInCalendar
	}
	switch {
	case m.number <= 10:
		return 1392, referenceOK
	case m.number == 11 && day < lastOf1392:
		return 1392, referenceOK
	case m.number == 11:
		return 1391, referenceOK
	case day >= 30:
		return 1390, referenceOK
	}
	return 1391, referenceOK
}

// umalquraReference is UmmAlQura::ecma_reference_year.
func umalquraReference(m month, day int) (int, errReference) {
	if m.leap {
		return 0, referenceNotInCalendar
	}
	switch m.number {
	case 1, 4, 6, 8, 9:
		return 1392, referenceOK
	case 2, 10:
		if day >= 30 {
			return 1390, referenceOK
		}
		return 1392, referenceOK
	case 3, 5:
		if day >= 30 {
			return 1391, referenceOK
		}
		return 1392, referenceOK
	case 7:
		if day >= 30 {
			return 1389, referenceOK
		}
		return 1392, referenceOK
	case 11:
		if day < 26 {
			return 1392, referenceOK
		}
		return 1391, referenceOK
	case 12:
		if day >= 30 {
			return 1390, referenceOK
		}
		return 1391, referenceOK
	}
	return 0, referenceNotInCalendar
}
