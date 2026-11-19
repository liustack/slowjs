// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"strings"
	"testing"
)

// fuzzRegExpFlags are the flags a bit of FuzzRegExp's flags byte adds, in
// the order of RegExp.prototype.flags. u and v together are a SyntaxError,
// which the fuzzer keeps a few of.
const fuzzRegExpFlags = "dgimsuvy"

// fuzzRegExpScript compiles the pattern P with the flags F and runs it on S
// every way a script can, then checks what the engine must keep: the source
// of a regexp compiles again to the same source and flags, and matches the
// same.
const fuzzRegExpScript = `(() => {
	const fail = (what) => { const e = new Error(what); e.name = '` + fuzzContract + `'; throw e }
	const re = new RegExp(P, F)
	const out = [re.source, re.flags, String(re)]
	const again = new RegExp(re.source, re.flags)
	if (again.source !== re.source || again.flags !== re.flags) fail('the source ' + JSON.stringify(re.source) + ' compiles again to ' + JSON.stringify(again.source))
	const first = re.exec(S)
	re.lastIndex = 0
	const second = again.exec(S)
	if (JSON.stringify(first) !== JSON.stringify(second)) fail('the source compiled again matches otherwise')
	if (first) out.push(first.index, first.length, first.groups && Object.keys(first.groups), first.indices && first.indices.length)
	re.lastIndex = 0
	out.push(re.test(S), re.lastIndex)
	re.lastIndex = 0
	out.push(S.search(re), S.replace(re, '[$&|$1|$<n>|$` + "`" + `|$\']'), S.replace(re, (...a) => a.length))
	out.push(S.split(re, 32).length, S.match(re))
	if (re.global) {
		let n = 0
		for (const m of S.matchAll(re)) if (++n > 4096) break
		out.push(n, S.replaceAll(re, '$$'))
	}
	re.lastIndex = S.length + 1
	out.push(re.exec(S))
	return JSON.stringify(out)
})()`

// FuzzRegExp compiles a pattern with flags that a byte picks and runs it on
// a subject: exec, test, search, replace, split, match, matchAll and
// replaceAll, as fuzzRegExpScript does. limit picks the memory limit.
func FuzzRegExp(f *testing.F) {
	type seed struct {
		p   string
		fl  byte
		s   string
		lim byte
	}
	const (
		g = 1 << 1
		i = 1 << 2
		u = 1 << 5
		v = 1 << 6
		y = 1 << 7
		d = 1 << 0
	)
	for _, c := range []seed{
		{`a+b*`, g, "aaabbb ab", 0},
		{`(?<n>\d+)-(\d+)`, g | d, "12-34 5-6", 0},
		{`^\p{L}+$`, u, "héllo", 0},
		{`[\p{L}--[a-z]]+`, v, "abcÀÉ", 0},
		{`[[a-z]&&[aeiou]]`, v | g, "education", 0},
		{`\p{RGI_Emoji}`, v, "👍🏽x", 0},
		{`[\q{abc|d}x]`, v | i, "ABC", 0},
		{`(a|ab)(c|bcd)(d*)`, 0, "abcd", 0},
		{`(?=(a+))a*b\1`, 0, "baaabac", 0},
		{`(?<=\$)\d+(\.\d*)?`, g, "$10.53 $2", 0},
		{`(?i:a)b`, 0, "Ab ab AB", 0},
		{`(a*)*b`, 0, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 0},
		{`(x+x+)+y`, 0, "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", 0},
		{`\b\w+\b`, g | y, "one two", 0},
		{`.`, g | u, "\U0001F600\xed\xa0\x80", 0},
		{`\k<a>(?<a>x)`, 0, "xx", 0},
		{`(?<a>x)|(?<a>y)`, g, "xy", 0},
		{`a{2,3}?`, g, "aaaa", 3},
		{`[^]`, g | i | u | y, "Σσς", 2},
	} {
		f.Add(c.p, c.fl, c.s, c.lim)
	}
	fuzzNativeSeeds(2, func(limit byte, rest []byte) {
		if len(rest) == 0 {
			return
		}
		p, s, _ := strings.Cut(string(rest[1:]), "\x00")
		f.Add(p, rest[0], s, limit)
	})
	f.Fuzz(func(t *testing.T, pattern string, flagBits byte, subject string, limit byte) {
		if len(pattern)+len(subject) > fuzzMaxInput {
			return
		}
		var flags []byte
		for b := range len(fuzzRegExpFlags) {
			if flagBits&(1<<b) != 0 {
				flags = append(flags, fuzzRegExpFlags[b])
			}
		}
		m := newFuzzVM(t, int(limit))
		m.set("P", pattern)
		m.set("F", string(flags))
		m.set("S", subject)
		m.eval("the regexp script", fuzzRegExpScript, EvalGlobal)
		m.close()
	})
}
