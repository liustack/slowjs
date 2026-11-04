package intl

import (
	"strings"
	"time"
	"unicode/utf16"

	"github.com/liustack/quickjs/internal/gointl/internal/datedata"
)

// Ranges of two moments: formatRange and formatRangeToParts.
//
// V8 writes a range with ICU's DateIntervalFormat, made from the skeleton of
// the formatter's own pattern, with the resolved hour cycle added to the
// locale as -u-hc. The interval formatter finds the largest field in which
// the two moments differ and writes them with the locale's interval pattern
// for that field -- "Jan 5 – 7, 2024" -- or failing one joins two whole
// dates with the fallback, "{0} – {1}". Where the moments do not differ in
// anything the pattern shows, V8 writes the first alone.
//
// This follows ICU's dtitvfmt.cpp and dtitvinf.cpp; the comments name the
// functions it follows.

// The ICU calendar fields an interval pattern is keyed by, in
// datedata.Interval* order: their pattern letters and their levels, the
// larger the smaller the unit.
var (
	intervalLetters = [datedata.IntervalFields]byte{'G', 'y', 'M', 'd', 'a', 'h', 'm', 's', 'S'}
	intervalLevels  = [datedata.IntervalFields]int{0, 10, 20, 30, 40, 50, 60, 70, 80}
)

// A rangePattern is an interval pattern cut in two at the point where the
// second moment starts. A fallback has only a second part: the whole pattern
// each moment is written in, joined by the fallback.
type rangePattern struct {
	first, second string
	laterFirst    bool
}

// rangeFormat is ICU's DateIntervalFormat for one formatter.
type rangeFormat struct {
	info *intervalInfo
	// skeleton is the formatter's skeleton, and pattern what the interval
	// formatter writes a single moment in.
	skeleton string
	pattern  string
	patterns [datedata.IntervalFields]rangePattern
	// datePattern and timePattern write the date and the time apart, for
	// a range within one day that has no interval pattern; dateTimeGlue
	// joins them.
	datePattern, timePattern string
	hasDate, hasTime         bool
	dateTimeGlue             string
}

// newRangeFormat builds the interval formatter as V8 creates it
// (LazyCreateDateIntervalFormat): from a skeleton, the formatter's pattern's
// or a Temporal kind's, in the locale with the resolved hour cycle.
func (f *DateTimeFormat) newRangeFormat(src Source, g *dtpg, skeleton string) (*rangeFormat, error) {
	if letter, ok := hourLetters[f.hourCycle]; ok && letter != g.defaultHourChar {
		_, allowed, err := allowedHourFormats(src, f.locale)
		if err != nil {
			return nil, err
		}
		g = f.qjsGenerator(letter, allowed, true)
	}
	r := &rangeFormat{info: newIntervalInfo(f.patterns), skeleton: skeleton}
	r.pattern = g.bestPattern(r.skeleton, 0)
	r.initialize(g, f.data.DateTimeGlue)
	return r, nil
}

// initialize is DateIntervalFormat::initializePattern.
func (r *rangeFormat) initialize(g *dtpg, glue string) {
	for i := range r.patterns {
		r.patterns[i].laterFirst = r.info.laterFirst
	}
	converted := normalizeHourMetacharacters(r.skeleton, g)
	dateSkeleton, normalizedDate, timeSkeleton, normalizedTime := dateTimeSkeleton(converted)

	if timeSkeleton != "" && dateSkeleton != "" && len(glue) >= 3 {
		r.dateTimeGlue = glue
	}

	found := r.setSeparateDateTimePattern(g, normalizedDate, normalizedTime)
	if !found || dateSkeleton == "" {
		// A time with no date, or with seconds, which no interval pattern
		// has: a range across days writes each end as a short date and
		// the time.
		if timeSkeleton != "" && dateSkeleton == "" {
			pattern := g.bestPattern("yMd"+timeSkeleton, 0)
			for _, field := range []int{datedata.IntervalDay, datedata.IntervalMonth, datedata.IntervalYear} {
				r.setPatternInfo(field, "", pattern, r.info.laterFirst)
			}
			pattern = g.bestPattern("GyMd"+timeSkeleton, 0)
			r.setPatternInfo(datedata.IntervalEra, "", pattern, r.info.laterFirst)
		}
		return
	}
	if timeSkeleton == "" {
		return
	}

	// A date and a time: a range across days writes both ends whole, each
	// with every field down to the one that differs; a range within a day
	// writes the date once and the time as a range.
	skeleton := r.skeleton
	for _, field := range []int{datedata.IntervalDay, datedata.IntervalMonth, datedata.IntervalYear, datedata.IntervalEra} {
		letter := intervalLetters[field]
		if strings.IndexByte(dateSkeleton, letter) < 0 {
			skeleton = string(letter) + skeleton
			r.setPatternInfo(field, "", g.bestPattern(skeleton, 0), r.info.laterFirst)
		}
	}
	if r.dateTimeGlue == "" {
		return
	}
	datePattern := g.bestPattern(dateSkeleton, 0)
	for _, field := range []int{datedata.IntervalDayPeriod, datedata.IntervalHour, datedata.IntervalMinute} {
		r.concatDateToTimeInterval(datePattern, field)
	}
}

