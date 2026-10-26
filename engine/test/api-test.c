/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * Tests of the API the C tree changes, on real runtimes: JS_DumpMemoryUsage,
 * JS_NewClassID, Atomics, abort, and snprintf given a format it does not do.
 * api-test.sh builds and runs it.
 */
#include <stdio.h>
#include <stdlib.h>
#include <signal.h>
#include <string.h>
#include <sys/wait.h>
#include <unistd.h>

#include "host.h"

static int failures;
static long checks;

#define CHECK(cond, ...) do { \
        checks++; \
        if (!(cond)) { \
            if (failures++ < 20) { \
                fprintf(stderr, "api-test: %s:%d: ", __FILE__, __LINE__); \
                fprintf(stderr, __VA_ARGS__); \
                fputc('\n', stderr); \
            } \
        } \
    } while (0)

static JSContext *new_context(JSRuntime *rt)
{
    JSContext *ctx = JS_NewContext(rt);
    if (!ctx) {
        fprintf(stderr, "api-test: JS_NewContext failed\n");
        exit(2);
    }
    return ctx;
}

static void eval_ok(JSContext *ctx, const char *code)
{
    JSValue v = JS_Eval(ctx, code, strlen(code), "<api-test>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(v)) {
        js_std_dump_error(ctx);
        exit(2);
    }
    JS_FreeValue(ctx, v);
}

/* The report of a runtime that holds objects: the same text at every size, the
   length when there is no buffer, and a cut at every length from nothing to
   past the end, with a canary after the cut. */
static void test_dump(void)
{
    JSRuntime *rt = host_new_runtime();
    JSContext *ctx = new_context(rt);
    JSMemoryUsage s;
    size_t n, n_null;
    char *full, *cut;

    eval_ok(ctx,
            "globalThis.keep = [];"
            "for (let i = 0; i < 200; i++) keep.push({ i, s: 'item' + i, a: [i, i + 1], f() { return i; } });"
            "keep.push(new Map([[1, 2]]), new Set([1]), /re/g, new Date(0), new Error('x'), Promise.resolve(1),"
            "          new Uint8Array(10), Symbol('s'), 10n ** 30n);");
    JS_ComputeMemoryUsage(rt, &s);

    n = JS_DumpMemoryUsage(NULL, 0, &s, rt);
    CHECK(n > 500, "the report is too short: %zu", n);
    full = malloc(n + 1);
    CHECK(JS_DumpMemoryUsage(full, n + 1, &s, rt) == n, "length with a buffer differs from the length without");
    CHECK(strlen(full) == n, "report length %zu, text length %zu", n, strlen(full));
    CHECK(strstr(full, "QuickJS memory usage") == full, "the report does not start as it should");
    CHECK(strstr(full, "JSObject classes") != NULL, "no class counts: the part that names classes did not run");
    CHECK(strstr(full, "Array") && strstr(full, "Map") && strstr(full, "RegExp"), "class names are missing");
    CHECK(strstr(full, "objects") && strstr(full, "shapes") && strstr(full, "bytecode"), "a table row is missing");
    CHECK(strchr(full, '%') == NULL, "a conversion was left in the text");

    /* Cut to every size from 0 to past the end. The text before the cut is the
       start of the full report, a 0 ends it, and every byte after what was
       written, to the end of the buffer, is untouched. */
    cut = malloc(n + 16);
    for (size_t size = 0; size <= n + 3; size++) {
        size_t r;
        memset(cut, 0x5a, n + 16);
        r = JS_DumpMemoryUsage(cut, size, &s, rt);
        CHECK(r == n, "size %zu: length %zu, want %zu", size, r, n);
        {
            /* size - 1 characters and the 0, or the whole report and the 0 */
            size_t written = size == 0 ? 0 : size - 1 < n ? size : n + 1;
            size_t i;

            if (size) {
                size_t keep = written - 1;
                CHECK(!memcmp(cut, full, keep), "size %zu: text differs from the full report", size);
                CHECK(cut[keep] == 0, "size %zu: no 0 after the text", size);
            }
            for (i = written; i < n + 16; i++)
                if ((unsigned char)cut[i] != 0x5a)
                    break;
            CHECK(i == n + 16, "size %zu: the byte %zu past the %zu written was changed", size, i - written, written);
        }
    }

    /* Without a runtime there are no class counts and no object sizes */
    n_null = JS_DumpMemoryUsage(NULL, 0, &s, NULL);
    CHECK(n_null > 100 && n_null < n, "report without a runtime: %zu (with: %zu)", n_null, n);

    free(cut);
    free(full);
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
}

