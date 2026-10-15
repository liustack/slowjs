// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// The javascript half of modernc.org/quickjs/intl. It owns the ECMA-402
// surface - the constructors, their prototypes and the option plumbing the
// spec makes observable - and leaves every formatting decision to the Go host
// functions it is called with. See call() below for what crosses between them.

(function (dtfResolve, dtfFormat, nfResolve, nfFormat, canonicalLocales, prResolve, prSelect) {
	'use strict';

	const g = globalThis;
	const internals = new WeakMap();

	// A host function answers with its result as a string. An error instead
	// answers with '\x01' + the error constructor's name + '\x01' + the
	// message, a shape no formatted result can have. Nothing is wrapped in json
	// so that format(), the call a page makes per row of a table, does not pay
	// for a JSON.parse of its own answer.
	const errorMark = '\u0001';

	function call(fn, args) {
		const r = fn.apply(null, args);
		if (r.charCodeAt(0) === 1) {
			const i = r.indexOf(errorMark, 1);
			const ctor = g[r.slice(1, i)];
			throw new (typeof ctor === 'function' ? ctor : Error)(r.slice(i + 1));
		}
		return r;
	}

	function callJSON(fn, args) {
		return JSON.parse(call(fn, args));
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
		if (locales === undefined) {
			return [];
		}
		if (typeof locales === 'string') {
			return [locales];
		}
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

	// publicOptions turns the host's resolved state into a resolvedOptions()
	// result, dropping the underscore prefixed keys the host keeps for itself.
	function publicOptions(json) {
		const o = JSON.parse(json);
		const keys = Object.keys(o);
		for (let i = 0; i < keys.length; i++) {
			if (keys[i].charAt(0) === '_') {
				delete o[keys[i]];
			}
		}
		return o;
	}

	function options(o) {
		if (o === null) {
			throw new TypeError('Options must be an object');
		}
		return o === undefined ? {} : Object(o);
	}

	function supportedLocalesOf(service) {
		return function (locales) {
			return callJSON(canonicalLocales, [JSON.stringify(localeList(locales)), service]);
		};
	}

	// ---------------------------------------------------------- DateTimeFormat

	const dateTimeKeys = [
		'localeMatcher', 'calendar', 'numberingSystem', 'timeZone', 'hourCycle', 'weekday', 'era', 'year',
		'month', 'day', 'dayPeriod', 'hour', 'minute', 'second', 'timeZoneName', 'formatMatcher',
		'dateStyle', 'timeStyle'
	];

	// readDateTimeOptions copies the options a DateTimeFormat reads. required
	// and defaults are ECMA-402's ToDateTimeOptions arguments: they are what
	// make toLocaleDateString a date and toLocaleTimeString a time. The host
	// applies them, because the defaults it adds depend on the locale's hour
	// cycle.
	function readDateTimeOptions(o, required, defaults) {
		o = o === undefined ? {} : options(o);
		const r = {_required: required, _defaults: defaults};
		for (let i = 0; i < dateTimeKeys.length; i++) {
			const v = o[dateTimeKeys[i]];
			if (v !== undefined) {
				r[dateTimeKeys[i]] = String(v);
			}
		}
		if (o.hour12 !== undefined) {
			r.hour12 = Boolean(o.hour12);
		}
		if (o.fractionalSecondDigits !== undefined) {
			r.fractionalSecondDigits = Number(o.fractionalSecondDigits);
		}
		return r;
	}

	function initDateTimeFormat(f, locales, opts, required, defaults) {
		internals.set(f, {
			kind: 'DateTimeFormat',
			resolved: call(dtfResolve, [JSON.stringify({locales: localeList(locales), options: readDateTimeOptions(opts, required, defaults)})])
		});
		return f;
	}

	function DateTimeFormat(locales, opts) {
		if (!(this instanceof DateTimeFormat)) {
			return new DateTimeFormat(locales, opts);
		}
		initDateTimeFormat(this, locales, opts, 'any', 'date');
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
					return call(dtfFormat, [i.resolved, timeValue(date), false]);
				};
			}
			return i.bound;
		}
	});

	defineMethod(DateTimeFormat.prototype, 'formatToParts', function (date) {
		return callJSON(dtfFormat, [internal(this, 'DateTimeFormat').resolved, timeValue(date), true]);
	});

	defineMethod(DateTimeFormat.prototype, 'resolvedOptions', function () {
		return publicOptions(internal(this, 'DateTimeFormat').resolved);
	});

	defineMethod(DateTimeFormat, 'supportedLocalesOf', supportedLocalesOf('DateTimeFormat'));
	defineTag(DateTimeFormat.prototype, 'Intl.DateTimeFormat');

	// ------------------------------------------------------------ NumberFormat

	const numberKeys = [
		'localeMatcher', 'style', 'currency', 'currencyDisplay', 'currencySign', 'unit', 'unitDisplay',
		'numberingSystem', 'notation', 'compactDisplay', 'signDisplay', 'roundingMode', 'trailingZeroDisplay'
	];

	const numberDigitKeys = [
		'minimumIntegerDigits', 'minimumFractionDigits', 'maximumFractionDigits',
		'minimumSignificantDigits', 'maximumSignificantDigits'
	];

	function readNumberOptions(o) {
		o = options(o);
		const r = {};
		let i;
		for (i = 0; i < numberKeys.length; i++) {
			const v = o[numberKeys[i]];
			if (v !== undefined) {
				r[numberKeys[i]] = String(v);
			}
		}
		for (i = 0; i < numberDigitKeys.length; i++) {
			const v = o[numberDigitKeys[i]];
			if (v !== undefined) {
				r[numberDigitKeys[i]] = Number(v);
			}
		}
		if (o.useGrouping !== undefined) {
			r.useGrouping = typeof o.useGrouping === 'boolean' ? o.useGrouping : String(o.useGrouping);
		}
		return r;
	}

	function NumberFormat(locales, opts) {
		if (!(this instanceof NumberFormat)) {
			return new NumberFormat(locales, opts);
		}
		internals.set(this, {
			kind: 'NumberFormat',
			resolved: call(nfResolve, [JSON.stringify({locales: localeList(locales), options: readNumberOptions(opts)})])
		});
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
					return call(nfFormat, [i.resolved, numberValue(value), false]);
				};
			}
			return i.bound;
		}
	});

	defineMethod(NumberFormat.prototype, 'formatToParts', function (value) {
		return callJSON(nfFormat, [internal(this, 'NumberFormat').resolved, numberValue(value), true]);
	});

	defineMethod(NumberFormat.prototype, 'resolvedOptions', function () {
		return publicOptions(internal(this, 'NumberFormat').resolved);
	});

	defineMethod(NumberFormat, 'supportedLocalesOf', supportedLocalesOf('NumberFormat'));
	defineTag(NumberFormat.prototype, 'Intl.NumberFormat');

	// ------------------------------------------------------------- PluralRules

	const pluralKeys = ['localeMatcher', 'type', 'notation', 'roundingMode', 'roundingPriority', 'trailingZeroDisplay'];

	function readPluralOptions(o) {
		o = options(o);
		const r = {};
		let i;
		for (i = 0; i < pluralKeys.length; i++) {
			const v = o[pluralKeys[i]];
			if (v !== undefined) {
				r[pluralKeys[i]] = String(v);
			}
		}
		for (i = 0; i < numberDigitKeys.length; i++) {
			const v = o[numberDigitKeys[i]];
			if (v !== undefined) {
				r[numberDigitKeys[i]] = Number(v);
			}
		}
		return r;
	}

	function PluralRules(locales, opts) {
		if (new.target === undefined) {
			throw new TypeError("Constructor Intl.PluralRules requires 'new'");
		}
		internals.set(this, {
			kind: 'PluralRules',
			resolved: call(prResolve, [JSON.stringify({locales: localeList(locales), options: readPluralOptions(opts)})])
		});
	}

	defineMethod(PluralRules.prototype, 'select', function (value) {
		return call(prSelect, [internal(this, 'PluralRules').resolved, Number(value)]);
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
		return call(prSelect, [i.resolved, x, y]);
	});

	defineMethod(PluralRules.prototype, 'resolvedOptions', function () {
		return publicOptions(internal(this, 'PluralRules').resolved);
	});

	defineMethod(PluralRules, 'supportedLocalesOf', supportedLocalesOf('PluralRules'));
	defineTag(PluralRules.prototype, 'Intl.PluralRules');

	// ------------------------------------------------------------------- Intl

	const Intl = {};
	defineTag(Intl, 'Intl');
	// How the host recognizes its own installation, so that a second Register
	// on the same VM can say so instead of failing on a duplicate host function
	// name. Keyed by the import path, invisible to enumeration and to JSON.
	Object.defineProperty(Intl, Symbol.for('modernc.org/quickjs/intl'), {value: true});
	defineMethod(Intl, 'DateTimeFormat', DateTimeFormat);
	defineMethod(Intl, 'NumberFormat', NumberFormat);
	defineMethod(Intl, 'PluralRules', PluralRules);
	defineMethod(Intl, 'getCanonicalLocales', function (locales) {
		return callJSON(canonicalLocales, [JSON.stringify(localeList(locales))]);
	});
	Object.defineProperty(g, 'Intl', {value: Intl, writable: true, enumerable: false, configurable: true});

	// ------------------------------------------- the toLocale* methods ECMA-402
	// replaces. Without them the engine's own fallbacks stay in place, and those
	// ignore both the locale and the timeZone argument: they answer in the host
	// zone whatever zone was asked for, which is a wrong time rather than a
	// missing one.

	const dateValueOf = Date.prototype.valueOf;
	const numberValueOf = Number.prototype.valueOf;

	// Formatters are memoized for the common shapes because toLocaleString is
	// called per row of a table and constructing one costs a host call. Only
	// argument shapes that are safe to use as a cache key take part.
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
		return kind + ' ' + (locales === undefined ? '' : locales);
	}

	function dateFormatter(locales, opts, required, defaults) {
		return cached(cacheKey(required, locales, opts), function () {
			return initDateTimeFormat(Object.create(DateTimeFormat.prototype), locales, opts, required, defaults);
		});
	}

	defineMethod(Date.prototype, 'toLocaleString', function (locales, opts) {
		const t = dateValueOf.call(this);
		return t !== t ? 'Invalid Date' : dateFormatter(locales, opts, 'any', 'all').format(t);
	});

	defineMethod(Date.prototype, 'toLocaleDateString', function (locales, opts) {
		const t = dateValueOf.call(this);
		return t !== t ? 'Invalid Date' : dateFormatter(locales, opts, 'date', 'date').format(t);
	});

	defineMethod(Date.prototype, 'toLocaleTimeString', function (locales, opts) {
		const t = dateValueOf.call(this);
		return t !== t ? 'Invalid Date' : dateFormatter(locales, opts, 'time', 'time').format(t);
	});

	defineMethod(Number.prototype, 'toLocaleString', function (locales, opts) {
		const v = numberValueOf.call(this);
		return cached(cacheKey('number', locales, opts), function () {
			return new NumberFormat(locales, opts);
		}).format(v);
	});

	if (typeof BigInt === 'function') {
		const bigintValueOf = BigInt.prototype.valueOf;
		defineMethod(BigInt.prototype, 'toLocaleString', function (locales, opts) {
			const v = bigintValueOf.call(this);
			return cached(cacheKey('number', locales, opts), function () {
				return new NumberFormat(locales, opts);
			}).format(v);
		});
	}
})(
	globalThis.__quickjs_intl_dtfResolve__,
	globalThis.__quickjs_intl_dtfFormat__,
	globalThis.__quickjs_intl_nfResolve__,
	globalThis.__quickjs_intl_nfFormat__,
	globalThis.__quickjs_intl_canonicalLocales__,
	globalThis.__quickjs_intl_prResolve__,
	globalThis.__quickjs_intl_prSelect__
);

delete globalThis.__quickjs_intl_dtfResolve__;
delete globalThis.__quickjs_intl_dtfFormat__;
delete globalThis.__quickjs_intl_nfResolve__;
delete globalThis.__quickjs_intl_nfFormat__;
delete globalThis.__quickjs_intl_canonicalLocales__;
delete globalThis.__quickjs_intl_prResolve__;
delete globalThis.__quickjs_intl_prSelect__;
