package intl

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
	"github.com/liustack/slowjs/internal/gointl/internal/layout"
)

// Canonicalization: the form ECMA-402's CanonicalizeUnicodeLocaleId gives an
// identifier, as ICU 78.3 gives it, which is what Node answers
// Intl.getCanonicalLocales with.
//
// A tag is first checked against the grammar, strictly: only hyphens, no
// extended language subtags, no variant or singleton given twice. Then the
// aliases CLDR lists are replaced, as ICU's AliasReplacer (locid.cpp)
// replaces them -- deprecated and overlong languages, regions, scripts and
// variants, again and again until nothing changes -- and the Unicode and
// transformed extensions are put in their canonical form: every spelling of
// an extension type becomes its BCP 47 id, "true" is left out, and the
// subdivisions of "sd" and "rg" are replaced like any other alias.
//
// The aliases are data (data/aliases.bin, from ICU's metadata and
// keyTypeData), an index read where it lies: a Canonicalizer looks up the
// aliases a tag has rather than reading them all when it is built.

// CanonicalizeOptions choose how a Canonicalizer answers.
type CanonicalizeOptions struct {
	// Compat chooses between the standard and Node's observable behavior.
	Compat Compat
}

// A Canonicalizer puts locale identifiers into canonical form. It never
// changes after it is built and is safe for any number of goroutines to
// share.
type Canonicalizer struct {
	opts CanonicalizeOptions
	// aliases is keyed by a table and an alias, "language aa_saaho",
	// "type ca islamicc".
	aliases blob.Index
	// legacy and redundant are the whole tags ICU's parser rewrites first,
	// in its order, a line each.
	legacy, redundant string
	likely            *Fallbacker
}

// NewCanonicalizer reads the alias tables from a source.
func NewCanonicalizer(src Source, opts CanonicalizeOptions) (*Canonicalizer, error) {
	b, err := src.Open(MarkerAliases, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the locale aliases: %w", err)
	}
	aliases, err := blob.ReadIndex(b, layout.Aliases)
	if err != nil {
		return nil, fmt.Errorf("intl: the locale aliases: %w", err)
	}
	c := &Canonicalizer{opts: opts, aliases: aliases}
	legacy, ok1 := aliases.Find("legacy")
	redundant, ok2 := aliases.Find("redundant")
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("intl: the locale aliases have no legacy tags")
	}
	c.legacy, c.redundant = string(legacy), string(redundant)
	if c.likely, err = NewFallbacker(src); err != nil {
		return nil, err
	}
	return c, nil
}

// Canonicalize reads a tag and answers with its canonical form. A tag that
// is not a structurally valid Unicode BCP 47 locale identifier is an error,
// ECMA-402's RangeError.
//
// The steps are V8's (ValidateAndCanonicalizeUnicodeLocaleId) and ICU's: the
// language identifier's shape is checked first; then ICU's parser rewrites a
// legacy or redundant tag it starts with, "zh-hakka" to "hak"; then the whole
// is checked, a variant given twice included; then the aliases are replaced.
func (c *Canonicalizer) Canonicalize(tag string) (Locale, error) {
	if err := checkLanguagePrefix(tag); err != nil {
		return Locale{}, err
	}
	if c.opts.Compat.Has(TwoLetterTags) && isTwoLetterFastPath(tag) {
		// V8 answers a lone two-letter language as it came, without the
		// aliases: "bh" stays "bh", where ECMA-402 makes it "bho".
		l, err := ParseLocale(tag)
		return l, err
	}
	// Lowercased as ASCII, so that a letter outside it stays and is
	// refused: Unicode's lowercase of "İ" is an ASCII "i".
	tag = c.rewriteLegacy(lowerASCII(tag))
	if err := checkStructure(tag); err != nil {
		return Locale{}, err
	}
	l, err := ParseLocale(tag)
	if err != nil {
		return Locale{}, err
	}
	if l, err = legacyVariants(l); err != nil {
		return Locale{}, err
	}
	return c.CanonicalizeLocale(l), nil
}

// lowerASCII is a string with its ASCII letters in lowercase, and nothing
// else changed.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		b[i] = toLower(c)
	}
	return string(b)
}

// isTwoLetterFastPath is V8's shortcut: a tag of two lowercase letters,
// other than the deprecated and legacy ones ICU would change.
func isTwoLetterFastPath(tag string) bool {
	if len(tag) != 2 || tag[0] < 'a' || tag[0] > 'z' || tag[1] < 'a' || tag[1] > 'z' {
		return false
	}
	switch tag {
	case "in", "iw", "ji", "jw", "mo", "sh", "tl", "no":
		return false
	}
	return true
}

