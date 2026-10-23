// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command linkprobe uses every part of package intl, for TestLinksOnlyReadData
// to look at which of go-intl's packs it links.
package main

import (
	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/intl"
)

func main() {
	vm, err := slowjs.NewVM()
	if err != nil {
		panic(err)
	}
	defer vm.Close()
	if err := intl.Register(vm); err != nil {
		panic(err)
	}
	if _, err := vm.Eval(`[
		new Date(0).toLocaleString('de', {timeZone: 'Europe/Berlin', timeZoneName: 'long'}),
		new Intl.NumberFormat('ja', {style: 'currency', currency: 'JPY'}).format(1),
		new Intl.PluralRules('ar').select(3),
		new Intl.Collator('de').compare('a', 'b'),
		[...new Intl.Segmenter('ja', {granularity: 'word'}).segment('今日は')].length,
		new Intl.RelativeTimeFormat('fr').format(-1, 'day'),
		new Intl.ListFormat('es').format(['a', 'b']),
		new Intl.DisplayNames('zh', {type: 'region'}).of('US'),
		new Intl.DurationFormat('en').format({hours: 1}),
		new Intl.Locale('en-US').getTimeZones(),
		Intl.supportedValuesOf('timeZone').length,
		'i'.toLocaleUpperCase('tr'),
	].join()`, slowjs.EvalGlobal); err != nil {
		panic(err)
	}
}
