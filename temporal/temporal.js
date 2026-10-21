// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// The javascript half of package temporal. It owns the surface of Temporal:
// the constructors, their prototypes, and the reading of arguments, options
// and property bags, in the order V8 reads them and with its conversions,
// which test262 checks. Everything else is go-intl's, through the bridge
// (bridge.go), one Builtin whose first argument names the operation. It
// evaluates to a function of the built-ins stub.js took before the page ran,
// the names of the operations, the bridge, the options of
// Intl.DateTimeFormat and the Builtin formatting a Temporal value with them,
// and the VM's default time zone, which returns what stub.js puts in place.
//
// Nothing here may depend on what a page can change: it uses only what it
// is called with, reads only own properties of its own objects, and builds
// the arrays it passes on without a prototype.

(function (P, opNames, T, dtfOptions, format, defaultZone, dfOptions, dfResolve, dfFormat, state) {
	'use strict';

	const {
		apply, defineProperty, getOwnPropertyDescriptor, ownKeys, setPrototypeOf, parse,
		Object, Number, String, BigInt, Error, TypeError, RangeError, toStringTag, toPrimitive, dateValueOf, floor, stringSlice
	} = P;

	// O maps the name of an operation of the bridge to its index.
	const O = setPrototypeOf({}, null);
	{
		const names = parse(opNames);
		for (let i = 0; i < names.length; i++) {
			O[names[i]] = i;
		}
	}
	const dtfList = parse(dtfOptions), dfList = parse(dfOptions);
	const noArgs = bare([]);
	const noOptions = setPrototypeOf({}, null);

	function bare(a) {
		return setPrototypeOf(a, null);
	}

	// call calls the bridge with the arguments in a, a bare array.
	function call(a) {
		return apply(T, undefined, a);
	}

	function push(a, v) {
		a[a.length] = v;
		return a;
	}

	// Slots holds the internal slots of the objects Temporal makes, a record
	// with their kind, in a private field nothing outside this function can
	// read, forge or replace (see the same class in package intl).
	class Stamp {
		constructor(o) {
			return o;
		}
	}

	class Slots extends Stamp {
		#slots;

		constructor(o, slots) {
			super(o);
			this.#slots = slots;
		}

		static of(o) {
			return typeof o === 'object' && o !== null && #slots in o ? o.#slots : undefined;
		}
	}

	const KD = 'PlainDate', KT = 'PlainTime', KDT = 'PlainDateTime', KYM = 'PlainYearMonth', KMD = 'PlainMonthDay',
		KI = 'Instant', KZ = 'ZonedDateTime', KDU = 'Duration';

	// protos are the prototypes of the kinds, which make() gives the objects
	// a method returns.
	const protos = setPrototypeOf({}, null);

	function make(rec) {
		return new Slots({__proto__: protos[rec.kind]}, rec);
	}

	function slot(o, kind, name) {
		const s = Slots.of(o);
		if (s === undefined || s.kind !== kind) {
			throw new TypeError('Method Temporal.' + kind + '.prototype.' + name + ' called on incompatible receiver ' +
				describe(o));
		}
		return s;
	}

	// describe is a value as V8 names it in an error, as far as that needs
	// nothing a page could see: an object other than Temporal's is
	// #<Object>, whatever its constructor.
	function describe(v) {
		const s = Slots.of(v);
		if (s !== undefined) {
			return '[object Temporal.' + s.kind + ']';
		}
		switch (typeof v) {
		case 'object':
			return v === null ? 'null' : '#<Object>';
		case 'function':
			return '#<Function>';
		case 'symbol':
			return String(v);
		}
		return '' + v;
	}

	function isObject(v) {
		return typeof v === 'object' ? v !== null : typeof v === 'function';
	}

	// ------------------------------------------------------------- records
	//
	// The records of the kinds, made of what the bridge returns: a date's
	// ISO year, month and day and calendar, a time's hour to nanosecond, an
	// instant's epoch milliseconds, floored, and the nanoseconds past them,
	// a zoned date-time's instant, offset, wall clock, zone and calendar,
	// and a duration's ten fields.

	function recDate(a) {
		return {__proto__: null, kind: KD, y: a[0], m: a[1], d: a[2], cal: a[3], info: undefined};
	}

	function recTime(a) {
		return {__proto__: null, kind: KT, h: a[0], mi: a[1], s: a[2], ms: a[3], us: a[4], ns: a[5]};
	}

	function recDateTime(a) {
		return {
			__proto__: null, kind: KDT, y: a[0], m: a[1], d: a[2], h: a[3], mi: a[4], s: a[5], ms: a[6], us: a[7], ns: a[8],
			cal: a[9], info: undefined
		};
	}

	function recYearMonth(a) {
		return {__proto__: null, kind: KYM, y: a[0], m: a[1], d: a[2], cal: a[3], info: undefined};
	}

	function recMonthDay(a) {
		return {__proto__: null, kind: KMD, y: a[0], m: a[1], d: a[2], cal: a[3], info: undefined};
	}

	function recInstant(ems, sub) {
		return {__proto__: null, kind: KI, ems: ems, sub: sub};
	}

	function recZoned(a) {
		return {
			__proto__: null, kind: KZ, ems: a[0], sub: a[1], off: a[2], offset: a[3], y: a[4], m: a[5], d: a[6],
			h: a[7], mi: a[8], s: a[9], ms: a[10], us: a[11], ns: a[12], tz: a[13], cal: a[14], info: undefined
		};
	}

	function recDuration(a) {
		const f = bare([]);
		for (let i = 0; i < 10; i++) {
			f[i] = a[i];
		}
		return {__proto__: null, kind: KDU, f: f};
	}

	function dateOf(s) {
		return {__proto__: null, kind: KD, y: s.y, m: s.m, d: s.d, cal: s.cal, info: undefined};
	}

	function timeOf(s) {
		return {__proto__: null, kind: KT, h: s.h, mi: s.mi, s: s.s, ms: s.ms, us: s.us, ns: s.ns};
	}

	// pushDate and the others append a record's fields as the bridge takes
	// them.
	function pushDate(a, s) {
		a[a.length] = s.y;
		a[a.length] = s.m;
		a[a.length] = s.d;
		a[a.length] = s.cal;
		return a;
	}

	function pushTime(a, s) {
		a[a.length] = s.h;
		a[a.length] = s.mi;
		a[a.length] = s.s;
		a[a.length] = s.ms;
		a[a.length] = s.us;
		a[a.length] = s.ns;
		return a;
	}

	function pushNoTime(a) {
		for (let i = 0; i < 6; i++) {
			a[a.length] = undefined;
		}
		return a;
	}

	function pushDateTime(a, s) {
		a[a.length] = s.y;
		a[a.length] = s.m;
		a[a.length] = s.d;
		pushTime(a, s);
		a[a.length] = s.cal;
		return a;
	}

	function pushZoned(a, s) {
		a[a.length] = s.ems;
		a[a.length] = s.sub;
		a[a.length] = s.tz;
		a[a.length] = s.cal;
		return a;
	}

	function pushDuration(a, f) {
		for (let i = 0; i < 10; i++) {
			a[a.length] = f[i];
		}
		return a;
	}

	// pushFields appends a property bag's calendar fields, then pushTimeFields
	// its time fields, as prepareFields read them.
	function pushFields(a, r) {
		a[a.length] = r.day;
		a[a.length] = r.era;
		a[a.length] = r.eraYear;
		a[a.length] = r.month;
		a[a.length] = r.monthCode;
		a[a.length] = r.year;
		return a;
	}

	function pushTimeFields(a, r) {
		a[a.length] = r.hour;
		a[a.length] = r.minute;
		a[a.length] = r.second;
		a[a.length] = r.millisecond;
		a[a.length] = r.microsecond;
		a[a.length] = r.nanosecond;
		return a;
	}

	// checked returns what an operation returned, or throws the error it
	// deferred (see bridge.go): an array of its name and message.
	function checked(r) {
		if (typeof r[0] === 'string') {
			const name = r[0], msg = r[1];
			throw name === 'RangeError' ? new RangeError(msg) : name === 'TypeError' ? new TypeError(msg) : new Error(msg);
		}
		return r;
	}

	// --------------------------------------------------------- conversions
	//
	// +v is ToNumber, which throws for a BigInt or a Symbol where Number()
	// converts them, and toStr ToString.

	function rangeError(msg) {
		return new RangeError('Temporal error: ' + msg);
	}

	function typeError(msg) {
		return new TypeError('Temporal error: ' + msg);
	}

	// toStr is ToString. Not `${v}`, which QuickJS compiles to a call of
	// String.prototype.concat, a page's to change, and not String(v), which
	// converts a Symbol.
	function toStr(v) {
		if (isObject(v)) {
			v = toPrim(v, 'string');
		}
		if (typeof v === 'symbol') {
			throw new TypeError('Cannot convert a Symbol value to a string');
		}
		return '' + v;
	}

	// toNumber is ToNumber, with V8's errors.
	function toNumber(v) {
		if (isObject(v)) {
			v = toPrim(v, 'number');
		}
		switch (typeof v) {
		case 'bigint':
			throw new TypeError('Cannot convert a BigInt value to a number');
		case 'symbol':
			throw new TypeError('Cannot convert a Symbol value to a number');
		}
		return +v;
	}

	// toInteger is ToIntegerWithTruncation.
	function toInteger(v) {
		const n = toNumber(v);
		if (n !== n || n === 1 / 0 || n === -1 / 0) {
			throw rangeError('Expected finite integer.');
		}
		return n - n % 1 + 0;
	}

	// toIntegral is ToIntegerIfIntegral.
	function toIntegral(v) {
		const n = toNumber(v);
		if (n !== n || n === 1 / 0 || n === -1 / 0 || n % 1 !== 0) {
			throw rangeError('Expected finite integer.');
		}
		return n;
	}

	// toPositive is ToPositiveIntegerWithTruncation.
	function toPositive(v) {
		const n = toInteger(v);
		if (n <= 0) {
			throw rangeError('Expected positive integer.');
		}
		return n;
	}

	const hintString = bare(['string']);
	const hintNumber = bare(['number']);
	const stringOrder = bare(['toString', 'valueOf']);
	const numberOrder = bare(['valueOf', 'toString']);

	// toPrim is ToPrimitive(v, hint).
	function toPrim(v, hint) {
		if (!isObject(v)) {
			return v;
		}
		const exotic = v[toPrimitive];
		if (exotic !== undefined && exotic !== null) {
			if (typeof exotic !== 'function') {
				throw new TypeError('Symbol.toPrimitive is not a function');
			}
			const r = apply(exotic, v, hint === 'string' ? hintString : hintNumber);
			if (isObject(r)) {
				throw new TypeError('Cannot convert object to primitive value');
			}
			return r;
		}
		const order = hint === 'string' ? stringOrder : numberOrder;
		for (let i = 0; i < 2; i++) {
			const f = v[order[i]];
			if (typeof f === 'function') {
				const r = apply(f, v, noArgs);
				if (!isObject(r)) {
					return r;
				}
			}
		}
		throw new TypeError('Cannot convert object to primitive value');
	}

	// toBigInt is ToBigInt.
	function toBigInt(v) {
		const p = toPrim(v, 'number');
		switch (typeof p) {
		case 'bigint':
			return p;
		case 'boolean':
			return p ? 1n : 0n;
		case 'string':
			return BigInt(p);
		}
		throw new TypeError('Cannot convert ' + (typeof p === 'symbol' ? 'a Symbol' : toStr(p)) + ' to a BigInt');
	}

	const nsPerMs = 1000000n;
	const nsLimit = 8640000000000000000000n;

	// instantOfNs is the record of an instant of epoch nanoseconds, which
	// must be within Temporal's range.
	function instantOfNs(ns) {
		if (ns < -nsLimit || ns > nsLimit) {
			throw rangeError('Nanoseconds out of range.');
		}
		let q = ns / nsPerMs, r = ns % nsPerMs;
		if (r < 0n) {
			q -= 1n;
			r += nsPerMs;
		}
		return recInstant(Number(q), Number(r));
	}

	function epochNanoseconds(s) {
		return BigInt(s.ems) * nsPerMs + BigInt(s.sub);
	}

	// ------------------------------------------------------------- options

	function optionsObject(v) {
		if (v === undefined) {
			return noOptions;
		}
		if (isObject(v)) {
			return v;
		}
		throw new TypeError('Options must be an object');
	}

	// stringOption is GetOption of a string among values: the index of the
	// value, def where the option is undefined, which throws where def is -1.
	// where is the method V8 names in its errors, which is not always the
	// method reading the option.
	function stringOption(o, key, values, def, where) {
		const v = o[key];
		if (v === undefined) {
			if (def < 0) {
				throw new RangeError('Value undefined out of range for ' + where + ' options property ' + key);
			}
			return def;
		}
		return indexOf(toStr(v), key, values, where);
	}

	// indexOf is the index of s in values, which it must be among.
	function indexOf(s, key, values, where) {
		for (let i = 0; i < values.length; i++) {
			if (values[i] === s) {
				return i;
			}
		}
		throw new RangeError('Value ' + s + ' out of range for ' + where + ' options property ' + key);
	}

	const overflows = bare(['constrain', 'reject']);
	const disambiguations = bare(['compatible', 'earlier', 'later', 'reject']);
	const offsetNames = bare(['prefer', 'use', 'ignore', 'reject']);
	// offsetCodes are go-intl's OffsetDisambiguation of offsetNames.
	const offsetCodes = bare([2, 1, 3, 4]);
	const OFFSET_PREFER = 0, OFFSET_REJECT = 3;

	function optionObjectOf(opts, name) {
		if (!isObject(opts)) {
			throw typeError('Option must be object: ' + name + '.');
		}
		return opts;
	}

	// overflow is GetTemporalOverflowOptionHandleUndefined.
	function overflow(opts, where) {
		if (opts === undefined) {
			return 0;
		}
		return stringOption(optionObjectOf(opts, 'overflow'), 'overflow', overflows, 0, where);
	}

	function disambiguation(opts, where) {
		if (opts === undefined) {
			return 0;
		}
		return stringOption(optionObjectOf(opts, 'disambiguation'), 'disambiguation', disambiguations, 0, where);
	}

	function offsetOption(opts, def, where) {
		if (opts === undefined) {
			return offsetCodes[def];
		}
		return offsetCodes[stringOption(optionObjectOf(opts, 'offset'), 'offset', offsetNames, def, where)];
	}

	// zonedOptions are the disambiguation, offset and overflow options,
	// read only where present says.
	function zonedOptions(opts, present, where) {
		if (!present || opts === undefined) {
			return bare([0, offsetCodes[OFFSET_REJECT], 0]);
		}
		const o = optionObjectOf(opts, 'disambiguation');
		const d = stringOption(o, 'disambiguation', disambiguations, 0, where);
		const off = stringOption(o, 'offset', offsetNames, OFFSET_REJECT, where);
		const ov = stringOption(o, 'overflow', overflows, 0, where);
		return bare([d, offsetCodes[off], ov]);
	}

	const unitNames = bare(['year', 'month', 'week', 'day', 'hour', 'minute', 'second', 'millisecond', 'microsecond',
		'nanosecond', 'auto', 'years', 'months', 'weeks', 'days', 'hours', 'minutes', 'seconds', 'milliseconds',
		'microseconds', 'nanoseconds']);
	// unitCodes are go-intl's Unit of unitNames.
	const unitCodes = bare([11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2]);
	const UNIT_AUTO = 1, UNIT_DAY = 8, UNIT_HOUR = 7;
	const GROUP_DATE = 0, GROUP_TIME = 1, GROUP_DATETIME = 2;

	// unitOption is GetTemporalUnitValuedOption: 0 where unset.
	function unitOption(o, key, required, where) {
		const v = o[key];
		if (v === undefined) {
			if (required) {
				throw new RangeError('Value undefined out of range for ' + where + ' options property ' + key);
			}
			return 0;
		}
		return unitCodes[indexOf(toStr(v), key, unitNames, where)];
	}

	// validateUnit is ValidateTemporalUnitValue.
	function validateUnit(u, group, extra) {
		if (u === 0 || extra !== 0 && u === extra) {
			return;
		}
		if (u === UNIT_AUTO) {
			throw rangeError('Auto unit not allowed here');
		}
		if (u >= UNIT_DAY) {
			if (group !== GROUP_TIME) {
				return;
			}
			throw rangeError('Found date unit, expect time unit');
		}
		if (group !== GROUP_DATE) {
			return;
		}
		throw rangeError('Found time unit, expect date unit');
	}

	function roundingIncrement(o) {
		const v = o.roundingIncrement;
		if (v === undefined) {
			return 1;
		}
		const n = toInteger(v);
		if (n < 1 || n > 1e9) {
			throw rangeError('Integer out of range.');
		}
		return n;
	}

	const roundingModes = bare(['ceil', 'floor', 'expand', 'trunc', 'halfCeil', 'halfFloor', 'halfExpand',
		'halfTrunc', 'halfEven']);
	// MODE_TRUNC and MODE_HALF_EXPAND are go-intl's RoundingMode, one more
	// than the index in roundingModes.
	const MODE_TRUNC = 4, MODE_HALF_EXPAND = 7;

	function roundingMode(o, def, where) {
		return stringOption(o, 'roundingMode', roundingModes, def - 1, where) + 1;
	}

	// fractionalSecondDigits is GetTemporalFractionalSecondDigitsOption: -1
	// for auto.
	function fractionalSecondDigits(o) {
		const v = o.fractionalSecondDigits;
		if (v === undefined) {
			return -1;
		}
		if (typeof v !== 'number') {
			if (toStr(v) !== 'auto') {
				throw new RangeError('fractionalSecondDigits value is out of range.');
			}
			return -1;
		}
		if (v !== v || v === 1 / 0 || v === -1 / 0) {
			throw new RangeError('fractionalSecondDigits value is out of range.');
		}
		const d = floor(v);
		if (d < 0 || d > 9) {
			throw new RangeError('fractionalSecondDigits value is out of range.');
		}
		return d;
	}

	const calendarNames = bare(['auto', 'always', 'never', 'critical']);

	function calendarName(o, where) {
		return stringOption(o, 'calendarName', calendarNames, 0, where);
	}

	// toStringOptions reads fractionalSecondDigits, roundingMode and
	// smallestUnit, with between called after the first, and returns them
	// in the order the bridge takes them: digits, unit, mode.
	function toStringOptions(o, between, where) {
		const digits = fractionalSecondDigits(o);
		if (between !== undefined) {
			between();
		}
		const mode = roundingMode(o, MODE_TRUNC, where);
		const smallest = unitOption(o, 'smallestUnit', false, where);
		return bare([digits, smallest, mode]);
	}

	function pushToString(a, t) {
		a[a.length] = t[0];
		a[a.length] = t[1];
		a[a.length] = t[2];
		return a;
	}

	// differenceSettings are until's and since's options: largestUnit,
	// smallestUnit, roundingMode and roundingIncrement, as the bridge takes
	// them.
	function differenceSettings(opts, where) {
		const o = optionsObject(opts);
		const largest = unitOption(o, 'largestUnit', false, where);
		const inc = roundingIncrement(o);
		const mode = roundingMode(o, MODE_TRUNC, where);
		const smallest = unitOption(o, 'smallestUnit', false, where);
		return bare([largest, smallest, mode, inc]);
	}

	function pushSettings(a, s) {
		for (let i = 0; i < 4; i++) {
			a[a.length] = s[i];
		}
		return a;
	}

	// roundTo is round's reading of its argument: a string is the smallest
	// unit.
	function roundTo(v, name) {
		if (v === undefined) {
			throw typeError('Must specify a roundTo parameter.');
		}
		if (typeof v === 'string') {
			return {__proto__: null, smallestUnit: v};
		}
		if (isObject(v)) {
			return v;
		}
		throw name === KI ? new TypeError('Options must be an object') : typeError('roundTo must be an object.');
	}

	// roundingOptions reads round's options: the increment, the mode and the
	// smallest unit, a time unit or extra. They are returned as the bridge
	// takes them: unit, mode, increment.
	function roundingOptions(o, extra, where) {
		const inc = roundingIncrement(o);
		const mode = roundingMode(o, MODE_HALF_EXPAND, where);
		const smallest = unitOption(o, 'smallestUnit', true, where);
		validateUnit(smallest, GROUP_TIME, extra);
		return bare([smallest, mode, inc]);
	}

	function pushRounding(a, r) {
		a[a.length] = r[0];
		a[a.length] = r[1];
		a[a.length] = r[2];
		return a;
	}

	// ------------------------------------------------ calendars and zones

	const eras = setPrototypeOf({iso8601: false, gregory: true}, null);

	function hasEras(cal) {
		let e = eras[cal];
		if (e === undefined) {
			e = T(O.calEras, cal) === 1;
			eras[cal] = e;
		}
		return e;
	}

	// toCalendar is ToTemporalCalendarIdentifier.
	function toCalendar(v) {
		const s = Slots.of(v);
		if (s !== undefined && s.cal !== undefined) {
			return s.cal;
		}
		if (typeof v !== 'string') {
			throw typeError('Calendar must be string or calendared Temporal object.');
		}
		return T(O.calParse, v);
	}

	// calendarArg is a constructor's calendar: ISO where undefined.
	function calendarArg(v) {
		if (v === undefined) {
			return 'iso8601';
		}
		if (typeof v !== 'string') {
			throw typeError('Calendar must be string.');
		}
		return T(O.calCanon, v);
	}

	// calendarOfBag is GetTemporalCalendarIdentifierWithISODefault.
	function calendarOfBag(o) {
		const s = Slots.of(o);
		if (s !== undefined && s.cal !== undefined) {
			return s.cal;
		}
		const c = o.calendar;
		return c === undefined ? 'iso8601' : toCalendar(c);
	}

	// toTimeZone is ToTemporalTimeZoneIdentifier.
	function toTimeZone(v) {
		const s = Slots.of(v);
		if (s !== undefined && s.kind === KZ) {
			return s.tz;
		}
		if (typeof v !== 'string') {
			throw typeError('Time zone must be string or ZonedDateTime object.');
		}
		return T(O.tzParse, v);
	}

	// ------------------------------------------------------- property bags

	const F_DAY = 1, F_MONTH = 2, F_YEAR = 4, F_TIME = 8, F_OFFSET = 16, F_ZONE = 32,
		F_DATE = F_DAY | F_MONTH | F_YEAR;
	const REQUIRE_NONE = 0, REQUIRE_PARTIAL = 1, REQUIRE_ZONE = 2;

	function isDigit(c) {
		return c >= 48 && c <= 57;
	}

	// toMonthCode is V8's ToMonthCode.
	function toMonthCode(v) {
		const p = toPrim(v, 'string');
		if (typeof p !== 'string') {
			throw typeError('Month code out of range.');
		}
		const n = p.length;
		if (n !== 3 && n !== 4 || p[0] !== 'M' || !isDigit(code(p, 1)) || !isDigit(code(p, 2)) ||
			n === 4 && p[3] !== 'L' || p[1] === '0' && p[2] === '0' && n !== 4) {
			throw rangeError('Month code out of range.');
		}
		return p;
	}

	// code is the code unit at i of a string, without its methods.
	function code(s, i) {
		const c = s[i];
		return c === undefined ? -1 : c < '0' ? 0 : c > '9' ? 99 : +c + 48;
	}

	// toOffsetString is V8's ToOffsetString.
	function toOffsetString(v) {
		const p = toPrim(v, 'string');
		if (typeof p !== 'string') {
			throw typeError('Offset must be string.');
		}
		T(O.offsetCheck, p);
		return p;
	}

	// prepareFields is PrepareCalendarFields: the fields asked for, read in
	// the order of their names, each converted as it is read.
	function prepareFields(cal, o, which, require) {
		const r = {
			__proto__: null, day: undefined, era: undefined, eraYear: undefined, month: undefined, monthCode: undefined,
			year: undefined, hour: undefined, minute: undefined, second: undefined, millisecond: undefined,
			microsecond: undefined, nanosecond: undefined, offset: undefined, timeZone: undefined
		};
		const withEras = (which & F_YEAR) !== 0 && hasEras(cal);
		const time = (which & F_TIME) !== 0;
		let found = false, v;
		if (which & F_DAY) {
			v = o.day;
			if (v !== undefined) {
				found = true;
				r.day = toPositive(v);
			}
		}
		if (withEras) {
			v = o.era;
			if (v !== undefined) {
				found = true;
				r.era = toStr(v);
			}
			v = o.eraYear;
			if (v !== undefined) {
				found = true;
				r.eraYear = toInteger(v);
			}
		}
		if (time) {
			v = o.hour;
			if (v !== undefined) {
				found = true;
				r.hour = toInteger(v);
			}
			v = o.microsecond;
			if (v !== undefined) {
				found = true;
				r.microsecond = toInteger(v);
			}
			v = o.millisecond;
			if (v !== undefined) {
				found = true;
				r.millisecond = toInteger(v);
			}
			v = o.minute;
			if (v !== undefined) {
				found = true;
				r.minute = toInteger(v);
			}
		}
		if (which & F_MONTH) {
			v = o.month;
			if (v !== undefined) {
				found = true;
				r.month = toPositive(v);
			}
			v = o.monthCode;
			if (v !== undefined) {
				found = true;
				r.monthCode = toMonthCode(v);
			}
		}
		if (time) {
			v = o.nanosecond;
			if (v !== undefined) {
				found = true;
				r.nanosecond = toInteger(v);
			}
		}
		if (which & F_OFFSET) {
			v = o.offset;
			if (v !== undefined) {
				found = true;
				r.offset = toOffsetString(v);
			}
		}
		if (time) {
			v = o.second;
			if (v !== undefined) {
				found = true;
				r.second = toInteger(v);
			}
		}
		if (which & F_ZONE) {
			v = o.timeZone;
			if (v === undefined) {
				if (require === REQUIRE_ZONE) {
					throw typeError('Must specify time zone.');
				}
			} else {
				found = true;
				r.timeZone = toTimeZone(v);
			}
		}
		if (which & F_YEAR) {
			v = o.year;
			if (v !== undefined) {
				found = true;
				r.year = toInteger(v);
			}
		}
		if (require === REQUIRE_PARTIAL && !found) {
			throw typeError('Must specify at least one calendar field.');
		}
		return r;
	}

	// timeBag reads a property bag's time fields, in the order of their
	// names, each converted as it is read: undefined where it has none.
	function timeBag(o) {
		const r = {
			__proto__: null, hour: undefined, minute: undefined, second: undefined, millisecond: undefined,
			microsecond: undefined, nanosecond: undefined
		};
		let found = false, v;
		v = o.hour;
		if (v !== undefined) {
			found = true;
			r.hour = toInteger(v);
		}
		v = o.microsecond;
		if (v !== undefined) {
			found = true;
			r.microsecond = toInteger(v);
		}
		v = o.millisecond;
		if (v !== undefined) {
			found = true;
			r.millisecond = toInteger(v);
		}
		v = o.minute;
		if (v !== undefined) {
			found = true;
			r.minute = toInteger(v);
		}
		v = o.nanosecond;
		if (v !== undefined) {
			found = true;
			r.nanosecond = toInteger(v);
		}
		v = o.second;
		if (v !== undefined) {
			found = true;
			r.second = toInteger(v);
		}
		if (!found) {
			throw typeError('Must specify at least one time field.');
		}
		return r;
	}

	// requirePartial throws where a with() argument is no partial Temporal
	// object (IsPartialTemporalObject).
	function requirePartial(v) {
		if (isObject(v) && Slots.of(v) === undefined && v.calendar === undefined && v.timeZone === undefined) {
			return;
		}
		throw typeError('Argument to with() must contain some date/time fields.');
	}

	// ------------------------------------------------------- conversions to
	// the kinds, which return records.

	function discardOverflow(opts, where) {
		overflow(opts, where);
	}

	function toPlainDate(v, opts, where) {
		const s = Slots.of(v);
		if (s !== undefined) {
			switch (s.kind) {
			case KD:
				discardOverflow(opts, where);
				return dateOf(s);
			case KDT:
			case KZ:
				discardOverflow(opts, where);
				return recDate(T(O.pdOf, s.y, s.m, s.d, s.cal));
			}
		}
		if (typeof v === 'string') {
			const r = T(O.pdParse, v);
			discardOverflow(opts, where);
			return recDate(checked(r));
		}
		if (!isObject(v)) {
			throw typeError('Date argument must be object or string.');
		}
		const cal = calendarOfBag(v);
		const rec = prepareFields(cal, v, F_DATE, REQUIRE_NONE);
		const ov = overflow(opts, where);
		return recDate(call(push(pushFields(bare([O.pdFields, cal]), rec), ov)));
	}

	function toPlainTime(v, opts, where) {
		const s = Slots.of(v);
		if (s !== undefined) {
			switch (s.kind) {
			case KT:
			case KDT:
			case KZ:
				discardOverflow(opts, where);
				return timeOf(s);
			}
		}
		if (typeof v === 'string') {
			const r = T(O.ptParse, v);
			discardOverflow(opts, where);
			return recTime(r);
		}
		if (!isObject(v)) {
			throw typeError('Time-like argument must be object or string');
		}
		const t = timeBag(v);
		const a = bare([O.ptFields, t.hour, t.minute, t.second, t.millisecond, t.microsecond, t.nanosecond]);
		for (let i = 1; i < 7; i++) {
			if (a[i] === undefined) {
				a[i] = 0;
			}
		}
		return recTime(call(push(a, overflow(opts, where))));
	}

	// timeOrMidnight is ToTimeRecordOrMidnight: undefined for undefined.
	function timeOrMidnight(v) {
		return v === undefined ? undefined : toPlainTime(v, undefined);
	}

	function pushOptTime(a, t) {
		return t === undefined ? pushNoTime(a) : pushTime(a, t);
	}

	function toPlainDateTime(v, opts, where) {
		const s = Slots.of(v);
		if (s !== undefined) {
			switch (s.kind) {
			case KDT:
				discardOverflow(opts, where);
				return recDateTime(pushDateTime(bare([]), s));
			case KZ: {
				const ov = overflow(opts, where);
				return recDateTime(call(push(pushTime(pushDate(bare([O.pdtOf]), s), s), ov)));
			}
			case KD: {
				const ov = overflow(opts, where);
				return recDateTime(call(push(pushNoTime(pushDate(bare([O.pdtOf]), s)), ov)));
			}
			}
		}
		if (typeof v === 'string') {
			const r = T(O.pdtParse, v);
			discardOverflow(opts, where);
			return recDateTime(checked(r));
		}
		if (!isObject(v)) {
			throw typeError('DateTime argument must be object or string.');
		}
		const cal = calendarOfBag(v);
		const rec = prepareFields(cal, v, F_DATE | F_TIME, REQUIRE_NONE);
		const ov = overflow(opts, where);
		return recDateTime(call(push(pushTimeFields(pushFields(bare([O.pdtFields, cal]), rec), rec), ov)));
	}

	function toPlainYearMonth(v, opts, where) {
		const s = Slots.of(v);
		if (s !== undefined && s.kind === KYM) {
			discardOverflow(opts, where);
			return recYearMonth(pushDate(bare([]), s));
		}
		if (typeof v === 'string') {
			const r = T(O.ymParse, v);
			discardOverflow(opts, where);
			return recYearMonth(checked(r));
		}
		if (!isObject(v)) {
			throw typeError('YearMonth argument must be object or string.');
		}
		const cal = calendarOfBag(v);
		const rec = prepareFields(cal, v, F_YEAR | F_MONTH, REQUIRE_NONE);
		const ov = overflow(opts, where);
		return recYearMonth(call(push(pushFields(bare([O.ymFields, cal]), rec), ov)));
	}

	function toPlainMonthDay(v, opts, where) {
		const s = Slots.of(v);
		if (s !== undefined && s.kind === KMD) {
			discardOverflow(opts, where);
			return recMonthDay(pushDate(bare([]), s));
		}
		if (typeof v === 'string') {
			const r = T(O.mdParse, v);
			discardOverflow(opts, where);
			return recMonthDay(checked(r));
		}
		if (!isObject(v)) {
			throw typeError('MonthDay argument must be object or string.');
		}
		const cal = calendarOfBag(v);
		const rec = prepareFields(cal, v, F_DATE, REQUIRE_NONE);
		const ov = overflow(opts, where);
		return recMonthDay(call(push(pushFields(bare([O.mdFields, cal]), rec), ov)));
	}

	// toInstant is ToTemporalInstant: an object other than an Instant or a
	// ZonedDateTime is read as a string.
	function toInstant(v) {
		const s = Slots.of(v);
		if (s !== undefined && (s.kind === KI || s.kind === KZ)) {
			return recInstant(s.ems, s.sub);
		}
		const p = isObject(v) ? toPrim(v, 'string') : v;
		if (typeof p !== 'string') {
			throw typeError('Instant argument must be Instant or string.');
		}
		const r = T(O.iParse, p);
		return recInstant(r[0], r[1]);
	}

	// toZoned is ToTemporalZonedDateTime; present says whether its options
	// are read at all.
	function toZoned(v, opts, present, where) {
		const s = Slots.of(v);
		if (s !== undefined && s.kind === KZ) {
			zonedOptions(opts, present, where);
			return recZoned(zonedArray(s));
		}
		if (typeof v === 'string') {
			T(O.zParse, v);
			const z = zonedOptions(opts, present, where);
			return recZoned(T(O.zFromString, v, z[0], z[1]));
		}
		if (!isObject(v)) {
			throw typeError('ZonedDateTime argument must be object or string.');
		}
		const cal = calendarOfBag(v);
		const rec = prepareFields(cal, v, F_DATE | F_TIME | F_OFFSET | F_ZONE, REQUIRE_ZONE);
		const z = zonedOptions(opts, present, where);
		const a = pushTimeFields(pushFields(bare([O.zFields, cal]), rec), rec);
		push(a, rec.offset);
		push(a, rec.timeZone);
		push(a, z[0]);
		push(a, z[1]);
		push(a, z[2]);
		return recZoned(call(a));
	}

	function zonedArray(s) {
		return bare([s.ems, s.sub, s.off, s.offset, s.y, s.m, s.d, s.h, s.mi, s.s, s.ms, s.us, s.ns, s.tz, s.cal]);
	}

	const durationKeys = bare(['days', 'hours', 'microseconds', 'milliseconds', 'minutes', 'months', 'nanoseconds',
		'seconds', 'weeks', 'years']);
	// durationIndex is the field of each of durationKeys.
	const durationIndex = bare([3, 4, 8, 7, 5, 1, 9, 6, 2, 0]);
	const int64Limit = 9223372036854775808;

	// partialDuration is ToTemporalPartialDurationRecord: the fields read in
	// the order of their names, undefined where absent.
	function partialDuration(v) {
		if (!isObject(v)) {
			throw typeError('Must provide a duration.');
		}
		const f = bare([undefined, undefined, undefined, undefined, undefined, undefined, undefined, undefined,
			undefined, undefined]);
		let found = false;
		for (let i = 0; i < 10; i++) {
			const x = v[durationKeys[i]];
			if (x === undefined) {
				continue;
			}
			const n = toIntegral(x);
			const j = durationIndex[i];
			if (j < 8 && !(n >= -int64Limit && n < int64Limit)) {
				throw rangeError('Duration field out of range.');
			}
			f[j] = n;
			found = true;
		}
		if (!found) {
			throw typeError('Did not provide any valid Duration fields.');
		}
		return f;
	}

	// toDuration is ToTemporalDuration, as the duration's fields.
	function toDuration(v) {
		const s = Slots.of(v);
		if (s !== undefined && s.kind === KDU) {
			return s.f;
		}
		if (typeof v === 'string') {
			return recDuration(T(O.dParse, v)).f;
		}
		if (!isObject(v)) {
			throw typeError('Duration argument must be Duration or string.');
		}
		return recDuration(call(pushDuration(bare([O.dPartial]), partialDuration(v)))).f;
	}

	// relativeTo is GetTemporalRelativeToOption, as the bridge takes it.
	function relativeTo(o) {
		const v = o.relativeTo;
		if (v === undefined) {
			return bare([0, undefined, undefined, undefined, undefined]);
		}
		const s = Slots.of(v);
		if (s !== undefined) {
			switch (s.kind) {
			case KZ:
				return bare([2, s.ems, s.sub, s.tz, s.cal]);
			case KD:
				return bare([1, s.y, s.m, s.d, s.cal]);
			case KDT: {
				const d = T(O.pdOf, s.y, s.m, s.d, s.cal);
				return bare([1, d[0], d[1], d[2], d[3]]);
			}
			}
		}
		if (typeof v === 'string') {
			const r = T(O.relParse, v);
			return bare([r[0], r[1], r[2], r[3], r[4]]);
		}
		if (!isObject(v)) {
			throw typeError('relativeTo must be object or string.');
		}
		const cal = calendarOfBag(v);
		const rec = prepareFields(cal, v, F_DATE | F_TIME | F_OFFSET | F_ZONE, REQUIRE_NONE);
		const a = pushTimeFields(pushFields(bare([O.relFields, cal]), rec), rec);
		push(a, rec.offset);
		push(a, rec.timeZone);
		const r = call(a);
		return bare([r[0], r[1], r[2], r[3], r[4]]);
	}

	function pushRelativeTo(a, r) {
		for (let i = 0; i < 5; i++) {
			a[a.length] = r[i];
		}
		return a;
	}

	function sameCalendar(a, b) {
		if (a.cal !== b.cal) {
			throw new RangeError('Mismatched calendars.');
		}
	}

	function compareNumbers(a, b) {
		return a < b ? -1 : a > b ? 1 : 0;
	}

	function compareDates(a, b) {
		return compareNumbers(a.y, b.y) || compareNumbers(a.m, b.m) || compareNumbers(a.d, b.d);
	}

	function compareTimes(a, b) {
		return compareNumbers(a.h, b.h) || compareNumbers(a.mi, b.mi) || compareNumbers(a.s, b.s) ||
			compareNumbers(a.ms, b.ms) || compareNumbers(a.us, b.us) || compareNumbers(a.ns, b.ns);
	}

	function compareInstants(a, b) {
		return compareNumbers(a.ems, b.ems) || compareNumbers(a.sub, b.sub);
	}

	// ------------------------------------------------------ shaping built-ins

	// define puts the properties of props on o as Temporal has them: not
	// enumerable, configurable, and writable but for Symbol.toStringTag. A
	// function written in props as a method or a getter is shaped like a
	// built-in function: no prototype property, no constructing, the
	// method's name ("get year" for a getter) and the number of its
	// parameters before the first with a default as its length.
	function define(o, props) {
		const keys = ownKeys(props);
		for (let i = 0; i < keys.length; i++) {
			const d = setPrototypeOf(getOwnPropertyDescriptor(props, keys[i]), null);
			d.enumerable = false;
			if ('value' in d) {
				d.writable = keys[i] !== toStringTag;
			}
			defineProperty(o, keys[i], d);
		}
		return o;
	}

	// defineOrdered defines the properties named in order, from the objects
	// in sources, the first that has each: the order of the properties of
	// Temporal's prototypes is V8's.
	function defineOrdered(o, sources, order) {
		for (let i = 0; i < order.length; i++) {
			const k = order[i];
			let d;
			for (let j = 0; j < sources.length && d === undefined; j++) {
				d = getOwnPropertyDescriptor(sources[j], k);
			}
			if (d === undefined) {
				throw new Error('temporal: internal error: no property ' + k);
			}
			d = setPrototypeOf(d, null);
			d.enumerable = false;
			if ('value' in d) {
				d.writable = k !== toStringTag;
			}
			defineProperty(o, k, d);
		}
		return o;
	}

	// defineConstructor finishes C: its prototype property read only, its
	// statics, its prototype's properties in order.
	function defineConstructor(C, kind, statics, sources, order) {
		defineProperty(C, 'prototype', {__proto__: null, writable: false});
		define(C, statics);
		defineOrdered(C.prototype, sources, order);
		protos[kind] = C.prototype;
		return C;
	}

	function newRequired(name) {
		return new TypeError("Constructor Temporal." + name + " requires 'new'");
	}

	function valueOfError(name) {
		return new TypeError('Do not use Temporal.' + name + '.prototype.valueOf; use Temporal.' + name +
			'.prototype.compare for comparison.');
	}

	// ------------------------------------------------------- toLocaleString

	function toLength(v) {
		const n = +v;
		if (!(n > 0)) {
			return 0;
		}
		return n < 9007199254740991 ? n - n % 1 : 9007199254740991;
	}

	// pushLocales appends the two locale arguments of package intl: a
	// single tag as itself, a list as one string of the tags each preceded
	// by its length and a colon (CanonicalizeLocaleList's argument handling).
	function pushLocales(a, locales) {
		const tag = state.locale === undefined ? undefined : state.locale(locales);
		if (tag !== undefined) locales = tag;
		if (typeof locales === 'string' || locales === undefined) {
			push(a, locales);
			return push(a, undefined);
		}
		if (locales === null) {
			throw new TypeError('Cannot convert null to object');
		}
		const o = Object(locales), n = toLength(o.length);
		let s = '';
		for (let i = 0; i < n; i++) {
			if (i in o) {
				const v = o[i];
				if (typeof v !== 'string' && !isObject(v)) {
					throw new TypeError('Language ID should be string or object');
				}
				const tag = state.locale === undefined ? undefined : state.locale(v);
				const t = tag === undefined ? toStr(v) : tag;
				s += t.length + ':' + t;
			}
		}
		push(a, undefined);
		return push(a, s);
	}

	// pushFormatOptions appends the formatter's options of opts,
	// converted as package intl does.
	function pushFormatOptions(a, opts, list) {
		if (opts === null) {
			throw new TypeError('Options must be an object');
		}
		if (opts === undefined) {
			return a;
		}
		const o = Object(opts), names = list.names, kinds = list.kinds;
		for (let i = 0; i < names.length; i++) {
			let v = o[names[i]];
			if (v !== undefined) {
				switch (kinds[i]) {
				case 's':
					v = toStr(v);
					break;
				case 'n':
					v = +v;
					break;
				case 'b':
					v = !!v;
					break;
				}
			}
			a[a.length] = v;
		}
		return a;
	}

	function localeString(kind, cal, ms, zone, locales, options, offset) {
		const a = pushLocales(bare([kind, cal, ms, zone, offset]), locales);
		return apply(format, undefined, pushFormatOptions(a, options, dtfList));
	}

	// epochDays is the days of an ISO date since 1970-01-01.
	function epochDays(y, m, d) {
		y -= m <= 2 ? 1 : 0;
		const era = floor(y / 400);
		const yoe = y - era * 400;
		const doy = floor((153 * (m > 2 ? m - 3 : m + 9) + 2) / 5) + d - 1;
		const doe = yoe * 365 + floor(yoe / 4) - floor(yoe / 100) + doy;
		return era * 146097 + doe - 719468;
	}

	function timeMs(s) {
		return s.h * 3600000 + s.mi * 60000 + s.s * 1000 + s.ms;
	}

	function durationFieldsString(fields) {
		let text = '' + fields[0];
		for (let i = 1; i < fields.length; i++) text += ',' + fields[i];
		return text;
	}

	state.temporal = function (value) {
		const s = Slots.of(value);
		if (s === undefined) return undefined;
		if (s.kind === KDU) return {__proto__: null, kind: 'duration', fields: durationFieldsString(s.f)};
		let kind, ms, cal = 'iso8601';
		switch (s.kind) {
		case KD: kind = 'date'; break;
		case KT: kind = 'time'; ms = timeMs(s); break;
		case KDT: kind = 'dateTime'; break;
		case KYM: kind = 'yearMonth'; break;
		case KMD: kind = 'monthDay'; break;
		case KI: kind = 'instant'; ms = s.ems; break;
		case KZ: kind = 'zoned'; ms = s.ems; break;
		default: throw new Error('temporal: unknown internal kind');
		}
		if (s.kind === KD || s.kind === KDT || s.kind === KYM || s.kind === KMD) {
			cal = s.cal;
			ms = epochDays(s.y, s.m, s.d) * 86400000 + (s.kind === KDT ? timeMs(s) : 43200000);
		}
		return {__proto__: null, kind: kind, cal: cal, ms: ms};
	};

	// ----------------------------------------------------- calendar getters

	// dateInfo is what the bridge says of the date of a record (see
	// dateInfo in ops_date.go), kept in the record.
	function dateInfo(s) {
		let i = s.info;
		if (i === undefined) {
			i = T(O.pdInfo, s.y, s.m, s.d, s.cal);
			s.info = i;
		}
		return i;
	}

	function monthCodeOf(m) {
		return m < 10 ? 'M0' + m : 'M' + m;
	}

	// dateGetters are the calendar getters of the kinds with a date.
	function dateGetters(kind) {
		return {
			get calendarId() {
				return slot(this, kind, 'calendarId').cal;
			},
			get era() {
				return dateInfo(slot(this, kind, 'era'))[4];
			},
			get eraYear() {
				return dateInfo(slot(this, kind, 'eraYear'))[5];
			},
			get year() {
				const s = slot(this, kind, 'year');
				return s.cal === 'iso8601' ? s.y : dateInfo(s)[0];
			},
			get month() {
				const s = slot(this, kind, 'month');
				return s.cal === 'iso8601' ? s.m : dateInfo(s)[1];
			},
			get monthCode() {
				const s = slot(this, kind, 'monthCode');
				return s.cal === 'iso8601' ? monthCodeOf(s.m) : dateInfo(s)[2];
			},
			get day() {
				const s = slot(this, kind, 'day');
				return s.cal === 'iso8601' ? s.d : dateInfo(s)[3];
			},
			get dayOfWeek() {
				return dateInfo(slot(this, kind, 'dayOfWeek'))[6];
			},
			get dayOfYear() {
				return dateInfo(slot(this, kind, 'dayOfYear'))[7];
			},
			get weekOfYear() {
				return dateInfo(slot(this, kind, 'weekOfYear'))[8];
			},
			get yearOfWeek() {
				return dateInfo(slot(this, kind, 'yearOfWeek'))[9];
			},
			get daysInWeek() {
				return dateInfo(slot(this, kind, 'daysInWeek'))[10];
			},
			get daysInMonth() {
				return dateInfo(slot(this, kind, 'daysInMonth'))[11];
			},
			get daysInYear() {
				return dateInfo(slot(this, kind, 'daysInYear'))[12];
			},
			get monthsInYear() {
				return dateInfo(slot(this, kind, 'monthsInYear'))[13];
			},
			get inLeapYear() {
				return dateInfo(slot(this, kind, 'inLeapYear'))[14] === 1;
			}
		};
	}

	function timeGetters(kind) {
		return {
			get hour() {
				return slot(this, kind, 'hour').h;
			},
			get minute() {
				return slot(this, kind, 'minute').mi;
			},
			get second() {
				return slot(this, kind, 'second').s;
			},
			get millisecond() {
				return slot(this, kind, 'millisecond').ms;
			},
			get microsecond() {
				return slot(this, kind, 'microsecond').us;
			},
			get nanosecond() {
				return slot(this, kind, 'nanosecond').ns;
			}
		};
	}

	// ----------------------------------------------------------- PlainDate

	function PlainDate(isoYear, isoMonth, isoDay, calendar = undefined) {
		if (new.target === undefined) {
			throw newRequired(KD);
		}
		const y = toInteger(isoYear), m = toInteger(isoMonth), d = toInteger(isoDay);
		const cal = calendarArg(calendar);
		new Slots(this, recDate(T(O.pdNew, y, m, d, cal)));
	}

	defineConstructor(PlainDate, KD, {
		from(item, options = undefined) {
			return make(toPlainDate(item, options, 'Temporal.PlainDate.from'));
		},
		compare(one, two) {
			return compareDates(toPlainDate(one, undefined), toPlainDate(two, undefined));
		}
	}, [dateGetters(KD), {
		toPlainYearMonth() {
			const s = slot(this, KD, 'toPlainYearMonth');
			return make(recYearMonth(T(O.pdToYM, s.y, s.m, s.d, s.cal)));
		},
		toPlainMonthDay() {
			const s = slot(this, KD, 'toPlainMonthDay');
			return make(recMonthDay(T(O.pdToMD, s.y, s.m, s.d, s.cal)));
		},
		add(temporalDurationLike, options = undefined) {
			return dateAdd(slot(this, KD, 'add'), temporalDurationLike, options, false, 'Temporal.PlainDate.prototype.add');
		},
		subtract(temporalDurationLike, options = undefined) {
			return dateAdd(slot(this, KD, 'subtract'), temporalDurationLike, options, true, 'Temporal.PlainDate.prototype.subtract');
		},
		with(temporalDateLike, options = undefined) {
			const s = slot(this, KD, 'with');
			requirePartial(temporalDateLike);
			const rec = prepareFields(s.cal, temporalDateLike, F_DATE, REQUIRE_PARTIAL);
			const ov = overflow(options, 'Temporal.PlainDate.prototype.with');
			return make(recDate(call(push(pushFields(pushDate(bare([O.pdWith]), s), rec), ov))));
		},
		withCalendar(calendarLike) {
			const s = slot(this, KD, 'withCalendar');
			const d = dateOf(s);
			d.cal = toCalendar(calendarLike);
			return make(d);
		},
		until(other, options = undefined) {
			return dateDiff(slot(this, KD, 'until'), other, options, false, 'Temporal.PlainDate.prototype.until');
		},
		since(other, options = undefined) {
			return dateDiff(slot(this, KD, 'since'), other, options, true, 'Temporal.PlainDate.prototype.since');
		},
		equals(other) {
			const s = slot(this, KD, 'equals');
			const o = toPlainDate(other, undefined);
			return compareDates(s, o) === 0 && s.cal === o.cal;
		},
		toPlainDateTime(temporalTime = undefined) {
			const s = slot(this, KD, 'toPlainDateTime');
			const t = timeOrMidnight(temporalTime);
			return make(recDateTime(call(pushOptTime(pushDate(bare([O.pdToPDT]), s), t))));
		},
		toZonedDateTime(item) {
			const s = slot(this, KD, 'toZonedDateTime');
			let tz, timeLike;
			if (isObject(item)) {
				const tzLike = item.timeZone;
				if (tzLike === undefined) {
					tz = toTimeZone(item);
				} else {
					tz = toTimeZone(tzLike);
					timeLike = item.plainTime;
				}
			} else {
				tz = toTimeZone(item);
			}
			const t = timeLike === undefined ? undefined : toPlainTime(timeLike, undefined);
			return make(recZoned(call(pushOptTime(push(pushDate(bare([O.pdToZDT]), s), tz), t))));
		},
		toString(options = undefined) {
			const s = slot(this, KD, 'toString');
			const show = calendarName(optionsObject(options), 'Temporal.PlainDate.prototype.toString');
			return T(O.pdStr, s.y, s.m, s.d, s.cal, show);
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KD, 'toLocaleString');
			return localeString('date', s.cal, epochDays(s.y, s.m, s.d) * 86400000 + 43200000, undefined, locales, options);
		},
		toJSON() {
			const s = slot(this, KD, 'toJSON');
			return T(O.pdStr, s.y, s.m, s.d, s.cal, 0);
		},
		valueOf() {
			throw valueOfError(KD);
		},
		[toStringTag]: 'Temporal.PlainDate'
	}], bare(['era', 'eraYear', 'calendarId', 'year', 'month', 'monthCode', 'day', 'dayOfWeek', 'dayOfYear',
		'weekOfYear', 'yearOfWeek', 'daysInWeek', 'daysInMonth', 'daysInYear', 'monthsInYear', 'inLeapYear',
		'toPlainYearMonth', 'toPlainMonthDay', 'add', 'subtract', 'with', 'withCalendar', 'until', 'since', 'equals',
		'toPlainDateTime', 'toZonedDateTime', 'toString', 'toLocaleString', 'toJSON', 'valueOf', toStringTag]));

	function dateAdd(s, durationLike, options, sub, where) {
		const f = toDuration(durationLike);
		const ov = overflow(options, where);
		return make(recDate(call(push(push(pushDuration(pushDate(bare([O.pdAdd]), s), f), ov), sub))));
	}

	function dateDiff(s, other, options, since, where) {
		const o = toPlainDate(other, undefined);
		sameCalendar(s, o);
		const settings = differenceSettings(options, where);
		return make(recDuration(call(push(pushSettings(pushDate(pushDate(bare([O.pdDiff]), s), o), settings), since))));
	}

	// ----------------------------------------------------------- PlainTime

	function timeArgs(a, args, from) {
		for (let i = 0; i < 6; i++) {
			const v = args[from + i];
			a[a.length] = v === undefined ? 0 : toInteger(v);
		}
		return a;
	}

	function PlainTime(hour = undefined, minute = undefined, second = undefined, millisecond = undefined,
		microsecond = undefined, nanosecond = undefined) {
		if (new.target === undefined) {
			throw newRequired(KT);
		}
		const a = timeArgs(bare([O.ptNew]), bare([hour, minute, second, millisecond, microsecond, nanosecond]), 0);
		new Slots(this, recTime(call(a)));
	}

	defineConstructor(PlainTime, KT, {
		from(item, options = undefined) {
			return make(toPlainTime(item, options, 'Temporal.PlainTime.from'));
		},
		compare(one, two) {
			return compareTimes(toPlainTime(one, undefined), toPlainTime(two, undefined));
		}
	}, [timeGetters(KT), {
		add(temporalDurationLike) {
			return timeAdd(slot(this, KT, 'add'), temporalDurationLike, false);
		},
		subtract(temporalDurationLike) {
			return timeAdd(slot(this, KT, 'subtract'), temporalDurationLike, true);
		},
		with(temporalTimeLike, options = undefined) {
			const s = slot(this, KT, 'with');
			requirePartial(temporalTimeLike);
			const t = timeBag(temporalTimeLike);
			const ov = overflow(options, 'Temporal.PlainTime.prototype.with');
			return make(recTime(call(push(pushTimeFields(pushTime(bare([O.ptWith]), s), t), ov))));
		},
		until(other, options = undefined) {
			return timeDiff(slot(this, KT, 'until'), other, options, false, 'Temporal.PlainTime.prototype.until');
		},
		since(other, options = undefined) {
			return timeDiff(slot(this, KT, 'since'), other, options, true, 'Temporal.PlainTime.prototype.since');
		},
		round(roundToArg) {
			const s = slot(this, KT, 'round');
			// V8 names PlainTime's round as PlainDateTime's.
			const r = roundingOptions(roundTo(roundToArg, KT), 0, 'Temporal.PlainDateTime.prototype.round');
			return make(recTime(call(pushRounding(pushTime(bare([O.ptRound]), s), r))));
		},
		equals(other) {
			const s = slot(this, KT, 'equals');
			return compareTimes(s, toPlainTime(other, undefined)) === 0;
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KT, 'toLocaleString');
			return localeString('time', 'iso8601', timeMs(s), undefined, locales, options);
		},
		toString(options = undefined) {
			const s = slot(this, KT, 'toString');
			const t = toStringOptions(optionsObject(options), undefined, 'Temporal.PlainTime.prototype.toString');
			validateUnit(t[1], GROUP_TIME, 0);
			return call(pushToString(pushTime(bare([O.ptStr]), s), t));
		},
		toJSON() {
			const s = slot(this, KT, 'toJSON');
			return call(push(push(push(pushTime(bare([O.ptStr]), s), -1), 0), 0));
		},
		valueOf() {
			throw valueOfError(KT);
		},
		[toStringTag]: 'Temporal.PlainTime'
	}], bare(['hour', 'minute', 'second', 'millisecond', 'microsecond', 'nanosecond', 'add', 'subtract', 'with',
		'until', 'since', 'round', 'equals', 'toLocaleString', 'toString', 'toJSON', 'valueOf', toStringTag]));

	function timeAdd(s, durationLike, sub) {
		const f = toDuration(durationLike);
		return make(recTime(call(push(pushDuration(pushTime(bare([O.ptAdd]), s), f), sub))));
	}

	function timeDiff(s, other, options, since, where) {
		const o = toPlainTime(other, undefined);
		const settings = differenceSettings(options, where);
		return make(recDuration(call(push(pushSettings(pushTime(pushTime(bare([O.ptDiff]), s), o), settings), since))));
	}

	// ------------------------------------------------------- PlainDateTime

	function PlainDateTime(isoYear, isoMonth, isoDay, hour = undefined, minute = undefined, second = undefined,
		millisecond = undefined, microsecond = undefined, nanosecond = undefined, calendar = undefined) {
		if (new.target === undefined) {
			throw newRequired(KDT);
		}
		const a = bare([O.pdtNew, toInteger(isoYear), toInteger(isoMonth), toInteger(isoDay)]);
		timeArgs(a, bare([hour, minute, second, millisecond, microsecond, nanosecond]), 0);
		push(a, calendarArg(calendar));
		new Slots(this, recDateTime(call(a)));
	}

	defineConstructor(PlainDateTime, KDT, {
		from(item, options = undefined) {
			return make(toPlainDateTime(item, options, 'Temporal.PlainDateTime.from'));
		},
		compare(one, two) {
			const a = toPlainDateTime(one, undefined), b = toPlainDateTime(two, undefined);
			return compareDates(a, b) || compareTimes(a, b);
		}
	}, [dateGetters(KDT), timeGetters(KDT), {
		with(temporalDateTimeLike, options = undefined) {
			const s = slot(this, KDT, 'with');
			requirePartial(temporalDateTimeLike);
			const rec = prepareFields(s.cal, temporalDateTimeLike, F_DATE | F_TIME, REQUIRE_PARTIAL);
			const ov = overflow(options, 'Temporal.PlainDateTime.prototype.with');
			return make(recDateTime(call(push(pushTimeFields(pushFields(pushDateTime(bare([O.pdtWith]), s), rec), rec), ov))));
		},
		withCalendar(calendarLike) {
			const s = slot(this, KDT, 'withCalendar');
			const r = recDateTime(pushDateTime(bare([]), s));
			r.cal = toCalendar(calendarLike);
			return make(r);
		},
		withPlainTime(plainTimeLike = undefined) {
			const s = slot(this, KDT, 'withPlainTime');
			const t = timeOrMidnight(plainTimeLike);
			return make(recDateTime(call(pushOptTime(pushDateTime(bare([O.pdtWithTime]), s), t))));
		},
		add(temporalDurationLike, options = undefined) {
			return dateTimeAdd(slot(this, KDT, 'add'), temporalDurationLike, options, false, 'Temporal.PlainDateTime.prototype.add');
		},
		subtract(temporalDurationLike, options = undefined) {
			return dateTimeAdd(slot(this, KDT, 'subtract'), temporalDurationLike, options, true, 'Temporal.PlainDateTime.prototype.subtract');
		},
		until(other, options = undefined) {
			return dateTimeDiff(slot(this, KDT, 'until'), other, options, false, 'Temporal.PlainDateTime.prototype.until');
		},
		since(other, options = undefined) {
			return dateTimeDiff(slot(this, KDT, 'since'), other, options, true, 'Temporal.PlainDateTime.prototype.since');
		},
		round(roundToArg) {
			const s = slot(this, KDT, 'round');
			const r = roundingOptions(roundTo(roundToArg, KDT), UNIT_DAY, 'Temporal.PlainDateTime.prototype.round');
			return make(recDateTime(call(pushRounding(pushDateTime(bare([O.pdtRound]), s), r))));
		},
		equals(other) {
			const s = slot(this, KDT, 'equals');
			const o = toPlainDateTime(other, undefined);
			return compareDates(s, o) === 0 && compareTimes(s, o) === 0 && s.cal === o.cal;
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KDT, 'toLocaleString');
			return localeString('dateTime', s.cal, epochDays(s.y, s.m, s.d) * 86400000 + timeMs(s), undefined, locales,
				options);
		},
		toJSON() {
			const s = slot(this, KDT, 'toJSON');
			return call(push(push(push(push(pushDateTime(bare([O.pdtStr]), s), -1), 0), 0), 0));
		},
		toString(options = undefined) {
			const s = slot(this, KDT, 'toString');
			const o = optionsObject(options);
			// V8 names PlainDateTime's toString so.
			const show = calendarName(o, 'Temporal.DateTime.prototype.toString');
			const t = toStringOptions(o, undefined, 'Temporal.DateTime.prototype.toString');
			validateUnit(t[1], GROUP_TIME, 0);
			return call(push(pushToString(pushDateTime(bare([O.pdtStr]), s), t), show));
		},
		valueOf() {
			throw valueOfError(KDT);
		},
		toZonedDateTime(temporalTimeZoneLike, options = undefined) {
			const s = slot(this, KDT, 'toZonedDateTime');
			const tz = toTimeZone(temporalTimeZoneLike);
			const dis = disambiguation(options, 'Temporal.PlainDateTime.prototype.toZonedDateTime');
			return make(recZoned(call(push(push(pushDateTime(bare([O.pdtToZDT]), s), tz), dis))));
		},
		toPlainDate() {
			return make(dateOf(slot(this, KDT, 'toPlainDate')));
		},
		toPlainTime() {
			return make(timeOf(slot(this, KDT, 'toPlainTime')));
		},
		[toStringTag]: 'Temporal.PlainDateTime'
	}], bare(['calendarId', 'era', 'eraYear', 'year', 'month', 'monthCode', 'day', 'hour', 'minute', 'second',
		'millisecond', 'microsecond', 'nanosecond', 'dayOfWeek', 'dayOfYear', 'weekOfYear', 'yearOfWeek', 'daysInWeek',
		'daysInMonth', 'daysInYear', 'monthsInYear', 'inLeapYear', 'with', 'withCalendar', 'withPlainTime', 'add',
		'subtract', 'until', 'since', 'round', 'equals', 'toLocaleString', 'toJSON', 'toString', 'valueOf',
		'toZonedDateTime', 'toPlainDate', 'toPlainTime', toStringTag]));

	function dateTimeAdd(s, durationLike, options, sub, where) {
		const f = toDuration(durationLike);
		const ov = overflow(options, where);
		return make(recDateTime(call(push(push(pushDuration(pushDateTime(bare([O.pdtAdd]), s), f), ov), sub))));
	}

	function dateTimeDiff(s, other, options, since, where) {
		const o = toPlainDateTime(other, undefined);
		sameCalendar(s, o);
		const settings = differenceSettings(options, where);
		return make(recDuration(call(push(pushSettings(pushDateTime(pushDateTime(bare([O.pdtDiff]), s), o), settings),
			since))));
	}

	// ------------------------------------------------------ PlainYearMonth

	function yearMonthInfo(s) {
		let i = s.info;
		if (i === undefined) {
			i = T(O.ymInfo, s.y, s.m, s.d, s.cal);
			s.info = i;
		}
		return i;
	}

	function PlainYearMonth(isoYear, isoMonth, calendar = undefined, referenceISODay = undefined) {
		if (new.target === undefined) {
			throw newRequired(KYM);
		}
		const y = toInteger(isoYear), m = toInteger(isoMonth);
		const cal = calendarArg(calendar);
		const ref = referenceISODay === undefined ? 1 : toInteger(referenceISODay);
		new Slots(this, recYearMonth(T(O.ymNew, y, m, cal, ref)));
	}

	defineConstructor(PlainYearMonth, KYM, {
		from(item, options = undefined) {
			return make(toPlainYearMonth(item, options, 'Temporal.PlainYearMonth.from'));
		},
		compare(one, two) {
			return compareDates(toPlainYearMonth(one, undefined), toPlainYearMonth(two, undefined));
		}
	}, [{
		get calendarId() {
			return slot(this, KYM, 'calendarId').cal;
		},
		get era() {
			return yearMonthInfo(slot(this, KYM, 'era'))[3];
		},
		get eraYear() {
			return yearMonthInfo(slot(this, KYM, 'eraYear'))[4];
		},
		get year() {
			const s = slot(this, KYM, 'year');
			return s.cal === 'iso8601' ? s.y : yearMonthInfo(s)[0];
		},
		get month() {
			const s = slot(this, KYM, 'month');
			return s.cal === 'iso8601' ? s.m : yearMonthInfo(s)[1];
		},
		get monthCode() {
			const s = slot(this, KYM, 'monthCode');
			return s.cal === 'iso8601' ? monthCodeOf(s.m) : yearMonthInfo(s)[2];
		},
		get daysInYear() {
			return yearMonthInfo(slot(this, KYM, 'daysInYear'))[6];
		},
		get daysInMonth() {
			return yearMonthInfo(slot(this, KYM, 'daysInMonth'))[5];
		},
		get monthsInYear() {
			return yearMonthInfo(slot(this, KYM, 'monthsInYear'))[7];
		},
		get inLeapYear() {
			return yearMonthInfo(slot(this, KYM, 'inLeapYear'))[8] === 1;
		},
		with(temporalYearMonthLike, options = undefined) {
			const s = slot(this, KYM, 'with');
			requirePartial(temporalYearMonthLike);
			const rec = prepareFields(s.cal, temporalYearMonthLike, F_YEAR | F_MONTH, REQUIRE_PARTIAL);
			const ov = overflow(options, 'Temporal.PlainYearMonth.prototype.with');
			return make(recYearMonth(call(push(pushFields(pushDate(bare([O.ymWith]), s), rec), ov))));
		},
		add(temporalDurationLike, options = undefined) {
			return yearMonthAdd(slot(this, KYM, 'add'), temporalDurationLike, options, false, 'Temporal.PlainYearMonth.prototype.add');
		},
		subtract(temporalDurationLike, options = undefined) {
			return yearMonthAdd(slot(this, KYM, 'subtract'), temporalDurationLike, options, true, 'Temporal.PlainYearMonth.prototype.subtract');
		},
		until(other, options = undefined) {
			return yearMonthDiff(slot(this, KYM, 'until'), other, options, false, 'Temporal.PlainYearMonth.prototype.until');
		},
		since(other, options = undefined) {
			return yearMonthDiff(slot(this, KYM, 'since'), other, options, true, 'Temporal.PlainYearMonth.prototype.since');
		},
		equals(other) {
			const s = slot(this, KYM, 'equals');
			const o = toPlainYearMonth(other, undefined);
			return compareDates(s, o) === 0 && s.cal === o.cal;
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KYM, 'toLocaleString');
			return localeString('yearMonth', s.cal, epochDays(s.y, s.m, s.d) * 86400000 + 43200000, undefined, locales,
				options);
		},
		toString(options = undefined) {
			const s = slot(this, KYM, 'toString');
			const show = calendarName(optionsObject(options), 'Temporal.PlainYearMonth.prototype.toString');
			return T(O.ymStr, s.y, s.m, s.d, s.cal, show);
		},
		toJSON() {
			const s = slot(this, KYM, 'toJSON');
			return T(O.ymStr, s.y, s.m, s.d, s.cal, 0);
		},
		valueOf() {
			throw valueOfError(KYM);
		},
		toPlainDate(item) {
			const s = slot(this, KYM, 'toPlainDate');
			if (!isObject(item)) {
				throw typeError('year argument must be an object.');
			}
			const rec = prepareFields(s.cal, item, F_DAY, REQUIRE_NONE);
			return make(recDate(call(pushFields(pushDate(bare([O.ymToPD]), s), rec))));
		},
		[toStringTag]: 'Temporal.PlainYearMonth'
	}], bare(['calendarId', 'era', 'eraYear', 'year', 'month', 'monthCode', 'daysInYear', 'daysInMonth',
		'monthsInYear', 'inLeapYear', 'with', 'add', 'subtract', 'until', 'since', 'equals', 'toLocaleString',
		'toString', 'toJSON', 'valueOf', 'toPlainDate', toStringTag]));

	function yearMonthAdd(s, durationLike, options, sub, where) {
		const f = toDuration(durationLike);
		const ov = overflow(options, where);
		return make(recYearMonth(call(push(push(pushDuration(pushDate(bare([O.ymAdd]), s), f), ov), sub))));
	}

	function yearMonthDiff(s, other, options, since, where) {
		const o = toPlainYearMonth(other, undefined);
		sameCalendar(s, o);
		const settings = differenceSettings(options, where);
		return make(recDuration(call(push(pushSettings(pushDate(pushDate(bare([O.ymDiff]), s), o), settings), since))));
	}

	// ------------------------------------------------------- PlainMonthDay

	function monthDayInfo(s) {
		let i = s.info;
		if (i === undefined) {
			i = T(O.mdInfo, s.y, s.m, s.d, s.cal);
			s.info = i;
		}
		return i;
	}

	function PlainMonthDay(isoMonth, isoDay, calendar = undefined, referenceISOYear = undefined) {
		if (new.target === undefined) {
			throw newRequired(KMD);
		}
		const m = toInteger(isoMonth), d = toInteger(isoDay);
		const cal = calendarArg(calendar);
		const ref = referenceISOYear === undefined ? 1972 : toInteger(referenceISOYear);
		new Slots(this, recMonthDay(T(O.mdNew, m, d, cal, ref)));
	}

	defineConstructor(PlainMonthDay, KMD, {
		from(item, options = undefined) {
			return make(toPlainMonthDay(item, options, 'Temporal.PlainMonthDay.from'));
		}
	}, [{
		get calendarId() {
			return slot(this, KMD, 'calendarId').cal;
		},
		get monthCode() {
			const s = slot(this, KMD, 'monthCode');
			return s.cal === 'iso8601' ? monthCodeOf(s.m) : monthDayInfo(s)[0];
		},
		get day() {
			const s = slot(this, KMD, 'day');
			return s.cal === 'iso8601' ? s.d : monthDayInfo(s)[1];
		},
		with(temporalMonthDayLike, options = undefined) {
			const s = slot(this, KMD, 'with');
			requirePartial(temporalMonthDayLike);
			const rec = prepareFields(s.cal, temporalMonthDayLike, F_DATE, REQUIRE_PARTIAL);
			// V8 names PlainMonthDay's with as PlainYearMonth's.
			const ov = overflow(options, 'Temporal.PlainYearMonth.prototype.with');
			return make(recMonthDay(call(push(pushFields(pushDate(bare([O.mdWith]), s), rec), ov))));
		},
		equals(other) {
			const s = slot(this, KMD, 'equals');
			const o = toPlainMonthDay(other, undefined);
			return compareDates(s, o) === 0 && s.cal === o.cal;
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KMD, 'toLocaleString');
			return localeString('monthDay', s.cal, epochDays(s.y, s.m, s.d) * 86400000 + 43200000, undefined, locales,
				options);
		},
		toString(options = undefined) {
			const s = slot(this, KMD, 'toString');
			const show = calendarName(optionsObject(options), 'Temporal.PlainMonthDay.prototype.toString');
			return T(O.mdStr, s.y, s.m, s.d, s.cal, show);
		},
		toJSON() {
			const s = slot(this, KMD, 'toJSON');
			return T(O.mdStr, s.y, s.m, s.d, s.cal, 0);
		},
		valueOf() {
			throw valueOfError(KMD);
		},
		toPlainDate(item) {
			const s = slot(this, KMD, 'toPlainDate');
			if (!isObject(item)) {
				throw typeError('year argument must be an object.');
			}
			const rec = prepareFields(s.cal, item, F_YEAR, REQUIRE_NONE);
			return make(recDate(call(pushFields(pushDate(bare([O.mdToPD]), s), rec))));
		},
		[toStringTag]: 'Temporal.PlainMonthDay'
	}], bare(['calendarId', 'monthCode', 'day', 'with', 'equals', 'toLocaleString', 'toString', 'toJSON', 'valueOf',
		'toPlainDate', toStringTag]));

	// ------------------------------------------------------------- Instant

	function Instant(epochNanoseconds) {
		if (new.target === undefined) {
			throw newRequired(KI);
		}
		new Slots(this, instantOfNs(toBigInt(epochNanoseconds)));
	}

	defineConstructor(Instant, KI, {
		from(item) {
			return make(toInstant(item));
		},
		fromEpochMilliseconds(epochMilliseconds) {
			const n = toNumber(epochMilliseconds);
			if (n !== n || n === 1 / 0 || n === -1 / 0 || n % 1 !== 0) {
				throw rangeError('Expected finite integer.');
			}
			if (n < -8.64e15 || n > 8.64e15) {
				throw rangeError('Instant nanoseconds are not within a valid epoch range.');
			}
			return make(recInstant(n + 0, 0));
		},
		fromEpochNanoseconds(epochNanoseconds) {
			return make(instantOfNs(toBigInt(epochNanoseconds)));
		},
		compare(one, two) {
			return compareInstants(toInstant(one), toInstant(two));
		}
	}, [{
		get epochMilliseconds() {
			return slot(this, KI, 'epochMilliseconds').ems;
		},
		get epochNanoseconds() {
			return epochNanoseconds(slot(this, KI, 'epochNanoseconds'));
		},
		add(temporalDurationLike) {
			return instantAdd(slot(this, KI, 'add'), temporalDurationLike, false);
		},
		subtract(temporalDurationLike) {
			return instantAdd(slot(this, KI, 'subtract'), temporalDurationLike, true);
		},
		until(other, options = undefined) {
			return instantDiff(slot(this, KI, 'until'), other, options, false, 'Temporal.Instant.prototype.until');
		},
		since(other, options = undefined) {
			return instantDiff(slot(this, KI, 'since'), other, options, true, 'Temporal.Instant.prototype.since');
		},
		round(roundToArg) {
			const s = slot(this, KI, 'round');
			const r = roundingOptions(roundTo(roundToArg, KI), 0, 'Temporal.Instant.prototype.round');
			const a = T(O.iRound, s.ems, s.sub, r[0], r[1], r[2]);
			return make(recInstant(a[0], a[1]));
		},
		equals(other) {
			const s = slot(this, KI, 'equals');
			return compareInstants(s, toInstant(other)) === 0;
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KI, 'toLocaleString');
			return localeString('instant', 'iso8601', s.ems, undefined, locales, options);
		},
		toString(options = undefined) {
			const s = slot(this, KI, 'toString');
			const o = optionsObject(options);
			const t = toStringOptions(o, undefined, 'Temporal.Instant.prototype.toString');
			const tzLike = o.timeZone;
			validateUnit(t[1], GROUP_TIME, 0);
			if (t[1] === UNIT_HOUR) {
				throw new RangeError('smallestUnit value is out of range.');
			}
			const tz = tzLike === undefined ? undefined : toTimeZone(tzLike);
			return T(O.iStr, s.ems, s.sub, tz, t[0], t[1], t[2]);
		},
		toJSON() {
			const s = slot(this, KI, 'toJSON');
			return T(O.iStr, s.ems, s.sub, undefined, -1, 0, 0);
		},
		valueOf() {
			throw valueOfError(KI);
		},
		toZonedDateTimeISO(timeZone) {
			const s = slot(this, KI, 'toZonedDateTimeISO');
			const tz = toTimeZone(timeZone);
			return make(recZoned(T(O.zMake, s.ems, s.sub, tz, 'iso8601')));
		},
		[toStringTag]: 'Temporal.Instant'
	}], bare(['epochMilliseconds', 'epochNanoseconds', 'add', 'subtract', 'until', 'since', 'round', 'equals',
		'toLocaleString', 'toString', 'toJSON', 'valueOf', 'toZonedDateTimeISO', toStringTag]));

	function instantAdd(s, durationLike, sub) {
		const f = toDuration(durationLike);
		const a = call(push(pushDuration(bare([O.iAdd, s.ems, s.sub]), f), sub));
		return make(recInstant(a[0], a[1]));
	}

	function instantDiff(s, other, options, since, where) {
		const o = toInstant(other);
		const settings = differenceSettings(options, where);
		return make(recDuration(call(push(pushSettings(bare([O.iDiff, s.ems, s.sub, o.ems, o.sub]), settings), since))));
	}

	// ------------------------------------------------------- ZonedDateTime

	function ZonedDateTime(epochNanoseconds, timeZone, calendar = undefined) {
		if (new.target === undefined) {
			throw newRequired(KZ);
		}
		const i = instantOfNs(toBigInt(epochNanoseconds));
		if (typeof timeZone !== 'string') {
			throw typeError('Time zone must be string');
		}
		const tz = T(O.tzIdent, timeZone);
		const cal = calendarArg(calendar);
		new Slots(this, recZoned(T(O.zMake, i.ems, i.sub, tz, cal)));
	}

	const transitionDirections = bare(['next', 'previous']);

	defineConstructor(ZonedDateTime, KZ, {
		from(item, options = undefined) {
			return make(toZoned(item, options, true, 'Temporal.ZonedDateTime.from'));
		},
		compare(one, two) {
			return compareInstants(toZoned(one, undefined, false), toZoned(two, undefined, false));
		}
	}, [dateGetters(KZ), timeGetters(KZ), {
		get timeZoneId() {
			return slot(this, KZ, 'timeZoneId').tz;
		},
		get epochMilliseconds() {
			return slot(this, KZ, 'epochMilliseconds').ems;
		},
		get epochNanoseconds() {
			return epochNanoseconds(slot(this, KZ, 'epochNanoseconds'));
		},
		get hoursInDay() {
			const s = slot(this, KZ, 'hoursInDay');
			return call(pushZoned(bare([O.zHoursInDay]), s));
		},
		get offsetNanoseconds() {
			return slot(this, KZ, 'offsetNanoseconds').off;
		},
		get offset() {
			return slot(this, KZ, 'offset').offset;
		},
		with(temporalZonedDateTimeLike, options = undefined) {
			const s = slot(this, KZ, 'with');
			requirePartial(temporalZonedDateTimeLike);
			const rec = prepareFields(s.cal, temporalZonedDateTimeLike, F_DATE | F_TIME | F_OFFSET, REQUIRE_PARTIAL);
			const dis = disambiguation(options, 'Temporal.ZonedDateTime.prototype.with');
			const off = offsetOption(options, OFFSET_PREFER, 'Temporal.ZonedDateTime.prototype.with');
			const ov = overflow(options, 'Temporal.ZonedDateTime.prototype.with');
			const a = pushTimeFields(pushFields(pushZoned(bare([O.zWith]), s), rec), rec);
			push(a, rec.offset);
			push(a, undefined);
			push(a, dis);
			push(a, off);
			push(a, ov);
			return make(recZoned(call(a)));
		},
		withCalendar(calendarLike) {
			const s = slot(this, KZ, 'withCalendar');
			const cal = toCalendar(calendarLike);
			return make(recZoned(T(O.zMake, s.ems, s.sub, s.tz, cal)));
		},
		withPlainTime(plainTimeLike = undefined) {
			const s = slot(this, KZ, 'withPlainTime');
			const t = timeOrMidnight(plainTimeLike);
			return make(recZoned(call(pushOptTime(pushZoned(bare([O.zWithTime]), s), t))));
		},
		withTimeZone(timeZoneLike) {
			const s = slot(this, KZ, 'withTimeZone');
			const tz = toTimeZone(timeZoneLike);
			return make(recZoned(T(O.zMake, s.ems, s.sub, tz, s.cal)));
		},
		add(temporalDurationLike, options = undefined) {
			return zonedAdd(slot(this, KZ, 'add'), temporalDurationLike, options, false, 'Temporal.ZonedDateTime.prototype.add');
		},
		subtract(temporalDurationLike, options = undefined) {
			return zonedAdd(slot(this, KZ, 'subtract'), temporalDurationLike, options, true, 'Temporal.ZonedDateTime.prototype.subtract');
		},
		until(other, options = undefined) {
			return zonedDiff(slot(this, KZ, 'until'), other, options, false, 'Temporal.ZonedDateTime.prototype.since');
		},
		since(other, options = undefined) {
			return zonedDiff(slot(this, KZ, 'since'), other, options, true, 'Temporal.ZonedDateTime.prototype.since');
		},
		round(roundToArg) {
			const s = slot(this, KZ, 'round');
			// V8 names ZonedDateTime's round as PlainDateTime's.
			const r = roundingOptions(roundTo(roundToArg, KZ), UNIT_DAY, 'Temporal.PlainDateTime.prototype.round');
			return make(recZoned(call(pushRounding(pushZoned(bare([O.zRound]), s), r))));
		},
		equals(other) {
			const s = slot(this, KZ, 'equals');
			const o = toZoned(other, undefined, false);
			return call(pushZoned(pushZoned(bare([O.zEquals]), s), o)) === 1;
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KZ, 'toLocaleString');
			return localeString('zoned', s.cal, s.ems, s.tz, locales, options, s.off);
		},
		toString(options = undefined) {
			const s = slot(this, KZ, 'toString');
			const o = optionsObject(options);
			const where = 'Temporal.ZonedDateTime.prototype.toString';
			const showCal = calendarName(o, where);
			let showOffset;
			const t = toStringOptions(o, function () {
				showOffset = stringOption(o, 'offset', offsetDisplays, 0, where);
			}, where);
			const showZone = stringOption(o, 'timeZoneName', zoneDisplays, 0, where);
			validateUnit(t[1], GROUP_TIME, 0);
			if (t[1] === UNIT_HOUR) {
				throw rangeError('smallestUnit cannot be Hour.');
			}
			const a = pushToString(pushZoned(bare([O.zStr]), s), t);
			push(a, showCal);
			push(a, showOffset);
			push(a, showZone);
			return call(a);
		},
		toJSON() {
			const s = slot(this, KZ, 'toJSON');
			const a = pushZoned(bare([O.zStr]), s);
			push(a, -1);
			push(a, 0);
			push(a, 0);
			push(a, 0);
			push(a, 0);
			push(a, 0);
			return call(a);
		},
		valueOf() {
			throw valueOfError(KZ);
		},
		startOfDay() {
			const s = slot(this, KZ, 'startOfDay');
			return make(recZoned(call(pushZoned(bare([O.zStartOfDay]), s))));
		},
		getTimeZoneTransition(directionParam) {
			const s = slot(this, KZ, 'getTimeZoneTransition');
			let o;
			if (directionParam === undefined) {
				throw typeError('Must specify a direction parameter.');
			} else if (typeof directionParam === 'string') {
				o = {__proto__: null, direction: directionParam};
			} else if (isObject(directionParam)) {
				o = directionParam;
			} else {
				throw typeError('directionParam must be object or string.');
			}
			const next = stringOption(o, 'direction', transitionDirections, -1, 'Temporal.ZonedDateTime.prototype.getTimeZoneTransition') === 0;
			const r = call(push(pushZoned(bare([O.zTransition]), s), next));
			return r === undefined ? null : make(recZoned(r));
		},
		toInstant() {
			const s = slot(this, KZ, 'toInstant');
			return make(recInstant(s.ems, s.sub));
		},
		toPlainDate() {
			return make(dateOf(slot(this, KZ, 'toPlainDate')));
		},
		toPlainTime() {
			return make(timeOf(slot(this, KZ, 'toPlainTime')));
		},
		toPlainDateTime() {
			const s = slot(this, KZ, 'toPlainDateTime');
			return make(recDateTime(pushDateTime(bare([]), s)));
		},
		[toStringTag]: 'Temporal.ZonedDateTime'
	}], bare(['timeZoneId', 'calendarId', 'era', 'eraYear', 'year', 'month', 'monthCode', 'day', 'hour', 'minute',
		'second', 'millisecond', 'microsecond', 'nanosecond', 'epochMilliseconds', 'epochNanoseconds', 'dayOfWeek',
		'dayOfYear', 'weekOfYear', 'yearOfWeek', 'hoursInDay', 'daysInWeek', 'daysInMonth', 'daysInYear',
		'monthsInYear', 'inLeapYear', 'offsetNanoseconds', 'offset', 'with', 'withCalendar', 'withPlainTime',
		'withTimeZone', 'add', 'subtract', 'until', 'since', 'round', 'equals', 'toLocaleString', 'toString', 'toJSON',
		'valueOf', 'startOfDay', 'getTimeZoneTransition', 'toInstant', 'toPlainDate', 'toPlainTime', 'toPlainDateTime',
		toStringTag]));

	const offsetDisplays = bare(['auto', 'never']);
	const zoneDisplays = bare(['auto', 'never', 'critical']);

	function zonedAdd(s, durationLike, options, sub, where) {
		const f = toDuration(durationLike);
		const ov = overflow(options, where);
		return make(recZoned(call(push(push(pushDuration(pushZoned(bare([O.zAdd]), s), f), ov), sub))));
	}

	function zonedDiff(s, other, options, since, where) {
		const o = toZoned(other, undefined, false);
		sameCalendar(s, o);
		const settings = differenceSettings(options, where);
		return make(recDuration(call(push(pushSettings(pushZoned(pushZoned(bare([O.zDiff]), s), o), settings), since))));
	}

	// ------------------------------------------------------------ Duration

	function Duration(years = undefined, months = undefined, weeks = undefined, days = undefined, hours = undefined,
		minutes = undefined, seconds = undefined, milliseconds = undefined, microseconds = undefined,
		nanoseconds = undefined) {
		if (new.target === undefined) {
			throw newRequired(KDU);
		}
		const args = bare([years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds]);
		const a = bare([O.dNew]);
		for (let i = 0; i < 10; i++) {
			const v = args[i];
			if (v === undefined) {
				a[a.length] = 0;
				continue;
			}
			const n = toIntegral(v);
			if (i < 8 && !(n >= -int64Limit && n < int64Limit)) {
				throw rangeError('Integer out of range.');
			}
			a[a.length] = n;
		}
		new Slots(this, recDuration(call(a)));
	}

	function durationSign(f) {
		for (let i = 0; i < 10; i++) {
			if (f[i] !== 0) {
				return f[i] > 0 ? 1 : -1;
			}
		}
		return 0;
	}

	function negated(f) {
		const a = bare([]);
		for (let i = 0; i < 10; i++) {
			a[i] = f[i] === 0 ? 0 : -f[i];
		}
		return {__proto__: null, kind: KDU, f: a};
	}

	defineConstructor(Duration, KDU, {
		from(item) {
			return make({__proto__: null, kind: KDU, f: toDuration(item)});
		},
		compare(one, two, options = undefined) {
			const a = toDuration(one), b = toDuration(two);
			let rel = relativeToNone;
			if (options !== undefined) {
				if (!isObject(options)) {
					throw typeError('Option must be object: relativeTo.');
				}
				rel = relativeTo(options);
			}
			return call(pushRelativeTo(pushDuration(pushDuration(bare([O.dCompare]), a), b), rel));
		}
	}, [{
		get years() {
			return slot(this, KDU, 'years').f[0];
		},
		get months() {
			return slot(this, KDU, 'months').f[1];
		},
		get weeks() {
			return slot(this, KDU, 'weeks').f[2];
		},
		get days() {
			return slot(this, KDU, 'days').f[3];
		},
		get hours() {
			return slot(this, KDU, 'hours').f[4];
		},
		get minutes() {
			return slot(this, KDU, 'minutes').f[5];
		},
		get seconds() {
			return slot(this, KDU, 'seconds').f[6];
		},
		get milliseconds() {
			return slot(this, KDU, 'milliseconds').f[7];
		},
		get microseconds() {
			return slot(this, KDU, 'microseconds').f[8];
		},
		get nanoseconds() {
			return slot(this, KDU, 'nanoseconds').f[9];
		},
		get sign() {
			return durationSign(slot(this, KDU, 'sign').f);
		},
		get blank() {
			return durationSign(slot(this, KDU, 'blank').f) === 0;
		},
		with(temporalDurationLike) {
			const s = slot(this, KDU, 'with');
			const p = partialDuration(temporalDurationLike);
			for (let i = 0; i < 10; i++) {
				if (p[i] === undefined) {
					p[i] = s.f[i];
				}
			}
			return make(recDuration(call(pushDuration(bare([O.dPartial]), p))));
		},
		negated() {
			return make(negated(slot(this, KDU, 'negated').f));
		},
		abs() {
			const f = slot(this, KDU, 'abs').f;
			return make(durationSign(f) < 0 ? negated(f) : {__proto__: null, kind: KDU, f: f});
		},
		add(other) {
			return durationAdd(slot(this, KDU, 'add'), other, false);
		},
		subtract(other) {
			return durationAdd(slot(this, KDU, 'subtract'), other, true);
		},
		round(roundToArg) {
			const s = slot(this, KDU, 'round');
			let o;
			if (roundToArg === undefined) {
				throw typeError('Must specify a roundTo parameter.');
			} else if (typeof roundToArg === 'string') {
				o = {__proto__: null, smallestUnit: roundToArg};
			} else if (isObject(roundToArg)) {
				o = roundToArg;
			} else {
				throw typeError('roundTo must be an object.');
			}
			const where = 'Temporal.Duration.prototype.round';
			const largest = unitOption(o, 'largestUnit', false, where);
			const rel = relativeTo(o);
			const inc = roundingIncrement(o);
			const mode = roundingMode(o, MODE_HALF_EXPAND, where);
			const smallest = unitOption(o, 'smallestUnit', false, where);
			validateUnit(smallest, GROUP_DATETIME, 0);
			const a = pushDuration(bare([O.dRound]), s.f);
			push(a, largest);
			push(a, smallest);
			push(a, mode);
			push(a, inc);
			return make(recDuration(call(pushRelativeTo(a, rel))));
		},
		total(totalOf) {
			const s = slot(this, KDU, 'total');
			let o;
			if (totalOf === undefined) {
				throw typeError('Must specify a totalOf parameter');
			} else if (typeof totalOf === 'string') {
				o = {__proto__: null, unit: totalOf};
			} else if (isObject(totalOf)) {
				o = totalOf;
			} else {
				throw typeError('totalOf must be an object.');
			}
			const rel = relativeTo(o);
			const unit = unitOption(o, 'unit', true, 'Temporal.Duration.prototype.total');
			validateUnit(unit, GROUP_DATETIME, 0);
			return call(pushRelativeTo(push(pushDuration(bare([O.dTotal]), s.f), unit), rel));
		},
		toLocaleString(locales = undefined, options = undefined) {
			const s = slot(this, KDU, 'toLocaleString');
			const a = pushFormatOptions(pushLocales(bare([]), locales), optionsObject(options), dfList);
			const fields = durationFieldsString(s.f);
			for (let j = 0; j < 8; j++) {
				const id = apply(dfResolve, undefined, a);
				const result = dfFormat(id, fields, true);
				if (result !== undefined) return result;
			}
			throw new Error('Intl.DurationFormat: other code made formatters too fast to keep this one');
		},
		toString(options = undefined) {
			const s = slot(this, KDU, 'toString');
			const o = optionsObject(options);
			const digits = fractionalSecondDigits(o);
			const mode = roundingMode(o, MODE_TRUNC, 'Temporal.Duration.prototype.toString');
			const smallest = unitOption(o, 'smallestUnit', false, 'Temporal.Duration.prototype.toString');
			validateUnit(smallest, GROUP_TIME, 0);
			return call(push(push(push(pushDuration(bare([O.dStr]), s.f), digits), smallest), mode));
		},
		toJSON() {
			const s = slot(this, KDU, 'toJSON');
			return call(push(push(push(pushDuration(bare([O.dStr]), s.f), -1), 0), 0));
		},
		valueOf() {
			throw valueOfError(KDU);
		},
		[toStringTag]: 'Temporal.Duration'
	}], bare(['years', 'months', 'weeks', 'days', 'hours', 'minutes', 'seconds', 'milliseconds', 'microseconds',
		'nanoseconds', 'sign', 'blank', 'with', 'negated', 'abs', 'add', 'subtract', 'round', 'total',
		'toLocaleString', 'toString', 'toJSON', 'valueOf', toStringTag]));

	const relativeToNone = bare([0, undefined, undefined, undefined, undefined]);

	function durationAdd(s, other, sub) {
		const o = toDuration(other);
		return make(recDuration(call(push(pushDuration(pushDuration(bare([O.dAdd]), s.f), o), sub))));
	}

	// ----------------------------------------------------------------- Now

	// nowZoned is the current instant in a zone, the VM's default zone where
	// undefined, which is Intl's (ECMA-402's DefaultTimeZone).
	function nowZoned(timeZone) {
		const tz = toTimeZone(timeZone === undefined ? defaultZone : timeZone);
		const n = T(O.now);
		return recZoned(T(O.zMake, n[0], n[1], tz, 'iso8601'));
	}

	const Now = define({}, {
		instant() {
			const n = T(O.now);
			return make(recInstant(n[0], n[1]));
		},
		timeZoneId() {
			return toTimeZone(defaultZone);
		},
		plainDateTimeISO(temporalTimeZoneLike = undefined) {
			return make(recDateTime(pushDateTime(bare([]), nowZoned(temporalTimeZoneLike))));
		},
		zonedDateTimeISO(temporalTimeZoneLike = undefined) {
			return make(nowZoned(temporalTimeZoneLike));
		},
		plainDateISO(temporalTimeZoneLike = undefined) {
			return make(dateOf(nowZoned(temporalTimeZoneLike)));
		},
		plainTimeISO(temporalTimeZoneLike = undefined) {
			return make(timeOf(nowZoned(temporalTimeZoneLike)));
		},
		[toStringTag]: 'Temporal.Now'
	});

	const Temporal = define({}, {
		Now: Now,
		PlainDate: PlainDate,
		PlainTime: PlainTime,
		PlainDateTime: PlainDateTime,
		ZonedDateTime: ZonedDateTime,
		Duration: Duration,
		Instant: Instant,
		PlainYearMonth: PlainYearMonth,
		PlainMonthDay: PlainMonthDay,
		[toStringTag]: 'Temporal'
	});

	const date = {
		toTemporalInstant() {
			const t = apply(dateValueOf, this, noArgs);
			if (t !== t) {
				throw new RangeError('The number NaN cannot be converted to a BigInt because it is not an integer');
			}
			return make(recInstant(t, 0));
		}
	};

	return {__proto__: null, Temporal: Temporal, toTemporalInstant: date.toTemporalInstant};
})
