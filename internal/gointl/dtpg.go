package intl

import (
	"strconv"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/datedata"
)

// ICU's date-time pattern generator, ported from dtptngen.cpp in ICU 78.3.
//
// A caller that names fields -- a year, a long month, a day -- gets a pattern
// that puts them in the locale's order with the locale's punctuation. UTS #35
// describes how in outline; Node answers with ICU's particular
// implementation of it, whose distance weights, tie-breaking, field
// adjustment and appending rules decide the answer in the cases the outline
// leaves open. So this follows the C++ closely, names and all, and departs
// from it only where Go needs it to.

// The pattern generator's fields, UDateTimePatternField.
const (
	dtpgEra = iota
	dtpgYear
	dtpgQuarter
	dtpgMonth
	dtpgWeekOfYear
	dtpgWeekOfMonth
	dtpgWeekday
	dtpgDayOfYear
	dtpgDayOfWeekInMonth
	dtpgDay
	dtpgDayPeriod
	dtpgHour
	dtpgMinute
	dtpgSecond
	dtpgFractionalSecond
	dtpgZone
	dtpgFieldCount
)

// The type values of dtTypes, from dtptngen_impl.h.
const (
	dtNarrow   = -0x101
	dtShorter  = -0x102
	dtShort    = -0x103
	dtLong     = -0x104
	dtNumeric  = 0x100
	dtDelta    = 0x10
	extraField = 0x10000
	missField  = 0x1000
	maxDTToken = 50

	fractionalMask          = 1 << dtpgFractionalSecond
	secondAndFractionalMask = 1<<dtpgSecond | 1<<dtpgFractionalSecond

	// The match options, UDateTimePatternMatchOptions.
	matchHourFieldLength   = 1 << dtpgHour
	matchMinuteFieldLength = 1 << dtpgMinute
	matchSecondFieldLength = 1 << dtpgSecond

	// The internal flags.
	flagFixFractionalSeconds = 1
	flagSkeletonUsesCapJ     = 2
)

// canonicalItems are the single letters the generator is seeded with.
const canonicalItems = "GyQMwWEDFdaHmsSv"

type dtType struct {
	ch     byte
	field  int
	typ    int32
	minLen int
}