// normalizeHourMetacharacters is ICU's: the hour letter and the day period
// the locale actually uses for the skeleton's hour, with the day period
// written as wide as the skeleton asked for.
func normalizeHourMetacharacters(skeleton string, g *dtpg) string {
	var hourMeta, dayPeriod byte
	hourStart, hourLen, periodStart, periodLen := 0, 0, 0, 0
	for i := 0; i < len(skeleton); i++ {
		switch c := skeleton[i]; c {
		case 'j', 'J', 'C', 'h', 'H', 'k', 'K':
			if hourMeta == 0 {
				hourMeta, hourStart = c, i
			}
			hourLen++
		case 'a', 'b', 'B':
			if dayPeriod == 0 {
				dayPeriod, periodStart = c, i
			}
			periodLen++
		default:
			if hourMeta != 0 && dayPeriod != 0 {
				i = len(skeleton)
			}
		}
	}
	if hourMeta == 0 {
		return skeleton
	}
	hourChar := byte('H')
	converted := g.bestPattern(string(hourMeta), 0)
	// Literal text goes, so that the h of German's "Uhr" is not taken for
	// an hour.
	for {
		q := strings.IndexByte(converted, '\'')
		if q < 0 {
			break
		}
		end := strings.IndexByte(converted[q+1:], '\'')
		if end < 0 {
			end = q
		} else {
			end += q + 1
		}
		converted = converted[:q] + converted[end+1:]
	}
	switch {
	case strings.IndexByte(converted, 'h') >= 0:
		hourChar = 'h'
	case strings.IndexByte(converted, 'K') >= 0:
		hourChar = 'K'
	case strings.IndexByte(converted, 'k') >= 0:
		hourChar = 'k'
	}
	switch {
	case strings.IndexByte(converted, 'b') >= 0:
		dayPeriod = 'b'
	case strings.IndexByte(converted, 'B') >= 0:
		dayPeriod = 'B'
	case dayPeriod == 0:
		dayPeriod = 'a'
	}
	replacement := string(hourChar)
	if hourChar != 'H' && hourChar != 'k' {
		n := 1
		switch {
		case periodLen >= 5 || hourLen >= 5:
			n = 5
		case periodLen >= 3 || hourLen >= 3:
			n = 3
		}
		replacement += strings.Repeat(string(dayPeriod), n)
	}
	out := skeleton[:hourStart] + replacement + skeleton[hourStart+hourLen:]
	if periodLen > 0 {
		if periodStart > hourStart {
			periodStart += len(replacement) - hourLen
		}
		out = out[:periodStart] + out[periodStart+periodLen:]
	}
	return out
}

