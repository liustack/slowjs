//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Each case is one comparison with Node's answer, chosen for a behavior that
// was wrong once or is easy to get wrong.
func TestCollatorCompare(t *testing.T) {
	tests := []struct {
		name   string
		locale string
		opts   intl.CollatorOptions
		a, b   string
		want   int
	}{
		// The export keeps the combining diacritics out of the trie, in a
		// table of their own; missing it gave U+0308 an unassigned weight.
		{"diacritic table", "en", intl.CollatorOptions{}, "A\u0308hre", "apple", -1},
		// Likewise the conjoining jamo.
		{"jamo table", "en", intl.CollatorOptions{}, "\u1100", "\u3131", -1},
		{"jamo table, base", "en", intl.CollatorOptions{Sensitivity: intl.SensitivityBase}, "\u1100", "\u3131", 0},
		// Search makes a trailing consonant weigh as the leading one, and a
		// double consonant as the pair; the export loses both.
		{"search jamo", "en", intl.CollatorOptions{Usage: intl.UsageSearch}, "\u1100", "\u11a8", 0},
		{"search double jamo", "en", intl.CollatorOptions{Usage: intl.UsageSearch}, "\u1101", "\u1100\u1100", 0},
		// Search is a tailoring of the root with data of its own.
		{"root search", "en", intl.CollatorOptions{Usage: intl.UsageSearch, Sensitivity: intl.SensitivityBase}, "ا", "آ", 0},
		{"root sort", "en", intl.CollatorOptions{Sensitivity: intl.SensitivityBase}, "ا", "آ", 1},
		// A locale's search is a type of its own, and Czech has none: it
		// takes the root's, which has no "ch".
		{"cs sort", "cs", intl.CollatorOptions{}, "ch", "h", 1},
		{"cs search", "cs", intl.CollatorOptions{Usage: intl.UsageSearch}, "ch", "h", -1},
		// Thai ignores punctuation by default; a shifted element keeps its
		// primary for the quaternary level and must not count at the first.
		{"th shifted", "th", intl.CollatorOptions{}, "file 1", "file1", 0},
		{"th not shifted", "th", intl.CollatorOptions{IgnorePunctuation: intl.Bool(false)}, "file 1", "file1", -1},
		{"shifted", "en", intl.CollatorOptions{IgnorePunctuation: intl.Bool(true)}, "a-b", "ab", 0},
		// The collation tree: Bokmål collates as Norwegian, traditional
		// Chinese by stroke.
		{"nb from no", "nb", intl.CollatorOptions{}, "å", "z", 1},
		{"en", "en", intl.CollatorOptions{}, "å", "z", -1},
		{"zh-TW stroke", "zh-TW", intl.CollatorOptions{}, "张", "王", 1},
		{"zh pinyin", "zh", intl.CollatorOptions{}, "张", "王", 1},
		// Han in radical-stroke order, not code point blocks.
		{"unihan", "en", intl.CollatorOptions{}, "㐀", "龠", -1},
		{"numeric", "en", intl.CollatorOptions{Numeric: intl.Bool(true)}, "file9", "file10", -1},
		{"not numeric", "en", intl.CollatorOptions{}, "file9", "file10", 1},
		{"numeric long", "en", intl.CollatorOptions{Numeric: intl.Bool(true)}, "12345678901234567890", "12345678901234567891", -1},
		{"numeric zeros", "en", intl.CollatorOptions{Numeric: intl.Bool(true)}, "007", "7", 0},
		// Canonically equivalent text is equal, in whatever order its marks
		// were written.
		{"canonical order", "en", intl.CollatorOptions{}, "a\u0328\u0301", "a\u0301\u0328", 0},
		{"precomposed", "en", intl.CollatorOptions{}, "\u0105\u0301", "a\u0301\u0328", 0},
		// Lithuanian does not weigh a dot above an accented i.
		{"lt dot above", "lt", intl.CollatorOptions{}, "i\u0307\u0301", "i\u0301", 0},
		{"en dot above", "en", intl.CollatorOptions{}, "i\u0307\u0301", "i\u0301", 1},
		{"upper first", "en", intl.CollatorOptions{CaseFirst: intl.CaseFirstUpper}, "a", "A", 1},
		{"da upper first", "da", intl.CollatorOptions{}, "a", "A", 1},
		{"lower first", "en", intl.CollatorOptions{}, "a", "A", -1},
		{"case level", "en", intl.CollatorOptions{Sensitivity: intl.SensitivityCase}, "a", "á", 0},
		{"case level upper", "en", intl.CollatorOptions{Sensitivity: intl.SensitivityCase}, "a", "A", -1},
		{"accent", "en", intl.CollatorOptions{Sensitivity: intl.SensitivityAccent}, "a", "A", 0},
		{"phonebook", "de", intl.CollatorOptions{Collation: "phonebk"}, "ä", "af", -1},
		{"traditional", "es", intl.CollatorOptions{Collation: "trad"}, "ch", "cz", 1},
		{"sv", "sv", intl.CollatorOptions{}, "ö", "z", 1},
		// Canadian French weighs the last accent first.
		{"fr-CA backward", "fr-CA", intl.CollatorOptions{}, "côte", "coté", -1},
		{"fr forward", "fr", intl.CollatorOptions{}, "côte", "coté", 1},
		// Japanese: the length mark takes the vowel before it, by a prefix.
		{"ja length mark", "ja", intl.CollatorOptions{}, "カー", "カア", -1},
		{"ja kana", "ja", intl.CollatorOptions{}, "あ", "ア", 0},
		{"nul", "en", intl.CollatorOptions{}, "\x00", "a", -1},
		{"unassigned", "en", intl.CollatorOptions{}, "\u0378", "\u0379", -1},
		{"private use", "en", intl.CollatorOptions{}, "\U0010ffff", "\U000f0000", 1},
	}
	for _, tt := range tests {
		loc, err := intl.ParseLocale(tt.locale)
		if err != nil {
			t.Fatal(err)
		}
		c, err := intl.NewCollator(loc, tt.opts)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := c.Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("%s: Compare(%+q, %+q) in %s = %d, want %d", tt.name, tt.a, tt.b, tt.locale, got, tt.want)
		}
		if got := c.Compare(tt.b, tt.a); got != -tt.want {
			t.Errorf("%s: Compare(%+q, %+q) in %s = %d, want %d", tt.name, tt.b, tt.a, tt.locale, got, -tt.want)
		}
	}
}

