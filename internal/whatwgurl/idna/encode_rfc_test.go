// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package idna

// encodeRFC is the Punycode encoder of golang.org/x/net/idna v0.62.0, as it
// was, without the stop function: the loop of RFC 3492 section 6.3, which goes
// over the label once for each code point that is not ASCII. TestEncode
// checks encode against it.
//
// The "while h < length(input)" line in the specification becomes "for
// remaining != 0" in the Go code, because len(s) in Go is in bytes, not runes.
func encodeRFC(prefix, s string) (string, error) {
	output := make([]byte, len(prefix), len(prefix)+1+2*len(s))
	copy(output, prefix)
	delta, n, bias := int32(0), initialN, initialBias
	b, remaining := int32(0), int32(0)
	for _, r := range s {
		if r == 0xfffd {
			return s, &labelError{s, "A3"}
		}
		if r < 0x80 {
			b++
			output = append(output, byte(r))
		} else {
			remaining++
		}
	}
	h := b
	if b > 0 {
		output = append(output, '-')
	}
	overflow := false
	for remaining != 0 {
		m := int32(0x7fffffff)
		for _, r := range s {
			if m > r && r >= n {
				m = r
			}
		}
		delta, overflow = madd(delta, m-n, h+1)
		if overflow {
			return "", punyError(s)
		}
		n = m
		for _, r := range s {
			if r < n {
				delta++
				if delta < 0 {
					return "", punyError(s)
				}
				continue
			}
			if r > n {
				continue
			}
			q := delta
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
			bias = adapt(delta, h+1, h == b)
			delta = 0
			h++
			remaining--
		}
		delta++
		n++
	}
	return string(output), nil
}
