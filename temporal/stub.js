// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// What Register puts on a VM: a stand-in for the global Temporal and one for
// Date.prototype.toTemporalInstant. The first time a page touches either,
// install() returns the real things, made by temporal.js, and they replace
// the stand-ins a page has left in place. A VM that never uses Temporal
// never pays for it.
//
// Register calls this function with the install Builtin before any code of
// the page runs. So it takes here the built-ins Temporal's code uses, which
// a page may change later, and that code uses nothing else.

(function (install, state) {
	'use strict';

	const g = globalThis;
	const dateProto = Date.prototype;
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
		Number: Number,
		String: String,
		BigInt: BigInt,
		Error: Error,
		TypeError: TypeError,
		RangeError: RangeError,
		toStringTag: Symbol.toStringTag,
		toPrimitive: Symbol.toPrimitive,
		dateValueOf: dateProto.valueOf,
		floor: Math.floor,
		stringSlice: String.prototype.slice
	};

	const getOwn = Object.getOwnPropertyDescriptor;
	const hasOwn = Object.hasOwn;
	let api;

	// ensure installs the shim, once it works, and returns what it made. An
	// install cut short by an interrupt or by running out of memory, which
	// no catch sees, has changed nothing: the next use starts over. Once
	// made, the real things take the place of the stand-ins that are still
	// there, as settle says.
	function ensure() {
		if (api === undefined) {
			api = install(P, state);
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
		const t = getOwn(g, 'Temporal');
		if (t !== undefined && t.configurable && hasOwn(t, 'get') && t.get === standIn.get) {
			setTemporal(a.Temporal);
		}
		const d = getOwn(dateProto, 'toTemporalInstant');
		if (d !== undefined && d.configurable && hasOwn(d, 'value') && d.value === toTemporalInstant) {
			defineProperty(dateProto, 'toTemporalInstant', {__proto__: null, value: a.toTemporalInstant, writable: true, enumerable: false, configurable: true});
		}
	}

	function setTemporal(v) {
		defineProperty(g, 'Temporal', {__proto__: null, value: v, writable: true, enumerable: false, configurable: true});
	}

	const standIn = {
		__proto__: null,
		enumerable: false,
		configurable: true,
		get() {
			return ensure().Temporal;
		},
		set(v) {
			// A page assigning its own Temporal, a polyfill say, keeps it,
			// unless the stand-in was frozen in place, as a frozen built-in
			// would be.
			const t = getOwn(g, 'Temporal');
			if (t !== undefined && t.configurable) {
				setTemporal(v);
			}
		}
	};
	defineProperty(g, 'Temporal', standIn);

	const toTemporalInstant = {
		toTemporalInstant() {
			return apply(ensure().toTemporalInstant, this, []);
		}
	}.toTemporalInstant;
	defineProperty(dateProto, 'toTemporalInstant', {__proto__: null, value: toTemporalInstant, writable: true, enumerable: false, configurable: true});

	// How the host recognizes a VM it has registered, without installing
	// anything: keyed by the import path, invisible to enumeration and JSON.
	defineProperty(g, Symbol.for('modernc.org/quickjs/temporal'), {__proto__: null, value: true});
})
