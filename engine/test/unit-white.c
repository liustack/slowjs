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
   place of quickjs.o). They took over the
   tests of JSON, backtraces and the young collection that did this on the
   engine before it.

     unit-white */
#include <errno.h>
#include <sys/wait.h>
#include <unistd.h>

#include "compile-scripts.h"
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
int white_function_debug(JSValueConst script, int i, const uint8_t **source, size_t *source_len, const uint8_t **pc2line, int *shared);
int white_regexp_cache(JSRuntime *rt, char *out, size_t cap, size_t *bytes, uint32_t *buckets);
int white_lre_split_budget(void);
JSValue white_compile_failing(JSContext *ctx, const char *js, int fail, int mode, int *nallocs);
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
int white_string_is_rope(JSValueConst v);
int white_string_room(JSContext *ctx, JSValueConst v);
int white_every_block_is_a_request(void);
int white_fused_ops(const uint8_t *code, int len, int *count);
size_t white_object_block(JSContext *ctx, JSValueConst obj, size_t *full);
int white_props_inline(JSValueConst obj, int *room);
uintptr_t white_shape_of(JSValueConst obj);
JSValue white_call_failing(JSContext *ctx, JSValueConst func, JSValueConst this_val, int argc, JSValueConst *argv, int fail, int *nallocs);
void white_work_reset(void);
int white_jsstring_header(void);
int white_sb_write8(struct StringBuffer *b, const uint8_t *p, int len);
int white_sb_putc16(struct StringBuffer *b, uint32_t c);
unsigned long white_work_longest(void);
unsigned long white_work_sum(void);

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

/* ---- the block of an object ---- */

/* An ordinary object has no room for the union of the data of the classes
   (the engine allocates the bytes in front of it), and every other object has.
   The block of an object literal also has the room of its properties. */
static void test_object_block(void)
{
    VM v = vm_new();
    JSValue plain = JS_NewObject(v.ctx), arr = JS_NewArray(v.ctx), lit;
    const char *src = "({a: 1, b: \"x\"})";
    size_t full = 0, a, p, f, plain_room;

    p = white_object_block(v.ctx, plain, &full);
    plain_room = p;
    a = white_object_block(v.ctx, arr, &f);
    CHECK(p > 0 && p < full, "an ordinary object has room for %zu bytes, a JSObject is %zu", p, full);
    CHECK(a >= full, "an array has room for %zu bytes, a JSObject is %zu", a, full);
    JS_FreeValue(v.ctx, plain);
    JS_FreeValue(v.ctx, arr);
    lit = JS_Eval(v.ctx, src, strlen(src), "<unit>", JS_EVAL_TYPE_GLOBAL);
    p = white_object_block(v.ctx, lit, &full);
    /* the block of an object literal of two keys also holds their two properties of 16 bytes */
    CHECK(p >= plain_room + 2 * 16 && p < plain_room + 2 * 16 + 8, "an object literal of 2 keys has room for %zu bytes, an ordinary object %zu", p, plain_room);
    JS_FreeValue(v.ctx, lit);
    vm_free(&v);
}

/* ---- the properties of an object ---- */

static int props_inline(VM *v, const char *expr, int *room)
{
    JSValue o = JS_Eval(v->ctx, expr, strlen(expr), "<unit>", JS_EVAL_TYPE_GLOBAL);
    int r = white_props_inline(o, room);

    JS_FreeValue(v->ctx, o);
    return r;
}

/* The properties of an object literal sit in the block of the object, with
   room for the properties of its shape. Adding a key past that room moves
   them to a block of their own, and everything else that changes an object
   (flags, prototype, deleting keys, even until the properties are packed)
   leaves them where they are. */
static void test_object_inline_props(void)
{
    VM v = vm_new();
    int room = -1;

    CHECK(eval_ok(v.ctx, "var lit = { a: 1, b: [2], c: 'three' }; var open = {}; var arr = [1];\n"
                         "var big = { k0: 0, k1: 1, k2: 2, k3: 3, k4: 4, k5: 5, k6: 6, k7: 7,\n"
                         "            k8: 8, k9: 9, k10: 10, k11: 11, k12: 12, k13: 13, k14: 14, k15: 15 };\n"
                         "var made = []; for (var i = 0; i < 4; i++) made.push({ a: i, b: i });\n"
                         "void 0"), "the script threw");
    CHECK(props_inline(&v, "lit", &room) == 1 && room >= 3, "a literal of 3 keys: room %d", room);
    CHECK(props_inline(&v, "made[3]", &room) == 1 && room >= 2, "a literal of 2 keys: room %d", room);
    CHECK(props_inline(&v, "big", &room) == 1 && room >= 16, "a literal of 16 keys: room %d", room);
    CHECK(props_inline(&v, "open", &room) == 0, "{} has its properties apart");
    CHECK(props_inline(&v, "arr", &room) == 0, "an array has its properties apart");
    CHECK(props_inline(&v, "Object.create(null)", &room) == 0, "Object.create has its properties apart");
    /* changes that keep the room */
    CHECK(eval_ok(v.ctx, "Object.setPrototypeOf(lit, { p: 1 }); Object.defineProperty(lit, 'a', { writable: false });\n"
                         "Object.defineProperty(lit, 'c', { get() { return 5 }, enumerable: true, configurable: true }); void 0"),
          "the script threw");
    CHECK(props_inline(&v, "lit", &room) == 1, "after a prototype, a flag and an accessor");
    CHECK(eval_is(v.ctx, "lit.p + ',' + lit.a + ',' + lit.b + ',' + lit.c + ',' + Object.keys(lit)", "1,1,2,5,a,b,c"),
          "values after the changes");
    CHECK(eval_ok(v.ctx, "Object.freeze(made[1]); Object.seal(made[2]); Object.preventExtensions(made[3]); void 0"), "the script threw");
    CHECK(props_inline(&v, "made[1]", &room) == 1 && props_inline(&v, "made[2]", &room) == 1 &&
          props_inline(&v, "made[3]", &room) == 1, "frozen, sealed, not extensible");
    /* deleting keys, until the shape packs what is left */
    CHECK(eval_ok(v.ctx, "for (var i = 0; i < 12; i++) delete big['k' + i]; void 0"), "the script threw");
    CHECK(props_inline(&v, "big", &room) == 1, "after deleting 12 of 16 keys: room %d", room);
    CHECK(eval_is(v.ctx, "Object.keys(big) + ' ' + JSON.stringify(big)", "k12,k13,k14,k15 {\"k12\":12,\"k13\":13,\"k14\":14,\"k15\":15}"),
          "keys after deleting");
    /* one key more than the room moves them out */
    CHECK(eval_ok(v.ctx, "big.x = 1; big.y = 2; void 0"), "the script threw");
    CHECK(eval_is(v.ctx, "Object.keys(big) + ' ' + big.k12 + big.k15 + big.x + big.y", "k12,k13,k14,k15,x,y 121512"),
          "keys after adding");
    CHECK(eval_ok(v.ctx, "var o2 = { a: 1, b: 2 }; for (var i = 0; i < 20; i++) o2['n' + i] = i; void 0"), "the script threw");
    CHECK(props_inline(&v, "o2", &room) == 0, "20 more keys: they moved out");
    CHECK(eval_is(v.ctx, "o2.a + o2.b + o2.n19 + Object.keys(o2).length", "44"), "values after growing");
    /* once an object of a literal got a key, the next objects of that literal have
       their properties apart, and the objects of other literals do not */
    CHECK(eval_ok(v.ctx, "function mk() { return { q1: 1, q2: 2, q3: 3 } }\n"
                         "function other() { return { r1: 1, r2: 2, r3: 3 } }\n"
                         "var m1 = mk(), m2 = mk(); void 0"), "the script threw");
    CHECK(props_inline(&v, "m1", &room) == 1 && props_inline(&v, "m2", &room) == 1, "two objects of a literal");
    CHECK(eval_ok(v.ctx, "m1.q4 = 4; var m3 = mk(), o1 = other(); o1.r4 = 4; void 0"), "the script threw");
    CHECK(props_inline(&v, "m1", &room) == 0, "the object that got a key");
    CHECK(props_inline(&v, "m2", &room) == 1, "the object made before it");
    CHECK(props_inline(&v, "m3", &room) == 0, "an object made after it: room %d", room);
    CHECK(props_inline(&v, "other()", &room) == 0, "an object of a literal with the same number of keys that got a key");
    CHECK(props_inline(&v, "({ s1: 1, s2: 2, s3: 3 })", &room) == 1, "an object of a literal with other keys");
    CHECK(eval_is(v.ctx, "[m1, m2, m3].map(o => o.q1 + o.q2 + o.q3).join() + Object.keys(m1) + Object.keys(m3)", "6,6,6q1,q2,q3,q4q1,q2,q3"),
          "values of the objects");
    /* objects in a cycle, under a WeakRef and as the key of a WeakMap are freed by the
       collectors, and the block of one a weak reference still needs stays until it goes */
    CHECK(eval_ok(v.ctx, "var wm = new WeakMap(), wr = [], keep = [];\n"
                         "for (var i = 0; i < 60; i++) {\n"
                         "  var c = { self: null, n: i, w: 'x' + i }; c.self = c;\n"
                         "  var d = { a: i, b: { c } };\n"
                         "  wr.push(new WeakRef(c)); wm.set(c, i); wm.set(d, i);\n"
                         "  if (i % 10 == 0) { keep.push(c); c.z = 1; c.y = 2; c.x = 3 }\n"
                         "}\n"
                         "void 0"), "the script threw");
    JS_RunYoungGC(v.rt);
    JS_RunGC(v.rt);
    CHECK(eval_is(v.ctx, "keep.length + ' ' + keep[3].n + keep[3].self.w + keep[5].x", "6 30x303"), "the objects the script kept");
    CHECK(eval_ok(v.ctx, "keep = null; wr = null; wm = null; c = d = null; void 0"), "the script threw");
    JS_RunGC(v.rt);
    JS_RunGC(v.rt);
    CHECK(eval_ok(v.ctx, "lit = big = o2 = made = null; void 0"), "the script threw");
    JS_RunGC(v.rt);
    vm_free(&v);
}

/* Whether chk(arg) ran and returned true: a check that throws is a failed
   check, not a passed one (JS_ToBool says -1 for an exception). */
static int check_true(JSContext *ctx, JSValueConst chk, JSValueConst arg)
{
    JSValue r = JS_Call(ctx, chk, JS_UNDEFINED, 1, &arg);
    int ok;

    if (JS_IsException(r)) {
        JS_FreeValue(ctx, JS_GetException(ctx));
        return 0;
    }
    ok = JS_ToBool(ctx, r) == 1;
    JS_FreeValue(ctx, r);
    return ok;
}

/* A call of the allocator that fails while an object grows or packs its
   properties, or while the first object of a literal is made, leaves the
   object as it was (or no object), and the heap whole (vm_free finds a
   leak). Every call is failed in turn. */
