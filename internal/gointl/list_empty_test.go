//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// An empty item: ECMA-402's CreatePartsFromList writes it as an element,
// and ICU leaves it out, joining the literals either side of it
// (EmptyListItems). The text is the same. Node's are its answers (ISSUES.md
// NF-19).
func TestEmptyListItems(t *testing.T) {
	el := func(v string) intl.ListPart { return intl.ListPart{Kind: intl.ListElement, Value: v} }
	lit := func(v string) intl.ListPart { return intl.ListPart{Kind: intl.ListLiteral, Value: v} }
	for _, c := range []struct {
		tag      string
		items    []string
		standard []intl.ListPart
		node     []intl.ListPart
	}{
		{"en", []string{"a", ""}, []intl.ListPart{el("a"), lit(" and "), el("")},
			[]intl.ListPart{el("a"), lit(" and ")}},
		{"en", []string{"", "b"}, []intl.ListPart{el(""), lit(" and "), el("b")},
			[]intl.ListPart{lit(" and "), el("b")}},
		{"en", []string{"a", "", "b"}, []intl.ListPart{el("a"), lit(", "), el(""), lit(", and "), el("b")},
			[]intl.ListPart{el("a"), lit(", , and "), el("b")}},
		{"en", []string{""}, []intl.ListPart{el("")}, []intl.ListPart{}},
		{"es", []string{"a", ""}, []intl.ListPart{el("a"), lit(" y "), el("")},
			[]intl.ListPart{el("a"), lit(" y ")}},
		{"en", []string{"a", "b"}, []intl.ListPart{el("a"), lit(" and "), el("b")},
			[]intl.ListPart{el("a"), lit(" and "), el("b")}},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []struct {
			compat intl.Compat
			want   []intl.ListPart
		}{{intl.Standard, c.standard}, {intl.EmptyListItems, c.node}, {intl.NodeICU, c.node}} {
			f, err := intl.NewListFormat(loc, intl.ListFormatOptions{Compat: side.compat})
			if err != nil {
				t.Fatal(err)
			}
			got := f.FormatToParts(c.items)
			if len(got) == 0 && len(side.want) == 0 {
				continue
			}
			if !reflect.DeepEqual(got, side.want) {
				t.Errorf("%s %q %v: %q, want %q", c.tag, c.items, side.compat, got, side.want)
			}
		}
	}
}
