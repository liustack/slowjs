// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// This file is a port of ICU's DateTimePatternGenerator (dtptngen.cpp), the
// part of ICU that turns a skeleton such as "yMMMd" into the pattern a locale
// writes it with ("MMM d, y" in en, "y年M月d日" in zh). Browsers build every
// Intl.DateTimeFormat pattern this way, so matching them means following the
// same matching rules, down to how ties are broken.

// The pattern fields, in ICU's UDateTimePatternField order.
const (
	fieldEra = iota
	fieldYear
	fieldQuarter
	fieldMonth
	fieldWeekOfYear
	fieldWeekOfMonth
	fieldWeekday
	fieldDayOfYear
	fieldDayOfWeekInMonth
	fieldDay
	fieldDayPeriod
	fieldHour
	fieldMinute
	fieldSecond
	fieldFractionalSecond
	fieldZone
	fieldCount
)

const (
	dateMask            = 1<<fieldDayPeriod - 1
	timeMask            = 1<<fieldCount - 1 - dateMask
	fractionalMask      = 1 << fieldFractionalSecond
	secondAndFractional = 1<<fieldSecond | fractionalMask
)

// The kinds of field, which the distance between two skeletons is measured
// in. A numeric field counts its length, a text one is one of the widths.
const (
	dtNarrow  = -0x101
	dtShorter = -0x102
	dtShort   = -0x103
	dtLong    = -0x104
	dtNumeric = 0x100
	dtDelta   = 0x10
)

const (
	extraField   = 0x10000
	missingField = 0x1000
)

type dtType struct {
	ch     byte
	field  int
	typ    int
	minLen int
}