func TestCollatorResolvedOptions(t *testing.T) {
	tests := []struct {
		locale    string
		opts      intl.CollatorOptions
		wantLoc   string
		wantColl  string
		wantPunct bool
		wantCase  intl.CaseFirst
	}{
		{"de-u-co-phonebk", intl.CollatorOptions{}, "de-u-co-phonebk", "phonebk", false, intl.CaseFirstFalse},
		// ECMA-402's ResolveLocale keeps a keyword only when the options did
		// not overrule it, and adds none for a value the options chose.
		{"de-u-co-phonebk", intl.CollatorOptions{Collation: "eor"}, "de", "eor", false, intl.CaseFirstFalse},
		{"de", intl.CollatorOptions{Collation: "eor"}, "de", "eor", false, intl.CaseFirstFalse},
		{"de", intl.CollatorOptions{Collation: "eor", Compat: intl.NodeICU}, "de-u-co-eor", "eor", false, intl.CaseFirstFalse},
		{"de-u-co-phonebk", intl.CollatorOptions{Collation: "pinyin"}, "de-u-co-phonebk", "phonebk", false, intl.CaseFirstFalse},
		{"en-u-co-phonebk", intl.CollatorOptions{}, "en", "default", false, intl.CaseFirstFalse},
		// A keyword the locale does not support does not stop Node writing
		// the option's collation in; one it honours does.
		{"en-u-co-phonebk", intl.CollatorOptions{Collation: "emoji"}, "en", "emoji", false, intl.CaseFirstFalse},
		{"en-u-co-phonebk", intl.CollatorOptions{Collation: "emoji", Compat: intl.NodeICU}, "en-u-co-emoji", "emoji", false, intl.CaseFirstFalse},
		{"de-u-co-pinyin", intl.CollatorOptions{Collation: "phonebk", Compat: intl.NodeICU}, "de-u-co-phonebk", "phonebk", false, intl.CaseFirstFalse},
		{"de-u-co-phonebk", intl.CollatorOptions{Collation: "eor", Compat: intl.NodeICU}, "de", "eor", false, intl.CaseFirstFalse},
		// Search reports no collation, but keeps a keyword it supports.
		{"de-u-co-phonebk", intl.CollatorOptions{Usage: intl.UsageSearch}, "de-u-co-phonebk", "default", false, intl.CaseFirstFalse},
		// "standard" and "search" are not collations a caller may name.
		{"de-u-co-search", intl.CollatorOptions{}, "de", "default", false, intl.CaseFirstFalse},
		{"de-u-co-standard", intl.CollatorOptions{Usage: intl.UsageSearch}, "de", "default", false, intl.CaseFirstFalse},
		// Node reports Chinese's default collation by name.
		{"zh", intl.CollatorOptions{}, "zh", "pinyin", false, intl.CaseFirstFalse},
		{"zh-TW", intl.CollatorOptions{}, "zh-TW", "stroke", false, intl.CaseFirstFalse},
		{"th", intl.CollatorOptions{}, "th", "default", true, intl.CaseFirstFalse},
		{"da", intl.CollatorOptions{}, "da", "default", false, intl.CaseFirstUpper},
		{"en-u-kf-upper", intl.CollatorOptions{}, "en-u-kf-upper", "default", false, intl.CaseFirstUpper},
		{"en-u-kf-upper", intl.CollatorOptions{CaseFirst: intl.CaseFirstLower}, "en", "default", false, intl.CaseFirstLower},
		// "-u-kn-true" is written "-u-kn".
		{"de-u-kn-true", intl.CollatorOptions{}, "de-u-kn", "default", false, intl.CaseFirstFalse},
		{"de-u-kn", intl.CollatorOptions{Numeric: intl.Bool(false)}, "de", "default", false, intl.CaseFirstFalse},
	}
	for _, tt := range tests {
		loc, err := intl.ParseLocale(tt.locale)
		if err != nil {
			t.Fatal(err)
		}
		c, err := intl.NewCollator(loc, tt.opts)
		if err != nil {
			t.Fatalf("%s: %v", tt.locale, err)
		}
		r := c.ResolvedOptions()
		if r.Locale != tt.wantLoc || r.Collation != tt.wantColl || r.IgnorePunctuation != tt.wantPunct || r.CaseFirst != tt.wantCase {
			t.Errorf("%s %+v: resolved %+v, want locale %s, collation %s, ignorePunctuation %v, caseFirst %v",
				tt.locale, tt.opts, r, tt.wantLoc, tt.wantColl, tt.wantPunct, tt.wantCase)
		}
	}
}

