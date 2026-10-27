/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit-white checks what the public API cannot reach: the static functions of
   the JSON runs, which it feeds buffers of every room, the marker of a pending
   stack, and the lists the collector keeps. The engine it runs on is white.c,
   which is quickjs.c with the functions below added (build.sh links it in
   place of quickjs.o). They are the tests of lib/jsonstr, lib/backtrace* and
   lib/younggc that did this on the old engine.

     unit-white */
#include "testutil.h"

struct StringBuffer;
struct StringBuffer *white_sb_new(JSContext *ctx, int size, int wide, int pre);
void white_sb_free(JSContext *ctx, struct StringBuffer *b);
int white_sb_units(const struct StringBuffer *b, uint16_t *out, int cap);
int white_sb_error(const struct StringBuffer *b);
void white_sb_fake_length(struct StringBuffer *b, int len, int size);
int white_string_len_max(void);
const uint8_t *white_json_string_run(struct StringBuffer *b, const uint8_t *p, const uint8_t *end, int sep);
int white_json_quote_run(struct StringBuffer *b, JSValueConst s, int *pi);
uintptr_t white_json_run_limit(uintptr_t p, uintptr_t end);
int white_stack_pending(JSContext *ctx, JSValueConst obj);
int white_backtrace_class_has_handlers(JSRuntime *rt);
int white_pending_needs_data_property(JSContext *ctx, JSValueConst err);
int64_t white_backtrace_frame_size(void);
void white_gc_lists(JSRuntime *rt, int *old, int *young, int *bad);
int white_error_nest(JSRuntime *rt, int n);
JSValue white_bare_function(JSContext *ctx, JSValueConst proto);
int white_function_set_properties(JSContext *ctx, JSValueConst f, JSAtom name, int len);
int white_own_keys(JSContext *ctx, JSValueConst obj, int flags, int got, char *out, size_t cap);
int white_op(int i);
int white_op_count(void);
int white_poll_run(void);
void white_poll_skip(int on);
const char *white_poll_scan(const uint8_t *code, int len);
int white_poll_insert(JSContext *ctx, const uint8_t *code, int len, uint8_t *out, int cap);
int white_code_shape(const uint8_t *code, int len, int *shape, int max);
const uint8_t *white_child_code(JSValueConst fun, int i, int *len);
int white_nops(const uint8_t *code, int len, int *places, int max);
int white_resolve_var(JSContext *ctx, int kind, int in_arrow, size_t room, int *pending, int *made);
int white_eval_vars(JSContext *ctx, int in_arrow, size_t room, int *pending, int *made);

/* ---- texts of every kind of character ---- */

/* A text is code points; text_utf8 writes it as UTF-8 and text_utf16 as the
   code units a string of the engine holds. */
typedef struct {
    uint32_t cp[700];
    int n;
} Text;

static void text_add(Text *t, uint32_t c, int times)
{
    while (times-- > 0)
        t->cp[t->n++] = c;
}

static size_t text_utf8(const Text *t, uint8_t *out)
{
    size_t len = 0;
    int i;

    for (i = 0; i < t->n; i++) {
        uint32_t c = t->cp[i];

        if (c < 0x80) {
            out[len++] = c;
        } else if (c < 0x800) {
            out[len++] = 0xc0 | (c >> 6);
            out[len++] = 0x80 | (c & 63);
        } else if (c < 0x10000) {
            out[len++] = 0xe0 | (c >> 12);
            out[len++] = 0x80 | ((c >> 6) & 63);
            out[len++] = 0x80 | (c & 63);
        } else {
            out[len++] = 0xf0 | (c >> 18);
            out[len++] = 0x80 | ((c >> 12) & 63);
            out[len++] = 0x80 | ((c >> 6) & 63);
            out[len++] = 0x80 | (c & 63);
        }
    }
    return len;
}

static int text_utf16(const Text *t, uint16_t *out)
{
    int len = 0, i;

    for (i = 0; i < t->n; i++) {
        uint32_t c = t->cp[i];

        if (c >= 0x10000) {
            out[len++] = 0xd800 + ((c - 0x10000) >> 10);
            out[len++] = 0xdc00 + (c & 0x3ff);
        } else {
            out[len++] = c;
        }
    }
    return len;
}

#define A 'a'
#define B 'b'
#define E_ACUTE 0xe9
#define HAN 0x6f22
#define EMOJI 0x1f600

/* the strings of the run tests: ASCII, Latin-1, wide characters and a
   surrogate pair, at the lengths around the room of the buffers */
static const int lengths[] = { 0, 1, 2, 7, 8, 9, 15, 16, 17, 30, 31, 32, 33, 34, 47, 48, 49, 63, 64, 65, 66, 100, 300 };

static void run_text(Text *t, int length_index, int kind)
{
    int n = lengths[length_index];

    t->n = 0;
    switch (kind) {
    case 0: text_add(t, A, n); break;
    case 1: text_add(t, E_ACUTE, n); break;
    case 2: text_add(t, A, 1); text_add(t, B, 1); text_add(t, HAN, n); break;
    case 3: text_add(t, A, n); text_add(t, E_ACUTE, 1); break;
    case 4: text_add(t, A, n); text_add(t, HAN, 1); text_add(t, B, n); break;
    default: text_add(t, A, n); text_add(t, EMOJI, 1); break;
    }
}

static const int sizes[] = { 1, 2, 3, 8, 16, 31, 32, 33, 64 };

/* ---- the runs of JSON ---- */

/* Every run of JSON.parse, into a buffer of every room and kind, leaves the
   characters of the string in it as string_buffer_putc would, stops at the
   quote and never fills the buffer past its room. */
static void test_json_string_run(void)
{
    VM v = vm_new();
    size_t si, li;
    int pi, wide, kind;
    static uint8_t src[4000];
    static uint16_t want[1000], got[1000];

    for (si = 0; si < countof(sizes); si++) {
        int size = sizes[si];
        int pres[4] = { 0, 1, size - 1, size };

        for (pi = 0; pi < 4; pi++) {
            for (wide = 0; wide < 2; wide++) {
                for (li = 0; li < countof(lengths); li++) {
                    for (kind = 0; kind < 6; kind++) {
                        Text t;
                        struct StringBuffer *b;
                        size_t n8;
                        int nw, ng, k;
                        const uint8_t *r;

                        run_text(&t, li, kind);
                        n8 = text_utf8(&t, src);
                        src[n8] = '"';
                        b = white_sb_new(v.ctx, size, wide, pres[pi]);
                        CHECK(b != NULL, "no buffer");
                        if (!b)
                            continue;
                        r = white_json_string_run(b, src, src + n8 + 1, '"');
                        CHECK(r == src + n8, "size %d, %d before, wide %d, kind %d, %d: stops %ld bytes on, want %zu", size, pres[pi], wide, kind, lengths[li],
                              r ? (long)(r - src) : -1L, n8);
                        for (k = 0; k < pres[pi]; k++)
                            want[k] = 'x';
                        nw = pres[pi] + text_utf16(&t, want + pres[pi]);
                        ng = white_sb_units(b, got, 1000);
                        CHECK(ng == nw && memcmp(got, want, nw * sizeof(uint16_t)) == 0,
                              "size %d, %d before, wide %d, kind %d, %d: holds %d units, want %d", size, pres[pi], wide, kind, lengths[li], ng, nw);
                        white_sb_free(v.ctx, b);
                    }
                }
            }
        }
    }
    vm_free(&v);
}

