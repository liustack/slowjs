// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command linkprobedtf formats a date with go-intl's DateTimeFormat, for
// TestLinksOnlyReadData to check that a reader of dates.bin and
// zonenames.bin links them.
package main

import (
	"fmt"
	"time"

	gointl "github.com/liustack/slowjs/internal/gointl"
)

func main() {
	l, err := gointl.ParseLocale("de")
	if err != nil {
		panic(err)
	}
	f, err := gointl.NewDateTimeFormat(l, gointl.DateTimeFormatOptions{TimeZoneName: gointl.ZoneLong})
	if err != nil {
		panic(err)
	}
	fmt.Println(f.Format(time.Unix(0, 0)))
}
