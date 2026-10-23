package intl

import (
	"fmt"
	"math"
	"sort"

	"github.com/liustack/slowjs/internal/gointl/internal/plurdata"
)

// Intl.PluralRules.
//
// Which form a word takes beside a number is the language's business, not the
// number's: English has two forms, Polish four, Arabic six, Japanese one. A
// PluralRules answers which form a particular number calls for, so that a
// caller can choose between wordings it holds itself.
//
// The answer depends on how the number is *written*, not only on its value.
// English "1 day" and "1.0 days" are the same quantity and different plurals,
// because one has a written decimal and the other does not. That is why the
// digit options are here and why selecting formats the number first.

// PluralType is whether the rules are for counting or for ordering.
type PluralType int

const (
	// Cardinal is for counting: one day, two days. It is the default.
	Cardinal PluralType = iota
	// Ordinal is for ordering: the 1st, the 2nd, the 3rd.
	Ordinal
)

// PluralCategory is the form a number calls for.
type PluralCategory string

const (
	PluralZero  PluralCategory = "zero"
	PluralOne   PluralCategory = "one"
	PluralTwo   PluralCategory = "two"
	PluralFew   PluralCategory = "few"
	PluralMany  PluralCategory = "many"
	PluralOther PluralCategory = "other"
)

// PluralRulesOptions mirrors the option bag of Intl.PluralRules. Its zero
// value is ECMA-402's default in every field.
type PluralRulesOptions struct {
	Type PluralType

	// ECMA-402 gives PluralRules the whole set of digit options a
	// NumberFormat has, because which form a number takes depends on how it
	// would be written and these decide that.
	MinimumIntegerDigits     int
	MinimumFractionDigits    *int
	MaximumFractionDigits    *int
	MinimumSignificantDigits *int
	MaximumSignificantDigits *int
	RoundingPriority         RoundingPriority
	RoundingMode             RoundingMode
	RoundingIncrement        int
	TrailingZeroDisplay      TrailingZeroDisplay
	Notation                 Notation
	// CompactDisplay is how compact notation writes a magnitude, which
	// decides the power of ten written apart: a locale may have a word for
	// ten thousand in one width and not the other.
	CompactDisplay CompactDisplay

	// Compat chooses Node's side of PluralRulesDigits.
	Compat Compat
}

// A PluralRules chooses the plural form for a number in one locale. It never
// changes after it is built and is safe for any number of goroutines to share.
type PluralRules struct {
	locale Locale
	opts   PluralRulesOptions
	// rules are in CLDR's order, which Categories reports; tried are the
	// same rules in the order ICU tries them.
	rules  []compiledRule
	tried  []compiledRule
	ranges []plurdata.Range
	digitPlan
	// written is how a number is written in compact, scientific or
	// engineering notation, whose power of ten the rules can count; nil in
	// standard notation.
	written *NumberFormat
}

type compiledRule struct {
	category PluralCategory
	rule     *pluralRule
}

// NewPluralRules builds rules from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewPluralRules(loc Locale, opts PluralRulesOptions) (*PluralRules, error) {
	return NewPluralRulesFrom(Embedded, loc, opts)
}

