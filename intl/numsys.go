// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// numberSystem is a locale's symbols and number patterns in one numbering
// system. It is built once per process for each locale and system a formatter
// asks for and shared by every formatter and VM.
type numberSystem struct {
	data *cldrData
	ns   string

	digits []string // the ten digits, nil for the ASCII ones

	decimal, group, minus, plus, percent, exponential, infinity, nan string
	currencyDecimal, currencyGroup                                   string

	minGrouping int

	compact sync.Map // "decimal/short" and the like -> *compactData
}

var numberSystems sync.Map // data locale + "/" + numbering system -> *numberSystem

// numberSystemFor returns the symbols of the data locale d in the numbering
// system ns.
func numberSystemFor(d *cldrData, ns string) *numberSystem {
	key := d.name + "/" + ns
	if s, ok := numberSystems.Load(key); ok {
		return s.(*numberSystem)
	}

	s := &numberSystem{data: d, ns: ns}
	if ns != "latn" {
		s.digits = supp().digits[ns]
	}
	sym := func(name string) string { return s.lookup("symbols", name) }
	s.decimal = sym("decimal")
	s.group = sym("group")
	s.minus = sym("minusSign")
	s.plus = sym("plusSign")
	s.percent = sym("percentSign")
	s.exponential = sym("exponential")
	s.infinity = sym("infinity")
	s.nan = sym("nan")
	s.currencyDecimal = s.decimal
	if v := sym("currencyDecimal"); v != "" {
		s.currencyDecimal = v
	}
	s.currencyGroup = s.group
	if v := sym("currencyGroup"); v != "" {
		s.currencyGroup = v
	}
	s.minGrouping = 1
	if n, err := strconv.Atoi(d.str("n/minimumGroupingDigits")); err == nil {
		s.minGrouping = n
	}
	v, _ := numberSystems.LoadOrStore(key, s)
	return v.(*numberSystem)
}

// lookup returns the value of the numbers key kind/rest in the formatter's
// numbering system, or in latn when the locale has none for that system,
// which is how ICU fills a symbol or pattern in.
func (s *numberSystem) lookup(kind, rest string) string {
	if v, ok := s.data.get("n/" + kind + "-numberSystem-" + s.ns + "/" + rest); ok {
		return v
	}

	return s.data.str("n/" + kind + "-numberSystem-latn/" + rest)
}

// pattern returns the parsed number pattern kind/rest.
func (s *numberSystem) pattern(kind, rest string) *numPattern {
	return parseNumPattern(s.lookup(kind, rest))
}

// transliterate replaces the ASCII digits in ascii with the system's.
func (s *numberSystem) transliterate(ascii string) string {
	if s.digits == nil {
		return ascii
	}

	var b strings.Builder
	for i := 0; i < len(ascii); i++ {
		if c := ascii[i]; c >= '0' && c <= '9' {
			b.WriteString(s.digits[c-'0'])
			continue
		}

		b.WriteByte(ascii[i])
	}
	return b.String()
}

// affixTok is one element of a pattern's prefix or suffix: literal text, or a
// symbol that formatting replaces.
type affixTok struct {
	kind byte // 0 for literal text, else one of - + % ‰ ¤ (as 'c')
	n    int  // the number of ¤ signs
	text string
}

// numPattern is a parsed CLDR number pattern such as "¤#,##0.00;(¤#,##0.00)".
// Only what ECMA-402 does not override is kept: the affixes and the grouping
// sizes. The digit counts come from the formatter's options.
type numPattern struct {
	affix       [2][2][]affixTok // [negative][suffix]
	hasNeg      bool
	posHasPlus  bool
	negHasMinus bool
	hasCurrency bool
	noDigits    bool // the number is spelled out, as in French "mille"
	g1, g2      int  // primary and secondary grouping size, 0 for none
}

var numPatterns sync.Map // source -> *numPattern

func parseNumPattern(src string) *numPattern {
	if p, ok := numPatterns.Load(src); ok {
		return p.(*numPattern)
	}

	p := &numPattern{}
	pos, neg, hasNeg := splitPattern(src)
	var groups []int
	p.affix[0][0], groups, p.affix[0][1] = parseSubpattern(pos)
	_, rest := parseAffix(pos)
	p.noDigits = rest == ""
	if hasNeg {
		p.hasNeg = true
		p.affix[1][0], _, p.affix[1][1] = parseSubpattern(neg)
	}
	for i, side := range p.affix {
		for _, a := range side {
			for _, t := range a {
				switch {
				case t.kind == '+' && i == 0:
					p.posHasPlus = true
				case t.kind == '-' && i == 1:
					p.negHasMinus = true
				case t.kind == 'c':
					p.hasCurrency = true
				}
			}
		}
	}
	// groups holds the digit counts between the grouping separators, the
	// last group first. One separator gives a single size for every group.
	switch len(groups) {
	case 0, 1:
		// nop
	case 2:
		p.g1, p.g2 = groups[0], groups[0]
	default:
		p.g1, p.g2 = groups[0], groups[1]
	}
	v, _ := numPatterns.LoadOrStore(src, p)
	return v.(*numPattern)
}

