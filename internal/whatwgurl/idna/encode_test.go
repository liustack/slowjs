// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package idna

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// sameAsRFC fails t when encode and encodeRFC differ on s.
func sameAsRFC(t *testing.T, s string) {
	t.Helper()
	got, gerr := encode(acePrefix, s)
	want, werr := encodeRFC(acePrefix, s)
	if got != want || (gerr == nil) != (werr == nil) {
		t.Fatalf("encode(%+q) = %q, %v; the encoder of RFC 3492 gives %q, %v", s, got, gerr, want, werr)
	}
}

// TestEncodeRandom compares the encoder with the one of RFC 3492 on 200,000
// random labels, of runs of ASCII, repeated characters and distinct ones from
// every plane, most up to 64 characters long and one in a hundred up to 300, and twenty of
// 1,024, the longest a URL can give it.
func TestEncodeRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	pools := [][2]rune{{0x61, 0x7a}, {0x80, 0x7ff}, {0x800, 0xd7ff}, {0xe000, 0xfffc}, {0x10000, 0x10ffff}, {0x4e00, 0x4e10}}
	var b strings.Builder
	for i := 0; i < 200000; i++ {
		b.Reset()
		n := r.IntN(64) + 1
		if i%100 == 0 {
			n = r.IntN(300) + 1
		} else if i%10000 == 1 {
			n = 1024
		}
		for j := 0; j < n; j++ {
			p := pools[r.IntN(len(pools))]
			c := p[0] + r.Int32N(p[1]-p[0]+1)
			if !utf8.ValidRune(c) {
				c = 'x'
			}
			b.WriteRune(c)
		}
		sameAsRFC(t, b.String())
	}
}

// TestEncodeWPT compares the encoder with the one of RFC 3492 on every label of
// the inputs and outputs of the IDNA test files of the web-platform-tests, and
// on what each label that is Punycode decodes to.
func TestEncodeWPT(t *testing.T) {
	labels := 0
	for _, file := range []string{"toascii.json", "IdnaTestV2.json", "IdnaTestV2-removed.json"} {
		b, err := os.ReadFile("../testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var raw []json.RawMessage
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatal(err)
		}
		for _, r := range raw {
			if r[0] == '"' {
				continue
			}
			var c struct {
				Input  string
				Output *string
			}
			json.Unmarshal(r, &c)
			texts := []string{c.Input}
			if c.Output != nil {
				texts = append(texts, *c.Output)
			}
			for _, text := range texts {
				for _, l := range strings.Split(text, ".") {
					if l == "" {
						continue
					}
					sameAsRFC(t, l)
					labels++
					if len(l) > 4 && strings.EqualFold(l[:4], acePrefix) {
						if d, err := decode(l[4:]); err == nil {
							sameAsRFC(t, d)
							labels++
						}
					}
				}
			}
		}
	}
	if labels < 10000 {
		t.Fatalf("only %d labels: the files were not read", labels)
	}
}

// TestEncodeLinear checks the work of the encoder on the worst label a URL can
// give it, as long as whatwgurl.MaxDomainUnits and all different code points,
// against one of a quarter of the length: four times as long takes at most ten
// times as long, where the encoder of RFC 3492 took sixteen.
func TestEncodeLinear(t *testing.T) {
	distinct := func(n int) string {
		var b strings.Builder
		for r := rune(0x4e00); r < 0x4e00+rune(n); r++ {
			b.WriteRune(r)
		}
		return b.String()
	}
	const h = 1024
	long, short := distinct(h), distinct(h/4)
	best := func(s string) time.Duration {
		d := time.Hour
		for i := 0; i < 20; i++ {
			start := time.Now()
			for j := 0; j < 10; j++ {
				encode(acePrefix, s)
			}
			d = min(d, time.Since(start))
		}
		return d
	}
	for try := 0; ; try++ {
		l, s := best(long), best(short)
		if l <= 10*s {
			break
		}
		if try == 4 {
			t.Fatalf("%v for %d distinct code points, %v for %d: the work is not near linear", l/10, h, s/10, h/4)
		}
	}
}
