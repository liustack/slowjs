// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"os"
	"testing"
)

// shapeScript describes the properties of the classes and their prototypes:
// whether each is data or an accessor, its attributes, and the length and
// name of its function. testdata/shape.txt holds what WebIDL says they are
// (https://webidl.spec.whatwg.org/#es-interfaces): the members of an
// interface are enumerable, @@iterator and @@toStringTag are not.
const shapeScript = `(function () {
	var out = '';
	function name(k) { return typeof k === 'symbol' ? 'Symbol(' + k.description + ')' : k; }
	function dump(label, o) {
		var keys = Reflect.ownKeys(o);
		out += label + ' [' + keys.map(name).join(' ') + ']\n';
		keys.forEach(function (k) {
			var d = Object.getOwnPropertyDescriptor(o, k), s = '  ' + label + '.' + name(k) + ':';
			if ('value' in d) {
				s += ' data ' + (d.writable ? 'W' : '-') + (d.enumerable ? 'E' : '-') + (d.configurable ? 'C' : '-');
				if (typeof d.value === 'function') s += ' fn ' + d.value.length + ' ' + JSON.stringify(d.value.name);
				else s += ' ' + typeof d.value + (typeof d.value === 'string' ? ' ' + d.value : '');
			} else {
				s += ' accessor ' + (d.enumerable ? 'E' : '-') + (d.configurable ? 'C' : '-');
				if (d.get) s += ' get ' + d.get.length + ' ' + JSON.stringify(d.get.name);
				if (d.set) s += ' set ' + d.set.length + ' ' + JSON.stringify(d.set.name);
			}
			out += s + '\n';
		});
	}
	dump('URL', URL);
	dump('URL.prototype', URL.prototype);
	dump('URLSearchParams', URLSearchParams);
	dump('URLSearchParams.prototype', URLSearchParams.prototype);
	var it = Object.getPrototypeOf(new URLSearchParams().entries());
	dump('Iterator.prototype', it);
	out += 'iteratorProto ' + (Object.getPrototypeOf(it) === Object.getPrototypeOf(Object.getPrototypeOf([][Symbol.iterator]()))) + '\n';
	out += 'iterator is entries ' + (URLSearchParams.prototype[Symbol.iterator] === URLSearchParams.prototype.entries) + '\n';
	var g = Object.getOwnPropertyDescriptor(globalThis, 'URL');
	out += 'global URL ' + (g.writable ? 'W' : '-') + (g.enumerable ? 'E' : '-') + (g.configurable ? 'C' : '-') + '\n';
	g = Object.getOwnPropertyDescriptor(globalThis, 'URLSearchParams');
	out += 'global URLSearchParams ' + (g.writable ? 'W' : '-') + (g.enumerable ? 'E' : '-') + (g.configurable ? 'C' : '-') + '\n';
	var u = new URL('http://a/');
	out += 'own keys of a URL [' + Reflect.ownKeys(u).length + '] of a URLSearchParams [' + Reflect.ownKeys(new URLSearchParams()).length + ']\n';
	out += 'keys ' + Object.keys(URL.prototype).length + '\n';
	out += 'frozen ' + Object.isFrozen(URL.prototype) + '\n';
	return out;
})()`

func TestShape(t *testing.T) {
	got := evalString(t, newVM(t), shapeScript)
	if os.Getenv("UPDATE_SHAPE") != "" {
		os.WriteFile("testdata/shape.txt", []byte(got), 0o644)
	}
	if want := readFile(t, "testdata/shape.txt"); got != want {
		t.Fatalf("shape\n%s\nwant\n%s", got, want)
	}
}
