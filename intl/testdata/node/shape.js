// shape() describes the built-ins of Intl and the toLocale* methods: their
// names, lengths, own properties, attributes and whether they construct.
// TestBuiltinShape runs it on a VM and compares the result with what Node
// 24 answers, recorded in shape.json by:
//
//	node -e "$(cat shape.js); console.log(shape())" > shape.json
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

	const ctors = {
		DateTimeFormat: ['format', 'formatToParts', 'resolvedOptions'],
		NumberFormat: ['format', 'formatToParts', 'resolvedOptions'],
		PluralRules: ['select', 'selectRange', 'resolvedOptions'],
	};
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
		const o = new C('en');
		r[p + ' instance toString'] = Object.prototype.toString.call(o);
		r[p + ' instance keys'] = Reflect.ownKeys(o).map(String).join(' ');
		if (typeof o.format === 'function') {
			fn(p + ' bound format', o.format);
			r[p + ' bound format same'] = o.format === o.format;
		}
		try {
			P.resolvedOptions();
			r[p + '.prototype.resolvedOptions()'] = 'no error';
		} catch (e) {
			r[p + '.prototype.resolvedOptions()'] = e.constructor.name;
		}
	}

	for (const [name, o, keys] of methods) {
		for (const k of keys) {
			prop(name + '.' + k, o, k);
			fn(name + '.' + k, o[k]);
		}
	}
	return JSON.stringify(r, null, '\t');
}
