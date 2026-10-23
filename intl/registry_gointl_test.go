// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/liustack/slowjs"
)

// goIntlUses are objects of the constructors built on go-intl, made once and
// held, with a use of each and its answer in Chromium 156.
var goIntlUses = []struct{ make, use, want string }{
	{`var col = new Intl.Collator('de', {sensitivity: 'base'}), colCompare = col.compare`,
		`col.compare('a', 'A') + ' ' + colCompare('a', 'b') + ' ' + col.resolvedOptions().sensitivity`, "0 -1 base"},
	{``, `'ä'.localeCompare('z', 'sv', {sensitivity: 'base'})`, "1"},
	{`var seg = new Intl.Segmenter('th', {granularity: 'word'}), segs = seg.segment('สวัสดีครับ'), segsUsed = seg.segment('ab cd.'); segsUsed.containing(0)`,
		`[...segs].map((s) => s.segment).join('|') + ' ' + segsUsed.containing(3).segment + ' ' + [...seg.segment('x y')].length + ' ' + seg.resolvedOptions().granularity`,
		"สวัสดี|ครับ cd 3 word"},
	{`var rtf = new Intl.RelativeTimeFormat('de', {numeric: 'auto', style: 'short'})`,
		`rtf.format(-1, 'day') + ' ' + rtf.format(3, 'hours') + ' ' + rtf.formatToParts(2, 'week').length + ' ' + rtf.resolvedOptions().style`,
		"gestern in 3 Std. 3 short"},
	{`var lf = new Intl.ListFormat('es', {type: 'disjunction'})`,
		`lf.format(['siete', 'ocho']) + ' ' + lf.formatToParts(['a', 'b', 'c']).length + ' ' + lf.resolvedOptions().type`,
		"siete u ocho 5 disjunction"},
	{`var dn = new Intl.DisplayNames('fr', {type: 'region', style: 'short'})`,
		`dn.of('US') + ' ' + new Intl.DisplayNames('fr', {type: 'language'}).of('de-AT') + ' ' + dn.of('BU') + ' ' + dn.resolvedOptions().style`,
		"É.-U. allemand autrichien Myanmar (Birmanie) short"},
	{`var df = new Intl.DurationFormat('de', {style: 'digital'})`,
		`df.format({hours: 1, minutes: 2, seconds: 3}) + ' ' + df.formatToParts('PT1M').length + ' ' + df.resolvedOptions().minutes`,
		"1:02:03 5 2-digit"},
	{`var loc = new Intl.Locale('de-at', {hourCycle: 'h12'})`,
		`loc.maximize() + ' ' + loc.getWeekInfo().firstDay + ' ' + loc.getHourCycles() + ' ' + loc.getTimeZones() + ' ' + ` +
			`new Intl.NumberFormat(loc).format(1.5) + ' ' + Intl.supportedValuesOf('unit').length`,
		"de-Latn-AT-u-hc-h12 1 h12 Europe/Vienna 1,5 45"},
}

// goIntlHold makes the objects of goIntlUses, and goIntlCheck uses them: it
// answers each use that is not right.
func goIntlHold() string {
	var b strings.Builder
	for _, u := range goIntlUses {
		if u.make != "" {
			b.WriteString(u.make + ";\n")
		}
	}
	return b.String() + "0"
}

func goIntlCheck() string {
	var b strings.Builder
	b.WriteString("(() => {\n\tconst bad = [];\n")
	for _, u := range goIntlUses {
		fmt.Fprintf(&b, "\t{ const s = String(%s); if (s !== %q) bad.push(%q + ': ' + s); }\n", u.use, u.want, u.use)
	}
	b.WriteString("\treturn bad.join('; ');\n})()")
	return b.String()
}

// TestRegistryGoIntl drops the go-intl formatters of held objects, by making
// more formatters than the registry keeps and by having the registry drop
// each new one a few times: every use resolves its formatter again and
// answers as before.
func TestRegistryGoIntl(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, goIntlHold())
	evalTests(t, vm, [][2]string{{goIntlCheck(), ""}})
	evalTests(t, vm, [][2]string{{floodJS, floodWant}})
	evalTests(t, vm, [][2]string{{goIntlCheck(), ""}})
	defer formatters.dropping.Store(0)

	formatters.dropping.Store(7)
	evalTests(t, vm, [][2]string{{goIntlCheck(), ""}})
	formatters.dropping.Store(0)
	evalTests(t, vm, [][2]string{{goIntlCheck(), ""}})
}

// TestRegistryGoIntlConcurrent has VMs on several goroutines use the go-intl
// objects while they turn the registry over. Run with -race.
func TestRegistryGoIntlConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for g := range 6 {
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

			if _, err := vm.Eval(goIntlHold(), slowjs.EvalGlobal); err != nil {
				t.Error(err)
				return
			}

			for i := range 20 {
				src := fmt.Sprintf(`for (let k = 0; k < 60; k++) {
					new Intl.NumberFormat('en', {minimumIntegerDigits: 1 + k %% 21, maximumFractionDigits: %d});
				}
				%s`, (g*20+i)%101, goIntlCheck())
				if v, err := vm.Eval(src, slowjs.EvalGlobal); err != nil || v != "" {
					t.Errorf("goroutine %d: %v %v", g, v, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
