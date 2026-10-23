// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package intlhook connects the Intl and Temporal shims. Package intl sets
// the date and duration formatting built-ins at initialization. State shares
// their private slot readers in each VM.
package intlhook // import "github.com/liustack/slowjs/internal/intlhook"

import "github.com/liustack/slowjs"

var (
	// Format formats a Temporal value. Its arguments are the kind of the
	// value ("date", "time", "dateTime", "yearMonth", "monthDay",
	// "instant" or "zoned"), the value's calendar, its epoch milliseconds
	// (of its wall clock at UTC for the plain kinds), the zone of a zoned
	// one and its offset from UTC in nanoseconds, the two locale arguments
	// of package intl's resolves and the options of Intl.DateTimeFormat, in
	// the order Options lists them. A zoned value is shown at the offset
	// given, which is Temporal's, whatever offset package intl's zone data
	// has for the zone then.
	Format *slowjs.Builtin

	// DefaultZone returns a VM's default time zone, the one intl.TimeZone
	// gave its Register, "UTC" without one.
	DefaultZone func(*slowjs.VM) string

	// Options is the json of the options Intl.DateTimeFormat reads, in
	// order: {"names": [...], "kinds": "..."}, a kind being a byte of
	// "snbg" as in package intl.
	Options string

	// Duration uses the same registry and formatter as Intl.DurationFormat.
	DurationOptions                 string
	DurationResolve, DurationFormat *slowjs.Builtin
)
