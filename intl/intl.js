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
	optionsOf, canonicalLocales, supportedLocales) {
	'use strict';

	const {
		apply, defineProperty, getOwnPropertyDescriptor, ownKeys, setPrototypeOf, parse,
		Object, String, Number, Boolean, Error, TypeError, RangeError, toStringTag,
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

	function internal(o, kind) {
		const i = Slots.of(o);
		if (i === undefined || i.kind !== kind) {
			throw new TypeError('Intl.' + kind + '.prototype method called on an incompatible receiver');
		}
		return i;
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
		const o = Object(locales), n = o.length >>> 0, r = bare([]);
		for (let i = 0; i < n; i++) {
			if (i in o) {
				const v = o[i];
				if (typeof v !== 'string' && (v === null || typeof v !== 'object')) {
					throw new TypeError('Language ID should be string or object');
				}
				r[r.length] = String(v);
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

	// pushOptions appends the options of o in list, from optionLists, to
	// args, converted as their kind says.
	function pushOptions(args, o, list) {
		const n = args.length;
		for (let i = 0; i < list.length; i++) {
			let v = o[list[i][0]];
			if (v !== undefined) {
				switch (list[i][1]) {
				case 's':
					v = String(v);
					break;
				case 'n':
					v = Number(v);
					break;
				case 'b':
					v = Boolean(v);
					break;
				default:
					if (typeof v !== 'boolean') {
						v = String(v);
					}
				}
			}
			args[n + i] = v;
		}
		return args;
	}

	function supportedLocalesOf(service, locales) {
		return parse(apply(supportedLocales, undefined, pushLocales(bare([service]), locales)));
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
			return new DateTimeFormat(locales, options);
		}
		new Slots(this, dateFormat(locales, options, 'any', 'date'));
	}

	function timeValue(date) {
		return date === undefined ? dateNow() : Number(date);
	}

	defineConstructor(DateTimeFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('DateTimeFormat', locales);
		}
	}, {
		get format() {
			// The spec hands out a bound function so that [d1, d2].map(f.format)
			// keeps working. Code in the wild does exactly that.
			const i = internal(this, 'DateTimeFormat');
			if (i.bound === undefined) {
				i.bound = (date) => call(i, dtfFormat, timeValue(date));
			}
			return i.bound;
		},
		formatToParts(date) {
			return parse(call(internal(this, 'DateTimeFormat'), dtfParts, timeValue(date)));
		},
		resolvedOptions() {
			return parse(call(internal(this, 'DateTimeFormat'), optionsOf));
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
			return new NumberFormat(locales, options);
		}
		new Slots(this, numberFormat(locales, options));
	}

	// numberValue keeps a BigInt and a numeric string exact by passing them on
	// as a string: the host parses decimal digits, so nothing round trips
	// through a double that did not start as one.
	function numberValue(v) {
		const t = typeof v;
		if (t === 'bigint' || t === 'string') {
			return String(v);
		}
		return Number(v);
	}

	defineConstructor(NumberFormat, {
		supportedLocalesOf(locales, options = undefined) {
			return supportedLocalesOf('NumberFormat', locales);
		}
	}, {
		get format() {
			const i = internal(this, 'NumberFormat');
			if (i.bound === undefined) {
				i.bound = (value) => call(i, nfFormat, numberValue(value));
			}
			return i.bound;
		},
		formatToParts(value) {
			return parse(call(internal(this, 'NumberFormat'), nfParts, numberValue(value)));
		},
		resolvedOptions() {
			return parse(call(internal(this, 'NumberFormat'), optionsOf));
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
			return supportedLocalesOf('PluralRules', locales);
		}
	}, {
		select(value) {
			return call(internal(this, 'PluralRules'), prSelect, Number(value));
		},
		selectRange(start, end) {
			const i = internal(this, 'PluralRules');
			if (start === undefined || end === undefined) {
				throw new TypeError('start or end is undefined');
			}
			const x = Number(start), y = Number(end);
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

	// ------------------------------------------------------------------- Intl

	const Intl = define({}, {
		DateTimeFormat: DateTimeFormat,
		NumberFormat: NumberFormat,
		PluralRules: PluralRules,
		getCanonicalLocales(locales) {
			return parse(apply(canonicalLocales, undefined, pushLocales(bare([]), locales)));
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

	const api = {
		__proto__: null,
		Intl: Intl,
		dateToLocaleString: date.toLocaleString,
		dateToLocaleDateString: date.toLocaleDateString,
		dateToLocaleTimeString: date.toLocaleTimeString,
		numberToLocaleString: number.toLocaleString
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