// dtTypes is ICU's table of pattern letters. Rows for one letter are
// adjacent and ordered by length.
var dtTypes = []dtType{
	{'G', fieldEra, dtShort, 1},
	{'G', fieldEra, dtLong, 4},
	{'G', fieldEra, dtNarrow, 5},

	{'y', fieldYear, dtNumeric, 1},
	{'Y', fieldYear, dtNumeric + dtDelta, 1},
	{'u', fieldYear, dtNumeric + 2*dtDelta, 1},
	{'r', fieldYear, dtNumeric + 3*dtDelta, 1},
	{'U', fieldYear, dtShort, 1},
	{'U', fieldYear, dtLong, 4},
	{'U', fieldYear, dtNarrow, 5},

	{'Q', fieldQuarter, dtNumeric, 1},
	{'Q', fieldQuarter, dtShort, 3},
	{'Q', fieldQuarter, dtLong, 4},
	{'Q', fieldQuarter, dtNarrow, 5},
	{'q', fieldQuarter, dtNumeric + dtDelta, 1},
	{'q', fieldQuarter, dtShort - dtDelta, 3},
	{'q', fieldQuarter, dtLong - dtDelta, 4},
	{'q', fieldQuarter, dtNarrow - dtDelta, 5},

	{'M', fieldMonth, dtNumeric, 1},
	{'M', fieldMonth, dtShort, 3},
	{'M', fieldMonth, dtLong, 4},
	{'M', fieldMonth, dtNarrow, 5},
	{'L', fieldMonth, dtNumeric + dtDelta, 1},
	{'L', fieldMonth, dtShort - dtDelta, 3},
	{'L', fieldMonth, dtLong - dtDelta, 4},
	{'L', fieldMonth, dtNarrow - dtDelta, 5},
	{'l', fieldMonth, dtNumeric + dtDelta, 1},

	{'w', fieldWeekOfYear, dtNumeric, 1},

	{'W', fieldWeekOfMonth, dtNumeric, 1},

	{'E', fieldWeekday, dtShort, 1},
	{'E', fieldWeekday, dtLong, 4},
	{'E', fieldWeekday, dtNarrow, 5},
	{'E', fieldWeekday, dtShorter, 6},
	{'c', fieldWeekday, dtNumeric + 2*dtDelta, 1},
	{'c', fieldWeekday, dtShort - 2*dtDelta, 3},
	{'c', fieldWeekday, dtLong - 2*dtDelta, 4},
	{'c', fieldWeekday, dtNarrow - 2*dtDelta, 5},
	{'c', fieldWeekday, dtShorter - 2*dtDelta, 6},
	{'e', fieldWeekday, dtNumeric + dtDelta, 1},
	{'e', fieldWeekday, dtShort - dtDelta, 3},
	{'e', fieldWeekday, dtLong - dtDelta, 4},
	{'e', fieldWeekday, dtNarrow - dtDelta, 5},
	{'e', fieldWeekday, dtShorter - dtDelta, 6},

	{'d', fieldDay, dtNumeric, 1},
	{'g', fieldDay, dtNumeric + dtDelta, 1},

	{'D', fieldDayOfYear, dtNumeric, 1},

	{'F', fieldDayOfWeekInMonth, dtNumeric, 1},

	{'a', fieldDayPeriod, dtShort, 1},
	{'a', fieldDayPeriod, dtLong, 4},
	{'a', fieldDayPeriod, dtNarrow, 5},
	{'b', fieldDayPeriod, dtShort - dtDelta, 1},
	{'b', fieldDayPeriod, dtLong - dtDelta, 4},
	{'b', fieldDayPeriod, dtNarrow - dtDelta, 5},
	{'B', fieldDayPeriod, dtShort - 3*dtDelta, 1},
	{'B', fieldDayPeriod, dtLong - 3*dtDelta, 4},
	{'B', fieldDayPeriod, dtNarrow - 3*dtDelta, 5},

	{'H', fieldHour, dtNumeric + 10*dtDelta, 1},
	{'k', fieldHour, dtNumeric + 11*dtDelta, 1},
	{'h', fieldHour, dtNumeric, 1},
	{'K', fieldHour, dtNumeric + dtDelta, 1},
	{'J', fieldHour, dtNumeric + 5*dtDelta, 1},
	{'j', fieldHour, dtNumeric + 6*dtDelta, 1},
	{'C', fieldHour, dtNumeric + 7*dtDelta, 1},

	{'m', fieldMinute, dtNumeric, 1},

	{'s', fieldSecond, dtNumeric, 1},
	{'A', fieldSecond, dtNumeric + dtDelta, 1},

	{'S', fieldFractionalSecond, dtNumeric, 1},

	{'v', fieldZone, dtShort - 2*dtDelta, 1},
	{'v', fieldZone, dtLong - 2*dtDelta, 4},
	{'z', fieldZone, dtShort, 1},
	{'z', fieldZone, dtLong, 4},
	{'Z', fieldZone, dtNarrow - dtDelta, 1},
	{'Z', fieldZone, dtLong - dtDelta, 4},
	{'Z', fieldZone, dtShort - dtDelta, 5},
	{'O', fieldZone, dtShort - dtDelta, 1},
	{'O', fieldZone, dtLong - dtDelta, 4},
	{'V', fieldZone, dtShort - dtDelta, 1},
	{'V', fieldZone, dtLong - dtDelta, 2},
	{'V', fieldZone, dtLong - 1 - dtDelta, 3},
	{'V', fieldZone, dtLong - 2 - dtDelta, 4},
	{'X', fieldZone, dtNarrow - dtDelta, 1},
	{'X', fieldZone, dtShort - dtDelta, 2},
	{'X', fieldZone, dtLong - dtDelta, 4},
	{'x', fieldZone, dtNarrow - dtDelta, 1},
	{'x', fieldZone, dtShort - dtDelta, 2},
	{'x', fieldZone, dtLong - dtDelta, 4},
}

// canonicalItems are the single letter patterns every generator starts with.
const canonicalItems = "GyQMwWEDFdaHmsSv"

// appendItemKeys are the CLDR appendItems keys of the fields, "" for a field
// that has none.
var appendItemKeys = [fieldCount]string{
	"Era", "Year", "Quarter", "Month", "Week", "", "Day-Of-Week",
	"", "", "Day", "", "Hour", "Minute", "Second", "", "Timezone",
}

