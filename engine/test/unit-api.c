/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit-api checks through the public API what scripts cannot see: what the
   runtime holds after errors and their stacks, what JS_ParseJSON does with
   bytes that are not UTF-8, and a property a host puts on a proxy. They are
   the tests of the Go packages (lib/backtrace*, lib/jsonstr, errorstack,
   lib/shareddebug in unit-white) that did this on the old engine.

     unit-api */
#include "cutils.h"
#include "testutil.h"

/* usage runs the collector and returns the memory usage of the runtime. */
static JSMemoryUsage usage(VM *v)
{
    JSMemoryUsage u;

    JS_RunGC(v->rt);
    JS_ComputeMemoryUsage(v->rt, &u);
    return u;
}

/* ---- pending stacks and memory ---- */

/* Reading the stacks of errors kept alive must not give each error a shape of
   its own: the formatted stack is a plain data property, as when the stack
   was formatted at once. The second variant gives each error another property
   while its stack is pending. */
static void test_backtrace_shares_shape(void)
{
    static const struct { const char *src; int n; } cases[] = {
        { "var held\n"
          "function make() { held = []; for (var i = 0; i < 10000; i++) held.push(new Error(\"kept\")) }\n"
          "function read() { for (var e of held) void e.stack }\n", 10000 },
        { "var held\n"
          "function make() { held = []; for (var i = 0; i < 1000; i++) { var e = new Error(\"kept\"); e.code = i; held.push(e) } }\n"
          "function read() { for (var e of held) void e.stack }\n", 1000 },
    };
    int i;

    for (i = 0; i < 2; i++) {
        VM v = vm_new();
        JSMemoryUsage pending, read;

        eval_ok(v.ctx, cases[i].src);
        JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
        pending = usage(&v);
        JS_FreeValue(v.ctx, call_global(v.ctx, "read"));
        read = usage(&v);
        CHECK(read.shape_count <= pending.shape_count + 2, "reading %d stacks added %lld shapes", cases[i].n,
              (long long)(read.shape_count - pending.shape_count));
        vm_free(&v);
    }
}

/* Replacing an error's prototype must not keep the old prototype, and the
   object graph behind it, alive while the stack is pending. */
static void test_backtrace_old_prototype(void)
{
    VM v = vm_new();
    JSMemoryUsage u;

    eval_ok(v.ctx,
            "var kept\n"
            "function make() {\n"
            "\tfunction Target() {}\n"
            "\tTarget.prototype = { payload: new ArrayBuffer(4 * 1024 * 1024) }\n"
            "\tvar e = Reflect.construct(Error, [\"retained prototype\"], Target)\n"
            "\tObject.setPrototypeOf(e, null)\n"
            "\tkept = e\n"
            "}\n");
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    u = usage(&v);
    /* the 4 MiB ArrayBuffer of the old prototype must already be gone */
    CHECK(u.memory_used_size <= 2 << 20, "a pending error keeps %lld bytes, the old prototype is retained", (long long)u.memory_used_size);
    vm_free(&v);
}

/* Printing an error reads its stack from C. That read formats the stack once
   and keeps the text, like a read from JS: the second read gives the same
   string. */
static void test_backtrace_native_read_caches(void)
{
    VM v = vm_new();
    JSAtom a;
    JSValue e, s1, s2;

    eval_ok(v.ctx, "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\nfunction make() { return deep(40) }\n");
    e = call_global(v.ctx, "make");
    a = JS_NewAtom(v.ctx, "stack");
    s1 = JS_GetProperty(v.ctx, e, a);
    s2 = JS_GetProperty(v.ctx, e, a);
    CHECK(JS_IsString(s1) && JS_IsString(s2), "the stack is not a string");
    CHECK(JS_VALUE_GET_PTR(s1) == JS_VALUE_GET_PTR(s2), "each read formatted the stack again");
    JS_FreeValue(v.ctx, s1);
    JS_FreeValue(v.ctx, s2);
    JS_FreeAtom(v.ctx, a);
    JS_FreeValue(v.ctx, e);
    vm_free(&v);
}

/* Errors whose stack is never read are freed, with the functions in their
   stack, including when an error and a closure from its own stack reference
   each other. */
