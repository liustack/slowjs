// shape() describes the built-ins of Intl and the toLocale* methods: their
// names, lengths, own properties, attributes and whether they construct.
// TestBuiltinShape runs it on a VM and compares the result with what
// Chromium answers, recorded in shape.json by:
//
//	node record.cjs shape
function shape() {
	'use strict';
	const r = {};

	function isConstructor(f) {
		try {
			Reflect.construct(String, [], f);
			return true;
		} catch (e) {
			return false;
		}
	}

	function attrs(d) {
		if (d === undefined) {
			return 'absent';
		}
		let s = (d.enumerable ? 'e' : '-') + (d.configurable ? 'c' : '-');
		if ('value' in d) {
			s += d.writable ? 'w' : '-';
			s += ' ' + (typeof d.value === 'function' ? 'function' : JSON.stringify(d.value));
		} else {
			s += ' get:' + typeof d.get + ' set:' + typeof d.set;
		}
		return s;
	}

	function fn(path, f) {
		if (typeof f !== 'function') {
			r[path] = typeof f;
			return;
		}
		r[path] = {
			name: attrs(Object.getOwnPropertyDescriptor(f, 'name')),
			length: attrs(Object.getOwnPropertyDescriptor(f, 'length')),
			prototype: attrs(Object.getOwnPropertyDescriptor(f, 'prototype')),
			keys: Reflect.ownKeys(f).map(String).sort().join(' '),
			constructor: isConstructor(f),
			proto: Object.getPrototypeOf(f) === Function.prototype,
			native: /^function [\w$ ]*\(\) \{\s*\[native code\]\s*\}$/.test(Function.prototype.toString.call(f)),
		};
	}

	function prop(path, o, key) {
		const d = Object.getOwnPropertyDescriptor(o, key);
		r[path + ' property'] = attrs(d);
		if (d !== undefined && d.get !== undefined) {
			fn(path + ' getter', d.get);
		}
	}

	// The toLocale* methods first: on a VM they are stand-ins until Intl is
	// touched.
	const methods = [
		['Date.prototype', Date.prototype, ['toLocaleString', 'toLocaleDateString', 'toLocaleTimeString']],
		['Number.prototype', Number.prototype, ['toLocaleString']],
		['BigInt.prototype', BigInt.prototype, ['toLocaleString']],
		['String.prototype', String.prototype, ['localeCompare', 'toLocaleUpperCase', 'toLocaleLowerCase']],
	];
	for (const [name, o, keys] of methods) {
		for (const k of keys) {
			prop('before ' + name + '.' + k, o, k);
			fn('before ' + name + '.' + k, o[k]);
		}
	}

	// On a VM the global Intl is an accessor until the first use installs
	// it, which typeof does.
	r['typeof Intl'] = typeof Intl;
	prop('globalThis.Intl', globalThis, 'Intl');
	r['Intl proto'] = Object.getPrototypeOf(Intl) === Object.prototype;
	prop('Intl[@@toStringTag]', Intl, Symbol.toStringTag);
	prop('Intl.getCanonicalLocales', Intl, 'getCanonicalLocales');
	fn('Intl.getCanonicalLocales', Intl.getCanonicalLocales);
	prop('Intl.supportedValuesOf', Intl, 'supportedValuesOf');
	fn('Intl.supportedValuesOf', Intl.supportedValuesOf);

	const ctors = {
		DateTimeFormat: ['format', 'formatToParts', 'formatRange', 'formatRangeToParts', 'resolvedOptions'],
		NumberFormat: ['format', 'formatToParts', 'resolvedOptions'],
		PluralRules: ['select', 'selectRange', 'resolvedOptions'],
		Collator: ['compare', 'resolvedOptions'],
		Segmenter: ['segment', 'resolvedOptions'],
		RelativeTimeFormat: ['format', 'formatToParts', 'resolvedOptions'],
		ListFormat: ['format', 'formatToParts', 'resolvedOptions'],
		DisplayNames: ['of', 'resolvedOptions'],
		DurationFormat: ['format', 'formatToParts', 'resolvedOptions'],
	};
	const args = {DisplayNames: {type: 'region'}};
	for (const name of Object.keys(ctors)) {
		const C = Intl[name], p = 'Intl.' + name;
		prop(p, Intl, name);
		fn(p, C);
		prop(p + '.prototype', C, 'prototype');
		prop(p + '.supportedLocalesOf', C, 'supportedLocalesOf');
		fn(p + '.supportedLocalesOf', C.supportedLocalesOf);
		const P = C.prototype;
		r[p + '.prototype proto'] = Object.getPrototypeOf(P) === Object.prototype;
		r[p + '.prototype toString'] = Object.prototype.toString.call(P);
		prop(p + '.prototype.constructor', P, 'constructor');
		r[p + '.prototype.constructor is ' + name] = P.constructor === C;
		prop(p + '.prototype[@@toStringTag]', P, Symbol.toStringTag);
		for (const k of ctors[name]) {
			prop(p + '.prototype.' + k, P, k);
			const d = Object.getOwnPropertyDescriptor(P, k);
			if (d.get === undefined) {
				fn(p + '.prototype.' + k, d.value);
			}
		}
		const o = new C('en', args[name]);
		r[p + ' instance toString'] = Object.prototype.toString.call(o);
		r[p + ' instance keys'] = Reflect.ownKeys(o).map(String).join(' ');
		for (const k of ['format', 'compare']) {
			const d = Object.getOwnPropertyDescriptor(P, k);
			if (d !== undefined && d.get !== undefined) {
				fn(p + ' bound ' + k, o[k]);
				r[p + ' bound ' + k + ' same'] = o[k] === o[k];
			}
		}
		try {
			P.resolvedOptions();
			r[p + '.prototype.resolvedOptions()'] = 'no error';
		} catch (e) {
			r[p + '.prototype.resolvedOptions()'] = e.constructor.name;
		}
	}

	// Intl.Locale, which has getters and no supportedLocalesOf, and the
	// order of Intl's keys and its prototype's.
	// V8's v8BreakIterator is its own, no part of ECMA-402.
	r['Intl keys'] = Reflect.ownKeys(Intl).map(String).filter((k) => k !== 'v8BreakIterator').join(' ');
	prop('Intl.Locale', Intl, 'Locale');
	fn('Intl.Locale', Intl.Locale);
	prop('Intl.Locale.prototype', Intl.Locale, 'prototype');
	r['Intl.Locale keys'] = Reflect.ownKeys(Intl.Locale).map(String).sort().join(' ');
	{
		const P = Intl.Locale.prototype, p = 'Intl.Locale.prototype';
		r[p + ' keys'] = Reflect.ownKeys(P).map(String).join(' ');
		r[p + ' proto'] = Object.getPrototypeOf(P) === Object.prototype;
		r[p + ' toString'] = Object.prototype.toString.call(P);
		for (const k of Reflect.ownKeys(P)) {
			const name = p + '.' + String(k);
			prop(name, P, k);
			const d = Object.getOwnPropertyDescriptor(P, k);
			if (typeof d.value === 'function') {
				fn(name, d.value);
			} else if (d.get !== undefined) {
				fn(name + ' get', d.get);
			}
		}
		const o = new Intl.Locale('en');
		r['Intl.Locale instance toString'] = Object.prototype.toString.call(o);
		r['Intl.Locale instance keys'] = Reflect.ownKeys(o).map(String).join(' ');
		try {
			P.language;
			r[p + '.language'] = 'no error';
		} catch (e) {
			r[p + '.language'] = e.constructor.name;
		}
	}

	// What Intl.Segmenter's segment returns, and its iterator.
	const segments = new Intl.Segmenter('en').segment('ab');
	const S = Object.getPrototypeOf(segments);
	r['Segments proto'] = Object.getPrototypeOf(S) === Object.prototype;
	r['Segments prototype keys'] = Reflect.ownKeys(S).map(String).sort().join(' ');
	r['Segments toString'] = Object.prototype.toString.call(segments);
	r['Segments instance keys'] = Reflect.ownKeys(segments).map(String).join(' ');
	prop('Segments.prototype.containing', S, 'containing');
	fn('Segments.prototype.containing', S.containing);
	prop('Segments.prototype[@@iterator]', S, Symbol.iterator);
	fn('Segments.prototype[@@iterator]', S[Symbol.iterator]);
	const iterator = segments[Symbol.iterator]();
	const I = Object.getPrototypeOf(iterator);
	r['Segment Iterator proto'] = Object.getPrototypeOf(I) === Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]()));
	r['Segment Iterator prototype keys'] = Reflect.ownKeys(I).map(String).sort().join(' ');
	r['Segment Iterator instance keys'] = Reflect.ownKeys(iterator).map(String).join(' ');
	prop('Segment Iterator.prototype.next', I, 'next');
	fn('Segment Iterator.prototype.next', I.next);
	prop('Segment Iterator.prototype[@@toStringTag]', I, Symbol.toStringTag);
	const data = iterator.next().value;
	r['segment data'] = Reflect.ownKeys(data).map((k) => k + ':' + attrs(Object.getOwnPropertyDescriptor(data, k))).join(', ');
	r['segment data proto'] = Object.getPrototypeOf(data) === Object.prototype;
	const word = new Intl.Segmenter('en', {granularity: 'word'}).segment('ab').containing(0);
	r['word segment data'] = Reflect.ownKeys(word).map((k) => k + ':' + attrs(Object.getOwnPropertyDescriptor(word, k))).join(', ');
	const step = iterator.next();
	r['iterator result'] = Reflect.ownKeys(step).map((k) => k + ':' + attrs(Object.getOwnPropertyDescriptor(step, k)).replace(/ .*/, '')).join(', ');

	// The objects the other methods return.
	const kinds = {
		'Collator resolvedOptions': new Intl.Collator('en').resolvedOptions(),
		'RelativeTimeFormat formatToParts': new Intl.RelativeTimeFormat('en').formatToParts(1, 'day')[1],
		'RelativeTimeFormat formatToParts literal': new Intl.RelativeTimeFormat('en').formatToParts(1, 'day')[0],
		'ListFormat formatToParts': new Intl.ListFormat('en').formatToParts(['a'])[0],
		'DisplayNames resolvedOptions': new Intl.DisplayNames('en', {type: 'language'}).resolvedOptions(),
		'DisplayNames resolvedOptions region': new Intl.DisplayNames('en', {type: 'region'}).resolvedOptions(),
		'DurationFormat resolvedOptions': new Intl.DurationFormat('en', {fractionalDigits: 2}).resolvedOptions(),
		'DurationFormat formatToParts': new Intl.DurationFormat('en').formatToParts({hours: 1, minutes: 2})[0],
		'DurationFormat formatToParts literal': new Intl.DurationFormat('en').formatToParts({hours: 1, minutes: 2})[3],
		'Locale getTextInfo': new Intl.Locale('ar').getTextInfo(),
		'Locale getWeekInfo': new Intl.Locale('en-US').getWeekInfo(),
	};
	for (const k of Object.keys(kinds)) {
		const o = kinds[k];
		r[k] = Reflect.ownKeys(o).map((p) => p + ':' + attrs(Object.getOwnPropertyDescriptor(o, p)).replace(/ .*/, '')).join(', ');
		r[k + ' proto'] = Object.getPrototypeOf(o) === Object.prototype;
	}

	for (const [name, o, keys] of methods) {
		for (const k of keys) {
			prop(name + '.' + k, o, k);
			fn(name + '.' + k, o[k]);
		}
	}
	return JSON.stringify(r, null, '\t');
}
