// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

func evalString(t testing.TB, vm *quickjs.VM, src string) string {
	t.Helper()
	v, err := vm.Eval(src, quickjs.EvalGlobal)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s: %T %v, want a string", src, v, v)
	}
	return s
}

func TestRegisterTwice(t *testing.T) {
	vm := newVM(t)
	if err := Register(vm); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("second Register: %v, want an error", err)
	}
}

// TestCoreJSDetection runs the code core-js 3.50.0 uses to decide whether the
// URL of the page is good enough to leave alone (internals/url-constructor-detection.js),
// as it is, and the checks of web.url.can-parse. Its result must be that the
// native one is good, or core-js installs its own over this one.
func TestCoreJSDetection(t *testing.T) {
	vm := newVM(t)
	src := readFile(t, "testdata/corejs/url-constructor-detection.js")
	got := evalString(t, vm, `(function () {
		var module = {exports: undefined};
		function fails(f) { try { return !!f(); } catch (e) { return true; } }
		function require(name) {
			switch (name) {
			case '../internals/fails': return fails;
			case '../internals/well-known-symbol': return function (n) { return Symbol[n]; };
			case '../internals/descriptors': return true;
			case '../internals/is-pure': return false;
			}
			throw new Error(name);
		}
		(function (module, require) {`+src+`})(module, require);
		var canParse = (function () {
			if (typeof URL.canParse !== 'function' || URL.canParse.length !== 1) return false;
			try { URL.canParse(); return false; } catch (e) { return e instanceof TypeError; }
		})();
		return module.exports + ',' + canParse + ',' + (typeof URL.parse === 'function');
	})()`)
	if got != "true,true,true" {
		t.Fatalf("detection = %s, want true,true,true", got)
	}
}

// TestCoreJSKeepsNative checks that a bundle that tests for a native URL as
// core-js does, and installs a polyfill only when the test fails, leaves the
// registered one in place.
func TestCoreJSKeepsNative(t *testing.T) {
	vm := newVM(t)
	got := evalString(t, vm, `(function () {
		var before = globalThis.URL, beforeSP = globalThis.URLSearchParams;
		function polyfillNeeded() {
			try {
				var u = new URL('b?a=1&b=2&c=3', 'https://a');
				return u.searchParams.get('c') !== '3' || new URL('https://тест').host !== 'xn--e1aybc';
			} catch (e) { return true; }
		}
		if (polyfillNeeded()) { globalThis.URL = function () {}; }
		return (globalThis.URL === before) + ',' + (globalThis.URLSearchParams === beforeSP);
	})()`)
	if got != "true,true" {
		t.Fatalf("got %s", got)
	}
}

// TestBattery checks the answers of the battery of testdata/battery.js: the
// same on every VM, and what a browser answers for what it asks.
func TestBattery(t *testing.T) {
	got := evalString(t, newVM(t), "("+readFile(t, "testdata/battery.js")+")(Object.getOwnPropertyDescriptor, Reflect.apply, Object.prototype.toString, URL, URLSearchParams)")
	want := readFile(t, "testdata/battery.txt")
	if got != want {
		t.Fatalf("battery answered\n%s\nwant\n%s", got, want)
	}
}

