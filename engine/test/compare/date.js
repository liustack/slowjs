// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// Local time is UTC, whatever the machine says: the engine has no time zone
// to ask, and the same page gives the same text everywhere.
const out = []
const ev = (src) => (0, eval)(src)
// Each case is a source and the string it gives.
for (const [src, want] of [
	// getYear is the year minus 1900 around the edges of a year
	["12+34", "46"],
	["new Date(1899, 0).getYear()", "-1"],
	["new Date(1899, 11, 31, 23, 59, 59, 999).getYear()", "-1"],
	["new Date(1900, 0).getYear()", "0"],
	["new Date(1900, 11, 31, 23, 59, 59, 999).getYear()", "0"],
	["new Date(1970, 0).getYear()", "70"],
	["new Date(1970, 11, 31, 23, 59, 59, 999).getYear()", "70"],
	["new Date(2000, 0).getYear()", "100"],
	["new Date(2000, 11, 31, 23, 59, 59, 999).getYear()", "100"],
	// local time is UTC
	["new Date(0).getHours()", "0"],
	["new Date(0).getTimezoneOffset()", "0"],
	["new Date(Date.UTC(2026, 6, 1)).getTimezoneOffset()", "0"],
	["new Date(0).toString()", "Thu Jan 01 1970 00:00:00 GMT+0000"],
	["new Date(0).toTimeString()", "00:00:00 GMT+0000"],
	["new Date(2026, 0, 1, 13, 5).getTime() === Date.UTC(2026, 0, 1, 13, 5)", "true"],
	["Date.parse('2026-01-01T13:05:00') === Date.UTC(2026, 0, 1, 13, 5)", "true"],
]) {
	const got = String(ev(src))
	out.push(got === want ? src + " = " + got : "FAIL " + src + ": got " + got + ", want " + want)
}
globalThis.result = out.join("\n")
