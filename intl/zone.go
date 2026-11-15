// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/slowjs/intl"

import gointl "github.com/liustack/slowjs/internal/gointl"

// resolveTimeZone uses the embedded ICU tz pack, never the host's zoneinfo.
func resolveTimeZone(name string) (string, error) {
	zone, err := gointl.ResolveTimeZone(gointl.Embedded, name, goCompat)
	if err != nil {
		return "", rangeError("invalid time zone specified: %s", name)
	}
	return zone, nil
}
