package date

import (
	"math"
	"unicode/utf16"

	intl "github.com/liustack/slowjs/internal/gointl"
)

// Date.parse, as V8's DateParser reads a string: ECMA-262's date time
// string format first, and where that fails, the legacy forms browsers
// have long taken.

// Parse is Date.parse for a string given in UTF-16, as JavaScript strings
// are: the time value, NaN where the string is no date. A date-time without
// an offset is local time.
func (e *Environment) Parse(s []uint16) float64 {
	var out [outputSize]float64
	if !parseDate(s, &out) {
		return math.NaN()
	}
	day := MakeDay(out[outYear], out[outMonth], out[outDay])
	t := MakeTime(out[outHour], out[outMinute], out[outSecond], out[outMillisecond])
	date := MakeDate(day, t)
	if math.IsNaN(out[outUTCOffset]) {
		if !(date >= -maxTimeBeforeUTC && date <= maxTimeBeforeUTC) {
			return math.NaN()
		}
		ms := int64(date)
		return TimeClip(float64(ms - e.offsetFromLocal(ms)))
	}
	// The product is rounded before the difference, as MakeDate's is.
	return TimeClip(date - float64(out[outUTCOffset]*1000))
}

// ParseString is Parse for a Go string.
func (e *Environment) ParseString(s string) float64 { return e.Parse(utf16.Encode([]rune(s))) }

func (e *Environment) offsetFromLocal(ms int64) int64 {
	return int64(e.UTCOffsetFromLocal(ms)) * msPerSecond
}

// UTCOffsetFromLocal is the offset, in seconds, a local time value is read
// with.
func (e *Environment) UTCOffsetFromLocal(local int64) int {
	return e.tz.OffsetFromLocal(local, intl.Former, intl.Former).Total()
}

const (
	outYear = iota
	outMonth
	outDay
	outHour
	outMinute
	outSecond
	outMillisecond
	outUTCOffset
	outputSize
)

// none is kNone, a value not given.
const none = math.MaxInt32

const maxSignificantDigits = 9

func between(x, lo, hi int) bool { return uint32(x-lo) <= uint32(hi-lo) }

// inputReader is DateParser::InputReader: a character at a time, 0 at the
// end, and a NUL ending the string as the end does.
type inputReader struct {
	s     []uint16
	index int
	ch    rune
}

func (r *inputReader) next() {
	if r.index < len(r.s) {
		r.ch = rune(r.s[r.index])
	} else {
		r.ch = 0
	}
	r.index++
}

func (r *inputReader) position() int { return r.index }

func (r *inputReader) readUnsignedNumeral() int {
	n, i := 0, 0
	for r.ch == '0' {
		r.next()
	}
	for r.isDigit() {
		if i < maxSignificantDigits {
			n = n*10 + int(r.ch-'0')
		}
		i++
		r.next()
	}
	return n
}

func (r *inputReader) readWord(prefix *[3]rune) int {
	n := 0
	for ; r.ch >= 'A' && !isWhiteSpace(r.ch); r.next() {
		if n < len(prefix) {
			prefix[n] = r.ch | 0x20
		}
		n++
	}
	for i := n; i < len(prefix); i++ {
		prefix[i] = 0
	}
	return n
}

func (r *inputReader) skip(c rune) bool {
	if r.ch == c {
		r.next()
		return true
	}
	return false
}

func (r *inputReader) isDigit() bool { return r.ch >= '0' && r.ch <= '9' }
func (r *inputReader) isEnd() bool   { return r.ch == 0 }

