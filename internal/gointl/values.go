package intl

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// The lists Intl.supportedValuesOf answers with. Calendars, numbering
// systems and units are go-intl's own; collations, currencies and time zones
// are V8's, built from ICU's (data/values.bin; see internal/availgen):
// every collation ICU's collation data names, the common current ISO
// currencies with an English name, and ICU's canonical time zones that are
// in a region.

// Calendars lists the calendars a date can be written in, sorted. The
// standard lists those a DateTimeFormat resolves to themselves, as
// test262's calendars-accepted-by-DateTimeFormat requires, which islamic
// and islamic-rgsa are not; V8 lists them (IslamicFallback).
func Calendars(compat Compat) []string {
	out := make([]string, 0, len(implemented))
	for c := range implemented {
		if (c == Islamic || c == IslamicRGSA) && !compat.Has(IslamicFallback) {
			continue
		}
		out = append(out, string(c))
	}
	sort.Strings(out)
	return out
}

// Collations lists the collations, sorted.
func Collations() ([]string, error) { return CollationsFrom(Embedded) }

// CollationsFrom is Collations for a source of the caller's own.
func CollationsFrom(src Source) ([]string, error) { return values(src, "collation") }

// Currencies lists the currencies, by ISO code, sorted.
func Currencies() ([]string, error) { return CurrenciesFrom(Embedded) }

// CurrenciesFrom is Currencies for a source of the caller's own.
func CurrenciesFrom(src Source) ([]string, error) { return values(src, "currency") }

// TimeZones lists the time zones, by their canonical identifiers, sorted:
// ICU's canonical zones in a region, as V8 lists them, and on the standard
// side the zones in none as well, UTC and Etc/GMT's, as test262's
// timeZones-include-non-continental requires (RegionZones).
func TimeZones(compat Compat) ([]string, error) { return TimeZonesFrom(Embedded, compat) }

// TimeZonesFrom is TimeZones for a source of the caller's own.
func TimeZonesFrom(src Source, compat Compat) ([]string, error) {
	zones, err := values(src, "timezone")
	if err != nil || compat.Has(RegionZones) {
		return zones, err
	}
	// The zones in no region are the IANA database's own names under Etc,
	// the links among them aside, and UTC.
	b, err := src.Open(MarkerTemporalZones, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the time zone names: %w", err)
	}
	zones = append(zones, "UTC")
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Etc/") && !strings.Contains(line, " ") {
			zones = append(zones, line)
		}
	}
	sort.Strings(zones)
	return slices.Compact(zones), nil
}

func values(src Source, key string) ([]string, error) {
	b, err := src.Open(MarkerValues, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the supported values: %w", err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, " "); ok && k == key {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("intl: no supported values for %s", key)
	}
	return out, nil
}
