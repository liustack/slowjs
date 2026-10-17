// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// What Register puts on a VM: stand-ins for Intl and the toLocale* methods.
// The first time a page touches any of them, install() runs intl.js, which
// replaces them all with the real thing. A VM that never formats anything
// never pays for the shim.

(function () {
	'use strict';

	const hostNames = [
		'__quickjs_intl_dtfResolve__', '__quickjs_intl_dtfFormat__', '__quickjs_intl_nfResolve__',
		'__quickjs_intl_nfFormat__', '__quickjs_intl_canonicalLocales__', '__quickjs_intl_prResolve__',
		'__quickjs_intl_prSelect__'
	];

	const g = globalThis;
	const install = g.__quickjs_intl_install__;
	const installRunning = g.__quickjs_intl_installRunning__;
	delete g.__quickjs_intl_install__;
	delete g.__quickjs_intl_installRunning__;

	// The host functions wait out of sight until intl.js comes to take them.
	const host = [];
	for (let i = 0; i < hostNames.length; i++) {
		host.push(g[hostNames[i]]);
		delete g[hostNames[i]];
	}

	let installed = false, installing = false;
	let userIntl, userSetIntl = false;
	const standIns = [];

	// ensure installs the shim, once it works. An install that fails, say for
	// want of stack, rolls back to the stand-ins and throws, so that this use
	// fails and the next one tries again.
	//
	// An interrupt, from vm.Interrupt or a timeout, and running out of memory
	// are no exception a catch or finally sees: one that lands here leaves
	// installing set with nothing rolled back, and the host functions
	// possibly in sight. The host knows
	// whether an install is running, so the next use tells a nested call
	// from an install cut short, rolls the latter back and starts over.
	function ensure() {
		if (installed) {
			return;
		}
		if (installing) {
			if (installRunning()) {
				throw new Error('intl: used while being installed');
			}
			rollBack();
		}
		installing = true;
		try {
			for (let i = 0; i < hostNames.length; i++) {
				Object.defineProperty(g, hostNames[i], {value: host[i], writable: true, enumerable: false, configurable: true});
			}
			install();
		} catch (e) {
			rollBack();
			installing = false;
			throw e;
		}
		if (userSetIntl) {
			setIntl(userIntl);
		}
		installed = true;
		installing = false;
	}

	// rollBack undoes what a failed install got done: the host functions go
	// back out of sight and the stand-ins, or the page's own Intl, back in
	// place.
	function rollBack() {
		for (let i = 0; i < hostNames.length; i++) {
			delete g[hostNames[i]];
		}
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

	function standIn(o, name) {
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
		standIns.push({o: o, name: name, f: f});
		Object.defineProperty(o, name, {value: f, writable: true, enumerable: false, configurable: true});
	}

	standIn(Date.prototype, 'toLocaleString');
	standIn(Date.prototype, 'toLocaleDateString');
	standIn(Date.prototype, 'toLocaleTimeString');
	standIn(Number.prototype, 'toLocaleString');
	if (typeof BigInt === 'function') {
		standIn(BigInt.prototype, 'toLocaleString');
	}

	// How the host recognizes a VM it has registered, without installing
	// anything: keyed by the import path, invisible to enumeration and JSON.
	Object.defineProperty(g, Symbol.for('modernc.org/quickjs/intl'), {value: true});
})();
