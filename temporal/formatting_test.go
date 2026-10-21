// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// The focused corpora use the Intl recorder's error names. Error wording is
// not part of the Intl bridge's contract.
func checkFormattingCorpus(t *testing.T, name string, vm *quickjs.VM) {
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
		got, err := vm.Eval(c[0], quickjs.EvalGlobal)
		value := ""
		if err != nil {
			e, ok := err.(*quickjs.Error)
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
		vm, err := quickjs.NewVM()
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
