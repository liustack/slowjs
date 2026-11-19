// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
//
// loops.js: the second input of the profile (engine/pgo/train.sh). tifa's
// pages never run these paths, so the profile of the pages alone leaves them
// at a count of 0, and the compiler then treats them as never taken: it moves
// their spills into them and their blocks out of the way, and a script that
// does run them pays for it (docs/changes/generator.md). Each part below runs
// one kind of path a few thousand times, enough for a count above 0 and far
// too little to outweigh the pages.
//
// These are plain loops written for the training. None of the micro
// benchmarks of compare/testdata is in here, and none may be: train.sh runs
// those only to check that this file still reaches the paths it is for.
// When a part stops reaching its path, train.sh says so and stops.

var sink = 0;
function use(v) { sink = (sink + (typeof v === "number" ? v : 1)) | 0; }

// Typed arrays of every element kind: read and write at a variable index,
// read and write past the end, and a few methods. The interpreter reads an
// element of a typed array through rv_ta_get and writes one through
// js_set_fast_array_element_out_of_line.
var kinds = [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array,
             Float32Array, Float64Array];
if (typeof Float16Array === "function") kinds.push(Float16Array);
function taNumber(K) {
    var a = new K(128), s = 0;
    for (var r = 0; r < 40; r++) {
        for (var i = 0; i < a.length; i++) a[i] = (i * 7 + r) & 127;
        for (var i = 0; i < a.length; i++) s += a[i];
        for (var i = a.length - 1; i >= 0; i--) a[i] += 1;
        s += a[a.length] === undefined ? 1 : 0;
        a[a.length + 3] = 1;
    }
    var b = a.subarray(8, 40);
    b.set([1, 2, 3], 4);
    a.fill(3, 0, 16);
    use(s + b[5] + a.indexOf(3) + a.slice(2, 9).length);
}
function taBigInt(K) {
    var a = new K(64), s = 0n;
    for (var r = 0; r < 40; r++) {
        for (var i = 0; i < a.length; i++) a[i] = BigInt(i + r);
        for (var i = 0; i < a.length; i++) s += a[i];
    }
    use(Number(s & 0xffffn));
}
for (var k = 0; k < kinds.length; k++) taNumber(kinds[k]);
taBigInt(BigInt64Array);
taBigInt(BigUint64Array);

// DataView: every get and set, both byte orders.
(function () {
    var dv = new DataView(new ArrayBuffer(256)), s = 0;
    for (var r = 0; r < 60; r++) {
        for (var i = 0; i < 32; i++) {
            var le = (i & 1) === 0;
            dv.setInt8(i, i - 16); s += dv.getInt8(i) + dv.getUint8(i);
            dv.setInt16(i * 2, i * 300, le); s += dv.getInt16(i * 2, le) + dv.getUint16(i * 2, !le);
            dv.setInt32(i * 4, i * 70000, le); s += dv.getInt32(i * 4, le) + dv.getUint32(i * 4, le);
            dv.setFloat32(i * 4, i / 3, le); s += dv.getFloat32(i * 4, le);
            dv.setFloat64(i * 8 % 248, i / 7, le); s += dv.getFloat64(i * 8 % 248, le);
            dv.setBigInt64(i * 8 % 248, BigInt(i) - 9n, le); s += Number(dv.getBigInt64(i * 8 % 248, le));
            dv.setBigUint64(i * 8 % 248, BigInt(i), le); s += Number(dv.getBigUint64(i * 8 % 248, le));
        }
    }
    use(s);
})();

// Regular expressions on text that is not Latin-1, so the matcher runs on
// UTF-16: with and without the u flag, exec, replace, split and match.
(function () {
    var text = "", words = ["żółw", "Ελλάδα", "日本語", "naïve", "😀ok", "plain", "Straße"];
    for (var i = 0; i < 200; i++) text += words[i % words.length] + (i % 3 ? " " : ", ");
    var res = [/\p{L}+/gu, /[à-ž]+/g, /(\w+)\s(\w+)/g, /[^\x00-\x7f]{2,}/g, /^.*?語/u, /é|ß|ö/gi, /\S+?,/g];
    var n = 0;
    for (var r = 0; r < 30; r++) {
        for (var j = 0; j < res.length; j++) {
            var re = res[j], m;
            re.lastIndex = 0;
            if (re.global) { while ((m = re.exec(text)) !== null) n += m[0].length; }
            else { m = re.exec(text); n += m ? m.index : 0; }
            n += text.replace(re, "#").length;
        }
        n += text.split(/[,\s]+/u).length + (text.match(/[぀-鿿]+/g) || []).length;
    }
    use(n);
})();