/* The one decimal of the averages is what printf's %.1f gives for the same
   double. The report is made from numbers set here, with no runtime, and its
   text is compared with the C library's, for the values that round the wrong
   way when a number is rounded twice (1.05 is a little over 1.05 as a double,
   0.15 and 0.05 a little under, and 1.25 is halfway). */
static void test_tenths(void)
{
    /* sprintf, and not snprintf: the snprintf of the program is the engine's,
       which does no floating point, and sprintf is the system's. */
    static const struct {
        int64_t num, den;
        const char *want;
    } known[] = {
        { 21, 20, "1.1" }, { 3, 20, "0.1" }, { 1, 20, "0.1" }, { 49, 20, "2.5" }, { 5, 4, "1.2" },
        { 1, 8, "0.1" }, { 3, 8, "0.4" }, { 5, 8, "0.6" }, { 7, 8, "0.9" }, { 0, 1, "0.0" },
    };
    char text[1024];
    JSMemoryUsage s;
    int64_t num, den;
    size_t i;
    long compared = 0;

    for (i = 0; i < sizeof known / sizeof *known; i++) {
        char want[32];
        const char *p, *q;
        size_t len;

        memset(&s, 0, sizeof s);
        s.atom_count = known[i].den;
        s.atom_size = known[i].num;
        JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
        p = strstr(text, "  (");
        q = strstr(text, " per atom)");
        len = p && q && q > p + 3 ? (size_t)(q - p - 3) : 0;
        sprintf(want, "%.1f", (double)known[i].num / known[i].den);
        CHECK(!strcmp(want, known[i].want), "the C library gives %s for %lld/%lld, not %s", want, (long long)known[i].num,
              (long long)known[i].den, known[i].want);
        CHECK(len == strlen(want) && !strncmp(p + 3, want, len), "%lld/%lld: the report has [%.*s], printf %s",
              (long long)known[i].num, (long long)known[i].den, (int)len, p ? p + 3 : "", want);
    }
    /* many more, with the atoms line and with the line of memory slack, whose
       numbers go below 0 */
    for (den = 1; den <= 70; den++) {
        for (num = -300; num <= 2500; num++) {
            char want[32];
            const char *p, *q;
            size_t len;

            memset(&s, 0, sizeof s);
            s.malloc_count = 1;
            s.malloc_size = num;
            s.memory_used_count = den;
            JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
            p = strstr(text, "overhead, ");
            q = strstr(text, " average slack)");
            len = p && q && q > p + 10 ? (size_t)(q - p - 10) : 0;
            sprintf(want, "%.1f", (double)num / den);
            compared++;
            if (len != strlen(want) || strncmp(p + 10, want, len)) {
                CHECK(0, "%lld/%lld: the report has [%.*s], printf %s", (long long)num, (long long)den, (int)len, p ? p + 10 : "", want);
            }
            if (num < 0)
                continue;
            memset(&s, 0, sizeof s);
            s.atom_count = den;
            s.atom_size = num;
            JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
            p = strstr(text, "  (");
            q = strstr(text, " per atom)");
            len = p && q && q > p + 3 ? (size_t)(q - p - 3) : 0;
            compared++;
            if (len != strlen(want) || strncmp(p + 3, want, len)) {
                CHECK(0, "%lld/%lld: the atoms line has [%.*s], printf %s", (long long)num, (long long)den, (int)len, p ? p + 3 : "", want);
            }
        }
    }
    CHECK(compared > 190000, "only %ld values were compared", compared);
    /* a value printf would write with many digits, or that is not a number, has a word */
    memset(&s, 0, sizeof s);
    s.malloc_count = 1;
    JS_DumpMemoryUsage(text, sizeof text, &s, NULL);
    CHECK(strstr(text, "nan average slack)") != NULL, "0/0 is not written as nan");
}

