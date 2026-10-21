package intl

import (
	"fmt"
	"strings"
	"time"

	"github.com/liustack/quickjs/internal/gointl/internal/zonedata"
)

// What a locale calls a time zone.
//
// A locale does not name zones one by one. It names metazones -- the Eastern
// Time that a dozen American zones share -- and a zone is written with the
// name of the metazone it belongs to at the instant. Which of that metazone's
// names is used depends on the instant too: Eastern Standard Time in winter
// and Eastern Daylight Time in summer.
//
// This follows ICU's TimeZoneFormat (tzfmt.cpp), TimeZoneNamesImpl
// (tznames_impl.cpp) and TimeZoneGenericNames (tzgnames.cpp), which is what
// V8 formats with:
//
//   - A specific name, "z" and "zzzz", is the zone's own name for the season,
//     else its metazone's. There is no falling back from one kind of name to
//     another: a locale that has no short standard name for a zone writes the
//     offset.
//   - A generic name, "v" and "vvvv", is the zone's own generic name, else its
//     metazone's -- or the standard one, when the zone keeps no summer time
//     near the instant -- qualified by a place when the zone's offset is not
//     the one the metazone's name means in the reader's region: "Pacific Time
//     (Canada)". Failing a name, it is the zone's location: "United Kingdom
//     Time", "Los Angeles Time".
//   - Failing everything, the offset: "GMT-5", "GMT-05:00". UTC itself is
//     "GMT+0" and "GMT+00:00"; ICU writes the locale's word for zero offset,
//     "GMT", only when it has a name to write, and it never does for these.

// zoneNames is a locale's zone data together with the tables every locale
// shares.
type zoneNames struct {
	// src is where the reference zones a metazone's name stands for are
	// read from.
	src    Source
	locale *zonedata.Locale
	meta   *zonedata.Meta
	// region is the region the reader is in, which decides whether a
	// metazone's name needs qualifying.
	region string
}

func loadZoneNames(src Source, loc Locale) (*zoneNames, error) {
	chain := loc.Fallback()
	fb, fbErr := NewFallbacker(src)
	if fbErr == nil {
		// zonegen resolves ICU's zone and region trees itself for every
		// locale it writes, so only a locale without a file of its own, such
		// as the alias "sr-ME", is redirected.
		chain = fb.ChainIn(treeZone, loc.Data())
		if _, err := src.Open(MarkerZoneNames, loc.Data()); err == nil {
			chain = fb.Chain(loc.Data())
		}
	}
	pool, err := openShared(src, MarkerZoneNamesShared, zonedata.Version)
	if err != nil {
		return nil, err
	}
	var data *zonedata.Locale
	for _, d := range chain {
		b, err := src.Open(MarkerZoneNames, d)
		if err != nil {
			continue
		}
		data, err = zonedata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the zone names for %s: %w", d, err)
		}
		break
	}
	if data == nil {
		// Without names every zone is written as its offset, which is a worse
		// answer than the locale's own but not a wrong one.
		data = &zonedata.Locale{}
	}
	if data.GMTFormat == "" {
		data.GMTFormat = "GMT{0}"
	}
	if data.HourFormat == "" {
		data.HourFormat = "+HH:mm;-HH:mm"
	}
	if data.RegionFormat == "" {
		data.RegionFormat = "{0}"
	}
	if data.FallbackFormat == "" {
		data.FallbackFormat = "{1} ({0})"
	}

	out := &zoneNames{src: src, locale: data, meta: &zonedata.Meta{}}
	if b, err := src.Open(MarkerMetazones, DataLocale{}); err == nil {
		if out.meta, err = zonedata.DecodeMeta(b); err != nil {
			return nil, fmt.Errorf("intl: the zone table: %w", err)
		}
	}
	// TZGNCore's target region: the locale's, or the one it most likely
	// means.
	out.region = loc.Region.String()
	if out.region == "" && fbErr == nil {
		if full, ok := fb.Maximize(loc.Data()); ok {
			out.region = full.Region.String()
		}
	}
	return out, nil
}

