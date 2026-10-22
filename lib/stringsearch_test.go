// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"math/rand"
	"runtime"
	"testing"
	"unsafe"
)

// stringSearchJS checks every method whose loop calls stringFind, and
// split, replace and replaceAll, against a search written with charCodeAt
// alone, on strings of 8-bit and of 16-bit
// characters, ropes and slices, found and not found, at each position
// argument, at the ends, more than once, and longer than stringFindRoom.
const stringSearchJS = `(() => {
  function find(s, v, from, back) {
    const n = s.length, m = v.length
    const at = (i) => { for (let k = 0; k < m; k++) if (s.charCodeAt(i + k) !== v.charCodeAt(k)) return false; return true }
    if (back) { for (let i = Math.min(from, n - m); i >= 0; i--) if (at(i)) return i; return -1 }
    for (let i = Math.max(from, 0); i + m <= n; i++) if (at(i)) return i
    return -1
  }
  function split(s, v) {
    const out = []; let i = 0
    if (v.length === 0) { for (let k = 0; k < s.length; k++) out.push(s[k]); return out }
    for (;;) { const j = find(s, v, i, false); if (j < 0) break; out.push(s.slice(i, j)); i = j + v.length }
    out.push(s.slice(i)); return out
  }
  function replaceAll(s, v, w) { return split(s, v).join(w) }
  const parts = ['a', 'ab', '<!--$!-->', '<!-- -->', '<', '!', '\xe9', '\xff', '\u4e00', '\u6161', '\u0161', 'aa', '\x00', '-->', '\u6100']
  let seed = 7
  const rnd = (n) => (seed = (seed * 1103515245 + 12345) % 2147483648) % n
  const make = (k) => { let s = ''; for (let i = 0; i < k; i++) s += parts[rnd(parts.length)]; return s }
  const hays = [], needles = ['', 'a', 'ab', '<!--$!-->', '\u4e00', '\u6161a', 'a\u6100', '\xe9\xff', 'x', '\x00a', 'a'.repeat(70), '\u4e00'.repeat(70), 'ab'.repeat(40) + '\xe9']
  for (let k = 0; k < 25; k++) { hays.push(make(rnd(40))); needles.push(make(1 + rnd(3))) }
  hays.push('', 'a'.repeat(200), 'a'.repeat(69) + '\u4e00' + 'a'.repeat(71), ('ab'.repeat(40) + '\xe9').repeat(3), ('\u4e00'.repeat(70) + 'a').repeat(2))
  hays.push(('x' + '\u4e00').slice(1) + 'ab', ('\u4e00' + 'abab').slice(1), 'q'.repeat(1000) + '<!--$!-->' + 'q'.repeat(1000))
  let n = 0, bad = []
  const check = (what, got, want) => { n++; if (JSON.stringify(got) !== JSON.stringify(want) && bad.length < 20) bad.push(what + ': ' + JSON.stringify(got) + ' want ' + JSON.stringify(want)) }
  for (const h0 of hays) for (const v of needles) {
    for (const h of [h0, h0 + v, v + h0 + v + h0]) {
      const L = h.length
      const tag = JSON.stringify([h.slice(0, 30), L, v.slice(0, 10), v.length])
      check('includes ' + tag, h.includes(v), find(h, v, 0, false) >= 0)
      check('indexOf ' + tag, h.indexOf(v), find(h, v, 0, false))
      check('lastIndexOf ' + tag, h.lastIndexOf(v), find(h, v, L, true))
      check('startsWith ' + tag, h.startsWith(v), find(h.slice(0, v.length), v, 0, false) === 0)
      check('endsWith ' + tag, h.endsWith(v), L >= v.length && find(h, v, L - v.length, false) === L - v.length)
      for (const p of [-1, 0, 1, 2, 7, L >> 1, L - v.length, L - 1, L, L + 3]) {
        check('includes at ' + p + ' ' + tag, h.includes(v, p), find(h, v, Math.min(Math.max(p, 0), L), false) >= 0)
        check('indexOf at ' + p + ' ' + tag, h.indexOf(v, p), find(h, v, Math.min(Math.max(p, 0), L), false))
        check('lastIndexOf at ' + p + ' ' + tag, h.lastIndexOf(v, p), find(h, v, Math.max(p, 0), true))
        const q = Math.min(Math.max(p, 0), L)
        check('startsWith at ' + p + ' ' + tag, h.startsWith(v, p), q + v.length <= L && find(h, v, q, false) === q)
        check('endsWith at ' + p + ' ' + tag, h.endsWith(v, p), q - v.length >= 0 && find(h.slice(0, q), v, q - v.length, false) === q - v.length)
      }
      check('split ' + tag, h.split(v), split(h, v))
      if (v.length > 0) {
        check('replace ' + tag, h.replace(v, '#'), find(h, v, 0, false) < 0 ? h : h.slice(0, find(h, v, 0, false)) + '#' + h.slice(find(h, v, 0, false) + v.length))
        check('replaceAll ' + tag, h.replaceAll(v, '#'), replaceAll(h, v, '#'))
      }
    }
  }
  return bad.length ? bad.join('\n') : 'ok ' + n
})()`

