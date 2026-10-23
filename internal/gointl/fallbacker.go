package intl

import (
	"fmt"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
	"github.com/liustack/slowjs/internal/gointl/internal/layout"
)

// The fallback chain, once CLDR has a say in it.
//
// DataLocale.Fallback walks the identifier alone, which is right for the
// common shapes and wrong where CLDR says so. A Fallbacker carries the two
// tables that say so:
//
//   - The parent locales redirect a chain that truncation would send somewhere
//     wrong. "zh-Hant" truncates to "zh", but traditional Chinese inheriting
//     from simplified is worse than inheriting from the root, so CLDR gives it
//     the root as its parent instead. "en-AU" goes to "en-001" rather than
//     straight to "en", so it keeps the spellings the world outside America
//     shares.
//   - The likely subtags fill in what an identifier left out, so that "zh-TW"
//     and "zh-Hant-TW" are looked for in one place rather than two.
//
// The two compose: maximize first if the identifier is partial, then chain.
//
// ICU's own index says where ICU looks instead, which is what Node answers
// with. For each tree of ICU's data it lists the bundles, the aliases among
// them and the parents they name, with the default scripts and parent
// locales ICU's fallback reads for a bundle that does not exist. ChainIn
// resolves a locale in it as ures_open does: an alias bundle is the bundle it
// names, "sr-ME" Serbian in Latin; a locale without a bundle drops a default
// script, "zh-TW" being "zh-Hant-TW", or falls to the root, as "az-Arab",
// which ICU has no data for, does. The trees differ: "sr-Cyrl-ME" has its own
// dates, but Latin units.

// A Fallbacker answers where to look for data. It is read-only once built and
// safe for concurrent use.
type Fallbacker struct {
	parents pairTable
	likely  pairTable
	src     Source
}

// icuTree is ICU's index of one of its trees, read when a locale is
// resolved in it and looked up in where it lies: the tree's bundles,
// aliases and parents, and, only if a bundle is missing, the tables ICU's
// fallback then reads. availgen writes both as indexes (blob.Index).
type icuTree struct {
	src      Source
	index    blob.Index
	fallback *blob.Index // data/icufallback.bin, read when first needed
}

func (x *icuTree) has(name string) bool {
	_, ok := x.index.Find("bundle " + name)
	return ok
}

// value looks up "alias <name>" or "parent <name>".
func (x *icuTree) value(key string) (string, bool) {
	v, ok := x.index.Find(key)
	return string(v), ok
}

// fallbackValue looks up "defaultscript <name>" or "icuparent <name>".
func (x *icuTree) fallbackValue(key string) (string, bool) {
	if x.fallback == nil {
		b, err := x.src.Open(MarkerICUFallback, DataLocale{})
		if err != nil {
			return "", false
		}
		index, err := blob.ReadIndex(b, layout.ICUFallback)
		if err != nil {
			return "", false
		}
		x.fallback = &index
	}
	v, ok := x.fallback.Find(key)
	return string(v), ok
}

// The trees of ICU's data a service's data comes from, for ChainIn.
const (
	treeLocales = "locales"
	treeUnit    = "unit"
	treeCurr    = "curr"
	treeLang    = "lang"
	treeRegion  = "region"
	treeZone    = "zone"
	treeColl    = "coll"
	treeBrkitr  = "brkitr"
)

// NewFallbacker reads the tables it needs from a source.
func NewFallbacker(src Source) (*Fallbacker, error) {
	parents, err := loadPairs(src, MarkerParentLocales)
	if err != nil {
		return nil, err
	}
	likely, err := loadPairs(src, MarkerLikelySubtags)
	if err != nil {
		return nil, err
	}
	return &Fallbacker{parents: parents, likely: likely, src: src}, nil
}

// icuTree reads ICU's index of a tree, or reports that the source has none.
func (f *Fallbacker) icuTree(tree string) (*icuTree, bool) {
	b, err := f.src.Open(Marker("icutree-"+tree), DataLocale{})
	if err != nil {
		return nil, false
	}
	index, err := blob.ReadIndex(b, layout.ICUTree)
	if err != nil {
		return nil, false
	}
	return &icuTree{src: f.src, index: index}, true
}