static void test_object_inline_oom(void)
{
    static const char setup[] =
        "function mk() { return { a: 1, b: [2], c: 'three' } }\n"
        "function mk16() { return { k0: 0, k1: 1, k2: 2, k3: 3, k4: 4, k5: 5, k6: 6, k7: 7,\n"
        "                          k8: 8, k9: 9, k10: 10, k11: 11, k12: 12, k13: 13, k14: 14, k15: 15 } }\n"
        "function grow(o) { for (var i = 0; i < 8; i++) o['n' + i] = i; return o }\n"
        "function shrink(o) { for (var i = 0; i < 12; i++) delete o['k' + i]; return o }\n"
        /* the properties leave the block, then a delete packs them in a block of their own */
        "function growshrink(o) { grow(o); return shrink(o) }\n"
        /* the shapes {a}, {a,b}, {a,b,c} and {a,b,c,d} are in the shape hash table, with room for 2, 3, 3 and 4 */
        "var keep = [{}, {}]; for (var o of keep) o.a = 1; for (var o of keep) o.b = [2]; for (var o of keep) o.c = 'three'; for (var o of keep) o.d = 4;\n"
        "function mk3() { return { a: 1, b: [2], c: 'three' } }\n"
        "function addd(o) { o.d = 4; return o }\n"
        /* the first run of this literal makes its shape */
        "function mkfirst() { return { fa: 1, fb: [2], fc: 'three' } }\n"
        "function checkf(o) { return Object.keys(o).join() == 'fa,fb,fc' && o.fa === 1 && o.fb[0] === 2 && o.fc === 'three' }\n"
        "function check3(o) { return Object.keys(o).slice(0, 3).join() == 'a,b,c' && o.a === 1 && o.b[0] === 2 && o.c === 'three' }\n"
        "function check16(o) { var ks = Object.keys(o); for (var i = 12; i < 16; i++) if (o['k' + i] !== i) return false; return ks.length >= 4 }\n"
        "function checkbad(o) { throw new Error('check failed') }\n"
        "void 0";
    static const struct {
        const char *name, *mk, *op, *chk;
        int factory; /* op makes the object, there is none before it */
        int moves;   /* the object leaves its block when op succeeds */
    } kinds[] = {
        { "grow", "mk", "grow", "check3", 0, 1 },
        { "shrink", "mk16", "shrink", "check16", 0, 0 },
        { "grow then shrink", "mk16", "growshrink", "check16", 0, 1 },
        { "found shape", "mk3", "addd", "check3", 0, 1 },
        { "first run", NULL, "mkfirst", "checkf", 1, 0 },
    };
    long cases = 0, failed_calls = 0;
    int kind;

    /* the check must see a check that throws as a failure */
    {
        VM v = vm_new();
        JSValue global, chk;

        CHECK(eval_ok(v.ctx, setup), "the script threw");
        global = JS_GetGlobalObject(v.ctx);
        chk = JS_GetPropertyStr(v.ctx, global, "checkbad");
        CHECK(!check_true(v.ctx, chk, JS_UNDEFINED), "a check that throws passed");
        JS_FreeValue(v.ctx, chk);
        JS_FreeValue(v.ctx, global);
        vm_free(&v);
    }
    for (kind = 0; kind < (int)countof(kinds); kind++) {
        int n = 0, fail;

        for (fail = 0; fail <= 1 || fail <= n; fail++) {
            VM v = vm_new();
            JSValue g, mk = JS_UNDEFINED, chk, obj = JS_UNDEFINED, r, global;
            int made, room;

            CHECK(eval_ok(v.ctx, setup), "the script threw");
            global = JS_GetGlobalObject(v.ctx);
            if (kinds[kind].mk)
                mk = JS_GetPropertyStr(v.ctx, global, kinds[kind].mk);
            g = JS_GetPropertyStr(v.ctx, global, kinds[kind].op);
            chk = JS_GetPropertyStr(v.ctx, global, kinds[kind].chk);
            if (!kinds[kind].factory) {
                obj = JS_Call(v.ctx, mk, JS_UNDEFINED, 0, NULL);
                CHECK(white_props_inline(obj, &room) == 1, "%s: the object starts in its block", kinds[kind].name);
            }
            r = white_call_failing(v.ctx, g, JS_UNDEFINED, !kinds[kind].factory, &obj, fail, &made);
            if (fail == 0)
                n = made;
            cases++;
            if (JS_IsException(r)) {
                failed_calls++;
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            } else if (JS_HasException(v.ctx)) {
                /* the call worked and an error is still pending: a delete packs the properties, which
                   is optional, and a failed packing must leave nothing behind */
                CHECK(0, "%s with call %d failing: the call worked and left an error pending", kinds[kind].name, fail);
                JS_FreeValue(v.ctx, r);
                JS_FreeValue(v.ctx, JS_GetException(v.ctx));
            } else if (kinds[kind].factory) {
                CHECK(check_true(v.ctx, chk, r), "%s with call %d failing: the new object is wrong", kinds[kind].name, fail);
                JS_FreeValue(v.ctx, r);
            } else {
                JS_FreeValue(v.ctx, r);
            }
            if (kinds[kind].factory) {
                /* the shape the failed run may have made or left must serve the next run */
                r = JS_Call(v.ctx, g, JS_UNDEFINED, 0, NULL);
                CHECK(!JS_IsException(r) && check_true(v.ctx, chk, r), "%s with call %d failing: the next object is wrong", kinds[kind].name, fail);
                JS_FreeValue(v.ctx, r);
            } else {
                CHECK(check_true(v.ctx, chk, obj), "%s with call %d failing: the object lost a property", kinds[kind].name, fail);
                if (fail == 0 && kinds[kind].moves)
                    CHECK(white_props_inline(obj, &room) == 0, "%s: the properties left the block", kinds[kind].name);
                if (fail == 0 && !kinds[kind].moves)
                    CHECK(white_props_inline(obj, &room) == 1, "%s: the properties stay in the block", kinds[kind].name);
                if (fail == 0 && !strcmp(kinds[kind].name, "found shape")) {
                    JSValue k = JS_Eval(v.ctx, "keep[0]", 7, "<unit>", JS_EVAL_TYPE_GLOBAL);

                    CHECK(white_shape_of(obj) == white_shape_of(k), "found shape: the object took the shape that was in the shape hash table");
                    JS_FreeValue(v.ctx, k);
                }
            }
            JS_FreeValue(v.ctx, obj);
            JS_FreeValue(v.ctx, chk);
            JS_FreeValue(v.ctx, g);
            JS_FreeValue(v.ctx, mk);
            JS_FreeValue(v.ctx, global);
            vm_free(&v);
        }
        CHECK(n > 0, "%s makes no call of the allocator", kinds[kind].name);
    }
    CHECK(failed_calls > 0, "no call of %ld failed", cases);
    printf("unit-white: test_object_inline_oom: %ld cases, %ld failed calls\n", cases, failed_calls);
}


/* A string buffer that holds more than JS_COPY_STEP bytes and has to grow is
   copied by hand, JS_COPY_STEP bytes at a time with the flag read after each
   step, and not by the allocator in one call. Growing it by one character
   copies the header and the 1500000 characters it holds. Widening it (a
   character above 0xff) copies them the same way and then widens the 1500000
   characters, and no stretch of work is longer than a step of the copy. */
static void test_buffer_copy_work(void)
{
    VM v = vm_new();
    struct StringBuffer *b;
    unsigned long hdr = white_jsstring_header();

    b = white_sb_new(v.ctx, 1500000, 0, 1500000);
    CHECK(b != NULL, "the buffer");
    if (b) {
        white_work_reset();
        CHECK(white_sb_write8(b, (const uint8_t *)"y", 1) == 0, "the write failed");
        CHECK(white_work_sum() == hdr + 1500000, "grown: %lu units of work, want %lu", white_work_sum(), hdr + 1500000);
        CHECK(white_work_longest() <= (1 << 20), "grown: %lu units of work between two readings of the flag", white_work_longest());
        white_sb_free(v.ctx, b);
    }
    b = white_sb_new(v.ctx, 2000000, 0, 1500000);
    CHECK(b != NULL, "the buffer");
    if (b) {
        white_work_reset();
        CHECK(white_sb_putc16(b, 0x4e2d) == 0, "the putc failed");
        CHECK(white_work_sum() == hdr + 1500000 + 1500000, "widened: %lu units of work, want %lu", white_work_sum(), hdr + 3000000);
        CHECK(white_work_longest() <= (1 << 20), "widened: %lu units of work between two readings of the flag", white_work_longest());
        white_sb_free(v.ctx, b);
    }
    vm_free(&v);
}

/* The loops that read the interrupt flag after so many units of work say how
   much they did (JS_WORK in quickjs.c), and white.c adds it up between two
   readings of the flag. A call below does a known amount of work in all (the
   characters a pad writes, a slice scans and copies, a key's hash walks, and
   a leaf of a rope counts one), and no stretch between two readings, nor the
   one before the first or the one after the last, is longer than the step.
   The count of readings alone cannot show that: a loop that read the flag
   twice as often in one place and not at all in another reads as often. */
