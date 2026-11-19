// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import gointl "github.com/liustack/slowjs/internal/gointl"

// resolveTimeZone checks a time zone name, an IANA name in any case, a link
// such as US/Pacific, or an offset such as +05:30, and returns the name
// resolvedOptions reports for it, as V8 does: "Asia/Calcutta" for
// "asia/kolkata", "UTC" for "GMT". The zones are go-intl's, built into the
// binary: the host's zoneinfo is never read. An unknown name is a
// RangeError.
func resolveTimeZone(name string) (string, error) {
	r, err := gointl.ResolveTimeZone(gointl.Embedded, name, goCompat)
	if err != nil {
		return "", rangeError("Invalid time zone specified: %s", name)
	}
	return r, nil
}