static void test_error_stack_memory(void)
{
    VM v = vm_new();
    JSMemoryUsage base, held, freed;

    eval_ok(v.ctx,
            "function deep(n, f) { return n == 0 ? f() : deep(n - 1, f) }\n"
            "var kept\n"
            "function make(n) {\n"
            "\tkept = []\n"
            "\tfor (var i = 0; i < n; i++) {\n"
            "\t\tkept.push(deep(20, function () {\n"
            "\t\t\t// a function whose bytecode only the error keeps\n"
            "\t\t\tvar err = new Function(\"return new Error('e')\")()\n"
            "\t\t\t// the closure holds the error, and the error holds the closure's frame\n"
            "\t\t\terr.self = function () { return err }\n"
            "\t\t\treturn err\n"
            "\t\t}))\n"
            "\t}\n"
            "\treturn kept.length\n"
            "}\n"
            "function drop() { kept = undefined }\n"
            "function make1() { make(1) }\n"
            "function make2000() { make(2000) }\n");
    /* One round first, so that what the first call leaves behind for good
       (shapes, caches) is part of the baseline. */
    JS_FreeValue(v.ctx, call_global(v.ctx, "make1"));
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    base = usage(&v);
    JS_FreeValue(v.ctx, call_global(v.ctx, "make2000"));
    held = usage(&v);
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    freed = usage(&v);
    CHECK(held.obj_count - base.obj_count >= 2000, "the errors were not kept: %lld objects", (long long)(held.obj_count - base.obj_count));
    CHECK(freed.obj_count == base.obj_count, "%lld objects left after freeing the errors", (long long)(freed.obj_count - base.obj_count));
    /* the allocator may keep a little memory for reuse */
    CHECK(freed.malloc_size - base.malloc_size <= 64 << 10, "%lld bytes left after freeing the errors", (long long)(freed.malloc_size - base.malloc_size));
    vm_free(&v);
}

/* An error made while a script runs, and kept after it ends, does not keep the
   code of the script alive. Libraries make such errors when they load (React
   keeps one to signal suspense). The size of the functions kept after a
   script is the same whether the script keeps an object or an error. */
static int64_t kept_functions(const char *make)
{
    VM v = vm_new();
    JSMemoryUsage before, after;
    char *script = malloc(200 * 120 + 400);
    size_t len = 0;
    int i;
    int64_t r;

    before = usage(&v);
    len += sprintf(script, "(function load() {\n");
    for (i = 0; i < 200; i++)
        len += sprintf(script + len, "\tfunction f%d(x) { return [x, %d, 'padding to make the code larger'] }\n", i, i);
    len += sprintf(script + len, "\tfunction make() { return %s }\n\tglobalThis.sentinel = make()\n})()\n", make);
    eval_ok(v.ctx, script);
    after = usage(&v);
    r = after.js_func_size + after.js_func_code_size + after.js_func_pc2line_size -
        before.js_func_size - before.js_func_code_size - before.js_func_pc2line_size;
    free(script);
    vm_free(&v);
    return r;
}

static void test_error_stack_top_level(void)
{
    int64_t object = kept_functions("{ message: \"sentinel\" }");
    int64_t err = kept_functions("new Error(\"sentinel\")");

    CHECK(err == object, "keeping an error made by the script keeps %lld bytes of functions, want %lld", (long long)err, (long long)object);
}

/* ---- a property of the host on a proxy ---- */

/* A property the host puts on the proxy itself, with
   JS_SetPropertyFunctionList, is an own property of the proxy: lookups of it,
   full or of the attributes only, find it before the handler and never call a
   trap. Revoking the proxy changes nothing. */
