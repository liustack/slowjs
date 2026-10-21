package temporal

// Temporal's ISO time and date-time records, as temporal_rs 0.2.3's iso
// module has them, with its integer widths: a year is an i32 and wraps as
// one.

// An ISOTime is a time of day: hour 0 to 23, minute and second 0 to 59,
// and the millisecond, microsecond and nanosecond 0 to 999.
type ISOTime struct {
	Hour, Minute, Second, Millisecond, Microsecond, Nanosecond int
}

// An ISODateTime is a date and a time of day.
type ISODateTime struct {
	Date ISODate
	Time ISOTime
}

// noon is IsoTime::noon.
var noon = ISOTime{Hour: 12}

var unixEpoch = ISODate{1970, 1, 1}

// maxEpochDaysLimit is MAX_EPOCH_DAYS, 10^8 + 1.
const maxEpochDaysLimit = 100_000_001

// nsMaxInstant is the latest instant, 10^8 days after 1970.
var nsMaxInstant = i128(nsPerDay).mul64(100_000_000)
var nsMinInstant = nsMaxInstant.neg()

// Neri and Schneider's Gregorian arithmetic, as timezone_provider's utils
// compute it.

const (
	epochComputationalRataDie = 719_468
	daysIn400Years            = 146_097
	shiftConstant             = 3670
	shiftConstantExtended     = 5_368_710
)

// epochDaysFromGregorian is epoch_days_from_gregorian_date: days from 1970
// of a date whose month may be 0, which is the December before.
func epochDaysFromGregorian(year int32, month, day uint8) int64 {
	shift := int64(shiftConstantExtended)*daysIn400Years + epochComputationalRataDie
	j := int64(0)
	if month <= 2 {
		j = 1
	}
	compYear := uint64(int64(year) + 400*shiftConstantExtended - j)
	compMonth := int64(month) + 12*j
	compDay := int64(day) - 1
	century := compYear / 100
	yStar := 1461*compYear/4 - century + century/4
	mStar := (979*compMonth - 2919) / 32
	return int64(yStar) + mStar + compDay - shift
}

// ymdFromEpochDays is ymd_from_epoch_days, in u32 arithmetic.
func ymdFromEpochDays(epochDays int32) (int32, uint8, uint8) {
	rd := uint32(epochDays + epochComputationalRataDie + daysIn400Years*shiftConstant)
	n1 := 4*rd + 3
	century := n1 / daysIn400Years
	rem := n1 % daysIn400Years
	n2 := uint64(rem | 3)
	p2 := 2_939_745 * n2
	yearOfCentury := uint32(p2 / (1 << 32))
	dayOfYear := uint32(p2 % (1 << 32) / 2_939_745 / 4)
	year := 100*century + yearOfCentury
	n3 := 2141*dayOfYear + 197_913
	month := n3 / (1 << 16)
	day := n3 % (1 << 16) / 2141
	if dayOfYear >= 306 {
		year++
		month -= 12
	}
	return int32(year) - 400*shiftConstant, uint8(month), uint8(day + 1)
}

// ymdFromEpochMilliseconds is ymd_from_epoch_milliseconds.
func ymdFromEpochMilliseconds(ms int64) (int32, uint8, uint8) {
	return ymdFromEpochDays(int32(floorDiv(ms, msPerDay)))
}

// isoDateToEpochDays is iso_date_to_epoch_days, month counted from 1 and
// allowed to run past 12.
func isoDateToEpochDays(year, month, day int32) int64 {
	resolvedYear := year + int32(floorDiv(int64(month), 12))
	resolvedMonth := uint8(floorMod(int64(month), 12))
	return epochDaysFromGregorian(resolvedYear, resolvedMonth, 1) + int64(day) - 1
}

func (d ISODate) epochDays() int64 {
	return epochDaysFromGregorian(int32(d.Year), uint8(d.Month), uint8(d.Day))
}

// balanceISODate is IsoDate::balance.
func balanceISODate(year, month, day int32) ISODate {
	days := isoDateToEpochDays(year, month, day)
	y, m, dd := ymdFromEpochMilliseconds(days * msPerDay)
	return ISODate{int(y), int(m), int(dd)}
}