// rewriteLegacy is ultag_parse's first step: a legacy tag, or failing that a
// redundant one, that the tag starts with is replaced by its preferred form.
func (c *Canonicalizer) rewriteLegacy(tag string) string {
	for _, list := range [...]string{c.legacy, c.redundant} {
		for list != "" {
			var line string
			line, list, _ = strings.Cut(list, "\n")
			from, to, _ := strings.Cut(line, " ")
			if strings.HasPrefix(tag, from) && (len(tag) == len(from) || tag[len(from)] == '-') {
				return to + tag[len(from):]
			}
		}
	}
	return tag
}

// canonicalUValue is ECMA-402's CanonicalizeUValue: a Unicode extension
// keyword's value by its alias, "islamic-civil" for the calendar "islamicc",
// or as it is.
func canonicalUValue(src Source, key, value string) string {
	b, err := src.Open(MarkerAliases, DataLocale{})
	if err != nil {
		return value
	}
	aliases, err := blob.ReadIndex(b, layout.Aliases)
	if err != nil {
		return value
	}
	if to, ok := aliases.Find("type " + key + " " + value); ok {
		return string(to)
	}
	return value
}

// alias looks up an alias in one of the tables: "language", "territory",
// "script", "variant", "subdivision", or "type" with the key before the
// value, "ca islamicc".
func (c *Canonicalizer) alias(table, from string) (string, bool) {
	to, ok := c.aliases.Find(table + " " + from)
	return string(to), ok
}

// legacyVariants is ICU's reading of private use: what follows "lvariant"
// is variants, "-x-lvariant-posix" being "posix".
func legacyVariants(l Locale) (Locale, error) {
	if l.Private != "" {
		subtags := strings.Split(l.Private, "-")
		for i, s := range subtags {
			if s != "lvariant" {
				continue
			}
			if i == len(subtags)-1 {
				return Locale{}, fmt.Errorf("%w: -x-lvariant- names no variant", ErrSyntax)
			}
			for _, v := range subtags[i+1:] {
				parsed, err := ParseVariant(v)
				if err != nil {
					return Locale{}, err
				}
				// ICU's parser refuses a variant given twice, the ones
				// from lvariant among them: "en-fonipa-x-lvariant-fonipa".
				if slices.Contains(l.Variants, parsed) {
					return Locale{}, fmt.Errorf("%w: gives the variant %s twice", ErrSyntax, v)
				}
				l.Variants = append(l.Variants, parsed)
			}
			// What came before "lvariant" goes with it, as ICU's
			// ultag_parse drops it: "en-x-foo-lvariant-abcde" is
			// "en-abcde".
			l.Private = ""
			break
		}
	}
	return l, nil
}

// CanonicalizeLocale puts a parsed locale into canonical form.
func (c *Canonicalizer) CanonicalizeLocale(l Locale) Locale {
	out := l
	out.Variants = append([]Variant(nil), l.Variants...)
	c.replaceAliases(&out)

	// A lone variant "posix" left once the aliases are replaced is written
	// as the keyword "-u-va-posix", as ICU writes it when it makes the tag,
	// after AliasReplacer: "en-arevela-posix" is "en-u-va-posix".
	keywords := l.Keywords
	if len(out.Variants) == 1 && out.Variants[0].String() == "posix" {
		out.Variants = nil
		keywords = append(slices.Clip(keywords), Keyword{Key: "va", Value: "posix"})
	}

	out.Keywords = nil
	seen := map[string]bool{}
	for _, k := range keywords {
		// A key given twice means what it said first.
		if seen[k.Key] {
			continue
		}
		seen[k.Key] = true
		value := k.Value
		if to, ok := c.alias("type", k.Key+" "+value); ok {
			value = to
		}
		if (k.Key == "sd" || k.Key == "rg") && value != "" {
			value = c.replaceSubdivision(value)
		}
		// "true" is left out, and so is "yes" where the key's data makes it
		// "true". ICU takes "yes" as any keyword's value and leaves it out
		// everywhere (YesValues).
		if value == "true" || value == "yes" && c.opts.Compat.Has(YesValues) {
			value = ""
		}
		out.Keywords = append(out.Keywords, Keyword{Key: k.Key, Value: value})
	}
	out.Attributes = nil
	for _, a := range l.Attributes {
		if !slices.Contains(out.Attributes, a) {
			out.Attributes = append(out.Attributes, a)
		}
	}
	out.Extensions = nil
	for _, e := range l.Extensions {
		if e.Singleton == 't' {
			e.Value = c.canonicalTransformed(e.Value)
		}
		out.Extensions = append(out.Extensions, e)
	}
	out.canonicalize()
	return out
}

