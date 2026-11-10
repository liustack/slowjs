//go:build gointl_all

package temporal

import (
	"math"
	"math/big"
	"math/rand"
	"testing"
)

func (a int128) big() *big.Int {
	b := new(big.Int).SetInt64(a.hi)
	b.Lsh(b, 64)
	return b.Add(b, new(big.Int).SetUint64(a.lo))
}

func fromBig(b *big.Int) int128 {
	m := new(big.Int).Lsh(big.NewInt(1), 128)
	v := new(big.Int).Mod(b, m)
	lo := new(big.Int).And(v, new(big.Int).SetUint64(math.MaxUint64)).Uint64()
	hi := new(big.Int).Rsh(v, 64).Uint64()
	return int128{int64(hi), lo}
}

func randInt128(r *rand.Rand) int128 {
	switch r.Intn(4) {
	case 0:
		return i128(r.Int63n(2000) - 1000)
	case 1:
		return i128(int64(r.Uint64()))
	case 2:
		return int128{hi: r.Int63n(1<<20) - 1<<19, lo: r.Uint64()}
	}
	return int128{hi: int64(r.Uint64()), lo: r.Uint64()}
}

// TestInt128MatchesBig checks the arithmetic against math/big, reduced to
// 128 bits as Rust's wrapping arithmetic reduces it.
func TestInt128MatchesBig(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 200000; i++ {
		a, b := randInt128(r), randInt128(r)
		ba, bb := a.big(), b.big()
		if got, want := a.add(b), fromBig(new(big.Int).Add(ba, bb)); got != want {
			t.Fatalf("%v + %v = %v, want %v", ba, bb, got.big(), want.big())
		}
		if got, want := a.sub(b), fromBig(new(big.Int).Sub(ba, bb)); got != want {
			t.Fatalf("%v - %v = %v, want %v", ba, bb, got.big(), want.big())
		}
		if got, want := a.mul(b), fromBig(new(big.Int).Mul(ba, bb)); got != want {
			t.Fatalf("%v * %v = %v, want %v", ba, bb, got.big(), want.big())
		}
		if a.cmp(b) != ba.Cmp(bb) {
			t.Fatalf("cmp(%v, %v)", ba, bb)
		}
		if b.isZero() || a == (int128{hi: math.MinInt64}) {
			continue
		}
		q, rem := new(big.Int).QuoRem(ba, bb, new(big.Int))
		if got := a.quo(b); got != fromBig(q) {
			t.Fatalf("%v / %v = %v, want %v", ba, bb, got.big(), q)
		}
		if got := a.rem(b); got != fromBig(rem) {
			t.Fatalf("%v %% %v = %v, want %v", ba, bb, got.big(), rem)
		}
		d, m := new(big.Int).DivMod(ba, bb, new(big.Int))
		if got := a.divEuclid(b); got != fromBig(d) {
			t.Fatalf("%v div_euclid %v = %v, want %v", ba, bb, got.big(), d)
		}
		if got := a.remEuclid(b); got != fromBig(m) {
			t.Fatalf("%v rem_euclid %v = %v, want %v", ba, bb, got.big(), m)
		}
		f, _ := new(big.Float).SetInt(ba).Float64()
		if got := a.float64(); got != f {
			t.Fatalf("%v as f64 = %v, want %v", ba, got, f)
		}
		if got := int128FromFloat(f); got.float64() != f {
			t.Fatalf("%v as i128 = %v", f, got.big())
		}
	}
}