// The searches give what a search with charCodeAt alone gives.
func TestStringSearch(t *testing.T) {
	got := eval(t, "stringsearch.js", stringSearchJS)
	if len(got) < 3 || got[:3] != "ok " {
		t.Fatal(got)
	}
	t.Log(got)
}

// fakeString is a JSString of units, of 16-bit characters with wide, with
// the room around it that stringFind reads nothing of.
type fakeString struct {
	buf []uint64
	p   uintptr
}

func newFakeString(units []uint16, wide bool) *fakeString {
	f := &fakeString{buf: make([]uint64, 4+len(units))}
	f.p = uintptr(unsafe.Pointer(&f.buf[1]))
	head := (*uint32)(unsafe.Pointer(f.p + jsonStrLenOff))
	*head |= uint32(len(units)) << jsonStrLenShift & jsonStrLenMask
	if wide {
		*(*uint32)(unsafe.Pointer(f.p + jsonStrWideOff)) |= jsonStrWideMask
	}
	for k, c := range units {
		if wide {
			*(*uint16)(unsafe.Pointer(f.p + jsonStrData + 2*uintptr(k))) = c
		} else {
			*(*byte)(unsafe.Pointer(f.p + jsonStrData + uintptr(k))) = byte(c)
		}
	}
	return f
}

// stringFind gives the index the loop it stands for would stop at, or i at
// stop, for each width of the two strings, 16-bit strings that hold only characters
// below 0x100, characters whose bytes match at odd offsets, needles longer
// than stringFindRoom, and both directions.
func TestStringFindWidths(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	narrow := []uint16{0x00, 0x61, 0x62, 0xe9}
	wideSet := []uint16{0x00, 0x61, 0x62, 0xe9, 0x6100, 0x6161, 0x0161, 0x6200}
	pick := func(set []uint16, n int) []uint16 {
		s := make([]uint16, n)
		for k := range s {
			s[k] = set[r.Intn(len(set))]
		}
		return s
	}
	for round := 0; round < 40000; round++ {
		wide1, wide2 := r.Intn(2) == 1, r.Intn(2) == 1
		set1, set2 := narrow, narrow
		if wide1 {
			set1 = wideSet
		}
		if wide2 {
			set2 = wideSet
		}
		n2 := r.Intn(4)
		if r.Intn(20) == 0 {
			n2 = stringFindRoom - 2 + r.Intn(5)
		}
		n1 := n2 + r.Intn(12)
		if n2 > 4 {
			n1 = n2 + r.Intn(3)
		}
		h, v := pick(set1, n1), pick(set2, n2)
		if n2 > 0 && n1 >= n2 && r.Intn(3) == 0 {
			at := r.Intn(n1 - n2 + 1)
			for k := range v {
				if wide1 || v[k] < 0x100 {
					h[at+k] = v[k]
				}
			}
		}
		top := int32(n1 - n2)
		i, stop, inc := int32(r.Intn(int(top)+1)), top, int32(1)
		if r.Intn(2) == 0 {
			stop, inc = int32(r.Intn(int(i)+1)), -1
		}
		want := int32(-1)
		for k := i; ; k += inc {
			match := true
			for j := range v {
				if h[int(k)+j] != v[j] {
					match = false
					break
				}
			}
			if match {
				want = k
				break
			}
			if k == stop {
				break
			}
		}
		if i == stop {
			want = i
		}
		f1, f2 := newFakeString(h, wide1), newFakeString(v, wide2)
		if got := stringFind(f1.p, f2.p, i, stop, inc); got != want {
			t.Fatalf("stringFind(%x wide %v, %x wide %v, %d, %d, %d) = %d, want %d", h, wide1, v, wide2, i, stop, inc, got, want)
		}
		runtime.KeepAlive(f1)
		runtime.KeepAlive(f2)
	}
}

// From past stop, stringFind finds nothing and reads nothing. At stop,
// it leaves the one comparison to the loop.
func TestStringFindPastStop(t *testing.T) {
	h, v := newFakeString([]uint16{'a', 'b'}, false), newFakeString([]uint16{'b', 'c', 'd'}, false)
	if got := stringFind(h.p, v.p, 0, -1, 1); got != -1 {
		t.Errorf("forward past stop: %d", got)
	}
	if got := stringFind(h.p, h.p, 0, 1, -1); got != -1 {
		t.Errorf("backward past stop: %d", got)
	}
	x := newFakeString([]uint16{'x'}, false)
	if got := stringFind(h.p, x.p, 1, 1, 1); got != 1 {
		t.Errorf("at stop: %d", got)
	}
	runtime.KeepAlive(h)
	runtime.KeepAlive(v)
	runtime.KeepAlive(x)
}
