// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import "time"

// ChromiumPattern exposes the resolved ICU pattern to package intl's
// legacy Date formatter, which shares ISO styles with Temporal.
func (f *DateTimeFormat) ChromiumPattern() string { return f.patternText }

// ForTemporalChromium keeps the original pattern when the type's skeleton
// has not changed, as Chromium 156's GetSimpleDateTimeForTemporal does.
// Regenerating an unchanged skeleton loses literals in patterns such as
// Japanese M月d日, whose skeleton Md also matches M/d. The intervals still
// use their own patterns. Plain values keep ForTemporal's UTC zone.
func (f *DateTimeFormat) ForTemporalChromium(kind TemporalKind) (*DateTimeFormat, error) {
	d, err := f.ForTemporal(kind)
	if err != nil {
		return nil, err
	}
	original := staticSkeleton(f.patternText)
	skeleton := temporalSkeleton(original, f.explicit, kind,
		f.opts.DateStyle != LengthNone || f.opts.TimeStyle != LengthNone,
		f.opts.ToLocaleStringTimeZone, true)
	if skeleton == original {
		kept := *f
		kept.tz, kept.zoneID = d.tz, d.zoneID
		return &kept, nil
	}
	return d, nil
}

// FormatTemporalZoned honors the offset already computed by Temporal.
// When zone data versions disagree, its names no longer describe that
// offset, so use the offset's name as package intl's formatAt does.
func (f *DateTimeFormat) FormatTemporalZoned(t time.Time, offset int) string {
	if f.tz.offsetAt(t.UnixMilli()).total() == offset {
		return f.Format(t)
	}
	d := *f
	d.tz, d.zoneID = fixedZone(offset), ""
	if d.zones != nil {
		d.zone = d.zones.zone("", d.tz)
	}
	return d.Format(t)
}
