// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	zoneMu    sync.Mutex
	zoneCache = map[string]zoneCacheEntry{}
)

type zoneCacheEntry struct {
	name string
	loc  *time.Location
	err  error
}

// utcAliases are the identifiers ECMA-402 canonicalizes to "UTC".
var utcAliases = map[string]bool{
	"UTC":           true,
	"GMT":           true,
	"ETC/UTC":       true,
	"ETC/GMT":       true,
	"ETC/GREENWICH": true,
	"ETC/UNIVERSAL": true,
	"ETC/ZULU":      true,
	"GREENWICH":     true,
	"UNIVERSAL":     true,
	"ZULU":          true,
	"UCT":           true,
	"ETC/UCT":       true,
}

// timeZone resolves an IANA time zone name, a UTC alias or an offset time zone
// identifier like "+05:30" to its canonical name and location. The error is
// non-nil for anything else, including "Local": Go accepts it as a location
// name but it is not a time zone identifier and browsers reject it.
//
// Resolving a name needs the zoneinfo database. Go finds it in the operating
// system's copy or, when the program imports time/tzdata, in the one embedded
// in the binary.
func timeZone(name string) (string, *time.Location, error) {
	zoneMu.Lock()

	defer zoneMu.Unlock()

	if e, ok := zoneCache[name]; ok {
		return e.name, e.loc, e.err
	}

	cname, loc, err := resolveTimeZone(name)
	zoneCache[name] = zoneCacheEntry{cname, loc, err}
	return cname, loc, err
}

// loadLocation is time.LoadLocation, which a test replaces.
var loadLocation = time.LoadLocation

func resolveTimeZone(name string) (string, *time.Location, error) {
	// ECMA-402 rejects an unusable identifier with a RangeError, and code that
	// probes a zone catches exactly that.
	invalid := rangeError("invalid time zone specified: %s", name)
	switch {
	case name == "":
		return "", nil, invalid
	case utcAliases[strings.ToUpper(name)]:
		return "UTC", time.UTC, nil
	case strings.EqualFold(name, "local"):
		// Go resolves it to the host zone, ECMA-402 has no such identifier.
		return "", nil, invalid
	case name[0] == '+' || name[0] == '-':
		secs, err := parseOffset(name)
		if err != nil {
			return "", nil, invalid
		}

		if secs == 0 {
			return "UTC", time.UTC, nil
		}

		cname := offsetName(secs)
		return cname, time.FixedZone(cname, secs), nil
	}

	cname, ok := zoneNames()[strings.ToUpper(name)]
	if !ok {
		return "", nil, invalid
	}

	if utcAliases[strings.ToUpper(cname)] {
		return "UTC", time.UTC, nil
	}

	loc, err := loadLocation(cname)
	for _, id := range zoneAliases()[cname] {
		if err == nil {
			break
		}
		// CLDR's name is often a link of IANA's backward file, which a
		// system without tzdata-legacy leaves out (Asia/Calcutta,
		// Europe/Kiev): the zone loads under another of its names, IANA's
		// own among them, and keeps CLDR's, which is what browsers report.
		if l, e := loadLocation(id); e == nil {
			loc, err = l, nil
		}
	}
	if err != nil {
		// CLDR knows the zone but the zoneinfo database Go reads does not:
		// the host has none, or an old one, and the program does not import
		// time/tzdata.
		return "", nil, rangeError("invalid time zone specified: %s (%v)", name, err)
	}

	return cname, loc, nil
}

// zoneNames maps every IANA time zone identifier CLDR knows, upper cased, to
// the identifier CLDR treats as the canonical one of its zone. Browsers match
// identifiers case-insensitively and report this canonical name, so
// "asia/kolkata" is "Asia/Calcutta" - CLDR keeps the older name. Go's own
// lookup cannot be used for the case: it opens a file of that name, and on a
// case-insensitive file system it would accept the name as written.
var zoneNames = sync.OnceValue(func() map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(cldr().supplemental().str("tz"), "\n") {
		ids := strings.Fields(line)
		if len(ids) == 0 || ids[0] == "Etc/Unknown" {
			continue
		}

		for _, id := range ids {
			m[strings.ToUpper(id)] = ids[0]
		}
	}
	return m
})

// zoneAliases maps the canonical identifier of each zone CLDR knows to its
// other identifiers, in CLDR's order.
var zoneAliases = sync.OnceValue(func() map[string][]string {
	m := map[string][]string{}
	for _, line := range strings.Split(cldr().supplemental().str("tz"), "\n") {
		if ids := strings.Fields(line); len(ids) > 1 {
			m[ids[0]] = ids[1:]
		}
	}
	return m
})

// parseOffset returns the number of seconds represented by an offset time zone
// identifier: ±HH, ±HHMM or ±HH:MM.
func parseOffset(s string) (int, error) {
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	s = s[1:]
	var hh, mm string
	switch {
	case len(s) == 2:
		hh = s
	case len(s) == 4:
		hh, mm = s[:2], s[2:]
	case len(s) == 5 && s[2] == ':':
		hh, mm = s[:2], s[3:]
	default:
		return 0, fmt.Errorf("invalid offset %q", s)
	}
	h, err := strconv.Atoi(hh)
	if err != nil || h > 23 {
		return 0, fmt.Errorf("invalid offset hours %q", hh)
	}

	m := 0
	if mm != "" {
		if m, err = strconv.Atoi(mm); err != nil || m > 59 {
			return 0, fmt.Errorf("invalid offset minutes %q", mm)
		}
	}
	return sign * (h*3600 + m*60), nil
}

// offsetName renders secs as the canonical ±HH:MM offset time zone identifier.
func offsetName(secs int) string {
	sign := "+"
	if secs < 0 {
		sign = "-"
		secs = -secs
	}
	return fmt.Sprintf("%s%02d:%02d", sign, secs/3600, secs%3600/60)
}
