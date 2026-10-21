// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// What Register puts on a VM: stand-ins for Intl, the toLocale* methods and
// String.prototype.localeCompare.
// The first time a page touches any of them, install() returns the real
// things, made by intl.js, and they replace the stand-ins. A VM that never
// formats anything never pays for the shim.
//
// Register calls this function with the install Builtin and the VM's
// default time zone, before any code of the page runs. So it takes here the
// built-ins Intl's code uses, which a page may change later, and that code
// uses nothing else.

(function (install, zone) {
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
		hasInstance: Function.prototype[Symbol.hasInstance],
		dateNow: Date.now,
		dateValueOf: Date.prototype.valueOf,
		numberValueOf: Number.prototype.valueOf,
		bigintValueOf: typeof BigInt === 'function' ? BigInt.prototype.valueOf : undefined
	};

	let api, installing = false;
	let userIntl, userSetIntl = false;

	// ensure installs the shim, once it works, and returns what it made. An
	// install that fails, say for want of stack, throws, so that this use
	// fails and the next one tries again.
	//
	// An interrupt, from vm.Interrupt or a timeout, and running out of memory
	// are no exception a catch or finally sees: one that lands here leaves
	// installing set and maybe some of the real things in place. The next use
	// rolls those back and starts over.
	function ensure() {
		if (api !== undefined) {
			return api;
		}
		const retry = installing;
		if (retry) {
			rollBack();
		}
		installing = true;
		let a;
		try {
			a = install(P, zone, retry);
		} catch (e) {
			installing = false;
			throw e;
		}
		if (!userSetIntl) {
			setIntl(a.Intl);
		}
		for (let i = 0; i < standIns.length; i++) {
			const s = standIns[i];
			defineProperty(s.o, s.name, {__proto__: null, value: a[s.api], writable: true, enumerable: false, configurable: true});
		}
		api = a;
		installing = false;
		return a;
	}

	// rollBack undoes what an install cut short got done: the stand-ins, or
	// the page's own Intl, go back in place.
	function rollBack() {
		if (userSetIntl) {
			setIntl(userIntl);
		} else {
			defineProperty(g, 'Intl', intlStandIn);
		}
		for (let i = 0; i < standIns.length; i++) {
			const s = standIns[i];
			defineProperty(s.o, s.name, {__proto__: null, value: s.f, writable: true, enumerable: false, configurable: true});
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
			const a = ensure();
			return userSetIntl ? userIntl : a.Intl;
		},
		set(v) {
			// A page assigning its own Intl keeps it, before or after the
			// real one is installed.
			userIntl = v;
			userSetIntl = true;
			setIntl(v);
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
		standIn(String.prototype, 'localeCompare', 'stringLocaleCompare', 1)
	];
	if (typeof BigInt === 'function') {
		standIns[standIns.length] = standIn(BigInt.prototype, 'toLocaleString', 'bigintToLocaleString');
	}

	// How the host recognizes a VM it has registered, without installing
	// anything: keyed by the import path, invisible to enumeration and JSON.
	defineProperty(g, Symbol.for('modernc.org/quickjs/intl'), {__proto__: null, value: true});
})
