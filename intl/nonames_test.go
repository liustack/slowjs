// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build quickjs_intl_nonames

package intl

import "testing"

// Built with quickjs_intl_nonames, Intl.DisplayNames has no names: of
// answers what the option fallback says, the code or undefined, and nothing
// throws but for a code that is not one.
func TestDisplayNamesWithoutNames(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`new Intl.DisplayNames('en', {type: 'region'}).of('US')`, "US"},
		{`String(new Intl.DisplayNames('en', {type: 'region', fallback: 'none'}).of('US'))`, "undefined"},
		{`new Intl.DisplayNames('de', {type: 'region', style: 'short'}).of('BU')`, "MM"},
		{`new Intl.DisplayNames('de', {type: 'language'}).of('zh-hant-tw')`, "zh-Hant-TW"},
		{`new Intl.DisplayNames('fr', {type: 'currency'}).of('eur')`, "EUR"},
		{`new Intl.DisplayNames('ja', {type: 'dateTimeField'}).of('weekday')`, "weekday"},
		{`new Intl.DisplayNames('en', {type: 'script'}).of('Latn')`, "Latn"},
		{`new Intl.DisplayNames('en', {type: 'calendar'}).of('gregory')`, "gregory"},
		{`try { new Intl.DisplayNames('en', {type: 'region'}).of('USA') } catch (e) { e.name }`, "RangeError"},
		{`new Intl.DisplayNames('de-AT-u-ca-buddhist', {type: 'region'}).resolvedOptions().locale`, "de-AT"},
	})
}