/* Every run of JSON.stringify, from a string of 8 or 16 bits into a buffer of
   every room and kind, leaves the characters up to the first one to escape in
   it and never fills the buffer past its room. */
static void test_json_quote_run(void)
{
    static const char *const tails[] = { "", "\"z", "\n" };
    VM v = vm_new();
    size_t si, li, ti;
    int pi, wide, kind;
    static uint8_t src[4000];
    static uint16_t want[1000], got[1000];

    for (si = 0; si < countof(sizes); si++) {
        int size = sizes[si];
        int pres[4] = { 0, 1, size - 1, size };

        for (pi = 0; pi < 4; pi++) {
            for (wide = 0; wide < 2; wide++) {
                for (li = 0; li < countof(lengths); li++) {
                    for (kind = 0; kind < 6; kind++) {
                        for (ti = 0; ti < countof(tails); ti++) {
                            Text t, kept;
                            struct StringBuffer *b;
                            size_t n8;
                            JSValue str;
                            int i = 0, nw, ng, k;

                            run_text(&t, li, kind);
                            n8 = text_utf8(&t, src);
                            memcpy(src + n8, tails[ti], strlen(tails[ti]));
                            str = JS_NewStringLen(v.ctx, (const char *)src, n8 + strlen(tails[ti]));
                            CHECK(JS_IsString(str), "no string");
                            b = white_sb_new(v.ctx, size, wide, pres[pi]);
                            if (!b || !JS_IsString(str)) {
                                CHECK(0, "no buffer");
                                continue;
                            }
                            CHECK(white_json_quote_run(b, str, &i) == 0, "size %d, %d before, wide %d, kind %d: the run failed", size, pres[pi], wide, kind);
                            /* the run stops at the surrogate pair, which the loop writes */
                            kept = t;
                            for (k = 0; k < kept.n; k++)
                                if (kept.cp[k] == EMOJI) {
                                    kept.n = k;
                                    break;
                                }
                            for (k = 0; k < pres[pi]; k++)
                                want[k] = 'x';
                            nw = pres[pi] + text_utf16(&kept, want + pres[pi]);
                            CHECK(i == nw - pres[pi], "size %d, %d before, wide %d, kind %d, %d, tail %zu: stops at %d, want %d", size, pres[pi], wide, kind, lengths[li], ti, i, nw - pres[pi]);
                            ng = white_sb_units(b, got, 1000);
                            CHECK(ng == nw && memcmp(got, want, nw * sizeof(uint16_t)) == 0, "size %d, %d before, wide %d, kind %d, %d: holds %d units, want %d", size, pres[pi], wide, kind, lengths[li], ng, nw);
                            white_sb_free(v.ctx, b);
                            JS_FreeValue(v.ctx, str);
                        }
                    }
                }
            }
        }
    }
    vm_free(&v);
}

/* The blocks fill_memory took, to give back. */
static struct {
    void **p;
    size_t n, cap;
} taken;

static void fill_memory(VM *v)
{
    size_t size;

    JS_RunGC(v->rt);
    JS_SetMemoryLimit(v->rt, JS_GetMemorySize(v->rt));
    for (size = 4096; size >= 8; size -= 8) {
        for (;;) {
            void *p = js_malloc_rt(v->rt, size);

            if (!p)
                break;
            if (taken.n == taken.cap) {
                taken.cap = taken.cap ? taken.cap * 2 : 1024;
                taken.p = realloc(taken.p, taken.cap * sizeof(void *));
            }
            taken.p[taken.n++] = p;
        }
    }
}

static void release_memory(VM *v)
{
    while (taken.n)
        js_free_rt(v->rt, taken.p[--taken.n]);
    unlimit(v);
}

/* A run that needs more memory than is left fails as string_buffer_putc does,
   with the out of memory error: one that grows an 8-bit buffer, and a first
   wide character that widens a buffer with room left, for both runs. */
static void test_json_runs_oom(void)
{
    static const struct { const char *name; uint32_t cp; int count, pre; } cases[] = {
        { "grow", 'a', 100, 0 },
        { "widen", HAN, 1, 2 },
    };
    VM v = vm_new();
    size_t c;
    int quote;

    for (c = 0; c < countof(cases); c++) {
        for (quote = 0; quote < 2; quote++) {
            Text t;
            uint8_t src[256];
            struct StringBuffer *b = white_sb_new(v.ctx, 32, 0, cases[c].pre);
            JSValue str;
            size_t n8;
            int i = 0, failed;
            char *ex;
            int unc;

            t.n = 0;
            text_add(&t, cases[c].cp, cases[c].count);
            n8 = text_utf8(&t, src);
            src[n8] = '"';
            str = JS_NewStringLen(v.ctx, (const char *)src, n8);
            fill_memory(&v);
            if (quote)
                failed = white_json_quote_run(b, str, &i) != 0;
            else
                failed = white_json_string_run(b, src, src + n8 + 1, '"') == NULL;
            release_memory(&v);
            CHECK(failed, "%s, quote %d: the run did not fail", cases[c].name, quote);
            unc = JS_IsUncatchableException(v.ctx);
            ex = exception_text(v.ctx);
            CHECK(ex != NULL && unc, "%s, quote %d: no out of memory error", cases[c].name, quote);
            free(ex);
            CHECK(white_sb_error(b) != 0, "%s, quote %d: the buffer has no error", cases[c].name, quote);
            white_sb_free(v.ctx, b);
            JS_FreeValue(v.ctx, str);
        }
    }
    vm_free(&v);
}

/* An ASCII run of JSON.parse ends JS_STRING_LEN_MAX + 1 bytes on at the
   latest, so its length fits an int. The addresses are made up: no run of a
   gigabyte is read. */
static void test_json_run_limit(void)
{
    uintptr_t p = 0x1000, max = (uintptr_t)white_string_len_max();
    struct { uintptr_t end, want; } cases[] = {
        { p, p },
        { p + 1, p + 1 },
        { p + max, p + max },
        { p + max + 1, p + max + 1 },
        { p + max + 2, p + max + 1 },
        { p + ((uintptr_t)3 << 30), p + max + 1 },
        { p + ((uintptr_t)1 << 32) - 1, p + max + 1 },
        { p + ((uintptr_t)1 << 32) + 5, p + max + 1 },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        uintptr_t got = white_json_run_limit(p, cases[i].end);

        CHECK(got == cases[i].want, "json_run_limit(%#lx, %#lx) = %#lx, want %#lx",
              (unsigned long)p, (unsigned long)cases[i].end, (unsigned long)got,
              (unsigned long)cases[i].want);
    }
}

/* A run that would make the string longer than the longest string throws
   "string too long", as string_buffer_putc did. The buffer is made to look
   that long, so no gigabyte of input is needed. */