// dateTimeSkeleton is DateIntervalFormat::getDateTimeSkeleton: a skeleton's
// date and time fields apart, each as written and normalized.
func dateTimeSkeleton(skeleton string) (date, normalizedDate, clock, normalizedClock string) {
	var d, nd, t, nt strings.Builder
	var eCount, dCount, mCount, yCount, minCount, vCount, zCount int
	var hourChar byte
	for i := 0; i < len(skeleton); i++ {
		c := skeleton[i]
		switch c {
		case 'E':
			d.WriteByte(c)
			eCount++
		case 'd':
			d.WriteByte(c)
			dCount++
		case 'M':
			d.WriteByte(c)
			mCount++
		case 'y':
			d.WriteByte(c)
			yCount++
		case 'G', 'Y', 'u', 'Q', 'q', 'L', 'l', 'W', 'w', 'D', 'F', 'g', 'e', 'c', 'U', 'r':
			nd.WriteByte(c)
			d.WriteByte(c)
		case 'h', 'H', 'k', 'K':
			t.WriteByte(c)
			if hourChar == 0 {
				hourChar = c
			}
		case 'm':
			t.WriteByte(c)
			minCount++
		case 'z':
			zCount++
			t.WriteByte(c)
		case 'v':
			vCount++
			t.WriteByte(c)
		case 'a', 'V', 'Z', 'j', 's', 'S', 'A', 'b', 'B':
			t.WriteByte(c)
			nt.WriteByte(c)
		}
	}
	nd.WriteString(strings.Repeat("y", yCount))
	if mCount > 0 {
		if mCount < 3 {
			nd.WriteByte('M')
		} else {
			nd.WriteString(strings.Repeat("M", min(mCount, 5)))
		}
	}
	if eCount > 0 {
		if eCount <= 3 {
			nd.WriteByte('E')
		} else {
			nd.WriteString(strings.Repeat("E", min(eCount, 5)))
		}
	}
	if dCount > 0 {
		nd.WriteByte('d')
	}
	if hourChar != 0 {
		nt.WriteByte(hourChar)
	}
	if minCount > 0 {
		nt.WriteByte('m')
	}
	if zCount > 0 {
		nt.WriteByte('z')
	}
	if vCount > 0 {
		nt.WriteByte('v')
	}
	return d.String(), nd.String(), t.String(), nt.String()
}

// setSeparateDateTimePattern is DateIntervalFormat::setSeparateDateTimePtn:
// the interval patterns of the date skeleton, or of the time skeleton when
// there is one, adapted from the locale's nearest. It reports whether the
// locale had one near enough.
func (r *rangeFormat) setSeparateDateTimePattern(g *dtpg, dateSkeleton, timeSkeleton string) bool {
	skeleton := dateSkeleton
	if timeSkeleton != "" {
		skeleton = timeSkeleton
	}
	best, diff := r.info.bestSkeleton(skeleton)
	if best == "" {
		return false
	}
	if dateSkeleton != "" {
		r.datePattern, r.hasDate = g.bestPattern(dateSkeleton, 0), true
	}
	if timeSkeleton != "" {
		r.timePattern, r.hasTime = g.bestPattern(timeSkeleton, 0), true
	}
	if diff == -1 {
		return false
	}
	if timeSkeleton == "" {
		// ICU passes the skeletons by pointer, and once the month has
		// been found by extending them, the later calls work on the
		// extended ones -- and extend them further in place.
		var extended, extendedBest string
		sk, bs := &skeleton, &best
		r.setIntervalPattern(datedata.IntervalDay, sk, bs, diff, &extended, &extendedBest)
		if r.setIntervalPattern(datedata.IntervalMonth, sk, bs, diff, &extended, &extendedBest) {
			sk, bs = &extended, &extendedBest
		}
		r.setIntervalPattern(datedata.IntervalYear, sk, bs, diff, &extended, &extendedBest)
		r.setIntervalPattern(datedata.IntervalEra, sk, bs, diff, &extended, &extendedBest)
	} else {
		r.setIntervalPattern(datedata.IntervalMinute, &skeleton, &best, diff, nil, nil)
		r.setIntervalPattern(datedata.IntervalHour, &skeleton, &best, diff, nil, nil)
		r.setIntervalPattern(datedata.IntervalDayPeriod, &skeleton, &best, diff, nil, nil)
	}
	return true
}