// splitPattern splits a pattern at the ';' between its positive and negative
// subpatterns, honoring quotes.
func splitPattern(src string) (pos, neg string, ok bool) {
	quoted := false
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\'':
			quoted = !quoted
		case ';':
			if !quoted {
				return src[:i], src[i+1:], true
			}
		}
	}
	return src, "", false
}

// parseSubpattern splits one subpattern into its prefix, the digit counts of
// its integer groups from the right, and its suffix.
func parseSubpattern(s string) (prefix []affixTok, groups []int, suffix []affixTok) {
	prefix, s = parseAffix(s)
	i := 0
	groups = []int{0}
	inFraction, inExponent := false, false
loop:
	for ; i < len(s); i++ {
		switch c := s[i]; {
		case c == '#' || c == '@' || c >= '0' && c <= '9':
			if !inFraction && !inExponent {
				groups[len(groups)-1]++
			}
		case c == ',' && !inFraction && !inExponent:
			groups = append(groups, 0)
		case c == '.' && !inExponent:
			inFraction = true
		case c == 'E' && !inExponent:
			inExponent = true
		case c == '+' && inExponent && i > 0 && s[i-1] == 'E':
			// nop
		default:
			break loop
		}
	}
	// Reverse, so that the group nearest the decimal point comes first.
	for a, b := 0, len(groups)-1; a < b; a, b = a+1, b-1 {
		groups[a], groups[b] = groups[b], groups[a]
	}
	if len(groups) == 1 {
		groups = nil
	}
	suffix, _ = parseAffix(s[i:])
	return prefix, groups, suffix
}

// parseAffix reads affix tokens up to the first character of a number pattern
// and returns them along with the rest of s.
func parseAffix(s string) ([]affixTok, string) {
	var r []affixTok
	var lit strings.Builder
	flush := func() {
		if lit.Len() != 0 {
			r = append(r, affixTok{text: lit.String()})
			lit.Reset()
		}
	}
	quoted := false
	for i := 0; i < len(s); {
		c, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case c == '\'':
			if i+1 < len(s) && s[i+1] == '\'' {
				lit.WriteByte('\'')
				i += 2
				continue
			}

			quoted = !quoted
		case quoted:
			lit.WriteRune(c)
		case c == '#' || c == '@' || c == ',' || c == '.' || c >= '0' && c <= '9':
			flush()
			return r, s[i:]
		case c == '-' || c == '+' || c == '%':
			flush()
			r = append(r, affixTok{kind: byte(c)})
		case c == '‰':
			flush()
			r = append(r, affixTok{kind: 'm'})
		case c == '¤':
			flush()
			n := 0
			for strings.HasPrefix(s[i:], "¤") {
				n++
				i += len("¤")
			}
			r = append(r, affixTok{kind: 'c', n: n})
			continue
		default:
			lit.WriteRune(c)
		}
		i += size
	}
	flush()
	return r, ""
}

// Sign types, ICU's PatternSignType.
const (
	signPos = iota
	signPosSign
	signNeg
)

// affixFor returns the prefix or suffix to write for a number of signType,
// following ICU's PatternStringUtils::patternInfoToStringBuilder: a negative
// number uses the negative subpattern when there is one and otherwise puts a
// minus sign in front of the positive prefix, and an explicit plus sign takes
// the place of the minus sign.
func (p *numPattern) affixFor(suffix bool, signType int) []affixTok {
	plusReplacesMinus := signType == signPosSign && !p.posHasPlus
	useNeg := p.hasNeg && (signType == signNeg || p.negHasMinus && plusReplacesMinus)
	side := 0
	if suffix {
		side = 1
	}
	neg := 0
	if useNeg {
		neg = 1
	}
	toks := p.affix[neg][side]
	if !suffix && !useNeg && (signType == signNeg || plusReplacesMinus) {
		toks = append([]affixTok{{kind: '-'}}, toks...)
	}
	if plusReplacesMinus {
		r := make([]affixTok, len(toks))
		for i, t := range toks {
			if t.kind == '-' {
				t.kind = '+'
			}
			r[i] = t
		}
		toks = r
	}
	return toks
}

// compactData is a locale's compact notation patterns of one style, ICU's
// CompactData.
type compactData struct {
	patterns    map[int]map[string]*numPattern // magnitude -> "0", "1" or a plural category -> pattern, nil for "use the standard one"
	multipliers map[int]int
	largest     int
}