// fieldNameKeys are the CLDR dateFields keys of the fields.
var fieldNameKeys = [fieldCount]string{
	"era", "year", "quarter", "month", "week", "weekOfMonth", "weekday",
	"dayOfYear", "weekdayOfMonth", "day", "dayperiod", "hour", "minute", "second", "", "zone",
}

// canonicalIndex returns the dtTypes row of a run of n letters c, or -1.
func canonicalIndex(c byte, n int) int {
	for i := 0; i < len(dtTypes); i++ {
		if dtTypes[i].ch != c {
			continue
		}

		if i+1 == len(dtTypes) || dtTypes[i+1].ch != c {
			return i
		}

		if dtTypes[i+1].minLen <= n {
			continue
		}

		return i
	}
	return -1
}

// patternToken is one item of a pattern: a run of one ASCII letter, or any
// other single character.
type patternToken struct {
	s      string
	letter byte // the letter of a run, 0 for anything else
}

func tokenize(pattern string) (r []patternToken) {
	for i := 0; i < len(pattern); {
		c := pattern[i]
		if isASCIILetter(c) {
			j := i + 1
			for j < len(pattern) && pattern[j] == c {
				j++
			}
			r = append(r, patternToken{pattern[i:j], c})
			i = j
			continue
		}

		n := 1
		if c >= 0x80 {
			_, n = utf8.DecodeRuneInString(pattern[i:])
		}
		r = append(r, patternToken{s: pattern[i : i+n]})
		i += n
	}
	return r
}

func isASCIILetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// quoteLiteral returns the quoted literal starting at tokens[i], quotes
// included, and the index of its last token.
func quoteLiteral(tokens []patternToken, i int) (string, int) {
	var b strings.Builder
	if tokens[i].s == "'" {
		b.WriteString("'")
		i++
	}
	for ; i < len(tokens); i++ {
		if tokens[i].s == "'" {
			if i+1 < len(tokens) && tokens[i+1].s == "'" {
				b.WriteString("''")
				i++
				continue
			}

			b.WriteString("'")
			break
		}

		b.WriteString(tokens[i].s)
	}
	return b.String(), i
}

// skelFields holds one letter and length per field.
type skelFields struct {
	chars [fieldCount]byte
	lens  [fieldCount]int8
}

func (s *skelFields) set(field int, c byte, n int) { s.chars[field], s.lens[field] = c, int8(n) }

func (s *skelFields) String() string {
	var b strings.Builder
	for i := range s.chars {
		for j := 0; j < int(s.lens[i]); j++ {
			b.WriteByte(s.chars[i])
		}
	}
	return b.String()
}

func (s *skelFields) first() byte {
	for i := range s.lens {
		if s.lens[i] != 0 {
			return s.chars[i]
		}
	}
	return 0
}

type skeleton struct {
	typ                   [fieldCount]int
	original, base        skelFields
	addedDefaultDayPeriod bool
}

// newSkeleton parses a pattern or skeleton, ICU's DateTimeMatcher::set.
func newSkeleton(pattern string) *skeleton {
	s := &skeleton{}
	tokens := tokenize(pattern)
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.s == "'" {
			_, i = quoteLiteral(tokens, i)
			continue
		}

		if t.letter == 0 {
			continue
		}

		row := canonicalIndex(t.letter, len(t.s))
		if row < 0 {
			continue
		}

		d := dtTypes[row]
		s.original.set(d.field, t.letter, len(t.s))
		s.base.set(d.field, d.ch, d.minLen)
		sub := d.typ
		if sub > 0 {
			sub += len(t.s)
		}
		s.typ[d.field] = sub
	}

	// Minutes and fractions without seconds get the seconds.
	if s.original.lens[fieldMinute] != 0 && s.original.lens[fieldFractionalSecond] != 0 && s.original.lens[fieldSecond] == 0 {
		s.original.set(fieldSecond, 's', 1)
		s.base.set(fieldSecond, 's', 1)
		s.typ[fieldSecond] = dtNumeric + 1
	}

	// A 12 hour clock gets a day period, a 24 hour one loses it.
	if s.original.lens[fieldHour] != 0 {
		if c := s.original.chars[fieldHour]; c == 'h' || c == 'K' {
			if s.original.lens[fieldDayPeriod] == 0 {
				s.original.set(fieldDayPeriod, 'a', 1)
				s.base.set(fieldDayPeriod, 'a', 1)
				s.typ[fieldDayPeriod] = dtShort
				s.addedDefaultDayPeriod = true
			}
		} else {
			s.original.set(fieldDayPeriod, 0, 0)
			s.base.set(fieldDayPeriod, 0, 0)
			s.typ[fieldDayPeriod] = 0
		}
	}
	return s
}

