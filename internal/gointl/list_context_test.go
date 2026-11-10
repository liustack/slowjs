//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Spanish "y" is "e" before an "i" sound and "o" is "u" before an "o" sound,
// and Hebrew "ו" takes a dash before a word not written in Hebrew, as ICU's
// listformatter.cpp chooses in code: go-intl wrote "a, b y Isabel" where
// Node writes "a, b e Isabel". Node's answers (ISSUES.md NF-15).
func TestListContextualJoins(t *testing.T) {
	conj, disj, unit := intl.ListFormatOptions{}, intl.ListFormatOptions{Type: intl.Disjunction},
		intl.ListFormatOptions{Type: intl.UnitList}
	for _, c := range []struct {
		tag   string
		opts  intl.ListFormatOptions
		items []string
		want  string
	}{
		{"es", conj, []string{"a", "b", "Isabel"}, "a, b e Isabel"},
		{"es", conj, []string{"a", "Hi"}, "a e Hi"},
		{"es", conj, []string{"a", "hielo"}, "a y hielo"},
		{"es", conj, []string{"a", "hiato"}, "a y hiato"},
		{"es", conj, []string{"a", "hie"}, "a y hie"},
		{"es", conj, []string{"Isabel", "b"}, "Isabel y b"},
		{"es", intl.ListFormatOptions{Style: intl.ListNarrow}, []string{"a", "i"}, "a e i"},
		{"es", unit, []string{"a", "i"}, "a e i"},
		{"es-MX", conj, []string{"a", "i"}, "a e i"},
		{"es", disj, []string{"siete", "ocho"}, "siete u ocho"},
		{"es", disj, []string{"a", "11"}, "a u 11"},
		{"es", disj, []string{"a", "11 x"}, "a u 11 x"},
		{"es", disj, []string{"a", "110"}, "a o 110"},
		{"es", disj, []string{"a", "Hola", "b"}, "a, Hola o b"},
		{"es", intl.ListFormatOptions{Type: intl.Disjunction, Style: intl.ListShort}, []string{"a", "8"}, "a u 8"},
		{"es-419", disj, []string{"a", "o"}, "a u o"},
		{"he", conj, []string{"a", "Bob"}, "a ו-Bob"},
		{"he", conj, []string{"אב", "גד"}, "אב וגד"},
		{"he", conj, []string{"אב", "1"}, "אב ו-1"},
		{"he", conj, []string{"אב", "גד", "Bob"}, "אב, גד ו-Bob"},
		{"he", conj, []string{"a", "ְx"}, "a וְx"},
		{"he", intl.ListFormatOptions{Style: intl.ListNarrow}, []string{"a", "Bob"}, "a ו-Bob"},
		{"he", disj, []string{"a", "Bob"}, "a או Bob"},
		{"iw", conj, []string{"a", "b"}, "a ו-b"},
		{"en", conj, []string{"a", "i"}, "a and i"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		f, err := intl.NewListFormat(loc, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(c.items); got != c.want {
			t.Errorf("%s %+v %q: %q, want %q", c.tag, c.opts, c.items, got, c.want)
		}
	}
	loc, err := intl.ParseLocale("es")
	if err != nil {
		t.Fatal(err)
	}
	f, err := intl.NewListFormat(loc, conj)
	if err != nil {
		t.Fatal(err)
	}
	want := []intl.ListPart{{intl.ListElement, "a"}, {intl.ListLiteral, ", "}, {intl.ListElement, "b"},
		{intl.ListLiteral, " e "}, {intl.ListElement, "Isabel"}}
	if got := f.FormatToParts([]string{"a", "b", "Isabel"}); !reflect.DeepEqual(got, want) {
		t.Errorf("parts: %q, want %q", got, want)
	}
}
