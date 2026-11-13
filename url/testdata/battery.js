// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
//
// What the tamper tests ask of URL and URLSearchParams, and answer as one
// string. It builds the string with + and reads arrays by index only, so that
// a page that has changed the built-ins the tests change does not change the
// battery itself.

(function (gop, apply, ots, URL, URLSearchParams) {
	var out = '';
	function say(label, f) {
		var v;
		try {
			v = f();
			if (typeof v === 'object' && v !== null && 'length' in v && typeof v !== 'string') {
				var s = '[';
				for (var i = 0; i < v.length; i++) {
					s += (i ? ',' : '') + v[i];
				}
				v = s + ']';
			}
		} catch (e) {
			v = 'threw ' + e.name;
		}
		out += label + '=' + v + '\n';
	}
	say('basic', function () { return new URL('/a b/c?x=1&y=2#h', 'http://user:pw@Example.com:8080/p/q').href; });
	say('parts', function () { var u = new URL('https://a.b/c?d#e'); return u.protocol + '|' + u.host + '|' + u.pathname + '|' + u.search + '|' + u.hash + '|' + u.origin; });
	say('idna', function () { return new URL('http://тест.рф/').host; });
	say('relative', function () { return new URL('../x', 'http://a/b/c/d').href; });
	say('invalid', function () { return new URL('nope'); });
	say('noargs', function () { return new URL(); });
	say('canParse', function () { return URL.canParse('a:b') + '' + URL.canParse('b') + URL.canParse('b', 'a:c'); });
	say('parse', function () { return URL.parse('x') === null ? 'null' : 'url'; });
	say('setters', function () {
		var u = new URL('http://a/b?c#d');
		u.protocol = 'https'; u.username = 'u u'; u.password = 'p:p'; u.host = 'h.com:99'; u.pathname = '/x y'; u.search = 'q=1 2'; u.hash = 'h h';
		return u.href;
	});
	say('port', function () { var u = new URL('http://a:1/'); u.port = '80'; return u.href + u.port; });
	say('sp-get', function () { var p = new URLSearchParams('a=1&b=2&a=3'); var all = p.getAll('a'); return p.get('a') + all[0] + all[1] + all.length + p.has('b') + p.has('c') + p.size; });
	say('sp-record', function () { var p = new URLSearchParams({x: '1', y: '2'}); return p.toString(); });
	say('sp-seq', function () { var p = new URLSearchParams([['x', '1'], ['y', '2']]); return p.toString(); });
	say('sp-seq-bad', function () { return new URLSearchParams([['x']]); });
	say('sp-copy', function () { return new URLSearchParams(new URLSearchParams('a=b&c=d')).toString(); });
	say('sp-mutate', function () {
		var p = new URLSearchParams('a=1&b=2&a=3');
		p.append('c', 'x y'); p.set('a', '9'); p['delete']('b'); p.sort();
		return p.toString();
	});
	say('sp-sort', function () { var p = new URLSearchParams('z=1&b=2&a=3&b=1&😀=x&￿=y'); p.sort(); return p.toString(); });
	say('sp-iter', function () {
		var p = new URLSearchParams('a=1&b=2'), s = '';
		for (var e of p) { s += e[0] + e[1] + ';'; }
		for (var k of p.keys()) { s += k; }
		for (var v of p.values()) { s += v; }
		p.forEach(function (v, k) { s += k + v; });
		return s;
	});
	say('link', function () {
		var u = new URL('http://a/?x=1'), p = u.searchParams;
		p.append('y', '2'); var one = u.search;
		u.search = '?z=3'; var two = p.toString() + p.size;
		u.href = 'http://b/?w=4'; var three = p.toString() + (p === u.searchParams);
		p['delete']('w'); var four = u.href;
		return one + '|' + two + '|' + three + '|' + four;
	});
	say('toString', function () { var u = new URL('http://a/'); return u.toString() + u.toJSON() + ('' + u); });
	say('brand', function () { return apply(gop(URL.prototype, "href").get, {}, []); });
	say('brand2', function () { return apply(URLSearchParams.prototype.get, new URL('http://a/'), ['x']); });
	say('subclass', function () { var C = class extends URL { get twice() { return this.href + this.href; } }; var u = new C('http://a/'); return u.twice + (u instanceof URL) + (u instanceof C) + u.searchParams.size; });
	say('tag', function () { return apply(ots, new URL('http://a/'), []) + apply(ots, new URLSearchParams(), []) + apply(ots, new URLSearchParams().entries(), []); });
	return out;
})