// String is the skeleton without a day period newSkeleton added.
func (s *skeleton) String() string {
	r := s.original.String()
	if s.addedDefaultDayPeriod {
		if i := strings.IndexByte(r, 'a'); i >= 0 {
			r = r[:i] + r[i+1:]
		}
	}
	return r
}

func (s *skeleton) fieldMask() (m int) {
	for i, t := range s.typ {
		if t != 0 {
			m |= 1 << i
		}
	}
	return m
}

// distance is ICU's DateTimeMatcher::getDistance.
func (s *skeleton) distance(other *skeleton, includeMask int) (d, missing, extra int) {
	for i := 0; i < fieldCount; i++ {
		my := 0
		if includeMask&(1<<i) != 0 {
			my = s.typ[i]
		}
		o := other.typ[i]
		switch {
		case my == o:
		case my == 0:
			d += extraField
			extra |= 1 << i
		case o == 0:
			d += missingField
			missing |= 1 << i
		default:
			d += abs(my - o)
		}
	}
	return d, missing, extra
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}

type ptnElem struct {
	basePattern string
	skeleton    *skeleton
	pattern     string
	specified   bool // the skeleton came with the pattern, from availableFormats
}

// dtpg is a DateTimePatternGenerator for one locale, calendar and hour cycle
// keyword. It is immutable once built.
type dtpg struct {
	boot        [52][]*ptnElem
	appendItems [fieldCount]string
	fieldNames  [fieldCount]string
	dateTime    [4]string // full, long, medium, short
	decimal     string
	defaultHour byte     // h, H, k or K
	allowed     []string // the allowed hour formats, as in CLDR's timeData
}

var dtpgCache sync.Map

// generatorFor returns the generator of a locale's calendar data, with the
// hour cycle a -u-hc keyword asks for. std is false for the generator
// SimpleDateFormat builds, which leaves the locale's standard patterns out.
func generatorFor(c *calendarData, lang, region, hc string, decimal string, std bool) *dtpg {
	key := c.key() + "|" + lang + "|" + region + "|" + hc + "|" + decimal
	if !std {
		key += "|nostd"
	}
	if g, ok := dtpgCache.Load(key); ok {
		return g.(*dtpg)
	}

	g := &dtpg{decimal: decimal}
	for _, c := range canonicalItems {
		g.addPattern(string(c), "", false)
	}
	if std {
		for _, k := range []string{"timeFormats/full", "timeFormats/long", "timeFormats/medium", "timeFormats/short",
			"dateFormats/full", "dateFormats/long", "dateFormats/medium", "dateFormats/short"} {
			if p, ok := c.get(k); ok {
				g.addPattern(p, "", false)
			}
		}
	}
	for i, k := range appendItemKeys {
		g.appendItems[i] = "{0} ├{2}: {1}┤"
		if k != "" {
			if p, ok := c.get("dateTimeFormats/appendItems/" + k); ok && p != "" {
				g.appendItems[i] = p
			}
		}
	}
	for i, k := range fieldNameKeys {
		if n, ok := c.data.get("f/" + k); ok && k != "" && n != "" {
			g.fieldNames[i] = n
		} else {
			g.fieldNames[i] = "F" + strconv.Itoa(i)
		}
	}
	c.eachAvailableFormat(func(skel, pattern string) {
		g.addPattern(pattern, skel, true)
	})
	for i, style := range []string{"full", "long", "medium", "short"} {
		if p, ok := c.get("dateTimeFormats-atTime/standard/" + style); ok {
			g.dateTime[i] = p
		} else {
			g.dateTime[i], _ = c.get("dateTimeFormats/" + style)
		}
	}
	g.setHourFormats(lang, region, hc)

	v, _ := dtpgCache.LoadOrStore(key, g)
	return v.(*dtpg)
}

