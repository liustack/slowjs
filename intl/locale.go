// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"slices"
	"sort"
	"strings"
	"sync"
)

// defaultLocale is the locale a formatter falls back to when none of the
// requested ones is available, or none is requested. It is fixed rather than
// read from the host, so that a server renders the same text wherever it
// runs.
const defaultLocale = "en-US"

// langTag is a parsed Unicode BCP 47 locale identifier.
type langTag struct {
	lang, script, region string
	variants             []string
	keywords             [][2]string // the -u- extension's keywords, key and type
	attributes           []string    // the -u- extension's attributes
	other                []string    // the other extensions and private use, each "x-..."
}

// parseTag parses tag, the shape UTS 35 calls a unicode_locale_id, and
// reports whether it is well formed. Case is normalized; nothing is looked up.
func parseTag(tag string) (*langTag, bool) {
	if tag == "" || strings.ContainsAny(tag, "_ ") {
		return nil, false
	}

	parts := strings.Split(strings.ToLower(tag), "-")
	for _, p := range parts {
		if p == "" || len(p) > 8 || !isAlnum(p) {
			return nil, false
		}
	}
	t := &langTag{}
	i := 0
	switch p := parts[0]; {
	case isAlpha(p) && (len(p) >= 2 && len(p) <= 3 || len(p) >= 5):
		t.lang = p
		i++
	default:
		return nil, false
	}
	if i < len(parts) && len(parts[i]) == 4 && isAlpha(parts[i]) {
		t.script = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		i++
	}
	if i < len(parts) && (len(parts[i]) == 2 && isAlpha(parts[i]) || len(parts[i]) == 3 && isDigit(parts[i])) {
		t.region = strings.ToUpper(parts[i])
		i++
	}
	for i < len(parts) && (len(parts[i]) >= 5 || len(parts[i]) == 4 && isDigit(parts[i][:1])) {
		if slices.Contains(t.variants, parts[i]) {
			return nil, false
		}

		t.variants = append(t.variants, parts[i])
		i++
	}
	seen := map[string]bool{}
	for i < len(parts) {
		singleton := parts[i]
		if len(singleton) != 1 || seen[singleton] && singleton != "x" {
			return nil, false
		}

		seen[singleton] = true
		i++
		start := i
		if singleton == "x" {
			if i == len(parts) {
				return nil, false
			}

			t.other = append(t.other, "x-"+strings.Join(parts[i:], "-"))
			return t, true
		}

		for i < len(parts) && len(parts[i]) > 1 {
			i++
		}
		if i == start {
			return nil, false
		}

		sub := parts[start:i]
		if singleton != "u" {
			t.other = append(t.other, singleton+"-"+strings.Join(sub, "-"))
			continue
		}

		j := 0
		for ; j < len(sub) && len(sub[j]) >= 3; j++ {
			t.attributes = append(t.attributes, sub[j])
		}
		for j < len(sub) {
			key := sub[j]
			if len(key) != 2 || !isAlpha(key[1:]) {
				return nil, false
			}

			j++
			var typ []string
			for ; j < len(sub) && len(sub[j]) >= 3; j++ {
				typ = append(typ, sub[j])
			}
			if !slices.ContainsFunc(t.keywords, func(kw [2]string) bool { return kw[0] == key }) {
				// Only the first occurrence of a key counts.
				t.keywords = append(t.keywords, [2]string{key, strings.Join(typ, "-")})
			}
		}
	}
	return t, true
}

// canonicalize applies the CLDR aliases and orders the variants, keywords
// and extensions the way UTS 35 asks for.
func (t *langTag) canonicalize() {
	s := supp()
	if r, ok := s.languageAlias[t.lang]; ok {
		l, sc, rg := splitLang(r)
		t.lang = l
		if t.script == "" {
			t.script = sc
		}
		if t.region == "" {
			t.region = rg
		}
	}
	if r, ok := s.scriptAlias[t.script]; ok {
		t.script = r
	}
	if r, ok := s.territoryAlias[t.region]; ok {
		if f := strings.Fields(r); len(f) > 1 {
			// A region that split up becomes the part where the language
			// is most likely spoken, or else the first one.
			_, _, likely := s.maximize(t.lang, t.script, "")
			r = f[0]
			if slices.Contains(f, likely) {
				r = likely
			}
		}
		t.region = r
	}
	sort.Strings(t.variants)
	sort.Strings(t.attributes)
	sort.SliceStable(t.keywords, func(i, j int) bool { return t.keywords[i][0] < t.keywords[j][0] })
	for i, kw := range t.keywords {
		if kw[1] == "true" {
			t.keywords[i][1] = ""
		}
	}
	sort.SliceStable(t.other, func(i, j int) bool {
		// Private use always comes last.
		if (t.other[i][0] == 'x') != (t.other[j][0] == 'x') {
			return t.other[j][0] == 'x'
		}

		return t.other[i][0] < t.other[j][0]
	})
}