// setIntervalPattern is the six-argument DateIntervalFormat::setIntervalPattern.
// It reports whether it found the pattern by extending the skeleton.
func (r *rangeFormat) setIntervalPattern(field int, skeleton, bestSkeleton *string, diff int,
	extended, extendedBest *string) bool {
	best := *bestSkeleton
	pattern := r.info.pattern(best, field)
	suppress := strings.IndexByte(r.skeleton, 'J') >= 0
	if pattern == "" {
		if isFieldUnitIgnored(best, field) {
			return false
		}
		// A 24-hour locale may have no pattern for a change of day period,
		// which is then a change of hour.
		if field == datedata.IntervalDayPeriod {
			if pattern = r.info.pattern(best, datedata.IntervalHour); pattern != "" {
				r.setIntervalPatternOrdered(field, adjustFieldWidth(*skeleton, best, pattern, diff, suppress), r.info.laterFirst)
			}
			return false
		}
		// No pattern for a change of year in "MMMd": look in "yMMMd".
		if extended != nil {
			letter := string(intervalLetters[field])
			*extended = *skeleton
			*extendedBest = *bestSkeleton
			*extended = letter + *extended
			*extendedBest = letter + *extendedBest
			pattern = r.info.pattern(*extendedBest, field)
			if pattern == "" && diff == 0 {
				// getBestSkeleton sets the difference whether or not its
				// answer is taken.
				var tmp string
				tmp, diff = r.info.bestSkeleton(*extendedBest)
				if tmp != "" && diff != -1 {
					pattern = r.info.pattern(tmp, field)
					best = tmp
				}
			}
		}
	}
	if pattern == "" {
		return false
	}
	if diff != 0 || suppress {
		pattern = adjustFieldWidth(*skeleton, best, pattern, diff, suppress)
	}
	r.setIntervalPatternOrdered(field, pattern, r.info.laterFirst)
	return extended != nil && *extended != ""
}

// setIntervalPatternOrdered is the three-argument setIntervalPattern: a
// pattern may say which moment it writes first, and is cut in two.
func (r *rangeFormat) setIntervalPatternOrdered(field int, pattern string, laterFirst bool) {
	if rest, ok := strings.CutPrefix(pattern, "latestFirst:"); ok {
		pattern, laterFirst = rest, true
	} else if rest, ok := strings.CutPrefix(pattern, "earliestFirst:"); ok {
		pattern, laterFirst = rest, false
	}
	split := splitPatternInTwo(pattern)
	r.setPatternInfo(field, pattern[:split], pattern[split:], laterFirst)
}

func (r *rangeFormat) setPatternInfo(field int, first, second string, laterFirst bool) {
	r.patterns[field] = rangePattern{first: first, second: second, laterFirst: laterFirst}
}

// concatDateToTimeInterval is concatSingleDate2TimeInterval: a time range
// within a day, with the date written once beside it.
func (r *rangeFormat) concatDateToTimeInterval(datePattern string, field int) {
	p := r.patterns[field]
	if p.first == "" {
		return
	}
	combined := simpleFormat(r.dateTimeGlue, p.first+p.second, datePattern)
	r.setIntervalPatternOrdered(field, combined, p.laterFirst)
}

// splitPatternInTwo is splitPatternInto2Part: the index where a pattern
// letter first repeats, which is where the second moment starts.
func splitPatternInTwo(pattern string) int {
	var repeated [58]bool
	inQuote := false
	var prev byte
	count := 0
	found := false
	i := 0
	for ; i < len(pattern); i++ {
		c := pattern[i]
		if c != prev && count > 0 {
			if !repeated[prev-'A'] {
				repeated[prev-'A'] = true
			} else {
				found = true
				break
			}
			count = 0
		}
		if c == '\'' {
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				i++
			} else {
				inQuote = !inQuote
			}
		} else if !inQuote && isASCIILetter(c) {
			prev = c
			count++
		}
	}
	if count > 0 && !found && !repeated[prev-'A'] {
		count = 0
	}
	return i - count
}

