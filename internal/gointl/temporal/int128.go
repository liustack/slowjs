package temporal

import (
	"math"
	"math/bits"
)

// An int128 is Rust's i128, which temporal_rs counts nanoseconds in: two's
// complement, and wrapping on overflow as Rust's release builds do.
type int128 struct {
	hi int64
	lo uint64
}

func i128(v int64) int128 { return int128{hi: v >> 63, lo: uint64(v)} }

func (a int128) add(b int128) int128 {
	lo, carry := bits.Add64(a.lo, b.lo, 0)
	return int128{hi: a.hi + b.hi + int64(carry), lo: lo}
}

func (a int128) sub(b int128) int128 {
	lo, borrow := bits.Sub64(a.lo, b.lo, 0)
	return int128{hi: a.hi - b.hi - int64(borrow), lo: lo}
}

func (a int128) neg() int128 { return int128{}.sub(a) }

func (a int128) isNeg() bool { return a.hi < 0 }

func (a int128) isZero() bool { return a.hi == 0 && a.lo == 0 }

// sign is -1, 0 or 1.
func (a int128) sign() int {
	switch {
	case a.hi < 0:
		return -1
	case a.isZero():
		return 0
	}
	return 1
}

func (a int128) cmp(b int128) int {
	switch {
	case a.hi < b.hi:
		return -1
	case a.hi > b.hi:
		return 1
	case a.lo < b.lo:
		return -1
	case a.lo > b.lo:
		return 1
	}
	return 0
}

func (a int128) abs() int128 {
	if a.isNeg() {
		return a.neg()
	}
	return a
}

// mul is the wrapping product.
func (a int128) mul(b int128) int128 {
	hi, lo := bits.Mul64(a.lo, b.lo)
	hi += uint64(a.hi)*b.lo + a.lo*uint64(b.hi)
	return int128{hi: int64(hi), lo: lo}
}

func (a int128) mul64(b int64) int128 { return a.mul(i128(b)) }

// uint128 magnitude helpers.

type uint128 struct{ hi, lo uint64 }

func (a int128) magnitude() uint128 {
	m := a.abs()
	return uint128{uint64(m.hi), m.lo}
}

func (u uint128) cmp(v uint128) int {
	switch {
	case u.hi < v.hi:
		return -1
	case u.hi > v.hi:
		return 1
	case u.lo < v.lo:
		return -1
	case u.lo > v.lo:
		return 1
	}
	return 0
}

// divmod is unsigned division.
func (u uint128) divmod(v uint128) (q, r uint128) {
	if v.hi == 0 {
		// Two steps of 128-by-64 division.
		var r64 uint64
		q.hi, r64 = bits.Div64(0, u.hi, v.lo)
		q.lo, r64 = bits.Div64(r64, u.lo, v.lo)
		return q, uint128{0, r64}
	}
	// Shift and subtract, one bit at a time: the quotient has at most 64
	// bits.
	n := bits.LeadingZeros64(v.hi)
	rem := u
	var quo uint64
	for i := n; i >= 0; i-- {
		d := v.shl(uint(i))
		if rem.cmp(d) >= 0 {
			rem = rem.sub(d)
			quo |= 1 << uint(i)
		}
	}
	return uint128{0, quo}, rem
}

func (u uint128) shl(n uint) uint128 {
	switch {
	case n == 0:
		return u
	case n >= 64:
		return uint128{u.lo << (n - 64), 0}
	}
	return uint128{u.hi<<n | u.lo>>(64-n), u.lo << n}
}

func (u uint128) sub(v uint128) uint128 {
	lo, borrow := bits.Sub64(u.lo, v.lo, 0)
	return uint128{u.hi - v.hi - borrow, lo}
}

func (u uint128) signed(neg bool) int128 {
	v := int128{int64(u.hi), u.lo}
	if neg {
		return v.neg()
	}
	return v
}

// quo and rem are Rust's / and %: truncated toward zero, the remainder
// taking the dividend's sign.
func (a int128) quoRem(b int128) (int128, int128) {
	q, r := a.magnitude().divmod(b.magnitude())
	return q.signed(a.isNeg() != b.isNeg()), r.signed(a.isNeg())
}

func (a int128) quo(b int128) int128 { q, _ := a.quoRem(b); return q }

func (a int128) rem(b int128) int128 { _, r := a.quoRem(b); return r }

// divEuclid and remEuclid are Rust's div_euclid and rem_euclid: the
// remainder is never negative.
func (a int128) divEuclid(b int128) int128 {
	q, r := a.quoRem(b)
	if r.isNeg() {
		if b.isNeg() {
			return q.add(i128(1))
		}
		return q.sub(i128(1))
	}
	return q
}

func (a int128) remEuclid(b int128) int128 {
	r := a.rem(b)
	if r.isNeg() {
		return r.add(b.abs())
	}
	return r
}

// int64 is Rust's `as i64`: the low 64 bits.
func (a int128) int64() int64 { return int64(a.lo) }

// fitsInt64 is whether i64::try_from succeeds.
func (a int128) fitsInt64() bool { return a.hi == int64(a.lo)>>63 }

// float64 is Rust's `as f64`: the nearest double, ties to even.
func (a int128) float64() float64 {
	m := a.magnitude()
	var f float64
	if m.hi == 0 {
		f = float64(m.lo)
	} else {
		// Keep the top 64 bits, with the bits shifted out folded into the
		// lowest as a sticky bit: a double's 53 bits round the same.
		shift := uint(64 - bits.LeadingZeros64(m.hi))
		top := m.hi<<(64-shift) | m.lo>>shift
		if m.lo<<(64-shift) != 0 {
			top |= 1
		}
		f = math.Ldexp(float64(top), int(shift))
	}
	if a.isNeg() {
		return -f
	}
	return f
}

// int128FromFloat is Rust's `as i128`: truncated, saturating at the ends,
// NaN being 0.
func int128FromFloat(f float64) int128 {
	switch {
	case math.IsNaN(f):
		return int128{}
	case f >= 0x1p127:
		return int128{hi: math.MaxInt64, lo: math.MaxUint64}
	case f <= -0x1p127:
		return int128{hi: math.MinInt64}
	}
	t := math.Abs(math.Trunc(f))
	var u uint128
	if t < 0x1p64 {
		u = uint128{0, uint64(t)}
	} else {
		frac, exp := math.Frexp(t)
		mant := uint64(frac * 0x1p53)
		u = uint128{0, mant}.shl(uint(exp - 53))
	}
	return u.signed(f < 0)
}

// int64FromFloat is Rust's `as i64` from a double: truncated and
// saturating, NaN being 0.
func int64FromFloat(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= 0x1p63:
		return math.MaxInt64
	case f <= -0x1p63:
		return math.MinInt64
	}
	return int64(f)
}

// uint64FromFloat is Rust's `as u64` from a double.
func uint64FromFloat(f float64) uint64 {
	switch {
	case math.IsNaN(f) || f <= 0:
		return 0
	case f >= 0x1p64:
		return math.MaxUint64
	}
	return uint64(f)
}