// dtTypes is ICU's table of pattern letters, in its order: a letter's rows are
// adjacent, by increasing length.
var dtTypes = []dtType{
	{'G', dtpgEra, dtShort, 1},
	{'G', dtpgEra, dtLong, 4},
	{'G', dtpgEra, dtNarrow, 5},

	{'y', dtpgYear, dtNumeric, 1},
	{'Y', dtpgYear, dtNumeric + dtDelta, 1},
	{'u', dtpgYear, dtNumeric + 2*dtDelta, 1},
	{'r', dtpgYear, dtNumeric + 3*dtDelta, 1},
	{'U', dtpgYear, dtShort, 1},
	{'U', dtpgYear, dtLong, 4},
	{'U', dtpgYear, dtNarrow, 5},

	{'Q', dtpgQuarter, dtNumeric, 1},
	{'Q', dtpgQuarter, dtShort, 3},
	{'Q', dtpgQuarter, dtLong, 4},
	{'Q', dtpgQuarter, dtNarrow, 5},
	{'q', dtpgQuarter, dtNumeric + dtDelta, 1},
	{'q', dtpgQuarter, dtShort - dtDelta, 3},
	{'q', dtpgQuarter, dtLong - dtDelta, 4},
	{'q', dtpgQuarter, dtNarrow - dtDelta, 5},

	{'M', dtpgMonth, dtNumeric, 1},
	{'M', dtpgMonth, dtShort, 3},
	{'M', dtpgMonth, dtLong, 4},
	{'M', dtpgMonth, dtNarrow, 5},
	{'L', dtpgMonth, dtNumeric + dtDelta, 1},
	{'L', dtpgMonth, dtShort - dtDelta, 3},
	{'L', dtpgMonth, dtLong - dtDelta, 4},
	{'L', dtpgMonth, dtNarrow - dtDelta, 5},
	{'l', dtpgMonth, dtNumeric + dtDelta, 1},

	{'w', dtpgWeekOfYear, dtNumeric, 1},

	{'W', dtpgWeekOfMonth, dtNumeric, 1},

	{'E', dtpgWeekday, dtShort, 1},
	{'E', dtpgWeekday, dtLong, 4},
	{'E', dtpgWeekday, dtNarrow, 5},
	{'E', dtpgWeekday, dtShorter, 6},
	{'c', dtpgWeekday, dtNumeric + 2*dtDelta, 1},
	{'c', dtpgWeekday, dtShort - 2*dtDelta, 3},
	{'c', dtpgWeekday, dtLong - 2*dtDelta, 4},
	{'c', dtpgWeekday, dtNarrow - 2*dtDelta, 5},
	{'c', dtpgWeekday, dtShorter - 2*dtDelta, 6},
	{'e', dtpgWeekday, dtNumeric + dtDelta, 1},
	{'e', dtpgWeekday, dtShort - dtDelta, 3},
	{'e', dtpgWeekday, dtLong - dtDelta, 4},
	{'e', dtpgWeekday, dtNarrow - dtDelta, 5},
	{'e', dtpgWeekday, dtShorter - dtDelta, 6},

	{'d', dtpgDay, dtNumeric, 1},
	{'g', dtpgDay, dtNumeric + dtDelta, 1},

	{'D', dtpgDayOfYear, dtNumeric, 1},

	{'F', dtpgDayOfWeekInMonth, dtNumeric, 1},

	{'a', dtpgDayPeriod, dtShort, 1},
	{'a', dtpgDayPeriod, dtLong, 4},
	{'a', dtpgDayPeriod, dtNarrow, 5},
	{'b', dtpgDayPeriod, dtShort - dtDelta, 1},
	{'b', dtpgDayPeriod, dtLong - dtDelta, 4},
	{'b', dtpgDayPeriod, dtNarrow - dtDelta, 5},
	{'B', dtpgDayPeriod, dtShort - 3*dtDelta, 1},
	{'B', dtpgDayPeriod, dtLong - 3*dtDelta, 4},
	{'B', dtpgDayPeriod, dtNarrow - 3*dtDelta, 5},

	{'H', dtpgHour, dtNumeric + 10*dtDelta, 1},
	{'k', dtpgHour, dtNumeric + 11*dtDelta, 1},
	{'h', dtpgHour, dtNumeric, 1},
	{'K', dtpgHour, dtNumeric + dtDelta, 1},
	{'J', dtpgHour, dtNumeric + 5*dtDelta, 1},
	{'j', dtpgHour, dtNumeric + 6*dtDelta, 1},
	{'C', dtpgHour, dtNumeric + 7*dtDelta, 1},

	{'m', dtpgMinute, dtNumeric, 1},

	{'s', dtpgSecond, dtNumeric, 1},
	{'A', dtpgSecond, dtNumeric + dtDelta, 1},

	{'S', dtpgFractionalSecond, dtNumeric, 1},

	{'v', dtpgZone, dtShort - 2*dtDelta, 1},
	{'v', dtpgZone, dtLong - 2*dtDelta, 4},
	{'z', dtpgZone, dtShort, 1},
	{'z', dtpgZone, dtLong, 4},
	{'Z', dtpgZone, dtNarrow - dtDelta, 1},
	{'Z', dtpgZone, dtLong - dtDelta, 4},
	{'Z', dtpgZone, dtShort - dtDelta, 5},
	{'O', dtpgZone, dtShort - dtDelta, 1},
	{'O', dtpgZone, dtLong - dtDelta, 4},
	{'V', dtpgZone, dtShort - dtDelta, 1},
	{'V', dtpgZone, dtLong - dtDelta, 2},
	{'V', dtpgZone, dtLong - 1 - dtDelta, 3},
	{'V', dtpgZone, dtLong - 2 - dtDelta, 4},
	{'X', dtpgZone, dtNarrow - dtDelta, 1},
	{'X', dtpgZone, dtShort - dtDelta, 2},
	{'X', dtpgZone, dtLong - dtDelta, 4},
	{'x', dtpgZone, dtNarrow - dtDelta, 1},
	{'x', dtpgZone, dtShort - dtDelta, 2},
	{'x', dtpgZone, dtLong - dtDelta, 4},
}