// ChainIn is Chain for data read from one of ICU's trees: the bundles ICU
// reads for the locale in that tree, most specific first, as data locales,
// the root last. The index is optional, so that a source carrying only the
// likely subtags and parent locales still works, by CLDR's chain.
func (f *Fallbacker) ChainIn(tree string, d DataLocale) []DataLocale {
	x, ok := f.icuTree(tree)
	if !ok {
		return f.Chain(d)
	}
	var chain []DataLocale
	for _, name := range x.resolve(icuName(d)) {
		if name == "root" {
			break
		}
		l, err := ParseLocale(strings.ReplaceAll(name, "_", "-"))
		if err != nil || len(l.Variants) > 0 && l.Data().Variant.IsZero() {
			// A bundle with a variant a data locale does not hold.
			continue
		}
		chain = append(chain, l.Data())
	}
	return append(chain, DataLocale{})
}

// icuName is a data locale as ICU names it: "sr_Latn_ME", "root" for the
// root.
func icuName(d DataLocale) string {
	if d.IsRoot() {
		return "root"
	}
	name := d.Language.String()
	if !d.Script.IsZero() {
		name += "_" + d.Script.String()
	}
	if !d.Region.IsZero() {
		name += "_" + d.Region.String()
	}
	if !d.Variant.IsZero() {
		if d.Region.IsZero() {
			name += "_"
		}
		name += "_" + strings.ToUpper(d.Variant.String())
	}
	return name
}

// resolve is what ures_open reads for a locale in a tree, as
// icusrc.Locales.Resolve reads it at generation: the first bundle that
// exists, found by ICU's fallback, then along its aliases and its parents or
// its truncations to the root.
func (x *icuTree) resolve(name string) []string {
	orig := name
	for i := 0; !x.has(name) && i < maxChain; i++ {
		next, ok := x.missingParent(name, orig)
		if !ok {
			name = "root"
			break
		}
		name = next
	}
	var out []string
	for i := 0; name != "" && i < maxChain; i++ {
		if to, ok := x.value("alias " + name); ok {
			name = to
			continue
		}
		out = append(out, name)
		if name == "root" {
			break
		}
		next, ok := x.value("parent " + name)
		if !ok {
			if cut := strings.LastIndexByte(name, '_'); cut > 0 {
				next = name[:cut]
			} else {
				next = "root"
			}
		}
		name = next
	}
	return out
}

// missingParent is ICU's getParentLocaleID for a bundle that does not exist:
// the parent locale if CLDR names one, else the name without a region or
// script, where the script is the default ICU would assume.
func (x *icuTree) missingParent(name, orig string) (string, bool) {
	language, script, region, variant := splitICUName(name)
	if variant {
		if cut := strings.LastIndexByte(name, '_'); cut > 0 {
			return name[:cut], true
		}
		return "", false
	}
	if p, ok := x.fallbackValue("icuparent " + name); ok {
		return p, true
	}
	defaultScript := func(region string) string {
		if region != "" {
			if s, ok := x.fallbackValue("defaultscript " + language + "_" + region); ok {
				return s
			}
		}
		if s, ok := x.fallbackValue("defaultscript " + language); ok {
			return s
		}
		return "Latn"
	}
	switch {
	case script != "" && region != "":
		if defaultScript(region) == script {
			return language + "_" + region, true
		}
		return language + "_" + script, true
	case region != "":
		if _, origScript, _, _ := splitICUName(orig); origScript != "" {
			return language + "_" + origScript, true
		}
		return language + "_" + defaultScript(region), true
	case script != "":
		if defaultScript("") == script {
			return language, true
		}
	}
	return "", false
}

// splitICUName takes an ICU locale name apart.
func splitICUName(name string) (language, script, region string, variant bool) {
	parts := strings.Split(name, "_")
	language = parts[0]
	for _, p := range parts[1:] {
		switch {
		case script == "" && region == "" && len(p) == 4:
			script = p
		case region == "" && (len(p) == 2 || len(p) == 3 && p[0] >= '0' && p[0] <= '9'):
			region = p
		default:
			variant = true
		}
	}
	return language, script, region, variant
}

