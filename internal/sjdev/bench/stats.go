// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import "math"

// Mean returns the mean of x.
func Mean(x []float64) float64 {
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

// Var returns the sample variance of x (n-1 in the denominator), NaN below 2 values.
func Var(x []float64) float64 {
	if len(x) < 2 {
		return math.NaN()
	}
	m := Mean(x)
	s := 0.0
	for _, v := range x {
		s += (v - m) * (v - m)
	}
	return s / float64(len(x)-1)
}

// betacf is the continued fraction of the incomplete beta function (Numerical
// Recipes, 6.4).
func betacf(a, b, x float64) float64 {
	const eps, tiny = 1e-15, 1e-300
	qab, qap, qam := a+b, a+1, a-1
	c, d := 1.0, 1-qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m <= 300; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < eps {
			break
		}
	}
	return h
}

// IncBeta is the regularized incomplete beta function I_x(a, b).
func IncBeta(a, b, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	lab, _ := math.Lgamma(a + b)
	front := math.Exp(lab - la - lb + a*math.Log(x) + b*math.Log1p(-x))
	if x < (a+1)/(a+b+2) {
		return front * betacf(a, b, x) / a
	}
	return 1 - front*betacf(b, a, 1-x)/b
}

// TCDF is the distribution function of Student's t with df degrees of freedom.
func TCDF(t, df float64) float64 {
	x := df / (df + t*t)
	p := 0.5 * IncBeta(df/2, 0.5, x)
	if t > 0 {
		return 1 - p
	}
	return p
}

// TQuantile returns the t with TCDF(t, df) = p, for 0 < p < 1.
func TQuantile(p, df float64) float64 {
	if p == 0.5 {
		return 0
	}
	if p < 0.5 {
		return -TQuantile(1-p, df)
	}
	lo, hi := 0.0, 1.0
	for TCDF(hi, df) < p {
		hi *= 2
	}
	for range 200 {
		mid := (lo + hi) / 2
		if TCDF(mid, df) < p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// Interval is an estimate with its two-sided t interval.
type Interval struct {
	Est, SE, DF float64
	Lo, Hi      float64
}

// TInterval is the interval est ± t(1-alpha/2, df)·se. It is empty (NaN) when
// df < 1 or se is not a number.
func TInterval(est, se, df, alpha float64) Interval {
	if df < 1 || math.IsNaN(se) {
		return Interval{Est: est, SE: se, DF: df, Lo: math.NaN(), Hi: math.NaN()}
	}
	h := TQuantile(1-alpha/2, df) * se
	return Interval{est, se, df, est - h, est + h}
}

// TwoSidedP is the two-sided p value of t with df degrees of freedom.
func TwoSidedP(t, df float64) float64 {
	return 2 * TCDF(-math.Abs(t), df)
}