// compactPatterns returns the compact patterns for currencies or plain
// numbers in style "short" or "long", falling back the way ICU does: to latn
// digits, then to the short style.
func (s *numberSystem) compactPatterns(currency bool, style string) *compactData {
	key := strconv.FormatBool(currency) + "/" + style
	if c, ok := s.compact.Load(key); ok {
		return c.(*compactData)
	}

	var c *compactData
	for _, try := range [][2]string{{s.ns, style}, {"latn", style}, {s.ns, "short"}, {"latn", "short"}} {
		prefix := "n/decimalFormats-numberSystem-" + try[0] + "/" + try[1] + "/decimalFormat/"
		if currency {
			if try[1] != "short" {
				continue
			}

			prefix = "n/currencyFormats-numberSystem-" + try[0] + "/short/standard/"
		}
		if c = s.readCompact(prefix); c != nil {
			break
		}
	}
	if c == nil {
		c = &compactData{}
	}
	v, _ := s.compact.LoadOrStore(key, c)
	return v.(*compactData)
}

func (s *numberSystem) readCompact(prefix string) *compactData {
	c := &compactData{patterns: map[int]map[string]*numPattern{}, multipliers: map[int]int{}}
	s.data.each(prefix, func(key, value string) {
		rest := key[len(prefix):]
		pow, count, ok := strings.Cut(rest, "-count-")
		if !ok || strings.Contains(count, "-alt-") {
			return
		}

		magnitude := len(pow) - 1
		m := c.patterns[magnitude]
		if m == nil {
			m = map[string]*numPattern{}
			c.patterns[magnitude] = m
		}
		if value == "0" {
			m[count] = nil
		} else {
			m[count] = parseNumPattern(value)
			if _, ok := c.multipliers[magnitude]; !ok {
				if zeros := countZeros(value); zeros > 0 {
					c.multipliers[magnitude] = zeros - magnitude - 1
				}
			}
		}
		if magnitude > c.largest {
			c.largest = magnitude
		}
	})
	if len(c.patterns) == 0 {
		return nil
	}

	return c
}

func countZeros(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '0' {
			n++
		} else if n > 0 {
			break
		}
	}
	return n
}

// multiplier returns the power of ten a number of magnitude m is scaled by,
// zero or negative.
func (c *compactData) multiplier(m int) int {
	if m < 0 {
		return 0
	}

	if m > c.largest {
		m = c.largest
	}
	return c.multipliers[m]
}

// pattern returns the pattern for a scaled number of magnitude m, or nil for
// the standard one. integer is the number's value if it is 0 or 1, else -1,
// and plural its plural category.
func (c *compactData) pattern(m int, integer int64, plural string) *numPattern {
	if m < 0 || len(c.patterns) == 0 {
		return nil
	}

	if m > c.largest {
		m = c.largest
	}
	ps := c.patterns[m]
	switch integer {
	case 0, 1:
		if p, ok := ps[strconv.FormatInt(integer, 10)]; ok {
			return p
		}
	}
	if p, ok := ps[plural]; ok {
		return p
	}

	return ps["other"]
}

// span is a piece of formatted output and the field it belongs to, "" for
// literal text.
type span = part

// toParts turns spans into ECMA-402 parts the way V8 reads ICU's field
// positions: adjacent text of the same field is one part, the white space
// and bidi marks at the ends of a field other than a grouping separator are
// literal text, and literal text between fields is one part.
func toParts(spans []span) []part {
	var merged []span
	for _, s := range spans {
		if s.Value == "" {
			continue
		}

		if n := len(merged); n != 0 && merged[n-1].Type == s.Type && s.Type != "integer" && s.Type != "group" {
			merged[n-1].Value += s.Value
			continue
		}

		merged = append(merged, s)
	}
	var r []part
	add := func(typ, v string) {
		if v == "" {
			return
		}

		if n := len(r); n != 0 && typ == "literal" && r[n-1].Type == "literal" {
			r[n-1].Value += v
			return
		}

		r = append(r, part{typ, v})
	}
	for _, s := range merged {
		if s.Type == "" {
			add("literal", s.Value)
			continue
		}

		if s.Type == "group" {
			add(s.Type, s.Value)
			continue
		}

		v := s.Value
		lead := strings.TrimLeftFunc(v, ignorable)
		core := strings.TrimRightFunc(lead, ignorable)
		if core == "" {
			add("literal", v)
			continue
		}

		add("literal", v[:len(v)-len(lead)])
		add(s.Type, core)
		add("literal", lead[len(core):])
	}
	return r
}

// ignorable is ICU's DEFAULT_IGNORABLES: horizontal white space, bidi
// controls and variation selectors.
func ignorable(r rune) bool {
	switch {
	case r == '\t', unicode.Is(unicode.Zs, r), unicode.Is(unicode.Bidi_Control, r), unicode.Is(unicode.Variation_Selector, r):
		return true
	}
	return false
}

// currencySpaced reports whether currency spacing puts a space between a
// currency symbol ending (or starting) with r and a number starting (or
// ending) with digit: CLDR's currencyMatch [[:^S:]&[:^Z:]] against its
// surroundingMatch [:digit:], the same in every locale.
func currencySpaced(r, digit rune) bool {
	return !unicode.IsSymbol(r) && !unicode.In(r, unicode.Z) && unicode.IsDigit(digit)
}