func loadPairs(src Source, m Marker) (pairTable, error) {
	b, err := src.Open(m, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", m, err)
	}
	t, err := newPairTable(b)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", m, err)
	}
	return t, nil
}

// maxChain is a stop for a parent table that pointed in a circle. CLDR's does
// not, but a table is data and data can be wrong, and looping forever is a
// worse way to find out than returning a chain that is merely too short.
const maxChain = 16

// Chain returns the data locales to try, most specific first and the root
// last, following CLDR's parents where it has one and truncating where it does
// not.
func (f *Fallbacker) Chain(d DataLocale) []DataLocale {
	chain := []DataLocale{d}
	seen := map[DataLocale]bool{d: true}
	for cur := d; !cur.IsRoot() && len(chain) < maxChain; {
		// The parent table knows no variants: a variant goes first.
		next, ok := DataLocale{}, false
		if cur.Variant.IsZero() {
			next, ok = f.parents.lookup(cur)
		}
		if !ok {
			next = truncate(cur)
		}
		if seen[next] {
			break
		}
		seen[next] = true
		chain = append(chain, next)
		cur = next
	}
	// A parent table could in principle stop somewhere other than the root.
	// The root always has an answer, so the chain always reaches it.
	if last := chain[len(chain)-1]; !last.IsRoot() {
		chain = append(chain, DataLocale{})
	}
	return chain
}

// truncate drops the most specific part that is still there.
func truncate(d DataLocale) DataLocale {
	switch {
	case !d.Variant.IsZero():
		d.Variant = Variant{}
	case !d.Region.IsZero():
		d.Region = Region{}
	case !d.Script.IsZero():
		d.Script = Script{}
	default:
		d.Language = Language{}
	}
	return d
}

// Maximize fills in what an identifier left out, by UTS #35's likely-subtags
// lookup: the most specific key first, then progressively less of it. What the
// identifier did say is kept, so maximizing never overrules the caller. A
// language the data does not know is not maximized at all, as ICU's
// LikelySubtags leaves it: "xyz-DE" is itself, not "xyz-Latn-DE" by way of
// "und"; only "und" itself reaches the root's entry.
func (f *Fallbacker) Maximize(d DataLocale) (DataLocale, bool) {
	keys := [...]DataLocale{
		d,
		{Language: d.Language, Region: d.Region},
		{Language: d.Language, Script: d.Script},
		{Language: d.Language},
	}
	if d.Language.IsUnd() {
		// ICU's likely-subtags trie takes the script before the region
		// where the language is unknown, as ICU4X does: "und-Cyrl-CN" is
		// Russian in China, not Chinese in Cyrillic.
		keys[1], keys[2] = keys[2], keys[1]
	}
	for _, key := range keys {
		got, ok := f.likely.lookup(key)
		if !ok {
			continue
		}
		if !d.Language.IsUnd() {
			got.Language = d.Language
		}
		if !d.Script.IsZero() {
			got.Script = d.Script
		}
		if !d.Region.IsZero() {
			got.Region = d.Region
		}
		return got, true
	}
	return d, false
}

// Minimize removes what maximizing would put back, which is how a locale is
// written in its shortest unambiguous form: "zh-Hans-CN" is just "zh".
func (f *Fallbacker) Minimize(d DataLocale) (DataLocale, bool) {
	full, ok := f.Maximize(d)
	if !ok {
		return d, false
	}
	// The shortest form whose maximization is the same locale wins, the
	// region before the script, as ICU's minimizeSubtags tries them. The
	// forms are the maximized locale's: "zh-Hant" is "zh-TW".
	for _, try := range [...]DataLocale{
		{Language: full.Language},
		{Language: full.Language, Region: full.Region},
		{Language: full.Language, Script: full.Script},
	} {
		if got, ok := f.Maximize(try); ok && got == full {
			return try, true
		}
	}
	return full, true
}