// adjustFieldWidth is ICU's: a pattern for the nearest skeleton, with its
// fields widened to what the skeleton asked for, and its zone and hour
// letters the skeleton's.
func adjustFieldWidth(input, best, pattern string, diff int, suppressDayPeriod bool) string {
	inputWidths := skeletonWidths(input)
	bestWidths := skeletonWidths(best)
	if suppressDayPeriod {
		for _, s := range []string{" a", " a", "a ", "a ", "a"} {
			pattern = replaceUnquoted(pattern, s, "")
		}
		pattern = replaceUnquoted(pattern, "  ", " ")
		pattern = strings.TrimSpace(pattern)
	}
	if diff == 2 {
		if strings.IndexByte(input, 'z') >= 0 {
			pattern = replaceUnquoted(pattern, "v", "z")
		}
		if strings.IndexByte(input, 'K') >= 0 {
			pattern = replaceUnquoted(pattern, "h", "K")
		}
		if strings.IndexByte(input, 'k') >= 0 {
			pattern = replaceUnquoted(pattern, "H", "k")
		}
		if strings.IndexByte(input, 'b') >= 0 {
			pattern = replaceUnquoted(pattern, "a", "b")
		}
	}
	if strings.IndexByte(pattern, 'a') >= 0 && bestWidths['a'-'A'] == 0 {
		bestWidths['a'-'A'] = 1
	}
	if strings.IndexByte(pattern, 'b') >= 0 && bestWidths['b'-'A'] == 0 {
		bestWidths['b'-'A'] = 1
	}

	widen := func(prev byte, count int) (string, bool) {
		sk := prev
		if sk == 'L' {
			sk = 'M'
		}
		have, want := bestWidths[sk-'A'], inputWidths[sk-'A']
		if have == count && want > have {
			return strings.Repeat(string(prev), want-have), true
		}
		return "", false
	}
	b := []byte(pattern)
	inQuote := false
	var prev byte
	count := 0
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c != prev && count > 0 {
			if extra, ok := widen(prev, count); ok {
				b = append(b[:i], append([]byte(extra), b[i:]...)...)
				i += len(extra)
				c = b[i]
			}
			count = 0
		}
		if c == '\'' {
			if i+1 < len(b) && b[i+1] == '\'' {
				i++
			} else {
				inQuote = !inQuote
			}
		} else if !inQuote && isASCIILetter(c) {
			prev = c
			count++
		}
	}
	if count > 0 {
		if extra, ok := widen(prev, count); ok {
			b = append(b, extra...)
		}
	}
	return string(b)
}

// skeletonWidths counts each letter of a skeleton, from 'A'.
func skeletonWidths(skeleton string) [58]int {
	var w [58]int
	for i := 0; i < len(skeleton); i++ {
		if c := skeleton[i]; c >= 'A' && c <= 'z' {
			w[c-'A']++
		}
	}
	return w
}

// replaceUnquoted is findReplaceInPattern: a replacement everywhere but
// in quoted literal text.
func replaceUnquoted(pattern, old, new string) string {
	q := strings.IndexByte(pattern, '\'')
	if q < 0 {
		return strings.ReplaceAll(pattern, old, new)
	}
	var b strings.Builder
	source := pattern
	for q >= 0 {
		end := strings.IndexByte(source[q+1:], '\'')
		if end < 0 {
			end = len(source) - 1
		} else {
			end += q + 1
		}
		b.WriteString(strings.ReplaceAll(source[:q], old, new))
		b.WriteString(source[q : end+1])
		source = source[end+1:]
		q = strings.IndexByte(source, '\'')
	}
	b.WriteString(strings.ReplaceAll(source, old, new))
	return b.String()
}

// levelOf is SimpleDateFormat::getLevelFromChar.
func levelOf(c byte) int {
	switch c {
	case 'G', 'O', 'V', 'X', 'Z', 'g', 'l', 'v', 'x', 'z':
		return 0
	case 'U', 'Y', 'r', 'u', 'y':
		return 10
	case 'D', 'L', 'M', 'Q', 'q', 'w':
		return 20
	case 'E', 'F', 'W', 'c', 'd', 'e':
		return 30
	case 'A', 'a':
		return 40
	case 'H', 'K', 'h', 'k':
		return 50
	case 'm':
		return 60
	case 's':
		return 70
	case 'S':
		return 80
	}
	return -1
}

// isSyntaxChar is SimpleDateFormat::isSyntaxChar: the ASCII letters.
func isSyntaxChar(c byte) bool { return isASCIILetter(c) }

// isFieldUnitIgnored is SimpleDateFormat's: whether a pattern shows nothing
// as small as a field.
func isFieldUnitIgnored(pattern string, field int) bool {
	fieldLevel := intervalLevels[field]
	inQuote := false
	var prev byte
	count := 0
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c != prev && count > 0 {
			if fieldLevel <= levelOf(prev) {
				return false
			}
			count = 0
		}
		if c == '\'' {
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				i++
			} else {
				inQuote = !inQuote
			}
		} else if !inQuote && isSyntaxChar(c) {
			prev = c
			count++
		}
	}
	if count > 0 && fieldLevel <= levelOf(prev) {
		return false
	}
	return true
}

// intervalInfo is ICU's DateIntervalInfo: a calendar's interval patterns, in
// the order ICU's hash table walks them, and the fallback.
type intervalInfo struct {
	intervals []datedata.Interval
	// order is the intervals' indexes in ICU's walking order.
	order      []int
	fallback   string
	laterFirst bool
}

