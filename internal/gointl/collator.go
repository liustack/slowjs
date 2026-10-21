package intl

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/liustack/quickjs/internal/gointl/internal/colldata"
)

// Comparing strings the way a language sorts them.
//
// Comparing two strings is not comparing their characters. A letter counts for
// more than the accent on it, and the accent for more than the case, so two
// strings are compared a whole level at a time: the letters first, the accents
// only if the letters are all the same, the case only if the accents are. That
// is the Unicode Collation Algorithm, and what a sensitivity selects is how
// many of those levels count.
//
// What each character weighs comes from ICU's collation tables: the root order
// for every character, and each language's changes to it. Czech sorts "ch" as
// one letter after "h"; Swedish puts "ä" after "z"; Chinese orders by pinyin or
// by stroke count.

// CollatorUsage says what a collator is for.
type CollatorUsage int

const (
	// UsageSort orders a list.
	UsageSort CollatorUsage = iota
	// UsageSearch finds matches, where the language may count as equal what
	// it would sort apart.
	UsageSearch
)

// Sensitivity says which differences between two strings count.
type Sensitivity int

const (
	// SensitivityDefault takes the locale's, which is SensitivityVariant
	// everywhere.
	SensitivityDefault Sensitivity = iota
	// SensitivityBase counts the letters alone: a = á = A.
	SensitivityBase
	// SensitivityAccent counts the accents too: a = A, a ≠ á.
	SensitivityAccent
	// SensitivityCase counts the case but not the accents: a = á, a ≠ A.
	SensitivityCase
	// SensitivityVariant counts everything: a ≠ á ≠ A.
	SensitivityVariant
)

// CaseFirst says whether upper or lower case sorts first when the case is all
// that differs.
type CaseFirst int

const (
	// CaseFirstDefault takes the -u-kf keyword, or else the locale's.
	CaseFirstDefault CaseFirst = iota
	// CaseFirstUpper sorts "A" before "a".
	CaseFirstUpper
	// CaseFirstLower sorts "a" before "A".
	CaseFirstLower
	// CaseFirstFalse leaves it to the tertiary weights, which put lower case
	// first.
	CaseFirstFalse
)

// CollatorOptions is Intl.Collator's option bag.
type CollatorOptions struct {
	Usage       CollatorUsage
	Sensitivity Sensitivity
	// IgnorePunctuation, when nil, takes the locale's: Thai ignores
	// punctuation, other languages do not.
	IgnorePunctuation *bool
	// Numeric sorts runs of digits by their value, so "2" before "10". When
	// nil it takes the -u-kn keyword.
	Numeric   *bool
	CaseFirst CaseFirst
	// Collation names a collation type by its BCP 47 name -- "phonebk",
	// "pinyin". It overrides -u-co. A type the locale does not have is not an
	// error; the locale's default is used, as ECMA-402 says.
	Collation string

	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A Collator compares strings. It never changes after it is built and is safe
// for any number of goroutines to share.
type Collator struct {
	locale      Locale
	usage       CollatorUsage
	sensitivity Sensitivity
	collation   string

	root       *colldata.Root
	tailoring  *colldata.Data
	reordering *colldata.Reordering
	diacritics []uint16
	normalizer *Normalizer

	strength           int
	caseLevel          bool
	caseFirst          CaseFirst
	numeric            bool
	shifted            bool
	backwardSecondary  bool
	lithuanianDotAbove bool
	variableTop        uint32
	// skipPrefix is the IdenticalPrefix divergence.
	skipPrefix bool
}

// The strengths, as ICU numbers the levels.
const (
	strengthPrimary = iota
	strengthSecondary
	strengthTertiary
	strengthQuaternary
)

// NewCollator builds a collator from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewCollator(loc Locale, opts CollatorOptions) (*Collator, error) {
	return NewCollatorFrom(Embedded, loc, opts)
}

