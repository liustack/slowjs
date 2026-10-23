// Package zonedata is the model layer for time-zone names: what a locale calls
// a zone, and how it writes one it has no name for.
//
// A locale does not name zones one by one. It names *metazones* -- the Eastern
// Time that a dozen American zones share -- and CLDR keeps the mapping from a
// zone to the metazone it belongs to, which has changed over the years. A zone
// with no metazone, and a locale with no name for the one it has, is written
// by where it is -- "United Kingdom Time", "Los Angeles Time" -- or failing
// that by its offset from UTC.
//
// The names are ICU's, merged along ICU's fallback chain field by field, as
// ICU's TimeZoneNames reads them. The chain is not CLDR's: a locale ICU has no
// zone bundle for falls back by ICU's resource rules, and one field a locale
// leaves out is taken from its parent while the others stay its own.
package zonedata

import (
	"github.com/liustack/slowjs/internal/gointl/internal/blob"
)

// Version is the encoding's version.
const Version = 4

// A Names is what a locale calls one metazone or zone.
//
// Generic is the zone whatever the season -- "Eastern Time" -- while Standard
// and Daylight name the two halves of the year. Any of them may be empty.
type Names struct {
	LongGeneric   string
	LongStandard  string
	LongDaylight  string
	ShortGeneric  string
	ShortStandard string
	ShortDaylight string
}

// Empty reports whether the locale names this metazone at all.
func (n Names) Empty() bool {
	return n.LongGeneric == "" && n.LongStandard == "" && n.LongDaylight == "" &&
		n.ShortGeneric == "" && n.ShortStandard == "" && n.ShortDaylight == ""
}

// An Entry is one metazone and its names.
type Entry struct {
	Metazone string
	Names    Names
}

// Built is what one locale says about zones, as a generator builds it.
type Built struct {
	// GMTFormat wraps an offset, "GMT{0}", and HourFormat writes the offset
	// itself, "+HH:mm;-HH:mm".
	GMTFormat  string
	HourFormat string
	// RegionFormat names a zone by where it is, "{0} Time", and
	// FallbackFormat qualifies a metazone's name by a place in it,
	// "{1} ({0})".
	RegionFormat   string
	FallbackFormat string

	// Metazones are the metazones the locale names.
	Metazones []Entry
	// Zones are the zones the locale says something about itself -- a name
	// or the city it is written by -- in CLDR's canonical form
	// ("Asia/Calcutta").
	Zones []ZoneEntry
	// Regions are the locale's names for the regions zones are in. A region
	// it has no name for is written as its code.
	Regions []RegionEntry
}

// A ZoneEntry is one named zone.
type ZoneEntry struct {
	Zone  string
	Names Names
	// City is the exemplar city, where the locale gives one; otherwise it
	// is made from the zone's identifier.
	City string
}

// A RegionEntry is what the locale calls one region.
type RegionEntry struct {
	Region string
	Name   string
}

// Locale is what one locale says about zones, read where it lies: a
// formatter names one zone, so the metazones, zones and regions are looked
// up one at a time rather than read whole.
type Locale struct {
	GMTFormat      string
	HourFormat     string
	RegionFormat   string
	FallbackFormat string

	metazones, zones, regions blob.Table
}

// Metazone finds a metazone's names.
func (l *Locale) Metazone(name string) (Names, bool) {
	r, ok := l.metazones.Find(name)
	if !ok {
		return Names{}, false
	}
	n := readNames(r)
	return n, r.Err() == nil
}

// Zone finds what the locale says about one zone.
func (l *Locale) Zone(name string) (ZoneEntry, bool) {
	r, ok := l.zones.Find(name)
	if !ok {
		return ZoneEntry{}, false
	}
	e := ZoneEntry{Zone: name, Names: readNames(r), City: r.SharedString()}
	return e, r.Err() == nil
}

// Region finds the locale's name for a region.
func (l *Locale) Region(code string) (string, bool) {
	r, ok := l.regions.Find(code)
	if !ok {
		return "", false
	}
	name := r.SharedString()
	return name, r.Err() == nil
}

// Encode writes a locale's zone names, with what it shares with other
// locales -- every string, each set of names and each list -- in pool,
// which the generator writes beside the locales and Decode is given.
func Encode(l *Built, pool *blob.Pool) []byte {
	b := blob.NewPooledWriter(Version, pool)
	b.SharedString(l.GMTFormat)
	b.SharedString(l.HourFormat)
	b.SharedString(l.RegionFormat)
	b.SharedString(l.FallbackFormat)
	metazones := map[string]Names{}
	for _, e := range l.Metazones {
		metazones[e.Metazone] = e.Names
	}
	b.SharedTable(keys(metazones), func(k string, b *blob.Writer) { writeNames(b, metazones[k]) })
	zones := map[string]ZoneEntry{}
	for _, e := range l.Zones {
		zones[e.Zone] = e
	}
	b.SharedTable(keys(zones), func(k string, b *blob.Writer) {
		writeNames(b, zones[k].Names)
		b.SharedString(zones[k].City)
	})
	regions := map[string]string{}
	for _, e := range l.Regions {
		regions[e.Region] = e.Name
	}
	b.SharedTable(keys(regions), func(k string, b *blob.Writer) { b.SharedString(regions[k]) })
	return b.Bytes()
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func writeNames(b *blob.Writer, n Names) {
	b.Shared(func(b *blob.Writer) {
		for _, s := range [...]string{
			n.LongGeneric, n.LongStandard, n.LongDaylight,
			n.ShortGeneric, n.ShortStandard, n.ShortDaylight,
		} {
			b.SharedString(s)
		}
	})
}

func readNames(r *blob.Reader) Names {
	var n Names
	r.Shared(func(r *blob.Reader) {
		for _, p := range []*string{
			&n.LongGeneric, &n.LongStandard, &n.LongDaylight,
			&n.ShortGeneric, &n.ShortStandard, &n.ShortDaylight,
		} {
			*p = r.SharedString()
		}
	})
	return n
}

// Decode reads what Encode wrote, with the pool it wrote into.
func Decode(data []byte, pool blob.Shared) (*Locale, error) {
	r, err := blob.NewPooledReader(data, Version, pool)
	if err != nil {
		return nil, err
	}
	var l Locale
	l.GMTFormat = r.SharedString()
	l.HourFormat = r.SharedString()
	l.RegionFormat = r.SharedString()
	l.FallbackFormat = r.SharedString()
	l.metazones = r.SharedTable()
	l.zones = r.SharedTable()
	l.regions = r.SharedTable()
	if err := r.Err(); err != nil {
		return nil, err
	}
	return &l, nil
}