// NewPluralRulesFrom builds rules from a source of the caller's own.
func NewPluralRulesFrom(src Source, loc Locale, opts PluralRulesOptions) (*PluralRules, error) {
	for _, v := range []fmt.Stringer{opts.Type, opts.Notation, opts.CompactDisplay} {
		if !named(v) {
			return nil, fmt.Errorf("%w: %s", ErrOption, v)
		}
	}
	if err := (digitOptions{minInt: opts.MinimumIntegerDigits, minFrac: opts.MinimumFractionDigits,
		maxFrac: opts.MaximumFractionDigits, minSig: opts.MinimumSignificantDigits,
		maxSig: opts.MaximumSignificantDigits, increment: opts.RoundingIncrement,
		mode: opts.RoundingMode, priority: opts.RoundingPriority,
		trailing: opts.TrailingZeroDisplay}).check(); err != nil {
		return nil, err
	}
	data, err := loadPlurals(src, loc)
	if err != nil {
		return nil, err
	}
	set := data.Cardinal
	if opts.Type == Ordinal {
		set = data.Ordinal
	}

	// PluralRules uses nothing of the Unicode extension.
	p := &PluralRules{locale: loc.onlyKeywords(), opts: opts, ranges: data.Ranges}
	for _, r := range set {
		parsed, err := parsePluralRule(r.Condition)
		if err != nil {
			return nil, fmt.Errorf("intl: %s: the %q rule: %w", loc, r.Category, err)
		}
		p.rules = append(p.rules, compiledRule{PluralCategory(r.Category), parsed})
	}
	// ICU reads a language's rules out of a resource table, whose keys are
	// sorted, and keeps "other" last: few, many, one, two, zero. Where rules
	// overlap the first wins, and French's "many" -- any number with an
	// exponent outside 0 to 5 -- then wins over its "one" for 5E-1.
	p.tried = append([]compiledRule(nil), p.rules...)
	sort.SliceStable(p.tried, func(i, j int) bool {
		a, b := p.tried[i].category, p.tried[j].category
		if (a == PluralOther) != (b == PluralOther) {
			return b == PluralOther
		}
		return a < b
	})

	plan, err := digitRequest{
		minInt:         opts.MinimumIntegerDigits,
		minFrac:        opts.MinimumFractionDigits,
		maxFrac:        opts.MaximumFractionDigits,
		minSig:         opts.MinimumSignificantDigits,
		maxSig:         opts.MaximumSignificantDigits,
		priority:       opts.RoundingPriority,
		mode:           opts.RoundingMode,
		increment:      opts.RoundingIncrement,
		trailingZero:   opts.TrailingZeroDisplay,
		compact:        opts.Notation == NotationCompact,
		maxFracDefault: 3,
	}.resolve()
	if err != nil {
		return nil, err
	}
	p.digitPlan = plan
	if opts.Notation != NotationStandard {
		// The operands are those of the number as ICU's number formatter
		// writes it in the notation: "1.5M" is one and a half with an
		// exponent of six, which French calls "many".
		p.written, err = NewNumberFormatFrom(src, loc, NumberFormatOptions{
			Notation:                 opts.Notation,
			CompactDisplay:           opts.CompactDisplay,
			MinimumIntegerDigits:     opts.MinimumIntegerDigits,
			MinimumFractionDigits:    opts.MinimumFractionDigits,
			MaximumFractionDigits:    opts.MaximumFractionDigits,
			MinimumSignificantDigits: opts.MinimumSignificantDigits,
			MaximumSignificantDigits: opts.MaximumSignificantDigits,
			RoundingPriority:         opts.RoundingPriority,
			RoundingMode:             opts.RoundingMode,
			RoundingIncrement:        opts.RoundingIncrement,
			TrailingZeroDisplay:      opts.TrailingZeroDisplay,
		})
		if err != nil {
			return nil, err
		}
	}
	return p, nil
}

