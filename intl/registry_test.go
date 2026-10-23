// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/liustack/slowjs"
)

// The expected values in this file are Node 24.13's.

// TestRegistryShared makes the same formatter on two VMs, spelled two ways:
// the process makes it once.
func TestRegistryShared(t *testing.T) {
	vm1, vm2 := newVM(t), newVM(t)
	const nf = `{style: 'unit', unit: 'kilometer-per-hour', maximumFractionDigits: 7}`
	before := formatters.madeCount()
	evalTests(t, vm1, [][2]string{{`new Intl.NumberFormat('en', ` + nf + `).format(1.5)`, "1.5 km/h"}})
	made := formatters.madeCount()
	if made != before+1 {
		t.Fatalf("one new formatter made %d entries", made-before)
	}

	evalTests(t, vm2, [][2]string{
		{`new Intl.NumberFormat('en', ` + nf + `).format(2.25)`, "2.25 km/h"},
		{`new Intl.NumberFormat(['en', 'de'], {localeMatcher: 'lookup', ...` + nf + `}).format(3)`, "3 km/h"},
		{`(3).toLocaleString('en', ` + nf + `)`, "3 km/h"},
	})
	if n := formatters.madeCount(); n != made {
		t.Fatalf("the same formatter on another VM made %d more entries", n-made)
	}
}

// floodJS makes more distinct formatters than the registry keeps, and checks
// some of them.
const floodJS = `(() => {
	const r = [];
	for (let k = 1; k <= 21; k++) {
		for (let f = 0; f <= 60; f++) {
			const s = new Intl.NumberFormat('en', {minimumIntegerDigits: k, maximumFractionDigits: f}).format(1234.56789);
			if ((k === 1 || k === 5 || k === 21) && (f === 0 || f === 3 || f === 20)) {
				r.push(s);
			}
		}
	}
	return r.join(' ');
})()`

const floodWant = "1,235 1,234.568 1,234.56789 01,235 01,234.568 01,234.56789 " +
	"000,000,000,000,000,001,235 000,000,000,000,000,001,234.568 000,000,000,000,000,001,234.56789"

// TestRegistryBounded makes more formatters than the registry keeps. It keeps
// no more, and the objects made first, whose formatters it has dropped, go on
// working.
func TestRegistryBounded(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `
		var nf = new Intl.NumberFormat('en', {minimumIntegerDigits: 3, maximumFractionDigits: 1});
		var dtf = new Intl.DateTimeFormat('de', {dateStyle: 'medium', timeZone: 'Asia/Tokyo'});
		var pr = new Intl.PluralRules('en', {type: 'ordinal'});
		var nfFormat = nf.format;
		0`)
	evalTests(t, vm, [][2]string{{floodJS, floodWant}})
	if entries, requests := formatters.count(); entries > registrySlots || requests > registryRequests {
		t.Fatalf("the registry has %d formatters and %d requests", entries, requests)
	}

	evalTests(t, vm, [][2]string{
		{`nf.format(1.25)`, "001.3"},
		{`nfFormat(1.25)`, "001.3"},
		{`JSON.stringify(nf.formatToParts(1.25))`, `[{"type":"integer","value":"001"},{"type":"decimal","value":"."},{"type":"fraction","value":"3"}]`},
		{`JSON.stringify(nf.resolvedOptions())`, `{"locale":"en","numberingSystem":"latn","style":"decimal","minimumIntegerDigits":3,"minimumFractionDigits":0,"maximumFractionDigits":1,"useGrouping":"auto","notation":"standard","signDisplay":"auto","roundingIncrement":1,"roundingMode":"halfExpand","roundingPriority":"auto","trailingZeroDisplay":"auto"}`},
		{`dtf.format(0)`, "01.01.1970"},
		{`JSON.stringify(dtf.resolvedOptions())`, `{"locale":"de","calendar":"gregory","numberingSystem":"latn","timeZone":"Asia/Tokyo","dateStyle":"medium"}`},
		{`pr.select(2) + ' ' + pr.selectRange(1, 2)`, "two other"},
		// Node 24 has no notation in it, which Chromium 156 has.
		{`const {notation, ...o} = pr.resolvedOptions(); JSON.stringify(o)`, `{"locale":"en","type":"ordinal","minimumIntegerDigits":1,"minimumFractionDigits":0,"maximumFractionDigits":3,"pluralCategories":["one","two","few","other"],"roundingIncrement":1,"roundingMode":"halfExpand","roundingPriority":"auto","trailingZeroDisplay":"auto"}`},
	})
}

