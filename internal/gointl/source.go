package intl

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
	"github.com/liustack/slowjs/internal/gointl/internal/layout"
)

// Where the data comes from.
//
// This interface is the whole reusability story. Every table this library
// reads is asked for through it, so a caller can supply their own, load from a
// directory instead of the binary, or carry only the locales they need. The
// package it replaces welded its tables in with go:embed at package scope,
// which is why importing it cost fourteen megabytes whether or not anything
// was formatted.
//
// A Source hands back bytes, not decoded values. Decoding is the model
// layer's business, and keeping it out of here means a source can be a
// directory, an archive or a network cache without knowing what any of it
// means.

// A Marker names one data set.
type Marker string

const (
	// MarkerLikelySubtags is CLDR's likely-subtag table: what an identifier
	// that leaves out a script or a region most probably meant.
	MarkerLikelySubtags Marker = "likelysubtags"
	// MarkerParentLocales is CLDR's parent-locale table, which redirects a
	// fallback that truncation would send somewhere wrong.
	MarkerParentLocales Marker = "parentlocales"
	// MarkerValidRegions is the regions a "-u-rg-" or "-u-sd-" keyword may
	// name, as ICU's RegionValidateMap has them, a line each.
	MarkerValidRegions Marker = "validregions"
	// MarkerNumbers is one locale's number symbols and patterns.
	MarkerNumbers Marker = "numbers"
	// MarkerCurrencyDigits is how many decimals each currency is written
	// with, which does not vary by locale.
	MarkerCurrencyDigits Marker = "currencydigits"
	// MarkerCurrencyRegions is each region's currency, which does not vary
	// by locale either.
	MarkerCurrencyRegions Marker = "currencyregions"
	// MarkerPlurals is one language's plural rules.
	MarkerPlurals Marker = "plurals"
	// MarkerLists is one locale's list-joining patterns.
	MarkerLists Marker = "lists"
	// MarkerUnits is one locale's measurement patterns.
	MarkerUnits Marker = "units"
	// MarkerRelativeTime is one locale's relative-time wordings.
	MarkerRelativeTime Marker = "reltime"
	// MarkerNames is one locale's display names. It is much the largest of
	// these, and a program that never asks for one never reads it.
	MarkerNames Marker = "names"
	// MarkerDates is one locale's calendar names and date patterns.
	MarkerDates Marker = "dates"
	// MarkerDatesShared is what the locales' date data shares: every
	// string and list, kept once and read by number.
	MarkerDatesShared Marker = "datesshared"
	// MarkerNumbersShared is what the locales' number data shares.
	MarkerNumbersShared Marker = "numbersshared"
	// MarkerZoneNamesShared is what the locales' zone names share.
	MarkerZoneNamesShared Marker = "zonenamesshared"
	// MarkerNamesShared is what the locales' display names share.
	MarkerNamesShared Marker = "namesshared"
	// MarkerUnitsShared is what the locales' measurement patterns share.
	MarkerUnitsShared Marker = "unitsshared"
	// MarkerRelativeTimeShared is what the locales' relative-time wordings
	// share.
	MarkerRelativeTimeShared Marker = "reltimeshared"
	// MarkerZoneNames is what one locale calls the time zones.
	MarkerZoneNames Marker = "zonenames"
	// MarkerMetazones maps a zone to the metazone it belongs to, which is not
	// a matter of language and so is one table for every locale.
	MarkerMetazones Marker = "metazones"
	// MarkerCalendarPrefs maps a region to the calendar it reckons in, which
	// is likewise not a matter of language.
	MarkerCalendarPrefs Marker = "calendarprefs"
	// MarkerPatternCalendars maps the few locales whose patterns ICU reads
	// from another calendar than the one they reckon in, when none is asked
	// for, to that calendar (see PatternCalendar).
	MarkerPatternCalendars Marker = "patterncalendars"
	// MarkerTimeData is CLDR's hour-cycle preferences, by region or by
	// language and region.
	MarkerTimeData Marker = "timedata"
	// MarkerJapaneseEras is the first day of each era of the Japanese
	// calendar.
	MarkerJapaneseEras Marker = "japaneseeras"
	// MarkerValues is the collations, currencies and time zones
	// Intl.supportedValuesOf lists.
	MarkerValues Marker = "values"
	// MarkerICUFallback is the default scripts and parent locales ICU's
	// resource fallback reads for a bundle that does not exist. Beside it,
	// "icutree-<tree>" is ICU's index of one of its trees.
	MarkerICUFallback Marker = "icufallback"
	// MarkerAvailable is the locales each service is available in.
	MarkerAvailable Marker = "available"
	// MarkerAliases is CLDR's aliases for locale subtags and extension types,
	// as ICU holds them.
	MarkerAliases Marker = "aliases"
	// MarkerUmmAlQura is the Umm al-Qura calendar's table of month lengths.
	MarkerUmmAlQura Marker = "ummalqura"
	// MarkerTimeZones is ICU's time zones, one data set per name in
	// lowercase: MarkerTimeZones + "/" + "europe/paris", which is
	// tz/europe/paris.bin, asked for at the root.
	MarkerTimeZones Marker = "tz"
	// MarkerWindowsZones is CLDR's mapping of Windows zones to ICU's.
	MarkerWindowsZones Marker = "windowszones"
	// MarkerTemporalCalendars is ICU4X's tables of the Chinese, Korean and
	// Umm al-Qura years, which the temporal package's calendars read.
	MarkerTemporalCalendars Marker = "temporalcalendars"
	// MarkerTemporalZones is the time zone names Temporal takes, from
	// timezone_provider's table, and the primary name of each link.
	MarkerTemporalZones Marker = "temporalzones"
	// MarkerRBNF is ICU's rules for its algorithmic numbering systems, the
	// Roman and Hebrew numerals among them.
	MarkerRBNF Marker = "rbnf"
	// MarkerWeekData is CLDR's week conventions by region: the first day of
	// the week, the days the first week of a year needs, the weekend.
	MarkerWeekData Marker = "weekdata"
	// MarkerNormalization is how characters come apart and go back together,
	// which is Unicode's rather than any locale's.
	MarkerNormalization Marker = "normalization"
	// MarkerNumberingSystems is CLDR's numeric numbering systems, with their
	// digits and what the root says about writing numbers in each.
	MarkerNumberingSystems Marker = "numberingsystems"
	// MarkerCollation is the collations one locale defines: what it changes
	// about the root order, and what it sorts by default.
	MarkerCollation Marker = "collation"
	// MarkerCollationRoot is the root collation table, which every
	// collation falls back to.
	MarkerCollationRoot Marker = "collationroot"
	// MarkerCollationTree is how collation locales inherit from one
	// another, which differs from the ordinary fallback.
	MarkerCollationTree Marker = "collationtree"
	// MarkerProperties is the Unicode properties formatting reads -- general
	// categories, the default ignorables, the Hebrew script -- in the
	// Unicode ICU carries.
	MarkerProperties Marker = "properties"
	// MarkerAbbreviationZones is ICU's table of the zones a host's
	// abbreviations and offset stand for, where nothing else names its
	// zone.
	MarkerAbbreviationZones Marker = "abbreviationzones"
)

