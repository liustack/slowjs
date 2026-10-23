// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// What Register puts on a VM: stand-ins for Intl, the toLocale* methods and
// String.prototype.localeCompare.
// The first time a page touches any of them, install() returns the real
// things, made by intl.js, and they replace the stand-ins a page has left in
// place. A VM that never formats anything never pays for the shim.
//
// Register calls this function with the install Builtin and the VM's
// default time zone, before any code of the page runs. So it takes here the
// built-ins Intl's code uses, which a page may change later, and that code
// uses nothing else.

(function (install, zone, state) {
	'use strict';

	const g = globalThis;
	const {apply} = Reflect;
	const {defineProperty} = Object;
	const P = {
		__proto__: null,
		apply: apply,
		defineProperty: defineProperty,
		getOwnPropertyDescriptor: Object.getOwnPropertyDescriptor,
		ownKeys: Reflect.ownKeys,
		setPrototypeOf: Object.setPrototypeOf,
		parse: JSON.parse,
		Object: Object,
		String: String,
		Number: Number,
		Error: Error,
		TypeError: TypeError,
		RangeError: RangeError,
		Symbol: Symbol,
		toStringTag: Symbol.toStringTag,
		iterator: Symbol.iterator,
		iteratorPrototype: Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]())),
		stringSlice: String.prototype.slice,
		toUpperCase: String.prototype.toUpperCase,
		toLowerCase: String.prototype.toLowerCase,
		fromCharCode: String.fromCharCode,
		hasInstance: Function.prototype[Symbol.hasInstance],
		dateNow: Date.now,
		dateValueOf: Date.prototype.valueOf,
		numberValueOf: Number.prototype.valueOf,
		bigintValueOf: typeof BigInt === 'function' ? BigInt.prototype.valueOf : undefined
	};

	const {getOwnPropertyDescriptor: getOwn, hasOwn} = Object;
	let api, installing = false;

	// ensure installs the shim, once it works, and returns what it made. An
	// install that fails, say for want of stack, throws, so that this use
	// fails and the next one tries again.
	//
	// An interrupt, from vm.Interrupt or a timeout, and running out of memory
	// are no exception a catch or finally sees: one that lands in install
	// leaves installing set and nothing changed, and the next use starts
	// over. Once made, the real things take the place of the stand-ins that
	// are still there, as settle says.
	function ensure() {
		if (api === undefined) {
			const retry = installing;
			installing = true;
			let a;
			try {
				a = install(P, zone, retry, state);
			} catch (e) {
				installing = false;
				throw e;
			}
			api = a;
			installing = false;
		}
		settle(api);
		return api;
	}

	// settle puts the real things in place of the stand-ins, but only where
	// a stand-in is still there and can be replaced. What a page put there,
	// a property it deleted, or a stand-in frozen with its object stays as it
	// is, as it would with V8's built-ins, and a stand-in left in place goes
	// on forwarding to the real thing.
	function settle(a) {
		const d = getOwn(g, 'Intl');
		if (d !== undefined && d.configurable && hasOwn(d, 'get') && d.get === intlStandIn.get) {
			setIntl(a.Intl);
		}
		for (let i = 0; i < standIns.length; i++) {
			const s = standIns[i];
			const d = getOwn(s.o, s.name);
			if (d !== undefined && d.configurable && hasOwn(d, 'value') && d.value === s.f) {
				defineProperty(s.o, s.name, {__proto__: null, value: a[s.api], writable: true, enumerable: false, configurable: true});
			}
		}
	}

	function setIntl(v) {
		defineProperty(g, 'Intl', {__proto__: null, value: v, writable: true, enumerable: false, configurable: true});
	}

	const intlStandIn = {
		__proto__: null,
		enumerable: false,
		configurable: true,
		get() {
			return ensure().Intl;
		},
		set(v) {
			// A page assigning its own Intl keeps it, unless the stand-in was
			// frozen in place, as a frozen built-in would be.
			const d = getOwn(g, 'Intl');
			if (d !== undefined && d.configurable) {
				setIntl(v);
			}
		}
	};
	defineProperty(g, 'Intl', intlStandIn);

	// standIn puts a stand-in for o[name] in place, which api[apiName]
	// replaces. length is the built-in's.
	function standIn(o, name, apiName, length = 0) {
		// A method, so shaped as the built-in it stands for, with no
		// prototype property.
		const f = {
			[name]() {
				return apply(ensure()[apiName], this, arguments);
			}
		}[name];
		if (length !== 0) {
			defineProperty(f, 'length', {__proto__: null, value: length});
		}
		defineProperty(o, name, {__proto__: null, value: f, writable: true, enumerable: false, configurable: true});
		return {__proto__: null, o: o, name: name, api: apiName, f: f};
	}

	const standIns = [
		standIn(Date.prototype, 'toLocaleString', 'dateToLocaleString'),
		standIn(Date.prototype, 'toLocaleDateString', 'dateToLocaleDateString'),
		standIn(Date.prototype, 'toLocaleTimeString', 'dateToLocaleTimeString'),
		standIn(Number.prototype, 'toLocaleString', 'numberToLocaleString'),
		standIn(String.prototype, 'localeCompare', 'stringLocaleCompare', 1),
		standIn(String.prototype, 'toLocaleUpperCase', 'stringToLocaleUpperCase'),
		standIn(String.prototype, 'toLocaleLowerCase', 'stringToLocaleLowerCase')
	];
	if (typeof BigInt === 'function') {
		standIns[standIns.length] = standIn(BigInt.prototype, 'toLocaleString', 'bigintToLocaleString');
	}

	// How the host recognizes a VM it has registered, without installing
	// anything: keyed by the import path, invisible to enumeration and JSON.
	defineProperty(g, Symbol.for('modernc.org/quickjs/intl'), {__proto__: null, value: true});
})
