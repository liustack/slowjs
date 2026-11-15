package intl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/rbnfdata"
)

// Rule-based numbering: the algorithmic numbering systems date patterns may
// ask for -- Roman numerals for Hawaiian months, Hebrew numerals for Hebrew
// dates, 元 for the first year of a Japanese era. ICU writes them with its
// rule-based number formatter (RBNF), from rules its data carries; this
// interprets the same rules, for whole numbers, following nfrule.cpp and
// nfrs.cpp.
//
// A rule set is a list of rules by base value, "100: c[>>];". A number is
// written by the rule with the largest base at or below it: its text, with
// "<<" replaced by the number divided by the rule's divisor, ">>" by the
// remainder, and "==" by the number itself, each written by the same rule
// set or the one named between the marks. Text in brackets is left out when
// the remainder is zero.

// rbnfRules are a group of rule sets, which may call on each other.
type rbnfRules struct {
	sets map[string]*rbnfSet
}

type rbnfSet struct {
	rules    []rbnfRule // by base, ascending
	negative *rbnfRule
}

type rbnfRule struct {
	base     int64
	radix    int64
	exponent int
	parts    []rbnfPart
}

// An rbnfPart is literal text, or a substitution: '<' the quotient, '>' the
// remainder, '=' the number itself, written by a rule set or, where it
// names none, by a decimal pattern.
type rbnfPart struct {
	kind    byte // 0 for text
	text    string
	set     string // "" for the rule's own set
	decimal string // "0" or "#,##0" where the substitution is a pattern
}

// loadRBNF reads a numbering system's rules.
func loadRBNF(src Source, system string) (*rbnfRules, string, error) {
	qjsUseDates()
	b, err := src.Open(MarkerRBNF, DataLocale{})
	if err != nil {
		return nil, "", fmt.Errorf("intl: the rule-based numbering systems: %w", err)
	}
	d, err := rbnfdata.Decode(b)
	if err != nil {
		return nil, "", fmt.Errorf("intl: the rule-based numbering systems: %w", err)
	}
	s, ok := d.System(system)
	if !ok {
		return nil, "", fmt.Errorf("intl: %q is not a rule-based numbering system: %w", system, ErrNotFound)
	}
	g, ok := d.Group(s.Group)
	if !ok {
		return nil, "", fmt.Errorf("intl: the rules %s are missing: %w", s.Group, ErrNotFound)
	}
	rules, err := parseRBNF(g.Rules)
	if err != nil {
		return nil, "", fmt.Errorf("intl: the rules %s: %w", s.Group, err)
	}
	return rules, s.Set, nil
}

// parseRBNF reads a group of rule sets as ICU writes them: a "%name:" line
// opening each set, then one rule a line.
func parseRBNF(lines []string) (*rbnfRules, error) {
	out := &rbnfRules{sets: map[string]*rbnfSet{}}
	var current *rbnfSet
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "%") && strings.HasSuffix(line, ":") {
			current = &rbnfSet{}
			out.sets[strings.TrimSuffix(line, ":")] = current
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("a rule before any rule set: %q", line)
		}
		descriptor, body, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("a rule with no descriptor: %q", line)
		}
		body = strings.TrimSuffix(strings.TrimSpace(body), ";")
		body = strings.TrimPrefix(body, "'")
		descriptor = strings.TrimSpace(descriptor)
		if descriptor == "-x" {
			rule := rbnfRule{radix: 10, parts: parseRBNFBody(body)}
			current.negative = &rule
			continue
		}
		// Fractions, infinity and NaN: dates have none.
		if descriptor == "" || descriptor[0] < '0' || descriptor[0] > '9' {
			continue
		}
		rules, err := makeRBNFRules(descriptor, body)
		if err != nil {
			return nil, err
		}
		current.rules = append(current.rules, rules...)
	}
	return out, nil
}

// makeRBNFRules is NFRule::makeRules for a whole-number rule: a rule whose
// base is a multiple of its divisor and whose text has brackets becomes two,
// the base without the bracketed text and the next number up with it.
func makeRBNFRules(descriptor, body string) ([]rbnfRule, error) {
	base, radix := descriptor, "10"
	if cut := strings.IndexByte(descriptor, '/'); cut >= 0 {
		base, radix = descriptor[:cut], descriptor[cut+1:]
	}
	shifts := len(radix) - len(strings.TrimRight(radix, ">"))
	radix = strings.TrimRight(radix, ">")
	shifts += len(base) - len(strings.TrimRight(base, ">"))
	base = strings.TrimRight(base, ">")
	b, err := strconv.ParseInt(strings.ReplaceAll(base, ",", ""), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("a rule's base %q", descriptor)
	}
	r, err := strconv.ParseInt(radix, 10, 64)
	if err != nil || r < 2 {
		return nil, fmt.Errorf("a rule's radix %q", descriptor)
	}
	exponent := expectedExponent(b, r) - shifts
	rule := rbnfRule{base: b, radix: r, exponent: exponent}

	open := strings.IndexByte(body, '[')
	closing := strings.IndexByte(body, ']')
	if open < 0 || closing < open {
		rule.parts = parseRBNFBody(body)
		return []rbnfRule{rule}, nil
	}
	without := body[:open] + body[closing+1:]
	with := body[:open] + body[open+1:closing] + body[closing+1:]
	if b > 0 && b%pow64(r, exponent) == 0 {
		plain := rule
		plain.parts = parseRBNFBody(without)
		rule.base++
		rule.parts = parseRBNFBody(with)
		return []rbnfRule{plain, rule}, nil
	}
	rule.parts = parseRBNFBody(with)
	return []rbnfRule{rule}, nil
}