// NewCollatorFrom builds a collator from a source of the caller's own.
func NewCollatorFrom(src Source, loc Locale, opts CollatorOptions) (*Collator, error) {
	c := &Collator{usage: opts.Usage, skipPrefix: opts.Compat.Has(IdenticalPrefix)}

	// The collation type, as ECMA-402's ResolveLocale chooses it: the option
	// if the locale has that type, else the -u-co keyword if it has that,
	// else the locale's default. "standard" and "search" are not types a
	// caller may name; search is chosen by usage.
	option := namedCollation(opts.Collation)
	keyword, _ := loc.keywordValue("co")
	keyword = namedCollation(keyword)

	b, err := src.Open(MarkerCollationRoot, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the root collation: %w", err)
	}
	if c.root, err = colldata.DecodeRoot(b); err != nil {
		return nil, fmt.Errorf("intl: the root collation: %w", err)
	}
	chain, err := collationChain(src, loc.Data())
	if err != nil {
		return nil, err
	}
	keywordFound := keyword != "" && chain.find(collationTypeICU(keyword)) != nil
	var coll *colldata.Collation
	var name string
	switch {
	case opts.Usage == UsageSearch:
		coll, name = chain.find("search"), "search"
	case option != "" && chain.find(collationTypeICU(option)) != nil:
		name = collationTypeICU(option)
		coll = chain.find(name)
	case keywordFound:
		name = collationTypeICU(keyword)
		coll = chain.find(name)
	}
	if coll == nil {
		if coll, name = chain.fallback(); coll == nil {
			return nil, fmt.Errorf("intl: no collation for %s: %w", loc, ErrNotFound)
		}
	}
	c.tailoring = coll.Data
	c.reordering = coll.Reordering
	c.diacritics = c.root.Diacritics
	if coll.Meta&colldata.TailoredDiacriticsBit != 0 && coll.Diacritics != nil {
		c.diacritics = coll.Diacritics
	}
	if c.normalizer, err = NewNormalizerFrom(src); err != nil {
		return nil, err
	}

	// A keyword stays in the resolved locale only when it was honoured.
	keep := map[string]string{}
	c.collation = "default"
	switch {
	case name == "search":
		// Search has no name of its own to report, but a keyword the
		// locale supports is still a supported keyword, and Node keeps it.
		if keywordFound {
			keep["co"] = keyword
		}
	case name != "standard":
		c.collation = collationTypeBCP47(name)
		switch {
		case keyword == c.collation:
			keep["co"] = keyword
		case opts.Compat.Has(CollationKeyword) && option == c.collation && !keywordFound:
			// Node writes a collation the options chose into the locale
			// too, unless the locale had a keyword of its own it honoured.
			keep["co"] = option
		}
	}

	meta := coll.Meta
	c.shifted = meta&colldata.AlternateShiftedBit != 0
	c.backwardSecondary = meta&colldata.BackwardSecondLevelBit != 0
	c.lithuanianDotAbove = meta&colldata.LithuanianDotAboveBit != 0
	switch {
	case meta&colldata.CaseFirstBit == 0:
		c.caseFirst = CaseFirstFalse
	case meta&colldata.UpperFirstBit != 0:
		c.caseFirst = CaseFirstUpper
	default:
		c.caseFirst = CaseFirstLower
	}
	maxVariable := int(meta & colldata.MaxVariableMask)

	if kn, ok := loc.keywordValue("kn"); ok && (kn == "" || kn == "true" || kn == "false") {
		c.numeric = kn != "false"
		if opts.Numeric == nil || *opts.Numeric == c.numeric {
			// "-u-kn-true" is written "-u-kn", as UTS #35 canonicalizes it.
			if c.numeric {
				kn = ""
			}
			keep["kn"] = kn
		}
	}
	if opts.Numeric != nil {
		c.numeric = *opts.Numeric
	}
	if kf, ok := loc.keywordValue("kf"); ok {
		var want CaseFirst
		switch kf {
		case "upper":
			want = CaseFirstUpper
		case "lower":
			want = CaseFirstLower
		case "false":
			want = CaseFirstFalse
		}
		if want != CaseFirstDefault {
			c.caseFirst = want
			if opts.CaseFirst == CaseFirstDefault || opts.CaseFirst == want {
				keep["kf"] = kf
			}
		}
	}
	if opts.CaseFirst != CaseFirstDefault {
		c.caseFirst = opts.CaseFirst
	}
	if opts.IgnorePunctuation != nil {
		c.shifted = *opts.IgnorePunctuation
	}

	c.sensitivity = opts.Sensitivity
	if c.sensitivity == SensitivityDefault {
		c.sensitivity = SensitivityVariant
	}
	switch c.sensitivity {
	case SensitivityBase:
		c.strength = strengthPrimary
	case SensitivityAccent:
		c.strength = strengthSecondary
	case SensitivityCase:
		c.strength = strengthPrimary
		c.caseLevel = true
	default:
		c.strength = strengthTertiary
	}

	if c.shifted {
		// ICU keeps the high half of the variable group's last primary; the
		// last primary is one less than that half shifted up, and the top is
		// one past it, so that less-than finds the variable weights.
		c.variableTop = uint32(c.root.LastPrimaries[maxVariable]) << 16
	}

	c.locale = loc.withKeywords(keep)
	return c, nil
}

