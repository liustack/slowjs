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
	delete g.__quickjs_intl_install__;

	// The host functions wait out of sight until intl.js comes to take them.
	const host = [];
	for (let i = 0; i < hostNames.length; i++) {
		host.push(g[hostNames[i]]);
		delete g[hostNames[i]];
	}

	let installed = false;
	let userIntl, userSetIntl = false;

	function ensure() {
		if (installed) {
			return;
		}
		installed = true;
		for (let i = 0; i < hostNames.length; i++) {
			Object.defineProperty(g, hostNames[i], {value: host[i], writable: true, enumerable: false, configurable: true});
		}
		install();
		if (userSetIntl) {
			Object.defineProperty(g, 'Intl', {value: userIntl, writable: true, enumerable: false, configurable: true});
		}
	}

	Object.defineProperty(g, 'Intl', {
		enumerable: false,
		configurable: true,
		get: function () {
			ensure();
			return g.Intl;
		},
		set: function (v) {
			// A page assigning its own Intl keeps it, before or after the
			// real one is installed.
			userIntl = v;
			userSetIntl = true;
			Object.defineProperty(g, 'Intl', {value: v, writable: true, enumerable: false, configurable: true});
		}
	});

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
