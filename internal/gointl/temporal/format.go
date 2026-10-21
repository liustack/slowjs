package temporal

import (
	"strconv"
	"strings"
)

// The strings Temporal writes, as temporal_rs 0.2.3's IxdtfStringBuilder
// and Formattable types write them.

// ixdtfBuilder is IxdtfStringBuilder: a date, a time, an offset, a zone
// annotation and a calendar annotation, each written if set.
type ixdtfBuilder struct {
	strings.Builder
	hasDate bool
}

func writePadded2(b *strings.Builder, v int) {
	if v < 10 {
		b.WriteByte('0')
	}
	b.WriteString(strconv.Itoa(v))
}

// writeYear is write_year: four digits for 0 to 9999, else a sign and six.
func writeYear(b *strings.Builder, y int32) {
	if y >= 0 && y <= 9999 {
		b.WriteString(strconv.Itoa(int(y/1000)) + strconv.Itoa(int(y%1000/100)) + strconv.Itoa(int(y%100/10)) +
			strconv.Itoa(int(y%10)))
		return
	}
	if y < 0 {
		b.WriteByte('-')
	} else {
		b.WriteByte('+')
	}
	abs := uint32(y)
	if y < 0 {
		abs = uint32(-int64(y))
	}
	digits := u32Digits(abs)
	for _, d := range digits[3:9] {
		b.WriteByte('0' + d)
	}
}

// u32Digits is u32_to_digits: the last nine decimal digits.
func u32Digits(v uint32) [9]byte {
	var out [9]byte
	for i := 8; i >= 0; i-- {
		out[i] = byte(v % 10)
		v /= 10
	}
	return out
}

// writeDate is FormattableDate.
func writeDate(b *strings.Builder, year int32, month, day int) {
	writeYear(b, year)
	b.WriteByte('-')
	writePadded2(b, month)
	b.WriteByte('-')
	writePadded2(b, day)
}

func (b *ixdtfBuilder) date(d ISODate) {
	writeDate(&b.Builder, int32(d.Year), d.Month, d.Day)
	b.hasDate = true
}

// writeTime is FormattableTime.
func writeTime(b *strings.Builder, hour, minute, second int, nanosecond uint32, p Precision, sep bool) {
	writePadded2(b, hour)
	if sep {
		b.WriteByte(':')
	}
	writePadded2(b, minute)
	if p == PrecisionMinute {
		return
	}
	if sep {
		b.WriteByte(':')
	}
	writePadded2(b, second)
	if nanosecond == 0 && p == PrecisionAuto || p == 0 {
		return
	}
	b.WriteByte('.')
	writeNanosecond(b, nanosecond, p)
}

// writeNanosecond is write_nanosecond: the digits to the precision, or
// to the last nonzero one.
func writeNanosecond(b *strings.Builder, ns uint32, p Precision) {
	digits := u32Digits(ns)
	n := 0
	for i := 8; i >= 0; i-- {
		if digits[i] != 0 {
			n = i + 1
			break
		}
	}
	if p >= 0 && p <= 9 {
		n = int(p)
	}
	for _, d := range digits[:n] {
		b.WriteByte('0' + d)
	}
}

func (b *ixdtfBuilder) time(t ISOTime, p Precision) {
	if b.hasDate {
		b.WriteByte('T')
	}
	ns := uint32(t.Millisecond)*1_000_000 + uint32(t.Microsecond)*1000 + uint32(t.Nanosecond)
	writeTime(&b.Builder, t.Hour, t.Minute, t.Second, ns, p, true)
}

// minuteOffset is with_minute_offset.
func (b *ixdtfBuilder) minuteOffset(negative bool, hour, minute int, show DisplayOffset) {
	if show == OffsetNever {
		return
	}
	if negative {
		b.WriteByte('-')
	} else {
		b.WriteByte('+')
	}
	writeTime(&b.Builder, hour, minute, 0, 0, PrecisionMinute, true)
}

func (b *ixdtfBuilder) z(show DisplayOffset) {
	if show != OffsetNever {
		b.WriteByte('Z')
	}
}