// base returns the tag without its extensions.
func (t *langTag) base() string {
	var b strings.Builder
	b.WriteString(t.lang)
	for _, s := range []string{t.script, t.region} {
		if s != "" {
			b.WriteByte('-')
			b.WriteString(s)
		}
	}
	for _, v := range t.variants {
		b.WriteByte('-')
		b.WriteString(v)
	}
	return b.String()
}

func (t *langTag) String() string {
	var b strings.Builder
	b.WriteString(t.base())
	// Extensions are ordered by their singleton, -u- among them, and private
	// use comes last; canonicalize sorted t.other that way.
	wroteU := false
	for _, o := range t.other {
		if !wroteU && o[0] > 'u' {
			writeUnicodeExt(&b, t.attributes, t.keywords)
			wroteU = true
		}
		b.WriteByte('-')
		b.WriteString(o)
	}
	if !wroteU {
		writeUnicodeExt(&b, t.attributes, t.keywords)
	}
	return b.String()
}

func writeUnicodeExt(b *strings.Builder, attributes []string, keywords [][2]string) {
	if len(attributes) == 0 && len(keywords) == 0 {
		return
	}

	b.WriteString("-u")
	for _, a := range attributes {
		b.WriteByte('-')
		b.WriteString(a)
	}
	for _, kw := range keywords {
		b.WriteByte('-')
		b.WriteString(kw[0])
		if kw[1] != "" {
			b.WriteByte('-')
			b.WriteString(kw[1])
		}
	}
}

func (t *langTag) keyword(key string) (string, bool) {
	for _, kw := range t.keywords {
		if kw[0] == key {
			return kw[1], true
		}
	}
	return "", false
}

// canonicalLocale returns tag canonicalized, or a RangeError if it is not
// well formed.
func canonicalLocale(tag string) (string, error) {
	t, ok := parseTag(tag)
	if !ok {
		return "", rangeError("Incorrect locale information provided")
	}

	t.canonicalize()
	return t.String(), nil
}

// splitLang splits a language, language-script or language-region alias
// replacement.
func splitLang(s string) (lang, script, region string) {
	f := strings.Split(s, "-")
	lang = f[0]
	for _, p := range f[1:] {
		switch len(p) {
		case 4:
			script = p
		default:
			region = p
		}
	}
	return lang, script, region
}

// service is a constructor's set of available locales.
type service int

const (
	numberService service = iota
	dateService
	pluralService
)

// supplementalData is the locale independent part of the CLDR data, in the
// shapes the code wants.
type supplementalData struct {
	available      [3]map[string]bool
	likely         map[string]string
	parents        map[string]string
	languageAlias  map[string]string
	scriptAlias    map[string]string
	territoryAlias map[string]string
	digits         map[string][]string // numbering system -> its ten digits
}

var supp = sync.OnceValue(func() *supplementalData {
	d := cldr().supplemental()
	s := &supplementalData{
		likely:         map[string]string{},
		parents:        map[string]string{},
		languageAlias:  map[string]string{},
		scriptAlias:    map[string]string{},
		territoryAlias: map[string]string{},
		digits:         map[string][]string{},
	}
	for i, k := range []string{"av/nf", "av/dtf", "av/pr"} {
		s.available[i] = map[string]bool{}
		for _, l := range strings.Fields(d.str(k)) {
			s.available[i][l] = true
		}
	}
	for _, m := range []struct {
		prefix string
		m      map[string]string
	}{
		{"ls/", s.likely},
		{"pl/", s.parents},
		{"la/", s.languageAlias},
		{"sa/", s.scriptAlias},
		{"ta/", s.territoryAlias},
	} {
		d.each(m.prefix, func(k, v string) { m.m[k[len(m.prefix):]] = v })
	}
	// Only the simple language aliases apply: a key of one subtag.
	for k := range s.languageAlias {
		if strings.Contains(k, "-") {
			delete(s.languageAlias, k)
		}
	}
	d.each("ns/", func(k, v string) { s.digits[k[3:]] = strings.Split(v, "") })
	return s
})

