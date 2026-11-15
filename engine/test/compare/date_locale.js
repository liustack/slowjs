// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// The engine's own toLocaleString, toLocaleDateString and toLocaleTimeString,
// which package intl replaces in a VM of the Go API: month/day/year and a
// 12-hour clock, in UTC. Node formats these with Intl, so the file is what the
// engine printed before its date strings were written without snprintf.
const out = []
function utc(y, mo, d, h = 0, mi = 0, s = 0, ms = 0) {
	const t = new Date(0)
	t.setUTCFullYear(y, mo, d)
	t.setUTCHours(h, mi, s, ms)
	return t.getTime()
}
const strings = (d) => [d.toLocaleString(), d.toLocaleDateString(), d.toLocaleTimeString()].join(" | ")
for (const [y, mo, d, h, mi, s] of [[1970, 0, 1, 0, 0, 0], [1970, 0, 1, 11, 59, 59], [1970, 0, 1, 12, 0, 0],
	[1970, 0, 1, 13, 5, 9], [1969, 11, 31, 23, 59, 59], [2026, 6, 21, 9, 30, 0], [0, 0, 1, 0, 0, 0],
	[5, 4, 3, 2, 1, 0], [999, 8, 9, 10, 11, 12], [9999, 11, 31, 23, 59, 59], [10000, 0, 1, 0, 0, 0],
	[123456, 1, 29, 12, 34, 56], [275760, 8, 13, 0, 0, 0], [-1, 11, 31, 23, 59, 59], [-12, 0, 1, 1, 2, 3],
	[-123, 0, 1, 1, 2, 3], [-1234, 0, 1, 1, 2, 3], [-12345, 0, 1, 1, 2, 3], [-271821, 3, 20, 0, 0, 0]])
	out.push(y + "-" + (mo + 1) + "-" + d + " " + h + ":" + mi + ":" + s + ": " + strings(new Date(utc(y, mo, d, h, mi, s))))
out.push("NaN: " + strings(new Date(NaN)))
// a sum over 200001 time values spread over the whole range, every hour of a day among them
{
	let h = 0
	for (let i = 0; i <= 200000; i++) {
		const s = strings(new Date(Math.min(-8.64e15 + i * 4.32e10 + (i % 1000) * 3599999, 8.64e15)))
		for (let j = 0; j < s.length; j++)
			h = (Math.imul(h, 31) + s.charCodeAt(j)) | 0
	}
	out.push("range " + h)
}
globalThis.result = out.join("\n")