static void test_json_run_too_long(void)
{
    static const uint8_t text[] = "aaaaaaaaaa\"";
    VM v = vm_new();
    int wide;

    for (wide = 0; wide < 2; wide++) {
        struct StringBuffer *b = white_sb_new(v.ctx, 32, wide, 0);
        int max = white_string_len_max();
        const uint8_t *r;
        char *ex;

        white_sb_fake_length(b, max - 3, max - 3);
        r = white_json_string_run(b, text, text + sizeof(text) - 1, '"');
        CHECK(r == NULL, "wide %d: the run did not fail", wide);
        ex = exception_text(v.ctx);
        CHECK(ex && strcmp(ex, "InternalError: string too long") == 0, "wide %d: %s, want InternalError: string too long", wide, ex ? ex : "no exception");
        free(ex);
        white_sb_fake_length(b, 0, 32);
        white_sb_free(v.ctx, b);
    }
    vm_free(&v);
}

/* ---- pending stacks ---- */

static const char deep_error[] =
    "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\n"
    "function make() { return deep(40) }\n";

/* Operations that look only at whether 'stack' exists, its attributes or
   whether it is a getter do not need its text, so they leave the stack pending
   and cannot fail for lack of memory to format it. */
static void test_backtrace_attribute_checks(void)
{
    static const char *const ops[] = {
        "Object.keys(e)",
        "e.__lookupGetter__(\"stack\")",
        "e.__lookupSetter__(\"stack\")",
        "e.propertyIsEnumerable(\"stack\")",
        "Object.isFrozen(e)",
        "Object.isSealed(e)",
        "Object.seal(e)",
        "Object.freeze(e)",
        "({ ...e })",
        "Object.assign({}, e)",
        "Object.entries(e)",
        "new Proxy(e, {}).hasOwnProperty(\"stack\")",
        "\"stack\" in new Proxy(e, { has() { return false } })",
        "Reflect.ownKeys(new Proxy(e, { ownKeys(t) { return Reflect.ownKeys(t) } }))",
        "Reflect.deleteProperty(new Proxy(e, { deleteProperty() { return true } }), \"stack\")",
    };
    size_t i;

    for (i = 0; i < countof(ops); i++) {
        VM v = vm_new();
        JSValue ref, e, g;
        char *want, *got, src[400];

        eval_ok(v.ctx, deep_error);
        ref = call_global(v.ctx, "make");
        want = stack_text(v.ctx, ref);
        JS_FreeValue(v.ctx, ref);
        e = call_global(v.ctx, "make");
        snprintf(src, sizeof(src), "var held; function op() { const e = held; return %s }", ops[i]);
        eval_ok(v.ctx, src);
        g = JS_GetGlobalObject(v.ctx);
        JS_SetPropertyStr(v.ctx, g, "held", JS_DupValue(v.ctx, e));
        JS_FreeValue(v.ctx, g);
        JS_FreeValue(v.ctx, call_global(v.ctx, "op"));
        CHECK(white_stack_pending(v.ctx, e) == 1, "%s formatted the stack", ops[i]);
        got = stack_text(v.ctx, e);
        CHECK(got && want && strcmp(got, want) == 0, "read after %s gave a different text", ops[i]);
        free(got);
        free(want);
        JS_FreeValue(v.ctx, e);
        vm_free(&v);
    }
}

/* The class definition of the marker has no call or exotic handlers, so the
   marker is never taken for a function. */
static void test_backtrace_class(void)
{
    VM v = vm_new();

    eval_ok(v.ctx, deep_error);
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    CHECK(!white_backtrace_class_has_handlers(v.rt), "the class of the marker has a call or exotic handler");
    vm_free(&v);
}

/* The value of a property is only a JSValue when the property is a plain data
   property: an accessor holds two object pointers there and a global variable
   a pointer to its variable reference, with whatever bytes were there before
   after it. The pending-stack checks must look at the kind of the property
   first, and never read such bytes as a marker. */
static void test_backtrace_needs_data_property(void)
{
    VM v = vm_new();
    JSValue e;

    eval_ok(v.ctx, deep_error);
    e = call_global(v.ctx, "make");
    eval_ok(v.ctx, "var stack = 1");
    CHECK(white_pending_needs_data_property(v.ctx, e) == 0, "a variable reference was read as a pending stack");
    JS_FreeValue(v.ctx, e);
    vm_free(&v);
}

/* JS_ComputeMemoryUsage counts the frames of a pending stack. */
static void test_backtrace_memory_usage(void)
{
    VM v = vm_new();
    JSMemoryUsage base, held, freed;
    int64_t frames = 1000 * white_backtrace_frame_size();

    eval_ok(v.ctx,
            "var kept\n"
            "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\n"
            "function make() { kept = deep(1000) }\n"
            "function drop() { kept = undefined }\n");
    /* The first call allocates what stays for later calls. */
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &base);
    JS_FreeValue(v.ctx, call_global(v.ctx, "make"));
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &held);
    JS_FreeValue(v.ctx, call_global(v.ctx, "drop"));
    JS_RunGC(v.rt);
    JS_ComputeMemoryUsage(v.rt, &freed);
    CHECK(held.memory_used_size - base.memory_used_size >= frames,
          "a pending stack of 1001 frames adds %lld bytes of memory_used_size, want at least %lld",
          (long long)(held.memory_used_size - base.memory_used_size), (long long)frames);
    CHECK(freed.memory_used_size == base.memory_used_size, "memory_used_size %lld after freeing, want %lld",
          (long long)freed.memory_used_size, (long long)base.memory_used_size);
    vm_free(&v);
}

/* ---- the lists of the collector ---- */

static void lists(VM *v, int *old, int *young, const char *when)
{
    int bad;

    white_gc_lists(v->rt, old, young, &bad);
    CHECK(bad == 0, "%s: a GC object on a list has a count of 0 or less (%d) or its mark set (%d)", when, bad & 1, (bad >> 1) & 1);
}

/* New GC objects are young, a young collection makes what it keeps old, and a
   full collection and JS_ComputeMemoryUsage see the young objects. */
static void test_young_lists(void)
{
    VM v = vm_new();
    int old, young, old2, young2;
    JSMemoryUsage u;

    JS_SetGCThreshold(v.rt, (size_t)-1);
    JS_RunGC(v.rt);
    lists(&v, &old, &young, "after a full collection");
    CHECK(young == 0, "%d young objects after a full collection", young);
    eval_ok(v.ctx, "globalThis.kept = Array.from({length: 100}, (_, i) => ({i})); void 0");
    lists(&v, &old, &young, "after making 100 objects");
    CHECK(young >= 100, "%d young objects after making 100", young);
    JS_RunYoungGC(v.rt);
    lists(&v, &old2, &young2, "after a young collection");
    CHECK(young2 == 0 && old2 >= old + 100, "after a young collection: %d old, %d young, before %d old, %d young", old2, young2, old, young);
    eval_ok(v.ctx, "kept.push({}); void 0");
    JS_ComputeMemoryUsage(v.rt, &u);
    lists(&v, &old, &young, "after JS_ComputeMemoryUsage");
    CHECK(young == 0, "%d young objects after JS_ComputeMemoryUsage", young);
    vm_free(&v);
}

