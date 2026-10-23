// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The cases TestChromium runs, and that testdata/chromium.json has
// Chromium 156's answers to (TZ=UTC): expressions, each answered with its
// value as a string, or the name and message of the error it throws.

var cases = [];

(function () {
	function add(e) {
		cases.push(e);
	}

	// PlainDate: construction, parsing, bags, overflow.
	[
		'new Temporal.PlainDate(2024, 2, 29)', 'new Temporal.PlainDate(2023, 2, 29)', 'new Temporal.PlainDate(-271821, 4, 19)',
		'new Temporal.PlainDate(-271821, 4, 18)', 'new Temporal.PlainDate(275760, 9, 13)', 'new Temporal.PlainDate(275760, 9, 14)',
		'new Temporal.PlainDate(2024, 1, 5, "gregory")', 'new Temporal.PlainDate(2024, 1, 5, "GREGORY")', 'new Temporal.PlainDate(2024, 1, 5, "foo")',
		'new Temporal.PlainDate(2024.9, 1.9, 5.9)', 'new Temporal.PlainDate(NaN, 1, 1)', 'new Temporal.PlainDate(2024, 1, 5, 1)',
		'Temporal.PlainDate.from("2024-01-05")', 'Temporal.PlainDate.from("20240105")', 'Temporal.PlainDate.from("+002024-01-05")',
		'Temporal.PlainDate.from("-000000-01-05")', 'Temporal.PlainDate.from("2024-01-05T25:00")', 'Temporal.PlainDate.from("2024-01-05[u-ca=hebrew]")',
		'Temporal.PlainDate.from("2024-01-05[!u-ca=hebrew]")', 'Temporal.PlainDate.from("2024-01-05[u-ca=hebrew][u-ca=japanese]")',
		'Temporal.PlainDate.from("2024-01-05[!u-ca=hebrew][u-ca=japanese]")', 'Temporal.PlainDate.from("2024-01-05Z")',
		'Temporal.PlainDate.from("2024-01-05T12:00+01:00[Europe/Paris]")', 'Temporal.PlainDate.from("2024-02-30")',
		'Temporal.PlainDate.from({year: 2024, month: 2, day: 30})', 'Temporal.PlainDate.from({year: 2024, month: 2, day: 30}, {overflow: "reject"})',
		'Temporal.PlainDate.from({year: 2024, monthCode: "M02", day: 29})', 'Temporal.PlainDate.from({year: 2024, monthCode: "M13", day: 1})',
		'Temporal.PlainDate.from({year: 2024, month: 3, monthCode: "M02", day: 1})', 'Temporal.PlainDate.from({year: 2024, day: 1})',
		'Temporal.PlainDate.from({month: 1, day: 1})', 'Temporal.PlainDate.from({year: 2024, month: 0, day: 1})',
		'Temporal.PlainDate.from({year: 2024, month: 1, day: -1})', 'Temporal.PlainDate.from({year: 2024, month: 1, day: Infinity})',
		'Temporal.PlainDate.from({year: 5784, monthCode: "M05L", day: 1, calendar: "hebrew"})',
		'Temporal.PlainDate.from({year: 5785, monthCode: "M05L", day: 1, calendar: "hebrew"})',
		'Temporal.PlainDate.from({year: 5785, monthCode: "M05L", day: 1, calendar: "hebrew"}, {overflow: "reject"})',
		'Temporal.PlainDate.from({era: "reiwa", eraYear: 6, month: 1, day: 5, calendar: "japanese"})',
		'Temporal.PlainDate.from({era: "heisei", eraYear: 40, month: 1, day: 5, calendar: "japanese"})',
		'Temporal.PlainDate.from({era: "bce", eraYear: 1, month: 1, day: 1, calendar: "gregory"})',
		'Temporal.PlainDate.from({year: 1446, month: 9, day: 1, calendar: "islamic-umalqura"})',
		'Temporal.PlainDate.from({year: 2024, month: 1, day: 5, calendar: "chinese"})',
		'Temporal.PlainDate.from({year: 1403, month: 1, day: 1, calendar: "persian"})',
		'Temporal.PlainDate.from({year: 2017, month: 1, day: 1, calendar: "ethiopic"})',
		'Temporal.PlainDate.from(1)', 'Temporal.PlainDate.from(null)', 'Temporal.PlainDate.from("")',
		'Temporal.PlainDate.from("2024-01-05", {overflow: "foo"})', 'Temporal.PlainDate.from("2024-01-05", 1)',
	].forEach(add);

	// PlainDate getters across calendars.
	['iso8601', 'gregory', 'japanese', 'buddhist', 'roc', 'hebrew', 'islamic-civil', 'islamic-umalqura', 'persian', 'indian',
		'chinese', 'dangi', 'coptic', 'ethiopic', 'ethioaa'].forEach(function (cal) {
		add('(d => [d.calendarId, d.era, d.eraYear, d.year, d.month, d.monthCode, d.day, d.dayOfWeek, d.dayOfYear, d.weekOfYear, d.yearOfWeek, d.daysInWeek, d.daysInMonth, d.daysInYear, d.monthsInYear, d.inLeapYear].join())(Temporal.PlainDate.from("2024-03-15").withCalendar("' + cal + '"))');
		add('Temporal.PlainDate.from("2024-03-15").withCalendar("' + cal + '").add({years: 1, months: 2, days: 3}).toString()');
		add('Temporal.PlainDate.from("2024-03-15").withCalendar("' + cal + '").until(Temporal.PlainDate.from("2026-01-01").withCalendar("' + cal + '"), {largestUnit: "years"}).toString()');
		add('Temporal.PlainDate.from("2024-03-15").withCalendar("' + cal + '").toPlainYearMonth().toString()');
		add('Temporal.PlainDate.from("2024-03-15").withCalendar("' + cal + '").toPlainMonthDay().toString()');
	});

	// PlainDate arithmetic and differences.
	[
		['2024-01-31', '{months: 1}'], ['2024-01-31', '{months: 1}, {overflow: "reject"}'], ['2024-02-29', '{years: 1}'],
		['2024-02-29', '{years: -4}'], ['2024-01-05', '{weeks: 52}'], ['2024-01-05', '{days: 1e8}'], ['2024-01-05', '{hours: 47}'],
		['2024-01-05', '"P1Y2M3W4D"'], ['2024-01-05', '"-P1D"'], ['2024-01-05', '{}'], ['2024-01-05', '{years: 1, days: -1}'],
		['275760-09-13', '{days: 1}'],
	].forEach(function (c) {
		add('Temporal.PlainDate.from("' + c[0] + '").add(' + c[1] + ')');
		add('Temporal.PlainDate.from("' + c[0] + '").subtract(' + c[1] + ')');
	});
	['{}', '{largestUnit: "years"}', '{largestUnit: "months"}', '{largestUnit: "weeks"}', '{largestUnit: "hours"}',
		'{smallestUnit: "months", roundingMode: "halfExpand"}', '{smallestUnit: "weeks", roundingIncrement: 2}',
		'{largestUnit: "days", smallestUnit: "years"}', '{roundingIncrement: 0}', '{smallestUnit: "years", roundingMode: "ceil"}'].forEach(function (o) {
		add('Temporal.PlainDate.from("2024-01-31").until("2025-03-01", ' + o + ')');
		add('Temporal.PlainDate.from("2024-01-31").since("2025-03-01", ' + o + ')');
	});
	add('Temporal.PlainDate.from("2024-01-05").until(Temporal.PlainDate.from("2024-01-05[u-ca=japanese]"))');
	add('Temporal.PlainDate.compare("2024-01-05", {year: 2024, month: 1, day: 5})');
	add('Temporal.PlainDate.from("2024-01-05").equals("2024-01-05[u-ca=gregory]")');
	add('Temporal.PlainDate.from("2024-01-05").with({monthCode: "M02", day: 31})');
	add('Temporal.PlainDate.from("2024-01-05").with({calendar: "gregory"})');
	add('Temporal.PlainDate.from("2024-01-05").with({hour: 1})');
	add('Temporal.PlainDate.from("2024-01-05").with(Temporal.PlainDate.from("2025-01-01"))');
	add('Temporal.PlainDate.from("2024-01-05").toPlainDateTime("13:05:07.5")');
	add('Temporal.PlainDate.from("2024-01-05").toPlainDateTime()');
	add('Temporal.PlainDate.from("2024-01-05").toZonedDateTime("America/New_York")');
	add('Temporal.PlainDate.from("2024-03-10").toZonedDateTime({timeZone: "America/New_York", plainTime: "02:30"})');
	add('Temporal.PlainDate.from("2024-01-05").toZonedDateTime({plainTime: "02:30"})');
	['auto', 'always', 'never', 'critical', 'foo'].forEach(function (c) {
		add('Temporal.PlainDate.from("2024-01-05[u-ca=japanese]").toString({calendarName: "' + c + '"})');
		add('Temporal.PlainDate.from("2024-01-05").toString({calendarName: "' + c + '"})');
	});

	// PlainTime.
	[
		'new Temporal.PlainTime()', 'new Temporal.PlainTime(24)', 'new Temporal.PlainTime(23, 59, 59, 999, 999, 999)', 'new Temporal.PlainTime(-1)',
		'Temporal.PlainTime.from("13:05")', 'Temporal.PlainTime.from("T1305")', 'Temporal.PlainTime.from("13:05:60")', 'Temporal.PlainTime.from("24:00")',
		'Temporal.PlainTime.from("2024-01-05T13:05:07Z")', 'Temporal.PlainTime.from("2024-01-05T13:05:07+01:00")', 'Temporal.PlainTime.from("13:05[u-ca=japanese]")',
		'Temporal.PlainTime.from("10-12")', 'Temporal.PlainTime.from("T10-12")', 'Temporal.PlainTime.from({hour: 25})',
		'Temporal.PlainTime.from({hour: 25}, {overflow: "reject"})', 'Temporal.PlainTime.from({minute: 61, nanosecond: 1000})', 'Temporal.PlainTime.from({})',
		'Temporal.PlainTime.from("13:05").add({hours: 12, minutes: 70})', 'Temporal.PlainTime.from("13:05").subtract({days: 1, nanoseconds: 1})',
		'Temporal.PlainTime.from("13:05").until("01:00")', 'Temporal.PlainTime.from("13:05").since("01:00", {largestUnit: "minutes"})',
		'Temporal.PlainTime.from("13:05").until("01:00", {largestUnit: "days"})', 'Temporal.PlainTime.from("13:05:07.123456789").until("13:05", {smallestUnit: "microseconds", roundingMode: "halfEven"})',
		'Temporal.PlainTime.from("13:05").with({hour: 1, second: 70})', 'Temporal.PlainTime.from("13:05").with({})',
		'Temporal.PlainTime.from("13:05").equals("13:05:00.000000001")', 'Temporal.PlainTime.compare("13:05", "13:04:59.999")',
	].forEach(add);
	['"hour"', '"minute"', '{smallestUnit: "minute", roundingIncrement: 15}', '{smallestUnit: "minute", roundingIncrement: 7}',
		'{smallestUnit: "second", roundingMode: "floor"}', '{smallestUnit: "nanosecond"}', '{smallestUnit: "day"}', '{}', '"years"',
		'{smallestUnit: "hour", roundingIncrement: 24}', '{smallestUnit: "millisecond", roundingMode: "halfTrunc"}'].forEach(function (o) {
		add('Temporal.PlainTime.from("13:37:29.999500001").round(' + o + ')');
	});
	['{}', '{fractionalSecondDigits: 0}', '{fractionalSecondDigits: 3}', '{fractionalSecondDigits: 9}', '{fractionalSecondDigits: 10}',
		'{fractionalSecondDigits: "auto"}', '{fractionalSecondDigits: "3"}', '{smallestUnit: "minute"}', '{smallestUnit: "hour"}',
		'{smallestUnit: "second", roundingMode: "ceil"}', '{fractionalSecondDigits: 2.9}'].forEach(function (o) {
		add('Temporal.PlainTime.from("23:59:59.999999999").toString(' + o + ')');
		add('Temporal.PlainTime.from("13:05:07.12").toString(' + o + ')');
	});

	// PlainDateTime.
	[
		'new Temporal.PlainDateTime(2024, 1, 5)', 'new Temporal.PlainDateTime(2024, 1, 5, 24)', 'new Temporal.PlainDateTime(-271821, 4, 19, 0, 0, 0, 0, 0, 1)',
		'new Temporal.PlainDateTime(-271821, 4, 19)', 'Temporal.PlainDateTime.from("2024-01-05T13:05:07.123456789")', 'Temporal.PlainDateTime.from("2024-01-05 13:05")',
		'Temporal.PlainDateTime.from("2024-01-05t13:05")', 'Temporal.PlainDateTime.from("2024-01-05T13:05,5")', 'Temporal.PlainDateTime.from("2024-01-05T13:05:07.1234567891")',
		'Temporal.PlainDateTime.from({year: 2024, month: 1, day: 5, hour: 25})', 'Temporal.PlainDateTime.from({year: 2024, month: 1, day: 5, hour: 25}, {overflow: "reject"})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").with({day: 31, hour: 0})', 'Temporal.PlainDateTime.from("2024-01-05T13:05").withPlainTime("01:02")',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").withPlainTime()', 'Temporal.PlainDateTime.from("2024-01-05T13:05").add({days: 1, hours: 23, nanoseconds: -1})',
		'Temporal.PlainDateTime.from("2024-01-31T13:05").add({months: 1}, {overflow: "reject"})', 'Temporal.PlainDateTime.from("2024-01-05T13:05").until("2025-02-06T01:00")',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").until("2025-02-06T01:00", {largestUnit: "years", smallestUnit: "hours"})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").since("2025-02-06T01:00", {largestUnit: "months", smallestUnit: "days", roundingMode: "halfExpand"})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").round({smallestUnit: "hour"})', 'Temporal.PlainDateTime.from("2024-01-05T12:00").round("day")',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").round({smallestUnit: "day", roundingIncrement: 2})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").round({smallestUnit: "minute", roundingIncrement: 30})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").toString({fractionalSecondDigits: 4, calendarName: "always"})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").toZonedDateTime("Europe/London")',
		'Temporal.PlainDateTime.from("2024-03-31T01:30").toZonedDateTime("Europe/London")',
		'Temporal.PlainDateTime.from("2024-03-31T01:30").toZonedDateTime("Europe/London", {disambiguation: "earlier"})',
		'Temporal.PlainDateTime.from("2024-03-31T01:30").toZonedDateTime("Europe/London", {disambiguation: "later"})',
		'Temporal.PlainDateTime.from("2024-03-31T01:30").toZonedDateTime("Europe/London", {disambiguation: "reject"})',
		'Temporal.PlainDateTime.from("2024-10-27T01:30").toZonedDateTime("Europe/London", {disambiguation: "later"})',
		'Temporal.PlainDateTime.from("2024-10-27T01:30").toZonedDateTime("Europe/London", {disambiguation: "reject"})',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").toPlainDate()', 'Temporal.PlainDateTime.from("2024-01-05T13:05").toPlainTime()',
		'Temporal.PlainDateTime.compare("2024-01-05T13:05", "2024-01-05")', 'Temporal.PlainDateTime.from("2024-01-05").equals("2024-01-05T00:00")',
		'Temporal.PlainDateTime.from("2024-01-05T13:05").withCalendar("hebrew")',
		'(d => [d.year, d.month, d.monthCode, d.day, d.hour, d.minute, d.second, d.millisecond, d.microsecond, d.nanosecond, d.dayOfWeek].join())(Temporal.PlainDateTime.from("2024-01-05T13:05:07.123456789[u-ca=hebrew]"))',
	].forEach(add);

	// PlainYearMonth and PlainMonthDay.
	[
		'new Temporal.PlainYearMonth(2024, 2)', 'new Temporal.PlainYearMonth(2024, 13)', 'new Temporal.PlainYearMonth(275760, 9)', 'new Temporal.PlainYearMonth(275760, 10)',
		'new Temporal.PlainYearMonth(-271821, 4)', 'new Temporal.PlainYearMonth(-271821, 3)', 'new Temporal.PlainYearMonth(2024, 2, "gregory", 15)',
		'Temporal.PlainYearMonth.from("2024-02")', 'Temporal.PlainYearMonth.from("202402")', 'Temporal.PlainYearMonth.from("2024-02-15")',
		'Temporal.PlainYearMonth.from("2024-02-15[u-ca=hebrew]")', 'Temporal.PlainYearMonth.from("2024-02[u-ca=hebrew]")',
		'Temporal.PlainYearMonth.from({year: 2024, month: 2})', 'Temporal.PlainYearMonth.from({year: 5784, monthCode: "M05L", calendar: "hebrew"})',
		'Temporal.PlainYearMonth.from("2024-02").add({months: 11})', 'Temporal.PlainYearMonth.from("2024-02").add({days: 31})',
		'Temporal.PlainYearMonth.from("2024-02").subtract({years: 1, months: 1})', 'Temporal.PlainYearMonth.from("2024-02").until("2026-01")',
		'Temporal.PlainYearMonth.from("2024-02").until("2026-01", {largestUnit: "months"})', 'Temporal.PlainYearMonth.from("2024-02").until("2026-01", {smallestUnit: "years", roundingMode: "halfExpand"})',
		'Temporal.PlainYearMonth.from("2024-02").until("2026-01", {largestUnit: "days"})', 'Temporal.PlainYearMonth.from("2024-02").with({month: 12})',
		'Temporal.PlainYearMonth.from("2024-02").toPlainDate({day: 30})', 'Temporal.PlainYearMonth.from("2024-02").toPlainDate({})',
		'(d => [d.year, d.month, d.monthCode, d.daysInMonth, d.daysInYear, d.monthsInYear, d.inLeapYear, d.era, d.eraYear].join())(Temporal.PlainYearMonth.from({year: 2024, month: 2, calendar: "gregory"}))',
		'Temporal.PlainYearMonth.compare("2024-02", "2024-01")', 'Temporal.PlainYearMonth.from("2024-02").equals("2024-02")',
		'Temporal.PlainYearMonth.from("2024-02").toString({calendarName: "always"})',
		'Temporal.PlainYearMonth.from({year: 2024, month: 2, calendar: "japanese"}).toString()',
		'new Temporal.PlainMonthDay(2, 29)', 'new Temporal.PlainMonthDay(2, 30)', 'new Temporal.PlainMonthDay(2, 29, "iso8601", 2023)',
		'Temporal.PlainMonthDay.from("02-29")', 'Temporal.PlainMonthDay.from("--02-29")', 'Temporal.PlainMonthDay.from("0229")', 'Temporal.PlainMonthDay.from("2023-02-28")',
		'Temporal.PlainMonthDay.from({month: 2, day: 30})', 'Temporal.PlainMonthDay.from({month: 2, day: 30}, {overflow: "reject"})',
		'Temporal.PlainMonthDay.from({year: 2023, month: 2, day: 29})', 'Temporal.PlainMonthDay.from({monthCode: "M05L", day: 1, calendar: "hebrew"})',
		'Temporal.PlainMonthDay.from({monthCode: "M01", day: 30, calendar: "chinese"})', 'Temporal.PlainMonthDay.from({month: 1, day: 1, calendar: "hebrew"})',
		'Temporal.PlainMonthDay.from("02-29").with({day: 1})', 'Temporal.PlainMonthDay.from("02-29").with({month: 4, day: 31})',
		'Temporal.PlainMonthDay.from("02-29").toPlainDate({year: 2024})', 'Temporal.PlainMonthDay.from("02-29").toPlainDate({year: 2025})',
		'Temporal.PlainMonthDay.from("02-29").equals("02-29")', 'Temporal.PlainMonthDay.from("02-29").toString({calendarName: "always"})',
		'Temporal.PlainMonthDay.from({monthCode: "M02", day: 29, calendar: "gregory"}).toString()',
		'Temporal.PlainMonthDay.from("1972-02-29[u-ca=gregory]")',
	].forEach(add);

	// Instant.
	[
		'new Temporal.Instant(0n)', 'new Temporal.Instant(8640000000000000000000n)', 'new Temporal.Instant(8640000000000000000001n)', 'new Temporal.Instant(-1n)',
		'new Temporal.Instant(0)', 'new Temporal.Instant("123")', 'new Temporal.Instant(true)', 'Temporal.Instant.fromEpochMilliseconds(8.64e15)',
		'Temporal.Instant.fromEpochMilliseconds(8.64e15 + 1)', 'Temporal.Instant.fromEpochMilliseconds(1.5)', 'Temporal.Instant.fromEpochMilliseconds(-1)',
		'Temporal.Instant.fromEpochNanoseconds(-123456789n)', 'Temporal.Instant.from("2024-01-05T13:05Z")', 'Temporal.Instant.from("2024-01-05T13:05+05:30")',
		'Temporal.Instant.from("2024-01-05T13:05+05:30:15.5")', 'Temporal.Instant.from("2024-01-05T13:05")', 'Temporal.Instant.from("2024-01-05T13:05Z[Asia/Tokyo]")',
		'Temporal.Instant.from("2024-01-05T13:05-00:00[Asia/Tokyo]")', 'Temporal.Instant.from("2024-01-05T24:00Z")', 'Temporal.Instant.from("2024-01-05T23:59:60Z")',
		'Temporal.Instant.from("-271821-04-20T00:00Z")', 'Temporal.Instant.from("-271821-04-19T23:59:59.999999999Z")', 'Temporal.Instant.from("+275760-09-13T00:00Z")',
		'Temporal.Instant.from("+275760-09-13T00:00:00.000000001Z")', 'Temporal.Instant.from("2024-01-05T13:05Z").add({hours: 25})',
		'Temporal.Instant.from("2024-01-05T13:05Z").add({days: 1})', 'Temporal.Instant.from("2024-01-05T13:05Z").subtract("PT1.5S")',
		'Temporal.Instant.from("2024-01-05T13:05Z").until("2025-01-05T13:05Z")', 'Temporal.Instant.from("2024-01-05T13:05Z").until("2025-01-05T13:05Z", {largestUnit: "hours"})',
		'Temporal.Instant.from("2024-01-05T13:05Z").until("2025-01-05T13:05Z", {largestUnit: "days"})',
		'Temporal.Instant.from("2024-01-05T13:05Z").since("2024-01-05T13:04:59.123Z", {smallestUnit: "milliseconds", largestUnit: "seconds"})',
		'Temporal.Instant.from("2024-01-05T13:05:59.5Z").round("minute")', 'Temporal.Instant.from("2024-01-05T13:05Z").round({smallestUnit: "hour", roundingIncrement: 5})',
		'Temporal.Instant.from("2024-01-05T13:05Z").round({smallestUnit: "hour", roundingIncrement: 6})', 'Temporal.Instant.from("2024-01-05T13:05Z").round({smallestUnit: "day"})',
		'Temporal.Instant.from("2024-01-05T13:05Z").round({smallestUnit: "minute", roundingIncrement: 1440})',
		'Temporal.Instant.from("2024-01-05T13:05:07.123456789Z").toString({timeZone: "Asia/Kolkata"})',
		'Temporal.Instant.from("2024-01-05T13:05:07.123456789Z").toString({timeZone: "-03:30", smallestUnit: "second"})',
		'Temporal.Instant.from("2024-01-05T13:05:07.123456789Z").toString({smallestUnit: "hour"})',
		'Temporal.Instant.from("2024-01-05T13:05:07.123456789Z").toString({fractionalSecondDigits: 1, roundingMode: "ceil"})',
		'Temporal.Instant.from("2024-01-05T13:05:07Z").toZonedDateTimeISO("Australia/Lord_Howe")',
		'Temporal.Instant.compare("2024-01-05T13:05Z", "2024-01-05T14:05+01:00")', 'Temporal.Instant.from("2024-01-05T13:05Z").equals("2024-01-05T14:05+01:00")',
		'Temporal.Instant.from(Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo]"))', 'Temporal.Instant.from({})',
		'Temporal.Instant.from({toString() { return "2024-01-05T13:05Z"; }})', 'String(Temporal.Instant.from("1969-12-31T23:59:59.999999999Z").epochNanoseconds)',
		'String(Temporal.Instant.from("1969-12-31T23:59:59.999999999Z").epochMilliseconds)',
		'new Date(-1).toTemporalInstant()', 'new Date(8.64e15).toTemporalInstant()', 'new Date(NaN).toTemporalInstant()',
	].forEach(add);

	// ZonedDateTime.
	[
		'new Temporal.ZonedDateTime(0n, "UTC")', 'new Temporal.ZonedDateTime(0n, "utc")', 'new Temporal.ZonedDateTime(0n, "Etc/UTC")', 'new Temporal.ZonedDateTime(0n, "+01:00")',
		'new Temporal.ZonedDateTime(0n, "+01:00:30")', 'new Temporal.ZonedDateTime(0n, "Asia/Calcutta")', 'new Temporal.ZonedDateTime(0n, "america/new_york")',
		'new Temporal.ZonedDateTime(0n, "Mars/Olympus")', 'new Temporal.ZonedDateTime(0n, "2024-01-05T13:05Z[Asia/Tokyo]")', 'new Temporal.ZonedDateTime(0n, "UTC", "japanese")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo]")', 'Temporal.ZonedDateTime.from("2024-01-05T13:05+09:00[Asia/Tokyo]")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05+08:00[Asia/Tokyo]")', 'Temporal.ZonedDateTime.from("2024-01-05T13:05+08:00[Asia/Tokyo]", {offset: "prefer"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05+08:00[Asia/Tokyo]", {offset: "ignore"})', 'Temporal.ZonedDateTime.from("2024-01-05T13:05+08:00[Asia/Tokyo]", {offset: "use"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05Z[Asia/Tokyo]")', 'Temporal.ZonedDateTime.from("2024-01-05T13:05[!Asia/Tokyo]")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo][u-ca=japanese]")', 'Temporal.ZonedDateTime.from("2024-01-05T13:05")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[+05:30]")', 'Temporal.ZonedDateTime.from("2024-01-05T13:05+05:30:00.5[+05:30]")',
		'Temporal.ZonedDateTime.from("2024-03-10T02:30[America/New_York]")', 'Temporal.ZonedDateTime.from("2024-03-10T02:30[America/New_York]", {disambiguation: "earlier"})',
		'Temporal.ZonedDateTime.from("2024-03-10T02:30[America/New_York]", {disambiguation: "reject"})',
		'Temporal.ZonedDateTime.from("2024-11-03T01:30-04:00[America/New_York]")', 'Temporal.ZonedDateTime.from("2024-11-03T01:30-05:00[America/New_York]")',
		'Temporal.ZonedDateTime.from("2024-11-03T01:30[America/New_York]", {disambiguation: "later"})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, timeZone: "Asia/Tokyo"})', 'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, hour: 13, offset: "+08:00", timeZone: "Asia/Tokyo"})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, hour: 13, offset: "+08:00", timeZone: "Asia/Tokyo"}, {offset: "prefer"})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, offset: "+8", timeZone: "Asia/Tokyo"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").with({hour: 25})', 'Temporal.ZonedDateTime.from("2024-03-31T01:30[Europe/Paris]").with({hour: 2})',
		'Temporal.ZonedDateTime.from("2024-10-27T02:30+02:00[Europe/Paris]").with({minute: 45})', 'Temporal.ZonedDateTime.from("2024-10-27T02:30+01:00[Europe/Paris]").with({minute: 45})',
		'Temporal.ZonedDateTime.from("2024-10-27T02:30+02:00[Europe/Paris]").with({offset: "+01:00"})',
		'Temporal.ZonedDateTime.from("2024-10-27T02:30+02:00[Europe/Paris]").with({offset: "+05:00"}, {offset: "reject"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").with({timeZone: "UTC"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").withTimeZone("America/Los_Angeles")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").withPlainTime("23:59")', 'Temporal.ZonedDateTime.from("2024-03-31T13:05[Europe/Paris]").withPlainTime("02:30")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").withCalendar("islamic-civil")',
		'Temporal.ZonedDateTime.from("2024-03-09T02:30[America/New_York]").add({days: 1})', 'Temporal.ZonedDateTime.from("2024-03-09T02:30[America/New_York]").add({hours: 24})',
		'Temporal.ZonedDateTime.from("2024-03-09T02:30[America/New_York]").add({days: 1}, {overflow: "reject"})',
		'Temporal.ZonedDateTime.from("2024-01-31T12:00[America/New_York]").add({months: 1}, {overflow: "reject"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").subtract({years: 1, nanoseconds: 1})',
		'Temporal.ZonedDateTime.from("2024-03-09T12:00[America/New_York]").until("2024-03-11T12:00[America/New_York]")',
		'Temporal.ZonedDateTime.from("2024-03-09T12:00[America/New_York]").until("2024-03-11T12:00[America/New_York]", {largestUnit: "days"})',
		'Temporal.ZonedDateTime.from("2024-03-09T12:00[America/New_York]").until("2024-03-11T12:00[America/New_York]", {largestUnit: "years", smallestUnit: "hours"})',
		'Temporal.ZonedDateTime.from("2024-03-09T12:00[America/New_York]").until("2024-03-11T12:00[Europe/Paris]", {largestUnit: "days"})',
		'Temporal.ZonedDateTime.from("2024-03-09T12:00[America/New_York]").until("2024-03-11T12:00[Europe/Paris]")',
		'Temporal.ZonedDateTime.from("2024-01-31T12:00[America/New_York]").since("2023-02-28T13:00[America/New_York]", {largestUnit: "months", smallestUnit: "days", roundingMode: "ceil"})',
		'Temporal.ZonedDateTime.from("2024-03-10T12:00[America/New_York]").round("day")', 'Temporal.ZonedDateTime.from("2024-03-10T11:30[America/New_York]").round("day")',
		'Temporal.ZonedDateTime.from("2024-03-10T13:05:07.5[America/New_York]").round({smallestUnit: "second", roundingMode: "floor"})',
		'Temporal.ZonedDateTime.from("2024-03-10T13:05[America/New_York]").round({smallestUnit: "hour", roundingIncrement: 3})',
		'Temporal.ZonedDateTime.from("2024-03-10T13:05[America/New_York]").round({smallestUnit: "day", roundingIncrement: 2})',
		'Temporal.ZonedDateTime.from("2024-03-10T13:05[America/New_York]").hoursInDay', 'Temporal.ZonedDateTime.from("2024-11-03T13:05[America/New_York]").hoursInDay',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Australia/Lord_Howe]").hoursInDay', 'Temporal.ZonedDateTime.from("2024-04-07T13:05[Australia/Lord_Howe]").hoursInDay',
		'Temporal.ZonedDateTime.from("2011-12-30T12:00[Pacific/Apia]").hoursInDay', 'Temporal.ZonedDateTime.from("2011-12-29T12:00[Pacific/Apia]").add({days: 1})',
		'Temporal.ZonedDateTime.from("2024-03-10T13:05[America/New_York]").startOfDay()', 'Temporal.ZonedDateTime.from("1919-03-30T12:00[America/Toronto]").startOfDay()',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[America/New_York]").getTimeZoneTransition("next")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[America/New_York]").getTimeZoneTransition("previous")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Shanghai]").getTimeZoneTransition("previous")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Shanghai]").getTimeZoneTransition("next")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[+01:00]").getTimeZoneTransition("next")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/London]").getTimeZoneTransition("sideways")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/London]").getTimeZoneTransition()',
		'Temporal.ZonedDateTime.from("1847-01-05T13:05[Europe/London]").getTimeZoneTransition("previous")',
		'Temporal.ZonedDateTime.from("1847-01-05T13:05[Europe/London]").offset',
		'Temporal.ZonedDateTime.from("1900-01-05T13:05[Europe/Amsterdam]").offset',
		'Temporal.ZonedDateTime.from("1900-01-05T13:05[Europe/Amsterdam]").toString()',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").toInstant()', 'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").toPlainDateTime()',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Europe/Paris]").equals("2024-01-05T13:05+01:00[Europe/Paris]")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Kolkata]").equals("2024-01-05T13:05[Asia/Calcutta]")',
		'Temporal.ZonedDateTime.compare("2024-01-05T13:05[Europe/Paris]", "2024-01-05T12:05Z[UTC]")',
		'(z => [z.offsetNanoseconds, z.offset, String(z.epochNanoseconds), z.epochMilliseconds, z.dayOfYear, z.weekOfYear, z.yearOfWeek, z.timeZoneId, z.calendarId].join())(Temporal.ZonedDateTime.from("2024-12-30T13:05:07.123456789[America/St_Johns]"))',
	].forEach(add);
	['{}', '{offset: "never"}', '{timeZoneName: "never"}', '{timeZoneName: "critical"}', '{calendarName: "always"}', '{calendarName: "critical"}',
		'{smallestUnit: "minute"}', '{smallestUnit: "hour"}', '{fractionalSecondDigits: 6, roundingMode: "halfExpand"}', '{offset: "foo"}'].forEach(function (o) {
		add('Temporal.ZonedDateTime.from("2024-01-05T13:05:07.1234567[Asia/Tokyo][u-ca=japanese]").toString(' + o + ')');
	});

	// Duration.
	[
		'new Temporal.Duration()', 'new Temporal.Duration(1, -1)', 'new Temporal.Duration(1.5)', 'new Temporal.Duration(2 ** 32)', 'new Temporal.Duration(0, 0, 0, 0, 0, 0, 2 ** 53)',
		'new Temporal.Duration(0, 0, 0, 104249991375)', 'new Temporal.Duration(0, 0, 0, 104249991374)', 'new Temporal.Duration(0, 0, 0, 0, 0, 0, 0, 0, 0, Number.MAX_SAFE_INTEGER)',
		'new Temporal.Duration(0, 0, 0, 0, 0, 0, 0, 0, 0, 1e30)', 'new Temporal.Duration(-0)', 'Object.is(new Temporal.Duration(-0).years, 0)',
		'Temporal.Duration.from("P1Y2M3W4DT5H6M7.008009010S")', 'Temporal.Duration.from("-P1Y")', 'Temporal.Duration.from("PT1.5H")', 'Temporal.Duration.from("PT1.5H30M")',
		'Temporal.Duration.from("P1.5D")', 'Temporal.Duration.from("pt1m")', 'Temporal.Duration.from("P")', 'Temporal.Duration.from("PT")', 'Temporal.Duration.from("P1Y1Y")',
		'Temporal.Duration.from("PT0.0000000001S")', 'Temporal.Duration.from("PT1,5S")', 'Temporal.Duration.from("+P1D")', 'Temporal.Duration.from("\\u2212P1D")',
		'Temporal.Duration.from({days: 1.5})', 'Temporal.Duration.from({day: 1})', 'Temporal.Duration.from({})', 'Temporal.Duration.from({hours: "2"})',
		'Temporal.Duration.from({years: 1}).with({months: -1})', 'Temporal.Duration.from({years: 1}).with({months: 5})', 'Temporal.Duration.from("P1D").negated()',
		'Temporal.Duration.from("-PT1S").abs()', 'Temporal.Duration.from("P1D").sign', 'Temporal.Duration.from("-P1D").sign', 'Temporal.Duration.from("PT0S").blank',
		'Temporal.Duration.from("PT1H").add("PT59M60S")', 'Temporal.Duration.from("P1D").add("PT1H")', 'Temporal.Duration.from("P1M").add("P1D")',
		'Temporal.Duration.from("P1Y").subtract("P1Y")', 'Temporal.Duration.from("PT36H").round("days")', 'Temporal.Duration.from("PT36H").round({largestUnit: "days"})',
		'Temporal.Duration.from("PT36H").round({largestUnit: "days", smallestUnit: "days"})', 'Temporal.Duration.from("PT36H").round({smallestUnit: "hours", roundingIncrement: 5})',
		'Temporal.Duration.from("PT36H").round({smallestUnit: "hours", roundingIncrement: 6})', 'Temporal.Duration.from("P1Y").round({largestUnit: "days"})',
		'Temporal.Duration.from("P1Y").round({largestUnit: "days", relativeTo: "2024-01-01"})', 'Temporal.Duration.from("P1Y").round({largestUnit: "days", relativeTo: "2023-01-01"})',
		'Temporal.Duration.from("P1M").round({largestUnit: "days", relativeTo: "2024-02-01T00:00[America/New_York]"})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: "2024-03-10T00:00[America/New_York]"})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: "2024-03-10T00:00-05:00[America/New_York]"})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: "2024-03-10T00:00+01:00[America/New_York]"})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: {year: 2024, month: 3, day: 10, timeZone: "America/New_York"}})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: {year: 2024, month: 3, day: 10}})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: "2024-03-10T00:00Z"})',
		'Temporal.Duration.from("P1D").round({largestUnit: "hours", relativeTo: "2024-03-10T00:00"})',
		'Temporal.Duration.from("P1Y2M3W4D").round({smallestUnit: "months", relativeTo: "2024-01-31"})',
		'Temporal.Duration.from("P1Y2M3W4D").round({largestUnit: "weeks", relativeTo: "2024-01-31"})',
		'Temporal.Duration.from("P1Y2M3W4D").round({smallestUnit: "years", roundingMode: "halfEven", relativeTo: "2024-01-31"})',
		'Temporal.Duration.from("PT100H").round({largestUnit: "months", relativeTo: "2024-01-31"})', 'Temporal.Duration.from("PT1H").round({})',
		'Temporal.Duration.from("P1D").total("hours")', 'Temporal.Duration.from("P1M").total("days")', 'Temporal.Duration.from("P1M").total({unit: "days", relativeTo: "2024-02-01"})',
		'Temporal.Duration.from("P1M").total({unit: "days", relativeTo: "2023-02-01"})', 'Temporal.Duration.from("PT90M").total("hours")', 'Temporal.Duration.from("P1Y").total({unit: "months", relativeTo: "2024-01-01"})',
		'Temporal.Duration.from("P1D").total({unit: "hours", relativeTo: "2024-11-03T00:00[America/New_York]"})', 'Temporal.Duration.from("PT1.5S").total("milliseconds")',
		'Temporal.Duration.from("P1Y1M1W1D").total({unit: "weeks", relativeTo: "2024-01-31"})',
		'Temporal.Duration.compare("PT1H", "PT60M")', 'Temporal.Duration.compare("P1D", "PT24H")', 'Temporal.Duration.compare("P1M", "P30D")',
		'Temporal.Duration.compare("P1M", "P30D", {relativeTo: "2024-02-01"})', 'Temporal.Duration.compare("P1D", "PT24H", {relativeTo: "2024-03-10T00:00[America/New_York]"})',
		'Temporal.Duration.compare("P1D", "PT24H", 1)',
	].forEach(add);
	['{}', '{fractionalSecondDigits: 2}', '{fractionalSecondDigits: 0, roundingMode: "halfExpand"}', '{smallestUnit: "seconds"}', '{smallestUnit: "minutes"}',
		'{smallestUnit: "milliseconds", roundingMode: "ceil"}', '{fractionalSecondDigits: "auto"}'].forEach(function (o) {
		add('Temporal.Duration.from({hours: 1, minutes: 2, seconds: 59, milliseconds: 999, microseconds: 999, nanoseconds: 999}).toString(' + o + ')');
		add('Temporal.Duration.from({seconds: -0.5e9, nanoseconds: -1}).toString(' + o + ')');
	});
	['PT0S', 'P1Y', '-P1Y2M3W4DT5H6M7.008009010S', 'PT0.000001S', 'PT1000S', 'P0D', 'PT9007199254740991S'].forEach(function (d) {
		add('Temporal.Duration.from("' + d + '").toJSON()');
	});
	add('Temporal.Duration.from({milliseconds: Number.MAX_SAFE_INTEGER, microseconds: Number.MAX_SAFE_INTEGER, nanoseconds: Number.MAX_SAFE_INTEGER}).toString()');
	add('Temporal.Duration.from({seconds: Number.MAX_SAFE_INTEGER}).add({seconds: 1})');

	// Now: what does not depend on the clock.
	add('Temporal.Now.timeZoneId()');
	add('Temporal.Now.zonedDateTimeISO("Mars/Olympus")');
	add('typeof Temporal.Now.instant().epochNanoseconds');
	add('Temporal.Now.plainDateISO().calendarId');
	add('Temporal.Now.zonedDateTimeISO().timeZoneId');
	add('Temporal.Now.zonedDateTimeISO("+05:00").offset');

	// Conversions and errors of arguments.
	[
		'Temporal.PlainDate.from({year: 2024n, month: 1, day: 1})', 'Temporal.PlainDate.from({year: Symbol(), month: 1, day: 1})',
		'Temporal.PlainDate.from({year: "2024", month: "1", day: "1"})', 'Temporal.PlainDate.from({year: {valueOf() { return 2024; }}, month: 1, day: 1})',
		'Temporal.PlainDate.from({year: 2024, monthCode: 2, day: 1})', 'Temporal.PlainDate.from({year: 2024, monthCode: "m02", day: 1})',
		'Temporal.PlainDate.from({year: 2024, monthCode: "M00L", day: 1})', 'Temporal.PlainDate.from({year: 2024, month: 1, day: 1, calendar: 1})',
		'Temporal.PlainDate.from({year: 2024, month: 1, day: 1, calendar: "2024-01-01[u-ca=hebrew]"})',
		'Temporal.PlainDate.from({year: 2024, month: 1, day: 1, calendar: Temporal.PlainDate.from("2024-01-01[u-ca=hebrew]")})',
		'Temporal.PlainDate.from("2024-01-05").withCalendar("2024-01-01[u-ca=hebrew]")', 'Temporal.PlainDate.from("2024-01-05").withCalendar("islamic")',
		'Temporal.PlainDate.from("2024-01-05").withCalendar("islamicc")', 'Temporal.PlainDate.from("2024-01-05").withCalendar("ethiopic-amete-alem")',
		'Temporal.PlainDate.from("2024-01-05").until("2024-02-05", {largestUnit: "fortnights"})', 'Temporal.PlainDate.from("2024-01-05").until("2024-02-05", {largestUnit: "auto"})',
		'Temporal.PlainDate.from("2024-01-05").until("2024-02-05", {roundingIncrement: 1e9 + 1, smallestUnit: "days"})',
		'Temporal.PlainDate.from("2024-01-05").until("2024-02-05", {roundingMode: "HALFEXPAND"})',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo]").toString({timeZoneName: "foo"})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, timeZone: 1})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, timeZone: "2024-01-05T13:05Z"})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, timeZone: "2024-01-05T13:05"})',
		'Temporal.ZonedDateTime.from({year: 2024, month: 1, day: 5, timeZone: Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo]")})',
		'Temporal.Instant.from("2024-01-05T13:05Z").toString({timeZone: "Asia/Tokyo[u-ca=japanese]"})',
		'Temporal.PlainTime.from("13:05").toString({smallestUnit: "day"})', 'Temporal.PlainDate.prototype.toString.call(1)',
		'Object.getOwnPropertyDescriptor(Temporal.Instant.prototype, "epochNanoseconds").get.call(Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo]"))',
		'Temporal.PlainDate.from("2024-01-05") + ""', 'JSON.stringify(Temporal.ZonedDateTime.from("2024-01-05T13:05[Asia/Tokyo]"))',
		'Temporal.PlainDate.from("2024-01-05") < Temporal.PlainDate.from("2024-01-06")', '`${Temporal.Duration.from("PT1H")}`',
	].forEach(add);

	// toLocaleString.
	var values = [
		'Temporal.PlainDate.from("2024-01-05")', 'Temporal.PlainTime.from("13:05:07")', 'Temporal.PlainDateTime.from("2024-01-05T13:05:07")',
		'Temporal.ZonedDateTime.from("2024-01-05T13:05:07[America/New_York]")', 'Temporal.Instant.from("2024-01-05T13:05:07Z")',
		'Temporal.PlainYearMonth.from({year: 2024, month: 1, calendar: "gregory"})', 'Temporal.PlainMonthDay.from({monthCode: "M01", day: 5, calendar: "gregory"})',
	];
	var options = ['', '"en-US"', '"de"', '"ja"', '"en-US", {dateStyle: "full"}', '"en-US", {timeStyle: "short"}', '"en-US", {dateStyle: "medium", timeStyle: "medium"}',
		'"en-US", {year: "numeric"}', '"en-US", {hour: "numeric"}', '"en-US", {timeZoneName: "long"}', '"en-US", {era: "short"}', '"en-US", {timeZone: "Asia/Tokyo"}',
		'"zh-CN", {month: "long", day: "numeric"}', '"en-US", {weekday: "long"}'];
	values.forEach(function (v) {
		options.forEach(function (o) {
			add(v + '.toLocaleString(' + o + ')');
		});
	});
	add('Temporal.PlainYearMonth.from("2024-01").toLocaleString("en-US")');
	add('Temporal.PlainMonthDay.from("01-05").toLocaleString("en-US")');
	add('Temporal.PlainDate.from("2024-01-05[u-ca=japanese]").toLocaleString("en-US")');
	add('Temporal.PlainDate.from("2024-01-05[u-ca=japanese]").toLocaleString("ja-JP-u-ca-japanese")');
	add('Temporal.ZonedDateTime.from("2024-01-05T13:05:07+05:30[Asia/Kolkata]").toLocaleString("en-IN")');
	add('Temporal.ZonedDateTime.from("2024-01-05T13:05:07[+05:30]").toLocaleString("en-US")');
	add('Temporal.Duration.from("P1DT2H").toLocaleString("en-US")');
})();

function answer(e) {
	try {
		var v = (0, eval)(e);
		return typeof v === 'string' ? v : typeof v + ':' + String(v);
	} catch (x) {
		return 'throws ' + (x && x.name) + ': ' + (x && x.message);
	}
}