// Regular expressions with a literal every match holds, which the matcher
// looks for before it runs: in Latin-1 text and in UTF-16 text, where the
// text holds it and where it does not, of letters of either case (the i
// flag).
(function () {
    var json = "", wide = "";
    for (var i = 0; i < 300; i++) {
        json += '{"id":' + i + ',"d":"\\/Date(' + i * 7919 + ')\\/","n":"item ' + i + '"},';
        wide += "日本語 item " + i + (i % 5 ? " " : " Total, ");
    }
    var res = [/(^|[^\\])"\\\/Date\((-?[0-9]+)\)\\\/"/g, /(^|[^\\])"\\\/Qngr\((-?[0-9]+)\)\\\/"/g,
        /\s(\w+)\s+total,/gi, /[a-z]+:\/\/[^\s"]+/g, /(?:id|n)":"?(\d+)x/g];
    var n = 0;
    for (var r = 0; r < 20; r++) {
        for (var j = 0; j < res.length; j++) {
            for (var t = 0; t < 2; t++) {
                var re = res[j], s = t ? wide : json, m;
                re.lastIndex = 0;
                while ((m = re.exec(s)) !== null) n += m.index;
                n += s.replace(re, "#").length + (re.test(s) ? 1 : 0);
            }
        }
    }
    use(n);
})();

// BigInt arithmetic, comparison and BigInt keys of a Map.
(function () {
    var m = new Map(), s = 0n, x = 1n;
    for (var i = 0; i < 3000; i++) {
        var b = BigInt(i) * 1000003n;
        m.set(b, i);
        x = (x * 31n + b) % 1000000007n;
        s += b >> 3n;
        if (b > x) s -= 1n;
        s ^= BigInt.asIntN(64, b << 7n);
    }
    for (var i = 0; i < 3000; i++) use(m.get(BigInt(i) * 1000003n));
    use(Number(s % 1000n) + Number(x % 1000n) + (-s).toString(16).length);
})();

// Date.parse of the formats it knows, and the date strings.
(function () {
    var forms = ["2024-03-01T12:34:56Z", "Mar 1, 2024 10:00:00 GMT+0200", "Fri, 01 Mar 2024 12:00:00 GMT",
                 "1 March 2024", "2024/3/1 8:00 PM", "Tue Jan 02 2024 00:00:00 PST", "12/31/1999 23:59:59 EST"];
    var t = 0;
    for (var r = 0; r < 300; r++)
        for (var j = 0; j < forms.length; j++) {
            var v = Date.parse(forms[j]);
            t += isNaN(v) ? 1 : v % 997;
        }
    var d = new Date(1700000000000);
    for (var r = 0; r < 200; r++) t += d.toISOString().length + d.toUTCString().length;
    use(t);
})();

// The getters of Date, local and UTC, on a few time values by turns (the
// fields of the last one are kept, so a turn finds them or works them out),
// a setter between, and the strings of a Date.
(function () {
    var ds = [new Date(1700000000000), new Date(-5e12), new Date(0), new Date(2e14)], t = 0;
    for (var r = 0; r < 600; r++) {
        var d = ds[r & 3];
        t += d.getFullYear() + d.getMonth() + d.getDate() + d.getDay() + d.getHours() + d.getMinutes();
        t += d.getUTCSeconds() + d.getUTCMilliseconds() + d.getUTCDate() + d.getUTCFullYear();
        if ((r & 31) === 0) {
            d.setUTCHours(r & 15);
            t += d.toString().length + d.toLocaleString().length + d.toDateString().length;
        }
    }
    use(t);
})();

// Numbers to text in every form, and text to numbers.
(function () {
    var s = 0;
    for (var i = 1; i < 3000; i++) {
        var x = i * 1.000173 / 7;
        s += x.toFixed(i % 8).length + x.toPrecision(1 + i % 15).length + x.toExponential(i % 12).length;
        s += String(x).length + (i * 12345).toString(36).length + (-x).toString(2).length;
        s += parseFloat("  " + x + "e" + (i % 9)) > 0 ? 1 : 0;
        s += parseInt("0x" + i.toString(16)) + Number(" " + i + " ") + (+("1e" + (i % 20)) > 1 ? 1 : 0);
    }
    use(s);
})();

// Strings: case, search, slices, padding.
(function () {
    var s = 0, acc = "";
    for (var i = 0; i < 3000; i++) {
        var w = "Word" + i + "mixedCASE";
        s += w.toUpperCase().length + w.toLowerCase().charCodeAt(2) + w.indexOf("CASE") + w.lastIndexOf("d");
        acc += w.slice(1, 4);
        if (acc.length > 2000) acc = "";
        s += w.padStart(20, "-").length + w.trim().length + (w.startsWith("Wo") ? 1 : 0);
    }
    use(s + acc.length);
})();

// Objects and arrays: delete, a shorter length, pop, slice, holes, assign.
(function () {
    var s = 0;
    for (var r = 0; r < 300; r++) {
        var o = {};
        for (var i = 0; i < 20; i++) o["k" + i] = i;
        for (var i = 0; i < 20; i += 2) delete o["k" + i];
        for (var k in o) s += o[k];
        var a = [];
        for (var i = 0; i < 64; i++) a.push(i);
        a.length = 40;
        a[50] = 1;
        while (a.length > 10) s += a.pop() | 0;
        s += a.slice(2, 8).length + a.concat([1, 2]).length;
        var h = [1, , 3];
        h.length = 1;
        s += h.length;
        var o2 = Object.assign({}, o);
        s += Object.keys(o2).length;
    }
    use(s);
})();

// Arithmetic and comparison of values of mixed types, the slow paths of
// the operators.
(function () {
    var vals = [1, 2.5, -0, NaN, "3", "x", true, null, undefined, 1n << 40n, {}, [], 2147483647, -2147483648];
    var s = 0;
    for (var r = 0; r < 200; r++)
        for (var i = 0; i < vals.length; i++)
            for (var j = 0; j < vals.length; j++) {
                var a = vals[i], b = vals[j];
                if (typeof a === "bigint" || typeof b === "bigint") {
                    s += a == b ? 1 : 0;
                    s += a < b ? 1 : 0;
                    continue;
                }
                s += (a | b) + (a & 7) + (a ^ b) + (a << 3) + (a >>> 1) + (a % 3 || 0);
                s += a * 1.5 - b > 0 ? 1 : 0;
                s += a === b ? 1 : 0;
                s += a == b ? 2 : 0;
            }
    use(s);
})();

// Map and Set: set, get and delete with object, string and number keys.
(function () {
    var m = new Map(), st = new Set(), objs = [], s = 0;
    for (var i = 0; i < 200; i++) objs.push({ i: i });
    for (var r = 0; r < 15; r++) {
        for (var i = 0; i < 200; i++) { m.set(objs[i], i); m.set("s" + i, i); m.set(i * 1.5, i); st.add(i); }
        for (var i = 0; i < 200; i += 2) { m.delete(objs[i]); m.delete("s" + i); m.delete(i * 1.5); st.delete(i); }
        s += m.size + st.size;
        m.clear(); st.clear();
    }
    use(s);
})();

// Loop forms the pages hardly use (do-while, for-of over an array and a
// string), a loop of float arithmetic, building a string with + and +=,
// object spread, and Date.parse with milliseconds.
(function () {
    var s = 0, i = 0, x = 0.5, y = 1.25;
    do { s += i & 3; i++; } while (i < 30000);
    i = 30000;
    do { i--; } while (i > 0);
    var arr = [];
    for (var k = 0; k < 500; k++) arr.push(k * 0.5);
    for (var r = 0; r < 40; r++) for (var v of arr) s += v;
    for (var ch of "plain text and ünïcode") s += ch.length;
    for (var k = 0; k < 20000; k++) { x = x * 1.0001 + y / 3; y = y - x * 0.0001; if (x > 1e6) x = 0.5; }
    var str = "";
    for (var k = 0; k < 4000; k++) { str += "a" + k; str = str + "b"; if (str.length > 8000) str = `${str.length}`; }
    var o = { a: 1, b: 2, c: 3, d: 4, e: 5 };
    for (var k = 0; k < 2000; k++) {
        var c = { ...o, f: k };
        delete c.b;
        var c2 = { ...c };
        s += c2.f + Object.keys(c2).length;
    }
    for (var k = 0; k < 2000; k++) s += Date.parse("2024-03-0" + (1 + k % 9) + "T12:34:56." + (100 + k % 900) + "Z") % 7;
    use(s + x + str.length);
})();

// Floats added, subtracted and multiplied in every form the interpreter has
// for them: both operands in registers, a constant operand, the result into a
// local, and an int with a float. Numbers that are not both ints take a path
// of their own in those instructions, which tifa's pages run too little to
// train it.
(function () {
    var a = [], b = [], n = 200, s = 0.5, t = 1.5, u = 0.25, w = 0.75;
    for (var i = 0; i < n; i++) { a.push(i * 0.25 + 0.5); b.push(1.5 - i * 0.125); }
    for (var r = 0; r < 25; r++) {
        for (var i = 1; i < n - 1; i++) {
            var x = a[i], y = b[i];
            u = x + y;
            w = x - y;
            u = u + 0.25;
            w = w - 0.75;
            s = s + x * y - w * 1e-3;
            t = t - (y - 0.5) * (x + 1.25);
            a[i] = (a[i - 1] + a[i + 1]) * 0.5 - u * 1e-4 + i;
            b[i] = y * 0.999 - t * 1e-9 + (x - i) * 2;
        }
        if (s > 1e12 || s < -1e12) s = 0.5;
    }
    use(s + t + u + w);
})();

// Typed arrays of every kind and a plain array read and written at constant
// indexes (t[0], t[1], ...), which the interpreter runs through other
// instructions than a variable index.
(function () {
    var all = kinds.map(function (K) { var a = new K(8); for (var i = 0; i < 8; i++) a[i] = i; return a; });
    all.push([0, 1, 2, 3, 4, 5, 6, 7]);
    var s = 0;
    for (var k = 0; k < all.length; k++) {
        var t = all[k];
        for (var j = 0; j < 400; j++) {
            s += t[0] + t[1] + t[2] + t[3] + t[4] + t[5] + t[6] + t[7];
            t[0] = j & 7; t[3] = 1; t[5] = t[2]; t[7] = 2;
        }
    }
    use(s);
})();

// Arrays that are no longer fast arrays: one written past its end and one
// with an element deleted keep their elements as properties of their shape.
// The interpreter reads such an element through rv_getel_index (also a read
// past the end of a fast array, which finds nothing on the chain) and
// writes one it has through rv_putel_index.
(function () {
    var s = 0;
    for (var r = 0; r < 40; r++) {
        var a = [];
        for (var i = 0; i < 48; i++) a.push(i);
        a[100] = r;
        for (var i = 0; i < 48; i++) a[i] = a[i] + 1;
        for (var i = 0; i < 48; i++) s += a[i];
        var b = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16];
        delete b[5];
        for (var j = 0; j < 64; j++) {
            b[j & 15] = j;
            s += b[(j + 3) & 15] | 0;
        }
        var c = [1, 2, 3];
        for (var j = 0; j < 8; j++) s += c[j] === undefined ? 1 : c[j];
    }
    use(s);
})();

// instanceof of a plain function, which the interpreter answers in
// rv_instof: a hit at the first and the second step of the chain, a miss
// that walks to its end, a miss on an object with no prototype, and a left
// side that is not an object.
(function () {
    function A() {}
    function B() {}
    B.prototype = Object.create(A.prototype);
    function C() {}
    var vals = [new A(), new B(), {}, [], new C(), Object.create(null), 1, "x"];
    var s = 0;
    for (var r = 0; r < 600; r++)
        for (var i = 0; i < vals.length; i++) {
            var v = vals[i];
            if (v instanceof A) s += 1;
            if (v instanceof B) s += 2;
            if (v instanceof C) s += 4;
        }
    use(s);
})();

// Functions that pass their arguments on with f.apply(this, arguments),
// sloppy and strict, with 0 to 5 arguments, a constructor that passes its
// arguments to an init method that way, and arguments.length and
// arguments[i] read in a sloppy function.
(function () {
    function add3(a, b, c) { return (a | 0) + (b | 0) + (c | 0); }
    function fwd() { return add3.apply(this, arguments); }
    function fwdS() { "use strict"; return add3.apply(this, arguments); }
    function Point() { this.init.apply(this, arguments); }
    Point.prototype.init = function (x, y) { this.x = x; this.y = y | 0; };
    function count() {
        var n = 0;
        for (var i = 0; i < arguments.length; i++) n += arguments[i];
        return n;
    }
    var s = 0;
    for (var k = 0; k < 3000; k++) {
        s += fwd() + fwd(k) + fwd(k, 1) + fwd(k, 1, 2) + fwdS(k, 2, 3, 4) + fwdS(k, 1, 2, 3, 4);
        var p = new Point(k, k & 7);
        s += p.x + p.y + count(k, 1, 2) + count();
    }
    use(s);
})();
sink;