// isWhiteSpace is ECMA-262's WhiteSpace: tab, vertical tab, form feed,
// the byte order mark, and the space separators.
func isWhiteSpace(c rune) bool {
	switch c {
	case 0x09, 0x0b, 0x0c, 0x20, 0xa0, 0x1680, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return c >= 0x2000 && c <= 0x200a
}

func isLineTerminator(c rune) bool {
	return c == 0x0a || c == 0x0d || c == 0x2028 || c == 0x2029
}

func (r *inputReader) skipWhiteSpace() bool {
	if isWhiteSpace(r.ch) || isLineTerminator(r.ch) {
		r.next()
		return true
	}
	return false
}

func (r *inputReader) skipParentheses() bool {
	if r.ch != '(' {
		return false
	}
	balance := 0
	for {
		if r.ch == ')' {
			balance--
		} else if r.ch == '(' {
			balance++
		}
		r.next()
		if !(balance > 0 && r.ch != 0) {
			break
		}
	}
	return true
}

// Keyword types.
const (
	kwInvalid = iota
	kwMonthName
	kwTimeZoneName
	kwTimeSeparator
	kwAmPm
)

// Token tags.
const (
	tagInvalid    = -6
	tagUnknown    = -5
	tagWhiteSpace = -4
	tagNumber     = -3
	tagSymbol     = -2
	tagEndOfInput = -1
)

type token struct {
	tag, length, value int
}

func (t token) isInvalid() bool                { return t.tag == tagInvalid }
func (t token) isNumber() bool                 { return t.tag == tagNumber }
func (t token) isSymbol() bool                 { return t.tag == tagSymbol }
func (t token) isWhiteSpace() bool             { return t.tag == tagWhiteSpace }
func (t token) isEndOfInput() bool             { return t.tag == tagEndOfInput }
func (t token) isKeyword() bool                { return t.tag >= 0 }
func (t token) isSymbolOf(c int) bool          { return t.isSymbol() && t.value == c }
func (t token) isKeywordType(k int) bool       { return t.tag == k }
func (t token) isFixedLengthNumber(n int) bool { return t.isNumber() && t.length == n }
func (t token) isAsciiSign() bool              { return t.tag == tagSymbol && (t.value == '-' || t.value == '+') }
func (t token) asciiSign() int                 { return 44 - t.value }
func (t token) isKeywordZ() bool {
	return t.isKeywordType(kwTimeZoneName) && t.length == 1 && t.value == 0
}

// keywords is DateParser::KeywordTable: three letters, a type, a value.
var keywords = []struct {
	prefix [3]rune
	kind   int
	value  int
}{
	{[3]rune{'j', 'a', 'n'}, kwMonthName, 1},
	{[3]rune{'f', 'e', 'b'}, kwMonthName, 2},
	{[3]rune{'m', 'a', 'r'}, kwMonthName, 3},
	{[3]rune{'a', 'p', 'r'}, kwMonthName, 4},
	{[3]rune{'m', 'a', 'y'}, kwMonthName, 5},
	{[3]rune{'j', 'u', 'n'}, kwMonthName, 6},
	{[3]rune{'j', 'u', 'l'}, kwMonthName, 7},
	{[3]rune{'a', 'u', 'g'}, kwMonthName, 8},
	{[3]rune{'s', 'e', 'p'}, kwMonthName, 9},
	{[3]rune{'o', 'c', 't'}, kwMonthName, 10},
	{[3]rune{'n', 'o', 'v'}, kwMonthName, 11},
	{[3]rune{'d', 'e', 'c'}, kwMonthName, 12},
	{[3]rune{'a', 'm', 0}, kwAmPm, 0},
	{[3]rune{'p', 'm', 0}, kwAmPm, 12},
	{[3]rune{'u', 't', 0}, kwTimeZoneName, 0},
	{[3]rune{'u', 't', 'c'}, kwTimeZoneName, 0},
	{[3]rune{'z', 0, 0}, kwTimeZoneName, 0},
	{[3]rune{'g', 'm', 't'}, kwTimeZoneName, 0},
	{[3]rune{'c', 'd', 't'}, kwTimeZoneName, -5},
	{[3]rune{'c', 's', 't'}, kwTimeZoneName, -6},
	{[3]rune{'e', 'd', 't'}, kwTimeZoneName, -4},
	{[3]rune{'e', 's', 't'}, kwTimeZoneName, -5},
	{[3]rune{'m', 'd', 't'}, kwTimeZoneName, -6},
	{[3]rune{'m', 's', 't'}, kwTimeZoneName, -7},
	{[3]rune{'p', 'd', 't'}, kwTimeZoneName, -7},
	{[3]rune{'p', 's', 't'}, kwTimeZoneName, -8},
	{[3]rune{'t', 0, 0}, kwTimeSeparator, 0},
}

// lookupKeyword is KeywordTable::Lookup: a word longer than three letters
// matches only a month.
func lookupKeyword(prefix [3]rune, n int) (kind, value int) {
	for _, k := range keywords {
		if k.prefix == prefix && (n <= 3 || k.kind == kwMonthName) {
			return k.kind, k.value
		}
	}
	return kwInvalid, 0
}

// tokenizer is DateStringTokenizer.
type tokenizer struct {
	in   *inputReader
	peek token
}

func newTokenizer(in *inputReader) *tokenizer {
	t := &tokenizer{in: in}
	t.peek = t.scan()
	return t
}

func (t *tokenizer) next() token {
	r := t.peek
	t.peek = t.scan()
	return r
}

func (t *tokenizer) skipSymbol(c int) bool {
	if t.peek.isSymbolOf(c) {
		t.peek = t.scan()
		return true
	}
	return false
}

func (t *tokenizer) scan() token {
	in := t.in
	pre := in.position()
	if in.isEnd() {
		return token{tagEndOfInput, 0, -1}
	}
	if in.isDigit() {
		n := in.readUnsignedNumeral()
		return token{tagNumber, in.position() - pre, n}
	}
	for _, c := range []rune{':', '-', '+', '.', ')'} {
		if in.skip(c) {
			return token{tagSymbol, 1, int(c)}
		}
	}
	if in.ch >= 'A' && !isWhiteSpace(in.ch) {
		var prefix [3]rune
		n := in.readWord(&prefix)
		kind, value := lookupKeyword(prefix, n)
		return token{kind, n, value}
	}
	if in.skipWhiteSpace() {
		return token{tagWhiteSpace, in.position() - pre, -1}
	}
	if in.skipParentheses() {
		return token{tagUnknown, 1, -1}
	}
	in.next()
	return token{tagUnknown, 1, -1}
}

// timeZoneComposer is DateParser::TimeZoneComposer.
type timeZoneComposer struct {
	sign, hour, minute int
}

func newTimeZoneComposer() timeZoneComposer { return timeZoneComposer{none, none, none} }

func (z *timeZoneComposer) set(hours int) {
	z.sign = 1
	if hours < 0 {
		z.sign = -1
	}
	z.hour = hours * z.sign
	z.minute = 0
}

func (z *timeZoneComposer) setSign(sign int) {
	z.sign = 1
	if sign < 0 {
		z.sign = -1
	}
}

func (z *timeZoneComposer) isExpecting(n int) bool {
	return z.hour != none && z.minute == none && between(n, 0, 59)
}

func (z *timeZoneComposer) isUTC() bool   { return z.hour == 0 && z.minute == 0 }
func (z *timeZoneComposer) isEmpty() bool { return z.hour == none }

func (z *timeZoneComposer) write(out *[outputSize]float64) bool {
	if z.sign == none {
		out[outUTCOffset] = math.NaN()
		return true
	}
	if z.hour == none {
		z.hour = 0
	}
	if z.minute == none {
		z.minute = 0
	}
	// Unsigned arithmetic, as V8 does it to avoid overflow, wrapping.
	total := uint32(z.hour)*3600 + uint32(z.minute)*60
	if total > 1<<31-1 {
		return false
	}
	seconds := int(total)
	if z.sign < 0 {
		seconds = -seconds
	}
	out[outUTCOffset] = float64(seconds)
	return true
}

// timeComposer is DateParser::TimeComposer.
type timeComposer struct {
	comp       [4]int
	index      int
	hourOffset int
}

func (c *timeComposer) isEmpty() bool { return c.index == 0 }

func (c *timeComposer) isExpecting(n int) bool {
	return c.index == 1 && between(n, 0, 59) || c.index == 2 && between(n, 0, 59) ||
		c.index == 3 && between(n, 0, 999)
}

func (c *timeComposer) add(n int) bool {
	if c.index < len(c.comp) {
		c.comp[c.index] = n
		c.index++
		return true
	}
	return false
}

func (c *timeComposer) addFinal(n int) bool {
	if !c.add(n) {
		return false
	}
	for c.index < len(c.comp) {
		c.comp[c.index] = 0
		c.index++
	}
	return true
}

func (c *timeComposer) write(out *[outputSize]float64) bool {
	for c.index < len(c.comp) {
		c.comp[c.index] = 0
		c.index++
	}
	hour, minute, second, ms := c.comp[0], c.comp[1], c.comp[2], c.comp[3]
	if c.hourOffset != none {
		if !between(hour, 0, 12) {
			return false
		}
		hour %= 12
		hour += c.hourOffset
	}
	if !between(hour, 0, 23) || !between(minute, 0, 59) || !between(second, 0, 59) || !between(ms, 0, 999) {
		if hour != 24 || minute != 0 || second != 0 || ms != 0 {
			return false
		}
	}
	out[outHour], out[outMinute], out[outSecond], out[outMillisecond] =
		float64(hour), float64(minute), float64(second), float64(ms)
	return true
}

// dayComposer is DateParser::DayComposer.
type dayComposer struct {
	comp       [3]int
	index      int
	namedMonth int
	isoDate    bool
}

func (d *dayComposer) isEmpty() bool { return d.index == 0 }

func (d *dayComposer) add(n int) bool {
	if d.index < len(d.comp) {
		d.comp[d.index] = n
		d.index++
		return true
	}
	return false
}

func isDay(x int) bool   { return between(x, 1, 31) }
func isMonth(x int) bool { return between(x, 1, 12) }

func (d *dayComposer) write(out *[outputSize]float64) bool {
	if d.index < 1 {
		return false
	}
	for d.index < len(d.comp) {
		d.comp[d.index] = 1
		d.index++
	}
	year, month, day := 0, none, none
	if d.namedMonth == none {
		if d.isoDate || d.index == 3 && !isDay(d.comp[0]) {
			year, month, day = d.comp[0], d.comp[1], d.comp[2]
		} else {
			month, day = d.comp[0], d.comp[1]
			if d.index == 3 {
				year = d.comp[2]
			}
		}
	} else {
		month = d.namedMonth
		switch {
		case d.index == 1:
			day = d.comp[0]
		case !isDay(d.comp[0]):
			year, day = d.comp[0], d.comp[1]
		default:
			day, year = d.comp[0], d.comp[1]
		}
	}
	if !d.isoDate {
		if between(year, 0, 49) {
			year += 2000
		} else if between(year, 50, 99) {
			year += 1900
		}
	}
	if !isMonth(month) || !isDay(day) {
		return false
	}
	out[outYear], out[outMonth], out[outDay] = float64(year), float64(month-1), float64(day)
	return true
}

// readMilliseconds is DateParser::ReadMilliseconds: the first three
// significant digits of a numeral, by its length.
func readMilliseconds(t token) int {
	n, length := t.value, t.length
	if length < 3 {
		switch length {
		case 1:
			n *= 100
		case 2:
			n *= 10
		}
	} else if length > 3 {
		if length > maxSignificantDigits {
			length = maxSignificantDigits
		}
		factor := 1
		for {
			factor *= 10
			length--
			if length <= 3 {
				break
			}
		}
		n /= factor
	}
	return n
}

// parseDate is DateParser::Parse.
func parseDate(s []uint16, out *[outputSize]float64) bool {
	in := &inputReader{s: s}
	in.next()
	scanner := newTokenizer(in)
	tz := newTimeZoneComposer()
	tm := timeComposer{hourOffset: none}
	day := dayComposer{namedMonth: none}
	next := parseES5DateTime(scanner, &day, &tm, &tz)
	if next.isInvalid() {
		return false
	}
	hasReadNumber := !day.isEmpty()
	for tok := next; !tok.isEndOfInput(); tok = scanner.next() {
		switch {
		case tok.isNumber():
			hasReadNumber = true
			n := tok.value
			if scanner.skipSymbol(':') {
				if scanner.skipSymbol(':') {
					if !tm.isEmpty() {
						return false
					}
					tm.add(n)
					tm.add(0)
				} else {
					if !tm.add(n) {
						return false
					}
					if scanner.peek.isSymbolOf('.') {
						scanner.next()
					}
				}
			} else if scanner.skipSymbol('.') && tm.isExpecting(n) {
				tm.add(n)
				if !scanner.peek.isNumber() {
					return false
				}
				ms := readMilliseconds(scanner.next())
				if ms < 0 {
					return false
				}
				tm.addFinal(ms)
			} else if tz.isExpecting(n) {
				tz.minute = n
			} else if tm.isExpecting(n) {
				tm.addFinal(n)
				p := scanner.peek
				if !p.isEndOfInput() && !p.isWhiteSpace() && !p.isKeywordZ() && !p.isAsciiSign() {
					return false
				}
			} else {
				if !day.add(n) {
					return false
				}
				scanner.skipSymbol('-')
			}
		case tok.isKeyword():
			kind, value := tok.tag, tok.value
			switch {
			case kind == kwAmPm && !tm.isEmpty():
				tm.hourOffset = value
			case kind == kwMonthName:
				day.namedMonth = value
				scanner.skipSymbol('-')
			case kind == kwTimeZoneName && hasReadNumber:
				tz.set(value)
			default:
				if hasReadNumber {
					return false
				}
				if scanner.peek.isNumber() {
					return false
				}
			}
		case tok.isAsciiSign() && (tz.isUTC() || !tm.isEmpty()):
			tz.setSign(tok.asciiSign())
			n, length := 0, 0
			if scanner.peek.isNumber() {
				nt := scanner.next()
				length, n = nt.length, nt.value
			}
			hasReadNumber = true
			switch {
			case scanner.peek.isSymbolOf(':'):
				tz.hour = n
				tz.minute = none
			case length == 2 || length == 1:
				tz.hour = n
				tz.minute = 0
			case length == 4 || length == 3:
				tz.hour = n / 100
				tz.minute = n % 100
			default:
				return false
			}
		case (tok.isAsciiSign() || tok.isSymbolOf(')')) && hasReadNumber:
			return false
		}
	}
	return day.write(out) && tm.write(out) && tz.write(out)
}

// parseES5DateTime is DateParser::ParseES5DateTime: the date time string
// format, as far as the string follows it. It returns the next token for
// the legacy parser, the end where it is done, or an invalid token where
// the string can be no date.
func parseES5DateTime(scanner *tokenizer, day *dayComposer, tm *timeComposer, tz *timeZoneComposer) token {
	if scanner.peek.isAsciiSign() {
		sign := scanner.next()
		if !scanner.peek.isFixedLengthNumber(6) {
			return sign
		}
		s := sign.asciiSign()
		year := scanner.next().value
		if s < 0 && year == 0 {
			return sign
		}
		day.add(s * year)
	} else if scanner.peek.isFixedLengthNumber(4) {
		day.add(scanner.next().value)
	} else {
		return scanner.next()
	}
	if scanner.skipSymbol('-') {
		if !scanner.peek.isFixedLengthNumber(2) || !isMonth(scanner.peek.value) {
			return scanner.next()
		}
		day.add(scanner.next().value)
		if scanner.skipSymbol('-') {
			if !scanner.peek.isFixedLengthNumber(2) || !isDay(scanner.peek.value) {
				return scanner.next()
			}
			day.add(scanner.next().value)
		}
	}
	invalid := token{tagInvalid, 0, -1}
	if !scanner.peek.isKeywordType(kwTimeSeparator) {
		if !scanner.peek.isEndOfInput() {
			return scanner.next()
		}
	} else {
		scanner.next()
		if !scanner.peek.isFixedLengthNumber(2) || !between(scanner.peek.value, 0, 24) {
			return invalid
		}
		hourIs24 := scanner.peek.value == 24
		tm.add(scanner.next().value)
		if !scanner.skipSymbol(':') {
			return invalid
		}
		if !scanner.peek.isFixedLengthNumber(2) || !between(scanner.peek.value, 0, 59) ||
			hourIs24 && scanner.peek.value > 0 {
			return invalid
		}
		tm.add(scanner.next().value)
		if scanner.skipSymbol(':') {
			if !scanner.peek.isFixedLengthNumber(2) || !between(scanner.peek.value, 0, 59) ||
				hourIs24 && scanner.peek.value > 0 {
				return invalid
			}
			tm.add(scanner.next().value)
			if scanner.skipSymbol('.') {
				if !scanner.peek.isNumber() || hourIs24 && scanner.peek.value > 0 {
					return invalid
				}
				tm.add(readMilliseconds(scanner.next()))
			}
		}
		if scanner.peek.isKeywordZ() {
			scanner.next()
			tz.set(0)
		} else if scanner.peek.isSymbolOf('+') || scanner.peek.isSymbolOf('-') {
			if scanner.next().value == '+' {
				tz.setSign(1)
			} else {
				tz.setSign(-1)
			}
			if scanner.peek.isFixedLengthNumber(4) {
				hourmin := scanner.next().value
				hour, min := hourmin/100, hourmin%100
				if !between(hour, 0, 23) || !between(min, 0, 59) {
					return invalid
				}
				tz.hour, tz.minute = hour, min
			} else {
				if !scanner.peek.isFixedLengthNumber(2) || !between(scanner.peek.value, 0, 23) {
					return invalid
				}
				tz.hour = scanner.next().value
				if !scanner.skipSymbol(':') {
					return invalid
				}
				if !scanner.peek.isFixedLengthNumber(2) || !between(scanner.peek.value, 0, 59) {
					return invalid
				}
				tz.minute = scanner.next().value
			}
		}
		if !scanner.peek.isEndOfInput() {
			return invalid
		}
	}
	// A date alone is UTC; a date and time without an offset, local.
	if tz.isEmpty() && tm.isEmpty() {
		tz.set(0)
	}
	day.isoDate = true
	return token{tagEndOfInput, 0, -1}
}