/* After a young collection, no object that only the garbage held is left on a
   list with a count of 0, where the weak pass of the next collection would
   free it while still on the list. */
static void test_young_leaves_no_zero_count(void)
{
    static const char *const observe[] = {
        "", "wm.set(old, 1);", "wr = new WeakRef(old);", "fr.register(old, 1);",
        "old.f = async function () {};", "old.g = (function* () { yield 1 })(); old.g.next();",
    };
    size_t i;

    for (i = 0; i < countof(observe); i++) {
        VM v = vm_new();
        char src[400];
        int old, young;

        JS_SetGCThreshold(v.rt, (size_t)-1);
        snprintf(src, sizeof(src),
                 "globalThis.wm = new WeakMap(); globalThis.wr = null; globalThis.fr = new FinalizationRegistry(() => {});\n"
                 "globalThis.old = {inner: {deep: [1, 2, 3]}}; %s void 0", observe[i]);
        eval_ok(v.ctx, src);
        JS_RunGC(v.rt);
        eval_ok(v.ctx, "(() => { const a = {old}, b = {a}; a.b = b; old = null })(); void 0");
        JS_RunYoungGC(v.rt);
        lists(&v, &old, &young, "after a young collection");
        JS_RunYoungGC(v.rt);
        JS_RunGC(v.rt);
        lists(&v, &old, &young, "after a full collection");
        CHECK(eval_is(v.ctx, "String(wr === null || wr.deref() === undefined)", "true"), "observer %zu: the weak reference holds the object", i);
        vm_free(&v);
    }
}

/* JS_ThrowError2 makes at most two errors at once: one, and the out of
   memory error that making it can throw. Inside two it throws null,
   uncatchable, as it does when there is no memory for an error, and it does
   not abort. Inside one it makes the error as usual. Each leaves the count
   as it found it. */
static void test_error_nest(void)
{
    VM v = vm_new();
    int n;

    for (n = 0; n <= 3; n++) {
        JSValue e;
        int uncatchable;

        CHECK(white_error_nest(v.rt, n) == 0, "%d: the count of errors was not 0", n);
        JS_ThrowTypeError(v.ctx, "made inside %d", n);
        CHECK(white_error_nest(v.rt, 0) == n, "%d: the count of errors moved", n);
        uncatchable = JS_IsUncatchableException(v.ctx);
        e = JS_GetException(v.ctx);
        JS_SetUncatchableException(v.ctx, 0);
        if (n < 2) {
            JSValue m = JS_GetPropertyStr(v.ctx, e, "message");
            const char *s = JS_ToCString(v.ctx, m);
            char want[32];

            snprintf(want, sizeof want, "made inside %d", n);
            CHECK(JS_IsError(v.ctx, e) && !uncatchable, "%d: want a catchable error", n);
            CHECK(s && strcmp(s, want) == 0, "%d: message %s", n, s ? s : "(none)");
            JS_FreeCString(v.ctx, s);
            JS_FreeValue(v.ctx, m);
        } else {
            CHECK(JS_IsNull(e) && uncatchable, "%d: want null, uncatchable, got tag %d", n, JS_VALUE_GET_TAG(e));
        }
        JS_FreeValue(v.ctx, e);
    }
    CHECK(eval_is(v.ctx, "try { null.x } catch (e) { String(e instanceof TypeError) }", "true"), "the runtime does not throw after");
    vm_free(&v);
}

/* js_function_set_properties gives a new function its length and name. With
   no memory for the string of a name that is an index atom (which holds no
   string) or for a property, it returns -1 with the out of memory error
   pending, and the caller frees the function. It used to say nothing, and
   JS_NewCFunction2, JS_NewHostFunction and the others returned the function
   with the error still pending and its name the exception marker. Both
   paths: a function whose prototype is Function.prototype takes the shape
   the context keeps for it and gets only the string, and one with another
   prototype defines both properties. With memory, both get them. */
static void test_function_properties_oom(void)
{
    int other, full;

    for (other = 0; other < 2; other++) {
        for (full = 0; full < 2; full++) {
            VM v = vm_new();
            JSValue g = JS_GetGlobalObject(v.ctx), fp = JS_GetPropertyStr(v.ctx, g, "Function"), proto, f;
            JSAtom name = JS_NewAtomUInt32(v.ctx, 12345);
            const char *path = other ? "another prototype" : "Function.prototype";
            int r, pending, unc;

            proto = JS_GetPropertyStr(v.ctx, fp, "prototype");
            JS_FreeValue(v.ctx, fp);
            JS_FreeValue(v.ctx, g);
            if (other) {
                JSValue p2 = JS_NewObjectProto(v.ctx, proto);

                JS_FreeValue(v.ctx, proto);
                proto = p2;
            }
            f = white_bare_function(v.ctx, proto);
            CHECK(!JS_IsException(f), "%s: no function", path);
            if (full)
                fill_memory(&v);
            r = white_function_set_properties(v.ctx, f, name, 2);
            pending = JS_HasException(v.ctx);
            unc = JS_IsUncatchableException(v.ctx);
            if (full)
                release_memory(&v);
            if (full) {
                CHECK(r == -1 && pending && unc, "%s, full heap: returned %d, pending %d, uncatchable %d", path, r, pending, unc);
                free(exception_text(v.ctx));
            } else {
                JSValue n = JS_GetPropertyStr(v.ctx, f, "name"), l = JS_GetPropertyStr(v.ctx, f, "length");
                const char *s = JS_ToCString(v.ctx, n);

                CHECK(r == 0 && !pending, "%s: returned %d, pending %d", path, r, pending);
                CHECK(s && !strcmp(s, "12345") && JS_VALUE_GET_TAG(l) == JS_TAG_INT && JS_VALUE_GET_INT(l) == 2,
                      "%s: name %s, length tag %d", path, s ? s : "?", JS_VALUE_GET_TAG(l));
                JS_FreeCString(v.ctx, s);
                JS_FreeValue(v.ctx, n);
                JS_FreeValue(v.ctx, l);
            }
            JS_FreeValue(v.ctx, f);
            JS_FreeValue(v.ctx, proto);
            JS_FreeAtom(v.ctx, name);
            fflush(stdout);
            vm_free(&v);
        }
    }
}

/* ---- the polls of the bytecode ---- */

/* The opcodes of white_op, in its order. */
enum {
    W_INVALID, W_NOP, W_PUSH_0, W_DROP, W_GOTO, W_GOTO16, W_GOTO8,
    W_IF_FALSE, W_IF_TRUE, W_IF_TRUE8, W_IF_FALSE8, W_RETURN_UNDEF, W_CATCH,
    W_GOSUB, W_RET, W_WITH_GET_VAR, W_PUSH_I32, W_PUSH_I16,
};

/* Code written for a test, one instruction at a time. A jump is written
   with no target, which set_target puts in once its place is known. */
