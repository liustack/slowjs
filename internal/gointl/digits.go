package intl

import (
	"fmt"
	"strings"
)

// How many digits get written, and how the rest are rounded away.
//
// Both NumberFormat and PluralRules need this, and they need the same answer:
// which plural form a number takes depends on how it would be written, so the
// two cannot disagree about the digits without disagreeing about plurals. It
// lives here rather than inside either of them.

// roundingKind is which way of counting digits decides the rounding.
type roundingKind int

const (
	roundFractionDigits roundingKind = iota
	roundSignificantDigits
	roundMorePrecision
	roundLessPrecision
)

// A digitRequest is the part of an option bag that decides the plan.
type digitRequest struct {
	minInt           int
	minFrac, maxFrac *int
	minSig, maxSig   *int
	priority         RoundingPriority
	mode             RoundingMode
	increment        int
	trailingZero     TrailingZeroDisplay
	// compact says the number will be written in compact notation, which has
	// its own default: two significant digits or no decimals, whichever keeps
	// more.
	compact bool
	// The defaults for this style: none for a plain number, two for most
	// money, none at all for a percentage.
	minFracDefault, maxFracDefault int
}

// A digitPlan is what a request settled on.
type digitPlan struct {
	minInt           int
	minFrac, maxFrac int
	minSig, maxSig   int
	rounding         roundingKind
	mode             RoundingMode
	increment        int
	trailingZero     TrailingZeroDisplay
	priority         RoundingPriority
	// reportFrac and reportSig say which of the two counts resolvedOptions
	// reports: both, but for the decimals where significant digits alone
	// decide and the other way round.
	reportFrac, reportSig bool
}

// ResolvedDigits are the digit and rounding options a NumberFormat or a
// PluralRules settled on, as resolvedOptions reports them. The decimals and
// the significant digits are nil where it leaves them out: the decimals
// where significant digits alone decide, the significant digits where
// decimals alone do.
type ResolvedDigits struct {
	MinimumIntegerDigits     int
	MinimumFractionDigits    *int
	MaximumFractionDigits    *int
	MinimumSignificantDigits *int
	MaximumSignificantDigits *int
	RoundingIncrement        int
	RoundingMode             RoundingMode
	RoundingPriority         RoundingPriority
	TrailingZeroDisplay      TrailingZeroDisplay
}

// resolved is what resolvedOptions reports of the plan.
func (p *digitPlan) resolved() ResolvedDigits {
	r := ResolvedDigits{
		MinimumIntegerDigits: p.minInt,
		RoundingIncrement:    max(p.increment, 1),
		RoundingMode:         p.mode,
		RoundingPriority:     p.priority,
		TrailingZeroDisplay:  p.trailingZero,
	}
	if p.reportFrac {
		r.MinimumFractionDigits, r.MaximumFractionDigits = Digits(p.minFrac), Digits(p.maxFrac)
	}
	if p.reportSig {
		r.MinimumSignificantDigits, r.MaximumSignificantDigits = Digits(p.minSig), Digits(p.maxSig)
	}
	return r
}

// resolve settles the counts, by ECMA-402's SetNumberFormatDigitOptions.
//
// There are two ways of counting and both can be given. Which wins is:
// significant digits alone, decimals alone, or -- when both are given with a
// priority -- whichever of the two keeps more or less.
func (r digitRequest) resolve() (digitPlan, error) {
	var p digitPlan
	p.mode, p.increment, p.trailingZero, p.priority = r.mode, r.increment, r.trailingZero, r.priority

	p.minInt = r.minInt
	if p.minInt <= 0 {
		p.minInt = 1
	}

	hasFrac := r.minFrac != nil || r.maxFrac != nil
	hasSig := r.minSig != nil || r.maxSig != nil

	// SetNumberFormatDigitOptions: a minimum not given is the default, or
	// the maximum where that is less, and a maximum not given the default,
	// or the minimum where that is more.
	switch {
	case r.minFrac == nil && r.maxFrac != nil:
		p.maxFrac = *r.maxFrac
		p.minFrac = min(r.minFracDefault, p.maxFrac)
	case r.minFrac != nil && r.maxFrac == nil:
		p.minFrac = *r.minFrac
		p.maxFrac = max(r.maxFracDefault, p.minFrac)
	case r.minFrac != nil:
		p.minFrac, p.maxFrac = *r.minFrac, *r.maxFrac
	default:
		p.minFrac = r.minFracDefault
		p.maxFrac = max(p.minFrac, r.maxFracDefault)
	}
	if p.minFrac > p.maxFrac {
		return p, fmt.Errorf("%w: at least %d decimals but at most %d",
			ErrOption, p.minFrac, p.maxFrac)
	}

	p.minSig, p.maxSig = 1, 21
	if r.minSig != nil {
		p.minSig = *r.minSig
	}
	if r.maxSig != nil {
		p.maxSig = *r.maxSig
	}
	if hasSig && p.minSig > p.maxSig {
		return p, fmt.Errorf("%w: at least %d significant digits but at most %d",
			ErrOption, p.minSig, p.maxSig)
	}

	switch {
	case r.priority == MorePrecision:
		p.rounding = roundMorePrecision
	case r.priority == LessPrecision:
		p.rounding = roundLessPrecision
	case hasSig:
		// Significant digits win over decimals when both are given without a
		// priority, which is what ECMA-402 calls auto.
		p.rounding = roundSignificantDigits
	case hasFrac:
		p.rounding = roundFractionDigits
	case r.compact:
		p.minSig, p.maxSig = 1, 2
		p.minFrac, p.maxFrac = 0, 0
		// ECMA-402's [[ComputedRoundingPriority]], which resolvedOptions
		// reports.
		p.rounding, p.priority = roundMorePrecision, MorePrecision
	default:
		p.rounding = roundFractionDigits
	}

	// Which counts resolvedOptions reports: under the automatic priority,
	// the significant digits only where they were asked for, and the
	// decimals unless significant digits were asked for or a compact number
	// asked for neither; both, where either priority was asked for.
	needSig, needFrac := true, true
	if r.priority == PriorityAuto {
		needSig = hasSig
		needFrac = !hasSig && (hasFrac || !r.compact)
	}
	p.reportSig = needSig || !needFrac
	p.reportFrac = needFrac || !needSig

	if p.increment > 1 && (p.rounding != roundFractionDigits || p.minFrac != p.maxFrac) {
		return p, fmt.Errorf("%w: a rounding increment needs the same "+
			"smallest and largest number of decimals and no significant digits", ErrOption)
	}
	return p, nil
}