// withoutMarker is a source that has no data under one marker.
type withoutMarker struct {
	intl.Source
	hidden intl.Marker
}

func (s withoutMarker) Open(m intl.Marker, d intl.DataLocale) ([]byte, error) {
	if m == s.hidden {
		return nil, intl.ErrNotFound
	}
	return s.Source.Open(m, d)
}

// A source without ICU's index of the collation tree finds a locale's
// collation along the collation tree's own pairs, which only such a source
// reads.
func TestCollatorWithoutICUIndex(t *testing.T) {
	src := withoutMarker{intl.Embedded, "icutree-coll"}
	for _, tc := range []struct {
		locale string
		a, b   string
		want   int
	}{
		// Swedish sorts ö after z; the root sorts it with o.
		{"sv", "ö", "z", 1},
		{"sv-FI", "ö", "z", 1},
		{"en", "ö", "z", -1},
	} {
		loc, err := intl.ParseLocale(tc.locale)
		if err != nil {
			t.Fatal(err)
		}
		c, err := intl.NewCollatorFrom(src, loc, intl.CollatorOptions{})
		if err != nil {
			t.Fatalf("%s: %v", tc.locale, err)
		}
		if got := c.Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: Compare(%q, %q) = %d, want %d", tc.locale, tc.a, tc.b, got, tc.want)
		}
	}
}
