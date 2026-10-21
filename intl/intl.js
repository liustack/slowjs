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
	colResolve, colCompare) {
	'use strict';

	const {
		apply, defineProperty, getOwnPropertyDescriptor, ownKeys, setPrototypeOf, parse,
		Object, String, Number, Error, TypeError, RangeError, Symbol, toStringTag, hasInstance,
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

	// localeList implements CanonicalizeLocaleList's argument handling. The tags
	// themselves are canonicalized by the host.
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
				r[r.length] = toString(v);
			}
		}
		return r;
	}

	// pushLocales appends the two locale arguments of a Builtin to args: a
	// single tag as itself, a list as one string of the tags each preceded by
	// its length and a colon.
	function pushLocales(args, locales) {
		const n = args.length;
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

	// ------------------------------------------------------------------- Intl

	const Intl = define({}, {
		DateTimeFormat: DateTimeFormat,
		NumberFormat: NumberFormat,
		Collator: Collator,
		PluralRules: PluralRules,
		getCanonicalLocales(locales) {
			return apply(canonicalLocales, undefined, pushLocales(bare([]), locales));
		},
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
		}
	};

	const api = {
		__proto__: null,
		Intl: Intl,
		dateToLocaleString: date.toLocaleString,
		dateToLocaleDateString: date.toLocaleDateString,
		dateToLocaleTimeString: date.toLocaleTimeString,
		numberToLocaleString: number.toLocaleString,
		stringLocaleCompare: string.localeCompare
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