// setHourFormats is ICU's getAllowedHourFormats: the hour cycle a locale
// prefers and the ones it allows, from CLDR's timeData.
func (g *dtpg) setHourFormats(lang, region, hc string) {
	s := cldr().supplemental()
	td, ok := s.get("td/" + lang + "-" + region)
	if !ok {
		td, ok = s.get("td/" + region)
	}
	if !ok {
		if alias, found := supp().territoryAlias[region]; found {
			region = strings.Fields(alias)[0]
			if td, ok = s.get("td/" + lang + "-" + region); !ok {
				td, ok = s.get("td/" + region)
			}
		}
	}
	switch hc {
	case "h11":
		g.defaultHour = 'K'
	case "h12":
		g.defaultHour = 'h'
	case "h23":
		g.defaultHour = 'H'
	case "h24":
		g.defaultHour = 'k'
	}
	if !ok {
		if g.defaultHour == 0 {
			g.defaultHour = 'H'
		}
		g.allowed = []string{"H"}
		return
	}

	preferred, allowed, _ := strings.Cut(td, ";")
	g.allowed = strings.Fields(allowed)
	if preferred == "" && len(g.allowed) > 0 {
		preferred = g.allowed[0]
	}
	if g.defaultHour == 0 {
		switch preferred {
		case "h", "K", "k":
			g.defaultHour = preferred[0]
		default:
			g.defaultHour = 'H'
		}
	}
}

// hourCycle returns the hour cycle the generator defaults to.
func (g *dtpg) hourCycle() string {
	switch g.defaultHour {
	case 'K':
		return "h11"
	case 'h':
		return "h12"
	case 'k':
		return "h24"
	}
	return "h23"
}

// addPattern is ICU's addPatternWithOptionalSkeleton. skel is "" for a
// pattern whose skeleton is derived from the pattern itself.
func (g *dtpg) addPattern(pattern, skel string, override bool) {
	var s *skeleton
	if skel == "" {
		s = newSkeleton(pattern)
	} else {
		s = newSkeleton(skel)
	}
	base := s.base.String()
	if base == "" {
		return
	}

	if dup, specified := g.byBase(base); dup != nil && (!specified || skel != "" && !override) {
		if !override {
			return
		}
	}

	if dup, specified := g.bySkeleton(s); dup != nil {
		if !override || skel != "" && specified {
			return
		}
	}

	i := bootIndex(base[0])
	if i < 0 {
		return
	}

	for _, e := range g.boot[i] {
		if e.basePattern == base && e.skeleton.typ == s.typ {
			e.pattern = pattern
			e.specified = skel != ""
			return
		}
	}
	g.boot[i] = append(g.boot[i], &ptnElem{basePattern: base, skeleton: s, pattern: pattern, specified: skel != ""})
}

func bootIndex(c byte) int {
	switch {
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= 'a' && c <= 'z':
		return 26 + int(c-'a')
	}
	return -1
}

func (g *dtpg) byBase(base string) (*ptnElem, bool) {
	i := bootIndex(base[0])
	if i < 0 {
		return nil, false
	}

	for _, e := range g.boot[i] {
		if e.basePattern == base {
			return e, e.specified
		}
	}
	return nil, false
}

// bySkeleton finds the entry with s's exact fields and reports whether its
// skeleton was specified.
func (g *dtpg) bySkeleton(s *skeleton) (*ptnElem, bool) {
	i := bootIndex(s.base.first())
	if i < 0 {
		return nil, false
	}

	for _, e := range g.boot[i] {
		if e.skeleton.original == s.original {
			return e, e.specified
		}
	}
	return nil, false
}