typedef struct {
    uint8_t b[4096];
    int n;
} Code;

static void op_n(Code *c, int w, int times)
{
    while (times-- > 0)
        c->b[c->n++] = white_op(w);
}

static int jump(Code *c, int w)
{
    int at = c->n, size = 5;

    switch (w) {
    case W_GOTO16: size = 3; break;
    case W_GOTO8: case W_IF_TRUE8: case W_IF_FALSE8: size = 2; break;
    case W_WITH_GET_VAR: size = 10; break;
    }
    memset(c->b + at, 0, size);
    c->b[at] = white_op(w);
    c->n += size;
    return at;
}

static void set_target(Code *c, int at, int target)
{
    int op = c->b[at], d = target - (at + 1);
    int32_t d32;
    int16_t d16;

    if (op == white_op(W_WITH_GET_VAR)) {
        d32 = target - (at + 5);
        memcpy(c->b + at + 5, &d32, 4);
    } else if (op == white_op(W_GOTO16)) {
        d16 = d;
        memcpy(c->b + at + 1, &d16, 2);
    } else if (op == white_op(W_GOTO8) || op == white_op(W_IF_TRUE8) || op == white_op(W_IF_FALSE8)) {
        c->b[at + 1] = (uint8_t)d;
    } else {
        d32 = d;
        memcpy(c->b + at + 1, &d32, 4);
    }
}

/* scan_ok: c keeps to the rule of the polls. scan_fails: it does not, and
   what poll_scan says has want in it. */
static int scan_ok(const Code *c)
{
    const char *why = white_poll_scan(c->b, c->n);

    if (why)
        printf("poll_scan: %s\n", why);
    return why == NULL;
}

static int scan_fails(const Code *c, const char *want)
{
    const char *why = white_poll_scan(c->b, c->n);

    return why && strstr(why, want);
}

/* Code with a run of n instructions and a return. */
static Code straight(int n)
{
    Code c = {{0}, 0};

    op_n(&c, W_PUSH_0, n);
    op_n(&c, W_RETURN_UNDEF, 1);
    return c;
}

/* poll_scan, which check_polls runs on all code before it runs, takes a run
   of POLL_RUN instructions without a count and no more: in straight code, on
   a jump forward, which carries the run on, and into a finally block. A jump
   back, an OP_nop and the catch of an exception count. It also stops on code
   it cannot follow: an opcode the interpreter does not have, an instruction
   past the end, a jump into an instruction or out of the code, and a last
   instruction that runs off the end. */
static void test_poll_scan(void)
{
    int run = white_poll_run(), at, x;
    Code c;
    static const uint8_t past_end[] = { 0, 0, 0 };
    uint8_t bad[8];

    c = straight(run - 1);
    CHECK(scan_ok(&c), "a run of %d", run);
    c = straight(run);
    CHECK(scan_fails(&c, "without a count"), "a run of %d", run + 1);
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, 100);
    op_n(&c, W_NOP, 1);
    op_n(&c, W_PUSH_0, 100);
    op_n(&c, W_NOP, 1);
    op_n(&c, W_PUSH_0, 100);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "runs of 100 between OP_nop");

    /* a loop: the jump back counts */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 10);
    set_target(&c, jump(&c, W_GOTO), 0);
    CHECK(scan_ok(&c), "a loop");
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 10);
    set_target(&c, jump(&c, W_IF_TRUE8), 0);
    op_n(&c, W_PUSH_0, 9);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_fails(&c, "without a count"), "a run on past a jump back not taken");

    /* a jump forward carries the run on past code it skips */
    for (x = 20; x <= 30; x += 10) {
        int w;
        for (w = 0; w < 2; w++) {
            c = (Code){{0}, 0};
            op_n(&c, W_PUSH_0, 100);
            at = jump(&c, w ? W_WITH_GET_VAR : W_GOTO16);
            op_n(&c, W_RETURN_UNDEF, 1);
            set_target(&c, at, c.n);
            op_n(&c, W_PUSH_0, x);
            op_n(&c, W_RETURN_UNDEF, 1);
            if (x == 20)
                CHECK(scan_ok(&c), "%d after a jump at 101", x);
            else
                CHECK(scan_fails(&c, "without a count"), "%d after a jump at 101 (%d)", x, w);
        }
    }

    /* an exception reaches the catch, which counts: a jump would not */
    for (x = 0; x < 2; x++) {
        c = (Code){{0}, 0};
        op_n(&c, W_PUSH_0, 120);
        at = jump(&c, x ? W_IF_FALSE : W_CATCH);
        op_n(&c, W_RETURN_UNDEF, 1);
        set_target(&c, at, c.n);
        op_n(&c, W_PUSH_0, 20);
        op_n(&c, W_RETURN_UNDEF, 1);
        if (x)
            CHECK(scan_fails(&c, "without a count"), "a jump carries the run");
        else
            CHECK(scan_ok(&c), "a catch counts");
    }

    /* a finally block: OP_gosub goes on with the run, OP_ret counts */
    for (x = 120; x <= 130; x += 10) {
        c = (Code){{0}, 0};
        at = jump(&c, W_GOSUB);
        op_n(&c, W_RETURN_UNDEF, 1);
        set_target(&c, at, c.n);
        op_n(&c, W_PUSH_0, x);
        op_n(&c, W_RET, 1);
        if (x == 120)
            CHECK(scan_ok(&c), "a finally block of %d", x);
        else
            CHECK(scan_fails(&c, "without a count"), "a finally block of %d", x);
    }

    /* code it cannot follow */
    c = (Code){{0}, 0};
    op_n(&c, W_INVALID, 1);
    CHECK(scan_fails(&c, "an opcode the interpreter does not have"), "OP_invalid");
    bad[0] = white_op_count();
    bad[1] = white_op(W_RETURN_UNDEF);
    CHECK(white_poll_scan(bad, 2) && strstr(white_poll_scan(bad, 2), "does not have"), "an opcode past the last");
    memcpy(bad, past_end, sizeof(past_end));
    bad[0] = white_op(W_GOTO);
    CHECK(white_poll_scan(bad, 3) && strstr(white_poll_scan(bad, 3), "past the end"), "a cut instruction");
    c = (Code){{0}, 0};
    at = jump(&c, W_GOTO8);
    x = jump(&c, W_GOTO);
    set_target(&c, at, x + 2);
    set_target(&c, x, 0);
    CHECK(scan_fails(&c, "does not land"), "a jump into an instruction");
    c = (Code){{0}, 0};
    at = jump(&c, W_IF_FALSE8);
    op_n(&c, W_RETURN_UNDEF, 1);
    set_target(&c, at, -4);
    CHECK(scan_fails(&c, "does not land"), "a jump out of the code");
    /* a target of -1, the byte before the code, as a short and a long jump
       and a catch: [OP_goto8, 0xfe] is the whole code of the first */
    for (x = 0; x < 3; x++) {
        static const int w[] = { W_GOTO8, W_GOTO, W_CATCH };
        c = (Code){{0}, 0};
        at = jump(&c, w[x]);
        if (x == 2)
            op_n(&c, W_RETURN_UNDEF, 1);
        set_target(&c, at, -1);
        CHECK(scan_fails(&c, "does not land"), "a jump to -1 (%d)", x);
    }
    c = (Code){{0}, 0};
    at = jump(&c, W_WITH_GET_VAR);
    op_n(&c, W_RETURN_UNDEF, 1);
    set_target(&c, at, c.n);
    CHECK(scan_fails(&c, "does not land"), "a with_ jump past the end");
    c = straight(3);
    c.n--;
    CHECK(scan_fails(&c, "runs off the end"), "no return at the end");
    CHECK(white_poll_scan(bad, 0) && strstr(white_poll_scan(bad, 0), "runs off the end"), "no code");
}