/* The checks of Atomics: nothing waits, so wait throws and notify wakes nobody.
   They are what the six test262 tests excluded for the probe of safeBroadcast
   (Atomics.wait on a value that differs) say that still holds: notify with no
   waiter gives 0, and wait converts its arguments, in the order of the
   specification (the array and the shared buffer, the index, the value, the
   timeout), and throws what a conversion throws before it throws that it cannot
   block. */
static void test_atomics(JSContext *ctx)
{
    static const char code[] =
        "(function () {\n"
        "  const failures = [];\n"
        "  const sab = new SharedArrayBuffer(16);\n"
        "  const i32 = new Int32Array(sab), b64 = new BigInt64Array(sab);\n"
        "  const plain = new Int32Array(4);\n"
        "  let log = [];\n"
        "  const mark = (name, v) => ({ valueOf() { log.push(name); return v; } });\n"
        "  const poison = (name) => ({ valueOf() { log.push(name); throw new RangeError('poison ' + name); } });\n"
        "  const run = (f) => { try { return 'returned ' + String(f()); } catch (e) { return e.constructor.name + ': ' + e.message; } };\n"
        "  const expect = (what, got, want, wantLog) => {\n"
        "    if (got !== want) failures.push(what + ': got [' + got + '], want [' + want + ']');\n"
        "    if (wantLog !== undefined && log.join() !== wantLog) failures.push(what + ': converted [' + log + '], want [' + wantLog + ']');\n"
        "    log = [];\n"
        "  };\n"
        "  const noBlock = 'TypeError: cannot block in this thread';\n"
        "  const notNoBlock = (what, got, wantLog = '') => {\n"
        "    if (got === noBlock || !/^(TypeError|RangeError)/.test(got)) failures.push(what + ': got [' + got + ']');\n"
        "    if (log.join() !== wantLog) failures.push(what + ': converted [' + log + '], want [' + wantLog + ']');\n"
        "    log = [];\n"
        "  };\n"
        /* wait: the can-block error is the last, and only for arguments that are good */
        "  expect('wait', run(() => Atomics.wait(i32, 0, 0, 0)), noBlock);\n"
        "  expect('wait, a value that differs', run(() => Atomics.wait(i32, 0, 1, 0)), noBlock);\n"
        "  expect('wait, BigInt64Array', run(() => Atomics.wait(b64, 0, 0n, 0)), noBlock);\n"
        "  expect('wait, no timeout', run(() => Atomics.wait(i32, 0, 0)), noBlock);\n"
        "  expect('wait, order', run(() => Atomics.wait(i32, mark('index', 0), mark('value', 0), mark('timeout', 0))), noBlock, 'index,value,timeout');\n"
        /* a buffer that is not shared, and an index out of range, come before any conversion of value and timeout */
        "  notNoBlock('wait, not shared', run(() => Atomics.wait(plain, 0, mark('value', 0), poison('timeout'))));\n"
        "  notNoBlock('wait, index out of range', run(() => Atomics.wait(i32, 4, mark('value', 0), poison('timeout'))));\n"
        /* a Symbol, and an object whose valueOf throws, in each place */
        "  notNoBlock('wait, Symbol index', run(() => Atomics.wait(i32, Symbol('i'), 0, 0)));\n"
        "  notNoBlock('wait, Symbol value', run(() => Atomics.wait(i32, 0, Symbol('v'), 0)));\n"
        "  notNoBlock('wait, Symbol timeout', run(() => Atomics.wait(i32, 0, 0, Symbol('t'))));\n"
        "  expect('wait, poisoned timeout', run(() => Atomics.wait(i32, 0, 0, poison('timeout'))), 'RangeError: poison timeout', 'timeout');\n"
        "  expect('wait, poisoned value', run(() => Atomics.wait(i32, 0, poison('value'), mark('timeout', 0))), 'RangeError: poison value', 'value');\n"
        "  expect('wait, poisoned index', run(() => Atomics.wait(i32, poison('index'), mark('value', 0), mark('timeout', 0))), 'RangeError: poison index', 'index');\n"
        /* notify: nobody waits, so it is 0, after its own conversions */
        "  expect('notify', run(() => Atomics.notify(i32, 0, 1)), 'returned 0');\n"
        "  expect('notify, no count', run(() => Atomics.notify(i32, 0)), 'returned 0');\n"
        "  expect('notify, infinity', run(() => Atomics.notify(i32, 0, Infinity)), 'returned 0');\n"
        "  expect('notify, negative count', run(() => Atomics.notify(i32, 0, -3)), 'returned 0');\n"
        "  expect('notify, BigInt64Array', run(() => Atomics.notify(b64, 1, 5)), 'returned 0');\n"
        "  expect('notify, not shared', run(() => Atomics.notify(plain, 0, 1)), 'returned 0');\n"
        "  expect('notify, order', run(() => Atomics.notify(i32, mark('index', 0), mark('count', 1))), 'returned 0', 'index,count');\n"
        "  expect('notify, poisoned count', run(() => Atomics.notify(i32, 0, poison('count'))), 'RangeError: poison count', 'count');\n"
        "  expect('notify, poisoned count, not shared', run(() => Atomics.notify(plain, 0, poison('count'))), 'RangeError: poison count', 'count');\n"
        "  notNoBlock('notify, Symbol index', run(() => Atomics.notify(i32, Symbol('i'), 1)));\n"
        "  notNoBlock('notify, index out of range', run(() => Atomics.notify(i32, 4, 1)));\n"
        "  return failures.join('\\n');\n"
        "})()";
    JSValue v = JS_Eval(ctx, code, sizeof code - 1, "<atomics>", JS_EVAL_TYPE_GLOBAL);
    const char *text;

    if (JS_IsException(v)) {
        js_std_dump_error(ctx);
        exit(2);
    }
    text = JS_ToCString(ctx, v);
    CHECK(text && !*text, "Atomics:\n%s", text ? text : "(no text)");
    JS_FreeCString(ctx, text);
    JS_FreeValue(ctx, v);
}

