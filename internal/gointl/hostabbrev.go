package intl

import (
	"strconv"
	"strings"
	"time"
)

// The host zone where nothing names it: neither TZ, nor the zone
// /etc/localtime links to, nor a zone file it is a copy of. ECMA-262's
// SystemTimeZoneIdentifier is then the host's offset, an offset time zone
// identifier, which is Standard's answer. ICU's uprv_tzname (putil.cpp)
// instead guesses from the C library's abbreviations, which is Node's
// (HostAbbreviations): it asks localtime_r whether daylight saving is in
// force at the solstices of 2007, looks the abbreviations, that, and the
// standard offset up in OFFSET_ZONE_MAPPINGS (data/abbreviationzones.bin),
// and failing that takes the standard abbreviation itself for the zone's
// name, which detectHostZone makes a zone of if ICU knows it, and a fixed
// one at the host's offset if not.

// A hostAbbreviation is what ICU reads of the C library's zone: tzname's
// two abbreviations and when daylight saving falls, 0 never, 1 in June and
// 2 in December.
type hostAbbreviation struct {
	std, dst string
	daylight int
}

// The solstices ICU probes (putil.cpp).
const (
	juneSolstice     = 1182478260 // 2007-06-22 02:11 UTC
	decemberSolstice = 1198332540 // 2007-12-22 06:09 UTC
)

// abbreviationsOf is hostAbbreviation of a zone as Go's time package reads
// it from the same file the C library does. glibc's localtime_r sets
// tzname to the abbreviations of the standard and daylight time about the
// instant it converts, and ICU's probes leave them at 2007's: a zone with
// no daylight saving then has its standard abbreviation for both.
func abbreviationsOf(loc *time.Location) hostAbbreviation {
	var a hostAbbreviation
	for _, at := range []int64{decemberSolstice, juneSolstice} {
		t := time.Unix(at, 0).In(loc)
		name, _ := t.Zone()
		switch {
		case t.IsDST() && a.dst == "":
			a.dst = name
		case !t.IsDST() && a.std == "":
			a.std = name
		}
	}
	if a.std == "" {
		a.std = a.dst
	}
	if a.dst == "" {
		a.dst = a.std
	}
	switch {
	case time.Unix(decemberSolstice, 0).In(loc).IsDST():
		a.daylight = 2
	case time.Unix(juneSolstice, 0).In(loc).IsDST():
		a.daylight = 1
	}
	return a
}

// guessHostZone is the end of uprv_tzname(0): the zone of
// OFFSET_ZONE_MAPPINGS for the abbreviations, the daylight saving and the
// standard offset, in seconds east, else the standard abbreviation.
func guessHostZone(src Source, a hostAbbreviation, raw int) string {
	if b, err := src.Open(MarkerAbbreviationZones, DataLocale{}); err == nil {
		west := strconv.Itoa(-raw)
		daylight := strconv.Itoa(a.daylight)
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Split(line, "\t")
			if len(f) == 5 && f[0] == west && f[1] == daylight && f[2] == a.std && f[3] == a.dst {
				return f[4]
			}
		}
	}
	return a.std
}

// offsetHostZone is Standard's host zone where nothing names it: the
// host's standard offset, which an offset time zone identifier writes in
// whole minutes, "+01:00".
func offsetHostZone(src Source, raw int) *TimeZone {
	sign, minutes := '+', raw/60
	if minutes < 0 {
		sign, minutes = '-', -minutes
	}
	hours, minutes := minutes/60, minutes%60
	if hours > 23 {
		return unknownZone()
	}
	return createTimeZone(src, "GMT"+string(sign)+twoDigits(hours)+":"+twoDigits(minutes))
}

func twoDigits(n int) string { return string([]byte{byte('0' + n/10), byte('0' + n%10)}) }

// unnamedHostZone is the host zone where hostZone found no name.
func unnamedHostZone(src Source, compat Compat, raw int, now int64, abbreviations func() (hostAbbreviation, bool)) *TimeZone {
	if !compat.Has(HostAbbreviations) {
		return offsetHostZone(src, raw)
	}
	a, ok := abbreviations()
	if !ok || a.std == "" {
		return unknownZone()
	}
	return detectHostZone(src, guessHostZone(src, a, raw), raw, now)
}