/* insert_c puts the polls into c (insert_polls), checks that the result
   keeps to the rule and has the shape of c, and returns it. */
static Code insert_c(JSContext *ctx, const Code *c, const char *what)
{
    static int s1[8192], s2[8192];
    Code out = {{0}, 0};
    int n1, n2;

    out.n = white_poll_insert(ctx, c->b, c->n, out.b, sizeof(out.b));
    CHECK(out.n > 0, "%s: insert_polls failed", what);
    if (out.n <= 0)
        return out;
    CHECK(scan_ok(&out), "%s: the code with the polls", what);
    n1 = white_code_shape(c->b, c->n, s1, 4096);
    n2 = white_code_shape(out.b, out.n, s2, 4096);
    CHECK(n1 > 0 && n1 == n2 && memcmp(s1, s2, sizeof(int) * 2 * n1) == 0,
          "%s: the code changed shape (%d and %d instructions)", what, n1, n2);
    return out;
}

static int count_nops(const Code *c)
{
    int places[256];

    return white_nops(c->b, c->n, places, countof(places));
}

/* insert_polls puts an OP_nop in where a path reaches POLL_RUN - 1
   instructions, unless the instruction there ends it, and moves the jumps:
   one into a run, a short one that no longer reaches, which takes its long
   form, a jump of with_, of a catch and of a finally block. */
static void test_poll_insert(void)
{
    VM v = vm_new();
    int run = white_poll_run(), at, at2, at3;
    Code c, out;

    c = straight(2 * run);
    out = insert_c(v.ctx, &c, "straight");
    CHECK(count_nops(&out) == 2, "%d OP_nop in a run of %d", count_nops(&out), 2 * run + 1);
    c = straight(run - 1);
    out = insert_c(v.ctx, &c, "a run that fits");
    CHECK(out.n == c.n, "OP_nop in a run that fits");

    /* a short jump forward over the place of an OP_nop */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, 100);
    at = jump(&c, W_IF_FALSE8);
    op_n(&c, W_PUSH_0, 126);
    set_target(&c, at, c.n);
    op_n(&c, W_RETURN_UNDEF, 1);
    out = insert_c(v.ctx, &c, "a short jump forward");
    CHECK(count_nops(&out) == 1 && out.b[at] == white_op(W_IF_FALSE),
          "%d OP_nop and opcode %d, want 1 and if_false", count_nops(&out), out.b[at]);

    /* a short jump back, with an OP_nop before it */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 1);
    at = jump(&c, W_IF_TRUE8);
    set_target(&c, at, 0);
    op_n(&c, W_RETURN_UNDEF, 1);
    out = insert_c(v.ctx, &c, "a short jump back");
    CHECK(count_nops(&out) == 1 && out.b[at + 1] == white_op(W_IF_TRUE),
          "%d OP_nop and opcode %d, want 1 and if_true", count_nops(&out), out.b[at + 1]);

    /* a goto back that ends a path of POLL_RUN instructions needs none */
    c = (Code){{0}, 0};
    op_n(&c, W_PUSH_0, run - 1);
    set_target(&c, jump(&c, W_GOTO), 0);
    out = insert_c(v.ctx, &c, "a loop that fits");
    CHECK(count_nops(&out) == 0, "%d OP_nop in a loop that fits", count_nops(&out));

    /* with_, catch and a finally block, over long runs */
    c = (Code){{0}, 0};
    at = jump(&c, W_CATCH);
    at2 = jump(&c, W_WITH_GET_VAR);
    op_n(&c, W_PUSH_0, 200);
    at3 = jump(&c, W_GOSUB);
    set_target(&c, at2, c.n);
    op_n(&c, W_RETURN_UNDEF, 1);
    set_target(&c, at3, c.n);
    op_n(&c, W_PUSH_0, 200);
    op_n(&c, W_RET, 1);
    set_target(&c, at, c.n);
    op_n(&c, W_PUSH_0, 200);
    op_n(&c, W_RETURN_UNDEF, 1);
    out = insert_c(v.ctx, &c, "with_, catch and a finally block");
    CHECK(count_nops(&out) >= 4, "%d OP_nop in three runs of 200", count_nops(&out));
    vm_free(&v);
}

/* long_func: the source of a function of n lines, one assignment each, that
   returns n - 1, or with throws set throws on its last line. */
static char *long_func(int n, int throws)
{
    size_t cap = 32 * (size_t)n + 100, len;
    char *s = malloc(cap);
    int i;

    len = snprintf(s, cap, "(function () { var a = 0;\n");
    for (i = 1; i < n; i++)
        len += snprintf(s + len, cap - len, "a = %d;\n", i);
    snprintf(s + len, cap - len, throws ? "throw new Error('end') })" : "return a })");
    return s;
}

/* code_of: the code of the function of src, compiled only, in *top (the
   script), *code and *len. */