// bestRaw is ICU's getBestRaw: the entry closest to the fields of source in
// includeMask.
func (g *dtpg) bestRaw(source *skeleton, includeMask int) (best *ptnElem, missing, extra int) {
	bestDistance, bestMissing := int(^uint(0)>>1), -1
	for i := range g.boot {
		for _, e := range g.boot[i] {
			d, m, x := source.distance(e.skeleton, includeMask)
			if d < bestDistance || d == bestDistance && bestMissing < m {
				bestDistance, bestMissing = d, m
				best, missing, extra = e, m, x
				if d == 0 {
					return best, missing, extra
				}
			}
		}
	}
	return best, missing, extra
}

const (
	flagFixFractionalSeconds = 1 << iota
	flagSkeletonUsesCapJ
)

// adjustFieldTypes is ICU's adjustFieldTypes with the
// UDATPG_MATCH_HOUR_FIELD_LENGTH option, the one browsers pass: it gives the
// pattern found the field widths the request asked for.
func (g *dtpg) adjustFieldTypes(pattern string, specified *skeleton, req *skeleton, flags int) string {
	var b strings.Builder
	tokens := tokenize(pattern)
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if t.s == "'" {
			var lit string
			lit, i = quoteLiteral(tokens, i)
			b.WriteString(lit)
			continue
		}

		row := -1
		if t.letter != 0 {
			row = canonicalIndex(t.letter, len(t.s))
		}
		if row < 0 {
			b.WriteString(t.s)
			continue
		}

		d := dtTypes[row]
		field := t.s
		switch {
		case flags&flagFixFractionalSeconds != 0 && d.field == fieldSecond:
			field += g.decimal
			for j := 0; j < int(req.original.lens[fieldFractionalSecond]); j++ {
				field += string(req.original.chars[fieldFractionalSecond])
			}
		case req.typ[d.field] != 0:
			reqChar := req.original.chars[d.field]
			reqLen := int(req.original.lens[d.field])
			if reqChar == 'E' && reqLen < 3 {
				reqLen = 3
			}
			adjLen := reqLen
			switch {
			case d.field == fieldMinute || d.field == fieldSecond:
				adjLen = len(field)
			case specified != nil && reqChar != 'c' && reqChar != 'e':
				skelLen := int(specified.original.lens[d.field])
				patNumeric := d.typ > 0
				reqNumeric := req.typ[d.field] > 0
				if skelLen == reqLen || patNumeric != reqNumeric {
					adjLen = len(field)
				}
			}
			c := reqChar
			if d.field == fieldHour || d.field == fieldMonth || d.field == fieldWeekday || d.field == fieldYear && reqChar != 'Y' {
				c = field[0]
			}
			if c == 'E' && adjLen < 3 {
				c = 'e'
			}
			if d.field == fieldHour && g.defaultHour != 0 {
				switch {
				case flags&flagSkeletonUsesCapJ != 0 || reqChar == g.defaultHour:
					c = g.defaultHour
				case reqChar == 'h' && g.defaultHour == 'K':
					c = 'K'
				case reqChar == 'H' && g.defaultHour == 'k':
					c = 'k'
				case reqChar == 'k' && g.defaultHour == 'H':
					c = 'H'
				case reqChar == 'K' && g.defaultHour == 'h':
					c = 'h'
				}
			}
			field = strings.Repeat(string(c), adjLen)
		}
		b.WriteString(field)
	}
	return b.String()
}

// bestAppending is ICU's getBestAppending: a pattern for the fields in
// missingFields, made of several patterns joined with the appendItems ones
// when no single one has them all.
func (g *dtpg) bestAppending(req *skeleton, missingFields, flags int) string {
	if missingFields == 0 {
		return ""
	}

	e, missing, _ := g.bestRaw(req, missingFields)
	result := g.adjustFieldTypes(e.pattern, specifiedOf(e), req, flags)
	last := 0
	for missing != 0 {
		if last == missing {
			break
		}

		if missing&secondAndFractional == fractionalMask && missingFields&secondAndFractional == secondAndFractional {
			result = g.adjustFieldTypes(result, specifiedOf(e), req, flags|flagFixFractionalSeconds)
			missing &^= fractionalMask
			continue
		}

		start := missing
		e, missing, _ = g.bestRaw(req, missing)
		temp := g.adjustFieldTypes(e.pattern, specifiedOf(e), req, flags)
		top := topBit(start &^ missing)
		if g.appendItems[top] != "" {
			result = simpleFormat(g.appendItems[top], result, temp, "'"+g.fieldNames[top]+"'")
		}
		last = missing
	}
	return result
}