// base is the language identifier as AliasReplacer works on it: the
// language written out ("und" for none), the script, region and variants.
type base struct {
	language, script, region string
	variants                 []string
}

// replaceAliases is AliasReplacer::replace on the language identifier: the
// language aliases, most specific first, then the region, script and
// variant aliases, again until none applies.
func (c *Canonicalizer) replaceAliases(l *Locale) {
	b := base{language: l.Language.String(), script: l.Script.String(), region: l.Region.String()}
	for _, v := range l.Variants {
		b.variants = append(b.variants, v.String())
	}
	sort.Strings(b.variants)
	for i := 0; i < 16; i++ {
		if !(c.replaceLanguage(&b, true, true, true) ||
			c.replaceLanguage(&b, true, true, false) ||
			c.replaceLanguage(&b, true, false, true) ||
			c.replaceLanguage(&b, true, false, false) ||
			c.replaceLanguage(&b, false, false, true) ||
			c.replaceTerritory(&b) ||
			c.replaceScript(&b) ||
			c.replaceVariant(&b)) {
			break
		}
	}
	// An alias can name a variant the locale already has, "heploc" for
	// "alalc97", and ICU keeps it once: "en-heploc-alalc97" is "en-alalc97".
	slices.Sort(b.variants)
	b.variants = slices.Compact(b.variants)
	l.Language, _ = ParseLanguage(b.language)
	l.Script, _ = ParseScript(b.script)
	l.Region, _ = ParseRegion(b.region)
	l.Variants = l.Variants[:0]
	for _, v := range b.variants {
		if parsed, err := ParseVariant(v); err == nil {
			l.Variants = append(l.Variants, parsed)
		}
	}
}

// replaceLanguage is AliasReplacer::replaceLanguage: the language alias for
// the language, or "und", with the region and a variant as asked, replacing
// what matched and whatever else the alias names.
func (c *Canonicalizer) replaceLanguage(b *base, checkLanguage, checkRegion, checkVariants bool) bool {
	if checkRegion && b.region == "" || checkVariants && len(b.variants) == 0 {
		return false
	}
	count := 1
	if checkVariants {
		count = len(b.variants)
	}
	search := "und"
	if checkLanguage {
		search = b.language
	}
	searchRegion := ""
	if checkRegion {
		searchRegion = b.region
	}
	for vi := 0; vi < count; vi++ {
		searchVariant := ""
		if checkVariants {
			searchVariant = b.variants[vi]
		}
		key := search
		if searchRegion != "" {
			key += "_" + strings.ToUpper(searchRegion)
		}
		if searchVariant != "" {
			key += "_" + searchVariant
		}
		replacement, ok := c.alias("language", key)
		if !ok {
			continue
		}
		lang, script, region, variant, ext := parseLanguageReplacement(replacement)
		if lang == "und" {
			lang = b.language
		}
		script = deleteOrReplace(b.script, false, script)
		region = deleteOrReplace(b.region, searchRegion != "", region)
		variant = deleteOrReplace(searchVariant, searchVariant != "", variant)
		if lang == b.language && script == b.script && region == b.region &&
			variant == searchVariant && !ext {
			continue
		}
		b.language, b.script, b.region = lang, script, region
		if searchVariant != "" {
			if variant != "" {
				b.variants[vi] = strings.ToLower(variant)
			} else {
				b.variants = append(b.variants[:vi], b.variants[vi+1:]...)
			}
		}
		return true
	}
	return false
}

// deleteOrReplace is ICU's: a replacement fills a field the tag leaves
// empty and leaves one it fills; with no replacement, a field that took part
// in the match goes and one that did not stays.
func deleteOrReplace(input string, matched bool, replacement string) string {
	switch {
	case replacement != "" && input == "":
		return replacement
	case replacement != "":
		return input
	case !matched:
		return input
	}
	return ""
}

