// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strings"
	"sync"
	"time"
)

// Time zone names as ICU's TimeZoneFormat writes them (tzfmt.cpp,
// tzgnames.cpp, tznames_impl.cpp): a name specific to the zone, else the
// metazone's ("Eastern Daylight Time"), else the location ("Japan Time"),
// else a GMT offset.

// zoneNamer formats the time zone names of one locale.
type zoneNamer struct {
	d      *cldrData
	region string   // the locale's region, explicit or likely
	digits []string // the locale's digits, for GMT offsets
}

// metazoneUse is one period of a zone's metazone history.
type metazoneUse struct {
	mz       string
	from, to int64 // unix seconds
}

var metazones = sync.OnceValue(func() map[string][]metazoneUse {
	r := map[string][]metazoneUse{}
	s := cldr().supplemental()
	s.each("mz/", func(k, v string) {
		var uses []metazoneUse
		for _, u := range strings.Split(v, ";") {
			p := strings.Split(u, "|")
			uses = append(uses, metazoneUse{p[0], parseMetazoneTime(p[1], "1970-01-01 00:00"), parseMetazoneTime(p[2], "9999-12-31 23:59")})
		}
		r[k[3:]] = uses
	})
	return r
})

// parseMetazoneTime parses a bound of a metazone period. A missing bound is
// ICU's default (zonemeta.cpp): no zone is in a metazone before 1970, so
// older times get a GMT offset or a location rather than "China Standard
// Time".
func parseMetazoneTime(s, missing string) int64 {
	if s == "" {
		s = missing
	}

	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic("intl: bad metazone time " + s)
	}

	return t.Unix()
}

// zoneCountries counts the canonical zones per country.
var zoneCountries = sync.OnceValue(func() map[string]int {
	r := map[string]int{}
	cldr().supplemental().each("zr/", func(_, region string) { r[region]++ })
	return r
})

func zoneRegionOf(id string) string { return cldr().supplemental().str("zr/" + id) }

// metazone returns the metazone zone id is in at t.
func metazone(id string, t time.Time) string {
	u := t.Unix()
	for _, m := range metazones()[id] {
		if m.from <= u && u < m.to {
			return m.mz
		}
	}
	return ""
}

// referenceZone returns the zone metazone mz stands for in region.
func referenceZone(mz, region string) string {
	var world string
	for _, e := range strings.Fields(cldr().supplemental().str("mzr/" + mz)) {
		r, zone, _ := strings.Cut(e, ":")
		switch r {
		case region:
			return zone
		case "001":
			world = zone
		}
	}
	return world
}

// zoneName returns the name zone id itself has, ICU's
// getTimeZoneDisplayName. kind is "long/standard" and the like.
func (z *zoneNamer) zoneName(id, kind string) string {
	return z.d.str("z/zone/" + id + "/" + kind)
}

func (z *zoneNamer) metazoneName(mz, kind string) string {
	if mz == "" {
		return ""
	}

	return z.d.str("z/metazone/" + mz + "/" + kind)
}

// displayName is TimeZoneNames::getDisplayName: the zone's own name, else its
// metazone's at t.
func (z *zoneNamer) displayName(id, kind string, t time.Time) string {
	if n := z.zoneName(id, kind); n != "" {
		return n
	}

	return z.metazoneName(metazone(id, t), kind)
}

// exemplarCity returns the city a zone is named after in the locale.
func (z *zoneNamer) exemplarCity(id string) string {
	if c := z.d.str("z/zone/" + id + "/exemplarCity"); c != "" {
		return c
	}

	if strings.HasPrefix(id, "Etc/") || strings.HasPrefix(id, "SystemV/") || strings.Contains(id[1:], "Riyadh8") {
		return ""
	}

	i := strings.LastIndexByte(id, '/')
	if i <= 0 || i+1 == len(id) {
		return ""
	}

	return strings.ReplaceAll(id[i+1:], "_", " ")
}