// zoneInfo is one zone as the names see it, found once when a formatter is
// built.
type zoneInfo struct {
	// id is the zone's canonical identifier, empty for one ICU does not
	// know, which is then written only as an offset.
	id   string
	meta *zonedata.MetaZone
	own  zonedata.ZoneEntry
	tz   *timeZone
}

func (z *zoneNames) zone(name string, tz *timeZone) zoneInfo {
	info := zoneInfo{tz: tz}
	id, ok := z.meta.Canonical(name)
	if !ok {
		return info
	}
	info.id = id
	info.meta, _ = z.meta.Zone(id)
	info.own, _ = z.locale.Zone(id)
	return info
}

// metazone is the metazone a zone belongs to at an instant, if any.
func (info *zoneInfo) metazone(t time.Time) string {
	if info.meta == nil {
		return ""
	}
	m := t.Unix()/60 + 1
	if t.Unix() < 0 && t.Unix()%60 != 0 {
		m-- // round toward the past
	}
	for _, u := range info.meta.Uses {
		if (u.From == 0 || m >= u.From) && (u.To == 0 || m < u.To) {
			return u.Metazone
		}
	}
	return ""
}

// The kinds of name, as TimeZoneNames numbers them.
type zoneNameType int

const (
	longGeneric zoneNameType = iota
	longStandard
	longDaylight
	shortGeneric
	shortStandard
	shortDaylight
)

func (t zoneNameType) of(n zonedata.Names) string {
	switch t {
	case longGeneric:
		return n.LongGeneric
	case longStandard:
		return n.LongStandard
	case longDaylight:
		return n.LongDaylight
	case shortGeneric:
		return n.ShortGeneric
	case shortStandard:
		return n.ShortStandard
	}
	return n.ShortDaylight
}

// displayName is TimeZoneNames::getDisplayName: the zone's own name of the
// type, else its metazone's at the instant.
func (z *zoneNames) displayName(info *zoneInfo, typ zoneNameType, t time.Time) string {
	if name := typ.of(info.own.Names); name != "" {
		return name
	}
	return z.metazoneName(info.metazone(t), typ)
}

func (z *zoneNames) metazoneName(metazone string, typ zoneNameType) string {
	if metazone == "" {
		return ""
	}
	names, _ := z.locale.Metazone(metazone)
	return typ.of(names)
}

// specific is TimeZoneFormat::formatSpecific, "z" and "zzzz"; empty when the
// locale has no such name.
func (z *zoneNames) specific(info *zoneInfo, t time.Time, long bool) string {
	if info.id == "" {
		return ""
	}
	daylight := info.tz.offsetAt(t.UnixMilli()).dst != 0
	typ := shortStandard
	switch {
	case long && daylight:
		typ = longDaylight
	case long:
		typ = longStandard
	case daylight:
		typ = shortDaylight
	}
	return z.displayName(info, typ, t)
}

// generic is TZGNCore::getDisplayName for "v" and "vvvv": a generic name,
// else the zone's location; empty when there is neither.
func (z *zoneNames) generic(info *zoneInfo, t time.Time, long bool) string {
	if info.id == "" {
		return ""
	}
	if name := z.genericNonLocation(info, t, long); name != "" {
		return name
	}
	return z.locationName(info.id)
}

// dstCheckRange is how near, in milliseconds, summer time has to be for
// ICU to call a zone's winter by its generic name rather than its standard
// one.
const dstCheckRange = 184 * msPerDay