static void test_poll_work_gaps(void)
{
    enum { ROPE_STEP = 1 << 16, PAD_STEP = 1 << 16, SUB_STEP = 1 << 20 };
    static const char setup[] =
        "var f1 = 'ab'.repeat(65535) + 'c', f2 = 'ab'.repeat(32768), f3 = 'ab'.repeat(32769) + 'c'\n"
        "var wide = ['\\u4e00', '\\u00e9'.repeat(2 << 20)].join('')\n"
        "var wlast = ['\\u00e9'.repeat(3 << 20), '\\u4e00'].join('')\n"
        "var w15 = ['\\u4e00', '\\u00e9'.repeat(3 << 19)].join('')\n"
        "var nar = '\\u00e9'.repeat((3 << 20) + 5)\n"
        "var l1 = 'x'.repeat(65534), l2 = 'y'.repeat(65536), l3 = 'z'.repeat(65535), l4 = 'w'.repeat(65533)\n"
        "var flat = 'ab'.repeat(1 << 20)\n"
        "var ra = l1 + l2 + l3, rb = l1 + l2 + l3\n"
        "var fl = 'ab'.repeat(1 << 19), fl2 = 'ab'.repeat(1 << 19), mp = new Map([[fl, 1]])\n"
        "var kx = 'x'.repeat(1 << 21), ky = 'y'.repeat(1 << 21), ky2 = 'y'.repeat(1 << 21), ko = {}; ko[ky] = 1\n"
        "var h1 = 'x'.repeat(131071), h2 = 'x'.repeat(131071), hm = new Map([[h1, 1]]), ho = {}; ho[h1] = 1\n"
        "var g1 = 'x'.repeat(65537), g2 = 'x'.repeat(65537), gm = new Map([[g1, 1]]), go = {}; go[g1] = 1\n"
        /* 64 keys of the same hash and length that differ from cq at the first character of the pair A or B: (256*263+519 == 257*263+256) */
        "var A = '\\u0100\\u0207', B = '\\u0101\\u0100', P = 'x'.repeat(65532)\n"
        "function tail(i) { var r = []; for (var j = 0; j < 6; j++) r.push((i >> j & 1) ? A : B); return r.join('') }\n"
        "var ck = [], cm = new Map(), co = {}; for (var i = 0; i < 64; i++) { var k = [P, B, tail(i)].join(''); ck.push(k); cm.set(k, i); co[k] = i }\n"
        "var cq = [P, A, tail(0)].join('')\n"
        /* JSON text: strings of one run, of escapes, of runs and escapes, a long ASCII run and then a character of 3 bytes */
        "var ja = '\"' + 'a'.repeat(65537) + '\"'\n"
        "var je = '\"' + '\\\\n'.repeat(40000) + '\"'\n"
        "var jm = '\"' + ('a'.repeat(32768) + '\\\\n').repeat(3) + '\"'\n"
        "var ju = '\"' + '\\\\u0061'.repeat(20000) + '\"'\n"
        "var jw = ['\"', 'a'.repeat(70000), '\\u4e2d\"'].join(''), jwr = '\"' + 'a'.repeat(70000) + '\\u4e2d\"'\n"
        "var jx = '\"' + 'a'.repeat(70000) + '\\\\u4e2d\"'\n"
        "var jk = '{\"' + 'z'.repeat(200000) + '\":1}'\n"
        "var pn = 'a'.repeat(70000)\n"
        "var qa = 'a'.repeat(200000), qe = '\\n'.repeat(70000), qm = ('a'.repeat(32768) + '\\n').repeat(3), qw = '\\u4e2d'.repeat(70000)\n"
        "var e1 = 'x'.repeat(200000), e2 = 'x'.repeat(200000), e3 = 'x'.repeat(199999) + 'y'\n"
        "var x1 = '\\u4e2d'.repeat(200000), x2 = '\\u4e2d'.repeat(200000)\n"
        "var s0 = new Set(), c0; 0";
    static const struct {
        const char *call;
        unsigned long step, total;
    } cases[] = {
        /* a filler that ends in the middle of a step, or that is longer than one */
        { "'x'.padEnd(400000, f1)", PAD_STEP, 399999 },
        { "'x'.padStart(400000, f1)", PAD_STEP, 399999 },
        { "'x'.padEnd(400000, f2)", PAD_STEP, 399999 },
        { "'x'.padStart(400000, f3)", PAD_STEP, 399999 },
        { "'x'.padEnd(400000, 'abc')", PAD_STEP, 399999 },
        { "'x'.padEnd(400000, ' ')", PAD_STEP, 399999 },
        /* a wide character before or after a long narrow string: the buffer is widened (the characters before it), or the narrow string is written into a wide buffer */
        { "pn.padStart(70001, '\\u4e2d')", ROPE_STEP, 70001 },
        { "pn.padEnd(70001, '\\u4e2d')", ROPE_STEP, 70001 },
        /* a scan of 2097152 characters and a copy of as many, and a scan that stops at the first wide one */
        { "wide.slice(1)", SUB_STEP, 4194304 },
        /* the scan ends in the middle of a step, and the copy goes on from there */
        { "w15.slice(1)", SUB_STEP, 3 << 20 },
        { "wlast.slice(0, 3 << 20)", SUB_STEP, 6291456 },
        { "wlast.slice(1)", SUB_STEP, 6291456 },
        /* a string of 8 bits is copied, not scanned */
        { "nar.slice(3)", SUB_STEP, (3 << 20) + 2 },
        { "(' ' + nar).trim()", SUB_STEP, (3 << 20) + 5 },
        /* the hash of a key: leaves of 65534, 65536 and 65535 characters, and one for each leaf */
        { "s0.has(l1 + l2)", ROPE_STEP, 65534 + 65536 + 2 },
        { "s0.has(l3 + l2 + l1)", ROPE_STEP, 65535 + 65536 + 65534 + 3 },
        { "s0.has(l2 + l1 + l2 + l1)", ROPE_STEP, 2 * 65536 + 2 * 65534 + 4 },
        { "s0.has(l1 + l1 + l1 + l1 + l1)", ROPE_STEP, 5 * 65534 + 5 },
        { "s0.has(l4 + l1 + l3 + l2)", ROPE_STEP, 65533 + 65534 + 65535 + 65536 + 4 },
        { "s0.has(flat)", ROPE_STEP, 1 << 21 },
        /* equality: leaf by leaf, and a flat string in steps (each step counts one more than its characters, and a step is 65535) */
        { "fl === fl2", ROPE_STEP, (1 << 20) + 17 },
        /* ropes of 65534, 65536 and 65535 characters: 6 steps */
        { "ra === rb", ROPE_STEP, 196605 + 6 },
        { "ra < rb", ROPE_STEP, 196605 + 6 },
        { "mp.get(fl2)", ROPE_STEP, (1 << 21) + 17 },
        /* a key of a property: the hash, and the comparison with the atom of an equal string */
        { "ko[kx] = 1", ROPE_STEP, 1 << 21 },
        { "ko[ky2] = 2", ROPE_STEP, (1 << 22) + 33 },
        /* the tail of the hash and the head of the compare do not share a step: 131071 is 2 steps and 1 character, 65537 is 1 and 2 */
        { "hm.get(h2)", ROPE_STEP, 131071 + 131071 + 3 },
        { "ho[h2] = 2", ROPE_STEP, 131071 + 131071 + 3 },
        { "gm.get(g2)", ROPE_STEP, 65537 + 65537 + 2 },
        { "go[g2] = 2", ROPE_STEP, 65537 + 65537 + 2 },
        /* 64 keys of the hash of cq, each different from it in the first step: the compares of the chain share the budget and a compare that ends at a difference counts what it did */
        { "cm.get(cq)", ROPE_STEP, 65546 + 64 * 65536 },
        { "cm.has(cq)", ROPE_STEP, 65546 + 64 * 65536 },
        { "cm.delete(cq)", ROPE_STEP, 65546 + 64 * 65536 },
        { "co[cq] = 1", ROPE_STEP, 65546 + 64 * 65536 },
        /* a search for a string of the length of the one searched: the compare goes in steps */
        { "e1.indexOf(e2)", ROPE_STEP, 200000 },
        { "e1.indexOf(e3)", ROPE_STEP, 200000 },
        { "e1.lastIndexOf(e2)", ROPE_STEP, 200000 },
        { "e1.includes(e2)", ROPE_STEP, 200000 },
        { "e1.startsWith(e2)", ROPE_STEP, 200000 },
        { "e1.endsWith(e2)", ROPE_STEP, 200000 },
        { "e1.replace(e2, 'y').length", ROPE_STEP, 200000 },
        { "e1.split(e2).length", ROPE_STEP, 200000 },
        { "x1.indexOf(x2)", ROPE_STEP, 200000 },
        /* Array.prototype.indexOf, lastIndexOf and includes, and Object.is: equal strings of 200000 characters are 4 steps of 65535 */
        { "[1, e1].indexOf(e2)", ROPE_STEP, 200000 + 4 },
        { "[e1, 1].lastIndexOf(e2)", ROPE_STEP, 200000 + 4 },
        { "[1, e1].includes(e2)", ROPE_STEP, 200000 + 4 },
        { "Object.is(e1, e2)", ROPE_STEP, 200000 + 4 },
        /* JSON.parse: the C string (once for each character of an 8-bit string without escapes, and once more for a string that has to be converted), then the bytes of the string, escapes with their bytes, and the characters before a wide one when the buffer is widened */
        { "JSON.parse(ja)", ROPE_STEP + 5, 65539 + 65537 },
        { "JSON.parse(je)", ROPE_STEP + 5, 80002 + 80000 },
        { "JSON.parse(jm)", ROPE_STEP + 5, 98312 + 98310 },
        { "JSON.parse(ju)", ROPE_STEP + 5, 120002 + 120000 },
        { "JSON.parse(jw)", ROPE_STEP + 5, 70003 + 70003 + 70000 },
        /* a rope is made flat first, which widens the characters before the wide one too (1 for the quote) */
        { "JSON.parse(jwr)", ROPE_STEP + 5, 70003 + 70003 + 70000 + 70001 },
        { "JSON.parse(jx)", ROPE_STEP + 5, 70008 + 70006 + 70000 },
        /* the same for a key: and the hash of it */
        { "JSON.parse(jk)", ROPE_STEP + 5, 200006 + 200000 + 200000 },
        /* JSON.stringify: runs, escapes and runs of wide characters */
        { "JSON.stringify(qa).length", ROPE_STEP, 200000 },
        { "JSON.stringify(qe).length", ROPE_STEP, 70000 },
        { "JSON.stringify(qm).length", ROPE_STEP, 3 * (32768 + 1) },
        /* the quote that opens the string is widened by the first run: 1 more */
        { "JSON.stringify(qw).length", ROPE_STEP + 1, 70000 + 1 },
    };
    VM v = vm_new();
    size_t i;

    CHECK(eval_ok(v.ctx, setup), "the script threw");
    for (i = 0; i < countof(cases); i++) {
        unsigned long longest, total;

        white_work_reset();
        CHECK(eval_ok(v.ctx, cases[i].call), "%s: the script threw", cases[i].call);
        longest = white_work_longest();
        total = white_work_sum();
        CHECK(longest <= cases[i].step, "%s: %lu units of work between two readings of the flag, at most %lu", cases[i].call,
              longest, cases[i].step);
        CHECK(total == cases[i].total, "%s: %lu units of work, want %lu", cases[i].call, total, cases[i].total);
    }
    vm_free(&v);
}

/* A literal of 28 or 29 plain keys keeps its properties in its block (the
   header of 40 bytes and 29 properties of 16 are 504 bytes, the most a small
   block of the allocator holds), one of 30 does not. */
static void test_object_inline_limit(void)
{
    int n;

    for (n = 27; n <= 31; n++) {
        VM v = vm_new();
        char src[2048] = "var o = { ";
        int room = 0, k, inline_;
        char want[32];

        for (k = 0; k < n; k++)
            snprintf(src + strlen(src), sizeof(src) - strlen(src), "q%d: %d, ", k, k);
        strcat(src, "}; void 0");
        CHECK(eval_ok(v.ctx, src), "the script threw");
        inline_ = props_inline(&v, "o", &room);
        CHECK(inline_ == (n <= 29), "%d keys: in the block %d, room for %d", n, inline_, room);
        CHECK(room == n, "%d keys: room for %d", n, room);
        snprintf(want, sizeof(want), "%d:26", n);
        CHECK(eval_is(v.ctx, "Object.keys(o).length + ':' + o.q26", want), "values");
        vm_free(&v);
    }
}

static uintptr_t shape_of(VM *v, const char *expr)
{
    JSValue o = JS_Eval(v->ctx, expr, strlen(expr), "<unit>", JS_EVAL_TYPE_GLOBAL);
    uintptr_t r = white_shape_of(o);

    JS_FreeValue(v->ctx, o);
    return r;
}