// location is ICU's generic location name: "Japan Time", or for a country
// with several zones "New York Time".
func (z *zoneNamer) location(id string) string {
	country := zoneRegionOf(id)
	if country == "" {
		return ""
	}

	var place string
	if zoneCountries()[country] == 1 || cldr().supplemental().str("pz/"+country) == id {
		place = z.d.str("t/" + country)
		if place == "" {
			place = country
		}
	} else {
		place = z.exemplarCity(id)
	}
	if place == "" {
		return ""
	}

	return simpleFormat(z.format("regionFormat", "{0}"), place)
}

func (z *zoneNamer) format(key, def string) string {
	if s := z.d.str("z/" + key); s != "" {
		return s
	}

	return def
}

// specific is the z and zzzz pattern letters.
func (z *zoneNamer) specific(id string, t time.Time, long bool) string {
	kind := "short/"
	if long {
		kind = "long/"
	}
	if isDST(t) {
		kind += "daylight"
	} else {
		kind += "standard"
	}
	if id != "" {
		if n := z.displayName(id, kind, t); n != "" {
			return n
		}
	}

	_, offset := t.Zone()
	return z.gmt(offset, !long)
}

// generic is the v and vvvv pattern letters.
func (z *zoneNamer) generic(id string, t time.Time, long bool) string {
	if id != "" {
		if n := z.genericNonLocation(id, t, long); n != "" {
			return n
		}

		if n := z.location(id); n != "" {
			return n
		}
	}

	_, offset := t.Zone()
	return z.gmt(offset, !long)
}

// dstCheckRange is how far around an instant ICU looks for daylight saving
// time before calling a zone's time "standard" rather than generic.
const dstCheckRange = 184 * 24 * time.Hour

// genericNonLocation is ICU's formatGenericNonLocationName.
func (z *zoneNamer) genericNonLocation(id string, t time.Time, long bool) string {
	width := "short/"
	if long {
		width = "long/"
	}
	if n := z.zoneName(id, width+"generic"); n != "" {
		return n
	}

	mz := metazone(id, t)
	if mz == "" {
		return ""
	}

	var name string
	if !isDST(t) && !dstNear(t) {
		// A zone without daylight saving time around t is named by its
		// standard time, unless that reads the same as the generic name.
		if std := z.displayName(id, width+"standard", t); std != "" && !strings.EqualFold(std, z.metazoneName(mz, width+"generic")) {
			name = std
		}
	}
	if name != "" {
		return name
	}

	mzName := z.metazoneName(mz, width+"generic")
	if mzName == "" {
		return ""
	}

	golden := referenceZone(mz, z.region)
	if golden == "" || golden == id {
		return mzName
	}

	// The metazone's name stands for its reference zone. A zone of it whose
	// time differs at t gets the name with its location attached.
	if loc, err := time.LoadLocation(golden); err == nil {
		_, off := t.Zone()
		gt := t.In(loc)
		if _, goff := gt.Zone(); goff == off && isDST(gt) == isDST(t) {
			return mzName
		}
	}

	return z.partialLocation(id, mz, mzName)
}

// dstNear reports whether t's zone observes daylight saving time in a
// transition less than dstCheckRange away: the time just before t's offset
// and flag began or just after they end. A transition where neither changes,
// which the data Go reads can have, is passed over.
func dstNear(t time.Time) bool {
	_, offset := t.Zone()
	dst := isDST(t)
	same := func(u time.Time) bool {
		_, o := u.Zone()
		return o == offset && isDST(u) == dst
	}

	for c := t; ; {
		start, _ := periodBounds(c)
		if start.IsZero() || t.Sub(start) >= dstCheckRange {
			break
		}
		before := start.Add(-time.Nanosecond)
		if !same(before) {
			if isDST(before) {
				return true
			}
			break
		}
		c = before
	}

	for c := t; ; {
		_, end := periodBounds(c)
		if end.IsZero() || end.Sub(t) >= dstCheckRange {
			return false
		}
		if !same(end) {
			return isDST(end)
		}
		c = end
	}
}