/* A class ID of a runtime is the next one it has not given out. */
static void test_class_ids(void)
{
    JSRuntime *rt1 = host_new_runtime(), *rt2 = host_new_runtime();
    JSContext *c1 = new_context(rt1), *c2 = new_context(rt2);
    JSClassID a1 = 0, a2 = 0, b1 = 0, b2 = 0, again, preset, after;
    JSClassDef def = { "Thing" };
    JSValue o;
    JSValue builtin[8];
    int i;

    /* two runtimes taking IDs by turns count on their own */
    JS_NewClassID(rt1, &a1);
    JS_NewClassID(rt2, &b1);
    JS_NewClassID(rt1, &a2);
    JS_NewClassID(rt2, &b2);
    CHECK(a1 != 0 && b1 != 0, "no ID given");
    CHECK(a1 == b1, "the first IDs of two runtimes differ: %u %u", a1, b1);
    CHECK(a2 == a1 + 1 && b2 == b1 + 1, "IDs do not count up by one: %u %u, %u %u", a1, a2, b1, b2);

    /* asking again for the same variable gives the same ID and takes no new one */
    again = JS_NewClassID(rt1, &a1);
    CHECK(again == a1, "asking again gave %u, not %u", again, a1);
    {
        JSClassID next = 0;
        JS_NewClassID(rt1, &next);
        CHECK(next == a2 + 1, "the next ID is %u, want %u", next, a2 + 1);
    }

    /* an ID set by the caller is kept, and the runtime counts on from it */
    preset = a2 + 40;
    again = JS_NewClassID(rt2, &preset);
    CHECK(again == a2 + 40 && preset == a2 + 40, "a preset ID changed");
    after = 0;
    JS_NewClassID(rt2, &after);
    CHECK(after == a2 + 41, "after a preset ID the next is %u, want %u", after, a2 + 41);
    /* a smaller preset ID does not move the count back */
    preset = a1;
    JS_NewClassID(rt2, &preset);
    after = 0;
    JS_NewClassID(rt2, &after);
    CHECK(after == a2 + 42, "after a small preset ID the next is %u, want %u", after, a2 + 42);

    /* the built-in classes keep their numbers, which are below every ID given */
    builtin[0] = JS_NewObject(c1);
    builtin[1] = JS_NewArray(c1);
    builtin[2] = JS_Eval(c1, "(function(){})", 14, "<t>", 0);
    builtin[3] = JS_Eval(c1, "new Error('e')", 14, "<t>", 0);
    builtin[4] = JS_Eval(c1, "new Date(0)", 11, "<t>", 0);
    builtin[5] = JS_Eval(c1, "new Map()", 9, "<t>", 0);
    builtin[6] = JS_Eval(c1, "Promise.resolve(1)", 18, "<t>", 0);
    builtin[7] = JS_Eval(c1, "/x/", 3, "<t>", 0);
    for (i = 0; i < 8; i++) {
        JSClassID id = JS_GetClassID(builtin[i]);
        CHECK(id != 0 && id < a1, "built-in object %d has class ID %u, not below %u", i, id, a1);
        JS_FreeValue(c1, builtin[i]);
    }

    /* a class made with the ID works in its runtime, and is not in the other */
    CHECK(!JS_IsRegisteredClass(rt1, a1), "registered before JS_NewClass");
    CHECK(JS_NewClass(rt1, a1, &def) == 0, "JS_NewClass failed");
    CHECK(JS_IsRegisteredClass(rt1, a1) && !JS_IsRegisteredClass(rt2, a1), "class registered in the wrong runtime");
    o = JS_NewObjectClass(c1, a1);
    CHECK(!JS_IsException(o) && JS_GetClassID(o) == a1, "an object of the new class");
    JS_FreeValue(c1, o);
    /* the other runtime uses the same number for another class */
    {
        JSClassDef def2 = { "Other" };
        CHECK(JS_NewClass(rt2, b1, &def2) == 0 && b1 == a1, "the same number in the other runtime");
        o = JS_NewObjectClass(c2, b1);
        CHECK(!JS_IsException(o) && JS_GetClassID(o) == a1, "an object of the other class");
        JS_FreeValue(c2, o);
    }

    JS_FreeContext(c1);
    JS_FreeContext(c2);
    JS_FreeRuntime(rt1);
    JS_FreeRuntime(rt2);
}