static void test_proxy_own_property(void)
{
    int revoke;

    for (revoke = 0; revoke < 2; revoke++) {
        VM v = vm_new();
        JSValue p;
        static const JSCFunctionListEntry list[] = {
            JS_PROP_STRING_DEF("stack", "host value", JS_PROP_C_W_E),
        };

        eval_ok(v.ctx,
                "var log = []\n"
                "var r = Proxy.revocable({}, { getOwnPropertyDescriptor(t, k) { log.push(k); return undefined } })\n"
                "var p = r.proxy");
        p = JS_Eval(v.ctx, "p", 1, "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_SetPropertyFunctionList(v.ctx, p, list, 1) >= 0, "JS_SetPropertyFunctionList");
        JS_FreeValue(v.ctx, p);
        if (revoke)
            eval_ok(v.ctx, "r.revoke()");
        CHECK(eval_is(v.ctx,
                      "JSON.stringify([\n"
                      "\tObject.getOwnPropertyDescriptor(p, \"stack\").value,\n"
                      "\t(() => { try { return Object.prototype.propertyIsEnumerable.call(p, \"stack\") } catch (e) { return String(e) } })(),\n"
                      "\tlog])",
                      "[\"host value\",true,[]]"),
              "revoked %d", revoke);
        vm_free(&v);
    }
}

/* ---- JSON.parse of bytes that are not UTF-8 ---- */

/* JSON.parse of UTF-8 that is not valid gives the code units of the string, or
   the error and where. A string of javascript always converts to valid UTF-8
   (a lone surrogate to its three bytes), but the C API's JS_ParseJSON may get
   any bytes: each kind of byte a UTF-8 reader treats apart, valid or not. */
static void test_json_bytes(void)
{
    static const struct { const char *text; size_t len; const char *want; } cases[] = {
#define C(s, w) { s, sizeof(s) - 1, w }
        C("\"a" "\xc3\xa9" "b\"", "3: 61 e9 62"),
        C("\"\xc2\x80\xdf\xbf\"", "2: 80 7ff"),
        C("\"\xc0\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xc1\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe0\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe0\xa0\x80\"", "1: 800"),
        C("\"\xed\xa0\x80" "x\"", "2: d800 78"),
        C("\"\xed\xbf\xbf\"", "1: dfff"),
        C("\"\xef\xbf\xbf\"", "1: ffff"),
        C("\"\xf0\x8f\xbf\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf0\x90\x80\x80\"", "2: d800 dc00"),
        C("\"\xf4\x8f\xbf\xbf\"", "2: dbff dfff"),
        C("\"\xf4\x90\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf5\x80\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf7\xbf\xbf\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xf8\x88\x80\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xfc\x84\x80\x80\x80\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xfe\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xff\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\x80\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xbf\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xc3\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xc3", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe4\xb8\"", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe4\xb8", "SyntaxError: Bad UTF-8 sequence at 1:2"),
        C("\"\xe4\xb8\xad" "q\xe4\"", "SyntaxError: Bad UTF-8 sequence at 1:4"),
        C("\"\xc3\xa9\xc3\"", "SyntaxError: Bad UTF-8 sequence at 1:3"),
        C("\"\xc3\xa9\\n\xe4\xb8\xad\"", "3: e9 a 4e2d"),
#undef C
    };
    VM v = vm_new();
    JSValue g;
    size_t i;
    char *text = malloc(400), want[1024];
    int k;

    eval_ok(v.ctx, "globalThis.hex = function (s) { var o = []; for (var i = 0; i < s.length; i++) o.push(s.charCodeAt(i).toString(16)); return s.length + ': ' + o.join(' '); }; 0");
    g = JS_GetGlobalObject(v.ctx);
    for (i = 0; i < countof(cases) + 3; i++) {
        const char *src;
        size_t len;
        JSValue r;
        char *got;

        want[0] = 0;
        if (i < countof(cases)) {
            src = cases[i].text;
            len = cases[i].len;
            snprintf(want, sizeof(want), "%s", cases[i].want);
        } else if (i == countof(cases)) {
            /* 70 characters of 3 bytes and a lone lead byte */
            len = 0;
            text[len++] = '"';
            for (k = 0; k < 70; k++) { memcpy(text + len, "\xe4\xb8\xad", 3); len += 3; }
            memcpy(text + len, "\xc3\"", 2); len += 2;
            src = text;
            snprintf(want, sizeof(want), "SyntaxError: Bad UTF-8 sequence at 1:72");
        } else if (i == countof(cases) + 1) {
            /* 40 characters of 4 bytes */
            len = 0;
            text[len++] = '"';
            for (k = 0; k < 40; k++) { memcpy(text + len, "\xf0\x9f\x98\x80", 4); len += 4; }
            text[len++] = '"';
            src = text;
            strcpy(want, "80:");
            for (k = 0; k < 40; k++)
                strcat(want, " d83d de00");
        } else {
            /* 50 times a and a two byte character, then a byte that is never UTF-8 */
            len = 0;
            text[len++] = '"';
            for (k = 0; k < 50; k++) { memcpy(text + len, "a\xc3\xa9", 3); len += 3; }
            memcpy(text + len, "\xff\"", 2); len += 2;
            src = text;
            snprintf(want, sizeof(want), "SyntaxError: Bad UTF-8 sequence at 1:102");
        }
        /* JS_ParseJSON wants a zero after the text */
        if (src == text)
            text[len] = 0;
        r = JS_ParseJSON(v.ctx, src, len, "<bytes>");
        if (JS_IsException(r)) {
            JS_FreeValue(v.ctx, r);
            r = JS_GetException(v.ctx);
            JS_SetPropertyStr(v.ctx, g, "v", r);
            got = eval_text(v.ctx, "v.name + ': ' + v.message + ' at ' + v.lineNumber + ':' + v.columnNumber");
        } else {
            JS_SetPropertyStr(v.ctx, g, "v", r);
            got = eval_text(v.ctx, "hex(v)");
        }
        CHECK(got && strcmp(got, want) == 0, "case %zu: got %s, want %s", i, got ? got : "an exception", want);
        free(got);
    }
    JS_FreeValue(v.ctx, g);
    free(text);
    vm_free(&v);
}

int main(void)
{
    test_backtrace_shares_shape();
    test_backtrace_old_prototype();
    test_backtrace_native_read_caches();
    test_error_stack_memory();
    test_error_stack_top_level();
    test_proxy_own_property();
    test_json_bytes();
    if (failures) {
        printf("unit-api: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-api: %ld checks passed\n", checks);
    return 0;
}