// partialLocation is ICU's getPartialLocationName: "Mountain Time
// (Phoenix)".
func (z *zoneNamer) partialLocation(id, mz, mzName string) string {
	var place string
	if country := zoneRegionOf(id); country != "" {
		if referenceZone(mz, country) == id {
			place = z.d.str("t/" + country)
			if place == "" {
				place = country
			}
		} else {
			place = z.exemplarCity(id)
		}
	} else if place = z.exemplarCity(id); place == "" {
		place = id
	}
	return simpleFormat(z.format("fallbackFormat", "{1} ({0})"), place, mzName)
}

// gmt is ICU's localized GMT format: "GMT+2", "GMT+05:30".
func (z *zoneNamer) gmt(offset int, short bool) string {
	positive := offset >= 0
	if !positive {
		offset = -offset
	}
	h, m, s := offset/3600, offset%3600/60, offset%60
	pos, neg, _ := strings.Cut(z.format("hourFormat", "+H:mm;-H:mm"), ";")
	hm := pos
	if !positive {
		hm = neg
	}
	var pattern string
	switch {
	case s != 0:
		pattern = expandOffsetPattern(hm)
	case m != 0 || !short:
		pattern = hm
	default:
		pattern = truncateOffsetPattern(hm)
	}

	var b strings.Builder
	inQuote := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '\'':
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}

			inQuote = !inQuote
		case inQuote:
			b.WriteByte(c)
		case c == 'H' || c == 'm' || c == 's':
			j := i
			for j+1 < len(pattern) && pattern[j+1] == c {
				j++
			}
			i = j
			switch c {
			case 'H':
				if short {
					z.appendDigits(&b, h, 1)
				} else {
					z.appendDigits(&b, h, 2)
				}
			case 'm':
				z.appendDigits(&b, m, 2)
			default:
				z.appendDigits(&b, s, 2)
			}
		default:
			b.WriteByte(c)
		}
	}
	gmtFormat := z.format("gmtFormat", "GMT{0}")
	prefix, suffix, _ := strings.Cut(gmtFormat, "{0}")
	return unquote(prefix) + b.String() + unquote(suffix)
}

func (z *zoneNamer) appendDigits(b *strings.Builder, n, min int) {
	if n < 10 && min == 2 {
		b.WriteString(z.digits[0])
	}
	if n >= 10 {
		b.WriteString(z.digits[n/10])
	}
	b.WriteString(z.digits[n%10])
}

// expandOffsetPattern adds seconds to an hours and minutes pattern.
func expandOffsetPattern(hm string) string {
	i := strings.Index(hm, "mm")
	if i < 0 {
		return hm
	}

	sep := ""
	if h := strings.LastIndexByte(hm[:i], 'H'); h >= 0 {
		sep = hm[h+1 : i]
	}
	return hm[:i+2] + sep + "ss" + hm[i+2:]
}

// truncateOffsetPattern drops the minutes of an hours and minutes pattern.
func truncateOffsetPattern(hm string) string {
	i := strings.Index(hm, "mm")
	if i < 0 {
		return hm
	}

	if h := strings.LastIndex(hm[:i], "HH"); h >= 0 {
		return hm[:h+2]
	}

	if h := strings.LastIndexByte(hm[:i], 'H'); h >= 0 {
		return hm[:h+1]
	}

	return hm
}

// unquote removes the quoting of a pattern literal.
func unquote(s string) string {
	if !strings.Contains(s, "'") {
		return s
	}

	var b strings.Builder
	prevQuote := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			if prevQuote {
				b.WriteByte('\'')
				prevQuote = false
			} else {
				prevQuote = true
			}
			continue
		}

		prevQuote = false
		b.WriteByte(s[i])
	}
	return b.String()
}
