// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/beastjs/intl"

import (
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/liustack/beastjs"
	gointl "github.com/liustack/beastjs/internal/gointl"
)

// listItem is an item of a list, as a javascript literal and as the WTF-8
// Go reads it as.
type listItem struct{ js, go_ string }

// plainItem is an item written the same in both.
func plainItem(s string) listItem { return listItem{strconv.Quote(s), s} }

// listLasts are last items, each of the ways the patterns of Spanish and
// Hebrew read the first characters of one: an "i" or "o" sound in any case,
// eight and eleven, a word in Hebrew or not, a lone surrogate, a character
// past the BMP, and items longer than the head intl.js sends.
var listLasts = []listItem{
	plainItem("i"), plainItem("hi"), plainItem("hia"), plainItem("hie"), plainItem("HIe"), plainItem("Ion"),
	plainItem("o"), plainItem("ho"), plainItem("Hola"), plainItem("ola"), plainItem("8"), plainItem("80"),
	plainItem("11"), plainItem("11 x"), plainItem("110"), plainItem("11 abcdefghijk"), plainItem("iiiiiiiiiiiii"),
	plainItem("hiaaaaaaaaaaa"), plainItem("שלום"), plainItem("word"), plainItem(""), plainItem("😀x"),
	{`'\ud800'`, "\xed\xa0\x80"}, {`'\udc00i'`, "\xed\xb0\x80i"},
}

// listCases are the lists TestListFormatAsBefore formats: of 0 to 6 items,
// with empty items at each place, and with each of listLasts last in a list
// of two to four.
func listCases() [][]listItem {
	abc := func(n int) []listItem {
		var l []listItem
		for i := range n {
			l = append(l, plainItem(string(rune('a'+i))))
		}
		return l
	}
	var cases [][]listItem
	for n := 0; n <= 6; n++ {
		cases = append(cases, abc(n))
	}
	for n := 1; n <= 5; n++ {
		for k := range n {
			l := abc(n)
			l[k] = plainItem("")
			cases = append(cases, l)
		}
		l := abc(n)
		for k := range l {
			l[k] = plainItem("")
		}
		cases = append(cases, l)
	}
	for n := 2; n <= 4; n++ {
		for _, last := range listLasts {
			cases = append(cases, append(abc(n-1), last))
		}
	}
	return cases
}

// What a ListFormat makes of a list is what it made when Go joined the list
// (gointl's ListFormat.FormatToParts, as it reads the items in WTF-8), in
// every locale, type and style: the parts of formatToParts, each element
// one of the items in order, and format, which is their values joined.
func TestListFormatAsBefore(t *testing.T) {
	m, err := goMatcher(gointl.ServiceListFormat)
	if err != nil {
		t.Fatal(err)
	}
	cases := listCases()
	var src strings.Builder
	src.WriteString("var lists = [")
	for _, l := range cases {
		src.WriteString("[")
		for _, it := range l {
			src.WriteString(it.js + ",")
		}
		src.WriteString("],")
	}
	src.WriteString(`];
var describe = (lf) => JSON.stringify(lists.map((l) => {
	const parts = lf.formatToParts(l);
	let next = 0, joined = '';
	const d = parts.map((p) => {
		joined += p.value;
		if (p.type === 'literal') return 'L' + p.value;
		while (next < l.length && l[next] === '' && p.value !== '') next++;
		if (next < l.length && l[next] === p.value) return 'E' + next++;
		return 'X' + p.type;
	});
	if (lf.format(l) !== joined) d.push('format ' + JSON.stringify(lf.format(l)));
	return d;
}));
0`)
	vm := newVM(t)
	eval(t, vm, src.String())
	locales := m.Locales()
	if len(locales) < 100 {
		t.Fatalf("ListFormat is available in %d locales", len(locales))
	}
	for _, tag := range locales {
		for _, typ := range []string{"conjunction", "disjunction", "unit"} {
			for _, style := range []string{"long", "short", "narrow"} {
				lf, err := resolveListFormat(request{Locales: []string{tag}, Options: map[string]any{"type": typ, "style": style}})
				if err != nil {
					t.Fatal(err)
				}
				got := eval(t, vm, "describe(new Intl.ListFormat("+strconv.Quote(tag)+", {type: '"+typ+"', style: '"+style+"'}))").(string)
				var gotParts [][]string
				if err := json.Unmarshal([]byte(got), &gotParts); err != nil {
					t.Fatal(err)
				}
				for i, l := range cases {
					items := make([]string, len(l))
					for k, it := range l {
						items[k] = it.go_
					}
					var want []string
					next := 0
					for _, p := range lf.f.FormatToParts(items) {
						if p.Kind == gointl.ListLiteral {
							want = append(want, "L"+p.Value)
							continue
						}
						for next < len(items) && items[next] == "" && p.Value != "" {
							next++
						}
						want = append(want, "E"+strconv.Itoa(next))
						next++
					}
					if strings.Join(gotParts[i], "|") != strings.Join(want, "|") {
						t.Fatalf("%s %s %s, list %d %q: %q, want %q", tag, typ, style, i, items, gotParts[i], want)
					}
				}
			}
		}
	}
}

// A ListFormat joins a list of any length with no work in Go after its
// first call: the javascript of intl.js writes the parts. Go made a list of
// the items, a list of the parts and a value for each part, 1.8 GiB for
// 8 Mi items, outside the memory limit, grown and cleared whole with no
// check.
func TestListFormatTakesNoGo(t *testing.T) {
	const n = 1 << 20
	for _, c := range []struct{ name, setup, call string }{
		{"format of empty items", `var l = Array(N).fill(''), f = new Intl.ListFormat('en')`, `f.format(l).length`},
		{"formatToParts of short items", `var l = Array(N).fill('abcdefgh'), f = new Intl.ListFormat('en')`, `f.formatToParts(l).length`},
		{"format in Spanish", `var l = Array(N).fill('hielo'), f = new Intl.ListFormat('es')`, `f.format(l).length`},
		{"formatToParts in Hebrew", `var l = Array(N).fill('word'), f = new Intl.ListFormat('he')`, `f.formatToParts(l).length`},
	} {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			eval(t, vm, `f.format(['a', 'b', 'c']); f.format(['a', 'b'])`)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			if _, err := vm.Eval(c.call, beastjs.EvalGlobal); err != nil {
				t.Fatal(err)
			}
			runtime.ReadMemStats(&after)
			if a := after.TotalAlloc - before.TotalAlloc; a > 64<<10 {
				t.Errorf("%s: Go allocated %d KiB for %d items, want at most 64", c.call, a>>10, n)
			}
		})
	}
}
