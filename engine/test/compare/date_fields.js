// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// The fields of a time value as every getter of Date reads them, local and
// UTC, and the strings made from them, from the first to the last day a time
// value can name: what Node 24 prints with TZ=UTC (local time is UTC in the
// engine). toString and toTimeString leave out the name of the zone that Node
// adds in parentheses. A setter and then a getter, two Dates of one time value
// and Dates read by turns check that no getter gives the fields of another
// time value.
const out = []
const names = []
for (const g of ["FullYear", "Month", "Date", "Day", "Hours", "Minutes", "Seconds", "Milliseconds"])
	names.push("get" + g, "getUTC" + g)
names.push("getYear", "getTimezoneOffset")
const fields = (d) => names.map((g) => d[g]()).join(",")
const zone = (s) => s.replace(/ \([^)]*\)$/, "")
function strings(d) {
	let iso
	try { iso = d.toISOString() } catch (e) { iso = e.name }
	return [iso, d.toUTCString(), zone(d.toString()), d.toDateString(), zone(d.toTimeString()), d.toJSON()].join(" | ")
}
const show = (label, d) => out.push(label + ": " + d.getTime() + " " + fields(d) + " | " + strings(d))
// a time value from fields, for any year (Date.UTC takes 0 to 99 as 1900 on)
function utc(y, mo, d, h = 0, mi = 0, s = 0, ms = 0) {
	const t = new Date(0)
	t.setUTCFullYear(y, mo, d)
	t.setUTCHours(h, mi, s, ms)
	return t.getTime()
}

// around the epoch, a day and the limits of the range
for (const t of [0, -1, 1, 999, 1000, 59999, 60000, 3599999, 3600000, 86399999, 86400000,
	-86400000, -86400001, -86399999, 8.64e15, -8.64e15, 8.64e15 - 1, -8.64e15 + 1,
	8.64e15 + 1, -8.64e15 - 1, NaN])
	show("t " + t, new Date(t))
// the days around the ends of February and of the years of each kind of leap rule
for (const [y, mo, d] of [[2000, 1, 28], [2000, 1, 29], [2000, 2, 1], [1900, 1, 28], [1900, 2, 1],
	[2100, 1, 28], [2100, 2, 1], [1600, 1, 29], [1600, 11, 31], [1601, 0, 1], [1969, 11, 31],
	[2024, 1, 29], [2023, 1, 28], [2023, 2, 1], [2024, 11, 31], [2025, 0, 1]])
	show(y + "-" + (mo + 1) + "-" + d, new Date(utc(y, mo, d, 23, 59, 59, 999)))
// years with few digits, negative years and the first and last days of the range
for (const y of [0, 1, 4, 99, 100, 400, 999, 1000, 9999, 10000, 99999, 100000, 123456, 275759,
	-1, -4, -99, -100, -400, -999, -1000, -9999, -10000, -99999, -100000, -123456, -271820]) {
	show("y " + y + " first", new Date(utc(y, 0, 1)))
	show("y " + y + " last", new Date(utc(y, 11, 31, 23, 59, 59, 999)))
}
show("first day", new Date(utc(-271821, 3, 20)))
show("last day", new Date(utc(275760, 8, 13)))
show("before first", new Date(utc(-271821, 3, 19, 23, 59, 59, 999)))
// the first and last day of each month of a common and a leap year
for (const y of [2023, 2024])
	for (let mo = 0; mo < 12; mo++) {
		show(y + " month " + mo + " first", new Date(utc(y, mo, 1)))
		show(y + " month " + mo + " last", new Date(utc(y, mo + 1, 0, 23, 59, 59, 999)))
	}

// sums over many days: every day from 1599-12-25 to 2401-01-07, and 200001 time
// values spread over the whole range
const hash = (h, x) => (Math.imul(h, 31) + x) | 0
function sum(t) {
	const d = new Date(t)
	let h = 0
	for (const g of names)
		h = hash(h, d[g]())
	return h
}
function hashString(h, s) {
	for (let i = 0; i < s.length; i++)
		h = hash(h, s.charCodeAt(i))
	return h
}
{
	let h = 0, hs = 0, n = 0
	for (let t = utc(1599, 11, 25), end = utc(2401, 0, 7); t <= end; t += 86400000) {
		const x = t + (n * 7919) % 86400000
		h = hash(h, sum(x))
		if (n % 13 === 0)
			hs = hashString(hs, strings(new Date(x)))
		n++
	}
	out.push("days " + n + " fields " + h + " strings " + hs)
}
{
	let h = 0, hs = 0
	for (let i = 0; i <= 200000; i++) {
		const t = -8.64e15 + i * 4.32e10 + (i % 1000) * 86399
		h = hash(h, sum(Math.min(t, 8.64e15)))
		if (i % 17 === 0)
			hs = hashString(hs, strings(new Date(Math.min(t, 8.64e15))))
	}
	out.push("range fields " + h + " strings " + hs)
}