// tryBalanceISODate is IsoDate::try_balance.
func tryBalanceISODate(year, month int32, day int64) (ISODate, error) {
	days := isoDateToEpochDays(year, month, 1) + day - 1
	if days > maxEpochDaysLimit || days < -maxEpochDaysLimit {
		return ISODate{}, rangeError("epoch days exceed maximum range.")
	}
	y, m, d := ymdFromEpochMilliseconds(days * msPerDay)
	return ISODate{int(y), int(m), int(d)}, nil
}

// checkWithinLimits is IsoDate::check_within_limits: the date's noon is
// within a day of Temporal's instants.
func (d ISODate) checkWithinLimits() error {
	if !isoDateTimeWithinLimits(d, noon) {
		return rangeError("Date is not within ISO date time limits.")
	}
	return nil
}

// checkDayRange is IsoDate::is_valid_day_range.
func (d ISODate) checkDayRange() error {
	if days := d.epochDays(); days > 100_000_000 || days < -100_000_000 {
		return rangeError("Not in a valid ISO day range.")
	}
	return nil
}

// isoDateTimeWithinLimits is iso_dt_within_valid_limits.
func isoDateTimeWithinLimits(d ISODate, t ISOTime) bool {
	if days := d.epochDays(); days > maxEpochDaysLimit || days < -maxEpochDaysLimit {
		return false
	}
	ns := epochNanoseconds(d, t)
	max := nsMaxInstant.add(i128(nsPerDay))
	min := nsMinInstant.sub(i128(nsPerDay))
	return min.cmp(ns) < 0 && max.cmp(ns) > 0
}

// epochNanoseconds is to_unchecked_epoch_nanoseconds: the date and time
// read as UTC.
func epochNanoseconds(d ISODate, t ISOTime) int128 {
	ms := d.epochDays()*msPerDay + t.epochMilliseconds()
	return i128(ms).mul64(1_000_000).add(i128(int64(t.Microsecond)*1000 + int64(t.Nanosecond)))
}

func (t ISOTime) epochMilliseconds() int64 {
	return int64(t.Hour)*3_600_000 + int64(t.Minute)*60_000 + int64(t.Second)*1000 + int64(t.Millisecond)
}

// newISODateTime is IsoDateTime::new: the date and time, within Temporal's
// range.
func newISODateTime(d ISODate, t ISOTime) (ISODateTime, error) {
	if !isoDateTimeWithinLimits(d, t) {
		return ISODateTime{}, rangeError("IsoDateTime not within a valid range.")
	}
	return ISODateTime{d, t}, nil
}

func (dt ISODateTime) withinLimits() bool { return isoDateTimeWithinLimits(dt.Date, dt.Time) }

func (dt ISODateTime) checkWithinLimits() error {
	if !dt.withinLimits() {
		return rangeError("IsoDateTime not within a valid range.")
	}
	return nil
}

// epochNanoseconds is the date and time read as UTC.
func (dt ISODateTime) epochNanoseconds() int128 { return epochNanoseconds(dt.Date, dt.Time) }

func (dt ISODateTime) compare(o ISODateTime) int {
	if c := dt.Date.compare(o.Date); c != 0 {
		return c
	}
	return dt.Time.compare(o.Time)
}

func (t ISOTime) compare(o ISOTime) int {
	for _, p := range [...][2]int{{t.Hour, o.Hour}, {t.Minute, o.Minute}, {t.Second, o.Second},
		{t.Millisecond, o.Millisecond}, {t.Microsecond, o.Microsecond}, {t.Nanosecond, o.Nanosecond}} {
		if p[0] != p[1] {
			return sign(p[0] - p[1])
		}
	}
	return 0
}

// isoDateTimeFromEpochNanoseconds is IsoDateTime::from_epoch_nanos: the
// wall time of an instant at an offset in nanoseconds.
func isoDateTimeFromEpochNanoseconds(ns int128, offset int64) ISODateTime {
	million := i128(1_000_000)
	remainder := ns.remEuclid(million)
	ms := ns.sub(remainder).divEuclid(million).int64()
	y, m, d := ymdFromEpochMilliseconds(ms)
	hour := floorMod(floorDiv(ms, 3_600_000), 24)
	minute := floorMod(floorDiv(ms, 60_000), 60)
	second := floorMod(floorDiv(ms, 1000), 60)
	millis := floorMod(ms, 1000)
	r := remainder.int64()
	return balanceISODateTime(y, int32(m), int32(d), hour, minute, second, millis, i128(r/1000), i128(r%1000+offset))
}

