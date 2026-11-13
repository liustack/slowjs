// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// The javascript half of package url: URL and URLSearchParams as the URL
// standard (https://url.spec.whatwg.org/#api) defines them. Go parses, and
// keeps nothing: a URL holds, in a private field, the array its Builtin
// answered, the href, the parts the getters read and the bits of the record
// they do not show, and every setter hands the record, the part and the new
// value to Go, which makes the record of the parts, applies the setter and
// answers a new array. The record is the state of a URL, as the standard has
// it, and not its href, which the parser could make into another record. A
// URLSearchParams holds its list in a private field, as a flat array of names
// and values.
//
// It evaluates to a function of the five Builtins (see bridge.go) that
// installs both classes on the global object. Register calls it before any
// code of the page runs, so what it takes here is what the page cannot
// change: it uses only what it captured, object and array literals, null
// prototype arrays for what it appends to, and index loops: not the iterators
// of an array, spread, destructuring, template literals (QuickJS makes them
// calls of String.prototype.concat) or a method a page can replace.

(function (rawParse, rawSet, rawFormDecode, rawFormEncode, rawCanParse) {
	'use strict';

	const g = globalThis;
	const {apply, ownKeys} = Reflect;
	const {defineProperty, getOwnPropertyDescriptor, setPrototypeOf} = Object;
	const {String, TypeError, RangeError, Symbol} = g;
	const {iterator, toStringTag} = Symbol;
	const toWellFormed = String.prototype.toWellFormed;
	const indexOf = String.prototype.indexOf, slice = String.prototype.slice;
	const plusArgs = ['+'], percentArgs = ['%'], equalsArgs = ['='];
	const arrayPrototype = Array.prototype;
	const iteratorPrototype = Object.getPrototypeOf(Object.getPrototypeOf([][iterator]()));
	const noArgs = [];

	// The positions in the array that parse and set answer.
	const HREF = 0, PROTOCOL = 1, USERNAME = 2, PASSWORD = 3, HOST = 4, HOSTNAME = 5, PORT = 6,
		PATHNAME = 7, SEARCH = 8, HASH = 9, ORIGIN = 10, FLAGS = 11;
	// The parts set takes, in the order of the array above.
	const SET_PROTOCOL = 1, SET_USERNAME = 2, SET_PASSWORD = 3, SET_HOST = 4, SET_HOSTNAME = 5,
		SET_PORT = 6, SET_PATHNAME = 7, SET_SEARCH = 8, SET_HASH = 9;

	function bare(a) {
		return setPrototypeOf(a, null);
	}

	function isObject(v) {
		return (typeof v === 'object' && v !== null) || typeof v === 'function';
	}

	// toStr is ToString, which throws for a Symbol where String() names it.
	function toStr(v) {
		if (typeof v === 'string') {
			return v;
		}
		if (typeof v === 'symbol') {
			throw new TypeError('Cannot convert a Symbol value to a string');
		}
		return String(v);
	}

	// toUSV is the conversion to a USVString: ToString, then each lone
	// surrogate becomes U+FFFD.
	function toUSV(v) {
		return apply(toWellFormed, toStr(v), noArgs);
	}

	function parseURL(url, base) {
		return rawParse(toStr(url), base === undefined ? undefined : toStr(base));
	}

	function canParseURL(url, base) {
		return rawCanParse(toStr(url), base === undefined ? undefined : toStr(base));
	}

	// decodeField is a name or a value of a query string as the
	// application/x-www-form-urlencoded parser makes it. Go is asked only when
	// there is something to decode: a string that has neither + nor % is made of
	// itself, as it is well formed.
	function decodeField(f) {
		if (apply(indexOf, f, plusArgs) < 0 && apply(indexOf, f, percentArgs) < 0) {
			return f;
		}
		return rawFormDecode(f);
	}

	// parseQuery is the list of a query string, without a first question mark.
	// The loops are here, in javascript, where the engine's polls and memory
	// limit hold them, and Go sees one name or value at a time. Each & is found
	// once and each = in its own sequence, so the work is linear in the string.
	function parseQuery(text) {
		const s = apply(toWellFormed, text, noArgs), n = s.length;
		const l = bare([]);
		let start = n > 0 && s[0] === '?' ? 1 : 0;
		while (start < n) {
			let end = apply(indexOf, s, ['&', start]);
			if (end < 0) {
				end = n;
			}
			if (end > start) {
				const seq = apply(slice, s, [start, end]);
				const eq = apply(indexOf, seq, equalsArgs);
				if (eq < 0) {
					l[l.length] = decodeField(seq);
					l[l.length] = '';
				} else {
					l[l.length] = decodeField(apply(slice, seq, [0, eq]));
					l[l.length] = decodeField(apply(slice, seq, [eq + 1]));
				}
			}
			start = end + 1;
		}
		return l;
	}

	function encodeComponent(s) {
		return rawFormEncode(s);
	}

	function missing(what, required, present) {
		return new TypeError(what + ': ' + required + ' argument' + (required === 1 ? '' : 's') + ' required, but only ' + present + ' present.');
	}

	// How the two classes reach what is private to the other, and how a
	// URLSearchParams is made for a URL and a URL from a record (URL.parse).
	let urlSearch, urlSetSearch, spSetList, spList, makeIterator;

	// A Handoff carries a value from this code to a constructor of its own
	// that it calls, in an argument no page can make: the record of a URL for
	// URL.parse, the URL for the URLSearchParams of its searchParams. Nothing is
	// kept between the two calls, so a call that ends in the middle leaves
	// nothing behind.
	class Handoff {
		#v;

		constructor(v) {
			this.#v = v;
		}

		static is(o) {
			return typeof o === 'object' && o !== null && #v in o;
		}

		static value(o) {
			return o.#v;
		}
	}

	class URL {
		#r;
		#sp;

		static {
			urlSearch = (u) => u.#r[SEARCH];
			urlSetSearch = (u, q) => u.#change(SET_SEARCH, q);
		}

		constructor(url, base = undefined) {
			if (Handoff.is(url)) {
				this.#r = Handoff.value(url);
				return;
			}
			if (arguments.length === 0) {
				throw missing("Failed to construct 'URL'", 1, 0);
			}
			const r = parseURL(url, base);
			if (r === undefined) {
				throw new TypeError("Failed to construct 'URL': Invalid URL");
			}
			this.#r = r;
		}

		static canParse(url, base = undefined) {
			if (arguments.length === 0) {
				throw missing("Failed to execute 'canParse' on 'URL'", 1, 0);
			}
			return canParseURL(url, base) === 1;
		}

		static parse(url, base = undefined) {
			if (arguments.length === 0) {
				throw missing("Failed to execute 'parse' on 'URL'", 1, 0);
			}
			const r = parseURL(url, base);
			if (r === undefined) {
				return null;
			}
			return new URL(new Handoff(r));
		}

		// change sets part to v: Go makes the record of the parts, applies the
		// setter and answers the new parts. It answers 0 when v or the URL it
		// would make is longer than the limit, and then the URL is left as it
		// was and change is false. Parts that are not those of a URL, which
		// only a fault of this code could hand it, are a TypeError, and the URL
		// is left as it was too. No script reaches the record or rawSet.
		#change(part, v) {
			const o = this.#r;
			const r = rawSet(part, v, o[FLAGS], o[PROTOCOL], o[USERNAME], o[PASSWORD], o[HOSTNAME], o[PORT], o[PATHNAME], o[SEARCH], o[HASH]);
			if (typeof r === 'object') {
				this.#r = r;
				return true;
			}
			return false;
		}

		get href() {
			return this.#r[HREF];
		}

		set href(v) {
			this.#r[HREF];
			const r = parseURL(v, undefined);
			if (r === undefined) {
				throw new TypeError("Failed to set the 'href' property on 'URL': Invalid URL");
			}
			this.#r = r;
			if (this.#sp !== undefined) {
				spSetList(this.#sp, parseQuery(r[SEARCH]));
			}
		}

		get origin() {
			return this.#r[ORIGIN];
		}

		get protocol() {
			return this.#r[PROTOCOL];
		}

		set protocol(v) {
			this.#r[HREF];
			this.#change(SET_PROTOCOL, toStr(v));
		}

		get username() {
			return this.#r[USERNAME];
		}

		set username(v) {
			this.#r[HREF];
			this.#change(SET_USERNAME, toStr(v));
		}

		get password() {
			return this.#r[PASSWORD];
		}

		set password(v) {
			this.#r[HREF];
			this.#change(SET_PASSWORD, toStr(v));
		}

		get host() {
			return this.#r[HOST];
		}

		set host(v) {
			this.#r[HREF];
			this.#change(SET_HOST, toStr(v));
		}

		get hostname() {
			return this.#r[HOSTNAME];
		}

		set hostname(v) {
			this.#r[HREF];
			this.#change(SET_HOSTNAME, toStr(v));
		}

		get port() {
			return this.#r[PORT];
		}

		set port(v) {
			this.#r[HREF];
			this.#change(SET_PORT, toStr(v));
		}

		get pathname() {
			return this.#r[PATHNAME];
		}

		set pathname(v) {
			this.#r[HREF];
			this.#change(SET_PATHNAME, toStr(v));
		}

		get search() {
			return this.#r[SEARCH];
		}

		set search(v) {
			this.#r[HREF];
			if (this.#change(SET_SEARCH, toStr(v)) && this.#sp !== undefined) {
				spSetList(this.#sp, parseQuery(this.#r[SEARCH]));
			}
		}

		get searchParams() {
			this.#r[HREF];
			if (this.#sp === undefined) {
				this.#sp = new URLSearchParams(new Handoff(this));
			}
			return this.#sp;
		}

		get hash() {
			return this.#r[HASH];
		}

		set hash(v) {
			this.#r[HREF];
			this.#change(SET_HASH, toStr(v));
		}

		toString() {
			return this.#r[HREF];
		}

		toJSON() {
			return this.#r[HREF];
		}
	}

	// serialize is the application/x-www-form-urlencoded serializer of a list.
	function serialize(l) {
		let s = '';
		for (let i = 0; i < l.length; i += 2) {
			if (i > 0) {
				s += '&';
			}
			s += encodeComponent(l[i]) + '=' + encodeComponent(l[i + 1]);
		}
		return s;
	}

	// sortPairs sorts the pairs of the flat list l by name, in code units,
	// stably.
	function sortPairs(l) {
		const n = l.length >> 1;
		if (n < 2) {
			return;
		}
		let src = l, dst = bare([]);
		for (let w = 1; w < n; w *= 2) {
			for (let lo = 0; lo < n; lo += 2 * w) {
				const mid = lo + w < n ? lo + w : n, hi = lo + 2 * w < n ? lo + 2 * w : n;
				let a = lo, b = mid, o = lo;
				while (a < mid && b < hi) {
					if (src[2 * b] < src[2 * a]) {
						dst[2 * o] = src[2 * b];
						dst[2 * o + 1] = src[2 * b + 1];
						b++;
					} else {
						dst[2 * o] = src[2 * a];
						dst[2 * o + 1] = src[2 * a + 1];
						a++;
					}
					o++;
				}
				while (a < mid) {
					dst[2 * o] = src[2 * a];
					dst[2 * o + 1] = src[2 * a + 1];
					a++;
					o++;
				}
				while (b < hi) {
					dst[2 * o] = src[2 * b];
					dst[2 * o + 1] = src[2 * b + 1];
					b++;
					o++;
				}
			}
			const t = src;
			src = dst;
			dst = t;
		}
		if (src !== l) {
			for (let i = 0; i < l.length; i++) {
				l[i] = src[i];
			}
		}
	}

	class URLSearchParams {
		#list;
		#url;

		static {
			spSetList = (sp, list) => {
				sp.#list = bare(list);
			};
			spList = (sp) => sp.#list;
		}

		constructor(init = undefined) {
			if (Handoff.is(init)) {
				const owner = Handoff.value(init);
				this.#url = owner;
				this.#list = bare(parseQuery(urlSearch(owner)));
				return;
			}
			if (!isObject(init)) {
				this.#list = bare(parseQuery(init === undefined ? '' : toStr(init)));
				return;
			}
			const l = bare([]);
			const method = init[iterator];
			if (method !== undefined && method !== null) {
				if (typeof method !== 'function') {
					throw new TypeError("Failed to construct 'URLSearchParams': The provided value is not iterable");
				}
				const it = apply(method, init, noArgs);
				if (!isObject(it)) {
					throw new TypeError('Result of the Symbol.iterator method is not an object');
				}
				const next = it.next;
				// The whole sequence is converted first, and only then does the initialization
				// of URLSearchParams look at the length of each pair, so what a later
				// element does comes before the error of an earlier one.
				const rows = bare([]);
				for (;;) {
					const step = apply(next, it, noArgs);
					if (!isObject(step)) {
						throw new TypeError('Iterator result is not an object');
					}
					if (step.done) {
						break;
					}
					const pair = step.value;
					if (!isObject(pair)) {
						throw new TypeError("Failed to construct 'URLSearchParams': Each query pair must be iterable");
					}
					const pm = pair[iterator];
					if (typeof pm !== 'function') {
						throw new TypeError("Failed to construct 'URLSearchParams': Each query pair must be iterable");
					}
					const pit = apply(pm, pair, noArgs);
					if (!isObject(pit)) {
						throw new TypeError('Result of the Symbol.iterator method is not an object');
					}
					const pnext = pit.next;
					const items = bare([]);
					for (;;) {
						const pstep = apply(pnext, pit, noArgs);
						if (!isObject(pstep)) {
							throw new TypeError('Iterator result is not an object');
						}
						if (pstep.done) {
							break;
						}
						items[items.length] = toUSV(pstep.value);
					}
					rows[rows.length] = items;
				}
				for (let i = 0; i < rows.length; i++) {
					const items = rows[i];
					if (items.length !== 2) {
						throw new TypeError("Failed to construct 'URLSearchParams': Each query pair must be a name/value tuple");
					}
					l[l.length] = items[0];
					l[l.length] = items[1];
				}
			} else {
				const keys = ownKeys(init);
				const seen = bare({});
				for (let i = 0; i < keys.length; i++) {
					const key = keys[i];
					const d = getOwnPropertyDescriptor(init, key);
					if (d === undefined || !setPrototypeOf(d, null).enumerable) {
						continue;
					}
					const name = toUSV(key), value = toUSV(init[key]);
					if (name in seen) {
						l[seen[name] + 1] = value;
					} else {
						seen[name] = l.length;
						l[l.length] = name;
						l[l.length] = value;
					}
				}
			}
			this.#list = l;
		}

		// draft is the list a change edits: the list itself, or a copy when
		// the list is a URL's, so that a change the URL refuses leaves both as
		// they were.
		#draft() {
			const l = this.#list;
			if (this.#url === undefined) {
				return l;
			}
			const c = bare([]);
			for (let i = 0; i < l.length; i++) {
				c[i] = l[i];
			}
			return c;
		}

		// commit makes l the list, after the URL, if there is one, took its
		// serialization as its query.
		#commit(l) {
			const u = this.#url;
			if (u !== undefined && !urlSetSearch(u, serialize(l))) {
				throw new RangeError('URLSearchParams: the URL would be longer than 65536 UTF-16 code units');
			}
			this.#list = l;
		}

		get size() {
			return this.#list.length >> 1;
		}

		append(name, value) {
			this.#list;
			if (arguments.length < 2) {
				throw missing("Failed to execute 'append' on 'URLSearchParams'", 2, arguments.length);
			}
			const n = toUSV(name), v = toUSV(value);
			const l = this.#draft();
			l[l.length] = n;
			l[l.length] = v;
			this.#commit(l);
		}

		delete(name, value = undefined) {
			this.#list;
			if (arguments.length === 0) {
				throw missing("Failed to execute 'delete' on 'URLSearchParams'", 1, 0);
			}
			const n = toUSV(name);
			const v = value === undefined ? undefined : toUSV(value);
			const l = this.#draft();
			let o = 0;
			for (let i = 0; i < l.length; i += 2) {
				if (l[i] === n && (v === undefined || l[i + 1] === v)) {
					continue;
				}
				l[o] = l[i];
				l[o + 1] = l[i + 1];
				o += 2;
			}
			l.length = o;
			this.#commit(l);
		}

		get(name) {
			this.#list;
			if (arguments.length === 0) {
				throw missing("Failed to execute 'get' on 'URLSearchParams'", 1, 0);
			}
			const n = toUSV(name);
			const l = this.#list;
			for (let i = 0; i < l.length; i += 2) {
				if (l[i] === n) {
					return l[i + 1];
				}
			}
			return null;
		}

		getAll(name) {
			this.#list;
			if (arguments.length === 0) {
				throw missing("Failed to execute 'getAll' on 'URLSearchParams'", 1, 0);
			}
			const n = toUSV(name), r = bare([]);
			const l = this.#list;
			for (let i = 0; i < l.length; i += 2) {
				if (l[i] === n) {
					r[r.length] = l[i + 1];
				}
			}
			return setPrototypeOf(r, arrayPrototype);
		}

		has(name, value = undefined) {
			this.#list;
			if (arguments.length === 0) {
				throw missing("Failed to execute 'has' on 'URLSearchParams'", 1, 0);
			}
			const n = toUSV(name);
			const v = value === undefined ? undefined : toUSV(value);
			const l = this.#list;
			for (let i = 0; i < l.length; i += 2) {
				if (l[i] === n && (v === undefined || l[i + 1] === v)) {
					return true;
				}
			}
			return false;
		}

		set(name, value) {
			this.#list;
			if (arguments.length < 2) {
				throw missing("Failed to execute 'set' on 'URLSearchParams'", 2, arguments.length);
			}
			const n = toUSV(name), v = toUSV(value);
			const l = this.#draft();
			let found = false, o = 0;
			for (let i = 0; i < l.length; i += 2) {
				if (l[i] === n) {
					if (found) {
						continue;
					}
					found = true;
					l[i + 1] = v;
				}
				l[o] = l[i];
				l[o + 1] = l[i + 1];
				o += 2;
			}
			l.length = o;
			if (!found) {
				l[l.length] = n;
				l[l.length] = v;
			}
			this.#commit(l);
		}

		sort() {
			const l = this.#draft();
			sortPairs(l);
			this.#commit(l);
		}

		forEach(callback, thisArg = undefined) {
			this.#list;
			if (arguments.length === 0) {
				throw missing("Failed to execute 'forEach' on 'URLSearchParams'", 1, 0);
			}
			if (typeof callback !== 'function') {
				throw new TypeError("Failed to execute 'forEach' on 'URLSearchParams': parameter 1 is not of type 'Function'.");
			}
			// The list is read again at each turn: the callback may change it.
			for (let i = 0; i < this.#list.length; i += 2) {
				const l = this.#list;
				apply(callback, thisArg, [l[i + 1], l[i], this]);
			}
		}

		keys() {
			this.#list;
			return makeIterator(this, 0);
		}

		values() {
			this.#list;
			return makeIterator(this, 1);
		}

		entries() {
			this.#list;
			return makeIterator(this, 2);
		}

		toString() {
			return serialize(this.#list);
		}
	}


	// The iterators of a URLSearchParams read its list as it is when they
	// are asked for a value, so a change between two calls shows.
	class URLSearchParamsIterator {
		#sp;
		#kind;
		#i = 0;

		static {
			makeIterator = (sp, kind) => new URLSearchParamsIterator(sp, kind);
		}

		constructor(sp, kind) {
			this.#sp = sp;
			this.#kind = kind;
		}

		next() {
			const sp = this.#sp;
			const l = sp === undefined ? undefined : spList(sp);
			if (l === undefined || this.#i >= l.length) {
				this.#sp = undefined;
				return {value: undefined, done: true};
			}
			const i = this.#i;
			this.#i = i + 2;
			switch (this.#kind) {
			case 0:
				return {value: l[i], done: false};
			case 1:
				return {value: l[i + 1], done: false};
			}
			return {value: [l[i], l[i + 1]], done: false};
		}
	}

	// shape gives the members of a class the attributes WebIDL gives them
	// (enumerable, like the built-ins of a browser) and the properties
	// that remain.
	function shape(C, tag) {
		const proto = C.prototype;
		const keys = ownKeys(proto);
		for (let i = 0; i < keys.length; i++) {
			if (keys[i] !== 'constructor') {
				defineProperty(proto, keys[i], {__proto__: null, enumerable: true});
			}
		}
		const statics = ownKeys(C);
		for (let i = 0; i < statics.length; i++) {
			const k = statics[i];
			if (k !== 'length' && k !== 'name' && k !== 'prototype') {
				defineProperty(C, k, {__proto__: null, enumerable: true});
			}
		}
		defineProperty(proto, toStringTag, {__proto__: null, value: tag, writable: false, enumerable: false, configurable: true});
	}

	shape(URL, 'URL');
	shape(URLSearchParams, 'URLSearchParams');
	defineProperty(URLSearchParams.prototype, iterator, {__proto__: null, value: URLSearchParams.prototype.entries, writable: true, enumerable: false, configurable: true});

	// %URLSearchParamsIteratorPrototype% inherits from %IteratorPrototype%
	// and has no constructor: nothing makes one but entries(), keys() and
	// values().
	setPrototypeOf(URLSearchParamsIterator.prototype, iteratorPrototype);
	delete URLSearchParamsIterator.prototype.constructor;
	defineProperty(URLSearchParamsIterator.prototype, 'next', {__proto__: null, enumerable: true});
	defineProperty(URLSearchParamsIterator.prototype, toStringTag, {__proto__: null, value: 'URLSearchParams Iterator', writable: false, enumerable: false, configurable: true});

	defineProperty(g, 'URL', {__proto__: null, value: URL, writable: true, enumerable: false, configurable: true});
	defineProperty(g, 'URLSearchParams', {__proto__: null, value: URLSearchParams, writable: true, enumerable: false, configurable: true});

	// How the host recognizes a VM it has registered: keyed by the import
	// path, invisible to enumeration and JSON.
	defineProperty(g, Symbol.for('github.com/liustack/quickjs/url'), {__proto__: null, value: true});
})