// maximize is CLDR's add likely subtags.
func (s *supplementalData) maximize(lang, script, region string) (string, string, string) {
	if lang == "" {
		lang = "und"
	}
	for _, k := range [...]string{
		lang + "-" + script + "-" + region, lang + "-" + region, lang + "-" + script, lang,
		"und-" + script + "-" + region, "und-" + region, "und-" + script,
	} {
		if strings.Contains(k, "--") || strings.HasSuffix(k, "-") {
			continue
		}

		if m, ok := s.likely[k]; ok {
			ml, ms, mr := splitLang(m)
			if lang == "und" {
				lang = ml
			}
			if script == "" {
				script = ms
			}
			if region == "" {
				region = mr
			}
			break
		}
	}
	return lang, script, region
}

// resolvedLocale is the outcome of ECMA-402's ResolveLocale.
type resolvedLocale struct {
	tag      *langTag // the available locale chosen, with the requested -u- keywords
	locale   string   // tag without extensions
	region   string   // the region, explicit or likely
	data     *cldrData
	keywords map[string]string // the requested -u- keywords
}

// resolveLocale picks the first requested locale that is available to svc,
// truncating each from the end as ECMA-402's lookup does, and falls back to
// defaultLocale.
func resolveLocale(tags []string, svc service) (*resolvedLocale, error) {
	s := supp()
	for _, tag := range tags {
		t, ok := parseTag(tag)
		if !ok {
			return nil, rangeError("Incorrect locale information provided")
		}

		t.canonicalize()
		if l := bestAvailable(s.available[svc], t.base()); l != "" {
			return newResolvedLocale(l, t), nil
		}
	}
	t, _ := parseTag(defaultLocale)
	return newResolvedLocale(defaultLocale, t), nil
}

// bestAvailable is ECMA-402's BestAvailableLocale.
func bestAvailable(available map[string]bool, l string) string {
	for {
		if available[l] {
			return l
		}

		i := strings.LastIndexByte(l, '-')
		if i < 0 {
			return ""
		}

		if i >= 2 && l[i-2] == '-' {
			i -= 2
		}
		l = l[:i]
	}
}

func newResolvedLocale(l string, requested *langTag) *resolvedLocale {
	t, _ := parseTag(l)
	r := &resolvedLocale{tag: t, locale: l, keywords: map[string]string{}}
	for _, kw := range requested.keywords {
		r.keywords[kw[0]] = kw[1]
	}
	s := supp()
	_, _, r.region = s.maximize(t.lang, t.script, t.region)
	r.data = dataLocale(t)
	return r
}

// dataLocale returns the CLDR data a locale formats with: that of the first
// of language-script-region, language-region, language-script and language
// that has any, after filling in the likely subtags. zh-SG uses zh-Hans-SG,
// zh-TW zh-Hant, pt-BR pt.
func dataLocale(t *langTag) *cldrData {
	x := cldr()
	if d := x.locale(t.base()); d != nil {
		return d
	}

	lang, script, region := supp().maximize(t.lang, t.script, t.region)
	for _, c := range [...]string{lang + "-" + script + "-" + region, lang + "-" + region, lang + "-" + script, lang} {
		if d := x.locale(c); d != nil {
			return d
		}
	}
	return x.locale("en")
}

// unicodeKeyword resolves one relevant -u- keyword against its option, as
// ResolveLocale does: the option wins, and the keyword stays in the reported
// locale only if it was requested, is supported and the option agrees with
// it. supported reports whether a value can be used.
func (r *resolvedLocale) unicodeKeyword(key, option string, supported func(string) bool) (value string, inLocale bool) {
	if v, ok := r.keywords[key]; ok && v != "" && supported(v) {
		value, inLocale = v, true
	}
	if option != "" && supported(option) && option != value {
		value, inLocale = option, false
	}
	return value, inLocale
}

// localeString returns the resolved locale with the keywords that made it
// into it.
func (r *resolvedLocale) localeString(keywords [][2]string) string {
	if len(keywords) == 0 {
		return r.locale
	}

	sort.Slice(keywords, func(i, j int) bool { return keywords[i][0] < keywords[j][0] })
	var b strings.Builder
	b.WriteString(r.locale)
	writeUnicodeExt(&b, nil, keywords)
	return b.String()
}

// supportedLocales implements SupportedLocales for svc.
func supportedLocales(tags []string, svc service) ([]string, error) {
	r := []string{}
	for _, tag := range tags {
		t, ok := parseTag(tag)
		if !ok {
			return nil, rangeError("Incorrect locale information provided")
		}

		t.canonicalize()
		s := t.String()
		if slices.Contains(r, s) {
			continue
		}

		if bestAvailable(supp().available[svc], t.base()) != "" {
			r = append(r, s)
		}
	}
	return r, nil
}
