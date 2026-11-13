// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"testing"

	"github.com/liustack/quickjs"
)

// TestStateIsUnreachable checks that no script reaches the record of a URL or
// the Builtins of url.js, so none can hand set parts of its own: the record is
// in a private field of the URL class, the Builtins are arguments of the
// function url.js is, and neither is a property of anything.
//
// It walks every object a script reaches from the global object, a URL and
// what it makes, through every own property, getter, setter and prototype,
// and finds neither. It then puts getters, setters and functions that note
// what they see on what url.js could use, Array.prototype and
// Object.prototype, Symbol.iterator, the methods of String.prototype, and runs
// every member of both classes: none is called. Last it reads the state of a
// URL through a subclass, Reflect, a Proxy, a receiver that is not a URL and a
// value that changes the URL while it is converted.
func TestStateIsUnreachable(t *testing.T) {
	vm := newVM(t)
	run := func(js string) {
		t.Helper()
		if got := evalString(t, vm, js); got != "ok" {
			t.Error(got)
		}
	}
	run(`var u = new URL("http://u:p@h:8/p?a=1#f"), sp = u.searchParams, it = sp.entries();
		var seen = new Set(), bad = [];
		function walk(root, path) {
			const todo = [[root, path]];
			while (todo.length > 0) {
				const [o, at] = todo.pop();
				if ((typeof o !== 'object' && typeof o !== 'function') || o === null || seen.has(o)) {
					continue;
				}
				seen.add(o);
				if (Array.isArray(o) && o.length === 12 && o[0] === u.href) {
					bad.push('the record of u at ' + at);
				}
				if (typeof o === 'function' && o !== JSON.parse &&
						((o.name === 'set' && o.length === 11) || (o.name === 'parse' && o.length === 2) ||
						(o.name === 'canParse' && o.length === 2) || o.name === 'formDecode' || o.name === 'formEncode')) {
					bad.push('the Builtin ' + o.name + ' at ' + at);
				}
				todo.push([Object.getPrototypeOf(o), at + '.__proto__']);
				for (const k of Reflect.ownKeys(o)) {
					const d = Object.getOwnPropertyDescriptor(o, k);
					const name = at + '.' + String(k);
					todo.push([d.value, name], [d.get, name + '.get'], [d.set, name + '.set']);
				}
			}
		}
		walk(globalThis, 'globalThis');
		walk(u, 'u');
		walk(sp, 'sp');
		walk(it, 'it');
		bad.length === 0 && seen.has(URL.prototype) && seen.has(Object.getPrototypeOf(it)) ? 'ok' : bad.join('\n') || 'the walk missed the classes'`)

	// The test notes in a string, and walks its lists with index loops, so that
	// what it puts on the prototypes does not see the test itself.
	run(`var noted = '';
		function note(what) { return function () { noted += what + '; '; }; }
		const keys = ['length', 'href', 'protocol', 'then'];
		const methods = ['indexOf', 'slice', 'toWellFormed', 'concat', 'split', 'replace', 'charCodeAt', 'codePointAt', 'substring', 'toLowerCase', 'startsWith', 'includes', 'at'];
		const strings = {};
		const arrayIterator = Array.prototype[Symbol.iterator];
		for (let i = 0; i < 16; i++) {
			Object.defineProperty(Array.prototype, i, {get: note('Array.prototype[' + i + '] get'), set: note('Array.prototype[' + i + '] set'), configurable: true});
			Object.defineProperty(Object.prototype, i, {get: note('Object.prototype[' + i + '] get'), set: note('Object.prototype[' + i + '] set'), configurable: true});
		}
		for (let i = 0; i < keys.length; i++) {
			Object.defineProperty(Object.prototype, keys[i], {get: note('Object.prototype.' + keys[i] + ' get'), set: note('Object.prototype.' + keys[i] + ' set'), configurable: true});
		}
		Array.prototype[Symbol.iterator] = note('Array.prototype[Symbol.iterator]');
		for (let i = 0; i < methods.length; i++) {
			strings[methods[i]] = String.prototype[methods[i]];
			String.prototype[methods[i]] = note('String.prototype.' + methods[i]);
		}
		let out = '';
		const v = new URL("http://u:p@h:8/p?a=1&b=%41+x#f");
		v.protocol = "https"; v.username = "x"; v.password = "y"; v.host = "g:9"; v.hostname = "e";
		v.port = "10"; v.pathname = "/q"; v.search = "c=3&c=4"; v.hash = "z";
		const s = v.searchParams;
		s.append("d", "5"); s.set("c", "6"); s.delete("d"); s.sort();
		out += v.href + ',' + v.origin + ',' + v.protocol + ',' + v.username + ',' + v.password + ',' + v.host + ',' +
			v.hostname + ',' + v.port + ',' + v.pathname + ',' + v.search + ',' + v.hash + ',' + v.toString() + ',' +
			v.toJSON() + ',' + s.toString() + ',' + s.get("c") + ',' + s.has("a") + ',' + s.size + ',' +
			URL.canParse("x:") + ',' + URL.parse("y:").href;
		s.forEach(function (value, name) { out += ',' + name + '=' + value; });
		for (let e = s.entries(), r = e.next(); !r.done; r = e.next()) { out += ',' + typeof r.value; }
		out += ',' + new URLSearchParams("a=1&b=2").toString();
		v.href = "foo://h/";
		out += ',' + v.href;
		for (let i = 0; i < methods.length; i++) {
			String.prototype[methods[i]] = strings[methods[i]];
		}
		Array.prototype[Symbol.iterator] = arrayIterator;
		for (let i = 0; i < 16; i++) {
			delete Array.prototype[i];
			delete Object.prototype[i];
		}
		for (let i = 0; i < keys.length; i++) {
			delete Object.prototype[keys[i]];
		}
		const want = "https://x:y@e:10/q?c=6#z,https://e:10,https:,x,y,e:10,e,10,/q,?c=6,#z,https://x:y@e:10/q?c=6#z,https://x:y@e:10/q?c=6#z,c=6,6,false,1,true,y:,c=6,object,a=1&b=2,foo://h/";
		noted !== '' ? 'called: ' + noted : out === want ? 'ok' : 'got ' + out`)

	run(`{ const res = [];
		function attempt(f) {
			try {
				return String(f());
			} catch (e) {
				return e.name;
			}
		}
		const desc = (k) => Object.getOwnPropertyDescriptor(URL.prototype, k);
		const plain = new URL("http://h/p");
		class Sub extends URL {
			#r = 'not the record';
			get href() { return 'forged'; }
			peek() { return this.#r; }
		}
		const s = new Sub("http://h/p");
		s.pathname = "/q";
		res.push(s.peek(), s.href, desc('href').get.call(s), s.toString(), s instanceof URL);
		res.push(attempt(() => desc('href').get.call({})));
		res.push(attempt(() => desc('pathname').set.call({}, "/x")));
		res.push(attempt(() => Reflect.get(URL.prototype, 'href', {})));
		res.push(attempt(() => Reflect.set(URL.prototype, 'pathname', '/x', Object.create(URL.prototype))));
		res.push(attempt(() => Object.create(URL.prototype).href));
		res.push(attempt(() => new Proxy(plain, {}).href));
		res.push(attempt(() => { new Proxy(plain, {}).pathname = "/x"; }));
		res.push(attempt(() => Reflect.apply(desc('search').set, new Proxy(plain, {}), ['?q'])));
		const r = Reflect.construct(URL, ["http://r/"], Object);
		res.push(Object.getPrototypeOf(r) === Object.prototype, desc('href').get.call(r));
		res.push(plain.href);
		const w = new URL("http://h/p?a");
		w.pathname = {toString() { w.hostname = "other"; w.protocol = "ws"; w.search = "b"; return "/changed"; }};
		res.push(w.href, w.searchParams.toString());
		w.search = {toString() { w.href = "http://x/?a"; return "c"; }};
		res.push(w.href, w.searchParams.toString());
		res.push(attempt(() => { w.hash = {toString() { throw new RangeError("no"); }}; }), w.href);
		res.join(' ') === 'not the record forged http://h/q http://h/q true TypeError TypeError TypeError TypeError TypeError TypeError TypeError TypeError true http://r/ http://h/p ws://other/changed?b b= http://x/?c c= RangeError http://x/?c' ? 'ok' : res.join(' ') }`)

	// A VM is one realm: a URL of one VM is not a value of another.
	other, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := Register(other); err != nil {
		t.Fatal(err)
	}
	u, err := vm.EvalValue(`new URL("http://h/")`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer u.Free()
	get, err := other.EvalValue(`(u) => Object.getOwnPropertyDescriptor(URL.prototype, 'href').get.call(u)`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Free()
	if got, err := get.Call(quickjs.UndefinedValue, u); err == nil {
		t.Errorf("a URL of another VM: %v, want an error", got)
	}
}
