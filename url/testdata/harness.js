// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// The functions of testharness.js that the URL files of web-platform-tests
// use, and what they ask of the page around them. Results are collected in
// __results, and the promise tests in __pending.

var __results = [];
var __pending = [];
var __files = {};

function __same(a, b) {
	return Object.is(a, b);
}

function __show(v) {
	try {
		return typeof v === 'string' ? JSON.stringify(v) : String(v);
	} catch (e) {
		return typeof v;
	}
}

function assert_equals(actual, expected, description) {
	if (!__same(actual, expected)) {
		throw new Error((description ? description + ': ' : '') + 'got ' + __show(actual) + ', expected ' + __show(expected));
	}
}

function assert_not_equals(actual, unexpected, description) {
	if (__same(actual, unexpected)) {
		throw new Error((description ? description + ': ' : '') + 'got ' + __show(actual) + ', expected something else');
	}
}

function assert_true(actual, description) {
	assert_equals(actual, true, description);
}

function assert_false(actual, description) {
	assert_equals(actual, false, description);
}

function assert_unreached(description) {
	throw new Error('unreached: ' + (description || ''));
}

function assert_array_equals(actual, expected, description) {
	if (typeof actual !== 'object' || actual === null || actual.length !== expected.length) {
		throw new Error((description ? description + ': ' : '') + 'got ' + __show(actual) + ', expected an array of ' + expected.length);
	}
	for (var i = 0; i < expected.length; i++) {
		if (!__same(actual[i], expected[i])) {
			throw new Error((description ? description + ': ' : '') + 'item ' + i + ' is ' + __show(actual[i]) + ', expected ' + __show(expected[i]));
		}
	}
}

function assert_throws_js(constructor, fn, description) {
	try {
		fn();
	} catch (e) {
		if (e === null || typeof e !== 'object' || e.constructor !== constructor || !(e instanceof constructor)) {
			throw new Error((description ? description + ': ' : '') + 'threw ' + __show(e) + ', expected ' + constructor.name);
		}
		return;
	}
	throw new Error((description ? description + ': ' : '') + 'did not throw');
}

function assert_throws_exactly(exception, fn, description) {
	try {
		fn();
	} catch (e) {
		if (!__same(e, exception)) {
			throw new Error((description ? description + ': ' : '') + 'threw ' + __show(e) + ', expected ' + __show(exception));
		}
		return;
	}
	throw new Error((description ? description + ': ' : '') + 'did not throw');
}

function assert_throws_dom(name, fn, description) {
	try {
		fn();
	} catch (e) {
		if (e === null || typeof e !== 'object' || e.name !== name) {
			throw new Error((description ? description + ': ' : '') + 'threw ' + __show(e) + ', expected a DOMException ' + name);
		}
		return;
	}
	throw new Error((description ? description + ': ' : '') + 'did not throw');
}

function test(fn, name) {
	try {
		fn();
		__results.push({name: String(name), ok: true});
	} catch (e) {
		__results.push({name: String(name), ok: false, message: e && e.message !== undefined ? e.message : __show(e)});
	}
}

function promise_test(fn, name) {
	var p;
	try {
		p = fn();
	} catch (e) {
		p = Promise.reject(e);
	}
	__pending.push(p.then(function () {
		__results.push({name: String(name), ok: true});
	}, function (e) {
		__results.push({name: String(name), ok: false, message: e && e.message !== undefined ? e.message : __show(e)});
	}));
}

function subsetTestByKey(key, testFunc, fn, name) {
	testFunc(fn, name);
}

function done() {}

// fetch answers a relative path out of __files, which the host fills.
function fetch(path) {
	return Promise.resolve({
		json: function () {
			if (!(path in __files)) {
				throw new Error('no file ' + path);
			}
			return Promise.resolve(JSON.parse(__files[path]));
		}
	});
}

var self = globalThis;
self.GLOBAL = {isWindow: function () { return false; }, isWorker: function () { return false; }, isShadowRealm: function () { return false; }};