// tampers are pages that change the built-ins an implementation in javascript
// might use, after Register and before their first use of URL. None of the
// changes may show: what the battery answers is what it answered before,
// and the traps that count their calls were never called.
var tampers = map[string]string{
	"array index setters": `
		for (var i = 0; i < 8; i++) {
			(function (i) {
				Object.defineProperty(Array.prototype, i, {configurable: true, set: function (v) {
					trapped++;
					Object.defineProperty(this, i, {value: v, writable: true, enumerable: true, configurable: true});
				}});
			})(i);
		}`,
	"object property traps": `
		['value', 'done', 'next', 'enumerable', 'configurable', 'writable', 'get', 'set', 'href', 'length', 'type', 'constructor', 'then', 'size'].forEach(function (k) {
			if (k === 'constructor') return;
			var d = Object.create(null);
			d.configurable = true;
			d.get = function () { trapped++; };
			d.set = function (v) { trapped++; };
			Object.defineProperty(Object.prototype, k, d);
		});`,
	"array methods": `
		['push', 'pop', 'shift', 'unshift', 'slice', 'splice', 'concat', 'indexOf', 'lastIndexOf', 'includes', 'join', 'map', 'filter', 'forEach', 'reduce', 'sort', 'reverse', 'fill', 'find', 'keys', 'values', 'entries', 'flat', 'at', 'copyWithin'].forEach(function (k) {
			Array.prototype[k] = function () { trapped++; throw new Error('array ' + k); };
		});
		Array.isArray = Array.from = Array.of = function () { trapped++; throw new Error('Array.x'); };`,
	"array iterator": `
		var orig = Array.prototype[Symbol.iterator];
		Array.prototype[Symbol.iterator] = function () { trapped++; return orig.call(this); };
`,
	"string methods": `
		['slice', 'substring', 'substr', 'indexOf', 'lastIndexOf', 'charAt', 'charCodeAt', 'codePointAt', 'split', 'concat', 'replace', 'replaceAll', 'startsWith', 'endsWith', 'includes', 'toWellFormed', 'isWellFormed', 'toLowerCase', 'toUpperCase', 'trim', 'at', 'repeat', 'normalize', 'padStart'].forEach(function (k) {
			String.prototype[k] = function () { trapped++; throw new Error('string ' + k); };
		});
		String.fromCharCode = String.fromCodePoint = function () { trapped++; throw new Error('String.x'); };`,
	"object and reflect": `
		['defineProperty', 'defineProperties', 'getOwnPropertyDescriptor', 'getOwnPropertyDescriptors', 'getOwnPropertyNames', 'getOwnPropertySymbols', 'keys', 'values', 'entries', 'assign', 'create', 'freeze', 'getPrototypeOf', 'setPrototypeOf', 'fromEntries', 'hasOwn', 'is', 'isFrozen', 'preventExtensions'].forEach(function (k) {
			Object[k] = function () { trapped++; throw new Error('Object.' + k); };
		});
		['apply', 'construct', 'defineProperty', 'get', 'getOwnPropertyDescriptor', 'getPrototypeOf', 'has', 'ownKeys', 'set', 'setPrototypeOf'].forEach(function (k) {
			Reflect[k] = function () { trapped++; throw new Error('Reflect.' + k); };
		});`,
	"function methods": `
		['call', 'apply', 'bind'].forEach(function (k) {
			Function.prototype[k] = function () { trapped++; throw new Error('Function.' + k); };
		});
		Function.prototype[Symbol.hasInstance] = function () { trapped++; throw new Error('hasInstance'); };`,
	"globals": `
		JSON.parse = JSON.stringify = function () { trapped++; throw new Error('JSON'); };
		globalThis.String = globalThis.Number = globalThis.Object = globalThis.Array = globalThis.Symbol = globalThis.TypeError = globalThis.Error = globalThis.Reflect = globalThis.JSON = function () { trapped++; throw new Error('global'); };
		Symbol.for = Symbol.keyFor = function () { trapped++; throw new Error('Symbol.for'); };`,
	"the URL globals": `
		globalThis.URL = function () { trapped++; };
		globalThis.URLSearchParams = function () { trapped++; };`,
}

// TestTamper runs the battery on VMs whose page changed the built-ins first.
// TestTamper runs the battery on VMs whose page changed the built-ins first.
// The battery is a function of the built-ins it needs and of the classes, which
// the test reads before the page changes anything.
func TestTamper(t *testing.T) {
	want := readFile(t, "testdata/battery.txt")
	battery := readFile(t, "testdata/battery.js")
	for name, src := range tampers {
		t.Run(name, func(t *testing.T) {
			vm := newVM(t)
			evalString(t, vm, `var trapped = 0; var __b = `+battery+`;
				var __args = [Object.getOwnPropertyDescriptor, Reflect.apply, Object.prototype.toString, URL, URLSearchParams]; 'ok'`)
			evalString(t, vm, `(function(){`+src+`})(); 'ok'`)
			got := evalString(t, vm, `__b(__args[0], __args[1], __args[2], __args[3], __args[4])`)
			if got != want {
				t.Fatalf("battery answered\n%s\nwant\n%s", got, want)
			}
			// A page that changes how arrays iterate is seen by the
			// constructor of URLSearchParams that takes a sequence, as it is
			// in a browser.
			if n := evalString(t, vm, "'' + trapped"); n != "0" && name != "array iterator" {
				t.Fatalf("%s traps were called", n)
			}
		})
	}
}