// The default append item, "{0} ├{2}: {1}┤", for a field the locale gives
// none for.
const defaultAppendItem = "{0} \u251c{2}: {1}\u2524"

func isASCIILetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

// formatParser is ICU's FormatParser: a pattern taken apart into runs of one
// letter and single other characters, at most fifty of them.
type formatParser struct {
	items []string
}

func (fp *formatParser) set(pattern string) {
	fp.items = fp.items[:0]
	for start := 0; start < len(pattern) && len(fp.items) < maxDTToken; {
		c := pattern[start]
		n := 1
		if isASCIILetter(c) {
			for start+n < len(pattern) && pattern[start+n] == c {
				n++
			}
		} else if c >= 0x80 {
			// A character outside ASCII is one token, whatever its length
			// in bytes: ICU counts UTF-16 units, and a letter test on one
			// of those would fail the same way.
			for start+n < len(pattern) && pattern[start+n]&0xc0 == 0x80 {
				n++
			}
		}
		fp.items = append(fp.items, pattern[start:start+n])
		start += n
	}
}

// canonicalIndex is getCanonicalIndex with strict set: the dtTypes row a run
// of one letter belongs to, or -1.
func canonicalIndex(s string) int {
	if s == "" || !isASCIILetter(s[0]) {
		return -1
	}
	ch := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] != ch {
			return -1
		}
	}
	for i := 0; i < len(dtTypes); i++ {
		if dtTypes[i].ch != ch {
			continue
		}
		if i+1 == len(dtTypes) || dtTypes[i+1].ch != ch {
			return i
		}
		if dtTypes[i+1].minLen <= len(s) {
			continue
		}
		return i
	}
	return -1
}

func isQuoteLiteral(s string) bool { return s != "" && s[0] == '\'' }

// quoteLiteral gathers the items of a quoted literal starting at i, and
// returns it and the index of its last item.
func (fp *formatParser) quoteLiteral(i int) (string, int) {
	var b strings.Builder
	if fp.items[i][0] == '\'' {
		b.WriteString(fp.items[i])
		i++
	}
	for i < len(fp.items) {
		if fp.items[i][0] == '\'' {
			if i+1 < len(fp.items) && fp.items[i+1][0] == '\'' {
				// Two quotes, one escaped: 'o''clock'.
				b.WriteString(fp.items[i])
				b.WriteString(fp.items[i+1])
				i += 2
				continue
			}
			b.WriteString(fp.items[i])
			break
		}
		b.WriteString(fp.items[i])
		i++
	}
	return b.String(), i
}

// isPatternSeparator is ICU's, including its test of items[i] for a dot,
// where i indexes the field's characters: that is what ICU does.
func (fp *formatParser) isPatternSeparator(field string) bool {
	for i := 0; i < len(field); i++ {
		switch field[i] {
		case '\'', '\\', ' ', ':', '"', ',', '-':
			continue
		}
		if i < len(fp.items) && fp.items[i] != "" && fp.items[i][0] == '.' {
			continue
		}
		return false
	}
	return true
}

// skeletonFields is ICU's SkeletonFields: each field's letter and length.
type skeletonFields struct {
	chars   [dtpgFieldCount]byte
	lengths [dtpgFieldCount]int8
}

func (s *skeletonFields) populate(field int, ch byte, length int) {
	s.chars[field] = ch
	s.lengths[field] = int8(length)
}

func (s *skeletonFields) clearField(field int) {
	s.chars[field] = 0
	s.lengths[field] = 0
}

func (s *skeletonFields) empty(field int) bool { return s.lengths[field] == 0 }

func (s *skeletonFields) appendFieldTo(field int, b *strings.Builder) {
	for i := 0; i < int(s.lengths[field]); i++ {
		b.WriteByte(s.chars[field])
	}
}

func (s *skeletonFields) String() string {
	var b strings.Builder
	for i := 0; i < dtpgFieldCount; i++ {
		s.appendFieldTo(i, &b)
	}
	return b.String()
}