// A collationSearch is the locales a collation is looked for in, most
// specific first.
type collationSearch []*colldata.Locale

// collationChain reads the collation data along a locale's chain in the
// collation tree.
func collationChain(src Source, d DataLocale) (collationSearch, error) {
	// ICU's index of the collation tree, where there is one, also knows
	// where ICU goes for a locale it has no bundle for: "ar-Latn" is the
	// root's, not Arabic's. The collation tree's own pairs are for a source
	// without it.
	var steps []DataLocale
	if fb, err := NewFallbacker(src); err == nil {
		if _, ok := fb.icuTree(treeColl); ok {
			steps = fb.ChainIn(treeColl, d)
		}
	}
	if steps == nil {
		tree, err := loadCollationTree(src)
		if err != nil {
			return nil, err
		}
		steps = tree.chain(d)
	}
	var chain collationSearch
	for _, step := range steps {
		b, err := src.Open(MarkerCollation, step)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		l, err := colldata.DecodeLocale(b)
		if err != nil {
			return nil, fmt.Errorf("intl: the collation data for %s: %w", step, err)
		}
		chain = append(chain, l)
	}
	return chain, nil
}

// find finds a collation type as ICU does: the first locale up the chain that
// defines it wins.
func (chain collationSearch) find(name string) *colldata.Collation {
	for _, l := range chain {
		if c, ok := l.Find(name); ok {
			return c
		}
	}
	return nil
}

// fallback is the collation used when none that was asked for exists: the
// default the nearest locale names, or "standard", which the root defines.
func (chain collationSearch) fallback() (*colldata.Collation, string) {
	def := "standard"
	for _, l := range chain {
		if l.Default != "" {
			def = l.Default
			break
		}
	}
	if c := chain.find(def); c != nil {
		return c, def
	}
	return chain.find("standard"), "standard"
}

// namedCollation returns a collation type a caller may name, or nothing.
func namedCollation(s string) string {
	if s == "standard" || s == "search" {
		return ""
	}
	return s
}

// collationTree is the collation tree with its pairs in maps.
type collationTree struct {
	aliases map[DataLocale]DataLocale
	parents map[DataLocale]DataLocale
}

func loadCollationTree(src Source) (*collationTree, error) {
	b, err := src.Open(MarkerCollationTree, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the collation tree: %w", err)
	}
	t, err := colldata.DecodeTree(b)
	if err != nil {
		return nil, fmt.Errorf("intl: the collation tree: %w", err)
	}
	out := &collationTree{aliases: map[DataLocale]DataLocale{}, parents: map[DataLocale]DataLocale{}}
	for _, pairs := range []struct {
		in  [][2]string
		out map[DataLocale]DataLocale
	}{{t.Aliases, out.aliases}, {t.Parents, out.parents}} {
		for _, p := range pairs.in {
			from, err1 := ParseLocale(p[0])
			to, err2 := ParseLocale(p[1])
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("intl: the collation tree pairs %q with %q", p[0], p[1])
			}
			pairs.out[from.Data()] = to.Data()
		}
	}
	return out, nil
}

// chain is where a locale's collation is looked for: aliases replaced, then
// the tree's parents where it names one and truncation where it does not.
func (t *collationTree) chain(d DataLocale) []DataLocale {
	var chain []DataLocale
	cur := d
	for len(chain) < maxChain {
		for i := 0; i < maxChain; i++ {
			to, ok := t.aliases[cur]
			if !ok {
				break
			}
			cur = to
		}
		chain = append(chain, cur)
		if cur.IsRoot() {
			break
		}
		if p, ok := t.parents[cur]; ok {
			cur = p
		} else {
			cur = truncate(cur)
		}
	}
	return chain
}

// BCP 47 names a few collation types differently from ICU.
var collationNames = [][2]string{
	{"phonebk", "phonebook"},
	{"trad", "traditional"},
	{"dict", "dictionary"},
	{"gb2312", "gb2312han"},
}

func collationTypeICU(bcp string) string {
	for _, n := range collationNames {
		if n[0] == bcp {
			return n[1]
		}
	}
	return bcp
}

func collationTypeBCP47(icu string) string {
	for _, n := range collationNames {
		if n[1] == icu {
			return n[0]
		}
	}
	return icu
}