static int code_of(JSContext *ctx, const char *src, JSValue *top, const uint8_t **code, int *len)
{
    *top = JS_Eval(ctx, src, strlen(src), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    if (JS_IsException(*top))
        return -1;
    *code = white_child_code(*top, 0, len);
    return *code ? 0 : -1;
}

/* run_read: read buf back with JS_ReadObject, run the script and call the
   function it gives. "ok" with the number it returned, or the exception. */
static void run_read(JSContext *ctx, const uint8_t *buf, size_t size, char *out, size_t cap)
{
    JSValue top, fun, r;
    char *t;
    int32_t n = -1;

    top = JS_ReadObject(ctx, buf, size, JS_READ_OBJ_BYTECODE);
    if (JS_IsException(top))
        goto exception;
    fun = JS_EvalFunction(ctx, top);
    if (JS_IsException(fun))
        goto exception;
    r = JS_Call(ctx, fun, JS_UNDEFINED, 0, NULL);
    JS_FreeValue(ctx, fun);
    if (JS_IsException(r))
        goto exception;
    JS_ToInt32(ctx, &n, r);
    JS_FreeValue(ctx, r);
    snprintf(out, cap, "ok %d", (int)n);
    return;
 exception:
    t = exception_text(ctx);
    snprintf(out, cap, "%s", t ? t : "?");
    free(t);
}

/* The compiler puts the polls into a long function, which runs and keeps
   its line numbers. Without them (white_poll_skip) check_polls stops the
   compiler, and JS_ReadObject reads the function back, but not with its
   OP_nop taken out or with an opcode the interpreter does not have. */
static void test_poll_compile(void)
{
    VM v = vm_new();
    char *src = long_func(400, 0), *src2 = long_func(400, 1), *t, got[256];
    const char *stack;
    JSValue top, fun, r;
    const uint8_t *code;
    uint8_t *buf, *copy;
    size_t size;
    int len, places[64], n, i;
    const uint8_t *at;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile a long function");
    CHECK(white_poll_scan(code, len) == NULL, "the code of a long function");
    n = white_nops(code, len, places, countof(places));
    CHECK(n >= 4, "%d OP_nop in a long function", n);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "write the long function");
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the long function in what JS_WriteObject wrote");
    run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(strcmp(got, "ok 399") == 0, "read back: %s", got);
    if (at) {
        copy = malloc(size);
        memcpy(copy, buf, size);
        for (i = 0; i < n && i < (int)countof(places); i++)
            copy[at - buf + places[i]] = white_op(W_PUSH_0);
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid bytecode: a run of more instructions than POLL_RUN"), "read without the polls: %s", got);
        memcpy(copy, buf, size);
        copy[at - buf] = white_op(W_INVALID);
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid bytecode: an opcode the interpreter does not have"), "read with OP_invalid: %s", got);
        free(copy);
    }
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);

    /* the line of an error after the OP_nop: the throw is on line 401 */
    fun = JS_Eval(v.ctx, src2, strlen(src2), "<poll>", JS_EVAL_TYPE_GLOBAL);
    r = JS_Call(v.ctx, fun, JS_UNDEFINED, 0, NULL);
    CHECK(JS_IsException(r), "the long function that throws");
    JS_FreeValue(v.ctx, r);
    JS_FreeValue(v.ctx, fun);
    r = JS_GetException(v.ctx);
    fun = JS_GetPropertyStr(v.ctx, r, "stack");
    stack = JS_ToCString(v.ctx, fun);
    CHECK(stack && strstr(stack, "<poll>:401"), "the stack of the long function: %s", stack ? stack : "(none)");
    JS_FreeCString(v.ctx, stack);
    JS_FreeValue(v.ctx, fun);
    JS_FreeValue(v.ctx, r);

    white_poll_skip(1);
    top = JS_Eval(v.ctx, src, strlen(src), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    white_poll_skip(0);
    CHECK(JS_IsException(top), "a long function compiled without the polls");
    t = exception_text(v.ctx);
    CHECK(t && strstr(t, "InternalError: compiled bytecode: a run of more instructions than POLL_RUN"), "without the polls: %s", t ? t : "(none)");
    free(t);
    JS_FreeValue(v.ctx, top);
    free(src);
    free(src2);
    vm_free(&v);
}

/* push_i32: an OP_push_i32 of v at the end of c. */
static void push_i32(Code *c, int32_t v)
{
    op_n(c, W_PUSH_I32, 1);
    memcpy(c->b + c->n, &v, 4);
    c->n += 4;
}

/* read_code: c in place of the code of a function, written with
   JS_WriteObject, read back and run (run_read). The function calls its
   argument with 300 numbers, which gives c a stack and a length of 1505
   bytes, and holds no atom, so that its code is in what JS_WriteObject
   writes as it is in memory. The code past c is OP_return_undef. */
static void read_code(JSContext *ctx, const Code *c, char *got, size_t cap)
{
    size_t scap = 16 * 300 + 100, n, size;
    char *src = malloc(scap);
    JSValue top;
    const uint8_t *code, *at;
    uint8_t *buf;
    int len, i;

    n = snprintf(src, scap, "(function (g) { return g(");
    for (i = 0; i < 300; i++)
        n += snprintf(src + n, scap - n, "%s%d", i ? ", " : "", 100000 + i);
    snprintf(src + n, scap - n, ") })");
    snprintf(got, cap, "no code");
    if (code_of(ctx, src, &top, &code, &len) == 0) {
        buf = JS_WriteObject(ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
        at = buf ? memmem(buf, size, code, len) : NULL;
        if (at && c->n <= len) {
            memcpy(buf + (at - buf), c->b, c->n);
            memset(buf + (at - buf) + c->n, white_op(W_RETURN_UNDEF), len - c->n);
            run_read(ctx, buf, size, got, cap);
        }
        js_free(ctx, buf);
    }
    JS_FreeValue(ctx, top);
    free(src);
}

/* OP_ret goes only where an OP_gosub returns to: code read with
   JS_ReadObject gives it any number, which must not take the interpreter
   into the middle of an instruction or to one that does not follow an
   OP_gosub, where it would run code check_polls did not follow. The two
   cases: OP_ret to 7, inside an OP_push_i32 whose operand holds an
   OP_return_undef, and a path of 255 instructions without a count, which
   runs inside the operands of two runs of 127 that check_polls takes, one
   ending with an OP_nop. And OP_ret to an instruction that follows no
   OP_gosub. A finally block read back runs. */
static void test_poll_ret(void)
{
    VM v = vm_new();
    static Code c;
    char got[256];
    const char *src = "(function () { var t = 0; for (var i = 0; i < 3; i++) { try { t += i } finally { t += 10 } } return t })";
    JSValue top;
    uint8_t *buf;
    size_t size;
    int i;

    /* OP_ret to 7 */
    c = (Code){{0}, 0};
    push_i32(&c, 7);
    op_n(&c, W_RET, 1);
    push_i32(&c, 0);
    c.b[7] = white_op(W_RETURN_UNDEF);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "OP_ret to 7: check_polls takes the code");
    read_code(v.ctx, &c, got, sizeof(got));
    CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret into an instruction: %s", got);

    /* 255 instructions without a count */
    c = (Code){{0}, 0};
    push_i32(&c, 7);
    op_n(&c, W_RET, 1);
    for (i = 0; i < 127; i++)
        push_i32(&c, 0);
    op_n(&c, W_NOP, 1);
    for (i = 0; i < 127; i++)
        push_i32(&c, 0);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(c.n == 1278, "the code of 255 instructions is %d bytes", c.n);
    for (i = 0; i < 126; i++) {
        c.b[6 + 5 * i + 1] = white_op(W_PUSH_I32);
        c.b[642 + 5 * i + 1] = white_op(W_PUSH_I32);
    }
    c.b[637] = white_op(W_PUSH_I16);
    c.b[640] = white_op(W_PUSH_I16);
    c.b[1273] = white_op(W_RETURN_UNDEF);
    CHECK(scan_ok(&c), "255 instructions: check_polls takes the code");
    read_code(v.ctx, &c, got, sizeof(got));
    CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret to a path of 255: %s", got);

    /* OP_ret to the start of an instruction that follows no OP_gosub */
    c = (Code){{0}, 0};
    push_i32(&c, 6);
    op_n(&c, W_RET, 1);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "OP_ret to 6: check_polls takes the code");
    read_code(v.ctx, &c, got, sizeof(got));
    CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret to no OP_gosub: %s", got);

    /* a finally block, compiled and read back */
    top = JS_Eval(v.ctx, src, strlen(src), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    CHECK(buf != NULL, "write a finally block");
    if (buf)
        run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(buf && strcmp(got, "ok 33") == 0, "a finally block read back: %s", buf ? got : "(none)");
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    vm_free(&v);
}