func (s *skeletonFields) firstChar() byte {
	for i := 0; i < dtpgFieldCount; i++ {
		if s.lengths[i] != 0 {
			return s.chars[i]
		}
	}
	return 0
}

// ptnSkeleton is ICU's PtnSkeleton.
type ptnSkeleton struct {
	typ                   [dtpgFieldCount]int32
	original              skeletonFields
	baseOriginal          skeletonFields
	addedDefaultDayPeriod bool
}

// setSkeleton is DateTimeMatcher::set: the skeleton of a pattern or of a
// skeleton string.
func setSkeleton(pattern string, fp *formatParser) ptnSkeleton {
	var s ptnSkeleton
	fp.set(pattern)
	for i := 0; i < len(fp.items); i++ {
		value := fp.items[i]
		if isQuoteLiteral(value) {
			_, i = fp.quoteLiteral(i)
			continue
		}
		idx := canonicalIndex(value)
		if idx < 0 {
			continue
		}
		row := dtTypes[idx]
		s.original.populate(row.field, value[0], len(value))
		s.baseOriginal.populate(row.field, row.ch, row.minLen)
		sub := row.typ
		if row.typ > 0 {
			sub += int32(len(value))
		}
		s.typ[row.field] = sub
	}

	// Minutes and fractions with no seconds get seconds (#20739).
	if !s.original.empty(dtpgMinute) && !s.original.empty(dtpgFractionalSecond) &&
		s.original.empty(dtpgSecond) {
		for _, row := range dtTypes {
			if row.field == dtpgSecond {
				s.original.populate(dtpgSecond, row.ch, row.minLen)
				s.baseOriginal.populate(dtpgSecond, row.ch, row.minLen)
				sub := row.typ
				if sub > 0 {
					sub++
				}
				s.typ[dtpgSecond] = sub
				break
			}
		}
	}

	// A twelve-hour clock gets a day period if it has none; a twenty-four
	// hour one loses any it has (#13183).
	if !s.original.empty(dtpgHour) {
		if c := s.original.chars[dtpgHour]; c == 'h' || c == 'K' {
			if s.original.empty(dtpgDayPeriod) {
				for _, row := range dtTypes {
					if row.field == dtpgDayPeriod {
						s.original.populate(dtpgDayPeriod, row.ch, row.minLen)
						s.baseOriginal.populate(dtpgDayPeriod, row.ch, row.minLen)
						s.typ[dtpgDayPeriod] = row.typ
						s.addedDefaultDayPeriod = true
						break
					}
				}
			}
		} else {
			s.original.clearField(dtpgDayPeriod)
			s.baseOriginal.clearField(dtpgDayPeriod)
			s.typ[dtpgDayPeriod] = 0
		}
	}
	return s
}

func (s *ptnSkeleton) fieldMask() int {
	mask := 0
	for i := 0; i < dtpgFieldCount; i++ {
		if s.typ[i] != 0 {
			mask |= 1 << i
		}
	}
	return mask
}

// distance is DateTimeMatcher::getDistance: how far another skeleton is from
// this one, counting only the fields in includeMask of this one.
func (s *ptnSkeleton) distance(other *ptnSkeleton, includeMask int) (dist, missing, extra int) {
	for i := 0; i < dtpgFieldCount; i++ {
		mine := int32(0)
		if includeMask&(1<<i) != 0 {
			mine = s.typ[i]
		}
		theirs := other.typ[i]
		if mine == theirs {
			continue
		}
		switch {
		case mine == 0:
			dist += extraField
			extra |= 1 << i
		case theirs == 0:
			dist += missField
			missing |= 1 << i
		default:
			d := int(mine - theirs)
			if d < 0 {
				d = -d
			}
			dist += d
		}
	}
	return dist, missing, extra
}

// ptnElem is one pattern the generator knows.
type ptnElem struct {
	skeleton  ptnSkeleton
	pattern   string
	specified bool
	// next is the next element of the bucket, counting from one; zero
	// ends it.
	next int32
}

