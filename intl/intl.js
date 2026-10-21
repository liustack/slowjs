// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// The javascript half of modernc.org/quickjs/intl. It owns the ECMA-402
// surface - the constructors, their prototypes and the option plumbing the
// spec makes observable - and leaves every formatting decision to the Go
// Builtins it is called with (see bridge.go). It evaluates to a function of
// the built-ins stub.js took before the page ran, the VM's default time zone,
// the json of each constructor's options and the Builtins, which returns what
// stub.js puts in place.
//
// Nothing here may depend on what a page can change: it uses only what it is
// called with, reads only own properties of its own objects, and builds
// property descriptors and the arrays it passes on without a prototype.

(function (P, zone, optionLists,
	nfResolve, nfFormat, nfParts, dtfResolve, dtfFormat, dtfParts, prResolve, prSelect, prSelectRange,
	optionsOf, canonicalLocales, supportedLocales,
	colResolve, colCompare, segResolve, segBreaks, rtfResolve, rtfFormat, rtfParts,
	lfResolve, lfParts, dnResolve, dnOf, dfResolve, dfFormat, dfParts, valuesOf, localeCheck, localeNew, localeInfo, caseLanguage, casePremap) {
	'use strict';

	const {
		apply, defineProperty, getOwnPropertyDescriptor, ownKeys, setPrototypeOf, parse,
		Object, String, Number, Error, TypeError, RangeError, Symbol, toStringTag, hasInstance,
		iterator, iteratorPrototype, stringSlice, toUpperCase, toLowerCase, fromCharCode,
		dateNow, dateValueOf, numberValueOf, bigintValueOf
	} = P;
	const lists = parse(optionLists);
	const noArgs = [];
	const noOptions = setPrototypeOf({}, null);

	// bare returns the array a without a prototype, so that adding to it calls
	// no setter a page put on Array.prototype.
	function bare(a) {
		return setPrototypeOf(a, null);
	}

	// Slots holds the internal slots of the objects Intl makes, in a private
	// field, which nothing outside this function can read, forge or replace.
	// Its base class returns the object it is given, so that the field goes on
	// that object.
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

	// A formatter is a record: the Builtin that resolves it, the arguments it
	// was resolved with and the id of what that resolved to, which every
	// other call passes on. A Builtin answers undefined for an id the
	// process has since dropped (see registry.go): call then resolves the
	// same arguments again and repeats the call. Other VMs making formatters
	// fast enough can drop the new one too before the call gets to it, so
	// call tries a few times, then throws: it never returns undefined.
	function resolve(kind, fn, args) {
		return {__proto__: null, kind: kind, fn: fn, args: args, id: apply(fn, undefined, args), bound: undefined};
	}

	const tries = 8;

	function call(r, fn, x, y) {
		for (let i = 0; i < tries; i++) {
			const v = fn(r.id, x, y);
			if (v !== undefined) {
				return v;
			}
			r.id = apply(r.fn, undefined, r.args);
		}
		throw new Error('Intl.' + r.kind + ': other code made formatters too fast to keep this one');
	}

	// internal returns the internal slots of o, a kind object. For the format
	// getter and resolvedOptions of DateTimeFormat and NumberFormat, C is the
	// constructor: an o that is not one, but inherits from C.prototype, may
	// hold one at the fallback symbol (see chain).
	function internal(o, kind, C) {
		let i = Slots.of(o);
		if (i === undefined && C !== undefined && apply(hasInstance, C, [o])) {
			i = Slots.of(o[fallback]);
		}
		if (i === undefined || i.kind !== kind) {
			throw new TypeError('Intl.' + kind + '.prototype method called on an incompatible receiver');
		}
		return i;
	}

	// chain is ECMA-402's ChainNumberFormat and ChainDateTimeFormat, the
	// normative optional mode browsers implement: C called as a function on o,
	// an object that inherits from C.prototype, keeps f, the formatter it
	// made, at a symbol property of o and returns o.
	const fallback = Symbol('IntlLegacyConstructedSymbol');

	function chain(C, f, o) {
		if (!apply(hasInstance, C, [o])) {
			return f;
		}
		defineProperty(o, fallback, {__proto__: null, value: f, writable: false, enumerable: false, configurable: false});
		return o;
	}

	// define puts the properties of the object literal props on o as ECMA-402
	// has those of its built-ins: not enumerable, configurable, and writable
	// but for constants such as Symbol.toStringTag. A function written in
	// props as a method or a getter is shaped like a built-in function: it
	// has no prototype property and cannot construct, its name is the
	// method's ("get format" for a getter) and its length the number of its
	// parameters before the first one with a default (x = undefined).
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

	// defineConstructor finishes C, a constructor written as a function declaration
	// whose parameters all have defaults, so that its length is 0: its
	// prototype property becomes read only, statics go on C and methods on
	// its prototype, both through define.
	function defineConstructor(C, statics, methods) {
		defineProperty(C, 'prototype', {__proto__: null, writable: false});
		define(C, statics);
		define(C.prototype, methods);
		return C;
	}

	// localeTag is the tag of o where o is an Intl.Locale, else undefined.
	function localeTag(o) {
		const i = Slots.of(o);
		return i !== undefined && i.kind === 'Locale' ? i.locale : undefined;
	}

	// localeList implements CanonicalizeLocaleList's argument handling. The tags
	// themselves are canonicalized by the host. An Intl.Locale is its tag,
	// whatever its toString says.
	function localeList(locales) {
		if (locales === null) {
			throw new TypeError('Cannot convert null to object');
		}
		const o = Object(locales), n = toLength(o.length), r = bare([]);
		for (let i = 0; i < n; i++) {
			if (i in o) {
				const v = o[i];
				if (typeof v !== 'string' && (v === null || typeof v !== 'object')) {
					throw new TypeError('Language ID should be string or object');
				}
				const tag = localeTag(v);
				r[r.length] = tag !== undefined ? tag : toString(v);
			}
		}
		return r;
	}

	// pushLocales appends the two locale arguments of a Builtin to args: a
	// single tag as itself, a list as one string of the tags each preceded by
	// its length and a colon. A single Intl.Locale is its tag.
	function pushLocales(args, locales) {
		const n = args.length;
		const tag = localeTag(locales);
		if (tag !== undefined) {
			locales = tag;
		}
		if (typeof locales === 'string') {
			args[n] = locales;
			args[n + 1] = undefined;
		} else if (locales === undefined) {
			args[n] = undefined;
			args[n + 1] = undefined;
		} else {
			const l = localeList(locales);
			let s = '';
			for (let i = 0; i < l.length; i++) {
				s += l[i].length + ':' + l[i];
			}
			args[n] = undefined;
			args[n + 1] = s;
		}
		return args;
	}

	// optionsObject is ECMA-402's GetOptionsObject: the constructors added
	// after the first edition take an object or nothing, and convert no
	// other value.
	function optionsObject(o) {
		if (o === undefined) {
			return noOptions;
		}
		if (o === null || (typeof o !== 'object' && typeof o !== 'function')) {
			throw new TypeError('Options must be an object');
		}
		return o;
	}

	// requireNew throws the TypeError of a constructor called as a function,
	// for those added after the first edition.
	function requireNew(newTarget, name) {
		if (newTarget === undefined) {
			throw new TypeError("Constructor Intl." + name + " requires 'new'");
		}
	}

	function toOptions(o) {
		if (o === null) {
			throw new TypeError('Options must be an object');
		}
		return o === undefined ? noOptions : Object(o);
	}

	// toString is ECMA-262's ToString, which throws for a Symbol where
	// String() names it. A template literal is ToString too, but QuickJS
	// compiles one into a call of String.prototype.concat, which a page can
	// replace.
	function toString(v) {
		if (typeof v === 'symbol') {
			throw new TypeError('Cannot convert a Symbol value to a string');
		}
		return String(v);
	}

	// toLength is ECMA-262's ToLength. The conversions here are the spec's:
	// +v is ToNumber, which throws for a BigInt where Number() converts it,
	// and toString ToString.
	function toLength(v) {
		const n = +v;
		if (!(n > 0)) {
			return 0;
		}
		return n < 9007199254740991 ? n - n % 1 : 9007199254740991;
	}

	// pushOptions appends the options of o in list, from optionLists, to
	// args, converted as their kind says. Without options, nothing: a
	// Builtin reads missing arguments as undefined.
	function pushOptions(args, o, list) {
		if (o === noOptions) {
			return args;
		}
		const n = args.length, names = list.names, kinds = list.kinds;
		for (let i = 0; i < names.length; i++) {
			let v = o[names[i]];
			if (v !== undefined) {
				switch (kinds[i]) {
				case 's':
					v = toString(v);
					break;
				case 'n':
					v = +v;
					break;
				case 'b':
					v = !!v;
					break;
				default:
					// GetBooleanOrStringNumberFormatOption: true, false for
					// anything falsy, else a string.
					if (v !== true) {
						v = v ? toString(v) : false;
					}
				}
			}
			args[n + i] = v;
		}
		return args;
	}

	// parts turns the array of types and values, in turn, a Builtin answers
	// for formatToParts into its result, in place: element i takes the part
	// made of elements 2i and 2i+1, already read, and the array is then cut
	// to half its length. Only own elements are set, so no setter on
	// Array.prototype runs.
	function parts(a) {
		const n = a.length / 2;
		for (let i = 0; i < n; i++) {
			a[i] = {type: a[2 * i], value: a[2 * i + 1]};
		}
		a.length = n;
		return a;
	}

	// unitParts is parts for the type, value and unit triples a Builtin
	// answers, with a unit for the parts that have one. Each part is an
	// object literal, so no setter on Object.prototype takes its unit.
	function unitParts(a) {
		const n = a.length / 3;
		for (let i = 0; i < n; i++) {
			const type = a[3 * i], value = a[3 * i + 1], unit = a[3 * i + 2];
			a[i] = unit !== '' ? {type: type, value: value, unit: unit} : {type: type, value: value};
		}
		a.length = n;
		return a;
	}

	// stringOption is ECMA-402's GetOption for a string option of o, an
	// options object, that takes the values in allowed: undefined when o
	// has none.
	function stringOption(o, name, allowed) {
		const v = o[name];
		if (v === undefined) {
			return undefined;
		}
		const s = toString(v);
		for (let i = 0; i < allowed.length; i++) {
			if (allowed[i] === s) {
				return s;
			}
		}
		throw new RangeError('Value ' + s + ' out of range for options property ' + name);
	}

	const matchers = bare(['lookup', 'best fit']);

	// supportedLocalesOf is ECMA-402's SupportedLocales after
	// CanonicalizeLocaleList: the host canonicalizes the locales, which
	// throws for a tag that is not one, and then the options are read. Both
	// matchers match alike, so localeMatcher is only checked.
	function supportedLocalesOf(service, locales, options) {
		const r = apply(supportedLocales, undefined, pushLocales(bare([service]), locales));
		const o = toOptions(options);
		if (o !== noOptions) {
			stringOption(o, 'localeMatcher', matchers);
		}
		return r;
	}

	// ---------------------------------------------------------- DateTimeFormat

	// dateFormat resolves a DateTimeFormat. required and defaults are
	// ECMA-402's ToDateTimeOptions arguments: they are what make
	// toLocaleDateString a date and toLocaleTimeString a time. The host
	// applies them, because the defaults it adds depend on the locale's hour
	// cycle.
	function dateFormat(locales, opts, required, defaults) {
		const args = pushLocales(bare([zone, required, defaults]), locales);
		return resolve('DateTimeFormat', dtfResolve, pushOptions(args, toOptions(opts), lists.DateTimeFormat));
	}

	function DateTimeFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			return chain(DateTimeFormat, new DateTimeFormat(locales, options), this);
		}
		new Slots(this, dateFormat(locales, options, 'any', 'date'));
	}

	function timeValue(date) {
		return date === undefined ? dateNow() : +date;
	}

	defineConstructor(DateTimeFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('DateTimeFormat', locales, options);
		}
	}, {
		get format() {
			// The spec hands out a bound function so that [d1, d2].map(f.format)
			// keeps working. Code in the wild does exactly that.
			const i = internal(this, 'DateTimeFormat', DateTimeFormat);
			if (i.bound === undefined) {
				i.bound = (date) => call(i, dtfFormat, timeValue(date));
			}
			return i.bound;
		},
		formatToParts(date) {
			return parts(call(internal(this, 'DateTimeFormat'), dtfParts, timeValue(date)));
		},
		resolvedOptions() {
			return parse(call(internal(this, 'DateTimeFormat', DateTimeFormat), optionsOf));
		},
		[toStringTag]: 'Intl.DateTimeFormat'
	});

	// ------------------------------------------------------------ NumberFormat

	function numberFormat(locales, opts) {
		const args = pushLocales(bare([]), locales);
		return resolve('NumberFormat', nfResolve, pushOptions(args, toOptions(opts), lists.NumberFormat));
	}

	function NumberFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			return chain(NumberFormat, new NumberFormat(locales, options), this);
		}
		new Slots(this, numberFormat(locales, options));
	}

	// numberValue is ECMA-402's ToIntlMathematicalValue. It keeps a BigInt
	// and a numeric string exact by passing them on as a string, also when an
	// object, such as Object(1n), converts to one: the host parses decimal
	// digits, so nothing round trips through a double that did not start as
	// one.
	function numberValue(v) {
		const p = isObject(v) ? toPrimitiveNumber(v) : v;
		const t = typeof p;
		if (t === 'bigint' || t === 'string') {
			return String(p);
		}
		return +p;
	}

	function isObject(v) {
		return typeof v === 'object' ? v !== null : typeof v === 'function';
	}

	// toPrimitiveNumber is ToPrimitive(o, number), for an object o: what
	// Number(o) does before it converts the result to a number.
	const toPrimitive = Symbol.toPrimitive;
	const hintNumber = bare(['number']);
	const ordinary = bare(['valueOf', 'toString']);

	function toPrimitiveNumber(o) {
		const exotic = o[toPrimitive];
		if (exotic !== undefined && exotic !== null) {
			if (typeof exotic !== 'function') {
				throw new TypeError('Symbol.toPrimitive is not a function');
			}
			const r = apply(exotic, o, hintNumber);
			if (isObject(r)) {
				throw new TypeError('Cannot convert object to primitive value');
			}
			return r;
		}
		for (let i = 0; i < ordinary.length; i++) {
			const f = o[ordinary[i]];
			if (typeof f === 'function') {
				const r = apply(f, o, noArgs);
				if (!isObject(r)) {
					return r;
				}
			}
		}
		throw new TypeError('Cannot convert object to primitive value');
	}

	defineConstructor(NumberFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('NumberFormat', locales, options);
		}
	}, {
		get format() {
			const i = internal(this, 'NumberFormat', NumberFormat);
			if (i.bound === undefined) {
				i.bound = (value) => call(i, nfFormat, numberValue(value));
			}
			return i.bound;
		},
		formatToParts(value) {
			return parts(call(internal(this, 'NumberFormat'), nfParts, numberValue(value)));
		},
		resolvedOptions() {
			return parse(call(internal(this, 'NumberFormat', NumberFormat), optionsOf));
		},
		[toStringTag]: 'Intl.NumberFormat'
	});

	// ------------------------------------------------------------- PluralRules

	function PluralRules(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw new TypeError("Constructor Intl.PluralRules requires 'new'");
		}
		const args = pushLocales(bare([]), locales);
		new Slots(this, resolve('PluralRules', prResolve, pushOptions(args, toOptions(options), lists.PluralRules)));
	}

	defineConstructor(PluralRules, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('PluralRules', locales, options);
		}
	}, {
		select(value) {
			return call(internal(this, 'PluralRules'), prSelect, +value);
		},
		selectRange(start, end) {
			const i = internal(this, 'PluralRules');
			if (start === undefined || end === undefined) {
				throw new TypeError('start or end is undefined');
			}
			const x = +start, y = +end;
			if (x !== x || y !== y) {
				throw new RangeError('start or end is NaN');
			}
			return call(i, prSelectRange, x, y);
		},
		resolvedOptions() {
			return parse(call(internal(this, 'PluralRules'), optionsOf));
		},
		[toStringTag]: 'Intl.PluralRules'
	});

	// ---------------------------------------------------------------- Collator

	function collator(locales, opts) {
		const args = pushLocales(bare([]), locales);
		return resolve('Collator', colResolve, pushOptions(args, toOptions(opts), lists.Collator));
	}

	function Collator(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			return new Collator(locales, options);
		}
		new Slots(this, collator(locales, options));
	}

	defineConstructor(Collator, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('Collator', locales, options);
		}
	}, {
		get compare() {
			// Bound, as format is, because it is handed to sort.
			const i = internal(this, 'Collator');
			if (i.bound === undefined) {
				i.bound = (x, y) => call(i, colCompare, toString(x), toString(y));
			}
			return i.bound;
		},
		resolvedOptions() {
			return parse(call(internal(this, 'Collator'), optionsOf));
		},
		[toStringTag]: 'Intl.Collator'
	});

	// --------------------------------------------------------------- Segmenter

	function Segmenter(locales = undefined, options = undefined) {
		requireNew(new.target, 'Segmenter');
		const args = pushLocales(bare([]), locales);
		new Slots(this, resolve('Segmenter', segResolve, pushOptions(args, optionsObject(options), lists.Segmenter)));
	}

	defineConstructor(Segmenter, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('Segmenter', locales, options);
		}
	}, {
		segment(string) {
			const i = internal(this, 'Segmenter');
			return new Slots({__proto__: segmentsPrototype}, {
				__proto__: null, kind: 'Segments', segmenter: i, string: toString(string), breaks: undefined
			});
		},
		resolvedOptions() {
			return parse(call(internal(this, 'Segmenter'), optionsOf));
		},
		[toStringTag]: 'Intl.Segmenter'
	});

	// breaks returns what the segBreaks Builtin answers for s, a Segments'
	// slots: 1 if its segments may be word-like, else 0, then where each
	// segment ends, as twice the offset, plus one for a word-like segment.
	// It is asked on first use and kept.
	function breaks(s) {
		if (s.breaks === undefined) {
			s.breaks = call(s.segmenter, segBreaks, s.string);
		}
		return s.breaks;
	}

	function half(n) {
		return (n - n % 2) / 2;
	}

	// segmentAt is the segment data of the segment of s that holds the
	// offset n, which is in s's string, and where it ends: the first break
	// past n, found by bisection, ends it.
	function segmentAt(s, n) {
		const b = breaks(s);
		let lo = 1, hi = b.length;
		while (lo < hi) {
			const mid = (lo + hi) >>> 1;
			if (half(b[mid]) > n) {
				hi = mid;
			} else {
				lo = mid + 1;
			}
		}
		const start = lo === 1 ? 0 : half(b[lo - 1]), end = half(b[lo]);
		const segment = apply(stringSlice, s.string, [start, end]);
		// An object literal, so no setter on Object.prototype takes isWordLike.
		const d = b[0] === 1 ?
			{segment: segment, index: start, input: s.string, isWordLike: b[lo] % 2 === 1} :
			{segment: segment, index: start, input: s.string};
		return {__proto__: null, data: d, end: end};
	}

	// toIntegerOrInfinity is ECMA-262's ToIntegerOrInfinity.
	function toIntegerOrInfinity(v) {
		const n = +v;
		if (n !== n) {
			return 0;
		}
		return n === n + 1 ? n : n - n % 1;
	}

	const segmentsPrototype = define({}, {
		containing(index) {
			const s = internal(this, 'Segments');
			const n = toIntegerOrInfinity(index);
			if (n < 0 || n >= s.string.length) {
				return undefined;
			}
			return segmentAt(s, n).data;
		},
		[iterator]() {
			const s = internal(this, 'Segments');
			return new Slots({__proto__: segmentIteratorPrototype}, {__proto__: null, kind: 'Segment Iterator', segments: s, at: 0});
		}
	});

	const segmentIteratorPrototype = define({__proto__: iteratorPrototype}, {
		next() {
			const i = internal(this, 'Segment Iterator'), s = i.segments;
			if (i.at >= s.string.length) {
				return {value: undefined, done: true};
			}
			const r = segmentAt(s, i.at);
			i.at = r.end;
			return {value: r.data, done: false};
		},
		[toStringTag]: 'Segmenter String Iterator'
	});

	// ------------------------------------------------------ RelativeTimeFormat

	function RelativeTimeFormat(locales = undefined, options = undefined) {
		requireNew(new.target, 'RelativeTimeFormat');
		const args = pushLocales(bare([]), locales);
		new Slots(this, resolve('RelativeTimeFormat', rtfResolve,
			pushOptions(args, toOptions(options), lists.RelativeTimeFormat)));
	}

	defineConstructor(RelativeTimeFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('RelativeTimeFormat', locales, options);
		}
	}, {
		format(value, unit) {
			const i = internal(this, 'RelativeTimeFormat'), v = +value;
			return call(i, rtfFormat, v, toString(unit));
		},
		formatToParts(value, unit) {
			const i = internal(this, 'RelativeTimeFormat'), v = +value;
			return unitParts(call(i, rtfParts, v, toString(unit)));
		},
		resolvedOptions() {
			return parse(call(internal(this, 'RelativeTimeFormat'), optionsOf));
		},
		[toStringTag]: 'Intl.RelativeTimeFormat'
	});

	// -------------------------------------------------------------- ListFormat

	function ListFormat(locales = undefined, options = undefined) {
		requireNew(new.target, 'ListFormat');
		const args = pushLocales(bare([]), locales);
		new Slots(this, resolve('ListFormat', lfResolve, pushOptions(args, optionsObject(options), lists.ListFormat)));
	}

	// listOf is ECMA-402's StringListFromIterable, the items as one string
	// for the lfParts Builtin, each its length, a colon and the item, and
	// the items themselves.
	function listOf(iterable) {
		const items = bare([]);
		let s = '';
		if (iterable !== undefined) {
			for (const v of iterable) {
				if (typeof v !== 'string') {
					throw new TypeError('Iterable yielded ' + (typeof v === 'symbol' ? 'a symbol' : toString(v)) + ' which is not a string');
				}
				items[items.length] = v;
				s += v.length + ':' + v;
			}
		}
		return {__proto__: null, items: items, s: s};
	}

	defineConstructor(ListFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('ListFormat', locales, options);
		}
	}, {
		format(list) {
			const i = internal(this, 'ListFormat'), l = listOf(list), a = call(i, lfParts, l.s);
			let s = '';
			for (let k = 0; k < a.length; k++) {
				s += typeof a[k] === 'number' ? l.items[a[k]] : a[k];
			}
			return s;
		},
		formatToParts(list) {
			const i = internal(this, 'ListFormat'), l = listOf(list), a = call(i, lfParts, l.s);
			for (let k = 0; k < a.length; k++) {
				a[k] = typeof a[k] === 'number' ? {type: 'element', value: l.items[a[k]]} : {type: 'literal', value: a[k]};
			}
			return a;
		},
		resolvedOptions() {
			return parse(call(internal(this, 'ListFormat'), optionsOf));
		},
		[toStringTag]: 'Intl.ListFormat'
	});

	// ------------------------------------------------------------ DisplayNames

	function DisplayNames(locales = undefined, options = undefined) {
		requireNew(new.target, 'DisplayNames');
		const args = pushLocales(bare([]), locales);
		new Slots(this, resolve('DisplayNames', dnResolve, pushOptions(args, optionsObject(options), lists.DisplayNames)));
	}
	defineProperty(DisplayNames, 'length', {__proto__: null, value: 2});

	defineConstructor(DisplayNames, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('DisplayNames', locales, options);
		}
	}, {
		of(code) {
			const v = call(internal(this, 'DisplayNames'), dnOf, toString(code));
			return typeof v === 'number' ? undefined : v;
		},
		resolvedOptions() {
			return parse(call(internal(this, 'DisplayNames'), optionsOf));
		},
		[toStringTag]: 'Intl.DisplayNames'
	});

	// ---------------------------------------------------------- DurationFormat

	function DurationFormat(locales = undefined, options = undefined) {
		requireNew(new.target, 'DurationFormat');
		const args = pushLocales(bare([]), locales);
		new Slots(this, resolve('DurationFormat', dfResolve, pushOptions(args, optionsObject(options), lists.DurationFormat)));
	}

	// The fields of a duration in the order ToTemporalPartialDurationRecord
	// reads them, and where each goes in the order Go takes them, largest
	// first.
	const durationFields = ['days', 'hours', 'microseconds', 'milliseconds', 'minutes', 'months', 'nanoseconds', 'seconds',
		'weeks', 'years'];
	const durationOrder = [3, 4, 8, 7, 5, 1, 9, 6, 2, 0];

	// durationOf reads the duration d as dfFormat and dfParts take it: a
	// string as it is, or an object's fields, each ToIntegerIfIntegral,
	// joined by commas, empty where it has none.
	function durationOf(d) {
		if (typeof d === 'string') {
			return [d, false];
		}
		if (d === null || typeof d !== 'object' && typeof d !== 'function') {
			throw new TypeError('Invalid duration: ' + typeof d);
		}
		const fields = bare(['', '', '', '', '', '', '', '', '', '']);
		for (let i = 0; i < durationFields.length; i++) {
			const v = d[durationFields[i]];
			if (v !== undefined) {
				const n = +v;
				if (n % 1 !== 0) {
					throw new RangeError('Invalid ' + durationFields[i] + ': not a finite integer');
				}
				fields[durationOrder[i]] = toString(n);
			}
		}
		let s = fields[0];
		for (let i = 1; i < fields.length; i++) {
			s += ',' + fields[i];
		}
		return [s, true];
	}

	defineConstructor(DurationFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('DurationFormat', locales, options);
		}
	}, {
		format(duration) {
			const i = internal(this, 'DurationFormat'), d = durationOf(duration);
			return call(i, dfFormat, d[0], d[1]);
		},
		formatToParts(duration) {
			const i = internal(this, 'DurationFormat'), d = durationOf(duration);
			return unitParts(call(i, dfParts, d[0], d[1]));
		},
		resolvedOptions() {
			return parse(call(internal(this, 'DurationFormat'), optionsOf));
		},
		[toStringTag]: 'Intl.DurationFormat'
	});

	// ------------------------------------------------------------------ Locale

	// An Intl.Locale holds its tag and what its getters answer, which Go
	// works out when it is made. It is not a formatter: nothing of it is
	// kept in the registry, and its methods go to Go with its tag.
	const localeChecked = ['language', 'script', 'region', 'variants', 'calendar', 'collation', 'firstDayOfWeek'];
	const hourCycles = bare(['h11', 'h12', 'h23', 'h24']), caseFirsts = bare(['upper', 'lower', 'false']);

	function Locale(tag, options = undefined) {
		requireNew(new.target, 'Locale');
		if (tag === null || typeof tag !== 'string' && typeof tag !== 'object' && typeof tag !== 'function') {
			throw new TypeError('First argument to Intl.Locale constructor can\'t be empty or missing');
		}
		let s = localeTag(tag);
		if (s === undefined) {
			s = toString(tag);
		}
		const o = toOptions(options), args = bare([s]);
		localeCheck('tag', s);
		if (o !== noOptions) {
			// V8 checks the subtags as it reads them, and a calendar, a
			// collation and a first day of the week as ICU takes a type: it
			// checks their types in full once it has read them all.
			for (let i = 0; i < localeChecked.length; i++) {
				let v = o[localeChecked[i]];
				if (v !== undefined) {
					v = toString(v);
					localeCheck(localeChecked[i], v);
				}
				args[i + 1] = v;
			}
			args[8] = stringOption(o, 'hourCycle', hourCycles);
			args[9] = stringOption(o, 'caseFirst', caseFirsts);
			let v = o.numeric;
			args[10] = v === undefined ? v : v ? 'true' : 'false';
			v = o.numberingSystem;
			args[11] = v === undefined ? v : toString(v);
		}
		new Slots(this, setPrototypeOf(parse(apply(localeNew, undefined, args)), null));
	}

	function locale(o) {
		return internal(o, 'Locale');
	}

	defineConstructor(Locale, {}, {
		toString() {
			return locale(this).locale;
		},
		maximize() {
			return new Locale(localeInfo(locale(this).locale, 'maximize'));
		},
		minimize() {
			return new Locale(localeInfo(locale(this).locale, 'minimize'));
		},
		get language() {
			return locale(this).language;
		},
		get script() {
			return locale(this).script;
		},
		get region() {
			return locale(this).region;
		},
		get variants() {
			return locale(this).variants;
		},
		get baseName() {
			return locale(this).baseName;
		},
		get calendar() {
			return locale(this).calendar;
		},
		get caseFirst() {
			return locale(this).caseFirst;
		},
		get collation() {
			return locale(this).collation;
		},
		get firstDayOfWeek() {
			return locale(this).firstDayOfWeek;
		},
		get hourCycle() {
			return locale(this).hourCycle;
		},
		get numeric() {
			return locale(this).numeric;
		},
		get numberingSystem() {
			return locale(this).numberingSystem;
		},
		getCalendars() {
			return localeInfo(locale(this).locale, 'calendars');
		},
		getCollations() {
			return localeInfo(locale(this).locale, 'collations');
		},
		getHourCycles() {
			return localeInfo(locale(this).locale, 'hourCycles');
		},
		getNumberingSystems() {
			return localeInfo(locale(this).locale, 'numberingSystems');
		},
		getTimeZones() {
			return localeInfo(locale(this).locale, 'timeZones');
		},
		getTextInfo() {
			const d = localeInfo(locale(this).locale, 'direction');
			return d === undefined ? {} : {direction: d};
		},
		getWeekInfo() {
			const tag = locale(this).locale;
			return {firstDay: localeInfo(tag, 'firstDay'), weekend: localeInfo(tag, 'weekend')};
		},
		[toStringTag]: 'Intl.Locale'
	});

	// ------------------------------------------------------------------- Intl

	const Intl = define({}, {
		getCanonicalLocales(locales) {
			return apply(canonicalLocales, undefined, pushLocales(bare([]), locales));
		},
		supportedValuesOf(key) {
			return valuesOf(toString(key));
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

	// ------------------------------------------- the toLocale* methods ECMA-402
	// replaces. Without them the engine's own fallbacks stay in place, and those
	// ignore both the locale and the timeZone argument: they answer in the host
	// zone whatever zone was asked for, which is a wrong time rather than a
	// missing one.

	// Formatters are memoized for the common shapes because toLocaleString is
	// called per row of a table. Only argument shapes that are safe to use as
	// a cache key take part.
	let cache = setPrototypeOf({}, null), cacheSize = 0;

	function cached(key, make) {
		if (key === null) {
			return make();
		}
		let f = cache[key];
		if (f === undefined) {
			if (cacheSize >= 64) {
				cache = setPrototypeOf({}, null);
				cacheSize = 0;
			}
			f = make();
			cache[key] = f;
			cacheSize++;
		}
		return f;
	}

	function cacheKey(kind, locales, opts) {
		if (opts !== undefined || (locales !== undefined && typeof locales !== 'string')) {
			return null;
		}
		return kind + ' ' + (locales === undefined ? '' : locales);
	}

	function dateFormatter(locales, opts, required, defaults) {
		return cached(cacheKey(required, locales, opts), function () {
			return dateFormat(locales, opts, required, defaults);
		});
	}

	function numberFormatter(locales, opts) {
		return cached(cacheKey('number', locales, opts), function () {
			return numberFormat(locales, opts);
		});
	}

	const date = {
		toLocaleString(locales = undefined, options = undefined) {
			const t = apply(dateValueOf, this, noArgs);
			return t !== t ? 'Invalid Date' : call(dateFormatter(locales, options, 'any', 'all'), dtfFormat, t);
		},
		toLocaleDateString(locales = undefined, options = undefined) {
			const t = apply(dateValueOf, this, noArgs);
			return t !== t ? 'Invalid Date' : call(dateFormatter(locales, options, 'date', 'date'), dtfFormat, t);
		},
		toLocaleTimeString(locales = undefined, options = undefined) {
			const t = apply(dateValueOf, this, noArgs);
			return t !== t ? 'Invalid Date' : call(dateFormatter(locales, options, 'time', 'time'), dtfFormat, t);
		}
	};

	const number = {
		toLocaleString(locales = undefined, options = undefined) {
			return call(numberFormatter(locales, options), nfFormat, apply(numberValueOf, this, noArgs));
		}
	};

	function thisString(s, method) {
		if (s === undefined || s === null) {
			throw new TypeError('String.prototype.' + method + ' called on null or undefined');
		}
		return toString(s);
	}

	const string = {
		localeCompare(that, locales = undefined, options = undefined) {
			const s = thisString(this, 'localeCompare'), t = toString(that);
			const c = cached(cacheKey('collator', locales, options), function () {
				return collator(locales, options);
			});
			return call(c, colCompare, s, t);
		},
		toLocaleUpperCase(locales = undefined) {
			return caseMapped(thisString(this, 'toLocaleUpperCase'), locales, true);
		},
		toLocaleLowerCase(locales = undefined) {
			return caseMapped(thisString(this, 'toLocaleLowerCase'), locales, false);
		}
	};

	// caseTag is the tag the case of a string maps by: the first of the
	// locales, as V8 reads them, which checks no other, or undefined for the
	// default locale.
	function caseTag(locales) {
		const tag = localeTag(locales);
		if (tag !== undefined || typeof locales === 'string' || locales === undefined) {
			return tag !== undefined ? tag : locales;
		}
		if (locales === null) {
			throw new TypeError('Cannot convert null to object');
		}
		const o = Object(locales), n = toLength(o.length);
		for (let i = 0; i < n; i++) {
			if (i in o) {
				const v = o[i];
				if (typeof v !== 'string' && !isObject(v)) {
					throw new TypeError('Language ID should be string or object');
				}
				const t = localeTag(v);
				return t !== undefined ? t : toString(v);
			}
		}
		return undefined;
	}

	// caseMapped is s in upper case or lower case by the rules of the first
	// of locales: Go maps what the rules of Turkish, Azeri, Lithuanian and
	// Greek change, and the engine the rest. The default locale, en-US, has
	// the engine's rules.
	function caseMapped(s, locales, upper) {
		const tag = caseTag(locales);
		const lang = tag === undefined ? '' : caseLanguage(tag);
		if (lang !== '') {
			const r = casePremap(s, lang, upper);
			if (typeof r === 'string') {
				s = r;
			} else if (r !== undefined) {
				s = '';
				for (let i = 0; i < r.length; i++) {
					s += typeof r[i] === 'number' ? fromCharCode(r[i]) : r[i];
				}
			}
		}
		return apply(upper ? toUpperCase : toLowerCase, s, noArgs);
	}

	const api = {
		__proto__: null,
		Intl: Intl,
		dateToLocaleString: date.toLocaleString,
		dateToLocaleDateString: date.toLocaleDateString,
		dateToLocaleTimeString: date.toLocaleTimeString,
		numberToLocaleString: number.toLocaleString,
		stringLocaleCompare: string.localeCompare,
		stringToLocaleUpperCase: string.toLocaleUpperCase,
		stringToLocaleLowerCase: string.toLocaleLowerCase
	};
	if (bigintValueOf !== undefined) {
		api.bigintToLocaleString = {
			toLocaleString(locales = undefined, options = undefined) {
				return call(numberFormatter(locales, options), nfFormat, String(apply(bigintValueOf, this, noArgs)));
			}
		}.toLocaleString;
	}
	return api;
})
