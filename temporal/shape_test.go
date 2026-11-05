// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// shapeScript describes the own properties of Temporal, its constructors,
// their prototypes and Date.prototype.toTemporalInstant, as
// testdata/chromium-shape.json has them from Chromium 156.
const shapeScript = `(() => {
	const out = {};
	const T = Temporal;
	function desc(o) {
		const r = [];
		for (const k of Reflect.ownKeys(o)) {
			const d = Object.getOwnPropertyDescriptor(o, k);
			const t = 'value' in d
				? (typeof d.value === 'function' ? 'fn:' + d.value.length + ':' + d.value.name
					: 'v:' + (typeof d.value === 'object' && d.value !== null ? 'obj' : String(d.value)))
				: 'acc:' + (d.get ? d.get.name : '') + ':' + (d.set ? 'set' : '');
			r.push([String(k), t, (d.writable ? 'w' : '') + (d.enumerable ? 'e' : '') + (d.configurable ? 'c' : '')].join(' '));
		}
		return r;
	}
	out.global = desc(globalThis).filter(x => x.startsWith('Temporal '));
	out.Temporal = desc(T);
	out.Now = desc(T.Now);
	for (const k of ['PlainDate', 'PlainTime', 'PlainDateTime', 'ZonedDateTime', 'Duration', 'Instant', 'PlainYearMonth', 'PlainMonthDay']) {
		out[k] = desc(T[k]);
		out[k + '.prototype'] = desc(T[k].prototype);
		// QuickJS prints the body of a built-in function on lines of its
		// own, V8 on one line.
		out[k + '.toString'] = Function.prototype.toString.call(T[k]).replace(/\s+/g, ' ');
	}
	out.toTemporalInstant = desc(Date.prototype).filter(x => x.startsWith('toTemporalInstant'));
	return JSON.stringify(out);
})()`

func TestShapeMatchesChromium(t *testing.T) {
	b, err := os.ReadFile("testdata/chromium-shape.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	vm := newVM(t)
	var got map[string]any
	if err := json.Unmarshal([]byte(eval(t, vm, shapeScript).(string)), &got); err != nil {
		t.Fatal(err)
	}
	for k, w := range want {
		if g := got[k]; !reflect.DeepEqual(g, w) {
			t.Errorf("%s:\n\tgot  %v\n\twant %v", k, g, w)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s: not in Chromium", k)
		}
	}
}

// TestBuiltinLike checks what the shape does not: the functions are like
// built-in ones, neither constructors nor with a prototype property, and
// the objects are what Chromium says they are.
func TestBuiltinLike(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`Object.getOwnPropertyNames(Temporal.PlainDate.prototype).filter(k => {
			const d = Object.getOwnPropertyDescriptor(Temporal.PlainDate.prototype, k);
			const f = d.value || d.get;
			return k !== 'constructor' && (f.hasOwnProperty('prototype') || !/\[native code\]/.test(Function.prototype.toString.call(f)));
		}).join()`, ""},
		{`new Temporal.PlainDate.prototype.add()`, "!TypeError"},
		{`new (Object.getOwnPropertyDescriptor(Temporal.Duration.prototype, 'years').get)()`, "!TypeError"},
		{`new Temporal.Now.instant()`, "!TypeError"},
		{`Object.prototype.toString.call(Temporal.Now)`, "[object Temporal.Now]"},
		{`Object.prototype.toString.call(Temporal.Instant.fromEpochMilliseconds(0))`, "[object Temporal.Instant]"},
		{`Object.getPrototypeOf(Temporal.PlainDate.prototype) === Object.prototype`, "true"},
		{`Object.getPrototypeOf(Temporal.PlainDate) === Function.prototype`, "true"},
		{`Temporal.PlainDate.prototype.year`, "!TypeError"},
		{`Object.getOwnPropertyDescriptor(Temporal.PlainDate.prototype, 'year').get.call({})`, "!TypeError"},
		{`Temporal.PlainDate.prototype.toString.call(Temporal.PlainTime.from('12:00'))`, "!TypeError"},
		{`Object.keys(Temporal.PlainDate.from('2024-01-05')).length`, "0"},
		{`Reflect.ownKeys(Temporal.PlainDate.from('2024-01-05')).length`, "0"},
		{`Object.isExtensible(Temporal.PlainDate.from('2024-01-05'))`, "true"},
		{`class D extends Temporal.PlainDate {}; const d = new D(2024, 1, 5); d instanceof D && d.add({days: 1}).constructor === Temporal.PlainDate`, "true"},
		{`Function.prototype.toString.call(Object.getOwnPropertyDescriptor(Temporal.PlainDate.prototype, 'year').get) ===
			Function.prototype.toString.call(Object.getOwnPropertyDescriptor(Map.prototype, 'size').get).replace('size', 'year')`, "true"},
		{`Function.prototype.toString.call(Temporal.PlainDate) === Function.prototype.toString.call(Map).replace('Map', 'PlainDate')`,
			"true"},
	})
}