/* Object literals of the same keys in the same order share one shape, made
   at whichever ran first, wherever they are written and whether or not the
   empty shape of Object.prototype and the shapes in between exist: with the
   shape cache on and off. A shape an object got key by key is taken too, also
   when it has room for more properties than the literal has keys. Keys in
   another order or with other flags are another shape. */
static void test_object_lit_shared_shape(void)
{
    int cache;

    for (cache = 0; cache <= 1; cache++) {
        VM v = vm_new();
        int room = -1;

        JS_SetShapeCache(v.ctx, cache ? 2048 : 0);
        CHECK(eval_ok(v.ctx, "function f1() { return { zz1: 1 } }\n"
                             "function f2() { return { zz1: 2 } }\n"
                             "var a = f1(), b = f2(), c = f1(); void 0"), "the script threw");
        CHECK(shape_of(&v, "a") != 0 && shape_of(&v, "a") == shape_of(&v, "b"),
              "cache %d: {zz1} in two functions has two shapes", cache);
        CHECK(shape_of(&v, "a") == shape_of(&v, "c"), "cache %d: one literal, two shapes", cache);
        CHECK(eval_is(v.ctx, "a.zz1 + ',' + b.zz1 + ',' + Object.keys(b)", "1,2,zz1"), "values of {zz1}");

        /* three keys, two places, and a literal written by eval */
        CHECK(eval_ok(v.ctx, "function g1(x) { return { yy1: x, yy2: 2, yy3: 3 } }\n"
                             "function g2(x) { return { yy1: x, yy2: 'b', yy3: [3] } }\n"
                             "var d = g1(1), e = g2(2), f = eval('({ yy1: 0, yy2: 0, yy3: 0 })'); void 0"),
              "the script threw");
        CHECK(shape_of(&v, "d") == shape_of(&v, "e") && shape_of(&v, "d") == shape_of(&v, "f"),
              "cache %d: {yy1, yy2, yy3} in three places", cache);

        /* the order of the keys and their flags are part of the shape */
        CHECK(eval_ok(v.ctx, "var p = (() => ({ pp1: 1, pp2: 2 }))(), q = (() => ({ pp2: 1, pp1: 2 }))();\n"
                             "var fl = {}; Object.defineProperty(fl, 'vv1', { value: 1, enumerable: true });\n"
                             "var lit = (() => ({ vv1: 2 }))(); void 0"), "the script threw");
        CHECK(shape_of(&v, "p") != shape_of(&v, "q"), "cache %d: keys in another order share a shape", cache);
        CHECK(shape_of(&v, "fl") != shape_of(&v, "lit"), "cache %d: a key with other flags shares a shape", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify(Object.getOwnPropertyDescriptor(lit, 'vv1'))",
                      "{\"value\":2,\"writable\":true,\"enumerable\":true,\"configurable\":true}"),
              "the key of the literal");
        CHECK(eval_is(v.ctx, "lit.vv1 = 5; delete lit.vv1; JSON.stringify(lit) + JSON.stringify(fl)", "{}{\"vv1\":1}"),
              "the literal after a write and a delete");

        /* a shape built key by key, with room for 6 properties, kept by its
           object: the literal of 5 keys takes it, room and all, and keeps it
           when the object grows on */
        CHECK(eval_ok(v.ctx, "var kk = {}; kk.kk1 = 1; kk.kk2 = 2; kk.kk3 = 3; kk.kk4 = 4; kk.kk5 = 5;\n"
                             "function lit5() { return { kk1: 'a', kk2: 'b', kk3: 'c', kk4: 'd', kk5: 'e' } }\n"
                             "var l1 = lit5(); kk.kk6 = 6; kk.kk7 = 7; var l2 = lit5(); void 0"),
              "the script threw");
        CHECK(props_inline(&v, "kk", &room) == 0 && room > 6, "cache %d: kk grown: room %d", cache, room);
        CHECK(props_inline(&v, "l1", &room) == 1 && room == 6,
              "cache %d: the literal of the shape of kk: room %d", cache, room);
        CHECK(shape_of(&v, "l1") == shape_of(&v, "l2") && shape_of(&v, "l1") != shape_of(&v, "kk"),
              "cache %d: the literal kept its shape when kk grew", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([l1, l2, kk])",
                      "[{\"kk1\":\"a\",\"kk2\":\"b\",\"kk3\":\"c\",\"kk4\":\"d\",\"kk5\":\"e\"},"
                      "{\"kk1\":\"a\",\"kk2\":\"b\",\"kk3\":\"c\",\"kk4\":\"d\",\"kk5\":\"e\"},"
                      "{\"kk1\":1,\"kk2\":2,\"kk3\":3,\"kk4\":4,\"kk5\":5,\"kk6\":6,\"kk7\":7}]"),
              "values of the objects");
        CHECK(eval_is(v.ctx, "l1.kk6 = 'f'; l1.kk7 = 'g'; Object.keys(l1) + ' ' + Object.keys(l2) + ' ' + l1.kk7",
                      "kk1,kk2,kk3,kk4,kk5,kk6,kk7 kk1,kk2,kk3,kk4,kk5 g"), "the literal grows on its own");

        /* an object of one literal that got a key turns off the room in the
           block for the next objects of every literal of its shape */
        CHECK(eval_ok(v.ctx, "function h1() { return { ww1: 1, ww2: 2 } }\n"
                             "function h2() { return { ww1: 3, ww2: 4 } }\n"
                             "var x1 = h1(), x2 = h2(); x1.ww3 = 3; var x3 = h2(); void 0"), "the script threw");
        CHECK(props_inline(&v, "x2", &room) == 1, "cache %d: an object made before the key", cache);
        CHECK(props_inline(&v, "x3", &room) == 0, "cache %d: an object of the other literal made after the key", cache);
        CHECK(eval_is(v.ctx, "JSON.stringify([x1, x2, x3])",
                      "[{\"ww1\":1,\"ww2\":2,\"ww3\":3},{\"ww1\":3,\"ww2\":4},{\"ww1\":3,\"ww2\":4}]"), "values of the objects");
        vm_free(&v);
    }
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
    W_GOSUB, W_RET, W_WITH_GET_VAR, W_PUSH_I32, W_PUSH_I16, W_OBJECT_LIT,
    W_TAIL_CALL, W_TAIL_CALL_METHOD,
    W_GET_FIELD, W_GET_FIELD2, W_PUSH_ATOM_VALUE, W_STRICT_EQ,
    W_GET_ARG0_GET_FIELD, W_GET_ARG1_GET_FIELD, W_GET_LOC0_GET_FIELD,
    W_GET_LOC8_GET_FIELD, W_DUP_CASE_ATOM,
    W_TYPEOF, W_TYPEOF_IS_UNDEFINED, W_TYPEOF_IS_FUNCTION, W_TYPEOF_IS, W_LNOT,
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
/* read_extra_flags: flags added to the ones run_read reads with. */
static int read_extra_flags;