func specifiedOf(e *ptnElem) *skeleton {
	if e.specified {
		return e.skeleton
	}

	return nil
}

func topBit(mask int) int {
	if mask == 0 {
		return 0
	}

	i := 0
	for mask != 0 {
		mask >>= 1
		i++
	}
	if i-1 > fieldZone {
		return fieldZone
	}

	return i - 1
}

// bestPattern is ICU's getBestPattern with UDATPG_MATCH_HOUR_FIELD_LENGTH.
func (g *dtpg) bestPattern(skel string) string {
	flags := 0
	mapped := g.mapMetacharacters(skel, &flags)
	req := newSkeleton(mapped)
	e, missing, extra := g.bestRaw(req, -1)
	if missing == 0 && extra == 0 {
		return g.adjustFieldTypes(e.pattern, specifiedOf(e), req, flags)
	}

	needed := req.fieldMask()
	date := g.bestAppending(req, needed&dateMask, flags)
	tim := g.bestAppending(req, needed&timeMask, flags)
	switch {
	case date == "":
		return tim
	case tim == "":
		return date
	}

	style := 3
	switch req.base.lens[fieldMonth] {
	case 4:
		style = 1
		if req.base.lens[fieldWeekday] > 0 {
			style = 0
		}
	case 3:
		style = 2
	}
	return simpleFormat(g.dateTime[style], tim, date)
}

// mapMetacharacters replaces the skeleton letters j, C and J with the hour
// and day period letters the locale uses.
func (g *dtpg) mapMetacharacters(skel string, flags *int) string {
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(skel); i++ {
		c := skel[i]
		switch {
		case c == '\'':
			inQuote = !inQuote
		case inQuote:
		case c == 'j' || c == 'C':
			extra := 0
			for i+1 < len(skel) && skel[i+1] == c {
				extra++
				i++
			}
			hourLen := 1 + extra&1
			dayPeriodLen := 1
			if extra >= 2 {
				dayPeriodLen = 3 + extra>>1
			}
			hourChar, dayPeriodChar := byte('h'), byte('a')
			if c == 'j' {
				hourChar = g.defaultHour
			} else {
				best := g.allowed[0]
				switch best {
				case "H", "HB", "Hb":
					hourChar = 'H'
				case "K", "KB", "Kb":
					hourChar = 'K'
				case "k":
					hourChar = 'k'
				}
				switch best {
				case "HB", "hB", "KB":
					dayPeriodChar = 'B'
				case "Hb", "hb", "Kb":
					dayPeriodChar = 'b'
				}
			}
			if hourChar == 'H' || hourChar == 'k' {
				dayPeriodLen = 0
			}
			b.WriteString(strings.Repeat(string(dayPeriodChar), dayPeriodLen))
			b.WriteString(strings.Repeat(string(hourChar), hourLen))
		case c == 'J':
			b.WriteByte('H')
			*flags |= flagSkeletonUsesCapJ
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// simpleFormat is ICU's SimpleFormatter: {0}, {1}... replaced by args. An
// apostrophe quotes only before a brace or another apostrophe, so the quotes
// of a date pattern pass through.
func simpleFormat(pattern string, args ...string) string {
	var b strings.Builder
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '\'' && i+1 < len(pattern) && pattern[i+1] == '\'':
			b.WriteByte('\'')
			i++
		case c == '\'' && i+1 < len(pattern) && (pattern[i+1] == '{' || pattern[i+1] == '}'):
			j := strings.IndexByte(pattern[i+1:], '\'')
			if j < 0 {
				b.WriteString(pattern[i+1:])
				return b.String()
			}

			b.WriteString(pattern[i+1 : i+1+j])
			i += j + 1
		case c == '{' && i+2 < len(pattern) && pattern[i+2] == '}' && pattern[i+1] >= '0' && int(pattern[i+1]-'0') < len(args):
			b.WriteString(args[pattern[i+1]-'0'])
			i += 2
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
