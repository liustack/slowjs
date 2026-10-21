// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// What Register puts on a VM: stand-ins for Intl and the toLocale* methods.
// The first time a page touches any of them, install() returns the real
// things, made by intl.js, and they replace the stand-ins. A VM that never
// formats anything never pays for the shim.
//
// Register calls this function with the install Builtin and the VM's
// default time zone.

(function (install, zone) {
	'use strict';

	const g = globalThis;

	let installed = false, installing = false;
	let userIntl, userSetIntl = false;
	const standIns = [];

	// ensure installs the shim, once it works. An install that fails, say for
	// want of stack, throws, so that this use fails and the next one tries
	// again.
	//
	// An interrupt, from vm.Interrupt or a timeout, and running out of memory
	// are no exception a catch or finally sees: one that lands here leaves
	// installing set and maybe some of the real things in place. The next use
	// rolls those back and starts over.
	function ensure() {
		if (installed) {
			return;
		}
		const retry = installing;
		if (retry) {
			rollBack();
		}
		installing = true;
		let api;
		try {
			api = install(zone, retry);
		} catch (e) {
			installing = false;
			throw e;
		}
		if (!userSetIntl) {
			setIntl(api.Intl);
		}
		for (let i = 0; i < standIns.length; i++) {
			const s = standIns[i];
			Object.defineProperty(s.o, s.name, {value: api[s.api], writable: true, enumerable: false, configurable: true});
		}
		installed = true;
		installing = false;
	}

	// rollBack undoes what an install cut short got done: the stand-ins, or
	// the page's own Intl, go back in place.
	function rollBack() {
		if (userSetIntl) {
			setIntl(userIntl);
		} else {
			Object.defineProperty(g, 'Intl', intlStandIn);
		}
		for (let i = 0; i < standIns.length; i++) {
			const s = standIns[i];
			Object.defineProperty(s.o, s.name, {value: s.f, writable: true, enumerable: false, configurable: true});
		}
	}

	function setIntl(v) {
		Object.defineProperty(g, 'Intl', {value: v, writable: true, enumerable: false, configurable: true});
	}

	const intlStandIn = {
		enumerable: false,
		configurable: true,
		get: function getIntl() {
			ensure();
			// Reading g.Intl through the stand-in itself would recurse.
			const d = Object.getOwnPropertyDescriptor(g, 'Intl');
			if (d !== undefined && d.get === getIntl) {
				throw new Error('intl: Intl was not installed');
			}
			return g.Intl;
		},
		set: function (v) {
			// A page assigning its own Intl keeps it, before or after the
			// real one is installed.
			userIntl = v;
			userSetIntl = true;
			setIntl(v);
		}
	};
	Object.defineProperty(g, 'Intl', intlStandIn);

	// standIn puts a stand-in for o[name] in place, which api[apiName]
	// replaces.
	function standIn(o, name, apiName) {
		const f = {
			[name]: function () {
				ensure();
				const real = o[name];
				if (real === f) {
					throw new Error('intl: ' + name + ' was not installed');
				}
				return real.apply(this, arguments);
			}
		}[name];
		standIns.push({o: o, name: name, api: apiName, f: f});
		Object.defineProperty(o, name, {value: f, writable: true, enumerable: false, configurable: true});
	}

	standIn(Date.prototype, 'toLocaleString', 'dateToLocaleString');
	standIn(Date.prototype, 'toLocaleDateString', 'dateToLocaleDateString');
	standIn(Date.prototype, 'toLocaleTimeString', 'dateToLocaleTimeString');
	standIn(Number.prototype, 'toLocaleString', 'numberToLocaleString');
	if (typeof BigInt === 'function') {
		standIn(BigInt.prototype, 'toLocaleString', 'bigintToLocaleString');
	}

	// How the host recognizes a VM it has registered, without installing
	// anything: keyed by the import path, invisible to enumeration and JSON.
	Object.defineProperty(g, Symbol.for('modernc.org/quickjs/intl'), {value: true});
})