// TestRegistryConcurrent has VMs on several goroutines make formatters, more
// than the registry keeps, and format with them, with objects held across
// the turnover. Run with -race.
func TestRegistryConcurrent(t *testing.T) {
	made := formatters.madeCount()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm, err := slowjs.NewVM()
			if err != nil {
				t.Error(err)
				return
			}

			defer vm.Close()

			if err := Register(vm); err != nil {
				t.Error(err)
				return
			}

			src := fmt.Sprintf(`(() => {
				const held = new Intl.NumberFormat('en', {minimumIntegerDigits: 3, maximumFractionDigits: 1});
				const bad = [];
				for (let i = 0; i < 400; i++) {
					const n = (i * 8 + %d) %% 2121, k = 1 + n %% 21, f = (n - n %% 21) / 21;
					new Intl.NumberFormat('en', {minimumIntegerDigits: k, maximumFractionDigits: f, roundingMode: 'halfEven'}).format(1.5);
					const s = new Intl.NumberFormat('en', {minimumIntegerDigits: 5, maximumFractionDigits: 3}).format(1234.56789);
					if (s !== '01,234.568') {
						bad.push(s);
					}
					if (held.format(1.25) !== '001.3') {
						bad.push(held.format(1.25));
					}
				}
				return bad.join(' ');
			})()`, g)
			v, err := vm.Eval(src, slowjs.EvalGlobal)
			if err != nil || v != "" {
				t.Errorf("goroutine %d: %v %v", g, v, err)
			}
		}()
	}
	wg.Wait()
	if n := formatters.madeCount() - made; n <= registrySlots {
		t.Fatalf("made %d formatters, not enough to turn the registry over", n)
	}

	if entries, requests := formatters.count(); entries > registrySlots || requests > registryRequests {
		t.Fatalf("the registry has %d formatters and %d requests", entries, requests)
	}
}

func TestSplitLocales(t *testing.T) {
	for _, test := range []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"5:en-US", []string{"en-US"}},
		{"5:en-US2:de", []string{"en-US", "de"}},
		{"0:", []string{""}},
		{"2:a:1:b", []string{"a:", "b"}},
		{"2:\U0001F600", []string{"\U0001F600"}},
		{"1:�3:abc", []string{"�", "abc"}},
	} {
		got, ok := splitLocales(test.in)
		if !ok || fmt.Sprint(got) != fmt.Sprint(test.want) || len(got) != len(test.want) {
			t.Errorf("%q: got %q, %v, want %q", test.in, got, ok, test.want)
		}
	}
	for _, in := range []string{"5:en", "x", "1", ":", "1:\U0001F600", "99999999999999999999:a"} {
		if got, ok := splitLocales(in); ok {
			t.Errorf("%q: got %q, want an error", in, got)
		}
	}
}

// testFormatter is a formatter for the tests of the registry alone.
type testFormatter struct {
	N int `json:"n"`
}

func (*testFormatter) init() error { return nil }

// The registry keeps no more than registrySlots formatters and
// registryRequests requests. An id it dropped finds nothing, and the same
// formatter made again gets a new id.
func TestRegistryDropsOldest(t *testing.T) {
	var r registry
	first, err := r.add([]byte("T0"), &testFormatter{N: 0})
	if err != nil {
		t.Fatal(err)
	}

	if r.entry(float64(first.id)) != first || r.lookup([]byte("T0")) != first {
		t.Fatal("the registry does not find what it was given")
	}

	for i := 1; i <= registryRequests+10; i++ {
		if _, err := r.add([]byte(fmt.Sprintf("T%d", i)), &testFormatter{N: i}); err != nil {
			t.Fatal(err)
		}
	}
	if entries, requests := r.count(); entries > registrySlots || requests > registryRequests {
		t.Fatalf("the registry has %d formatters and %d requests", entries, requests)
	}
	if r.entry(float64(first.id)) != nil || r.lookup([]byte("T0")) != nil {
		t.Error("the registry still finds a formatter it dropped")
	}

	again, err := r.add([]byte("T0"), &testFormatter{N: 0})
	if err != nil {
		t.Fatal(err)
	}

	if again.id == first.id || r.entry(float64(again.id)) != again {
		t.Errorf("made again, the formatter has id %d, and %d before", again.id, first.id)
	}
}