func newIntervalInfo(cal *datedata.Calendar) *intervalInfo {
	info := &intervalInfo{intervals: cal.Intervals, fallback: "{0} – {1}"}
	if p := cal.IntervalFallback; p != "" {
		first, second := strings.Index(p, "{0}"), strings.Index(p, "{1}")
		if first >= 0 && second >= 0 {
			info.fallback = p
			info.laterFirst = first > second
		}
	}
	keys := make([]string, len(cal.Intervals))
	for i := range cal.Intervals {
		keys[i] = cal.Intervals[i].Skeleton
	}
	info.order = uhashOrder(keys)
	return info
}

// pattern is getIntervalPattern: a skeleton's pattern for a field.
func (in *intervalInfo) pattern(skeleton string, field int) string {
	for i := range in.intervals {
		if in.intervals[i].Skeleton == skeleton {
			return in.intervals[i].Patterns[field]
		}
	}
	return ""
}

// bestSkeleton is getBestSkeleton: the locale's skeleton nearest to one
// asked for, and how it differs -- 0 not at all, 1 in widths, 2 in the zone
// or hour letter as well, -1 in its fields. Of two equally near, the first
// ICU's hash table walks to wins.
func (in *intervalInfo) bestSkeleton(skeleton string) (string, int) {
	replaced := false
	if strings.ContainsAny(skeleton, "zkKab") {
		skeleton = strings.NewReplacer("z", "v", "k", "H", "K", "h", "a", "", "b", "").Replace(skeleton)
		replaced = true
	}
	input := skeletonWidths(skeleton)
	const differentField, stringNumeric = 0x1000, 0x100
	bestDistance := int(^uint32(0) >> 1)
	best := ""
	info := 0
	for _, idx := range in.order {
		candidate := in.intervals[idx].Skeleton
		widths := skeletonWidths(candidate)
		distance := 0
		fieldDifference := 1
		for i := range widths {
			a, b := input[i], widths[i]
			switch {
			case a == b:
			case a == 0 || b == 0:
				fieldDifference = -1
				distance += differentField
			case byte(i)+'A' == 'M' && (a <= 2 && b > 2 || a > 2 && b <= 2):
				distance += stringNumeric
			case a > b:
				distance += a - b
			default:
				distance += b - a
			}
		}
		if distance < bestDistance {
			best, bestDistance, info = candidate, distance, fieldDifference
		}
		if distance == 0 {
			info = 0
			break
		}
	}
	if replaced && info != -1 {
		info = 2
	}
	return best, info
}

// uhashOrder is the order ICU's uhash walks string keys put into a fresh
// table in the given order: open addressing with double hashing over a
// prime-sized table, which grows past half full.
func uhashOrder(keys []string) []int {
	primes := [...]int{7, 13, 31, 61, 127, 251, 509, 1021, 2039, 4093, 8191, 16381, 32749}
	primeIndex := 4
	type slot struct {
		key  int // index plus one; zero is empty
		hash int32
	}
	table := make([]slot, primes[primeIndex])
	count := 0
	find := func(table []slot, hash int32) int {
		length := int32(len(table))
		start := (hash ^ 0x4000000) % length
		index, jump := start, int32(0)
		for {
			if table[index].key == 0 {
				return int(index)
			}
			if jump == 0 {
				jump = hash%(length-1) + 1
			}
			index = (index + jump) % length
			if index == start {
				panic("intl: uhashOrder filled its table")
			}
		}
	}
	for i, key := range keys {
		if count > int(float32(len(table))*0.5) && primeIndex+1 < len(primes) {
			primeIndex++
			old := table
			table = make([]slot, primes[primeIndex])
			for j := len(old) - 1; j >= 0; j-- {
				if old[j].key != 0 {
					table[find(table, old[j].hash)] = old[j]
				}
			}
		}
		hash := unicodeStringHash(key) & 0x7fffffff
		table[find(table, hash)] = slot{key: i + 1, hash: hash}
		count++
	}
	out := make([]int, 0, len(keys))
	for _, s := range table {
		if s.key != 0 {
			out = append(out, s.key-1)
		}
	}
	return out
}

// unicodeStringHash is UnicodeString::hashCode, over UTF-16 code units.
func unicodeStringHash(s string) int32 {
	units := utf16.Encode([]rune(s))
	var hash uint32
	inc := (len(units)-32)/32 + 1
	for i := 0; i < len(units); i += inc {
		hash = hash*37 + uint32(units[i])
	}
	if hash == 0 {
		return 1
	}
	return int32(hash)
}