// loadPlurals walks the fallback chain until a locale has rules. A language
// with none is not an error: the root has a single "other", which is right for
// a language that makes no distinction.
//
// The chain is truncation alone, as ICU's PluralRules walks it
// (getRuleFromResource, by uloc_getParent), not CLDR's parent locales: a
// Serbian written in Latin, whose parent is the root, still counts as
// Serbian does, so "1 sat" is singular.
func loadPlurals(src Source, loc Locale) (*plurdata.Locale, error) {
	var result plurdata.Locale
	for _, d := range loc.Fallback() {
		b, err := src.Open(MarkerPlurals, d)
		if err != nil {
			continue
		}
		l, err := plurdata.Decode(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the plural rules for %s: %w", d, err)
		}
		if len(result.Cardinal) == 0 {
			result.Cardinal = l.Cardinal
		}
		if len(result.Ordinal) == 0 {
			result.Ordinal = l.Ordinal
		}
		if len(result.Ranges) == 0 {
			result.Ranges = l.Ranges
		}
		if len(result.Cardinal) > 0 && len(result.Ordinal) > 0 && len(result.Ranges) > 0 {
			return &result, nil
		}
	}
	// A missing regional set inherits its language's rules independently.
	// The root supplies the unconditional category for an unlisted language.
	if len(result.Cardinal) == 0 {
		result.Cardinal = []plurdata.Rule{{Category: "other"}}
	}
	if len(result.Ordinal) == 0 {
		result.Ordinal = []plurdata.Rule{{Category: "other"}}
	}
	return &result, nil
}

// Select returns the plural form a number calls for.
func (p *PluralRules) Select(v float64) PluralCategory {
	// NaN and the infinities are "other" in every language, as ICU's
	// plural rules answer for a number that is not finite.
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return PluralOther
	}
	if p.written != nil {
		o := p.written.pluralOperands(v)
		return p.selectOperands(&o)
	}
	return p.selectWith(v, 0)
}

// selectWith is Select with a compact or scientific exponent, which is the "c"
// operand. A number written as "1.2M" has a different exponent from the same
// value written out, and a few languages notice.
func (p *PluralRules) selectWith(v float64, exponent int) PluralCategory {
	// The number is rounded with its sign, as a directional rounding mode
	// needs: to the floor, -1.5 is -2, which English calls "other".
	negative := v < 0
	if negative {
		v = -v
	}
	integer, fraction := p.round(magOf(v), negative)
	integer = padInteger(integer, p.minInt)

	o := operandsFor(integer, fraction, exponent)
	return p.selectOperands(&o)
}

// SelectRange returns the plural form a range of numbers calls for, from
// the forms of its ends as they are rounded: in English "1–2 days" is
// "other" whatever "1" alone would be. It follows ICU's
// StandardPluralRanges, which V8 uses for ordinal rules as well as cardinal
// ones. Either end being NaN is an error, as ECMA-402 throws.
func (p *PluralRules) SelectRange(start, end float64) (PluralCategory, error) {
	if math.IsNaN(start) || math.IsNaN(end) {
		return "", fmt.Errorf("intl: a plural range with an end that is not a number")
	}
	return PluralCategory(p.resolveRange(string(p.Select(start)), string(p.Select(end)))), nil
}

// Categories returns the forms this locale distinguishes, in CLDR's order.
func (p *PluralRules) Categories() []PluralCategory {
	out := make([]PluralCategory, 0, len(p.rules))
	for _, r := range p.rules {
		out = append(out, r.category)
	}
	return out
}

// ResolvedPluralRules is what a PluralRules settled on.
type ResolvedPluralRules struct {
	Locale string
	Type   PluralType
	ResolvedDigits
	PluralCategories []PluralCategory
	Notation         Notation
	// CompactDisplay is meaningful only in compact notation, which is the
	// only one ECMA-402 reports it for.
	CompactDisplay CompactDisplay
}

// ResolvedOptions returns what the rules settled on.
func (p *PluralRules) ResolvedOptions() ResolvedPluralRules {
	digits := p.digitPlan.resolved()
	if p.opts.Compat.Has(PluralRulesDigits) && digits.MinimumSignificantDigits != nil {
		// V8 reports the significant digits alone where its skeleton has
		// them (PluralRulesDigits).
		digits.MinimumFractionDigits, digits.MaximumFractionDigits = nil, nil
	}
	return ResolvedPluralRules{
		Locale:           p.locale.String(),
		Type:             p.opts.Type,
		ResolvedDigits:   digits,
		PluralCategories: p.Categories(),
		Notation:         p.opts.Notation,
		CompactDisplay:   p.opts.CompactDisplay,
	}
}