// A request key longer than maxRequestKey is not remembered: the registry
// finds its formatter by what it resolves to, and its size is bounded by
// that.
func TestRegistryLongRequest(t *testing.T) {
	var r registry
	short, long := []byte("Tshort"), []byte("T"+strings.Repeat("x", maxRequestKey))
	e, err := r.add(short, &testFormatter{N: 1})
	if err != nil {
		t.Fatal(err)
	}

	if r.lookup(short) != e {
		t.Fatal("a short request is not remembered")
	}

	e2, err := r.add(long, &testFormatter{N: 1})
	if err != nil {
		t.Fatal(err)
	}

	if e2 != e {
		t.Error("a long request that resolves alike does not share the formatter")
	}
	if r.lookup(long) != nil {
		t.Error("a long request is remembered")
	}
	if _, requests := r.count(); requests != 1 {
		t.Errorf("the registry remembers %d requests, want 1", requests)
	}
	if n := r.longestRequest(); n != len(short) {
		t.Errorf("the longest request remembered has %d bytes", n)
	}
}

// Long lists of locales, which a page may make of an Accept-Language header,
// leave no long request keys in the registry.
func TestRegistryLongLocaleLists(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{{`(() => {
		const r = [];
		for (let i = 0; i < 40; i++) {
			r.push(new Intl.NumberFormat(Array(2000).fill('en').concat(['de-x-' + i]), {}).format(1234.5));
		}
		return r[0] + ' ' + r[39];
	})()`, "1,234.5 1,234.5"}})
	if n := formatters.longestRequest(); n > maxRequestKey {
		t.Fatalf("the registry remembers a request of %d bytes", n)
	}
}

// A call whose formatter the registry dropped resolves it again, also when
// the new one is dropped too, a few times. After that it throws an Error: it
// never answers undefined, nor fails as something else.
func TestCallRetries(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `var nf = new Intl.NumberFormat('en', {maximumFractionDigits: 1});
		var dtf = new Intl.DateTimeFormat('en', {timeZone: 'UTC'});
		var pr = new Intl.PluralRules('en'); 0`)
	uses := []string{
		`nf.format(1.25)`,
		`nf.formatToParts(1.25).map(p => p.value).join('')`,
		`nf.resolvedOptions().maximumFractionDigits`,
		`dtf.format(0)`,
		`pr.select(1)`,
		`(1.25).toLocaleString('en', {maximumFractionDigits: 1})`,
	}
	want := []string{"1.3", "1.3", "1", "1/1/1970", "one", "1.3"}
	defer formatters.dropping.Store(0)

	for i, use := range uses {
		formatters.dropping.Store(7)
		evalTests(t, vm, [][2]string{{use, want[i]}})
		formatters.dropping.Store(8)
		evalTests(t, vm, [][2]string{{`try { String(` + use + `) } catch (e) { e.constructor.name + ' ' + e.message.includes('too fast') }`, "Error true"}})
		formatters.dropping.Store(0)
		evalTests(t, vm, [][2]string{{use, want[i]}})
	}
}

// TestRegistryEviction has VMs on other goroutines make formatters as fast as
// they can, turning the registry over and over, while a VM formats with
// objects it holds. On one CPU the others run between any two steps of the
// VM, which then finds its formatters dropped again and again: every call
// still gives the right answer. Run with -race.
func TestRegistryEviction(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))

	var stop atomic.Bool
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm, err := slowjs.NewVM()
			if err != nil {
				t.Error(err)
				return
			}

			defer vm.Close()

			if err := Register(vm); err != nil {
				t.Error(err)
				return
			}

			if _, err := vm.Eval(fmt.Sprintf(`var g = %d, k = 0`, g), slowjs.EvalGlobal); err != nil {
				t.Error(err)
				return
			}

			for !stop.Load() {
				if _, err := vm.Eval(`for (let i = 0; i < 50; i++, k++) {
					new Intl.NumberFormat('en', {minimumIntegerDigits: 1 + k % 21, maximumFractionDigits: g * 25 + (k - k % 21) / 21 % 25});
				}`, slowjs.EvalGlobal); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}

	n := 200000
	if testing.Short() {
		n = 20000
	}
	vm := newVM(t)
	made := formatters.madeCount()
	evalTests(t, vm, [][2]string{{fmt.Sprintf(`(() => {
		const nf = new Intl.NumberFormat('en', {minimumIntegerDigits: 2, roundingMode: 'trunc'});
		const bad = [];
		for (let i = 0; i < %d && bad.length < 5; i++) {
			const s = i %% 3 === 0 ? nf.format(1) : i %% 3 === 1 ? nf.formatToParts(1)[0].value : nf.resolvedOptions().roundingMode;
			if (s !== (i %% 3 === 2 ? 'trunc' : '01')) {
				bad.push(i + ': ' + s);
			}
		}
		return bad.join(', ');
	})()`, n), ""}})
	stop.Store(true)
	wg.Wait()
	t.Logf("%d formatters made meanwhile", formatters.madeCount()-made)
}
