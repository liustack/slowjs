//go:build gointl_all

package intl

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/gointl/internal/corpus"
)

// TestListWrittenInOrder pins that writing a list from the front gives the
// parts folding each item in gives, for every locale the golden file has,
// every type and style, and lists of three items or more -- fewer have
// patterns of their own -- and that a long list takes time in proportion to
// its length.
func TestListWrittenInOrder(t *testing.T) {
	f, err := corpus.Load(filepath.FromSlash("testdata/intl_golden.txt"))
	if err != nil {
		t.Fatal(err)
	}
	locales := map[string]bool{}
	for _, c := range f.Cases {
		locales[c.Locale] = true
	}
	checked := 0
	for tag := range locales {
		loc, err := ParseLocale(tag)
		if err != nil {
			continue
		}
		for _, typ := range []ListType{Conjunction, Disjunction, UnitList} {
			for _, style := range []ListStyle{ListLong, ListShort, ListNarrow} {
				lf, err := NewListFormat(loc, ListFormatOptions{Type: typ, Style: style})
				if err != nil {
					continue
				}
				for n := 3; n <= 7; n++ {
					items := make([]string, n)
					for i := range items {
						items[i] = fmt.Sprint("item", i)
					}
					got, ok := lf.formatInOrder(items)
					if !ok {
						t.Errorf("%s %d %d: patterns %+v not written in order", tag, typ, style, lf.patterns)
						continue
					}
					if want := lf.formatFolding(items); !reflect.DeepEqual(got, want) {
						t.Errorf("%s %d %d, %d items:\n got %v\nwant %v", tag, typ, style, n, got, want)
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("nothing checked")
	}

	en, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	lf, err := NewListFormat(en, ListFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]string, 200000)
	for i := range items {
		items[i] = "x"
	}
	start := time.Now()
	if got := len(lf.Format(items)); got != 200000+2*199999+len("and ") {
		t.Errorf("length %d", got)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("200000 items took %v", d)
	}
}