// parseLanguageReplacement takes a language alias's replacement apart:
// "sr_Latn", "und_AX", "ssy".
func parseLanguageReplacement(r string) (lang, script, region, variant string, ext bool) {
	parts := strings.Split(r, "_")
	lang = parts[0]
	rest := parts[1:]
	if len(rest) > 0 && len(rest[0]) == 4 && isAlpha(rest[0][0]) {
		script, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 && (len(rest[0]) == 2 || len(rest[0]) == 3) {
		region, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 && len(rest[0]) >= 4 {
		variant, rest = rest[0], rest[1:]
	}
	return lang, script, region, variant, len(rest) > 0
}

// replaceTerritory is AliasReplacer::replaceTerritory. A region that split
// into several becomes the one the language most likely means, when it is
// among them, else the first: the Soviet Union is Armenia for Armenian.
func (c *Canonicalizer) replaceTerritory(b *base) bool {
	if b.region == "" {
		return false
	}
	replacement, ok := c.alias("territory", b.region)
	to := strings.Fields(replacement)
	if !ok || len(to) == 0 {
		return false
	}
	region := to[0]
	if len(to) > 1 {
		var d DataLocale
		d.Language, _ = ParseLanguage(b.language)
		d.Script, _ = ParseScript(b.script)
		if full, ok := c.likely.Maximize(d); ok {
			likely := full.Region.String()
			for _, r := range to {
				if r == likely {
					region = r
				}
			}
		}
	}
	if region == b.region {
		return false
	}
	b.region = region
	return true
}

// replaceScript is AliasReplacer::replaceScript.
func (c *Canonicalizer) replaceScript(b *base) bool {
	if to, ok := c.alias("script", b.script); ok && b.script != "" && to != b.script {
		b.script = to
		return true
	}
	return false
}

// replaceVariant is AliasReplacer::replaceVariant, with its special case:
// "heploc" becomes "alalc97" and takes "hepburn" with it.
func (c *Canonicalizer) replaceVariant(b *base) bool {
	for i, v := range b.variants {
		to, ok := c.alias("variant", v)
		if !ok || to == v {
			continue
		}
		b.variants[i] = to
		if v == "heploc" {
			for j := 0; j < len(b.variants); j++ {
				if b.variants[j] == "hepburn" {
					b.variants = append(b.variants[:j], b.variants[j+1:]...)
					j--
				}
			}
		}
		return true
	}
	return false
}

// replaceSubdivision is AliasReplacer::replaceSubdivision: a subdivision's
// alias is the first it names, and a bare region is written as the region's
// "zzzz", its whole.
func (c *Canonicalizer) replaceSubdivision(value string) string {
	to, ok := c.alias("subdivision", value)
	if !ok {
		return value
	}
	first, _, _ := strings.Cut(to, " ")
	if len(first) < 2 || len(first) > 8 {
		return value
	}
	first = strings.ToLower(first)
	if len(first) == 2 {
		first += "zzzz"
	}
	return first
}

// canonicalTransformed is AliasReplacer::replaceTransformedExtensions: the
// source language canonicalized and written in lowercase, then the fields
// sorted by key, each value in its BCP 47 spelling.
func (c *Canonicalizer) canonicalTransformed(value string) string {
	subtags := strings.Split(value, "-")
	at := 0
	for at < len(subtags) && !isTKey(subtags[at]) {
		at++
	}
	var out []string
	if at > 0 {
		if l, err := ParseLocale(strings.Join(subtags[:at], "-")); err == nil {
			tl := Locale{Language: l.Language, Script: l.Script, Region: l.Region, Variants: l.Variants}
			c.replaceAliases(&tl)
			tl.canonicalize()
			out = append(out, strings.ToLower(tl.String()))
		} else {
			out = append(out, strings.Join(subtags[:at], "-"))
		}
	}
	type field struct{ key, value string }
	var fields []field
	for at < len(subtags) {
		key := subtags[at]
		at++
		start := at
		for at < len(subtags) && !isTKey(subtags[at]) {
			at++
		}
		v := strings.Join(subtags[start:at], "-")
		if to, ok := c.alias("type", key+" "+v); ok {
			v = to
		}
		fields = append(fields, field{key, v})
	}
	// By key and then by value, as ICU orders them: a key given twice is
	// written with its values in order, "m0-hwidth-names-m0-names-names".
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].key != fields[j].key {
			return fields[i].key < fields[j].key
		}
		return fields[i].value < fields[j].value
	})
	for _, f := range fields {
		out = append(out, f.key, f.value)
	}
	return strings.Join(out, "-")
}

// isTKey reports whether a subtag is a transformed extension's key: a
// letter then a digit.
func isTKey(s string) bool {
	return len(s) == 2 && isAlpha(s[0]) && isDigit(s[1])
}