// genericNonLocation is TZGNCore::formatGenericNonLocationName.
func (z *zoneNames) genericNonLocation(info *zoneInfo, t time.Time, long bool) string {
	typ, std := shortGeneric, shortStandard
	if long {
		typ, std = longGeneric, longStandard
	}
	if name := typ.of(info.own.Names); name != "" {
		return name
	}
	metazone := info.metazone(t)
	if metazone == "" {
		return ""
	}
	ms := t.UnixMilli()
	offset := info.tz.offsetAt(ms)
	// A zone that keeps no summer time around the instant is called by its
	// standard name, "Greenwich Mean Time" rather than "GMT Time", unless the
	// two are the same.
	if offset.dst == 0 && !info.tz.summerNear(ms) {
		if name := z.displayName(info, std, t); name != "" &&
			!strings.EqualFold(name, z.metazoneName(metazone, typ)) {
			return name
		}
	}
	name := z.metazoneName(metazone, typ)
	if name == "" {
		return ""
	}
	// The metazone's name means the offset of its reference zone in the
	// reader's region. A zone at another offset just now is named with a
	// place as well.
	golden := z.meta.Reference(metazone, z.region)
	if golden == "" || golden == info.id {
		return name
	}
	gz, err := loadTimeZone(z.src, golden)
	if err != nil {
		return name
	}
	// The reference zone's offset at the same wall time, which, read as a
	// local time, is not muddled by a repeated hour.
	if gz.localOffset(ms+int64(offset.total())*1000) == offset {
		return name
	}
	return z.partialLocationName(info, metazone, name)
}

// summerNear is TZGNCore's check that a zone in standard time keeps summer
// time around the instant: that it left summer time, or goes into it, within
// ICU's range.
func (z *timeZone) summerNear(ms int64) bool {
	if before, ok := z.previousTransition(ms, true); ok && ms-before.at < dstCheckRange && before.from.dst != 0 {
		return true
	}
	after, ok := z.nextTransition(ms, false)
	return ok && after.at-ms < dstCheckRange && after.to.dst != 0
}

// partialLocationName is TZGNCore::getPartialLocationName: a metazone's name
// qualified by where the zone is -- its region where it is the metazone's
// reference zone there, its city otherwise.
func (z *zoneNames) partialLocationName(info *zoneInfo, metazone, name string) string {
	var location string
	if country := info.country(); country != "" {
		if z.meta.Reference(metazone, country) == info.id {
			location = z.regionName(country)
		} else {
			location = z.exemplarCity(info.id)
		}
	} else {
		location = z.exemplarCity(info.id)
		if location == "" {
			location = info.id
		}
	}
	return simpleFormat(z.locale.FallbackFormat, location, name)
}

// country is the region a zone is in; empty for none.
func (info *zoneInfo) country() string {
	if info.meta == nil || info.meta.Region == "001" {
		return ""
	}
	return info.meta.Region
}

// locationName is TZGNCore::getGenericLocationName: a zone named by where it
// is. A region with one zone, or whose primary zone this is, is named by the
// region; any other zone by its city.
func (z *zoneNames) locationName(id string) string {
	mz, ok := z.meta.Zone(id)
	if !ok || mz.Region == "001" {
		return ""
	}
	if z.isPrimary(id, mz.Region) {
		return simpleFormat(z.locale.RegionFormat, z.regionName(mz.Region))
	}
	city := z.exemplarCity(id)
	if city == "" {
		return ""
	}
	return simpleFormat(z.locale.RegionFormat, city)
}

// isPrimary is ZoneMeta::getCanonicalCountry's answer to whether a zone
// stands for its region: the only canonical zone there, or the one CLDR
// names as primary.
func (z *zoneNames) isPrimary(id, region string) bool {
	return z.meta.ZonesIn(region) == 1 || z.meta.PrimaryZone(region) == id
}

func (z *zoneNames) regionName(code string) string {
	if name, ok := z.locale.Region(code); ok {
		return name
	}
	return code
}

// exemplarCity is TimeZoneNamesImpl::getExemplarLocationName: the city the
// locale names, or else the last part of the identifier with its underscores
// made spaces. Zones that are not places have none.
func (z *zoneNames) exemplarCity(id string) string {
	if e, ok := z.locale.Zone(id); ok && e.City != "" {
		return e.City
	}
	if id == "" || strings.HasPrefix(id, "Etc/") || strings.HasPrefix(id, "SystemV/") ||
		strings.Index(id, "Riyadh8") > 0 {
		return ""
	}
	sep := strings.LastIndexByte(id, '/')
	if sep <= 0 || sep+1 >= len(id) {
		return ""
	}
	return strings.ReplaceAll(id[sep+1:], "_", " ")
}
