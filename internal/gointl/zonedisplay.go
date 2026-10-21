package intl

import (
	"fmt"
	"time"
)

// TimeZoneDisplayName is ICU's TimeZone::getDisplayName(daylight, LONG,
// locale): what JavaScript's Date writes in brackets after a time, "Central
// European Summer Time". It is the zone's own long standard or daylight
// name in the locale, else its metazone's at the instant now, which is the
// current time and not the time being written; failing both, the zone's
// offset now in the localized GMT format, "GMT-05:00", with its daylight
// saving where daylight is asked for and the zone keeps daylight time this
// year.
func TimeZoneDisplayName(src Source, loc Locale, tz *TimeZone, daylight bool, now time.Time) (string, error) {
	zones, err := loadZoneNames(src, loc)
	if err != nil {
		return "", err
	}
	ms := now.UnixMilli()
	if tz.z.id != "" {
		info := zones.zone(tz.z.id, tz.z)
		typ := longStandard
		if daylight {
			typ = longDaylight
		}
		if name := zones.displayName(&info, typ, now); name != "" {
			return name, nil
		}
	}
	offset := tz.z.offsetAt(ms).raw
	if daylight && tz.z.useDaylightTime(ms) {
		offset += tz.z.dstSavings(ms)
	}
	text := gmtOffset(offset, zones.locale.GMTFormat, zones.locale.HourFormat, false)
	// TimeZoneFormat writes the offset in the locale's default digits.
	if numbers, err := loadNumbers(src, loc); err == nil {
		if chosen, _, err := selectNumberingSystem(src, numbers, loc, ""); err == nil {
			text = mapDigits(text, chosen.Digits)
		}
	}
	return text, nil
}

// useDaylightTime is OlsonTimeZone::useDaylightTime: whether the zone
// keeps daylight time at any point of the year an instant falls in.
func (z *timeZone) useDaylightTime(ms int64) bool {
	if z.final != nil && ms >= z.finalStart {
		return z.final.daylight
	}
	year, _, _, _ := gregoFields(floorDiv64(ms, msPerDay))
	start := gregoDay(year, 0, 1) * 86400
	limit := gregoDay(year+1, 0, 1) * 86400
	for i, t := range z.trans {
		if t >= limit {
			break
		}
		if t >= start && z.typeAt(i).dst != 0 || t > start && z.typeAt(i-1).dst != 0 {
			return true
		}
	}
	return false
}

// dstSavings is OlsonTimeZone::getDSTSavings, in seconds: the final rule's
// saving where the zone has one, else an hour if it keeps daylight time
// this year.
func (z *timeZone) dstSavings(ms int64) int {
	if z.final != nil {
		return z.final.savings
	}
	if z.useDaylightTime(ms) {
		return 3600
	}
	return 0
}

// String is the zone's ID, for printing.
func (z *TimeZone) String() string { return fmt.Sprintf("TimeZone(%s)", z.id) }