// checkStructure is ECMA-402's IsStructurallyValidLanguageTag: the tag is a
// Unicode BCP 47 locale identifier, written with hyphens, with no variant,
// singleton or transformed-language variant given twice.
func checkStructure(tag string) error {
	bad := func(why string) error { return fmt.Errorf("%w: %q %s", ErrSyntax, tag, why) }
	parts := strings.Split(tag, "-")
	for _, p := range parts {
		if p == "" || len(p) > 8 || !alphanumOnly(p) {
			return bad("has an ill-formed subtag")
		}
	}
	at, err := checkLanguageID(parts, 0)
	if err != nil {
		return bad(err.Error())
	}
	if at < len(parts) && len(parts[at]) != 1 {
		return bad("has a subtag out of place")
	}
	seen := map[byte]bool{}
	for at < len(parts) {
		if len(parts[at]) != 1 {
			return bad("has a subtag out of place")
		}
		s := toLower(parts[at][0])
		if seen[s] {
			return bad("gives a singleton twice")
		}
		seen[s] = true
		at++
		start := at
		switch s {
		case 'x':
			if at == len(parts) {
				return bad("has an empty private use extension")
			}
			return nil
		case 'u':
			// Attributes, then keywords: a key of a letter or digit and a
			// letter, and its type of subtags of three to eight.
			for at < len(parts) && len(parts[at]) >= 3 {
				at++
			}
			for at < len(parts) && len(parts[at]) == 2 {
				if !isAlpha(toLower(parts[at][1])) {
					return bad("has an ill-formed key")
				}
				at++
				for at < len(parts) && len(parts[at]) >= 3 {
					at++
				}
			}
		case 't':
			if at < len(parts) && isAlpha(toLower(parts[at][0])) && len(parts[at]) != 1 && !isTKey(strings.ToLower(parts[at])) {
				next, err := checkLanguageID(parts, at)
				if err != nil {
					return bad(err.Error())
				}
				at = next
			}
			for at < len(parts) && isTKey(strings.ToLower(parts[at])) {
				at++
				n := at
				for at < len(parts) && len(parts[at]) >= 3 {
					at++
				}
				if at == n {
					return bad("has a transformed field with no value")
				}
			}
		default:
			for at < len(parts) && len(parts[at]) >= 2 {
				at++
			}
		}
		if at == start {
			return bad("has an empty extension")
		}
	}
	return nil
}

// checkLanguagePrefix is V8's JSLocale::StartsWithUnicodeLanguageId: the tag
// begins with a well-formed language identifier, its variants unchecked for
// repeats.
func checkLanguagePrefix(tag string) error {
	parts := strings.Split(tag, "-")
	bad := fmt.Errorf("%w: %q does not begin with a language identifier", ErrSyntax, tag)
	lang := parts[0]
	if !alphaOnly(lang) || len(lang) < 2 || len(lang) == 4 || len(lang) > 8 {
		return bad
	}
	at := 1
	if at < len(parts) && len(parts[at]) == 1 {
		return nil
	}
	if at < len(parts) && len(parts[at]) == 4 && alphaOnly(parts[at]) {
		at++
	}
	if at < len(parts) && isRegionShaped(parts[at]) {
		at++
	}
	for ; at < len(parts); at++ {
		p := parts[at]
		if len(p) == 1 {
			return nil
		}
		if !((len(p) >= 5 && len(p) <= 8 && alphanumOnly(p)) || len(p) == 4 && isDigit(p[0]) && alphanumOnly(p)) {
			return bad
		}
	}
	return nil
}

// checkLanguageID checks a unicode_language_id starting at a subtag, and
// returns where it ends.
func checkLanguageID(parts []string, at int) (int, error) {
	lang := parts[at]
	if !alphaOnly(lang) || len(lang) < 2 || len(lang) == 4 || len(lang) > 8 {
		return 0, fmt.Errorf("has an ill-formed language")
	}
	at++
	if at < len(parts) && len(parts[at]) == 4 && alphaOnly(parts[at]) {
		at++
	}
	if at < len(parts) && isRegionShaped(parts[at]) {
		at++
	}
	variants := map[string]bool{}
	for at < len(parts) && len(parts[at]) >= 4 {
		v := strings.ToLower(parts[at])
		if len(v) == 4 && !isDigit(v[0]) {
			return 0, fmt.Errorf("has an ill-formed variant")
		}
		if variants[v] {
			return 0, fmt.Errorf("gives a variant twice")
		}
		variants[v] = true
		at++
	}
	return at, nil
}