// balanceISODateTime is IsoDateTime::balance.
func balanceISODateTime(year, month, day int32, hour, minute, second, millisecond int64, microsecond, nanosecond int128) ISODateTime {
	days, t := balanceISOTime(hour, minute, second, millisecond, microsecond, nanosecond)
	return ISODateTime{balanceISODate(year, month, day+int32(days)), t}
}

// balanceISOTime is IsoTime::balance: the days the fields overflow into,
// and the time of day.
func balanceISOTime(hour, minute, second, millisecond int64, microsecond, nanosecond int128) (int64, ISOTime) {
	thousand := i128(1000)
	microsecond = microsecond.add(nanosecond.divEuclid(thousand))
	nanosecond = nanosecond.remEuclid(thousand)
	millisecond += microsecond.divEuclid(thousand).int64()
	microsecond = microsecond.remEuclid(thousand)
	second += floorDiv(millisecond, 1000)
	millisecond = floorMod(millisecond, 1000)
	minute += floorDiv(second, 60)
	second = floorMod(second, 60)
	hour += floorDiv(minute, 60)
	minute = floorMod(minute, 60)
	days := floorDiv(hour, 24)
	hour = floorMod(hour, 24)
	return days, ISOTime{int(hour), int(minute), int(second), int(millisecond), int(microsecond.int64()),
		int(nanosecond.int64())}
}

// newISOTime is IsoTime::new: the fields constrained or checked.
func newISOTime(hour, minute, second, millisecond, microsecond, nanosecond int, overflow Overflow) (ISOTime, error) {
	if overflow == Constrain {
		return ISOTime{clamp(hour, 0, 23), clamp(minute, 0, 59), clamp(second, 0, 59),
			clamp(millisecond, 0, 999), clamp(microsecond, 0, 999), clamp(nanosecond, 0, 999)}, nil
	}
	if !validISOTime(hour, minute, second, millisecond, microsecond, nanosecond) {
		return ISOTime{}, rangeError("IsoTime is not valid")
	}
	return ISOTime{hour, minute, second, millisecond, microsecond, nanosecond}, nil
}

func validISOTime(hour, minute, second, millisecond, microsecond, nanosecond int) bool {
	return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 && second >= 0 && second <= 59 &&
		millisecond >= 0 && millisecond <= 999 && microsecond >= 0 && microsecond <= 999 &&
		nanosecond >= 0 && nanosecond <= 999
}

// PartialTime is a time's fields, each nil where not given, as the engine
// hands them over after regulating them (see RegulateTime).
type PartialTime struct {
	Hour, Minute, Second, Millisecond, Microsecond, Nanosecond *int
}

func (p PartialTime) isEmpty() bool {
	return p.Hour == nil && p.Minute == nil && p.Second == nil && p.Millisecond == nil &&
		p.Microsecond == nil && p.Nanosecond == nil
}

// with is IsoTime::with: the fields given replace the time's.
func (t ISOTime) with(p PartialTime, overflow Overflow) (ISOTime, error) {
	pick := func(v *int, def int) int {
		if v != nil {
			return *v
		}
		return def
	}
	return newISOTime(pick(p.Hour, t.Hour), pick(p.Minute, t.Minute), pick(p.Second, t.Second),
		pick(p.Millisecond, t.Millisecond), pick(p.Microsecond, t.Microsecond),
		pick(p.Nanosecond, t.Nanosecond), overflow)
}

// diff is DifferenceTime.
func (t ISOTime) diff(o ISOTime) timeDuration {
	return timeDurationFromComponents(int64(o.Hour-t.Hour), int64(o.Minute-t.Minute), int64(o.Second-t.Second),
		int64(o.Millisecond-t.Millisecond), i128(int64(o.Microsecond-t.Microsecond)),
		i128(int64(o.Nanosecond-t.Nanosecond)))
}