// expectedExponent is NFRule::expectedExponent: the power of the radix the
// base value reaches.
func expectedExponent(base, radix int64) int {
	if base < 1 {
		return 0
	}
	e := 0
	for p := radix; p <= base; p *= radix {
		e++
	}
	return e
}

func pow64(radix int64, exponent int) int64 {
	v := int64(1)
	for i := 0; i < exponent; i++ {
		v *= radix
	}
	return v
}

// parseRBNFBody splits a rule's text into literal text and substitutions.
func parseRBNFBody(body string) []rbnfPart {
	var parts []rbnfPart
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			parts = append(parts, rbnfPart{text: text.String()})
			text.Reset()
		}
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '<' && c != '>' && c != '=' {
			text.WriteByte(c)
			continue
		}
		// ">>>" is a remainder written by the rule itself, which for a
		// whole number is the same as ">>".
		if strings.HasPrefix(body[i:], ">>>") {
			flush()
			parts = append(parts, rbnfPart{kind: '>'})
			i += 2
			continue
		}
		end := strings.IndexByte(body[i+1:], c)
		if end < 0 {
			text.WriteByte(c)
			continue
		}
		flush()
		inner := body[i+1 : i+1+end]
		part := rbnfPart{kind: c}
		switch {
		case inner == "":
		case inner[0] == '%':
			part.set = inner
		default:
			part.decimal = inner
		}
		parts = append(parts, part)
		i += end + 1
	}
	flush()
	return parts
}

// format writes a whole number by a rule set.
func (r *rbnfRules) format(set string, n int64) string {
	s, ok := r.sets[set]
	if !ok {
		return strconv.FormatInt(n, 10)
	}
	var b strings.Builder
	r.write(&b, s, n, 0)
	return b.String()
}

func (r *rbnfRules) write(b *strings.Builder, s *rbnfSet, n int64, depth int) {
	if depth > 64 {
		b.WriteString(strconv.FormatInt(n, 10))
		return
	}
	if n < 0 {
		if s.negative == nil {
			n = -n
		} else {
			// The negative rule's ">>" writes the absolute value.
			for _, p := range s.negative.parts {
				if p.kind == 0 {
					b.WriteString(p.text)
				} else {
					r.substitute(b, s, p, -n, depth)
				}
			}
			return
		}
	}
	rule := s.find(n)
	if rule == nil {
		b.WriteString(strconv.FormatInt(n, 10))
		return
	}
	divisor := pow64(rule.radix, rule.exponent)
	for _, p := range rule.parts {
		switch p.kind {
		case 0:
			b.WriteString(p.text)
		case '<':
			r.substitute(b, s, p, n/divisor, depth)
		case '>':
			r.substitute(b, s, p, n%divisor, depth)
		default:
			r.substitute(b, s, p, n, depth)
		}
	}
}

func (r *rbnfRules) substitute(b *strings.Builder, owner *rbnfSet, p rbnfPart, n int64, depth int) {
	switch {
	case p.decimal != "":
		b.WriteString(rbnfDecimal(p.decimal, n))
	case p.set != "":
		if s, ok := r.sets[p.set]; ok {
			r.write(b, s, n, depth+1)
			return
		}
		b.WriteString(strconv.FormatInt(n, 10))
	default:
		r.write(b, owner, n, depth+1)
	}
}

// find is NFRuleSet::findNormalRule: the rule with the largest base at or
// below the number, or the one before it where the rollback rule says so.
func (s *rbnfSet) find(n int64) *rbnfRule {
	lo, hi := 0, len(s.rules)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case s.rules[mid].base == n:
			return &s.rules[mid]
		case s.rules[mid].base > n:
			hi = mid
		default:
			lo = mid + 1
		}
	}
	if hi == 0 {
		return nil
	}
	rule := &s.rules[hi-1]
	if rule.shouldRollBack(n) && hi >= 2 {
		rule = &s.rules[hi-2]
	}
	return rule
}

// shouldRollBack is NFRule's rollback rule: a rule with a remainder whose
// base is not a multiple of its divisor gives way, for a multiple, to the
// rule before it, so that 200 is "two hundred" and not "two hundred zero".
func (rule *rbnfRule) shouldRollBack(n int64) bool {
	for _, p := range rule.parts {
		if p.kind == '>' {
			d := pow64(rule.radix, rule.exponent)
			return n%d == 0 && rule.base%d != 0
		}
	}
	return false
}

// rbnfDecimal writes a whole number by the decimal patterns rules use: "0"
// plainly, "#,##0" with commas between thousands.
func rbnfDecimal(pattern string, n int64) string {
	s := strconv.FormatInt(n, 10)
	if !strings.Contains(pattern, ",") {
		return s
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