// a setter, then the getters
{
	const a = new Date(utc(2024, 1, 29, 12, 30, 45, 678))
	show("a", a)
	a.setUTCFullYear(2023); show("setUTCFullYear(2023)", a)
	a.setFullYear(2024, 1, 29); show("setFullYear(2024, 1, 29)", a)
	a.setMonth(5); show("setMonth(5)", a)
	a.setUTCMonth(13); show("setUTCMonth(13)", a)
	a.setDate(0); show("setDate(0)", a)
	a.setUTCDate(32); show("setUTCDate(32)", a)
	a.setHours(25); show("setHours(25)", a)
	a.setUTCHours(-1); show("setUTCHours(-1)", a)
	a.setMinutes(-1); show("setMinutes(-1)", a)
	a.setUTCMinutes(61, 61, 1001); show("setUTCMinutes(61, 61, 1001)", a)
	a.setSeconds(60); show("setSeconds(60)", a)
	a.setUTCSeconds(-1, -1); show("setUTCSeconds(-1, -1)", a)
	a.setMilliseconds(1000); show("setMilliseconds(1000)", a)
	a.setUTCMilliseconds(-1); show("setUTCMilliseconds(-1)", a)
	a.setTime(0); show("setTime(0)", a)
	a.setYear(99); show("setYear(99)", a)
	a.setYear(-5); show("setYear(-5)", a)
	a.setTime(8.64e15); show("setTime(8.64e15)", a)
	a.setUTCMilliseconds(1000); show("past the end", a)
	a.setFullYear(2020); show("NaN setFullYear(2020)", a)
	a.setHours(NaN); show("setHours(NaN)", a)
	a.setTime(-8.64e15); show("setTime(-8.64e15)", a)
	a.setUTCDate(a.getUTCDate() - 1); show("before the start", a)
}
// two Dates of one time value, one of them changed
{
	const t = utc(2026, 6, 21, 9, 30)
	const b = new Date(t), c = new Date(t)
	show("b", b)
	show("c", c)
	c.setUTCDate(c.getUTCDate() + 1)
	show("b after c changed", b)
	show("c changed", c)
	c.setTime(t)
	show("c back", c)
	const d = new Date(b)
	d.setUTCMonth(0)
	show("copy of b changed", d)
	show("b after its copy changed", b)
}
// getters by turns on Dates of different time values
{
	const ds = [new Date(0), new Date(utc(2026, 6, 21)), new Date(utc(-1, 11, 31)), new Date(NaN),
		new Date(utc(2026, 6, 22))]
	let h = 0
	for (let i = 0; i < 1000; i++) {
		const d = ds[i % ds.length]
		h = hash(h, d.getUTCDate() | 0)
		h = hash(h, ds[(i * 7) % ds.length].getFullYear() | 0)
		if (i % 100 === 0)
			ds[i % ds.length].setUTCHours(i % 24)
	}
	out.push("by turns " + h)
}
// NaN Dates
{
	const n = new Date(NaN)
	show("NaN", n)
	n.setHours(1); show("NaN setHours(1)", n)
	n.setUTCFullYear(1999); show("NaN setUTCFullYear(1999)", n)
	n.setTime(NaN); show("setTime(NaN)", n)
	show("Invalid", new Date("not a date"))
}
// a subclass, and receivers that are not Dates
{
	class D extends Date {}
	show("subclass", new D(utc(2026, 0, 1)))
	for (const [label, f] of [["{}", () => Date.prototype.getDay.call({})],
		["Date.prototype", () => Date.prototype.getUTCHours.call(Date.prototype)],
		["Object.create", () => Date.prototype.getFullYear.call(Object.create(Date.prototype))],
		["number", () => Date.prototype.getMonth.call(0)],
		["toISOString", () => Date.prototype.toISOString.call({})]]) {
		try { out.push(label + ": " + f()) } catch (e) { out.push(label + ": " + e.name) }
	}
}
globalThis.result = out.join("\n")
