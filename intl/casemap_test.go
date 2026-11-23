// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/tifajs/intl"

import "testing"

// A lone surrogate has combining class 0 and is neither cased nor case
// ignorable, so it ends every scan of the case rules of Turkish, Lithuanian
// and Greek, from both sides: a text with lone surrogates maps as its
// pieces between them map alone, the surrogates kept as they are. Random
// texts of letters, marks and surrogates hold the mapping of the whole text
// to the mapping of its pieces, which reach Go with no lone surrogate.
func TestCaseLoneSurrogatesAsPieces(t *testing.T) {
	vm := newVM(t)
	got := eval(t, vm, `(function () {
		let seed = 1;
		const rnd = (n) => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed % n; };
		const alpha = ['\ud800', '\udc00', '\udbff', '\udfff', '\ud800', '\udc00', 'i', 'I', 'İ', 'ı',
			'̇', '̀', '́', '̣', 'ͅ', '̈', 'j', 'į', 'Ì', 'ά', 'ή',
			'ι', 'υ', 'Σ', 'σ', 'ς', 'ϊ', 'ἀ', ' ', 'a', '.', '😀'];
		const lone = /([\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff])/;
		const hex = (s) => Array.from(s, (c) => c.codePointAt(0).toString(16)).join(' ');
		const bad = [];
		let tried = 0;
		for (const lang of ['tr', 'az', 'lt', 'el', 'tr-TR']) {
			for (let k = 0; k < 3000; k++) {
				let s = '';
				const n = 1 + rnd(k % 10 === 0 ? 200 : 12);
				for (let j = 0; j < n; j++) s += alpha[rnd(alpha.length)];
				if (!lone.test(s)) continue;
				for (const f of ['toLocaleUpperCase', 'toLocaleLowerCase']) {
					tried++;
					const got = s[f](lang);
					const want = s.split(lone).map((p, i) => i % 2 ? p : p[f](lang)).join('');
					if (got !== want) bad.push(lang + ' ' + f + ' [' + hex(s) + ']: [' + hex(got) + '], want [' + hex(want) + ']');
				}
			}
		}
		return tried > 10000 ? bad.slice(0, 5).join('\n') : 'only ' + tried + ' texts tried';
	})()`)
	if got != "" {
		t.Error(got)
	}
}
