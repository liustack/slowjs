// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// The javascript half of Intl: the constructors and prototypes of ECMA-402,
// the reading of locales and options, and the toLocale* methods. The work
// is Go's, through the Builtins of bridge.go, which this function takes as
// its parameters after the built-ins stub.js took (P), the VM's default time
// zone, the option lists of the constructors (optionLists) and the state the
// VM shares with Temporal. It returns what stub.js puts in place of its
// stand-ins.
//
// A page may change any built-in before it first uses Intl. So this code
// uses only P and its parameters, reads only own properties of its own
// objects, makes its arrays and records without a prototype, calls through
// apply, and converts with +v and toStr, never with a template literal,
// which QuickJS compiles into a call of String.prototype.concat. The
// internal state of each object is a private field of the class Slots,
// which a page can neither read nor forge.

(function (P, zone, optionLists, state,
	nfResolve, nfFormat, nfParts, nfRange, nfRangeParts,
	dtfResolve, dtfFormat, dtfParts,
	prResolve, prSelect, prSelectRange,
	optionsOf, canonicalLocales, supportedLocales,
	colResolve, colCompare,
	segResolve, segBreaks,
	rtfResolve, rtfFormat, rtfParts,
	lfResolve, lfParts,
	dnResolve, dnOf,
	dfResolve, dfFormat, dfParts,
	valuesOf, localeCheck, localeNew, localeInfo,
	caseLanguage, casePremap,
	tfResolve, tfFormat, tfParts, tfRange, tfRangeParts) {
	'use strict';

	const {
		apply, defineProperty, getOwnPropertyDescriptor, ownKeys, setPrototypeOf, parse,
		Object, String, Error, TypeError, RangeError, Symbol, toStringTag, hasInstance, iterator, iteratorPrototype,
		stringSlice, toUpperCase, toLowerCase, fromCharCode, dateNow, dateValueOf, numberValueOf, bigintValueOf
	} = P;
	const toPrimitive = Symbol.toPrimitive;

	const lists = parse(optionLists);
	const noArgs = bare([]);

	// --------------------------------------------------------------- basics

	function bare(a) {
		return setPrototypeOf(a, null);
	}

	function push(a, v) {
		a[a.length] = v;
	}

	function isObject(v) {
		return typeof v === 'object' ? v !== null : typeof v === 'function';
	}

	function toStr(v) {
		if (typeof v === 'string') {
			return v;
		}
		if (typeof v === 'symbol') {
			throw new TypeError('Cannot convert a Symbol value to a string');
		}
		return String(v);
	}

	function toLength(v) {
		const n = +v;
		if (!(n > 0)) {
			return 0;
		}
		return n < 9007199254740991 ? n - n % 1 : 9007199254740991;
	}

	function toInteger(v) {
		const n = +v;
		if (n !== n) {
			return 0;
		}
		if (n === Infinity || n === -Infinity) {
			return n;
		}
		return n - n % 1;
	}

	function oneOf(v, allowed) {
		for (let i = 0; i < allowed.length; i++) {
			if (allowed[i] === v) {
				return true;
			}
		}
		return false;
	}

	// ---------------------------------------------------------------- slots

	// Slots stamps an object with its internal slots, a record whose kind
	// names what the object is. The record sits in a private field, which
	// leaves the object without a property a page could see, and which no
	// other object can have.
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
			return isObject(o) && #slots in o ? o.#slots : undefined;
		}
	}

	function incompatible(kind) {
		return new TypeError('Intl.' + kind + '.prototype method called on an incompatible receiver');
	}

	// internal returns the record of o, which has to be a kind.
	function internal(o, kind) {
		const r = Slots.of(o);
		if (r === undefined || r.kind !== kind) {
			throw incompatible(kind);
		}
		return r;
	}

	// legacy is the symbol ECMA-402's legacy constructors (ChainNumberFormat,
	// ChainDateTimeFormat) keep a formatter under on an object made from the
	// constructor's prototype, one for each VM.
	const legacy = Symbol('IntlLegacyConstructedSymbol');

	// unwrap is internal for format and resolvedOptions of DateTimeFormat and
	// NumberFormat, which also take an object made by a legacy constructor
	// (UnwrapNumberFormat, UnwrapDateTimeFormat).
	function unwrap(o, kind, C) {
		let r = Slots.of(o);
		if ((r === undefined || r.kind !== kind) && isObject(o) && apply(hasInstance, C, bare([o]))) {
			r = Slots.of(o[legacy]);
		}
		if (r === undefined || r.kind !== kind) {
			throw incompatible(kind);
		}
		return r;
	}

	// chain is the end of a legacy constructor called without new, which made
	// the formatter f: an object made from the prototype of C keeps f under
	// the symbol, and is the result.
	function chain(C, self, f) {
		if (!isObject(self) || !apply(hasInstance, C, bare([self]))) {
			return f;
		}
		defineProperty(self, legacy, {__proto__: null, value: f, writable: false, enumerable: false, configurable: false});
		return self;
	}

	// ----------------------------------------------------------- formatters
	//
	// The record of a formatter has the Builtin that resolves it and its
	// arguments, and the id of the formatter Go keeps. Go may drop a
	// formatter it has more of than it keeps: a call then answers undefined,
	// and the formatter is resolved again from the same arguments.

	function record(kind, resolve, args) {
		const r = {__proto__: null, kind: kind, resolve: resolve, args: args, id: 0, bound: undefined, subs: undefined};
		r.id = apply(resolve, undefined, args);
		return r;
	}

	// call calls the Builtin f of the formatter r with its id and the
	// arguments x to w, resolving the formatter again as long as Go has
	// dropped it, a few times.
	function call(r, f, x, y, z, w) {
		for (let i = 0; i < 8; i++) {
			const v = f(r.id, x, y, z, w);
			if (v !== undefined) {
				return v;
			}
			r.id = apply(r.resolve, undefined, r.args);
		}
		throw new Error('Intl.' + r.kind + ': formatters are made and dropped too fast to use one');
	}

	// make stamps o with a new formatter of kind, resolved from args, the
	// locales and the options.
	function make(o, kind, resolve, args, locales, options, list, coerce) {
		pushLocales(args, locales);
		pushOptions(args, options, list, coerce);
		new Slots(o, record(kind, resolve, args));
		return o;
	}

	// --------------------------------------------------- locales and options

	// localeTag is the tag of an Intl.Locale, else undefined.
	function localeTag(v) {
		const s = Slots.of(v);
		return s !== undefined && s.kind === 'Locale' ? s.locale : undefined;
	}

	state.locale = localeTag;

	// pushLocales appends the two locale arguments of a resolve: a tag given
	// alone, or the list CanonicalizeLocaleList reads, each tag preceded by
	// its length and a colon, which Go makes canonical.
	function pushLocales(a, locales) {
		if (locales === undefined || typeof locales === 'string') {
			push(a, locales);
			push(a, undefined);
			return;
		}
		const tag = localeTag(locales);
		if (tag !== undefined) {
			push(a, tag);
			push(a, undefined);
			return;
		}
		if (locales === null) {
			throw new TypeError('Cannot convert null to object');
		}
		const o = Object(locales), n = toLength(o.length);
		let s = '';
		for (let k = 0; k < n; k++) {
			if (k in o) {
				const t = listTag(o[k]);
				s += t.length + ':' + t;
			}
		}
		push(a, undefined);
		push(a, s);
	}

	// listTag is the tag an element of a list of locales gives.
	function listTag(v) {
		if (typeof v === 'string') {
			return v;
		}
		if (!isObject(v)) {
			throw new TypeError('Language ID should be string or object');
		}
		const tag = localeTag(v);
		return tag !== undefined ? tag : toStr(v);
	}

	// optionsObject is CoerceOptionsToObject when coerce is true, else
	// GetOptionsObject: undefined for no options.
	function optionsObject(options, coerce) {
		if (options === undefined) {
			return undefined;
		}
		if (coerce ? options === null : !isObject(options)) {
			throw new TypeError('Options must be an object');
		}
		return Object(options);
	}

	// pushOptions appends the options of a constructor, each read once, in
	// the order of its list, and converted as its kind says: Go checks them.
	// Without options it appends nothing.
	function pushOptions(a, options, list, coerce) {
		const o = optionsObject(options, coerce);
		if (o === undefined) {
			return;
		}
		const names = list.names, kinds = list.kinds;
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
				default:
					v = v === true ? true : !v ? false : toStr(v);
				}
			}
			push(a, v);
		}
	}

	// stringOption is GetOption of a string option javascript checks itself.
	function stringOption(o, name, allowed) {
		const v = o[name];
		if (v === undefined) {
			return undefined;
		}
		const s = toStr(v);
		if (!oneOf(s, allowed)) {
			throw new RangeError('Value ' + s + ' out of range for options property ' + name);
		}
		return s;
	}

	const matchers = bare(['lookup', 'best fit']);

	// supported is supportedLocalesOf of the constructor named service: Go
	// makes the locales canonical before the options are read.
	function supported(service, locales, options) {
		const a = bare([service]);
		pushLocales(a, locales);
		const r = apply(supportedLocales, undefined, a);
		const o = optionsObject(options, true);
		if (o !== undefined) {
			stringOption(o, 'localeMatcher', matchers);
		}
		return r;
	}

	// ------------------------------------------------------------------ parts

	// partTypes are the types of parts Go gives as their index, made the
	// first time a VM needs them.
	function partTypes() {
		let t = lists.partTypes;
		if (t === null) {
			t = bare(ownKeys(parse(lists.partTypeNames)));
			lists.partTypes = t;
		}
		return t;
	}

	// parts makes the types and values of parts that Go answers, in turn, the
	// parts of formatToParts, in the same array.
	function parts(a) {
		const n = a.length / 2;
		let types;
		for (let i = 0; i < n; i++) {
			let t = a[2 * i];
			if (typeof t === 'number') {
				if (types === undefined) {
					types = partTypes();
				}
				t = types[t];
			}
			a[i] = {type: t, value: a[2 * i + 1]};
		}
		a.length = n;
		return a;
	}

	// triples is parts for parts with a third field, name: the source of a
	// range, or a unit, which a part without one leaves out.
	function triples(a, name) {
		const n = a.length / 3;
		let types;
		for (let i = 0; i < n; i++) {
			let t = a[3 * i];
			if (typeof t === 'number') {
				if (types === undefined) {
					types = partTypes();
				}
				t = types[t];
			}
			const v = a[3 * i + 1], x = a[3 * i + 2];
			if (name === 'source') {
				a[i] = {type: t, value: v, source: x};
			} else if (x === '') {
				a[i] = {type: t, value: v};
			} else {
				a[i] = {type: t, value: v, unit: x};
			}
		}
		a.length = n;
		return a;
	}

	// ------------------------------------------------------------- shaping

	// define puts the properties of props on o, as Intl's: not enumerable,
	// configurable, and writable when they are functions. A function written
	// in props as a method or a getter has the shape of a built-in: no
	// prototype, no constructing, the method's name ("get x" for a getter),
	// and its parameters before the first with a default as its length.
	function define(o, props) {
		const keys = ownKeys(props);
		for (let i = 0; i < keys.length; i++) {
			const d = setPrototypeOf(getOwnPropertyDescriptor(props, keys[i]), null);
			d.enumerable = false;
			if ('value' in d) {
				d.writable = typeof d.value === 'function';
			}
			defineProperty(o, keys[i], d);
		}
		return o;
	}

	// constructor finishes C: its prototype read only, its statics, and the
	// methods of its prototype, after the constructor property.
	function constructor(C, statics, methods) {
		defineProperty(C, 'prototype', {__proto__: null, writable: false});
		define(C, statics);
		define(C.prototype, methods);
		return C;
	}

	function needsNew(name) {
		return new TypeError("Constructor Intl." + name + " requires 'new'");
	}

	// ------------------------------------------------------- DateTimeFormat

	const DTF = 'DateTimeFormat';
	const zonedRefused = 'Temporal.ZonedDateTime is not supported in Intl.DateTimeFormat. Use toLocaleString instead.';

	function DateTimeFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			const f = make({__proto__: DateTimeFormat.prototype}, DTF, dtfResolve, bare([zone, 'any', 'date']), locales,
				options, lists.DateTimeFormat, true);
			return chain(DateTimeFormat, this, f);
		}
		make(this, DTF, dtfResolve, bare([zone, 'any', 'date']), locales, options, lists.DateTimeFormat, true);
	}

	// temporalOf is what state.temporal says of v when it is a Temporal
	// value other than a duration: its kind, calendar and milliseconds.
	function temporalOf(v) {
		if (typeof v !== 'object' || v === null) {
			return undefined;
		}
		const f = state.temporal;
		if (f === undefined) {
			return undefined;
		}
		const t = f(v);
		return t === undefined || t.kind === 'duration' ? undefined : t;
	}

	// sub is the formatter of r for a Temporal kind, made the first time.
	function sub(r, kind) {
		let subs = r.subs;
		if (subs === undefined) {
			subs = r.subs = {__proto__: null};
		}
		let s = subs[kind];
		if (s === undefined) {
			const a = bare([kind]);
			for (let i = 0; i < r.args.length; i++) {
				push(a, r.args[i]);
			}
			s = record(r.kind, tfResolve, a);
			subs[kind] = s;
		}
		return s;
	}

	function formatDate(r, date, toParts) {
		const t = temporalOf(date);
		if (t === undefined) {
			const ms = date === undefined ? dateNow() : +date;
			return toParts ? parts(call(r, dtfParts, ms)) : call(r, dtfFormat, ms);
		}
		if (t.kind === 'zoned') {
			throw new TypeError(zonedRefused);
		}
		const s = sub(r, t.kind);
		return toParts ? parts(call(s, tfParts, t.ms, t.cal)) : call(s, tfFormat, t.ms, t.cal);
	}

	// rangeEnd is an end of formatRange as a Temporal kind, a calendar and
	// milliseconds: a value that is not Temporal is of the kind legacy.
	function rangeEnd(v) {
		const t = temporalOf(v);
		return t !== undefined ? t : {__proto__: null, kind: 'legacy', cal: undefined, ms: +v};
	}

	function formatDateRange(r, start, end, toParts) {
		if (start === undefined || end === undefined) {
			throw new TypeError('Invalid time value');
		}
		const x = rangeEnd(start), y = rangeEnd(end);
		if (x.kind !== y.kind) {
			throw new TypeError('Invalid type for formatRange');
		}
		if (x.kind === 'zoned') {
			throw new TypeError(zonedRefused);
		}
		const s = sub(r, x.kind);
		return toParts ? triples(call(s, tfRangeParts, x.ms, x.cal, y.ms, y.cal), 'source') :
			call(s, tfRange, x.ms, x.cal, y.ms, y.cal);
	}

	constructor(DateTimeFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(DTF, locales, options);
		}
	}, {
		get format() {
			const r = unwrap(this, DTF, DateTimeFormat);
			if (r.bound === undefined) {
				r.bound = (date) => formatDate(r, date, false);
			}
			return r.bound;
		},
		formatToParts(date) {
			return formatDate(internal(this, DTF), date, true);
		},
		formatRange(startDate, endDate) {
			return formatDateRange(internal(this, DTF), startDate, endDate, false);
		},
		formatRangeToParts(startDate, endDate) {
			return formatDateRange(internal(this, DTF), startDate, endDate, true);
		},
		resolvedOptions() {
			return parse(call(unwrap(this, DTF, DateTimeFormat), optionsOf));
		},
		[toStringTag]: 'Intl.DateTimeFormat'
	});

	// --------------------------------------------------------- NumberFormat

	const NF = 'NumberFormat';

	function NumberFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			const f = make({__proto__: NumberFormat.prototype}, NF, nfResolve, bare([]), locales, options,
				lists.NumberFormat, true);
			return chain(NumberFormat, this, f);
		}
		make(this, NF, nfResolve, bare([]), locales, options, lists.NumberFormat, true);
	}

	// big says whether the last value numeric returned is the digits of a
	// BigInt.
	let big = false;

	// numeric is what Go formats of v: a Number, a numeric string, or the
	// digits of a BigInt, which big then says (ToIntlMathematicalValue).
	function numeric(v) {
		if (isObject(v)) {
			v = primitive(v);
		}
		if (typeof v === 'bigint') {
			big = true;
			return String(v);
		}
		big = false;
		return typeof v === 'string' ? v : +v;
	}

	// primitive is ToPrimitive of the object o, with the hint number.
	function primitive(o) {
		const f = o[toPrimitive];
		if (f !== undefined && f !== null) {
			if (typeof f !== 'function') {
				throw new TypeError('Symbol.toPrimitive is not a function');
			}
			const v = apply(f, o, bare(['number']));
			if (isObject(v)) {
				throw new TypeError('Cannot convert object to primitive value');
			}
			return v;
		}
		let m = o.valueOf;
		if (typeof m === 'function') {
			const v = apply(m, o, noArgs);
			if (!isObject(v)) {
				return v;
			}
		}
		m = o.toString;
		if (typeof m === 'function') {
			const v = apply(m, o, noArgs);
			if (!isObject(v)) {
				return v;
			}
		}
		throw new TypeError('Cannot convert object to primitive value');
	}

	function formatNumber(r, value) {
		if (typeof value === 'number') {
			return call(r, nfFormat, value, false);
		}
		const x = numeric(value);
		return call(r, nfFormat, x, big);
	}

	function formatNumberRange(r, start, end, toParts) {
		if (start === undefined || end === undefined) {
			throw new TypeError('start or end is undefined');
		}
		const x = numeric(start), xBig = big;
		const y = numeric(end), yBig = big;
		return toParts ? triples(call(r, nfRangeParts, x, xBig, y, yBig), 'source') : call(r, nfRange, x, xBig, y, yBig);
	}

	constructor(NumberFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(NF, locales, options);
		}
	}, {
		get format() {
			const r = unwrap(this, NF, NumberFormat);
			if (r.bound === undefined) {
				r.bound = (value) => formatNumber(r, value);
			}
			return r.bound;
		},
		formatToParts(value) {
			const r = internal(this, NF);
			const x = numeric(value);
			return parts(call(r, nfParts, x, big));
		},
		formatRange(start, end) {
			return formatNumberRange(internal(this, NF), start, end, false);
		},
		formatRangeToParts(start, end) {
			return formatNumberRange(internal(this, NF), start, end, true);
		},
		resolvedOptions() {
			return parse(call(unwrap(this, NF, NumberFormat), optionsOf));
		},
		[toStringTag]: 'Intl.NumberFormat'
	});

	// ---------------------------------------------------------- PluralRules

	const PR = 'PluralRules';

	function PluralRules(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(PR);
		}
		make(this, PR, prResolve, bare([]), locales, options, lists.PluralRules, true);
	}

	constructor(PluralRules, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(PR, locales, options);
		}
	}, {
		select(value) {
			const r = internal(this, PR);
			return call(r, prSelect, +value);
		},
		selectRange(start, end) {
			const r = internal(this, PR);
			if (start === undefined || end === undefined) {
				throw new TypeError('start or end is undefined');
			}
			const x = +start, y = +end;
			if (x !== x || y !== y) {
				throw new RangeError('start or end is NaN');
			}
			return call(r, prSelectRange, x, y);
		},
		resolvedOptions() {
			return parse(call(internal(this, PR), optionsOf));
		},
		[toStringTag]: 'Intl.PluralRules'
	});

	// ------------------------------------------------------------- Collator

	const CO = 'Collator';

	function Collator(locales = undefined, options = undefined) {
		const o = new.target === undefined ? {__proto__: Collator.prototype} : this;
		return make(o, CO, colResolve, bare([]), locales, options, lists.Collator, true);
	}

	function compare(r, x, y) {
		const s = toStr(x), t = toStr(y);
		return call(r, colCompare, s, t);
	}

	constructor(Collator, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(CO, locales, options);
		}
	}, {
		get compare() {
			const r = internal(this, CO);
			if (r.bound === undefined) {
				r.bound = (x, y) => compare(r, x, y);
			}
			return r.bound;
		},
		resolvedOptions() {
			return parse(call(internal(this, CO), optionsOf));
		},
		[toStringTag]: 'Intl.Collator'
	});

	// ------------------------------------------------------------ Segmenter

	const SG = 'Segmenter';

	function Segmenter(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(SG);
		}
		make(this, SG, segResolve, bare([]), locales, options, lists.Segmenter, false);
	}

	const segmentsProto = {};
	const segmentIteratorProto = setPrototypeOf({}, iteratorPrototype);
	const sliceArgs = bare([0, 0]);

	// segmentAt is the data of the segment of the Segments g that holds the
	// code unit n. g.breaks holds whether the segments may be word-like,
	// then the end of each segment found so far, twice its offset, one more
	// for a word-like segment. More are asked for as they are needed.
	function segmentAt(g, n) {
		let b = g.breaks;
		if (b === undefined) {
			b = g.breaks = bare([0]);
		}
		while (b.length === 1 || b[b.length - 1] >> 1 <= n) {
			const more = call(g.segmenter, segBreaks, g.string, g.cursor, b.length - 1);
			if (more.length <= 2) {
				throw new Error('Intl.Segmenter: internal error: no segment after ' + n);
			}
			g.cursor = more[0];
			if (b.length === 1) {
				b[0] = more[1];
			}
			for (let i = 2; i < more.length; i++) {
				push(b, more[i]);
			}
		}
		// The first end past n ends the segment.
		let lo = 1, hi = b.length - 1;
		while (lo < hi) {
			const mid = (lo + hi) >> 1;
			if (b[mid] >> 1 > n) {
				hi = mid;
			} else {
				lo = mid + 1;
			}
		}
		const start = lo === 1 ? 0 : b[lo - 1] >> 1, end = b[lo] >> 1;
		sliceArgs[0] = start;
		sliceArgs[1] = end;
		const segment = apply(stringSlice, g.string, sliceArgs);
		if (b[0] === 1) {
			return {segment: segment, index: start, input: g.string, isWordLike: (b[lo] & 1) === 1};
		}
		return {segment: segment, index: start, input: g.string};
	}

	define(segmentsProto, {
		containing(index) {
			const g = internal(this, 'Segments');
			const n = toInteger(index);
			if (n < 0 || n >= g.string.length) {
				return undefined;
			}
			return segmentAt(g, n);
		},
		[iterator]() {
			const g = internal(this, 'Segments');
			return new Slots({__proto__: segmentIteratorProto}, {__proto__: null, kind: 'Segment Iterator', segments: g, at: 0});
		}
	});

	define(segmentIteratorProto, {
		next() {
			const it = internal(this, 'Segment Iterator');
			const g = it.segments;
			if (it.at >= g.string.length) {
				return {value: undefined, done: true};
			}
			const d = segmentAt(g, it.at);
			it.at = d.index + d.segment.length;
			return {value: d, done: false};
		},
		[toStringTag]: 'Segmenter String Iterator'
	});

	constructor(Segmenter, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(SG, locales, options);
		}
	}, {
		segment(string) {
			const r = internal(this, SG);
			const s = toStr(string);
			return new Slots({__proto__: segmentsProto},
				{__proto__: null, kind: 'Segments', segmenter: r, string: s, breaks: undefined, cursor: 0});
		},
		resolvedOptions() {
			return parse(call(internal(this, SG), optionsOf));
		},
		[toStringTag]: 'Intl.Segmenter'
	});

	// --------------------------------------------------- RelativeTimeFormat

	const RT = 'RelativeTimeFormat';

	function RelativeTimeFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(RT);
		}
		make(this, RT, rtfResolve, bare([]), locales, options, lists.RelativeTimeFormat, true);
	}

	constructor(RelativeTimeFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(RT, locales, options);
		}
	}, {
		format(value, unit) {
			const r = internal(this, RT);
			const v = +value, u = toStr(unit);
			return call(r, rtfFormat, v, u);
		},
		formatToParts(value, unit) {
			const r = internal(this, RT);
			const v = +value, u = toStr(unit);
			return triples(call(r, rtfParts, v, u), 'unit');
		},
		resolvedOptions() {
			return parse(call(internal(this, RT), optionsOf));
		},
		[toStringTag]: 'Intl.RelativeTimeFormat'
	});

	// ----------------------------------------------------------- ListFormat

	const LF = 'ListFormat';

	function ListFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(LF);
		}
		make(this, LF, lfResolve, bare([]), locales, options, lists.ListFormat, false);
	}

	// stringList is StringListFromIterable.
	function stringList(list) {
		const items = bare([]);
		if (list === undefined) {
			return items;
		}
		for (const x of list) {
			if (typeof x !== 'string') {
				throw new TypeError('Iterable yielded ' + (typeof x === 'symbol' ? 'a symbol' : toStr(x)) +
					' which is not a string');
			}
			push(items, x);
		}
		return items;
	}

	// listParts is what Go makes of the items: the index of an item, or a
	// literal.
	function listParts(r, items) {
		let s = '';
		for (let i = 0; i < items.length; i++) {
			s += items[i].length + ':' + items[i];
		}
		return call(r, lfParts, s);
	}

	constructor(ListFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(LF, locales, options);
		}
	}, {
		format(list) {
			const r = internal(this, LF);
			const items = stringList(list);
			const parts = listParts(r, items);
			let s = '';
			for (let i = 0; i < parts.length; i++) {
				const p = parts[i];
				s += typeof p === 'number' ? items[p] : p;
			}
			return s;
		},
		formatToParts(list) {
			const r = internal(this, LF);
			const items = stringList(list);
			const parts = listParts(r, items);
			for (let i = 0; i < parts.length; i++) {
				const p = parts[i];
				parts[i] = typeof p === 'number' ? {type: 'element', value: items[p]} : {type: 'literal', value: p};
			}
			return parts;
		},
		resolvedOptions() {
			return parse(call(internal(this, LF), optionsOf));
		},
		[toStringTag]: 'Intl.ListFormat'
	});

	// --------------------------------------------------------- DisplayNames

	const DN = 'DisplayNames';

	function DisplayNames(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(DN);
		}
		make(this, DN, dnResolve, bare([]), locales, options, lists.DisplayNames, false);
	}

	defineProperty(DisplayNames, 'length', {__proto__: null, value: 2});

	constructor(DisplayNames, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(DN, locales, options);
		}
	}, {
		of(code) {
			const r = internal(this, DN);
			const v = call(r, dnOf, toStr(code));
			return typeof v === 'number' ? undefined : v;
		},
		resolvedOptions() {
			return parse(call(internal(this, DN), optionsOf));
		},
		[toStringTag]: 'Intl.DisplayNames'
	});

	// ------------------------------------------------------- DurationFormat

	const DF = 'DurationFormat';

	function DurationFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(DF);
		}
		make(this, DF, dfResolve, bare([]), locales, options, lists.DurationFormat, false);
	}

	// The fields of a duration in the order ECMA-402 reads them from an
	// object, and where Go takes each.
	const durationFields = bare(['days', 'hours', 'microseconds', 'milliseconds', 'minutes', 'months', 'nanoseconds',
		'seconds', 'weeks', 'years']);
	const durationSlots = bare([3, 4, 8, 7, 5, 1, 9, 6, 2, 0]);

	// fields says whether the last duration durationOf returned is a list of
	// fields, rather than a string Go parses.
	let fields = false;

	function durationOf(d) {
		if (typeof d === 'string') {
			fields = false;
			return d;
		}
		if (!isObject(d)) {
			throw new TypeError('Invalid duration: ' + typeof d);
		}
		fields = true;
		const f = state.temporal;
		if (f !== undefined) {
			const t = f(d);
			if (t !== undefined && t.kind === 'duration') {
				return t.fields;
			}
		}
		const slots = bare(['', '', '', '', '', '', '', '', '', '']);
		for (let i = 0; i < durationFields.length; i++) {
			const v = d[durationFields[i]];
			if (v !== undefined) {
				const n = +v;
				if (n !== n || n === Infinity || n === -Infinity || n % 1 !== 0) {
					throw new RangeError('Invalid ' + durationFields[i] + ': not a finite integer');
				}
				slots[durationSlots[i]] = String(n);
			}
		}
		let s = slots[0];
		for (let i = 1; i < slots.length; i++) {
			s += ',' + slots[i];
		}
		return s;
	}

	constructor(DurationFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supported(DF, locales, options);
		}
	}, {
		format(duration) {
			const r = internal(this, DF);
			const d = durationOf(duration);
			return call(r, dfFormat, d, fields);
		},
		formatToParts(duration) {
			const r = internal(this, DF);
			const d = durationOf(duration);
			return triples(call(r, dfParts, d, fields), 'unit');
		},
		resolvedOptions() {
			return parse(call(internal(this, DF), optionsOf));
		},
		[toStringTag]: 'Intl.DurationFormat'
	});

	// --------------------------------------------------------------- Locale

	const LO = 'Locale';
	const localeSubtags = bare(['language', 'script', 'region', 'variants', 'calendar', 'collation', 'firstDayOfWeek']);
	const hourCycles = bare(['h11', 'h12', 'h23', 'h24']);
	const caseFirsts = bare(['upper', 'lower', 'false']);

	function Locale(tag, options = undefined) {
		if (new.target === undefined) {
			throw needsNew(LO);
		}
		if (tag === null || typeof tag !== 'string' && !isObject(tag)) {
			throw new TypeError("First argument to Intl.Locale constructor can't be empty or missing");
		}
		const given = localeTag(tag);
		const t = given !== undefined ? given : toStr(tag);
		const o = optionsObject(options, true);
		localeCheck('tag', t);
		let json;
		if (o === undefined) {
			json = localeNew(t);
		} else {
			const a = bare([t]);
			for (let i = 0; i < localeSubtags.length; i++) {
				let v = o[localeSubtags[i]];
				if (v !== undefined) {
					v = toStr(v);
					localeCheck(localeSubtags[i], v);
				}
				push(a, v);
			}
			push(a, stringOption(o, 'hourCycle', hourCycles));
			push(a, stringOption(o, 'caseFirst', caseFirsts));
			let numeric = o.numeric;
			if (numeric !== undefined) {
				numeric = numeric ? 'true' : 'false';
			}
			push(a, numeric);
			let system = o.numberingSystem;
			if (system !== undefined) {
				system = toStr(system);
			}
			push(a, system);
			json = apply(localeNew, undefined, a);
		}
		new Slots(this, setPrototypeOf(parse(json), null));
	}

	function info(o, what) {
		return localeInfo(internal(o, LO).locale, what);
	}

	constructor(Locale, {}, {
		toString() {
			return internal(this, LO).locale;
		},
		maximize() {
			return new Locale(info(this, 'maximize'));
		},
		minimize() {
			return new Locale(info(this, 'minimize'));
		},
		get language() {
			return internal(this, LO).language;
		},
		get script() {
			return internal(this, LO).script;
		},
		get region() {
			return internal(this, LO).region;
		},
		get variants() {
			return internal(this, LO).variants;
		},
		get baseName() {
			return internal(this, LO).baseName;
		},
		get calendar() {
			return internal(this, LO).calendar;
		},
		get caseFirst() {
			return internal(this, LO).caseFirst;
		},
		get collation() {
			return internal(this, LO).collation;
		},
		get firstDayOfWeek() {
			return internal(this, LO).firstDayOfWeek;
		},
		get hourCycle() {
			return internal(this, LO).hourCycle;
		},
		get numeric() {
			return internal(this, LO).numeric;
		},
		get numberingSystem() {
			return internal(this, LO).numberingSystem;
		},
		getCalendars() {
			return info(this, 'calendars');
		},
		getCollations() {
			return info(this, 'collations');
		},
		getHourCycles() {
			return info(this, 'hourCycles');
		},
		getNumberingSystems() {
			return info(this, 'numberingSystems');
		},
		getTimeZones() {
			return info(this, 'timeZones');
		},
		getTextInfo() {
			const d = info(this, 'direction');
			return d === undefined ? {} : {direction: d};
		},
		getWeekInfo() {
			const tag = internal(this, LO).locale;
			return {firstDay: localeInfo(tag, 'firstDay'), weekend: localeInfo(tag, 'weekend')};
		},
		[toStringTag]: 'Intl.Locale'
	});

	// ----------------------------------------------------------------- Intl

	const Intl = {};
	define(Intl, {
		getCanonicalLocales(locales) {
			const a = bare([]);
			pushLocales(a, locales);
			return apply(canonicalLocales, undefined, a);
		},
		supportedValuesOf(key) {
			return valuesOf(toStr(key));
		},
		DateTimeFormat: DateTimeFormat,
		NumberFormat: NumberFormat,
		Collator: Collator,
		PluralRules: PluralRules,
		RelativeTimeFormat: RelativeTimeFormat,
		ListFormat: ListFormat,
		Locale: Locale,
		DisplayNames: DisplayNames,
		Segmenter: Segmenter,
		DurationFormat: DurationFormat,
		[toStringTag]: 'Intl'
	});

	// ------------------------------------------------------- toLocale*

	// The formatters the toLocale* methods make for a locale given as a
	// string, or none, and no options, which a page asks for again and again.
	// A full cache is emptied.
	let cache = {__proto__: null}, cached = 0;

	function cachedRecord(kind, locales, options, makeRecord) {
		if (options !== undefined || locales !== undefined && typeof locales !== 'string') {
			return makeRecord(locales, options);
		}
		const key = locales === undefined ? kind : kind + ' ' + locales;
		let r = cache[key];
		if (r === undefined) {
			r = makeRecord(locales, options);
			if (cached === 64) {
				cache = {__proto__: null};
				cached = 0;
			}
			cache[key] = r;
			cached++;
		}
		return r;
	}

	function newRecord(kind, resolve, args, locales, options, list) {
		pushLocales(args, locales);
		pushOptions(args, options, list, true);
		return record(kind, resolve, args);
	}

	function anyDate(locales, options) {
		return newRecord(DTF, dtfResolve, bare([zone, 'any', 'all']), locales, options, lists.DateTimeFormat);
	}

	function dateOnly(locales, options) {
		return newRecord(DTF, dtfResolve, bare([zone, 'date', 'date']), locales, options, lists.DateTimeFormat);
	}

	function timeOnly(locales, options) {
		return newRecord(DTF, dtfResolve, bare([zone, 'time', 'time']), locales, options, lists.DateTimeFormat);
	}

	function numberRecord(locales, options) {
		return newRecord(NF, nfResolve, bare([]), locales, options, lists.NumberFormat);
	}

	function collatorRecord(locales, options) {
		return newRecord(CO, colResolve, bare([]), locales, options, lists.Collator);
	}

	// defaultCollator is localeCompare's with no locales and no options.
	let defaultCollator;

	// firstLocale is the tag of the first locale of locales, the only one
	// toLocaleUpperCase and toLocaleLowerCase read, or undefined for none.
	function firstLocale(locales) {
		if (locales === undefined || typeof locales === 'string') {
			return locales;
		}
		const tag = localeTag(locales);
		if (tag !== undefined) {
			return tag;
		}
		if (locales === null) {
			throw new TypeError('Cannot convert null to object');
		}
		const o = Object(locales), n = toLength(o.length);
		for (let k = 0; k < n; k++) {
			if (k in o) {
				return listTag(o[k]);
			}
		}
		return undefined;
	}

	function changeCase(s, locales, upper) {
		const tag = firstLocale(locales);
		if (tag !== undefined) {
			const language = caseLanguage(tag);
			if (language !== '') {
				const m = casePremap(s, language, upper);
				if (typeof m === 'string') {
					s = m;
				} else if (m !== undefined) {
					let t = '';
					for (let i = 0; i < m.length; i++) {
						const x = m[i];
						t += typeof x === 'number' ? fromCharCode(x) : x;
					}
					s = t;
				}
			}
		}
		return apply(upper ? toUpperCase : toLowerCase, s, noArgs);
	}

	function stringThis(s, name) {
		if (s === undefined || s === null) {
			throw new TypeError('String.prototype.' + name + ' called on null or undefined');
		}
		return toStr(s);
	}

	const dateMethods = {
		toLocaleString(locales = undefined, options = undefined) {
			const t = apply(dateValueOf, this, noArgs);
			if (t !== t) {
				return 'Invalid Date';
			}
			return call(cachedRecord('any', locales, options, anyDate), dtfFormat, t);
		},
		toLocaleDateString(locales = undefined, options = undefined) {
			const t = apply(dateValueOf, this, noArgs);
			if (t !== t) {
				return 'Invalid Date';
			}
			return call(cachedRecord('date', locales, options, dateOnly), dtfFormat, t);
		},
		toLocaleTimeString(locales = undefined, options = undefined) {
			const t = apply(dateValueOf, this, noArgs);
			if (t !== t) {
				return 'Invalid Date';
			}
			return call(cachedRecord('time', locales, options, timeOnly), dtfFormat, t);
		}
	};

	const numberMethods = {
		toLocaleString(locales = undefined, options = undefined) {
			const r = cachedRecord('number', locales, options, numberRecord);
			return call(r, nfFormat, apply(numberValueOf, this, noArgs), false);
		}
	};

	const bigintMethods = {
		toLocaleString(locales = undefined, options = undefined) {
			const r = cachedRecord('number', locales, options, numberRecord);
			return call(r, nfFormat, String(apply(bigintValueOf, this, noArgs)), true);
		}
	};

	const stringMethods = {
		localeCompare(that, locales = undefined, options = undefined) {
			const s = stringThis(this, 'localeCompare'), t = toStr(that);
			let r;
			if (locales === undefined && options === undefined) {
				if (defaultCollator === undefined) {
					defaultCollator = collatorRecord(undefined, undefined);
				}
				r = defaultCollator;
			} else {
				r = cachedRecord('collator', locales, options, collatorRecord);
			}
			return call(r, colCompare, s, t);
		},
		toLocaleUpperCase(locales = undefined) {
			return changeCase(stringThis(this, 'toLocaleUpperCase'), locales, true);
		},
		toLocaleLowerCase(locales = undefined) {
			return changeCase(stringThis(this, 'toLocaleLowerCase'), locales, false);
		}
	};

	const api = {
		__proto__: null,
		Intl: Intl,
		dateToLocaleString: dateMethods.toLocaleString,
		dateToLocaleDateString: dateMethods.toLocaleDateString,
		dateToLocaleTimeString: dateMethods.toLocaleTimeString,
		numberToLocaleString: numberMethods.toLocaleString,
		stringLocaleCompare: stringMethods.localeCompare,
		stringToLocaleUpperCase: stringMethods.toLocaleUpperCase,
		stringToLocaleLowerCase: stringMethods.toLocaleLowerCase
	};
	if (bigintValueOf !== undefined) {
		api.bigintToLocaleString = bigintMethods.toLocaleString;
	}
	return api;
})
