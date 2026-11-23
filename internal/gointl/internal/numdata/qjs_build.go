// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package numdata

// Build materializes the lazy tables for the Chromium pack generator.
func (l *Locale) Build() *Locale {
	r := *l
	r.Currencies = append([]Currency(nil), l.Currencies...)
	for _, code := range l.currencies.Keys() {
		c, ok := l.Currency(code)
		if !ok {
			panic("numdata: unreadable currency " + code)
		}
		r.Currencies = append(r.Currencies, c)
	}
	return &r
}