// A rangeSeg is a written piece with where it falls in the text.
type rangeSeg struct {
	dateSeg
	start, limit int
}

// format is DateIntervalFormat::formatImpl. It returns the pieces and which
// moment was written first: 0 the start, 1 the end, -1 for a single date.
func (r *rangeFormat) format(f *DateTimeFormat, from, to dateParts) ([]dateSeg, int) {
	field := -1
	switch {
	case from.era != to.era:
		field = datedata.IntervalEra
	case from.year != to.year:
		field = datedata.IntervalYear
	case from.month != to.month:
		field = datedata.IntervalMonth
	case from.day != to.day:
		field = datedata.IntervalDay
	case from.afternoon != to.afternoon:
		field = datedata.IntervalDayPeriod
	case from.hour%12 != to.hour%12:
		field = datedata.IntervalHour
	case from.minute != to.minute:
		field = datedata.IntervalMinute
	case from.second != to.second:
		field = datedata.IntervalSecond
	case from.millis != to.millis:
		field = datedata.IntervalMillisecond
	}
	write := func(pattern string, p dateParts) []dateSeg {
		dp := compileDatePattern(pattern, f.patternOverrides(pattern, nil))
		return f.render(&dp, p)
	}
	if field < 0 {
		return write(r.pattern, from), -1
	}
	sameDay := field >= datedata.IntervalDayPeriod
	ip := r.patterns[field]
	if ip.first == "" && ip.second == "" {
		if isFieldUnitIgnored(r.pattern, field) {
			return write(r.pattern, from), -1
		}
		return r.fallbackFormat(write, r.pattern, from, to, sameDay)
	}
	if ip.first == "" {
		return r.fallbackFormat(write, ip.second, from, to, sameDay)
	}
	first, second, firstIndex := from, to, 0
	if ip.laterFirst {
		first, second, firstIndex = to, from, 1
	}
	out := write(ip.first, first)
	if ip.second != "" {
		out = append(out, write(ip.second, second)...)
	}
	return out, firstIndex
}

// fallbackFormat is DateIntervalFormat::fallbackFormat: two whole moments
// joined by the fallback, or, within a day, the date once beside the two
// times.
func (r *rangeFormat) fallbackFormat(write func(string, dateParts) []dateSeg, pattern string,
	from, to dateParts, sameDay bool) ([]dateSeg, int) {
	if sameDay && r.hasDate && r.hasTime && r.dateTimeGlue != "" {
		var out []dateSeg
		firstIndex := -1
		glueSegments(r.dateTimeGlue, func(arg int) {
			if arg == 0 {
				var segs []dateSeg
				segs, firstIndex = r.fallbackRange(write, r.timePattern, from, to)
				out = append(out, segs...)
			} else {
				out = append(out, write(r.datePattern, from)...)
			}
		}, func(text string) { out = append(out, dateSeg{0, text}) })
		return out, firstIndex
	}
	return r.fallbackRange(write, pattern, from, to)
}

// fallbackRange is fallbackFormatRange: both moments in one pattern, joined
// by the fallback.
func (r *rangeFormat) fallbackRange(write func(string, dateParts) []dateSeg, pattern string,
	from, to dateParts) ([]dateSeg, int) {
	var out []dateSeg
	firstIndex := 0
	if r.info.laterFirst {
		firstIndex = 1
	}
	glueSegments(r.info.fallback, func(arg int) {
		if arg == 0 {
			out = append(out, write(pattern, from)...)
		} else {
			out = append(out, write(pattern, to)...)
		}
	}, func(text string) { out = append(out, dateSeg{0, text}) })
	return out, firstIndex
}

// glueSegments walks a two-argument SimpleFormatter pattern, calling arg for
// each argument and text for the literal text between them.
func glueSegments(pattern string, arg func(int), text func(string)) {
	with := simpleFormat(pattern, "\x00", "\x01")
	start := 0
	for i := 0; i < len(with); i++ {
		if with[i] == 0 || with[i] == 1 {
			if i > start {
				text(with[start:i])
			}
			arg(int(with[i]))
			start = i + 1
		}
	}
	if start < len(with) {
		text(with[start:])
	}
}

// RangeSource says which end of a range a part of it writes.
type RangeSource int

