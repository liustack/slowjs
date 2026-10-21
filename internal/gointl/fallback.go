package intl

// Where to look when a locale has no data of its own.
//
// A table is rarely stored for every locale that can be asked for. "de-CH" may
// carry only what it says differently from "de", and "de" only what it says
// differently from the root. So a lookup is a walk: the most specific
// identifier first, then progressively less of it, ending at the root, which
// always has an answer.
//
// This is truncation inheritance, which UTS #35 defines on the identifier
// alone. Two refinements need data and arrive with it in stage 2:
//
//   - CLDR's parentLocales, which redirects a chain that truncation would send
//     somewhere wrong. "zh-Hant" must not fall back to "zh", because
//     traditional Chinese inheriting from simplified is worse than inheriting
//     from the root.
//   - Likely subtags, which fills in what an identifier leaves out, so that
//     "zh-TW" and "zh-Hant-TW" look in the same place.
//
// Until then the chain is what the identifier itself says, which is right for
// the common shapes and wrong only where CLDR says so.

// Fallback returns the data locales to try, most specific first and the root
// last. The receiver is always the first entry, so the chain is never empty.
func (d DataLocale) Fallback() []DataLocale {
	chain := []DataLocale{d}
	next := d
	// A variant is the most specific part, then the region, then the script.
	if !next.Variant.IsZero() {
		next.Variant = Variant{}
		chain = append(chain, next)
	}
	if !next.Region.IsZero() {
		next.Region = Region{}
		chain = append(chain, next)
	}
	if !next.Script.IsZero() {
		next.Script = Script{}
		chain = append(chain, next)
	}
	if !next.Language.IsUnd() {
		next.Language = Language{}
		chain = append(chain, next)
	}
	return chain
}

// Fallback returns the chain for the locale's data locale. The extensions play
// no part: they choose behavior, not which table is loaded.
func (l Locale) Fallback() []DataLocale { return l.Data().Fallback() }