/* The compiler adds the variable of this, new.target, the home object, the
   active function of a derived class constructor, arguments and the name of
   a function expression the first time code of the function, or of a
   function in it, uses one. When adding it runs out of memory, resolving the
   name fails with the out of memory error. It used to take the failure for a
   function without that variable and look further out: the name became a
   variable of the script, or a global, with the error left pending, and from
   an arrow function, arguments and the name of the function read the
   variable at index -1. The variables a direct eval can use fail the same
   way. With memory, each gets its variable. */
static void test_added_vars_oom(void)
{
    static const char *const kinds[] = {
        "this", "new.target", "home object", "active function", "arguments", "function name",
    };
    int kind, in_arrow, full;

    for (in_arrow = 0; in_arrow < 2; in_arrow++) {
        for (full = 0; full < 2; full++) {
            for (kind = 0; kind <= (int)countof(kinds); kind++) {
                VM v = vm_new();
                const char *what = kind < (int)countof(kinds) ? kinds[kind] : "eval";
                const char *where = in_arrow ? "in an arrow function" : "in the function";
                size_t room = full ? 4096 : (size_t)-1;
                int r, pending, made, unc;

                if (kind < (int)countof(kinds))
                    r = white_resolve_var(v.ctx, kind, in_arrow, room, &pending, &made);
                else
                    r = white_eval_vars(v.ctx, in_arrow, room, &pending, &made);
                unc = JS_IsUncatchableException(v.ctx);
                CHECK(r != -2, "%s %s: the functions could not be made", what, where);
                if (full)
                    CHECK(r == -1 && pending && unc, "%s %s, no memory: returned %d, pending %d, uncatchable %d", what, where, r, pending, unc);
                else
                    CHECK(r >= 0 && !pending && made, "%s %s: returned %d, pending %d, made %d", what, where, r, pending, made);
                free(exception_text(v.ctx));
                fflush(stdout);
                vm_free(&v);
            }
        }
    }
}

/* ---- the own keys of an object ---- */

/* objects of every kind of key, with the shapes the listing handles itself,
   keeps in the key cache, and leaves to the generic path */
static const struct { const char *name, *js; } own_keys_objects[] = {
    { "empty", "({})" },
    { "strings", "({a: 1, b: 2, c: 3})" },
    { "indexes", "({b: 1, 10: 2, 2: 3, a: 4, 4294967294: 5, 4294967295: 6, '01': 7})" },
    { "symbols", "({[Symbol.iterator]: 1, a: 2, [Symbol('s')]: 3, 1: 4, [Symbol.for('g')]: 5})" },
    { "hidden", "Object.defineProperties({a: 1}, {h: {value: 2}, [Symbol('h')]: {value: 3}, 3: {value: 4}})" },
    { "accessors", "({get a() { return 1 }, set b(v) {}, c: 3})" },
    { "deleted", "(() => { const o = {a: 1, b: 2, 1: 3, c: 4}; delete o.b; delete o[1]; return o })()" },
    { "private", "(() => { const s = Symbol('s'); class C { #p = 1; x = 2; #q = 3; constructor() { this[s] = 4; this[0] = 5 } }; return new C })()" },
    { "null proto", "Object.assign(Object.create(null), {z: 1, 0: 2, y: 3})" },
    { "frozen", "Object.freeze({a: 1, 2: 2})" },
    { "wide", "({'\\u00e9': 1, '\\u4e2d': 2, '1\\u4e2d': 3})" },
    { "many strings", "Object.fromEntries(Array.from({length: 40}, (_, i) => ['k' + i, i]))" },
    { "many indexes", "Object.fromEntries(Array.from({length: 20}, (_, i) => [String(19 - i), i]))" },
    { "function", "(function f(a, b) {})" },
    { "class", "(class K { static s = 1; static m() {} })" },
    { "error", "new Error('m')" },
    { "array", "Object.assign([1, 2], {x: 3})" },
    { "arguments", "(function () { return arguments })(1, 2)" },
    { "string object", "Object.assign(new String('ab'), {x: 1})" },
    { "typed array", "new Uint8Array(3)" },
    { "proxy", "new Proxy({a: 1}, {})" },
};

/* The flags of JS_GetOwnPropertyNames. */
#define OWN_KEYS_ALL_FLAGS (JS_GPN_STRING_MASK | JS_GPN_SYMBOL_MASK | JS_GPN_PRIVATE_MASK | JS_GPN_ENUM_ONLY | JS_GPN_SET_ENUM)

/* For every combination of flags, JS_GetOwnPropertyNames lists the own keys
   of an ordinary object as the engine does from its shape, also the second
   time, when the kinds come from the key cache, and lists those of exotic
   objects without failing. */
static void test_own_keys_flags(void)
{
    VM v = vm_new();
    static char got[8192], want[8192];
    size_t i;

    for (i = 0; i < countof(own_keys_objects); i++) {
        JSValue obj = JS_Eval(v.ctx, own_keys_objects[i].js, strlen(own_keys_objects[i].js),
                              "<unit>", JS_EVAL_TYPE_GLOBAL);
        int flags, pass, exotic = 0;

        CHECK(JS_IsObject(obj), "%s: %s is no object", own_keys_objects[i].name, own_keys_objects[i].js);
        if (!JS_IsObject(obj)) {
            free(exception_text(v.ctx));
            continue;
        }
        for (flags = 0; flags <= OWN_KEYS_ALL_FLAGS; flags++) {
            if (flags & ~OWN_KEYS_ALL_FLAGS)
                continue;
            for (pass = 0; pass < 2; pass++) {
                int ng = white_own_keys(v.ctx, obj, flags, 1, got, sizeof(got));
                int nw = white_own_keys(v.ctx, obj, flags, 0, want, sizeof(want));

                exotic = nw == -2;
                CHECK(ng >= 0, "%s, flags %#x: JS_GetOwnPropertyNames throws", own_keys_objects[i].name, flags);
                if (!exotic)
                    CHECK(ng == nw && strcmp(got, want) == 0,
                          "%s, flags %#x:\ngot  %s\nwant %s", own_keys_objects[i].name, flags, got, want);
            }
        }
        JS_FreeValue(v.ctx, obj);
    }
    vm_free(&v);
}

int main(void)
{
    test_json_string_run();
    test_json_quote_run();
    test_json_runs_oom();
    test_json_run_too_long();
    test_json_run_limit();
    test_backtrace_attribute_checks();
    test_backtrace_class();
    test_backtrace_needs_data_property();
    test_backtrace_memory_usage();
    test_young_lists();
    test_young_leaves_no_zero_count();
    test_error_nest();
    test_function_properties_oom();
    test_poll_scan();
    test_poll_insert();
    test_poll_compile();
    test_poll_ret();
    test_added_vars_oom();
    test_own_keys_flags();
    if (failures) {
        printf("unit-white: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-white: %ld checks passed\n", checks);
    return 0;
}
