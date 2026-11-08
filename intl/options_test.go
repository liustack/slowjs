// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// A number option is checked against its range before it is floored, as
// ECMA-402's DefaultNumberOption has it: 3.9 is out of 1 to 3, and 1.5 is
// in and is 1. The answers are Node 24's.
func TestNumberOptionRange(t *testing.T) {
	for _, c := range []struct{ ctor, option, value, want string }{
		{"DateTimeFormat", "fractionalSecondDigits", "3.9", "RangeError"},
		{"DateTimeFormat", "fractionalSecondDigits", "0.9", "RangeError"},
		{"DateTimeFormat", "fractionalSecondDigits", "3", "3"},
		{"DateTimeFormat", "fractionalSecondDigits", "1.5", "1"},
		{"NumberFormat", "minimumIntegerDigits", "21.9", "RangeError"},
		{"NumberFormat", "minimumIntegerDigits", "0.9", "RangeError"},
		{"NumberFormat", "minimumIntegerDigits", "21", "21"},
		{"NumberFormat", "minimumIntegerDigits", "1.5", "1"},
		{"NumberFormat", "maximumFractionDigits", "100.5", "RangeError"},
		{"NumberFormat", "maximumFractionDigits", "100", "100"},
		{"NumberFormat", "minimumFractionDigits", "-0.1", "RangeError"},
		{"NumberFormat", "minimumFractionDigits", "0.5", "0"},
		{"NumberFormat", "maximumSignificantDigits", "21.1", "RangeError"},
		{"NumberFormat", "minimumSignificantDigits", "0.5", "RangeError"},
		{"NumberFormat", "roundingIncrement", "5000.5", "RangeError"},
		{"NumberFormat", "roundingIncrement", "1.5", "1"},
		{"PluralRules", "minimumIntegerDigits", "21.9", "RangeError"},
		{"PluralRules", "maximumFractionDigits", "100.1", "RangeError"},
		{"PluralRules", "maximumSignificantDigits", "21.5", "RangeError"},
		{"PluralRules", "minimumFractionDigits", "2.7", "2"},
	} {
		vm := newVM(t)
		js := fmt.Sprintf(`new Intl.%s('en', {%s: %s}).resolvedOptions().%[2]s`, c.ctor, c.option, c.value)
		v, err := vm.Eval(js, quickjs.EvalGlobal)
		got := fmt.Sprint(v)
		if err != nil {
			got, _, _ = strings.Cut(err.Error(), ":")
		}
		if got != c.want {
			t.Errorf("%s: got %s (%v), want %s", js, got, err, c.want)
		}
	}
}