// ErrNotFound reports that a source has no data under a marker and locale. It
// is not a failure on its own: a lookup walks a fallback chain and expects to
// be told no on the way.
var ErrNotFound = errors.New("no data")

// A Source supplies the bytes of one data set.
//
// Data that is not kept per locale -- the likely subtags, which are one table
// for everything -- is asked for at the root, and a source that has it answers
// there.
//
// The bytes are read where they lie and never changed, by the package, by
// anyone it hands them to, or by the source: what a formatter reads from
// them it keeps without copying, for as long as it lives. A source may
// answer with memory that cannot be written.
type Source interface {
	Open(m Marker, d DataLocale) ([]byte, error)
}

// Embedded is the data built into this package. It is the default, so that the
// simple path needs no setting up, and it is only a default: anything taking a
// Source can be given another.
//
// In slowjs it is not go-intl's data.pack but the packs of qjsdata/, which
// qjs_source.go serves.
var Embedded EmbeddedSource

// EmbeddedSource is Embedded's type, the data built into the package. Its
// one value is its zero value, so that Embedded cannot be made to name
// other data, by one importer for every other.
type EmbeddedSource struct{}

// Open serves the embedded data.
func (EmbeddedSource) Open(m Marker, d DataLocale) ([]byte, error) {
	return qjsOpen(m, d)
}