// round cuts a number down to the digits that will be written, by whichever
// way of counting the options settled on, and then pads or trims the decimals.
func (p *digitPlan) round(magnitude mag, negative bool) (string, string) {
	var integer, fraction string
	// significant is whether the significant digits decided, whose minimum
	// the number is then padded to; otherwise the decimals' is.
	significant := false
	switch p.rounding {
	case roundSignificantDigits:
		significant = true
		integer, fraction = roundSignificant(magnitude, p.maxSig, negative, p.mode)
	case roundMorePrecision, roundLessPrecision:
		// The two ways are compared by where each would round: the smaller
		// place keeps more. Which of the two is wanted is the priority.
		// FormatNumericToString: morePrecision takes the significant
		// digits' result where its rounding magnitude is at or below the
		// decimals', and lessPrecision takes the decimals' there.
		sInteger, sFraction := roundSignificant(magnitude, p.maxSig, negative, p.mode)
		sig := significantPlace(sInteger, sFraction, p.maxSig)
		frac := -p.maxFrac
		useSig := sig <= frac
		if p.rounding == roundLessPrecision {
			useSig = sig > frac
		}
		// The one chosen is taken whole, its minimum with it, as ECMA-402's
		// FormatNumericToString takes sResult or fResult.
		significant = useSig
		if useSig {
			integer, fraction = sInteger, sFraction
		} else {
			integer, fraction = roundAt(magnitude, p.maxFrac, negative, p.mode)
		}
	default:
		if p.increment > 1 {
			// Once, to the increment: rounding to the decimals first would
			// round twice, and 1.25 to the nearest 0.2 would be 1.4.
			integer, fraction = roundToIncrement(magnitude.integer, magnitude.written(p.maxFrac+1), p.maxFrac,
				p.increment, negative, p.mode)
		} else {
			integer, fraction = roundAt(magnitude, p.maxFrac, negative, p.mode)
		}
	}
	return p.pad(integer, fraction, significant)
}

// pad trims the decimals that were not asked for and writes the ones that
// were, by the minimum of the way of counting that decided the rounding.
func (p *digitPlan) pad(integer, fraction string, significant bool) (string, string) {
	minFrac := p.minFrac
	if significant {
		// Significant digits set their own minimum: enough decimals to make up
		// the smallest count, and no more.
		minFrac = 0
		if strings.Trim(integer, "0") == "" && strings.Trim(fraction, "0") == "" {
			// Zero counts as one significant digit of its own, so it takes
			// decimals only when more than one was asked for.
			minFrac = p.minSig - 1
		} else if digits := len(strings.TrimLeft(integer, "0")); digits < p.minSig {
			if integer == "0" || digits == 0 {
				// A number below one counts its significant digits from the
				// first that is not a zero, wherever that falls.
				lead := len(fraction) - len(strings.TrimLeft(fraction, "0"))
				minFrac = lead + p.minSig
			} else {
				minFrac = p.minSig - digits
			}
		}
		if minFrac > len(fraction) {
			minFrac = min(minFrac, p.maxSig+len(fraction))
		}
	}
	fraction = trimTrailingZeros(fraction, minFrac)
	for len(fraction) < minFrac {
		fraction += "0"
	}
	if p.trailingZero == TrailingZeroStripIfInteger && strings.Trim(fraction, "0") == "" {
		fraction = ""
	}
	return integer, fraction
}
