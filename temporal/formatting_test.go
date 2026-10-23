// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/intl"
)

// The focused corpora use the Intl recorder's error names. Error wording is
// not part of the Intl bridge's contract.
func checkFormattingCorpus(t *testing.T, name string, vm *slowjs.VM) {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Browser string
		Cases   [][2]string
	}
	if err := json.Unmarshal(b, &corpus); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(corpus.Browser, "Chromium 156.") || len(corpus.Cases) == 0 {
		t.Fatal("missing Chromium 156 corpus")
	}
	for _, c := range corpus.Cases {
		got, err := vm.Eval(c[0], slowjs.EvalGlobal)
		value := ""
		if err != nil {
			e, ok := err.(*slowjs.Error)
			if !ok {
				t.Fatal(err)
			}
			value = "ERR:" + e.Name
		} else {
			value, _ = got.(string)
		}
		if value != c[1] {
			t.Errorf("%s\ngot  %q\nwant %q", c[0], value, c[1])
		}
	}
	t.Logf("%s: %d cases, %s", name, len(corpus.Cases), corpus.Browser)
}

func TestDurationLocaleString(t *testing.T) {
	checkFormattingCorpus(t, "duration-locale", newVM(t))
	t.Run("without Intl registration", func(t *testing.T) {
		vm, err := slowjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		defer vm.Close()
		if err := Register(vm); err != nil {
			t.Fatal(err)
		}
		checkFormattingCorpus(t, "duration-locale", vm)
	})
}

func TestDateTimeFormatTemporal(t *testing.T) {
	checkFormattingCorpus(t, "datetime-format", newVM(t))
}

// Both registration and first-use orders must share the same private slots.
func TestDateTimeFormatTemporalInstallOrder(t *testing.T) {
	for _, temporalFirst := range []bool{false, true} {
		for _, first := range []string{"new Intl.DateTimeFormat('en')", "Temporal.PlainDate.from('2024-01-05')"} {
			vm, err := slowjs.NewVM()
			if err != nil {
				t.Fatal(err)
			}
			if temporalFirst {
				if err := Register(vm); err != nil {
					t.Fatal(err)
				}
				if err := intl.Register(vm); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := intl.Register(vm); err != nil {
					t.Fatal(err)
				}
				if err := Register(vm); err != nil {
					t.Fatal(err)
				}
			}
			eval(t, vm, first)
			eval(t, vm, `var f = new Intl.DateTimeFormat('en'); var d = Temporal.PlainDate.from('2024-01-05');`)
			vm.RunGC()
			evalTests(t, vm, [][2]string{{`f.format(d)`, "1/5/2024"}})
			if err := vm.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDateTimeFormatTemporalTamper(t *testing.T) {
	const setup = `const f = new Intl.DateTimeFormat('en');
        const format = f.format, parts = f.formatToParts, range = f.formatRange, rangeParts = f.formatRangeToParts;
        const apply = Reflect.apply;`
	const probe = `(() => {
        const a = Temporal.PlainDate.from('2024-01-05'), b = Temporal.PlainDate.from('2024-01-07');
        let result = format(a) + '|' + apply(range, f, [a,b]);
        const methods = [parts,rangeParts];
        for (let m=0;m<methods.length;m++) {
            const method = methods[m];
            const p = apply(method, f, [a,b]);
            for (let i=0;i<p.length;i++) result += '|' + p[i].type + ':' + p[i].value + ':' + (p[i].source || '');
        }
        return result;
    })()`
	clean := newVM(t)
	eval(t, clean, setup)
	want := eval(t, clean, probe)
	for name, tamper := range tampers {
		for _, installed := range []bool{false, true} {
			t.Run(fmt.Sprint(name, installed), func(t *testing.T) {
				vm := newVM(t)
				eval(t, vm, setup)
				if installed {
					eval(t, vm, probe)
				}
				eval(t, vm, tamper+";undefined")
				if got := eval(t, vm, probe); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestDateTimeFormatTemporalEviction(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{{`(() => {
        const f = new Intl.DateTimeFormat('en'), d = Temporal.PlainDate.from('2024-01-05');
        const want = f.format(d) + '|' + f.format(Temporal.PlainTime.from('13:05'));
        for (let i=0;i<1100;i++) new Intl.NumberFormat('en', {
            minimumFractionDigits:i%101, maximumFractionDigits:i%101, minimumIntegerDigits:1+Math.floor(i/101)
        });
        return want === f.format(d) + '|' + f.format(Temporal.PlainTime.from('13:05')) ? 'same' : 'changed';
    })()`, "same"}})
}
