// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// A regexp whose ops take more than LRE_CHECK_RUN ticks gets gotos that read
// the interrupt flag in its code (lre_insert_checks, engine/libregexp.c),
// and every jump over them moves. Each long pattern here is written out
// where the short one beside it says the same with a count, which gets
// none: the two give the same exec, the same search, replace and split, for
// strings that match, that match late and that fail.
func TestRegExpLongRunsMatchTheSame(t *testing.T) {
	m := newTestVM(t)
	v, err := m.Eval(`(function () {
		var r = (s, n) => s.repeat(n), bad = [];
		var pairs = [
			// runs forward, 8-bit and 16-bit
			['^' + r('[ab]', 2500), '^[ab]{2500}', '', [r('a', 2500), r('ab', 1250) + 'y', r('a', 2499) + 'c']],
			[r('[ab]', 2500), '[ab]{2500}', '', ['x' + r('ab', 1250) + 'y', 'xy' + r('b', 2500)]],
			['^' + r('.', 5000), '^.{5000}', 'su', [r('中', 5000), r('😀', 5000), r('中', 4999)]],
			['^' + r('a', 500), '^a{500}', 'i', [r('A', 500), r('a', 250) + r('A', 250), r('A', 499) + 'B']],
			['^' + r('[ab]', 500), '^[ab]{500}', 'iu', [r('AB', 250) + '中', r('A', 499)]],
			// alternatives that fail late, and a jump over a long branch
			['^(?:' + r('[ab]', 2500) + 'c|' + r('[ab]', 2500) + 'd)', '^(?:[ab]{2500}c|[ab]{2500}d)', '',
				[r('a', 2500) + 'd', r('a', 2500) + 'c', r('a', 2500) + 'e']],
			['^(?:' + r('a', 5000) + ')?b', '^(?:a{5000})?b', '', [r('a', 5000) + 'b', 'b', r('a', 4999) + 'b']],
			['x|^' + r('[ab]', 2500) + '|y', 'x|^[ab]{2500}|y', '', [r('b', 2500), 'by', r('b', 10)]],
			// named groups and back references
			['^(?<n>' + r('a', 5000) + ')b\\k<n>', '^(?<n>a{5000})b\\k<n>', '', [r('a', 5000) + 'b' + r('a', 5000), r('a', 5000) + 'b' + r('a', 4999)]],
			['^(' + r('a', 5000) + ')b\\1', '^(a{5000})b\\1', 'i', [r('a', 5000) + 'b' + r('A', 5000)]],
			// lookarounds with long bodies
			['^(?=' + r('a', 5000) + ')a+', '^(?=a{5000})a+', '', [r('a', 5000), r('a', 4999)]],
			['^(?!' + r('a', 5000) + ')a+', '^(?!a{5000})a+', '', [r('a', 5000), r('a', 4999)]],
			['b(?<=^' + r('a', 5000) + 'b)', 'b(?<=^a{5000}b)', '', [r('a', 5000) + 'b', r('a', 4999) + 'b']],
			['b(?<!^' + r('a', 5000) + 'b)', 'b(?<!^a{5000}b)', '', [r('a', 5000) + 'b', r('a', 4999) + 'b']],
			['c(?<=^(' + r('[ab]', 2500) + ')c)', 'c(?<=^([ab]{2500})c)', '', [r('ab', 1250) + 'c']],
			// loops around a long body, greedy and lazy
			['^(?:' + r('[ab]', 2500) + ')+$', '^(?:[ab]{2500})+$', '', [r('ab', 3750), r('ab', 3750) + 'a']],
			['^(?:' + r('[ab]', 2500) + '){2,3}', '^(?:[ab]{2500}){2,3}', '', [r('ab', 3750), r('ab', 2000)]],
			['^(?:' + r('[ab]', 2500) + ')+?c', '^(?:[ab]{2500})+?c', '', [r('a', 5000) + 'c', r('a', 5001) + 'c']],
			['^(?:(a)|b' + r('[ab]', 5000) + ')*$', '^(?:(a)|b[ab]{5000})*$', '', ['ab' + r('a', 5000) + 'a', 'ab' + r('a', 4999)]],
			// multiline, sticky and global
			['^' + r('[ab]', 5000) + '$', '^[ab]{5000}$', 'm', ['x\n' + r('a', 5000) + '\ny']],
			[r('[ab]', 5000), '[ab]{5000}', 'g', [r('a', 15001)]],
			[r('[ab]', 5000), '[ab]{5000}', 'y', [r('a', 5000)]],
		];
		var show = (re, s) => {
			re.lastIndex = 0;
			var e = re.exec(s);
			return JSON.stringify([e, e && e.index, e && e.groups, re.lastIndex,
				s.search(re), s.replace(re, '<$&>').length, s.split(re).length]);
		};
		for (var [long, short, flags, inputs] of pairs) {
			var a = new RegExp(long, flags), b = new RegExp(short, flags);
			for (var s of inputs) {
				var x = show(a, s), y = show(b, s);
				if (x !== y)
					bad.push('/' + short + '/' + flags + ' on ' + s.length + ' units: ' + x.slice(0, 80) + ' != ' + y.slice(0, 80));
			}
		}
		// groups written out, which no count says the same as
		var g = new RegExp('^' + r('(a)(b)', 3000));
		var e = g.exec(r('ab', 3000));
		if (!e || e.length !== 6001 || e[1] !== 'a' || e[5999] !== 'a' || e[6000] !== 'b')
			bad.push('6000 groups: ' + (e && e.length));
		if (g.exec(r('ab', 2999) + 'a') !== null)
			bad.push('6000 groups: a match of a string one short');
		return bad.join('\n');
	})()`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Errorf("the long and the short patterns differ:\n%v", v)
	}
}