// add is IsoTime::add: the time moved by a time duration, and the days it
// moved over.
func (t ISOTime) add(d timeDuration) (int64, ISOTime) {
	seconds := int64(t.Second) + d.seconds()
	nanos := int32(t.Nanosecond) + d.subseconds()
	return balanceISOTime(int64(t.Hour), int64(t.Minute), seconds, int64(t.Millisecond),
		i128(int64(t.Microsecond)), i128(int64(nanos)))
}

// round is IsoTime::round: the time rounded, and the days it rounded into.
func (t ISOTime) round(o resolvedRounding) (int64, ISOTime, error) {
	h, mi, s := int64(t.Hour), int64(t.Minute), int64(t.Second)
	ms, us, ns := int64(t.Millisecond), int64(t.Microsecond), int64(t.Nanosecond)
	var quantity int64
	switch o.smallest {
	case Day, Hour:
		quantity = ((((h*60+mi)*60+s)*1000+ms)*1000+us)*1000 + ns
	case Minute:
		quantity = (((mi*60+s)*1000+ms)*1000+us)*1000 + ns
	case Second:
		quantity = ((s*1000+ms)*1000+us)*1000 + ns
	case Millisecond:
		quantity = (ms*1000+us)*1000 + ns
	case Microsecond:
		quantity = us*1000 + ns
	case Nanosecond:
		quantity = ns
	default:
		return 0, ISOTime{}, rangeError("Invalid smallestUnit value for time rounding.")
	}
	length := i128(o.smallest.nanoseconds())
	result := roundIncrement(i128(quantity), i128(int64(o.increment.get())).mul(length), o.mode).quo(length)
	if !result.fitsInt64() {
		return 0, ISOTime{}, rangeError("round result valid range.")
	}
	r := result.int64()
	var days int64
	var out ISOTime
	switch o.smallest {
	case Day:
		return r, ISOTime{}, nil
	case Hour:
		days, out = balanceISOTime(r, 0, 0, 0, int128{}, int128{})
	case Minute:
		days, out = balanceISOTime(h, r, 0, 0, int128{}, int128{})
	case Second:
		days, out = balanceISOTime(h, mi, r, 0, int128{}, int128{})
	case Millisecond:
		days, out = balanceISOTime(h, mi, s, r, int128{}, int128{})
	case Microsecond:
		days, out = balanceISOTime(h, mi, s, ms, i128(r), int128{})
	default:
		days, out = balanceISOTime(h, mi, s, ms, i128(us), i128(r))
	}
	return days, out, nil
}

// round is IsoDateTime::round.
func (dt ISODateTime) round(o resolvedRounding) (ISODateTime, error) {
	days, t, err := dt.Time.round(o)
	if err != nil {
		return ISODateTime{}, err
	}
	d, err := tryBalanceISODate(int32(dt.Date.Year), int32(dt.Date.Month), int64(dt.Date.Day)+days)
	if err != nil {
		return ISODateTime{}, err
	}
	return newISODateTime(d, t)
}

// diff is DifferenceISODateTime.
func (dt ISODateTime) diff(o ISODateTime, cal *Calendar, largest Unit) (internalDuration, error) {
	td := dt.Time.diff(o.Time)
	timeSign := td.sign()
	dateSign := o.Date.compare(dt.Date)
	adjusted := o.Date
	if timeSign == -dateSign {
		adjusted = balanceISODate(int32(adjusted.Year), int32(adjusted.Month), int32(adjusted.Day+timeSign))
		var err error
		if td, err = td.addDays(-int64(timeSign)); err != nil {
			return internalDuration{}, err
		}
	}
	two, err := newPlainDate(adjusted.Year, adjusted.Month, adjusted.Day, cal, Reject)
	if err != nil {
		return internalDuration{}, err
	}
	dateLargest := maxUnit(largest, Day)
	dd, err := PlainDate{iso: dt.Date, cal: cal}.diffDate(two, dateLargest)
	if err != nil {
		return internalDuration{}, err
	}
	days := dd.Days
	if largest != dateLargest {
		if td, err = td.addDays(dd.Days); err != nil {
			return internalDuration{}, err
		}
		days = 0
	}
	return newInternalDuration(DateDuration{dd.Years, dd.Months, dd.Weeks, days}, td)
}