const (
	// SourceShared is a part both ends share.
	SourceShared RangeSource = iota
	// SourceStartRange is a part of the start.
	SourceStartRange
	// SourceEndRange is a part of the end.
	SourceEndRange
)

// A RangePart is one piece of a formatted range.
type RangePart struct {
	Kind   PartKind
	Value  string
	Source RangeSource
}

// FormatRange writes a range of two moments.
func (f *DateTimeFormat) FormatRange(start, end time.Time) string {
	var b strings.Builder
	for _, p := range f.FormatRangeToParts(start, end) {
		b.WriteString(p.Value)
	}
	return b.String()
}

// FormatRangeToParts writes a range of two moments as the pieces it is made
// of, each marked with the end it belongs to. Moments that differ in nothing
// the formatter shows are written once, as Format writes them.
func (f *DateTimeFormat) FormatRangeToParts(start, end time.Time) []RangePart {
	segs, firstIndex := f.ranges.format(f, f.instant(start), f.instant(end))
	var spans [2][2]int
	hasSpans := false
	if firstIndex >= 0 {
		spans, hasSpans = overlapSpans(segs, firstIndex)
	}
	if !hasSpans {
		// No part of the range is the start's or the end's alone, which V8
		// reads as the moments being the same.
		var out []RangePart
		for _, p := range f.FormatToParts(start) {
			out = append(out, RangePart{Kind: p.Kind, Value: p.Value, Source: SourceShared})
		}
		return out
	}

	// V8's parts: each field with the end whose span holds it, and the text
	// between fields as one literal.
	source := func(from, to int) RangeSource {
		switch {
		case spans[0][0] <= from && to <= spans[0][1]:
			return SourceStartRange
		case spans[1][0] <= from && to <= spans[1][1]:
			return SourceEndRange
		}
		return SourceShared
	}
	var text strings.Builder
	var out []RangePart
	literalFrom := -1
	flush := func() {
		if literalFrom >= 0 && text.Len() > literalFrom {
			out = append(out, RangePart{PartLiteral, text.String()[literalFrom:], source(literalFrom, text.Len())})
		}
		literalFrom = -1
	}
	for _, s := range segs {
		if s.letter == 0 {
			if literalFrom < 0 {
				literalFrom = text.Len()
			}
			text.WriteString(s.value)
			continue
		}
		flush()
		from := text.Len()
		text.WriteString(s.value)
		out = append(out, RangePart{datePartKind(s.letter), s.value, source(from, text.Len())})
	}
	flush()
	if f.opts.Compat.Has(NarrowSpace) {
		for i := range out {
			out[i].Value = strings.ReplaceAll(out[i].Value, " ", " ")
		}
	}
	return out
}

// overlapSpans is FormattedValueFieldPositionIteratorImpl::addOverlapSpans:
// the extent of the fields that appear twice, first time and second, which
// ICU takes to be the two moments. The span of the start comes first.
func overlapSpans(segs []dateSeg, firstIndex int) ([2][2]int, bool) {
	type field struct {
		letter       byte
		start, limit int
	}
	var fields []field
	pos := 0
	for _, s := range segs {
		n := len(utf16.Encode([]rune(s.value)))
		if s.letter != 0 {
			fields = append(fields, field{s.letter, pos, pos + n})
		}
		pos += n
	}
	const none = int(^uint32(0) >> 1)
	s1a, s1b, s2a, s2b := none, 0, none, 0
	for i := range fields {
		for j := i + 1; j < len(fields); j++ {
			if fields[i].letter != fields[j].letter {
				continue
			}
			s1a, s1b = min(s1a, fields[i].start), max(s1b, fields[i].limit)
			s2a, s2b = min(s2a, fields[j].start), max(s2b, fields[j].limit)
			break
		}
	}
	if s1a == none {
		return [2][2]int{}, false
	}
	// Positions were counted in UTF-16, as ICU's are; the parts are
	// matched against byte offsets, so convert back.
	toBytes := func(u int) int {
		pos, units := 0, 0
		for _, s := range segs {
			for _, r := range s.value {
				if units >= u {
					return pos
				}
				units += len(utf16.Encode([]rune{r}))
				pos += len(string(r))
			}
		}
		return pos
	}
	var spans [2][2]int
	spans[firstIndex] = [2]int{toBytes(s1a), toBytes(s1b)}
	spans[1-firstIndex] = [2]int{toBytes(s2a), toBytes(s2b)}
	return spans, true
}
