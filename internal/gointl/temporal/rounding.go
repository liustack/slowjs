package temporal

import (
	"math"
	"math/bits"
)

// Rounding to an increment, as temporal_rs's IncrementRounder does it over
// integers.

// roundIncrement is IncrementRounder::round: n rounded to a multiple of
// increment, the mode applied to n's magnitude.
func roundIncrement(n, increment int128, mode RoundingMode) int128 {
	positive := !n.isNeg()
	q := applyUnsignedRounding(n.abs(), increment, mode.unsigned(positive))
	if !positive {
		q = q.neg()
	}
	return mulSaturating(q, increment)
}

// roundIncrementAsIfPositive is IncrementRounder::round_as_if_positive.
func roundIncrementAsIfPositive(n, increment int128, mode RoundingMode) int128 {
	q := applyUnsignedRounding(n, increment, mode.unsigned(true))
	return mulSaturating(q, increment)
}

// applyUnsignedRounding is ApplyUnsignedRoundingMode on dividend ÷
// divisor, as the quotient.
func applyUnsignedRounding(dividend, divisor int128, m unsignedRoundingMode) int128 {
	r1 := dividend.divEuclid(divisor)
	r2 := r1.add(i128(1))
	rem := dividend.remEuclid(divisor)
	if rem.isZero() {
		return r1
	}
	switch m {
	case roundZero:
		return r1
	case roundInfinity:
		return r2
	}
	midway := divisor.divEuclid(i128(2))
	c := rem.cmp(midway)
	if c == 0 && !divisor.remEuclid(i128(2)).isZero() {
		c = -1
	}
	switch {
	case c < 0:
		return r1
	case c > 0:
		return r2
	}
	switch m {
	case roundHalfZero:
		return r1
	case roundHalfInfinity:
		return r2
	}
	if r1.remEuclid(i128(2)).isZero() {
		return r1
	}
	return r2
}

// applyUnsignedRoundingRatio is UnsignedRoundingMode::apply: the ratio
// dividend ÷ divisor, between r1 and r2, rounded to one of them, computed
// with both multiplied by divisor.
func applyUnsignedRoundingRatio(m unsignedRoundingMode, dividend, divisor, r1, r2 int128) int128 {
	if dividend == r1.mul(divisor) {
		return r1
	}
	switch m {
	case roundZero:
		return r1
	case roundInfinity:
		return r2
	}
	d1 := dividend.sub(r1.mul(divisor))
	d2 := r2.mul(divisor).sub(dividend)
	switch c := d1.cmp(d2); {
	case c < 0:
		return r1
	case c > 0:
		return r2
	}
	switch m {
	case roundHalfZero:
		return r1
	case roundHalfInfinity:
		return r2
	}
	if r1.divEuclid(r2.sub(r1)).remEuclid(i128(2)).isZero() {
		return r1
	}
	return r2
}

// mulSaturating is i128::saturating_mul.
func mulSaturating(a, b int128) int128 {
	if a.isZero() || b.isZero() {
		return int128{}
	}
	neg := a.isNeg() != b.isNeg()
	u, v := a.magnitude(), b.magnitude()
	limit := uint128{1 << 63, 0} // 2^127, the magnitude of i128::MIN
	overflow := u.hi != 0 && v.hi != 0
	var p uint128
	if !overflow {
		// One of them fits in 64 bits.
		small, big := u.lo, v
		if u.hi != 0 {
			small, big = v.lo, u
		}
		h1, l1 := bits.Mul64(small, big.lo)
		h2, l2 := bits.Mul64(small, big.hi)
		var carry uint64
		p.lo = l1
		p.hi, carry = bits.Add64(h1, l2, 0)
		overflow = h2 != 0 || carry != 0
	}
	if !overflow {
		switch c := p.cmp(limit); {
		case c > 0:
			overflow = true
		case c == 0:
			overflow = !neg
		}
	}
	if overflow {
		if neg {
			return int128{hi: math.MinInt64}
		}
		return int128{hi: math.MaxInt64, lo: math.MaxUint64}
	}
	return p.signed(neg)
}
