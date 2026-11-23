// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl

// Literals are what the patterns of f put before, between and after two
// items, for a list of n items whose last item begins with last: for a list
// of two, the three of its pattern, and for a longer one, the three of the
// start pattern, of the middle pattern and of the end pattern, in turn. A
// caller writes the list from them as formatInOrder does, with no list of
// the items or of the parts (qjs): with s, m and e the start, middle and
// end, items 0 to n-2 each after the first of its pattern and before the
// second, s for item 0, e for item n-2 and m between, then item n-1, then
// the third of each pattern from n-2 back to 0.
//
// Only the first characters of the last item count: the patterns of
// Spanish and Hebrew read at most its first three bytes, or its first code
// point, and whether it is two bytes long. contextual is whether f reads it
// at all. ok is false for n under 2, and for a pattern that does not put
// {0} before {1}, which no locale's do.
func (f *ListFormat) Literals(n int, last string) (lits []string, contextual, ok bool) {
	var patterns []string
	switch {
	case n < 2:
		return nil, false, false
	case n == 2:
		patterns = []string{f.two(last)}
	default:
		patterns = []string{f.patterns.Start, f.patterns.Middle, f.end(last)}
	}
	lits = make([]string, 0, 3*len(patterns))
	for _, p := range patterns {
		s, ok := splitListPattern(p)
		if !ok {
			return nil, false, false
		}
		lits = append(lits, s[:]...)
	}
	return lits, f.context != nil, true
}