// patternMap is ICU's PatternMap: the patterns, bucketed by the first letter
// of their base pattern, A to Z and then a to z, in the order added. That
// order is the iteration order, and so decides ties.
//
// The elements are kept in one slice, and each bucket is a chain through
// it, counting from one so that zero is none. A base pattern is compared as
// the skeleton fields it is written from: every letter belongs to one field,
// so two base patterns are the same text exactly when their fields are the
// same.
type patternMap struct {
	elems      []ptnElem
	head, tail [52]int32
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

func (m *patternMap) add(skeleton *ptnSkeleton, pattern string, specified bool) {
	b := bootIndex(skeleton.baseOriginal.firstChar())
	if b < 0 {
		return
	}
	for i := m.head[b]; i != 0; i = m.elems[i-1].next {
		e := &m.elems[i-1]
		if e.skeleton.baseOriginal == skeleton.baseOriginal && e.skeleton.typ == skeleton.typ {
			e.pattern = pattern
			e.specified = specified
			return
		}
	}
	m.elems = append(m.elems, ptnElem{skeleton: *skeleton, pattern: pattern, specified: specified})
	n := int32(len(m.elems))
	if m.tail[b] == 0 {
		m.head[b] = n
	} else {
		m.elems[m.tail[b]-1].next = n
	}
	m.tail[b] = n
}

func (m *patternMap) fromBasePattern(base *skeletonFields) (*ptnElem, bool) {
	b := bootIndex(base.firstChar())
	if b < 0 {
		return nil, false
	}
	for i := m.head[b]; i != 0; i = m.elems[i-1].next {
		if e := &m.elems[i-1]; e.skeleton.baseOriginal == *base {
			return e, true
		}
	}
	return nil, false
}

// fromSkeleton finds the first pattern whose skeleton is this one, and its
// specified skeleton if it came from availableFormats.
func (m *patternMap) fromSkeleton(s *ptnSkeleton) (string, *ptnSkeleton, bool) {
	b := bootIndex(s.baseOriginal.firstChar())
	if b < 0 {
		return "", nil, false
	}
	for i := m.head[b]; i != 0; i = m.elems[i-1].next {
		e := &m.elems[i-1]
		if e.skeleton.original == s.original {
			if e.specified {
				return e.pattern, &e.skeleton, true
			}
			return e.pattern, nil, true
		}
	}
	return "", nil, false
}

// A dtpg is the pattern generator for one locale and calendar. It is built
// once per formatter and not changed after.
type dtpg struct {
	patterns       patternMap
	appendItems    [dtpgFieldCount]string
	fieldNames     [dtpgFieldCount]string
	dateTimeFormat [4]string
	decimal        string
	// defaultHourChar is the hour letter a "j" means, and allowedHours the
	// cycles the locale allows, preferred first.
	defaultHourChar byte
	allowedHours    []string
}

// newDTPG builds a generator as ICU's initData does: the canonical letters,
// the locale's style patterns, its available formats, its append items and
// its date-time glue. The patterns are those of the calendar ICU's
// getCalendarTypeToUse settles on, and the glue that of the calendar the
// dates are reckoned in (setDateTimeFromCalendar), which differ only for
// PatternCalendar.
func newDTPG(patterns, glue *datedata.Calendar, fieldNames [datedata.Fields]string, decimal string,
	hourChar byte, allowed []string) *dtpg {
	return buildDTPG(patterns, glue, fieldNames, decimal, hourChar, allowed, true)
}

// newDTPGWithoutStyles is ICU's createInstanceNoStdPat: a generator without
// the locale's style patterns, which SimpleDateFormat::construct makes a
// time style with.
func newDTPGWithoutStyles(patterns, glue *datedata.Calendar, fieldNames [datedata.Fields]string,
	decimal string, hourChar byte, allowed []string) *dtpg {
	return buildDTPG(patterns, glue, fieldNames, decimal, hourChar, allowed, false)
}

func buildDTPG(patterns, glue *datedata.Calendar, fieldNames [datedata.Fields]string, decimal string,
	hourChar byte, allowed []string, styles bool) *dtpg {
	cal := patterns
	g := &dtpg{decimal: decimal, defaultHourChar: hourChar, allowedHours: allowed}
	// Room for every pattern, so that the elements are allocated once.
	g.patterns.elems = make([]ptnElem, 0, len(canonicalItems)+2*datedata.Lengths+len(cal.Available))
	var fp formatParser
	for i := 0; i < len(canonicalItems); i++ {
		g.addPattern(string(canonicalItems[i]), "", false, false, &fp)
	}
	// The style patterns, the times from full to short and then the dates.
	for _, set := range [][datedata.Lengths]string{cal.TimeFormats, cal.DateFormats} {
		for _, p := range set {
			if p != "" && styles {
				g.addPattern(p, "", false, false, &fp)
			}
		}
	}
	for i := 0; i < dtpgFieldCount; i++ {
		g.appendItems[i] = cal.AppendItems[i]
		if g.appendItems[i] == "" {
			g.appendItems[i] = defaultAppendItem
		}
		g.fieldNames[i] = fieldNames[i]
		if g.fieldNames[i] == "" {
			g.fieldNames[i] = "F" + strconv.Itoa(i)
		}
	}
	// The availableFormats skeletons already added.
	added := make(map[string]bool, len(cal.Available))
	for _, s := range cal.Available {
		if added[s.ID] {
			continue
		}
		added[s.ID] = true
		g.addPattern(s.Pattern, s.ID, true, true, &fp)
	}
	for i := 0; i < 4; i++ {
		g.dateTimeFormat[i] = glue.AtTimeFormats[i]
		if g.dateTimeFormat[i] == "" {
			g.dateTimeFormat[i] = glue.DateTimeFormats[i]
		}
	}
	return g
}

// addPattern is addPatternWithOptionalSkeleton: the pattern, under the
// skeleton given, if hasSkeleton, or else its own.
func (g *dtpg) addPattern(pattern, skeletonToUse string, hasSkeleton, override bool, fp *formatParser) {
	var skeleton ptnSkeleton
	if !hasSkeleton {
		skeleton = setSkeleton(pattern, fp)
	} else {
		skeleton = setSkeleton(skeletonToUse, fp)
	}
	if dup, ok := g.patterns.fromBasePattern(&skeleton.baseOriginal); ok &&
		(!dup.specified || (hasSkeleton && !override)) {
		if !override {
			return
		}
	}
	if _, specified, ok := g.patterns.fromSkeleton(&skeleton); ok {
		if !override || (hasSkeleton && specified != nil) {
			return
		}
	}
	g.patterns.add(&skeleton, pattern, hasSkeleton)
}

// bestRaw is getBestRaw: the closest pattern to a skeleton, counting only
// the fields in includeMask, with the fields it lacks and has too many of.
func (g *dtpg) bestRaw(source *ptnSkeleton, includeMask int) (pattern string, specified *ptnSkeleton, missing, extra int) {
	bestDistance := int(^uint(0) >> 1)
	bestMissing := -1
	found := false
	for b := range g.patterns.head {
		for i := g.patterns.head[b]; i != 0; i = g.patterns.elems[i-1].next {
			e := &g.patterns.elems[i-1]
			d, m, x := source.distance(&e.skeleton, includeMask)
			if d < bestDistance || (d == bestDistance && bestMissing < m) {
				bestDistance, bestMissing = d, m
				pattern, specified, found = g.patterns.fromSkeleton(&e.skeleton)
				missing, extra = m, x
				if d == 0 {
					return pattern, specified, missing, extra
				}
			}
		}
	}
	if !found {
		return "", nil, missing, extra
	}
	return pattern, specified, missing, extra
}

// bestPattern is getBestPattern.
func (g *dtpg) bestPattern(skeletonText string, options int) string {
	flags := 0
	mapped := g.mapSkeletonMetacharacters(skeletonText, &flags)
	var fp formatParser
	req := setSkeleton(mapped, &fp)
	best, specified, missing, extra := g.bestRaw(&req, -1)
	if missing == 0 && extra == 0 {
		return g.adjustFieldTypes(best, specified, &req, flags, options)
	}
	needed := req.fieldMask()
	dateMask := 1<<dtpgDayPeriod - 1
	timeMask := 1<<dtpgFieldCount - 1 - dateMask
	datePattern := g.bestAppending(&req, needed&dateMask, flags, options)
	timePattern := g.bestAppending(&req, needed&timeMask, flags, options)
	switch {
	case datePattern == "":
		return timePattern
	case timePattern == "":
		return datePattern
	}
	style := datedata.Short
	switch req.baseOriginal.lengths[dtpgMonth] {
	case 4:
		if req.baseOriginal.lengths[dtpgWeekday] > 0 {
			style = datedata.Full
		} else {
			style = datedata.Long
		}
	case 3:
		style = datedata.Medium
	}
	return simpleFormat(g.dateTimeFormat[style], timePattern, datePattern)
}

// mapSkeletonMetacharacters replaces j, J and C with the locale's hour and
// day period letters.
func (g *dtpg) mapSkeletonMetacharacters(form string, flags *int) string {
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(form); i++ {
		c := form[i]
		if c == '\'' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		switch c {
		case 'j', 'C':
			extra := 0
			for i+1 < len(form) && form[i+1] == c {
				extra++
				i++
			}
			hourLen := 1 + extra&1
			periodLen := 1
			if extra >= 2 {
				periodLen = 3 + extra>>1
			}
			hourChar, periodChar := byte('h'), byte('a')
			if c == 'j' {
				hourChar = g.defaultHourChar
			} else {
				best := "H"
				if len(g.allowedHours) > 0 {
					best = g.allowedHours[0]
				}
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
					periodChar = 'B'
				case "Hb", "hb", "Kb":
					periodChar = 'b'
				}
			}
			if hourChar == 'H' || hourChar == 'k' {
				periodLen = 0
			}
			for ; periodLen > 0; periodLen-- {
				b.WriteByte(periodChar)
			}
			for ; hourLen > 0; hourLen-- {
				b.WriteByte(hourChar)
			}
		case 'J':
			b.WriteByte('H')
			*flags |= flagSkeletonUsesCapJ
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// adjustFieldTypes bends a found pattern's fields to the requested skeleton.
func (g *dtpg) adjustFieldTypes(pattern string, specified *ptnSkeleton, req *ptnSkeleton,
	flags, options int) string {
	var fp formatParser
	fp.set(pattern)
	var out strings.Builder
	for i := 0; i < len(fp.items); i++ {
		field := fp.items[i]
		if isQuoteLiteral(field) {
			q, j := fp.quoteLiteral(i)
			out.WriteString(q)
			i = j
			continue
		}
		if fp.isPatternSeparator(field) {
			out.WriteString(field)
			continue
		}
		idx := canonicalIndex(field)
		if idx < 0 {
			out.WriteString(field)
			continue
		}
		row := dtTypes[idx]
		typeValue := row.field

		if flags&flagFixFractionalSeconds != 0 && typeValue == dtpgSecond {
			var b strings.Builder
			b.WriteString(field)
			b.WriteString(g.decimal)
			req.original.appendFieldTo(dtpgFractionalSecond, &b)
			field = b.String()
		} else if req.typ[typeValue] != 0 {
			reqChar := req.original.chars[typeValue]
			reqLen := int(req.original.lengths[typeValue])
			if reqChar == 'E' && reqLen < 3 {
				reqLen = 3
			}
			adjLen := reqLen
			if (typeValue == dtpgHour && options&matchHourFieldLength == 0) ||
				(typeValue == dtpgMinute && options&matchMinuteFieldLength == 0) ||
				(typeValue == dtpgSecond && options&matchSecondFieldLength == 0) {
				adjLen = len(field)
			} else if specified != nil && reqChar != 'c' && reqChar != 'e' {
				skelLen := int(specified.original.lengths[typeValue])
				patNumeric := row.typ > 0
				reqNumeric := req.typ[typeValue] > 0
				if skelLen == reqLen || patNumeric != reqNumeric {
					adjLen = len(field)
				}
			}
			c := reqChar
			if typeValue == dtpgHour || typeValue == dtpgMonth || typeValue == dtpgWeekday ||
				(typeValue == dtpgYear && reqChar != 'Y') {
				c = field[0]
			}
			if c == 'E' && adjLen < 3 {
				c = 'e'
			}
			if typeValue == dtpgHour && g.defaultHourChar != 0 {
				switch {
				case flags&flagSkeletonUsesCapJ != 0 || reqChar == g.defaultHourChar:
					c = g.defaultHourChar
				case reqChar == 'h' && g.defaultHourChar == 'K':
					c = 'K'
				case reqChar == 'H' && g.defaultHourChar == 'k':
					c = 'k'
				case reqChar == 'k' && g.defaultHourChar == 'H':
					c = 'H'
				case reqChar == 'K' && g.defaultHourChar == 'h':
					c = 'h'
				}
			}
			field = strings.Repeat(string(c), adjLen)
		}
		out.WriteString(field)
	}
	return out.String()
}

// bestAppending is getBestAppending: the best pattern for some fields,
// adding the ones it lacks with the locale's append items.
func (g *dtpg) bestAppending(req *ptnSkeleton, missingFields, flags, options int) string {
	if missingFields == 0 {
		return ""
	}
	temp, specified, missing, _ := g.bestRaw(req, missingFields)
	result := g.adjustFieldTypes(temp, specified, req, flags, options)
	if missing == 0 {
		return result
	}
	last := 0
	for missing != 0 {
		if last == missing {
			break
		}
		if missing&secondAndFractionalMask == fractionalMask &&
			missingFields&secondAndFractionalMask == secondAndFractionalMask {
			result = g.adjustFieldTypes(result, specified, req, flags|flagFixFractionalSeconds, options)
			missing &^= fractionalMask
			continue
		}
		starting := missing
		// ICU reuses one variable for the specified skeleton, so the
		// fraction branch above sees the latest one.
		temp, specified, missing, _ = g.bestRaw(req, missing)
		temp = g.adjustFieldTypes(temp, specified, req, flags, options)
		found := starting &^ missing
		top := topBitNumber(found)
		if g.appendItems[top] != "" {
			name := "'" + g.fieldNames[top] + "'"
			result = simpleFormat(g.appendItems[top], result, temp, name)
		}
		last = missing
	}
	return result
}

func topBitNumber(mask int) int {
	if mask == 0 {
		return 0
	}
	i := 0
	for mask != 0 {
		mask >>= 1
		i++
	}
	if i-1 > dtpgZone {
		return dtpgZone
	}
	return i - 1
}

// simpleFormat substitutes {0}, {1} and so on as ICU's SimpleFormatter does:
// an apostrophe quotes a brace that follows it and a doubled apostrophe is
// one, and every other character, apostrophes included, is kept.
func simpleFormat(pattern string, args ...string) string {
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '\'':
			switch {
			case i+1 < len(pattern) && pattern[i+1] == '\'':
				b.WriteByte('\'')
				i++
			case inQuote:
				inQuote = false
			case i+1 < len(pattern) && (pattern[i+1] == '{' || pattern[i+1] == '}'):
				inQuote = true
			default:
				b.WriteByte('\'')
			}
		case c == '{' && !inQuote:
			end := strings.IndexByte(pattern[i:], '}')
			if end > 0 {
				n := 0
				ok := true
				for _, d := range pattern[i+1 : i+end] {
					if d < '0' || d > '9' {
						ok = false
						break
					}
					n = n*10 + int(d-'0')
				}
				if ok && end > 1 && n < len(args) {
					b.WriteString(args[n])
					i += end
					continue
				}
			}
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// staticSkeleton is staticGetSkeleton: the skeleton of a pattern, without a
// day period the matcher added on its own.
func staticSkeleton(pattern string) string {
	var fp formatParser
	s := setSkeleton(pattern, &fp)
	out := s.original.String()
	if s.addedDefaultDayPeriod {
		if pos := strings.IndexByte(out, 'a'); pos >= 0 {
			out = out[:pos] + out[pos+1:]
		}
	}
	return out
}

// defaultHourCycle is getDefaultHourCycle.
func (g *dtpg) defaultHourCycle() HourCycle {
	switch g.defaultHourChar {
	case 'K':
		return H11
	case 'h':
		return H12
	case 'k':
		return H24
	}
	return H23
}
