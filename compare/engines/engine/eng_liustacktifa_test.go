// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build eng_liustacktifa

package engine

import (
	"os"
	"strings"
	"testing"

	"github.com/liustack/slowjs"
)

// emotionHash is murmur2 of @emotion/hash 0.9.2 (MIT, Copyright (c) Emotion
// team and other contributors), as its dist files have it, without comments.
const emotionHash = `function emotionHash(str) {
  var h = 0;
  var k,
      i = 0,
      len = str.length;
  for (; len >= 4; ++i, len -= 4) {
    k = str.charCodeAt(i) & 0xff | (str.charCodeAt(++i) & 0xff) << 8 | (str.charCodeAt(++i) & 0xff) << 16 | (str.charCodeAt(++i) & 0xff) << 24;
    k = (k & 0xffff) * 0x5bd1e995 + ((k >>> 16) * 0xe995 << 16);
    k ^= k >>> 24;
    h = (k & 0xffff) * 0x5bd1e995 + ((k >>> 16) * 0xe995 << 16) ^ (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  }
  switch (len) {
    case 3: h ^= (str.charCodeAt(i + 2) & 0xff) << 16;
    case 2: h ^= (str.charCodeAt(i + 1) & 0xff) << 8;
    case 1: h ^= str.charCodeAt(i) & 0xff;
      h = (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  }
  h ^= h >>> 13;
  h = (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  return ((h ^ h >>> 15) >>> 0).toString(36);
}`

func TestMurmur2HookIsTheHashOfEmotion(t *testing.T) {
	vm, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	if err := registerHash(vm); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.Eval(emotionHash, slowjs.EvalGlobal); err != nil {
		t.Fatal(err)
	}
	got, err := vm.Eval(`(() => {
  let seed = 7
  const next = (n) => (seed = (seed * 1103515245 + 12345) % 2147483648) % n
  const strings = ['', 'a', 'ab', 'abc', 'abcd', 'abcde', 'color:red;', 'éÿĀ', '😀x', 'x\ud83d', '\udc00\ud800']
  for (let n = 0; n < 500; n++) {
    let s = ''
    const length = next(80)
    for (let i = 0; i < length; i++) {
      const kind = next(4)
      s += String.fromCharCode(kind === 0 ? next(0x80) : kind === 1 ? next(0x10000) : kind === 2 ? 0xd800 + next(0x800) : 32 + next(95))
    }
    strings.push(s)
  }
  const wrong = strings.filter((s) => __tifa_murmur2__(s) !== emotionHash(s))
  return strings.length + ' ' + wrong.length
})()`, slowjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if got != "511 0" {
		t.Errorf("strings and those hashed otherwise than @emotion/hash does: %s", got)
	}
}

func TestHashInGoChangesOnlyEmotionHash(t *testing.T) {
	const fn = `function sm(e){for(var t=0,n,r=0,i=e.length;i>=4;++r,i-=4)n=e.charCodeAt(r)&255|(e.charCodeAt(++r)&255)<<8;return t}`
	out, n := hashInGo("var a=1;" + fn + "var b=2;")
	if n != 1 {
		t.Fatalf("changed %d functions, want 1", n)
	}
	if !strings.Contains(out, `function sm(e){var tifaHash="function"===typeof __tifa_murmur2__?__tifa_murmur2__(e):void 0;if(void 0!==tifaHash)return tifaHash;for(var t=0,n,r=0,`) {
		t.Errorf("rewritten function: %s", out)
	}
	if out, n := hashInGo("function f(e){return e.length>=4}"); n != 0 || out != "function f(e){return e.length>=4}" {
		t.Errorf("another function changed: %d %s", n, out)
	}
}

// The MUI bundle of the comparison has one @emotion/hash, the others none.
func TestHashInGoFindsTheHashOfTheMUIBundle(t *testing.T) {
	for name, want := range map[string]int{"mui": 1, "shop": 0, "bench": 0} {
		b, err := os.ReadFile("../tifa/bundles/" + name + ".js")
		if err != nil {
			t.Fatal(err)
		}
		if _, n := hashInGo(string(b)); n != want {
			t.Errorf("%s: %d functions changed, want %d", name, n, want)
		}
	}
}