// keywordValue returns a Unicode extension keyword, reporting whether it was
// there at all: "-u-kn" is there with an empty value, which means true.
func (l Locale) keywordValue(key string) (string, bool) {
	for _, k := range l.Keywords {
		if k.Key == key {
			return k.Value, true
		}
	}
	return "", false
}

// withKeywords returns the locale with its Unicode extension replaced by the
// keywords given, in key order as a canonical tag has them.
//
// "-u-va-posix" stays whatever is kept: ICU reads it as the locale's
// variant POSIX, not as a keyword, so V8's ResolveLocale, which rebuilds the
// extension from the keywords a service uses, never drops it, and writes it
// back when it writes the locale.
func (l Locale) withKeywords(keep map[string]string) Locale {
	out := l
	out.Attributes = nil
	out.Keywords = nil
	if v, ok := l.Keyword("va"); ok && v == "posix" {
		if _, set := keep["va"]; !set {
			withVariant := map[string]string{"va": v}
			for key, value := range keep {
				withVariant[key] = value
			}
			keep = withVariant
		}
	}
	for key, value := range keep {
		out.Keywords = append(out.Keywords, Keyword{Key: key, Value: value})
	}
	sort.Slice(out.Keywords, func(i, j int) bool { return out.Keywords[i].Key < out.Keywords[j].Key })
	return out
}

// ResolvedCollator is what a collator settled on.
type ResolvedCollator struct {
	Locale            string
	Usage             CollatorUsage
	Sensitivity       Sensitivity
	IgnorePunctuation bool
	// Collation is the type's BCP 47 name, or "default".
	Collation string
	Numeric   bool
	CaseFirst CaseFirst
}

// ResolvedOptions returns what the collator settled on.
func (c *Collator) ResolvedOptions() ResolvedCollator {
	return ResolvedCollator{
		Locale:            c.locale.String(),
		Usage:             c.usage,
		Sensitivity:       c.sensitivity,
		IgnorePunctuation: c.shifted,
		Collation:         c.collation,
		Numeric:           c.numeric,
		CaseFirst:         c.caseFirst,
	}
}

// Compare returns -1, 0 or 1 as a sorts before, with or after b.
func (c *Collator) Compare(a, b string) int {
	if a == b {
		return 0
	}
	if c.skipPrefix {
		start := c.comparisonStart(a, b)
		return c.compareElements(c.sortElementsFrom(a, start), c.sortElementsFrom(b, start))
	}
	return c.compareElements(c.sortElements(a), c.sortElements(b))
}

// normalizedText is a string in NFD, as the characters the collator reads.
// A lone surrogate, which a JavaScript string may hold and which comes here
// in WTF-8, stays itself -- the normalizer would make each of its bytes a
// U+FFFD -- so that it weighs as ICU weighs it, by its code point.
func (c *Collator) normalizedText(s string) []rune {
	if strings.IndexByte(s, 0xed) < 0 {
		return []rune(c.normalizer.Normalize(s, NFD))
	}
	var out []rune
	start := 0
	for i := 0; i+2 < len(s); i++ {
		if s[i] != 0xed || s[i+1] < 0xa0 || s[i+1] > 0xbf || s[i+2] < 0x80 || s[i+2] > 0xbf {
			continue
		}
		// A surrogate is a starter, so the text either side of it
		// normalizes on its own.
		out = append(out, []rune(c.normalizer.Normalize(s[start:i], NFD))...)
		out = append(out, 0xd000|rune(s[i+1]&0x3f)<<6|rune(s[i+2]&0x3f))
		i += 2
		start = i + 1
	}
	return append(out, []rune(c.normalizer.Normalize(s[start:], NFD))...)
}

// sortElements returns what a string is compared by: its collation elements,
// with the variable ones shifted when punctuation is ignored.
func (c *Collator) sortElements(s string) []uint64 { return c.sortElementsFrom(s, 0) }

// sortElementsFrom is sortElements of the text from a byte offset, which
// starts a character that no combining mark or contraction joins to what
// precedes it, so that the text either side decomposes on its own. What
// precedes it is read only for the prefixes a character's weight depends
// on.
func (c *Collator) sortElementsFrom(s string, start int) []uint64 {
	text := c.normalizedText(s[start:])
	from := 0
	if start > 0 {
		before := c.normalizedText(s[:start])
		from = len(before)
		text = append(before, text...)
	}
	ces := c.elementsFrom(text, from)
	if c.shifted {
		c.shiftVariables(ces)
	}
	return ces
}
