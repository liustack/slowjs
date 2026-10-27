/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
#ifndef SLOWJS_TEST_COMPILE_SCRIPTS_H
#define SLOWJS_TEST_COMPILE_SCRIPTS_H

/* The scripts the tests of compiling with little memory use (unit-oom.c and
   unit-white.c): they run the parts of the compiler that move or rewrite
   code. Every name is static, so a program takes what it uses. */
#include <string.h>

#ifndef countof
#define countof(x) (sizeof(x) / sizeof((x)[0]))
#endif

/* compile_scripts use the parts of the compiler that move or rewrite code:
   loops, labels, scopes, closures, classes, regexps, templates, async code
   and eval. */
static const char *compile_scripts[] = {
    "class A { #p = 1; get p() { return this.#p } static s = [1,2,3].map(x => x * 2) }; const a = new A(); JSON.stringify({p: a.p, s: A.s})",
    "function f(a, {b, c = 2}, ...r) { let s = 0; for (const x of r) s += x; return a + b + c + s } f(1, {b: 2}, 3, 4, 5)",
    "const re = /(\\d+)-(\\w+)/g; 'a 12-ab 34-cd'.replace(re, (m, d, w) => w + d) + new RegExp('x(y)+', 'i').test('XYY')",
    "const t = (s, ...v) => s.raw.join('|') + v.join(); t`a${1}b${2}c` + `x${[1,2].join()}`",
    "(async () => { const r = await Promise.all([1, 2, Promise.resolve(3)]); return r.reduce((a, b) => a + b) })().then(v => globalThis.out = v)",
    "const m = new Map([[1, 'a']]); const s = new Set([1, 2]); let o = {...{x: 1}, y: [...s]}; try { null.x } catch { o.z = m.get(1) } JSON.parse(JSON.stringify(o))",
    "function* g() { let i = 0; while (i < 5) yield i++ } [...g()].map(String).join() + Array.from({length: 3}, (_, i) => i * i)",
    "eval('var q = 1; q + 1') + new Function('a', 'b', 'return a * b')(3, 4)",
    "with ({w: 1}) { w + 1 }",
    "label: for (let i = 0; i < 3; i++) { for (let j = 0; j < 3; j++) { if (j == 1) continue label; if (i == 2) break label } } typeof undefinedThing",
    "for (const k in {a: 1, b: 2}) { for (var i = 0, n = 0; i < 2; i++) n += i } try { undefinedThing } catch (e) { e.name }",
    /* Scopes in scopes, which grow the function's scope array: made by
       nested_scopes, as the Go test made it. */
    NULL,
};

/* nested_scopes makes the last script of compile_scripts: eight scopes in
   scopes, each a block, a loop, a try, a catch and a with. */
static char nested_script[2048];

static void nested_scopes(void)
{
    int i;

    strcpy(nested_script, "function f() { ");
    for (i = 0; i < 8; i++)
        strcat(nested_script, "{ let a = 1; for (let i = 0; i < 1; i++) { try { a++ } catch (e) { with ({}) { ");
    for (i = 0; i < 8; i++)
        strcat(nested_script, "} } } } ");
    strcat(nested_script, "} f()");
    compile_scripts[countof(compile_scripts) - 1] = nested_script;
}

/* compile_fine_scripts run out of memory where a sweep in steps of 32 bytes
   of memory left misses it. */
static const char *const compile_fine_scripts[] = {
    /* A class constructor with fields and a parameter with a default. When
       the code of the fields failed, opening the scope of the parameters
       failed without an exception. */
    "class A { x = 1; constructor(a = 1) { this.a = a } }; new A().a",
    /* A derived class constructor with a parameter with a default. When
       writing the constant pool index of the constructor failed, the parser
       put it in place at the end of the code anyway, after the bytes the
       buffer holds. */
    "class B { constructor() {} }; class A extends B { constructor(a = 1) { super(); this.a = a } }; new A().a",
    /* When adding an atom to the atom table of the bytecode failed, the
       writer wrote on and gave the bytecode, with the error left pending. */
    "for (var [a = 1] of [[]]) ; a",
};

#endif
