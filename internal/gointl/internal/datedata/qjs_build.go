// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package datedata

// Build materializes the lazy calendars for the Chromium pack generator.
func (l *Locale) Build() (*Locale, error) {
	r := *l
	r.Calendars = append([]NamedCalendar(nil), l.Calendars...)
	for _, p := range l.parts {
		c, _, err := l.Calendar(p.name)
		if err != nil {
			return nil, err
		}
		r.Calendars = append(r.Calendars, NamedCalendar{p.name, *c})
	}
	return &r, nil
}