func (b *ixdtfBuilder) timeZone(id string, show DisplayTimeZone) {
	if show == TimeZoneNever {
		return
	}
	b.WriteByte('[')
	if show == TimeZoneCritical {
		b.WriteByte('!')
	}
	b.WriteString(id)
	b.WriteByte(']')
}

// writeCalendar is FormattableCalendar.
func writeCalendar(b *strings.Builder, id string, show DisplayCalendar) {
	if show == CalendarNever || show == CalendarAuto && id == "iso8601" {
		return
	}
	b.WriteByte('[')
	if show == CalendarCritical {
		b.WriteByte('!')
	}
	b.WriteString("u-ca=")
	b.WriteString(id)
	b.WriteByte(']')
}

func (b *ixdtfBuilder) calendar(id string, show DisplayCalendar) { writeCalendar(&b.Builder, id, show) }

// formatOffset is FormattableOffset over an offset in nanoseconds, with
// seconds and a fraction where it has them.
func formatOffset(ns int64) string {
	var b strings.Builder
	sign := ns < 0
	if sign {
		b.WriteByte('-')
		ns = -ns
	} else {
		b.WriteByte('+')
	}
	nano := uint32(ns % 1_000_000_000)
	secs := ns / 1_000_000_000
	second := int(secs % 60)
	minute := int(secs / 60 % 60)
	hour := int(secs / 3600)
	p := PrecisionAuto
	if nano == 0 && second == 0 {
		p = PrecisionMinute
	}
	writeTime(&b, hour, minute, second, nano, p, true)
	return b.String()
}

// A formattableDuration is FormattableDuration with the seconds variant
// Duration's toString uses.
type formattableDuration struct {
	precision               Precision
	negative                bool
	hasDate                 bool
	years, months, weeks    uint32
	days                    uint64
	hours, minutes, seconds uint64
	subseconds              uint32
}

func (f formattableDuration) String() string {
	var b strings.Builder
	if f.negative {
		b.WriteByte('-')
	}
	b.WriteByte('P')
	suffix := func(v uint64, s byte) {
		if v != 0 {
			b.WriteString(strconv.FormatUint(v, 10))
			b.WriteByte(s)
		}
	}
	if f.hasDate {
		suffix(uint64(f.years), 'Y')
		suffix(uint64(f.months), 'M')
		suffix(uint64(f.weeks), 'W')
		suffix(f.days, 'D')
	}
	ns := f.subseconds
	belowMinute := !f.hasDate && f.hours == 0 && f.minutes == 0
	writeSecond := f.seconds != 0 || ns != 0 || belowMinute || f.precision >= 0
	if f.hours != 0 || f.minutes != 0 || writeSecond {
		b.WriteByte('T')
	}
	suffix(f.hours, 'H')
	suffix(f.minutes, 'M')
	if writeSecond {
		b.WriteString(strconv.FormatUint(f.seconds, 10))
		if f.precision == 0 || f.precision == PrecisionAuto && ns == 0 {
			b.WriteByte('S')
			return b.String()
		}
		b.WriteByte('.')
		writeNanosecond(&b, ns, f.precision)
		b.WriteByte('S')
	}
	return b.String()
}

// formattable is duration_to_formattable.
func (d Duration) formattable(p Precision) formattableDuration {
	a := d.Abs()
	f := formattableDuration{precision: p, negative: d.sign < 0}
	if a.Years()+a.Months()+a.Weeks()+a.Days() != 0 {
		f.hasDate = true
		f.years, f.months, f.weeks, f.days = uint32(a.Years()), uint32(a.Months()), uint32(a.Weeks()), uint64(a.Days())
	}
	f.hours = uint64(saturatingAbs(a.Hours()))
	f.minutes = uint64(saturatingAbs(a.Minutes()))
	t := timeDurationFromComponents(0, 0, a.Seconds(), a.Milliseconds(), a.microsecondsSigned(), a.nanosecondsSigned())
	f.seconds = unsignedAbs(t.seconds())
	sub := t.subseconds()
	if sub < 0 {
		sub = -sub
	}
	f.subseconds = uint32(sub)
	return f
}