/* Runs fn in a child process with its standard error in a pipe. The child must
   die of SIGABRT, which is how the host ends the process, after the host has
   said that it heard the abort. */
static void check_abort(const char *what, void (*fn)(void))
{
    int fds[2];
    pid_t pid;
    int status;
    char text[64] = "";
    ssize_t n;

    if (pipe(fds)) {
        perror("pipe");
        exit(2);
    }
    fflush(stderr);
    pid = fork();
    if (pid == 0) {
        dup2(fds[1], 2);
        close(fds[0]);
        close(fds[1]);
        fn();
        _exit(0);
    }
    close(fds[1]);
    n = read(fds[0], text, sizeof text - 1);
    close(fds[0]);
    waitpid(pid, &status, 0);
    CHECK(WIFSIGNALED(status) && WTERMSIG(status) == SIGABRT, "%s ended the process with status %d", what, status);
    CHECK(n > 0 && strstr(text, "engine abort"), "%s: the host did not hear the abort: [%s]", what, text);
}

static void call_abort(void)
{
    abort();
}

/* snprintf here is the one of engine/libc, which the link puts before the
   system's, and a format it does not do must reach the host as an abort. */
static const char *volatile bad_format;

static void call_snprintf(void)
{
    char buf[16];

    snprintf(buf, sizeof buf, bad_format);
}

static void test_abort(void)
{
    static const char *bad[] = { "%f", "%ls", "%n", "%1$d", "%", "%99999999999d" };
    size_t i;

    check_abort("abort", call_abort);
    for (i = 0; i < sizeof bad / sizeof *bad; i++) {
        bad_format = bad[i];
        check_abort(bad[i], call_snprintf);
    }
}

int main(void)
{
    JSRuntime *rt;
    JSContext *ctx;

    test_dump();
    test_tenths();
    test_class_ids();
    rt = host_new_runtime();
    ctx = new_context(rt);
    test_atomics(ctx);
    JS_FreeContext(ctx);
    JS_FreeRuntime(rt);
    test_abort();
    printf("api-test: %ld checks, %d failures\n", checks, failures);
    return failures != 0;
}
