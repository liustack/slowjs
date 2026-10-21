// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// The javascript half of modernc.org/quickjs/intl. It owns the ECMA-402
// surface - the constructors, their prototypes and the option plumbing the
// spec makes observable - and leaves every formatting decision to the Go
// Builtins it is called with (see bridge.go). It evaluates to a function of
// the VM's default time zone, the json of each constructor's options and the
// Builtins, which returns what stub.js puts in place.

(function (zone, optionLists,
	nfResolve, nfFormat, nfParts, dtfResolve, dtfFormat, dtfParts, prResolve, prSelect, prSelectRange,
	optionsOf, canonicalLocales, supportedLocales) {
	'use strict';

	const internals = new WeakMap();
	const lists = JSON.parse(optionLists);

	// A formatter is a record: the Builtin that resolves it, the arguments it
	// was resolved with and the id of what that resolved to, which every
	// other call passes on. A Builtin answers undefined for an id the
	// process has since dropped (see registry.go): call then resolves the
	// same arguments again and repeats the call. Other VMs making formatters
	// fast enough can drop the new one too before the call gets to it, so
	// call tries a few times, then throws: it never returns undefined.
	function resolve(kind, fn, args) {
		return {kind: kind, fn: fn, args: args, id: fn.apply(undefined, args), bound: undefined};
	}

	const tries = 8;

	function call(r, fn, x, y) {
		for (let i = 0; i < tries; i++) {
			const v = fn(r.id, x, y);
			if (v !== undefined) {
				return v;
			}
			r.id = r.fn.apply(undefined, r.args);
		}
		throw new Error('Intl.' + r.kind + ': other code made formatters too fast to keep this one');
	}

	function internal(o, kind) {
		const i = internals.get(o);
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
		const keys = Reflect.ownKeys(props);
		for (let i = 0; i < keys.length; i++) {
			const d = Object.getOwnPropertyDescriptor(props, keys[i]);
			d.enumerable = false;
			if ('value' in d) {
				d.writable = typeof d.value === 'function';
			}
			Object.defineProperty(o, keys[i], d);
		}
		return o;
	}

	// defineConstructor finishes C, a constructor written as a function declaration
	// whose parameters all have defaults, so that its length is 0: its
	// prototype property becomes read only, statics go on C and methods on
	// its prototype, both through define.
	function defineConstructor(C, statics, methods) {
		Object.defineProperty(C, 'prototype', {writable: false});
		define(C, statics);
		define(C.prototype, methods);
		return C;
	}

	// localeList implements CanonicalizeLocaleList's argument handling. The tags
	// themselves are canonicalized by the host.
	function localeList(locales) {
		const o = Object(locales), n = o.length >>> 0, r = [];
		for (let i = 0; i < n; i++) {
			if (i in o) {
				const v = o[i];
				if (typeof v !== 'string' && (v === null || typeof v !== 'object')) {
					throw new TypeError('Language ID should be string or object');
				}
				r.push(String(v));
			}
		}
		return r;
	}

	// pushLocales appends the two locale arguments of a Builtin to args: a
	// single tag as itself, a list as one string of the tags each preceded by
	// its length and a colon.
	function pushLocales(args, locales) {
		if (locales === undefined) {
			args.push(undefined, undefined);
		} else if (typeof locales === 'string') {
			args.push(locales, undefined);
		} else {
			const l = localeList(locales);
			let s = '';
			for (let i = 0; i < l.length; i++) {
				s += l[i].length + ':' + l[i];
			}
			args.push(undefined, s);
		}
		return args;
	}

	function toOptions(o) {
		if (o === null) {
			throw new TypeError('Options must be an object');
		}
		return o === undefined ? {} : Object(o);
	}

	// pushOptions appends the options of o in list, from optionLists, to
	// args, converted as their kind says.
	function pushOptions(args, o, list) {
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
			args.push(v);
		}
		return args;
	}

	function parse(json) {
		return JSON.parse(json);
	}

	function supportedLocalesOf(service, locales) {
		return parse(supportedLocales.apply(undefined, pushLocales([service], locales)));
	}

	// ---------------------------------------------------------- DateTimeFormat

	// dateFormat resolves a DateTimeFormat. required and defaults are
	// ECMA-402's ToDateTimeOptions arguments: they are what make
	// toLocaleDateString a date and toLocaleTimeString a time. The host
	// applies them, because the defaults it adds depend on the locale's hour
	// cycle.
	function dateFormat(locales, opts, required, defaults) {
		const args = pushLocales([zone, required, defaults], locales);
		return resolve('DateTimeFormat', dtfResolve, pushOptions(args, toOptions(opts), lists.DateTimeFormat));
	}

	function DateTimeFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			return new DateTimeFormat(locales, options);
		}
		internals.set(this, dateFormat(locales, options, 'any', 'date'));
	}

	function timeValue(date) {
		return date === undefined ? Date.now() : Number(date);
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
		[Symbol.toStringTag]: 'Intl.DateTimeFormat'
	});

	// ------------------------------------------------------------ NumberFormat

	function numberFormat(locales, opts) {
		const args = pushLocales([], locales);
		return resolve('NumberFormat', nfResolve, pushOptions(args, toOptions(opts), lists.NumberFormat));
	}

	function NumberFormat(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			return new NumberFormat(locales, options);
		}
		internals.set(this, numberFormat(locales, options));
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
		[Symbol.toStringTag]: 'Intl.NumberFormat'
	});

	// ------------------------------------------------------------- PluralRules

	function PluralRules(locales = undefined, options = undefined) {
		if (new.target === undefined) {
			throw new TypeError("Constructor Intl.PluralRules requires 'new'");
		}
		const args = pushLocales([], locales);
		internals.set(this, resolve('PluralRules', prResolve, pushOptions(args, toOptions(options), lists.PluralRules)));
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
		[Symbol.toStringTag]: 'Intl.PluralRules'
	});

	// ------------------------------------------------------------------- Intl

	const Intl = define({}, {
		DateTimeFormat: DateTimeFormat,
		NumberFormat: NumberFormat,
		PluralRules: PluralRules,
		getCanonicalLocales(locales) {
			return parse(canonicalLocales.apply(undefined, pushLocales([], locales)));
		},
		[Symbol.toStringTag]: 'Intl'
	});

	// ------------------------------------------- the toLocale* methods ECMA-402
	// replaces. Without them the engine's own fallbacks stay in place, and those
	// ignore both the locale and the timeZone argument: they answer in the host
	// zone whatever zone was asked for, which is a wrong time rather than a
	// missing one.

	const dateValueOf = Date.prototype.valueOf;
	const numberValueOf = Number.prototype.valueOf;

	// Formatters are memoized for the common shapes because toLocaleString is
	// called per row of a table. Only argument shapes that are safe to use as
	// a cache key take part.
	const cache = new Map();

	function cached(key, make) {
		if (key === null) {
			return make();
		}
		let f = cache.get(key);
		if (f === undefined) {
			if (cache.size >= 64) {
				cache.clear();
			}
			f = make();
			cache.set(key, f);
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
			const t = dateValueOf.call(this);
			return t !== t ? 'Invalid Date' : call(dateFormatter(locales, options, 'any', 'all'), dtfFormat, t);
		},
		toLocaleDateString(locales = undefined, options = undefined) {
			const t = dateValueOf.call(this);
			return t !== t ? 'Invalid Date' : call(dateFormatter(locales, options, 'date', 'date'), dtfFormat, t);
		},
		toLocaleTimeString(locales = undefined, options = undefined) {
			const t = dateValueOf.call(this);
			return t !== t ? 'Invalid Date' : call(dateFormatter(locales, options, 'time', 'time'), dtfFormat, t);
		}
	};

	const number = {
		toLocaleString(locales = undefined, options = undefined) {
			return call(numberFormatter(locales, options), nfFormat, numberValueOf.call(this));
		}
	};

	const api = {
		Intl: Intl,
		dateToLocaleString: date.toLocaleString,
		dateToLocaleDateString: date.toLocaleDateString,
		dateToLocaleTimeString: date.toLocaleTimeString,
		numberToLocaleString: number.toLocaleString
	};
	if (typeof BigInt === 'function') {
		const bigintValueOf = BigInt.prototype.valueOf;
		api.bigintToLocaleString = {
			toLocaleString(locales = undefined, options = undefined) {
				return call(numberFormatter(locales, options), nfFormat, String(bigintValueOf.call(this)));
			}
		}.toLocaleString;
	}
	return api;
})
