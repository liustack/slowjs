// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package idna

import (
	"cmp"
	"math"
	"slices"
)

// encode encodes a string as RFC 3492 section 6.3 specifies, and prepends
// prefix to the result. It is written here and is not the encoder of x/net.
//
// The algorithm of RFC 3492 goes over the whole label once for each code point
// that is not ASCII, which is the square of the label's length when they all
// differ. This gives the same output from the same deltas, but counts what the
// scan of the RFC counts with a Fenwick tree over the positions: the delta at
// an occurrence of the code point m is the number of code points below m between
// it and the occurrence before (or, for the first, since the last one of the
// code point before m), which is a difference of two prefix counts. The work is
// O(n log n) in the length of the label.
func encode(prefix, s string) (string, error) {
	output := make([]byte, len(prefix), len(prefix)+1+2*len(s))
	copy(output, prefix)
	var runes []rune
	b := int32(0)
	for _, r := range s {
		if r == 0xfffd {
			return s, &labelError{s, "A3"}
		}
		if r < 0x80 {
			b++
			output = append(output, byte(r))
		}
		runes = append(runes, r)
	}
	h := b
	if b > 0 {
		output = append(output, '-')
	}
	// The positions of the code points that are not ASCII, by code point and
	// then by position.
	var rest []int32
	for i, r := range runes {
		if r >= 0x80 {
			rest = append(rest, int32(i))
		}
	}
	slices.SortStableFunc(rest, func(x, y int32) int { return cmp.Compare(runes[x], runes[y]) })
	// tree counts the positions of the code points handled so far: ASCII first.
	tree := make([]int32, len(runes)+1)
	add := func(i int32) {
		for i++; int(i) < len(tree); i += i & -i {
			tree[i]++
		}
	}
	below := func(i int32) int64 { // how many handled positions are before i
		n := int64(0)
		for ; i > 0; i -= i & -i {
			n += int64(tree[i])
		}
		return n
	}
	for i, r := range runes {
		if r < 0x80 {
			add(int32(i))
		}
	}
	delta, n, bias := int64(0), initialN, initialBias
	for k := 0; k < len(rest); {
		m := runes[rest[k]]
		delta += int64(m-n) * int64(h+1)
		if delta > math.MaxInt32 {
			return "", punyError(s)
		}
		n = m
		handled := int64(h) // code points below m
		last := int64(0)    // handled positions before the occurrence before
		j := k
		for ; j < len(rest) && runes[rest[j]] == m; j++ {
			at := below(rest[j])
			delta += at - last
			if delta > math.MaxInt32 {
				return "", punyError(s)
			}
			last = at
			q := int32(delta)
			for k := base; ; k += base {
				t := k - bias
				if k <= bias {
					t = tmin
				} else if k >= bias+tmax {
					t = tmax
				}
				if q < t {
					break
				}
				output = append(output, encodeDigit(t+(q-t)%(base-t)))
				q = (q - t) / (base - t)
			}
			output = append(output, encodeDigit(q))
			bias = adapt(int32(delta), h+1, h == b)
			delta = 0
			h++
		}
		// The code points below m after its last occurrence, and one for the end
		// of the scan.
		delta = handled - last + 1
		if delta > math.MaxInt32 {
			return "", punyError(s)
		}
		for ; k < j; k++ {
			add(rest[k])
		}
		n++
	}
	return string(output), nil
}