// NewFS returns a Source that reads from a file system, such as the data
// directory on disk: the files the embedded pack was made from.
//
// A data set kept for one locale is the file <marker>/<locale>.bin, and one
// kept for everything is <marker>.bin.
func NewFS(fsys fs.FS) Source { return fsSource{fsys} }

type fsSource struct{ fsys fs.FS }

func (s fsSource) Open(m Marker, d DataLocale) ([]byte, error) {
	return openFiles(s, m, d)
}

func (s fsSource) readFile(name string) ([]byte, error) { return fs.ReadFile(s.fsys, name) }

// files is what a source reads its data from: a file system or a pack.
type files interface {
	readFile(name string) ([]byte, error)
}

// openFiles finds a data set's file for a data locale, as the generators
// lay them out.
func openFiles(s files, m Marker, d DataLocale) ([]byte, error) {
	var b []byte
	var err error
	if d.IsRoot() {
		b, err = s.readFile(string(m) + ".bin")
		if errors.Is(err, fs.ErrNotExist) {
			// A data set kept per locale has the root's as "und", where the
			// fallback chain ends.
			b, err = s.readFile(string(m) + "/und.bin")
		}
	} else {
		b, err = s.readFile(string(m) + "/" + d.String() + ".bin")
		if errors.Is(err, fs.ErrNotExist) && d.Variant.IsZero() {
			// A locale whose data is byte for byte another's has no file of
			// its own; the data set's same.bin names the locale whose it is.
			if to, ok := same(s, m, d); ok {
				// The root is written "und", as its String is.
				b, err = s.readFile(string(m) + "/" + to.String() + ".bin")
			}
		}
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s for %s", ErrNotFound, m, d)
		}
		return nil, err
	}
	return b, nil
}

// same looks a locale up in a data set's table of locales whose data is
// another's, written by the generators (internal/datawrite).
func same(s files, m Marker, d DataLocale) (DataLocale, bool) {
	b, err := s.readFile(string(m) + "/same.bin")
	if err != nil {
		return DataLocale{}, false
	}
	t, err := newPairTable(b)
	if err != nil {
		return DataLocale{}, false
	}
	return t.lookup(d)
}

// pairTable is a generated table of data locales mapped to data locales, laid
// out as a version byte, layout.Pairs, then records of two of them, sorted
// by the first.
//
// It is read where it lies. Nothing is decoded until something is looked up,
// and a lookup is a binary search over the bytes, so a table costs what it
// takes up and no more.
type pairTable []byte

const pairSize = 2 * DataLocaleSize

func newPairTable(b []byte) (pairTable, error) {
	if len(b) == 0 || b[0] != layout.Pairs {
		return nil, fmt.Errorf("a table of locale pairs not of version %d", layout.Pairs)
	}
	b = b[1:]
	if len(b)%pairSize != 0 {
		return nil, fmt.Errorf("a table of %d bytes does not divide into %d-byte records",
			len(b), pairSize)
	}
	return pairTable(b), nil
}

func (t pairTable) len() int { return len(t) / pairSize }

// at returns the key and value of one record.
func (t pairTable) at(i int) (key, value DataLocale) {
	rec := t[i*pairSize : (i+1)*pairSize]
	_ = key.UnmarshalBinary(rec[:DataLocaleSize])
	_ = value.UnmarshalBinary(rec[DataLocaleSize:])
	return key, value
}

// lookup finds what a data locale maps to.
func (t pairTable) lookup(want DataLocale) (DataLocale, bool) {
	key, _ := want.MarshalBinary()
	lo, hi := 0, t.len()
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch compareBytes(t[mid*pairSize:mid*pairSize+DataLocaleSize], key) {
		case 0:
			_, v := t.at(mid)
			return v, true
		case -1:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return DataLocale{}, false
}

func compareBytes(a, b []byte) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// openShared opens the pool a data set kept per locale shares, which its
// tables are read with.
func openShared(src Source, m Marker, version byte) (blob.Shared, error) {
	b, err := src.Open(m, DataLocale{})
	if err != nil {
		return blob.Shared{}, fmt.Errorf("intl: %s: %w", m, err)
	}
	pool, err := blob.ReadShared(b, version)
	if err != nil {
		return blob.Shared{}, fmt.Errorf("intl: %s: %w", m, err)
	}
	return pool, nil
}
