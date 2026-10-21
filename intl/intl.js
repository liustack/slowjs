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

	const g = globalThis;
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

	function defineMethod(o, name, value) {
		Object.defineProperty(o, name, {value: value, writable: true, enumerable: false, configurable: true});
	}

	function defineTag(o, tag) {
		if (typeof Symbol === 'function' && Symbol.toStringTag !== undefined) {
			Object.defineProperty(o, Symbol.toStringTag, {value: tag, writable: false, enumerable: false, configurable: true});
		}
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

	function options(o) {
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

	function supportedLocalesOf(service) {
		return function (locales) {
			return parse(supportedLocales.apply(undefined, pushLocales([service], locales)));
		};
	}

	// ---------------------------------------------------------- DateTimeFormat

	// dateFormat resolves a DateTimeFormat. required and defaults are
	// ECMA-402's ToDateTimeOptions arguments: they are what make
	// toLocaleDateString a date and toLocaleTimeString a time. The host
	// applies them, because the defaults it adds depend on the locale's hour
	// cycle.
	function dateFormat(locales, opts, required, defaults) {
		const args = pushLocales([zone, required, defaults], locales);
		return resolve('DateTimeFormat', dtfResolve, pushOptions(args, options(opts), lists.DateTimeFormat));
	}

	function DateTimeFormat(locales, opts) {
		if (!(this instanceof DateTimeFormat)) {
			return new DateTimeFormat(locales, opts);
		}
		internals.set(this, dateFormat(locales, opts, 'any', 'date'));
	}

	function timeValue(date) {
		return date === undefined ? Date.now() : Number(date);
	}

	Object.defineProperty(DateTimeFormat.prototype, 'format', {
		enumerable: false,
		configurable: true,
		get: function () {
			// The spec hands out a bound function so that [d1, d2].map(f.format)
			// keeps working. Code in the wild does exactly that.
			const i = internal(this, 'DateTimeFormat');
			if (i.bound === undefined) {
				i.bound = function (date) {
					return call(i, dtfFormat, timeValue(date));
				};
			}
			return i.bound;
		}
	});

	defineMethod(DateTimeFormat.prototype, 'formatToParts', function (date) {
		return parse(call(internal(this, 'DateTimeFormat'), dtfParts, timeValue(date)));
	});

	defineMethod(DateTimeFormat.prototype, 'resolvedOptions', function () {
		return parse(call(internal(this, 'DateTimeFormat'), optionsOf));
	});

	defineMethod(DateTimeFormat, 'supportedLocalesOf', supportedLocalesOf('DateTimeFormat'));
	defineTag(DateTimeFormat.prototype, 'Intl.DateTimeFormat');

	// ------------------------------------------------------------ NumberFormat

	function numberFormat(locales, opts) {
		const args = pushLocales([], locales);
		return resolve('NumberFormat', nfResolve, pushOptions(args, options(opts), lists.NumberFormat));
	}

	function NumberFormat(locales, opts) {
		if (!(this instanceof NumberFormat)) {
			return new NumberFormat(locales, opts);
		}
		internals.set(this, numberFormat(locales, opts));
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

	Object.defineProperty(NumberFormat.prototype, 'format', {
		enumerable: false,
		configurable: true,
		get: function () {
			const i = internal(this, 'NumberFormat');
			if (i.bound === undefined) {
				i.bound = function (value) {
					return call(i, nfFormat, numberValue(value));
				};
			}
			return i.bound;
		}
	});

	defineMethod(NumberFormat.prototype, 'formatToParts', function (value) {
		return parse(call(internal(this, 'NumberFormat'), nfParts, numberValue(value)));
	});

	defineMethod(NumberFormat.prototype, 'resolvedOptions', function () {
		return parse(call(internal(this, 'NumberFormat'), optionsOf));
	});

	defineMethod(NumberFormat, 'supportedLocalesOf', supportedLocalesOf('NumberFormat'));
	defineTag(NumberFormat.prototype, 'Intl.NumberFormat');

	// ------------------------------------------------------------- PluralRules

	function PluralRules(locales, opts) {
		if (new.target === undefined) {
			throw new TypeError("Constructor Intl.PluralRules requires 'new'");
		}
		const args = pushLocales([], locales);
		internals.set(this, resolve('PluralRules', prResolve, pushOptions(args, options(opts), lists.PluralRules)));
	}

	defineMethod(PluralRules.prototype, 'select', function (value) {
		return call(internal(this, 'PluralRules'), prSelect, Number(value));
	});

	defineMethod(PluralRules.prototype, 'selectRange', function (start, end) {
		const i = internal(this, 'PluralRules');
		if (start === undefined || end === undefined) {
			throw new TypeError('start or end is undefined');
		}
		const x = Number(start), y = Number(end);
		if (x !== x || y !== y) {
			throw new RangeError('start or end is NaN');
		}
		return call(i, prSelectRange, x, y);
	});

	defineMethod(PluralRules.prototype, 'resolvedOptions', function () {
		return parse(call(internal(this, 'PluralRules'), optionsOf));
	});

	defineMethod(PluralRules, 'supportedLocalesOf', supportedLocalesOf('PluralRules'));
	defineTag(PluralRules.prototype, 'Intl.PluralRules');

	// ------------------------------------------------------------------- Intl

	const Intl = {};
	defineTag(Intl, 'Intl');
	defineMethod(Intl, 'DateTimeFormat', DateTimeFormat);
	defineMethod(Intl, 'NumberFormat', NumberFormat);
	defineMethod(Intl, 'PluralRules', PluralRules);
	defineMethod(Intl, 'getCanonicalLocales', function (locales) {
		return parse(canonicalLocales.apply(undefined, pushLocales([], locales)));
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

	const api = {Intl: Intl};

	api.dateToLocaleString = function toLocaleString(locales, opts) {
		const t = dateValueOf.call(this);
		return t !== t ? 'Invalid Date' : call(dateFormatter(locales, opts, 'any', 'all'), dtfFormat, t);
	};

	api.dateToLocaleDateString = function toLocaleDateString(locales, opts) {
		const t = dateValueOf.call(this);
		return t !== t ? 'Invalid Date' : call(dateFormatter(locales, opts, 'date', 'date'), dtfFormat, t);
	};

	api.dateToLocaleTimeString = function toLocaleTimeString(locales, opts) {
		const t = dateValueOf.call(this);
		return t !== t ? 'Invalid Date' : call(dateFormatter(locales, opts, 'time', 'time'), dtfFormat, t);
	};

	api.numberToLocaleString = function toLocaleString(locales, opts) {
		const v = numberValueOf.call(this);
		return call(numberFormatter(locales, opts), nfFormat, v);
	};

	if (typeof BigInt === 'function') {
		const bigintValueOf = BigInt.prototype.valueOf;
		api.bigintToLocaleString = function toLocaleString(locales, opts) {
			const v = bigintValueOf.call(this);
			return call(numberFormatter(locales, opts), nfFormat, String(v));
		};
	}
	return api;
})