static void run_read(JSContext *ctx, const uint8_t *buf, size_t size, char *out, size_t cap)
{
    JSValue top, fun, r;
    char *t;
    int32_t n = -1;

    top = JS_ReadObject(ctx, buf, size, JS_READ_OBJ_BYTECODE | read_extra_flags);
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

/* A read with JS_READ_OBJ_POLLS_CHECKED does not run poll_scan again: the
   bytes were read once without the flag, which holds every function to the
   rule of the polls. So code that breaks the rule is read, where without the
   flag it is a SyntaxError (nothing runs it here). The rest of what
   check_polls makes stays: the places OP_ret may go, so OP_ret to a place
   that follows no OP_gosub is still an InternalError, and a finally block
   still runs. */
#define POLLS_CHECKED (1 << 5) /* JS_READ_OBJ_POLLS_CHECKED */

static void test_poll_checked_read(void)
{
    VM v = vm_new();
    char *src = long_func(400, 0), got[256];
    const char *fin;
    const uint8_t *code;
    uint8_t *buf, *copy;
    JSValue top, r;
    size_t size;
    int len, places[64], n, i;
    const uint8_t *at;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile a long function");
    n = white_nops(code, len, places, countof(places));
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the long function in what JS_WriteObject wrote");
    if (at) {
        copy = malloc(size);
        memcpy(copy, buf, size);
        for (i = 0; i < n && i < (int)countof(places); i++)
            copy[at - buf + places[i]] = white_op(W_PUSH_0);
        r = JS_ReadObject(v.ctx, copy, size, JS_READ_OBJ_BYTECODE);
        CHECK(JS_IsException(r), "code without its polls is read without the flag");
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        r = JS_ReadObject(v.ctx, copy, size, JS_READ_OBJ_BYTECODE | POLLS_CHECKED);
        CHECK(!JS_IsException(r), "code without its polls is read with the flag");
        JS_FreeValue(v.ctx, r);
        free(copy);
    }
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    free(src);

    read_extra_flags = POLLS_CHECKED;
    {
        Code c = {{0}, 0};

        push_i32(&c, 7);
        op_n(&c, W_RET, 1);
        push_i32(&c, 0);
        c.b[7] = white_op(W_RETURN_UNDEF);
        op_n(&c, W_RETURN_UNDEF, 1);
        read_code(v.ctx, &c, got, sizeof(got));
        CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret into an instruction with the flag: %s", got);
        c = (Code){{0}, 0};
        push_i32(&c, 6);
        op_n(&c, W_RET, 1);
        op_n(&c, W_RETURN_UNDEF, 1);
        read_code(v.ctx, &c, got, sizeof(got));
        CHECK(strstr(got, "InternalError: invalid ret value"), "OP_ret to no OP_gosub with the flag: %s", got);
    }
    fin = "(function () { var t = 0; for (var i = 0; i < 3; i++) { try { t += i } finally { t += 10 } } return t })";
    top = JS_Eval(v.ctx, fin, strlen(fin), "<poll>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    if (buf)
        run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(buf && strcmp(got, "ok 33") == 0, "a finally block read back with the flag: %s", buf ? got : "(none)");
    read_extra_flags = 0;
    js_free(v.ctx, buf);
    JS_FreeValue(v.ctx, top);
    vm_free(&v);
}

/* An object literal of plain keys is one OP_object_lit, whose operands are
   the number of its values and the index of its keys and shape in the
   function. Its bytecode reads back and runs, but not with either operand
   changed so that they no longer match the keys the function holds
   (SyntaxError: invalid object literal). The function holds no atom in its
   code, so that the code is in what JS_WriteObject writes as it is in
   memory. */
/* A call of a C function in the place of a return compiles to a tail call,
   which the interpreter makes without JS_CallInternal too (the recursions of
   TestCCallDepth that end with these return are made of them). */
static void test_tail_c_calls(void)
{
    static const struct { const char *src; int op; } cases[] = {
        { "function f() { return [1].map(f) }", W_TAIL_CALL_METHOD },
        { "const apply = Reflect.apply; function f() { return apply(f, null, []) }", W_TAIL_CALL },
        { "const abs = Math.abs; function f(x) { return abs(x) }", W_TAIL_CALL },
    };
    VM v = vm_new();
    int shape[2 * 64];

    for (size_t i = 0; i < countof(cases); i++) {
        const uint8_t *code;
        JSValue top;
        int len, n, found = 0;

        CHECK(code_of(v.ctx, cases[i].src, &top, &code, &len) == 0, "compile %s", cases[i].src);
        n = code ? white_code_shape(code, len, shape, 64) : -1;
        for (int k = 0; k < n; k++)
            found |= shape[2 * k] == white_op(cases[i].op);
        CHECK(found, "%s: no tail call in %d instructions", cases[i].src, n);
        JS_FreeValue(v.ctx, top);
    }
    vm_free(&v);
}


/* The compiler joins each pair of fused_runs into one opcode, unless a jump
   lands inside the pair or JS_SetFusedOps turned it off. poll_scan holds
   code read by JS_ReadObject to the rest of each run, and code written with
   the opcodes reads back and runs. */
static void test_fused_ops(void)
{
    static const char five[] =
        "(function () { function f(o, p) { var a = o.k1, b = p.k2, c = 1, d = 2, e = p;\n"
        "  var s = a.k4 + e.k3; switch (s) { case 'aa': return 1; case 'bb': return 2 } return 3 }\n"
        "  return f({ k1: { k4: 'a' } }, { k2: 0, k3: 'a' }) + 10 * f({ k1: { k4: 'b' } }, { k3: 'b' }) +\n"
        "    100 * f({ k1: {} }, { k3: 'x' }) })";
    static const char *names[5] = { "get_arg0 get_field", "get_arg1 get_field", "get_loc0 get_field",
                                    "get_loc8 get_field", "dup push_atom_value strict_eq if_false8" };
    VM v = vm_new();
    JSValue top;
    const uint8_t *code;
    uint8_t *buf, *at;
    size_t size;
    int len, count[5], n, on, k;
    char got[256];
    Code c;

    /* the inner function f, compiled alone, has each of the five; with the
       pairs off, none */
    for (on = 1; on >= 0; on--) {
        static const char inner[] =
            "function f(o, p) { var a = o.k1, b = p.k2, c = 1, d = 2, e = p;\n"
            "  var s = a.k4 + e.k3; switch (s) { case 'aa': return 1; case 'bb': return 2 } return 3 }";
        /* a case for each of the two strings */
        JS_SetFusedOps(v.ctx, on);
        CHECK(code_of(v.ctx, inner, &top, &code, &len) == 0, "compile f");
        n = code ? white_fused_ops(code, len, count) : -1;
        for (k = 0; k < 5; k++)
            CHECK(count[k] == (on ? (k == 4 ? 2 : 1) : 0), "on %d: %s: %d", on, names[k], count[k]);
        JS_FreeValue(v.ctx, top);
    }
    JS_SetFusedOps(v.ctx, 1);

    /* a jump to the get_field of a pair: the pair stays apart */
    CHECK(code_of(v.ctx, "function f(o, c) { return (c ? o : o).x }", &top, &code, &len) == 0, "compile");
    n = code ? white_fused_ops(code, len, count) : -1;
    CHECK(n == 0, "a jump into the pair: %d fused opcodes", n);
    JS_FreeValue(v.ctx, top);
    CHECK(code_of(v.ctx, "function f(o, c) { return o.x }", &top, &code, &len) == 0, "compile");
    n = code ? white_fused_ops(code, len, count) : -1;
    CHECK(n == 1 && count[0] == 1, "no jump: %d fused opcodes", n);
    JS_FreeValue(v.ctx, top);
    CHECK(eval_is(v.ctx, "(function f(o, c) { return (c ? o : o).x })({ x: 'jumped' }, 1)", "jumped"), "a jump into the pair runs");

    /* written and read back, it runs, and not with the rest of a run changed */
    top = JS_Eval(v.ctx, five, strlen(five), "<fused>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    JS_FreeValue(v.ctx, top);
    CHECK(buf != NULL, "write");
    if (buf) {
        run_read(v.ctx, buf, size, got, sizeof(got));
        CHECK(strcmp(got, "ok 321") == 0, "read back: %s", got);
        for (at = buf; at + 1 < buf + size; at++) {
            if (at[0] == white_op(W_GET_ARG1_GET_FIELD) && at[1] == white_op(W_GET_FIELD))
                break;
        }
        CHECK(at + 1 < buf + size, "no get_arg1 get_field in the bytecode written");
        if (at + 1 < buf + size) {
            at[1] = white_op(W_GET_FIELD2);
            run_read(v.ctx, buf, size, got, sizeof(got));
            CHECK(strstr(got, "invalid bytecode: a fused opcode without the instructions it reads") != NULL,
                  "read with the get_field changed: %s", got);
        }
        js_free(v.ctx, buf);
    }

    /* poll_scan: each fused opcode followed by the rest of its run */
    c = (Code){{0}, 0};
    op_n(&c, W_GET_ARG0_GET_FIELD, 1);
    jump(&c, W_GOTO); /* 5 bytes of nothing, to be the get_field */
    c.b[c.n - 5] = white_op(W_GET_FIELD);
    op_n(&c, W_DROP, 1);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "get_arg0 get_field");
    c.b[1] = white_op(W_GET_FIELD2);
    CHECK(scan_fails(&c, "a fused opcode without"), "get_arg0 then get_field2");
    c.n = 1;
    CHECK(scan_fails(&c, "a fused opcode without"), "get_arg0 get_field at the end");
    c = (Code){{0}, 0};
    op_n(&c, W_GET_LOC8_GET_FIELD, 2); /* the index is a second byte of the opcode */
    c.b[1] = 0;
    jump(&c, W_GOTO);
    c.b[c.n - 5] = white_op(W_GET_FIELD);
    op_n(&c, W_RETURN_UNDEF, 1);
    CHECK(scan_ok(&c), "get_loc8 get_field");
    c.b[2] = white_op(W_DROP);
    CHECK(scan_fails(&c, "a fused opcode without"), "get_loc8 then something else");
    for (k = 0; k < 3; k++) {
        int iff;
        c = (Code){{0}, 0};
        op_n(&c, W_PUSH_0, 1);
        op_n(&c, W_DUP_CASE_ATOM, 1);
        jump(&c, W_GOTO);
        c.b[c.n - 5] = white_op(W_PUSH_ATOM_VALUE);
        op_n(&c, W_STRICT_EQ, 1);
        iff = jump(&c, W_IF_FALSE8);
        op_n(&c, W_DROP, 1);
        set_target(&c, iff, c.n);
        op_n(&c, W_RETURN_UNDEF, 1);
        if (k == 0) {
            CHECK(scan_ok(&c), "dup push_atom_value strict_eq if_false8");
        } else {
            c.b[k == 1 ? 7 : 8] = white_op(k == 1 ? W_DROP : W_GOTO8);
            CHECK(scan_fails(&c, "a fused opcode without"), "the case with instruction %d changed", k + 1);
        }
    }
    vm_free(&v);
}
/* typeof x compared with a string is one opcode: typeof_is with the string,
   typeof_is_undefined and typeof_is_function for those two, a negated test
   takes the other branch or lnot, and typeof x < "u" or > "u" is
   typeof_is_undefined. With JS_SetFusedOps off, only the forms of before
   typeof_is are joined. Written and read back, the string comes along. */
static void test_typeof_is(void)
{
    static const struct {
        const char *body;
        int op, lnot; /* with the opcodes on */
        int off_op;   /* off: the opcode, or W_TYPEOF when none */
    } cases[] = {
        { "return typeof x == 'string'", W_TYPEOF_IS, 0, W_TYPEOF },
        { "return typeof x === `number`", W_TYPEOF_IS, 0, W_TYPEOF },
        { "return typeof x !== 'object'", W_TYPEOF_IS, 1, W_TYPEOF },
        { "if (typeof x != 'symbol') return 1", W_TYPEOF_IS, 0, W_TYPEOF },
        { "if (typeof x == 'bigint') return 1", W_TYPEOF_IS, 0, W_TYPEOF },
        { "return typeof x < 'u'", W_TYPEOF_IS_UNDEFINED, 1, W_TYPEOF },
        { "return typeof x > `u`", W_TYPEOF_IS_UNDEFINED, 0, W_TYPEOF },
        { "if (typeof x < 'u') return 1", W_TYPEOF_IS_UNDEFINED, 0, W_TYPEOF },
        { "return typeof x === 'undefined'", W_TYPEOF_IS_UNDEFINED, 0, W_TYPEOF_IS_UNDEFINED },
        { "return typeof x != 'function'", W_TYPEOF_IS_FUNCTION, 1, W_TYPEOF },
        { "if (typeof x !== 'function') return 1", W_TYPEOF_IS_FUNCTION, 0, W_TYPEOF_IS_FUNCTION },
        { "return typeof x <= 'u'", W_TYPEOF, 0, W_TYPEOF },
        { "return typeof x < 'v'", W_TYPEOF, 0, W_TYPEOF },
        { "return typeof x == 1", W_TYPEOF, 0, W_TYPEOF },
    };
    static const char script[] =
        "(function () { function f(x) { return typeof x == 'string' ? 1 : 0 }\n"
        "  function g(x) { return typeof x !== 'object' && typeof x < 'u' ? 1 : 0 }\n"
        "  return f('a') * 100 + f(1) * 10 + g(1) + g(null) * 2 + g(undefined) * 4 })";
    VM v = vm_new();
    int shape[2 * 64];
    char src[256], got[256];
    uint8_t *buf;
    size_t size;

    for (int on = 1; on >= 0; on--) {
        JS_SetFusedOps(v.ctx, on);
        for (size_t i = 0; i < countof(cases); i++) {
            const uint8_t *code;
            JSValue top;
            int len, n, ops = 0, lnot = 0, left = 0;
            int want = on ? cases[i].op : cases[i].off_op;

            snprintf(src, sizeof(src), "function f(x) { %s }", cases[i].body);
            CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile %s", src);
            n = code ? white_code_shape(code, len, shape, 64) : -1;
            for (int k = 0; k < n; k++) {
                ops += shape[2 * k] == white_op(want);
                lnot += shape[2 * k] == white_op(W_LNOT);
                left += shape[2 * k] == white_op(W_TYPEOF);
            }
            CHECK(ops == 1, "on %d: %s: %d of the opcode", on, cases[i].body, ops);
            CHECK(want == W_TYPEOF || left == 0, "on %d: %s: typeof left", on, cases[i].body);
            CHECK(lnot == (on && cases[i].lnot), "on %d: %s: %d lnot", on, cases[i].body, lnot);
            JS_FreeValue(v.ctx, top);
        }
    }
    JS_SetFusedOps(v.ctx, 1);

    {
        JSValue top = JS_Eval(v.ctx, script, strlen(script), "<typeof>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
        buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(v.ctx, top);
    }
    CHECK(buf != NULL, "write");
    if (buf) {
        run_read(v.ctx, buf, size, got, sizeof(got));
        CHECK(strcmp(got, "ok 101") == 0, "read back: %s", got);
        js_free(v.ctx, buf);
    }
    vm_free(&v);
}

static void test_object_lit_read(void)
{
    VM v = vm_new();
    const char *src = "(function () { var n = 0; for (var k in { a: 1, b: 2 }) n++; return n })";
    const uint8_t op[5] = { white_op(W_OBJECT_LIT), 2, 0, 0, 0 };
    const uint8_t *code, *at, *lit;
    uint8_t *buf, *copy;
    size_t size;
    JSValue top;
    char got[256];
    int len;

    CHECK(code_of(v.ctx, src, &top, &code, &len) == 0, "compile an object literal");
    lit = memmem(code, len, op, sizeof(op));
    CHECK(lit != NULL, "OP_object_lit 2 0 in the code of an object literal");
    buf = JS_WriteObject(v.ctx, &size, top, JS_WRITE_OBJ_BYTECODE);
    at = buf ? memmem(buf, size, code, len) : NULL;
    CHECK(at != NULL, "the code of the object literal in what JS_WriteObject wrote");
    run_read(v.ctx, buf, size, got, sizeof(got));
    CHECK(strcmp(got, "ok 2") == 0, "read back: %s", got);
    if (at && lit) {
        copy = malloc(size);
        memcpy(copy, buf, size);
        copy[at - buf + (lit - code) + 1] = 1;
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid object literal"), "one value for two keys: %s", got);
        memcpy(copy, buf, size);
        copy[at - buf + (lit - code) + 3] = 1;
        run_read(v.ctx, copy, size, got, sizeof(got));
        CHECK(strstr(got, "SyntaxError: invalid object literal"), "the keys of a second literal: %s", got);
        free(copy);
    }
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

/* ---- strings built by appending ---- */

/* A string built by appending to a variable grows in place, in the room its
   copies get: it stays a flat string past JS_STRING_ROPE_SHORT2_LEN, where
   QuickJS makes a rope of it, whatever kind of variable it is in. */
static void test_string_append_flat(void)
{
    static const struct {
        const char *what, *src;
    } ways[] = {
        { "closure variable", "(() => { let s = ''; const add = x => { s += x }; for (let i = 0; i < 20000; i++) add(i % 7 ? 'ab' : '\u00e9'); return s })()" },
        { "argument", "((n, s) => { s = ''; for (let i = 0; i < n; i++) s += i % 7 ? 'ab' : '\u4e2d'; return s })(20000)" },
        { "local", "(() => { const f = i => i % 7 ? 'ab' : 'c'; let s = ''; for (let i = 0; i < 20000; i++) s = s + f(i); return s })()" },
        { "local by add_loc", "(() => { let s = ''; for (let i = 0; i < 20000; i++) s += 'ab'; return s })()" },
    };
    size_t i;

    for (i = 0; i < countof(ways); i++) {
        VM v = vm_new();
        JSValue r = JS_Eval(v.ctx, ways[i].src, strlen(ways[i].src), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSValue len;

        CHECK(!JS_IsException(r), "%s: threw", ways[i].what);
        CHECK(white_string_is_rope(r) == 0, "%s: built %s", ways[i].what,
              white_string_is_rope(r) == 1 ? "a rope" : "no string");
        len = JS_GetPropertyStr(v.ctx, r, "length");
        CHECK(JS_VALUE_GET_TAG(len) == JS_TAG_INT && JS_VALUE_GET_INT(len) > 30000,
              "%s: built a string of length %d", ways[i].what, JS_VALUE_GET_INT(len));
        JS_FreeValue(v.ctx, len);
        JS_FreeValue(v.ctx, r);
        vm_free(&v);
    }
}

/* A string a variable holds can be an atom too, whose table holds the same
   string without counting it. Appending to it makes a new string: grown in
   place, the atom would change under its hash, and the key it names would no
   longer be the key of its text. Both ways a variable can be appended to in
   place: a closure variable that the add stores into (the stack holds the
   second reference), and a local that add_loc grows (the only reference). */
#define ATOM_TAIL ", with a tail that makes it longer than 64 characters"

static void test_string_append_atom(void)
{
    static const struct {
        const char *what, *holder;
    } ways[] = {
        { "closure variable", "var holder = (() => { let s; return [v => { s = v }, x => (s += x)] })()" },
        { "let local", "var holder = (() => { let held; return [v => { held = v }, x => { let t = held; held = undefined; t += x; return t }] })()" },
        { "var local, add_loc", "var holder = (() => { let held; return [v => { held = v }, x => { var t = held; held = undefined; t += x; return t }] })()" },
    };
    size_t i;

    /* An atom grows in place only into the room at the end of its block.
       Where every block is a request to the system malloc (asan), a block
       has no more room than was asked for, and nothing would be checked. */
    if (white_every_block_is_a_request()) {
        printf("unit-white: test_string_append_atom skipped, every block is a request in this build\n");
        return;
    }
    for (i = 0; i < countof(ways); i++) {
        VM v = vm_new();
        /* longer than JS_STRING_APPEND_MIN, so that an add takes the path of
           JS_ConcatStringAppend */
        JSAtom a = JS_NewAtom(v.ctx, "appended atom" ATOM_TAIL);
        JSValue s = JS_AtomToString(v.ctx, a), g, holder, set, r;

        CHECK(eval_ok(v.ctx, ways[i].holder), "%s: no holder", ways[i].what);
        g = JS_GetGlobalObject(v.ctx);
        holder = JS_GetPropertyStr(v.ctx, g, "holder");
        set = JS_GetPropertyUint32(v.ctx, holder, 0);
        r = JS_Call(v.ctx, set, JS_UNDEFINED, 1, &s);
        JS_FreeValue(v.ctx, r);
        CHECK(white_string_room(v.ctx, s) > 0, "%s: the atom has no room to grow into, the test checks nothing", ways[i].what);
        /* then the variable holds the only reference to the atom's string */
        JS_FreeValue(v.ctx, s);
        JS_FreeAtom(v.ctx, a);
        /* no literal of the atom's text in the scripts: it would hold the atom */
        CHECK(eval_ok(v.ctx, "var k = holder[1]('!')"), "%s: the append threw", ways[i].what);
        CHECK(eval_is(v.ctx, "var o = {}; o[k] = 1; String(o['appended ' + 'atom" ATOM_TAIL "!'])", "1"),
              "%s: the atom was changed in place", ways[i].what);
        JS_FreeValue(v.ctx, set);
        JS_FreeValue(v.ctx, holder);
        JS_FreeValue(v.ctx, g);
        vm_free(&v);
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

/* ---- source text and line tables shared with a buffer ---- */

/* With JS_READ_OBJ_SHARED_DEBUG the source text and line table of a function
   point into the buffer read, the function says so, the runtime neither
   counts nor frees them, and they hold what a copy holds. */
static void test_shared_debug_read(void)
{
    enum { PAD = 64 << 10 };
    static const char head[] = "function f(a) {\n\t/*", tail[] = "*/\n\treturn new Error(a).stack\n}\n";
    char *js = malloc(sizeof(head) + PAD + sizeof(tail));
    VM c = vm_new();
    JSValue compiled;
    uint8_t *out, *buf;
    size_t n;
    char *sources[2] = { NULL, NULL };
    JSMemoryUsage usage[2];
    int i;

    memcpy(js, head, sizeof(head) - 1);
    memset(js + sizeof(head) - 1, 'y', PAD);
    strcpy(js + sizeof(head) - 1 + PAD, tail);
    compiled = JS_Eval(c.ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
    CHECK(!JS_IsException(compiled), "the script did not compile");
    out = JS_WriteObject(c.ctx, &n, compiled, JS_WRITE_OBJ_BYTECODE);
    CHECK(out != NULL, "the bytecode was not written");
    buf = malloc(n);
    memcpy(buf, out, n);
    js_free(c.ctx, out);
    JS_FreeValue(c.ctx, compiled);
    vm_free(&c);
    free(js);

    for (i = 0; i < 2; i++) {
        int flags = JS_READ_OBJ_BYTECODE | (i ? JS_READ_OBJ_SHARED_DEBUG : 0);
        int shared = i, pooled, f_shared;
        const uint8_t *src, *pc;
        size_t src_len;
        VM v = vm_new();
        JSValue script = JS_ReadObject(v.ctx, buf, n, flags);
        int in_src, in_pc;

        CHECK(JS_VALUE_GET_TAG(script) == JS_TAG_FUNCTION_BYTECODE, "the bytecode was not read");
        pooled = white_function_debug(script, 0, &src, &src_len, &pc, &f_shared);
        CHECK(pooled == 1, "the script has %d constants, want f alone", pooled);
        in_src = src >= buf && src < buf + n;
        in_pc = pc >= buf && pc < buf + n;
        CHECK(in_src == shared && in_pc == shared && f_shared == shared,
              "flags %d: source in the buffer %d, line table %d, bit %d", flags, in_src, in_pc, f_shared);
        sources[i] = strndup((const char *)src, src_len);
        JS_ComputeMemoryUsage(v.rt, &usage[i]);
        /* freeing the functions must leave the buffer to its owner */
        JS_FreeValue(v.ctx, script);
        vm_free(&v);
    }
    CHECK(strcmp(sources[0], sources[1]) == 0 && strncmp(sources[0], "function f(a) {", 15) == 0,
          "source %.40s, shared %.40s", sources[0], sources[1]);
    CHECK(usage[1].js_func_pc2line_size == 0, "the runtime counts %lld bytes of shared line tables",
          (long long)usage[1].js_func_pc2line_size);
    CHECK(usage[0].malloc_size - usage[1].malloc_size >= PAD,
          "shared, the runtime takes %lld bytes less, want at least %d",
          (long long)(usage[0].malloc_size - usage[1].malloc_size), PAD);
    CHECK(usage[0].js_func_size - usage[1].js_func_size >= PAD,
          "shared, the runtime counts %lld bytes less in its functions, want at least %d",
          (long long)(usage[0].js_func_size - usage[1].js_func_size), PAD);
    free(sources[0]);
    free(sources[1]);
    free(buf);
}

/* ---- the regexp cache ---- */

/* The entries the cache of the runtime keeps, newest first (see
   white_regexp_cache), with its lists checked. */
static char *cache_entries(VM *v)
{
    static char out[4096];
    size_t bytes;
    uint32_t buckets;
    int n = white_regexp_cache(v->rt, out, sizeof(out), &bytes, &buckets);

    CHECK(n >= 0, "the lists of the regexp cache are not whole");
    return out;
}

static int cache_count(VM *v)
{
    char out[4096];
    size_t bytes;
    uint32_t buckets;

    return white_regexp_cache(v->rt, out, sizeof(out), &bytes, &buckets);
}

/* Only a pattern that is a string with flags that are undefined or a string
   is kept: anything else is converted again each time, which can run
   javascript. A compile that fails keeps nothing. */
static void test_regexp_cache_keeps(void)
{
    static const char *const fails[] = {
        "new RegExp('(')", "new RegExp('a', 'gg')", "new RegExp('a', 'uv')",
        "new RegExp('a', 'q')", "new RegExp('\\\\p{Nope}', 'u')",
    };
    VM v = vm_new();
    char src[256];
    int before, i, again;

    eval_ok(v.ctx, "var n = 0, f = {toString() { n++; return 'g' }}, p = {toString() { n++; return 'x' }}; 0");
    for (i = 0; i < 3; i++)
        eval_ok(v.ctx, "new RegExp('a', f); new RegExp(p, 'g'); new RegExp(new String('b')); new RegExp('c', new String('i')); 0");
    CHECK(eval_is(v.ctx, "String(n)", "6"), "toString did not run 6 times");
    /* The constructor converts a pattern that is not a regexp to a string
       before it compiles: that string is kept. Flags it hands on as they are. */
    CHECK(strcmp(cache_entries(&v), "b undefined,x g") == 0, "entries %s, want b undefined,x g", cache_entries(&v));
    before = cache_count(&v);
    for (i = 0; i < (int)countof(fails); i++) {
        for (again = 0; again < 2; again++) {
            snprintf(src, sizeof(src), "try { %s; 'made' } catch (e) { e.name }", fails[i]);
            CHECK(eval_is(v.ctx, src, "SyntaxError"), "%s did not throw a SyntaxError", fails[i]);
        }
    }
    CHECK(cache_count(&v) == before, "%d entries after compiles that fail, want %d", cache_count(&v), before);
    vm_free(&v);
}

/* The entries, their table and the strings they hold are the memory the cache
   takes: turning it off gives all of it back, and freeing a runtime with
   entries leaves no block (the host ends the process if one is left). */
static void test_regexp_cache_memory(void)
{
    static const char js[] = "for (var i = 0; i < 300; i++) new RegExp('(a|b)+c' + i, 'g'); 0";
    VM v = vm_new();
    VM w = vm_new();
    char out[64];
    size_t off, on, kept, bytes;
    uint32_t buckets;
    int n;

    JS_SetRegExpCache(v.rt, 0, 0);
    eval_ok(v.ctx, js);
    JS_RunGC(v.rt);
    off = JS_GetMemorySize(v.rt);
    JS_SetRegExpCache(v.rt, 256, 512 << 10);
    eval_ok(v.ctx, js);
    JS_RunGC(v.rt);
    on = JS_GetMemorySize(v.rt);
    n = white_regexp_cache(v.rt, out, sizeof(out), &bytes, &buckets);
    CHECK(n == 256 && on > off, "%d entries, %zu bytes with the cache, %zu without", n, on, off);
    /* The cache counts the usable size of each block: the allocator rounds
       up and adds a header, which the memory size has too. */
    kept = on - off;
    CHECK(kept >= bytes / 2 && kept <= bytes * 2 + (16 << 10), "the cache takes %zu bytes and counts %zu", kept, bytes);
    JS_SetRegExpCache(v.rt, 0, 0);
    JS_RunGC(v.rt);
    CHECK(JS_GetMemorySize(v.rt) == off, "%zu bytes after the cache let go, %zu before it", JS_GetMemorySize(v.rt), off);
    vm_free(&v);
    /* the default cache, freed with its entries */
    eval_ok(w.ctx, js);
    vm_free(&w);
}

/* ---- the split ops of the regexp machine ---- */

/* lre_split steps over at most LRE_SPLIT_BUDGET split ops in one call, and
   then gives back the one it is at to be pushed, so a chain of splits that
   leads back to itself, which lre_compile does not write, cannot hold it. An
   alarm ends the test if it does. */
static void test_regexp_split_budget(void)
{
    int ok;

    alarm(20);
    ok = white_lre_split_budget();
    alarm(0);
    CHECK(ok, "lre_split gave other than the split at 0 and its char at 5");
}

/* ---- compiling with one request to the heap failing ---- */

/* The bits of the mode of a compile: any call of js_malloc_rt or js_realloc_rt
   is a request (small blocks too), and js is a module. */
#define MODE_CALLS 1
#define MODE_MODULE 2

/* compile_failing compiles js in a new runtime where request number fail
   fails, 0 for none, and runs the code it compiled with every request
   working. A request is a request to the heap, or with MODE_CALLS a call of
   js_malloc_rt or js_realloc_rt (small blocks too). It puts the result of the
   code as a string in result (room cap), or "exception" when it threw, and
   returns 1 when the compile failed, else 0. *n is the number of requests of
   the compile. A compile may only fail with the out of memory error. */
static int compile_failing(const char *what, const char *js, int fail, int mode, int *n, char *result, size_t cap)
{
    VM v = vm_new();
    JSValue c = white_compile_failing(v.ctx, js, fail, mode, n);
    int failed = JS_IsException(c);

    result[0] = 0;
    if (failed) {
        int oom = JS_HasException(v.ctx) && JS_IsUncatchableException(v.ctx);
        char *t = exception_text(v.ctx);

        CHECK(oom, "%s, request %d failing: the compile fails with %s, not the out of memory error", what, fail, t ? t : "no exception");
        free(t);
    } else {
        JSValue r;
        char *t;

        CHECK(!JS_HasException(v.ctx), "%s, request %d failing: the compile works, but leaves an exception", what, fail);
        r = JS_EvalFunction(v.ctx, c);
        t = value_text(v.ctx, r);
        snprintf(result, cap, "%s", t ? t : "exception");
        free(t);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, r);
    }
    if (failed)
        JS_FreeValue(v.ctx, c);
    CHECK(!JS_HasException(v.ctx), "%s, request %d failing: an exception is pending", what, fail);
    vm_free(&v);
    return failed;
}

/* compile_failing_alone is compile_failing in a child process, so that a
   compile that aborts is one failed case with its script and request, not the
   end of the test. It returns 1 when the compile failed with the out of memory
   error, 0 when it worked and gave want, and -1 for any other outcome. */
static int compile_failing_alone(const char *what, const char *js, int fail, int mode, const char *want)
{
    pid_t pid, r;
    int st;

    fflush(stdout);
    pid = fork();
    if (pid < 0) {
        perror("fork");
        exit(2);
    }
    if (pid == 0) {
        char got[256];
        int n, failed;

        failures = 0;
        failed = compile_failing(what, js, fail, mode, &n, got, sizeof(got));
        if (!failed)
            CHECK(strcmp(got, want) == 0, "%s, request %d failing: the code compiled gives %s, want %s", what, fail, got, want);
        fflush(stdout);
        host_leave(failures ? 1 : failed ? 10 : 0);
    }
    while ((r = waitpid(pid, &st, 0)) < 0 && errno == EINTR)
        ;
    if (r != pid) {
        perror("waitpid");
        exit(2);
    }
    if (WIFEXITED(st) && (WEXITSTATUS(st) == 0 || WEXITSTATUS(st) == 10))
        return WEXITSTATUS(st) == 10;
    CHECK(0, "%s, request %d failing: the case %s", what, fail,
          WIFSIGNALED(st) ? "was stopped by a signal (the engine aborted)" : "failed");
    return -1;
}

/* More scripts for the sweep where any block may fail: each uses a place of
   the parser that looks ahead, patches code it wrote before, or adds to a
   table (the constant pool, the closure variables, the labels). */
static const char *const compile_alloc_scripts[] = {
    /* a destructuring pattern that is not an arrow function or a declaration:
       the parser looks ahead to the token after its closing bracket */
    "[aa, bb] = [1, 2]; ({cc, dd} = {cc: 1, dd: 2}); (ee, ff); [(gg), hh] = [3, 4]; aa + hh",
    "var a, b; for ([a, b] of [[1, 2]]) ; for ({a, b} of [{a: 1, b: 2}]) ; for ([a, b] in {x: 1}) ; typeof a",
    "var {x: {y}, z: [w]} = {x: {y: 1}, z: [2]}; ({p: [q]} = {p: [3]}); var q; y + w + q",
    /* parentheses and templates the lookahead reads through */
    "var o = {a: 1}; var t = (o.a, (o.a + 1) * 2, [o.a, (o)]); `x${(1, 2)}y${`z${(3)}`}` + (1) + t.length",
    /* class fields: the function that sets them, computed keys, static blocks */
    "class A { a = 1; ['b' + 1] = 2; static c = 3; static { A.d = 4 } #e = 5; e() { return this.#e } }; const o = new A(); o.a + o.b1 + A.c + A.d + o.e()",
    /* private methods, accessors and static ones, with a brand check */
    "class A { #m() { return 1 } get #g() { return 2 } set #g(v) {} static #s() { return 3 } static t(o) { return #m in o } u() { return this.#m() + this.#g + A.#s() } }; new A().u() + A.t(new A())",
    /* derived class with fields, an arrow in the constructor and super */
    "class B { constructor(x) { this.x = x } } class A extends B { f = () => this.x; constructor(a = 1, ...r) { super(a); this.r = r } }; new A(2, 3).f() + new A(4).r.length",
    /* a class with only static fields, one with only a brand, an empty one */
    "class A { static a = 1; static b = A.a + 1 } class B { #p = 1; static has(o) { return #p in o } } class C {} A.b + B.has(new B()) + typeof C",
    /* arrow functions: parameters that need a lookahead to tell from parens */
    "const f = (a, b = (1, 2), {c, d = 3} = {}, [e, ...g] = [5]) => a + b + c + d + e + g.length; const h = async (x) => await x; (a => a)(1) + f(1, undefined, {c: 4}) + ((x, y) => x * y)(2, 3)",
    "var v = 1; (v, (v), ((v))); ((a, b) => (a, b))(1, 2) + (typeof (() => {}))",
    /* regexp literals and flags, with a group name read before a class escape */
    "const a = /[a-z]+(?<n>\\d)\\k<n>/giu, b = /(?:x|y)+?z$/m, c = /\\p{L}+/v; 'ab11'.match(a)[0] + b.test('xyz') + c.test('ab') + /[/]\\//.source",
    "const r = /(?<year>\\d{4})-(?<month>\\d\\d)|[\\w&&[^\\d]]\\s\\S\\W\\D/gs; 'x 2024-05'.replace(r, '$<month>') + /\\u{1F600}/u.test('x')",
    /* templates, tagged and nested, with escapes (and a bad escape in a tag) */
    "const t = (s, ...v) => s.raw.length + v.length; `a${`b${1 + 1}c`}d` + t`x\\n${1}y${2}z` + `\\u{41}${'q'}`",
    "const t = (s, ...v) => String(s[0]) + s.raw[0] + v.length; t`\\unicode and \\xerxes ${1} \\u{110000}`",
    /* destructuring with defaults in every place */
    "var {a = 1, b: {c = 2, ...d} = {}, ...e} = {z: 1}; var [f = 3, [g = 4] = [], ...h] = []; function k({p = 5} = {}, [q = 6] = []) { return p + q } let l, m; ({l = 7, m = 8} = {}); a + c + f + g + k() + l + m + Object.keys(e).length",
    "var a, b, c; [a, [b = 2], ...c] = [1, [], 3, 4]; ({x: a, y: [b], ...c} = {x: 1, y: [2], z: 3}); for ([a, b] of [[1, 2]]) ; for ({a, b} of [{a: 1, b: 2}]) ; for (var {x, y = 2} of [{x: 1}]) ; a + b + x + y",
    "function f({a, b} = {}, [c, d] = [], ...[e, f]) { return a + b + c + d + e + f } const g = async ({a}, [b]) => a + b; f({a: 1, b: 2}, [3, 4], 5, 6)",
    /* generators, async generators, for await and labels */
    "async function* g() { yield 1; yield* [2, 3] } async function f() { let s = 0; outer: for await (const x of g()) { for (const y of [1, 2]) { if (y > 1) continue outer; s += x } } return s } f().then(v => globalThis.r = v); typeof g",
    /* getters and setters, computed names, spread, optional chaining */
    "const k = 'p'; const o = { get [k]() { return 1 }, set [k](v) {}, ['q' + 1]: 2, ...{r: 3}, m() { return super.toString }, async *n() {} }; o?.p + o.q1 + o.r + (o.zz?.[0] ?? 4) + (o.m?.() ? 1 : 0)",
    /* switch, try/finally and closures that capture loop variables */
    "var fs = []; for (let i = 0; i < 3; i++) { switch (i) { case 0: fs.push(() => i); break; default: try { fs.push(() => i * 2) } finally { i += 0 } } } fs.map(f => f()).join()",
    /* eval and with, which keep variables by name */
    "function f(s) { with ({w: 2}) { return eval(s) + w } } f('var u = 1; u + 1')",
    /* let as a name, async as a name, yield and await as names */
    "var let_ = 1; var async = (x) => x; async\n(1); var yield_ = function* () { var yield2 = yield 1 }; var o = { async, get: 1, set: 2, static: 3 }; async(2) + o.get",
    /* new.target, super calls and properties, arguments, home objects */
    "function F() { return new.target === F } class B { m() { return 1 } } class A extends B { m() { return super.m() + (() => super.m())() + arguments.length } static s() { return super.name } }; new F() instanceof F",
    /* strict mode: directives, names that are checked, duplicate parameters */
    "function f(a, b) { 'use strict'; var eval_ = 1; return (function g(c) { return c })(a + b) } (function () { 'use strict'; return [typeof f, this === undefined] })()",
    /* catch with a pattern, optional catch, finally with a return, labels */
    "function f() { try { throw {message: 'm', code: 1} } catch ({message, code = 2}) { try { throw 1 } catch { return message + code } finally { var z = 1 } } } a: b: for (;;) { break a } f()",
    /* class heritage expression, computed members and arrows in fields */
    "const mix = (B) => class extends B { ['x' + 1] = () => this; static ['y' + 2] = (a = 1) => a; get [Symbol.toStringTag]() { return 'M' } }; class A {} const M = mix(A); new M().x1() instanceof A",
    /* spread in calls and arrays, comma, in operator in a for head, delete, void */
    "var o = {a: 1}, arr = [1, 2]; for (var i = ('a' in o) ? 0 : 1, j = (1, 2); i < 1; i++) ; Math.max(...arr, ...[3], 4) + (delete o.a ? 1 : 0) + (void 0 === undefined) + [...arr, ...'ab'].length",
    /* async arrows and methods with destructuring and defaults */
    "const f = async ({a = 1, ...r} = {}, [b] = [2], c = async () => 3) => a + b + await c(); const o = { async m({x}) { return x }, async *g(a = 1) { yield a }, get v() { return 1 } }; f().then(() => {}); o.v",
    /* try/finally with break, continue and return across loops and a switch */
    "function f(n) { for (var i = 0; i < n; i++) { try { if (i == 1) continue; if (i == 3) break; switch (i) { case 2: return i; default: } } finally { n += 0 } } return -1 } f(5)",
    /* getters and setters on classes with super, static blocks, new.target in arrows */
    "class B { get a() { return 1 } set a(v) {} static m() { return 2 } } class A extends B { get a() { return super.a + 1 } set a(v) { super.a = v } static m() { return super.m() + (() => new.target)() } static { this.k = super.m() } }; new A().a + A.k",
    /* nested functions that close over arguments, this, eval-free vars */
    "function outer(a) { var x = arguments; return function () { return () => [a, x.length, this, new.target, arguments.length] } } outer(1, 2)()()",
    /* object literals: shorthand, methods, __proto__, numeric and string keys */
    "var a = 1, __proto__ = 2; var o = { a, b() {}, 1: 'n', 'c d': 3, [a + 1]: 4, __proto__: null, get 5() { return 5 }, set 'e'(v) {}, async f() {}, *g() {}, async *h() {} }; Object.keys(o).length",
    /* very long expression and a long chain of calls and properties */
    "var x = 1 + 2 * 3 - 4 / 2 % 3 << 1 >> 1 >>> 1 & 7 | 8 ^ 9; var y = x.toString().length.toFixed(1).split('').join('-').toUpperCase(); var z = x < 1 ? y : x > 2 ? 'a' : 'b'; (x && y || z) ?? x",
    /* numbers, bigints, strings with escapes, comments, labels, exponent */
    "var a = 0x1f + 0o7 + 0b11 + 1_000 + 1e3 + .5 + 5. + 08; var b = 10n ** 3n + 0xffn; var c = '\\x41\\u0042\\u{43}\\101\\\n' + \"\\0\"; /* c */ // d\nvar e = 2 ** 3 ** 2, f = (-2) ** 2; l: { break l } a + c.length + e + f + typeof b",
};

/* The modules of the sweep (compiled with MODE_MODULE): exports of every
   form, top level await and import.meta. None has an import: the compile
   loads the modules it imports, and nothing here loads one. */
static const char *const compile_module_scripts[] = {
    "export const a = 1, b = [2]; export let c; export var d = 4; export function f() { return a } export class K { static s = 1 } export default 5; export { a as e, b as default2 }; export { f as 'string name' }",
    "var r = import.meta.url, s = import.meta; import('lazy').then(() => {}, () => {}); export { r, s as meta }; export { r as 'quoted name' }",
    "export default function () { return 1 } export const g = async () => { await 1 }; await Promise.resolve(2); for await (const v of [Promise.resolve(1)]) ; export { g as h }",
    "export default class { static #p = 1; static q() { return #p in this } } export const { a, b: [c = 1] } = { a: 1, b: [] }; export const [d, ...e] = [1, 2, 3]",
};

/* Compiling with one request failing, and the others working, fails with the
   out of memory error or gives code that works as it should. A request is a
   request to the heap (what a large block failing under the memory limit
   looks like, as the small blocks still fit in their arenas), or with calls
   set any call of js_malloc_rt or js_realloc_rt, small blocks too (what every
   block failing alone looks like, as it does in the builds that take each
   block from the system malloc). */
static void test_compile_one_failure(int calls)
{
    static char big_scope[16384], deep_scope[8192], fields[8192], cases[8192];
    struct {
        const char *js;
        int mode;
    } scripts[countof(compile_scripts) + countof(compile_fine_scripts) + countof(compile_alloc_scripts) + countof(compile_module_scripts) + 5];
    size_t i, k = 0;
    long cases_run = 0, bad = 0;
    int j;

    nested_scopes();
    strcpy(big_scope, "function f() { let n = 0; ");
    for (j = 0; j < 400; j++)
        strcat(big_scope, "{ let a = 1; n += a } ");
    strcat(big_scope, "return n } f()");
    deep_scope[0] = 0;
    for (j = 0; j < 300; j++)
        strcat(deep_scope, "{ let a = 1; ");
    for (j = 0; j < 300; j++)
        strcat(deep_scope, "} ");
    strcat(deep_scope, "1");
    strcpy(fields, "class A { #m() { return 1 } ");
    for (j = 0; j < 200; j++)
        strcat(fields, "x = 1; ");
    strcat(fields, "y() { return this.#m() } }; new A().y()");
    strcpy(cases, "switch (2) { ");
    for (j = 0; j < 300; j++)
        strcat(cases, "case 1: break; ");
    strcat(cases, "default: 3 }");
    for (i = 0; i < countof(compile_scripts); i++)
        scripts[k++].js = compile_scripts[i];
    for (i = 0; i < countof(compile_fine_scripts); i++)
        scripts[k++].js = compile_fine_scripts[i];
    for (i = 0; i < countof(compile_alloc_scripts); i++)
        scripts[k++].js = compile_alloc_scripts[i];
    scripts[k++].js = big_scope;
    scripts[k++].js = deep_scope;
    scripts[k++].js = fields;
    scripts[k++].js = cases;
    /* Private methods and no fields: the code that adds the brand is made
       and patched at the end of the class, with no token between. */
    scripts[k++].js = "class A { #m() { return 1 } y() { return this.#m() } }; new A().y()";
    for (i = 0; i < k; i++)
        scripts[i].mode = calls ? MODE_CALLS : 0;
    for (i = 0; i < countof(compile_module_scripts); i++) {
        scripts[k].js = compile_module_scripts[i];
        scripts[k++].mode = (calls ? MODE_CALLS : 0) | MODE_MODULE;
    }
    for (i = 0; i < k; i++) {
        char what[32], want[256];
        int n, fail, failures_of_script = 0, bad_of_script = 0;

        snprintf(what, sizeof(what), "script %zu", i);
        CHECK(!compile_failing(what, scripts[i].js, 0, scripts[i].mode, &n, want, sizeof(want)), "%s does not compile", what);
        for (fail = 1; fail <= n; fail++) {
            int r = compile_failing_alone(what, scripts[i].js, fail, scripts[i].mode, want);

            cases_run++;
            if (r < 0) {
                bad++;
                bad_of_script++;
            } else {
                failures_of_script += r;
            }
        }
        if (bad_of_script)
            printf("unit-white: %s (%.40s): %d of %d requests bad\n", what, scripts[i].js, bad_of_script, n);
        CHECK(failures_of_script > 0, "%s: no compile of its %d requests failed", what, n);
    }
    printf("unit-white: test_compile_one_failure (%s): %ld cases, %ld bad\n", calls ? "every call" : "heap requests", cases_run, bad);
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
    test_object_block();
    test_object_inline_props();
    test_object_inline_oom();
    test_poll_work_gaps();
    test_buffer_copy_work();
    test_object_inline_limit();
    test_object_lit_shared_shape();
    test_young_lists();
    test_young_leaves_no_zero_count();
    test_error_nest();
    test_function_properties_oom();
    test_poll_scan();
    test_poll_insert();
    test_poll_compile();
    test_poll_ret();
    test_poll_checked_read();
    test_object_lit_read();
    test_tail_c_calls();
    test_fused_ops();
    test_typeof_is();
    test_added_vars_oom();
    test_string_append_flat();
    test_string_append_atom();
    test_own_keys_flags();
    test_shared_debug_read();
    test_regexp_cache_keeps();
    test_regexp_cache_memory();
    test_regexp_split_budget();
    test_compile_one_failure(0);
    test_compile_one_failure(1);
    if (failures) {
        printf("unit-white: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-white: %ld checks passed\n", checks);
    return 0;
}
