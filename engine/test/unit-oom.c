/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* unit-oom runs the engine with little memory left, which scripts cannot do:
   a call that runs out of memory fails with the out of memory error (null,
   uncatchable), leaves the runtime working and loses nothing. These are the
   tests of the Go packages that did this on the old engine (lib/outofmemory,
   lib/tocstring_oom, lib/backtrace*, lib/jsonstr, lib/objlit, compileoom,
   fieldchain, uncatchable). Each sweeps the memory it leaves: JS_SetMemoryLimit to what
   the runtime holds plus a room, for rooms from nothing up. A runtime that
   leaks a block ends the process with status 87 (host_free_runtime).

     unit-oom */
#include "cutils.h"
#include "compile-scripts.h"
#include "testutil.h"

/* ---- JS_ThrowError2 ---- */

/* The small blocks of the allocator come from arenas that are already counted,
   so a limit at what the runtime holds still lets a block out of a free slot.
   fill_memory runs a collection, sets the limit to what the runtime holds
   and takes every free block the arenas have left, of each size, so that the
   next allocation that needs new memory fails. take_memory does it with no
   collection first. release_blocks gives them back and lifts the limit.
   blocks[i] holds the blocks of size 8 * i. */
#define MAX_BLOCK 4096
static struct {
    void **p;
    size_t n, cap;
} blocks[MAX_BLOCK / 8 + 1];

static void take_memory(VM *v)
{
    size_t size;

    JS_SetMemoryLimit(v->rt, JS_GetMemorySize(v->rt));
    for (size = MAX_BLOCK; size >= 8; size -= 8) {
        for (;;) {
            void *p = js_malloc_rt(v->rt, size);
            size_t i = size / 8;

            if (!p)
                break;
            if (blocks[i].n == blocks[i].cap) {
                blocks[i].cap = blocks[i].cap ? blocks[i].cap * 2 : 64;
                blocks[i].p = realloc(blocks[i].p, blocks[i].cap * sizeof(void *));
            }
            blocks[i].p[blocks[i].n++] = p;
        }
    }
}

static void fill_memory(VM *v)
{
    JS_RunGC(v->rt);
    take_memory(v);
}

/* free_blocks frees up to n blocks of each size from min to max bytes. */
static void free_blocks(VM *v, size_t min, size_t max, size_t n)
{
    size_t size, k;

    for (size = min; size <= max; size += 8) {
        size_t i = size / 8;

        for (k = 0; k < n && blocks[i].n; k++)
            js_free_rt(v->rt, blocks[i].p[--blocks[i].n]);
    }
}

static void release_blocks(VM *v)
{
    free_blocks(v, 8, MAX_BLOCK, (size_t)-1);
    unlimit(v);
}

/* The rooms of a full heap: case i of fill_rooms frees rm[i].n blocks of each
   size from rm[i].min to rm[i].max after fill_memory. The first frees
   nothing. */
#define FILL_ROOMS (1 + 512 / 8 + 7 * 4)
typedef struct {
    size_t min, max, n;
} FillRoom;

static size_t fill_rooms(FillRoom *rm)
{
    size_t nrooms = 0, size, max, n;

    rm[nrooms].min = 8; rm[nrooms].max = 8; rm[nrooms++].n = 0;
    for (size = 8; size <= 512; size += 8) {
        rm[nrooms].min = size; rm[nrooms].max = size; rm[nrooms++].n = 1;
    }
    for (max = 64; max <= 4096; max *= 2) {
        for (n = 1; n <= 4; n++) {
            rm[nrooms].min = 8; rm[nrooms].max = max; rm[nrooms++].n = n;
        }
    }
    return nrooms;
}

/* When the memory left cannot hold an error, JS_ThrowError2 throws null,
   uncatchable, which the embedder reports as running out of memory. When it
   can hold the error object but not its message, it does the same, and never
   gives the error the exception marker as its message. Each case frees some
   blocks from a full heap (the small blocks of the engine's allocator need
   it), or lets the runtime allocate a number of bytes (which the system
   malloc of the asan build counts to the block), then throws a TypeError for
   a read of a property with a long name. Some rooms must fail the error and
   some must give it whole, so that both ends are checked. */
#define THROW_ERROR_BYTE_ROOMS (1 + 4096 / 16)

/* The error thrown with one room, taken after the memory is back. */
static void throw_error_check(VM *v, JSValue r, JSValue e, int uncatchable,
                              const char *kind, size_t i, int *nulls, int *whole)
{
    JSValue m;

    CHECK(JS_IsException(r), "%s room %zu: no exception", kind, i);
    if (JS_IsNull(e)) {
        (*nulls)++;
        CHECK(uncatchable, "%s room %zu: null thrown, want it uncatchable", kind, i);
    } else if (JS_IsObject(e)) {
        (*whole)++;
        m = JS_GetPropertyStr(v->ctx, e, "message");
        CHECK(JS_IsString(m), "%s room %zu: the message of the error has tag %d, want a string", kind, i, JS_VALUE_GET_TAG(m));
        JS_FreeValue(v->ctx, m);
    } else {
        CHECK(0, "%s room %zu: thrown value has tag %d", kind, i, JS_VALUE_GET_TAG(e));
    }
    JS_FreeValue(v->ctx, e);
}

static void test_throw_error_oom(void)
{
    VM v = vm_new();
    char name[151];
    JSAtom atom;
    FillRoom rm[FILL_ROOMS];
    size_t nrooms, i;
    int nulls = 0, whole = 0;

    memset(name, 'x', 150);
    name[150] = 0;
    /* make the atom of the property name, so that reading it needs no memory */
    atom = JS_NewAtom(v.ctx, name);
    JS_FreeValue(v.ctx, JS_GetProperty(v.ctx, JS_NULL, atom));
    JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    nrooms = fill_rooms(rm);
    for (i = 0; i < nrooms + THROW_ERROR_BYTE_ROOMS; i++) {
        JSValue r, e;
        int uncatchable;

        if (i < nrooms) {
            fill_memory(&v);
            free_blocks(&v, rm[i].min, rm[i].max, rm[i].n);
        } else {
            room(&v, (i - nrooms) * 16);
        }
        r = JS_GetProperty(v.ctx, JS_NULL, atom);
        uncatchable = JS_IsUncatchableException(v.ctx);
        e = JS_GetException(v.ctx);
        JS_SetUncatchableException(v.ctx, 0);
        if (i < nrooms)
            release_blocks(&v);
        else
            unlimit(&v);
        throw_error_check(&v, r, e, uncatchable, i < nrooms ? "fill" : "byte",
                          i < nrooms ? i : (i - nrooms) * 16, &nulls, &whole);
    }
    CHECK(nulls > 0, "no room made the error fail");
    CHECK(whole > 0, "no room gave a whole error: its message is not checked");
    JS_FreeAtom(v.ctx, atom);
    vm_free(&v);
}

/* ---- JS_ToCStringLen2 ---- */

/* JS_ToCStringLen2 takes a reference to its argument. When the memory limit
   makes the UTF-8 copy fail, that reference must still be released, otherwise
   the string outlives JS_FreeRuntime. ASCII strings never allocate a copy, so
   only strings with non-ASCII characters reach this path: 8-bit strings with
   characters >= 0x80 and wide (16-bit) strings take different branches. */
static void test_tocstring_oom(void)
{
    static const char *const kinds[] = { "\xc3\xa9", "\xe4\xb8\xad" };
    int k, i;

    for (k = 0; k < 2; k++) {
        VM v = vm_new();
        char buf[1024];
        JSValue str;
        JSMemoryUsage u;
        int64_t before;
        const char *p;
        size_t len;

        for (i = 0; i < 256; i++)
            memcpy(buf + 2 * i + (k ? i : 0), kinds[k], k ? 3 : 2);
        len = k ? 256 * 3 : 256 * 2;
        /* the blocks in the caches are blocks in use as far as the heap
           counts them, and a request that fails gives them back: count the
           blocks with the caches empty */
        JS_SetFrontCache(v.rt, 256);
        JS_ComputeMemoryUsage(v.rt, &u);
        before = u.malloc_count;
        str = JS_NewStringLen(v.ctx, buf, len);
        CHECK(JS_IsString(str), "%s: JS_NewStringLen: tag %d", k ? "wide" : "latin1", JS_VALUE_GET_TAG(str));
        /* no room for another allocation: the UTF-8 copy must fail */
        room(&v, 0);
        p = JS_ToCStringLen2(v.ctx, &len, str, 0);
        CHECK(p == NULL, "%s: JS_ToCStringLen2 succeeded, want an out of memory failure", k ? "wide" : "latin1");
        if (p)
            JS_FreeCString(v.ctx, p);
        unlimit(&v);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, str);
        JS_SetFrontCache(v.rt, 256);
        JS_ComputeMemoryUsage(v.rt, &u);
        CHECK(u.malloc_count == before, "%s: string not freed after a failed JS_ToCStringLen2: %lld allocations before, %lld after",
              k ? "wide" : "latin1", (long long)before, (long long)u.malloc_count);
        vm_free(&v);
    }
}

/* ---- JS_DefinePropertyValue ---- */

/* When add_property runs out of memory, JS_DefinePropertyValue releases the
   reference it was given, returns -1 and leaves the error pending, and the
   object stays as it was. Each case needs memory for the new property: a copy
   of a shape other objects share, or a larger property array. A reference
   that is not released is a block still in use when the runtime is freed (the
   value is made with JS_NewObject and given away). */
static void test_define_oom(void)
{
    static const char *const objects[] = { "({})", "({x: 1, y: 2})" };
    int c;
    VM v = vm_new();
    JSAtom prop = JS_NewAtom(v.ctx, "objlitOutOfMemory");

    for (c = 0; c < 2; c++) {
        JSValue obj = JS_Eval(v.ctx, objects[c], strlen(objects[c]), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSValue val = JS_NewObject(v.ctx);
        int r;

        fill_memory(&v);
        r = JS_DefinePropertyValue(v.ctx, obj, prop, val, JS_PROP_C_W_E);
        release_blocks(&v);
        CHECK(r == -1, "%s: JS_DefinePropertyValue = %d, want -1", objects[c], r);
        CHECK(JS_HasException(v.ctx), "%s: no pending exception", objects[c]);
        free(exception_text(v.ctx));
        CHECK(JS_HasProperty(v.ctx, obj, prop) == 0, "%s: the object has the property", objects[c]);
        /* with memory again, the same definition works */
        r = JS_DefinePropertyValue(v.ctx, obj, prop, JS_NewObject(v.ctx), JS_PROP_C_W_E);
        CHECK(r == 1, "%s: %d after the memory came back, want 1", objects[c], r);
        JS_FreeValue(v.ctx, obj);
    }
    JS_FreeAtom(v.ctx, prop);
    vm_free(&v);
}

/* ---- the name of a new function ---- */

/* A function made by JS_NewHostFunction or JS_NewCFunction2 with little
   memory left is either whole, with its name and length and no exception
   pending, or the call returns the exception with the out of memory error
   pending, and nothing it made is left (vm_free). The name is a number,
   whose atom is an index atom that holds no string: the string of the name
   property is made after the function. Three sweeps: the bytes the limit
   leaves, a full heap with a few blocks given back (fill_rooms), and a full
   heap with blocks of every size but one given back, so that only the
   requests of that size fail. (The string of the name never fails alone
   here, as making the atom frees a string of the same size just before.
   unit-white fails it alone.) */
#define FUNCTION_NAME_BYTE_ROOMS (1 + 1024 / 8)
#define FUNCTION_NAME_BUT_ONE (1024 / 8)

static JSValue function_name_noop(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    return JS_UNDEFINED;
}

static void test_function_name_oom(void)
{
    static const char *const kinds[] = { "JS_NewHostFunction", "JS_NewCFunction2" };
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), k;
    int kind;

    for (kind = 0; kind < 2; kind++) {
        int failed = 0, made = 0;

        for (k = 0; k < FUNCTION_NAME_BYTE_ROOMS + nfill + FUNCTION_NAME_BUT_ONE; k++) {
            VM v = vm_new();
            JSValue f, e, name, len;
            int bytes = k < FUNCTION_NAME_BYTE_ROOMS, butone = k >= FUNCTION_NAME_BYTE_ROOMS + nfill, pending, unc;
            size_t n = bytes ? k * 8 : butone ? 8 * (k - FUNCTION_NAME_BYTE_ROOMS - nfill + 1) : k - FUNCTION_NAME_BYTE_ROOMS;
            const char *sweep = bytes ? "byte" : butone ? "all but" : "fill";
            char *t;

            if (bytes) {
                room(&v, n);
            } else if (butone) {
                fill_memory(&v);
                free_blocks(&v, 8, n - 8, 4);
                free_blocks(&v, n + 8, MAX_BLOCK, 4);
            } else {
                fill_memory(&v);
                free_blocks(&v, rm[n].min, rm[n].max, rm[n].n);
            }
            if (kind == 0)
                f = JS_NewHostFunction(v.ctx, "12345", 5, 2, 7);
            else
                f = JS_NewCFunction2(v.ctx, function_name_noop, "12345", 2, JS_CFUNC_generic, 0);
            pending = JS_HasException(v.ctx);
            unc = JS_IsUncatchableException(v.ctx);
            if (bytes)
                unlimit(&v);
            else
                release_blocks(&v);
            if (JS_IsException(f)) {
                failed++;
                CHECK(pending && unc, "%s %s room %zu: pending %d, uncatchable %d", kinds[kind], sweep, n, pending, unc);
                JS_SetUncatchableException(v.ctx, 0);
                e = JS_GetException(v.ctx);
                JS_FreeValue(v.ctx, e);
            } else {
                made++;
                CHECK(!pending, "%s %s room %zu: a function came back with an exception pending", kinds[kind], sweep, n);
                free(exception_text(v.ctx));
                name = JS_GetPropertyStr(v.ctx, f, "name");
                len = JS_GetPropertyStr(v.ctx, f, "length");
                t = value_text(v.ctx, name);
                CHECK(t && !strcmp(t, "12345") && JS_VALUE_GET_TAG(len) == JS_TAG_INT && JS_VALUE_GET_INT(len) == 2,
                      "%s %s room %zu: name %s (tag %d), length tag %d", kinds[kind], sweep, n,
                      t ? t : "?", JS_VALUE_GET_TAG(name), JS_VALUE_GET_TAG(len));
                free(t);
                JS_FreeValue(v.ctx, name);
                JS_FreeValue(v.ctx, len);
                JS_FreeValue(v.ctx, f);
            }
            /* a lost object ends the process in vm_free: say what failed first */
            fflush(stdout);
            vm_free(&v);
        }
        CHECK(failed > 0, "%s: no room made it fail", kinds[kind]);
        CHECK(made > 0, "%s: no room let it work", kinds[kind]);
    }
}

/* ---- compiling and writing bytecode ---- */

/* compile_with_room evals, then compiles, js with n bytes of memory left, and
   checks that it fails only with the out of memory error, takes the error out
   of the context and leaves the runtime working. It returns 1 for a failure. */
static int compile_with_room(const char *what, const char *js, size_t n)
{
    VM v = vm_new();
    JSContext *ctx = v.ctx;
    int failed = 0, r;
    JSValue val;
    JSContext *jc;

    room(&v, n);
    val = JS_Eval(ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(val))
        failed = 1;
    JS_FreeValue(ctx, val);
    while (!failed && (r = JS_ExecutePendingJob(v.rt, &jc)) != 0) {
        if (r < 0)
            failed = 1;
    }
    if (!failed) {
        val = JS_Eval(ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
        if (JS_IsException(val))
            failed = 1;
        JS_FreeValue(ctx, val);
    }
    if (failed) {
        CHECK(JS_IsUncatchableException(ctx), "%s, room %zu: fails without the out of memory error", what, n);
    }
    unlimit(&v);
    free(exception_text(ctx));
    CHECK(!JS_HasException(ctx), "%s, room %zu: an exception is pending", what, n);
    while (JS_ExecutePendingJob(v.rt, &jc) != 0)
        free(exception_text(ctx));
    CHECK(eval_is(ctx, "1 + 1", "2"), "%s, room %zu: 1 + 1 afterwards", what, n);
    vm_free(&v);
    return failed;
}

/* Compiling with little memory left either works or fails with the out of
   memory error: the engine does not crash, does not report a syntax error the
   script does not have, and the runtime works afterwards. */
static void test_compile_oom(void)
{
    size_t i, n;

    for (i = 0; i < countof(compile_scripts); i++) {
        int failed = 0;
        char what[32];

        snprintf(what, sizeof(what), "script %zu", i);
        for (n = 0; n <= 48 << 10; n += 32)
            failed += compile_with_room(what, compile_scripts[i], n);
        CHECK(failed > 0, "script %zu never ran out of memory", i);
    }
}

/* test_compile_oom in steps of 4 bytes, from no memory left up to well past
   where the script first runs. */
static void test_compile_oom_fine(void)
{
    size_t i, n;

    for (i = 0; i < countof(compile_fine_scripts); i++) {
        int failed = 0, ran = 0;
        char what[32];

        snprintf(what, sizeof(what), "fine script %zu", i);
        for (n = 0; ran < 512; n += 4) {
            if (compile_with_room(what, compile_fine_scripts[i], n)) {
                failed++;
                ran = 0;
            } else {
                ran++;
            }
        }
        CHECK(failed > 0, "fine script %zu never ran out of memory", i);
    }
}

/* Writing the bytecode of a compiled script with little memory left either
   gives bytecode that runs or fails with the out of memory error. The writer
   used to fail without an exception, and could return bytecode cut short. */
static void test_write_bytecode_oom(void)
{
    char *js = malloc(3000 * 100);
    size_t len = 0, n;
    int i, failed = 0;

    for (i = 0; i < 3000; i++)
        len += sprintf(js + len, "var v%.*s%d = 'str%d';\n", i % 50, "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", i, i);
    for (n = 0; n <= 1 << 20; n += 2048) {
        VM v = vm_new();
        JSValue f = JS_Eval(v.ctx, js, len, "<unit>", JS_EVAL_TYPE_GLOBAL | JS_EVAL_FLAG_COMPILE_ONLY);
        size_t size = 0;
        uint8_t *buf, *copy = NULL;

        CHECK(!JS_IsException(f), "compile for writing");
        room(&v, n);
        buf = JS_WriteObject(v.ctx, &size, f, JS_WRITE_OBJ_BYTECODE);
        unlimit(&v);
        if (!buf) {
            failed++;
            CHECK(JS_IsUncatchableException(v.ctx), "room %zu: writing fails without the out of memory error", n);
            free(exception_text(v.ctx));
        } else {
            copy = malloc(size);
            memcpy(copy, buf, size);
            js_free(v.ctx, buf);
        }
        CHECK(!JS_HasException(v.ctx), "room %zu: an exception is pending", n);
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
        if (copy) {
            VM w = vm_new();
            JSValue g = JS_ReadObject(w.ctx, copy, size, JS_READ_OBJ_BYTECODE);
            JSValue r = JS_IsException(g) ? g : JS_EvalFunction(w.ctx, g);

            CHECK(!JS_IsException(r), "room %zu: the bytecode written fails: %s", n, JS_IsException(r) ? "an exception" : "");
            if (JS_IsException(r))
                free(exception_text(w.ctx));
            JS_FreeValue(w.ctx, r);
            vm_free(&w);
            free(copy);
        }
    }
    CHECK(failed > 0, "writing never ran out of memory");
    free(js);
}

/* The allocation of a DynBuf that fails: failing() answers the number
   failat of the allocations counted since allocs was set to 0. */
static int failat, allocs;

static void *failing_realloc(void *opaque, void *ptr, size_t size)
{
    if (size == 0) {
        free(ptr);
        return NULL;
    }
    if (++allocs == failat)
        return NULL;
    return realloc(ptr, size);
}

/* Once growing a buffer failed, it takes no more bytes, not even those that
   still fit in its memory, so that what it holds is a run of whole writes. */
static void test_buffer_after_failure(void)
{
    DynBuf b;
    uint8_t d[4096];
    static const size_t writes[] = { 64, 64, 1 };
    size_t i;

    memset(d, 7, sizeof(d));
    dbuf_init2(&b, NULL, failing_realloc);
    /* Writes of 64, 64 and 1 bytes leave room for more in the buffer's
       memory, as it grows by half. */
    allocs = 0;
    failat = 0;
    for (i = 0; i < countof(writes); i++)
        CHECK(dbuf_put(&b, d, writes[i]) == 0, "a write fails");
    CHECK(b.allocated_size > b.size, "the buffer holds %zu bytes in %zu, want room for more", b.size, b.allocated_size);
    failat = allocs + 1;
    CHECK(dbuf_put(&b, d, 4096) != 0, "a write past the memory of the buffer did not fail");
    failat = allocs + 1;
    CHECK(dbuf_put(&b, d, 1) != 0 && b.size == 129 && b.error, "a write that fits after a failed one: size %zu, error %d, want it to fail", b.size, b.error);
    dbuf_free(&b);
    failat = 0;
}

/* ---- the stack of an error, read with little memory ---- */

static const char deep_error[] =
    "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\n"
    "function make() { return deep(40) }\n";

static const size_t rooms[] = { 0, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384 };

/* checkread checks a read of a stack with little memory: it either gives the
   whole text and leaves no exception, or throws out of memory. */
static void checkread(VM *v, JSValue got, const char *want, const char *what, size_t n)
{
    int is_exc = JS_IsException(got);
    char *text = value_text(v->ctx, got);
    int unc = JS_IsUncatchableException(v->ctx);
    char *ex = exception_text(v->ctx);

    /* the out of memory error is an InternalError, or null when there was no
       memory for that */
    if (is_exc)
        CHECK(ex && (strstr(ex, "out of memory") || (unc && strcmp(ex, "null") == 0)), "%s, room %zu: read threw %s, want out of memory", what, n, ex ? ex : "nothing");
    else
        CHECK(!ex && text && strcmp(text, want) == 0, "%s, room %zu: read gave a wrong text or left the exception %s", what, n, ex ? ex : "-");
    JS_FreeValue(v->ctx, got);
    free(text);
    free(ex);
}

/* When there is no memory to format a stack, reading it throws, and the stack
   stays pending: reading it again later gives the whole text. A function name
   outside ASCII is converted to UTF-8 when the stack is formatted, and that
   conversion can run out of memory too. */
static void test_backtrace_read_oom(void)
{
    static const char *const chars[] = { "", "\xc3\xa9", "\xe4\xb8\xad", "\xf0\x9f\x98\x80" };
    size_t k, i;

    for (k = 0; k < countof(chars); k++) {
        for (i = 0; i < countof(rooms); i++) {
            VM v = vm_new();
            char src[512];
            const char *fn = k ? "named" : "make";
            JSValue ref, e, first;
            char *want, *again;
            JSAtom a;

            eval_ok(v.ctx, deep_error);
            if (k) {
                snprintf(src, sizeof(src),
                         "function named() { return new Error(\"unicode\") }\n"
                         "Object.defineProperty(named, \"name\", { value: \"%s\".repeat(10000) })\n", chars[k]);
                eval_ok(v.ctx, src);
            }
            ref = call_global(v.ctx, fn);
            want = stack_text(v.ctx, ref);
            JS_FreeValue(v.ctx, ref);
            CHECK(want != NULL, "no reference stack");
            if (k)
                CHECK(want && strncmp(want, "    at ", 7) == 0 && strncmp(want + 7, chars[k], strlen(chars[k])) == 0,
                      "the stack does not show the name");
            e = call_global(v.ctx, fn);
            a = JS_NewAtom(v.ctx, "stack");
            room(&v, rooms[i]);
            first = JS_GetProperty(v.ctx, e, a);
            unlimit(&v);
            checkread(&v, first, want ? want : "", k ? "unicode name" : "read", rooms[i]);
            again = stack_text(v.ctx, e);
            CHECK(again && want && strcmp(again, want) == 0, "room %zu: read again gave a different text", rooms[i]);
            free(again);
            free(want);
            JS_FreeAtom(v.ctx, a);
            JS_FreeValue(v.ctx, e);
            vm_free(&v);
        }
    }
}

/* Assigning or defining 'stack' replaces the pending stack without formatting
   it, so it works with no memory to spare, as it does when the stack is a
   string. After an error gets extra own properties, several errors share the
   new shape, and the replacement must not need to allocate a shape. */
static void test_backtrace_overwrite(void)
{
    static const size_t few[] = { 0, 64, 256 };
    int mutate, op;
    size_t i;

    for (mutate = 0; mutate < 2; mutate++) {
        for (op = 0; op < 2; op++) {
            for (i = 0; i < (mutate ? countof(few) : countof(rooms)); i++) {
                size_t n = mutate ? few[i] : rooms[i];
                VM v = vm_new();
                JSValue e;
                JSAtom a;
                int ret;
                char *t;

                if (mutate)
                    eval_ok(v.ctx,
                            "var held = []\n"
                            "function deep(n) { return n ? deep(n - 1) : new Error(\"deep\") }\n"
                            "function make() { for (var i = 0; i < 2; i++) { var e = deep(40); held.push(e); for (var j = 0; j < 30; j++) e[\"x\" + j] = j } return held[0] }\n");
                else
                    eval_ok(v.ctx, deep_error);
                e = call_global(v.ctx, "make");
                a = JS_NewAtom(v.ctx, "stack");
                room(&v, n);
                if (op == 0)
                    ret = JS_SetProperty(v.ctx, e, a, JS_NewInt32(v.ctx, 42));
                else
                    ret = JS_DefinePropertyValue(v.ctx, e, a, JS_NewInt32(v.ctx, 42), JS_PROP_CONFIGURABLE | JS_PROP_WRITABLE);
                unlimit(&v);
                CHECK(ret == 1 && !JS_HasException(v.ctx), "%s%s, room %zu: returned %d", mutate ? "mutated " : "", op ? "define" : "set", n, ret);
                free(exception_text(v.ctx));
                t = stack_text(v.ctx, e);
                CHECK(t && strcmp(t, "42") == 0, "stack is %s after the %s", t ? t : "?", op ? "define" : "set");
                free(t);
                JS_FreeAtom(v.ctx, a);
                JS_FreeValue(v.ctx, e);
                vm_free(&v);
            }
        }
    }
}

typedef struct {
    char *buf;
    size_t len;
} Sink;

static void sink_write(void *opaque, const char *s, size_t n)
{
    Sink *k = opaque;

    k->buf = realloc(k->buf, k->len + n + 1);
    memcpy(k->buf + k->len, s, n);
    k->len += n;
    k->buf[k->len] = 0;
}

/* Printing an error whose stack cannot be formatted for lack of memory must
   not leave an exception behind, and a later read gives the whole stack. A
   raw dump must not format a pending stack at all: it does no allocation, so
   it works with no memory to spare and leaves the error pending. The name of
   the function is outside ASCII for the last rows, so that the conversion to
   UTF-8 can run out of memory too. */
static void test_backtrace_print_oom(void)
{
    static const size_t few[] = { 0, 512, 8192 };
    int raw, unicode;
    size_t i, n;

    for (raw = 0; raw < 2; raw++) {
        for (unicode = 0; unicode < 2; unicode++) {
            size_t count = unicode ? 16384 / 128 + 1 : countof(few);

            for (i = 0; i < (raw ? 1 : count); i++) {
                VM v = vm_new();
                JSPrintValueOptions opt;
                Sink sink = { NULL, 0 };
                JSValue ref, e;
                char *want, *again;
                const char *fn = unicode ? "named" : "make";

                n = raw ? 0 : (unicode ? i * 128 : few[i]);
                eval_ok(v.ctx, deep_error);
                eval_ok(v.ctx,
                        "function named() { return new Error(\"printed\") }\n"
                        "Object.defineProperty(named, \"name\", { value: \"a\".repeat(1000) + \"\xe4\xb8\xad\" })\n");
                ref = call_global(v.ctx, fn);
                want = stack_text(v.ctx, ref);
                JS_FreeValue(v.ctx, ref);
                e = call_global(v.ctx, fn);
                JS_PrintValueSetDefaultOptions(&opt);
                opt.raw_dump = raw;
                room(&v, n);
                JS_PrintValue(v.ctx, sink_write, &sink, e, &opt);
                CHECK(!JS_HasException(v.ctx), "%s%s print, room %zu: left an exception", raw ? "raw " : "", unicode ? "unicode" : "plain", n);
                unlimit(&v);
                free(exception_text(v.ctx));
                again = stack_text(v.ctx, e);
                CHECK(again && want && strcmp(again, want) == 0, "%s print, room %zu: the read after gave a different text", raw ? "raw" : "plain", n);
                free(again);
                free(want);
                free(sink.buf);
                JS_FreeValue(v.ctx, e);
                vm_free(&v);
            }
        }
    }
}

static size_t tighten_room;
static VM *tighten_vm;

static JSValue js_limit(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    JS_SetMemoryLimit(tighten_vm->rt, JS_GetMemorySize(tighten_vm->rt) + tighten_room);
    return JS_UNDEFINED;
}

/* Throwing an error whose stack has to be built when there is no memory must
   not crash: building the stack used to free the error, which is the current
   exception, and then read it again. The runtime keeps working and the
   reference counts stay sound (the runtime frees with no block left). */
static void test_throw_stackless_oom(void)
{
    static const size_t rm[] = { 0, 256, 1024, 4096 };
    size_t i;

    for (i = 0; i < countof(rm); i++) {
        VM v = vm_new();
        JSValue g, thrower, r;
        char *t;

        tighten_vm = &v;
        tighten_room = rm[i];
        g = JS_GetGlobalObject(v.ctx);
        JS_SetPropertyStr(v.ctx, g, "limit", JS_NewCFunction(v.ctx, js_limit, "limit", 0));
        JS_FreeValue(v.ctx, g);
        eval_ok(v.ctx,
                "function make() {\n"
                "  function stackless() { var e = new Error(\"x\"); delete e.stack; limit(); return e }\n"
                "  function thrower() { throw stackless() }\n"
                "  Object.defineProperty(thrower, \"name\", { value: \"\xe5\x90\x8d\xe5\x89\x8d\".repeat(10000) })\n"
                "  return thrower\n"
                "}\n");
        thrower = call_global(v.ctx, "make");
        r = JS_Call(v.ctx, thrower, JS_UNDEFINED, 0, NULL);
        unlimit(&v);
        CHECK(JS_IsException(r), "room %zu: the call returned tag %d, want an exception", rm[i], JS_VALUE_GET_TAG(r));
        JS_FreeValue(v.ctx, r);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, thrower);
        t = eval_text(v.ctx, "6 * 7");
        CHECK(t && strcmp(t, "42") == 0, "room %zu: after the throw the runtime gave %s", rm[i], t ? t : "an exception");
        free(t);
        vm_free(&v);
    }
}

/* ---- lazy properties ---- */

/* Most built-in functions and objects, and the prototype of a function, are
   made at the first access to their property (an autoinit property). When
   making one runs out of memory, the access fails with the out of memory
   error and the property stays as it was, to be made at the next access. It
   used to become undefined for the life of the context: JSON.parse read for
   the first time with no memory left was not a function from then on. The
   global object holds its properties through references, which take a block
   of their own once the value is made, and a write that finds such a property
   on the prototype makes it first: neither loses a value when it fails.

   Each case reads (or writes) a property nobody accessed before, in a new
   runtime, with each room of two sweeps: the bytes the memory limit leaves
   (room, which the system malloc of the asan build counts to the block), and
   a full heap with a few blocks given back (fill_memory, which the small
   blocks of the engine's allocator need). With the memory back, the property
   must have its value, and the runtime must hold no lost block (vm_free). */
#define AUTOINIT_BYTE_ROOMS (1 + 1024 / 8)
static const struct {
    const char *holder; /* evaluated with memory: the object that has prop */
    const char *prop;
    int fn;             /* the value is a function, else an object that is not */
    int set;            /* write prop on an object whose prototype is holder */
} autoinit_cases[] = {
    { "JSON", "parse", 1, 0 },
    { "(function f() {})", "prototype", 0, 0 },
    { "globalThis", "decodeURIComponent", 1, 0 },
    { "globalThis", "Reflect", 0, 0 },
    { "JSON", "stringify", 1, 1 },
};

static int autoinit_value_ok(JSContext *ctx, JSValueConst v, int fn)
{
    return fn ? JS_IsFunction(ctx, v) : JS_IsObject(v) && !JS_IsFunction(ctx, v);
}

/* Each kind of failure is counted over the rooms of a case and checked once,
   with the first room it was seen in, so that every case has its line. */
typedef struct {
    int count;
    char first[32];
} AutoinitMiss;

static void autoinit_miss(AutoinitMiss *m, int bytes, size_t n)
{
    if (!m->count++)
        snprintf(m->first, sizeof(m->first), "%s room %zu", bytes ? "byte" : "fill", n);
}

static void test_autoinit_oom(void)
{
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), c, k;

    for (c = 0; c < countof(autoinit_cases); c++) {
        const char *hs = autoinit_cases[c].holder, *ps = autoinit_cases[c].prop;
        int fn = autoinit_cases[c].fn, set = autoinit_cases[c].set;
        int failed = 0, made = 0;
        AutoinitMiss catchable = { 0 }, wrong = { 0 }, lost = { 0 };

        for (k = 0; k < AUTOINIT_BYTE_ROOMS + nfill; k++) {
            VM v = vm_new();
            JSValue holder = JS_Eval(v.ctx, hs, strlen(hs), "<unit>", JS_EVAL_TYPE_GLOBAL);
            JSAtom atom = JS_NewAtom(v.ctx, ps);
            JSValue target, val = JS_UNDEFINED, r = JS_UNDEFINED, again;
            int bytes = k < AUTOINIT_BYTE_ROOMS, fail, unc;
            size_t n = bytes ? k * 8 : k - AUTOINIT_BYTE_ROOMS;

            target = set ? JS_NewObjectProto(v.ctx, holder) : JS_DupValue(v.ctx, holder);
            if (set)
                val = JS_NewObject(v.ctx);
            if (bytes) {
                room(&v, n);
            } else {
                fill_memory(&v);
                free_blocks(&v, rm[n].min, rm[n].max, rm[n].n);
            }
            if (set) {
                fail = JS_SetProperty(v.ctx, target, atom, val) < 0;
            } else {
                r = JS_GetProperty(v.ctx, target, atom);
                fail = JS_IsException(r);
            }
            unc = JS_IsUncatchableException(v.ctx);
            if (bytes)
                unlimit(&v);
            else
                release_blocks(&v);
            if (fail) {
                failed++;
                if (!unc)
                    autoinit_miss(&catchable, bytes, n);
                free(exception_text(v.ctx));
            } else {
                made++;
                if (!set && !autoinit_value_ok(v.ctx, r, fn))
                    autoinit_miss(&wrong, bytes, n);
            }
            JS_FreeValue(v.ctx, r);
            again = JS_GetProperty(v.ctx, holder, atom);
            if (!autoinit_value_ok(v.ctx, again, fn))
                autoinit_miss(&lost, bytes, n);
            JS_FreeValue(v.ctx, again);
            JS_FreeValue(v.ctx, target);
            JS_FreeValue(v.ctx, holder);
            JS_FreeAtom(v.ctx, atom);
            /* a lost object ends the process in vm_free: say what failed first */
            fflush(stdout);
            vm_free(&v);
        }
        CHECK(!catchable.count, "%s.%s: the access failed with a catchable exception in %d rooms, the first %s", hs, ps, catchable.count, catchable.first);
        CHECK(!wrong.count, "%s.%s: the access gave a value that is not %s in %d rooms, the first %s", hs, ps, fn ? "a function" : "an object", wrong.count, wrong.first);
        CHECK(!lost.count, "%s.%s: with the memory back it is not %s after %d rooms, the first %s", hs, ps, fn ? "a function" : "an object", lost.count, lost.first);
        CHECK(failed > 0, "%s.%s: no room made the access fail", hs, ps);
        CHECK(made > 0, "%s.%s: no room let the access work", hs, ps);
        fflush(stdout);
    }
}

/* ---- JSON ---- */

/* When the memory runs out while JSON.stringify or JSON.parse copies a run of
   a string, they throw the out of memory error, and with the memory back they
   give the right string. Each case calls a function, made beforehand, with
   more and more memory, until it returns true. */
static void test_json_strings_oom(void)
{
    static const char *const fns[] = {
        "() => JSON.stringify(narrow) === qn", "() => JSON.stringify(wide) === qw",
        "() => JSON.parse(qn) === narrow", "() => JSON.parse(qw) === wide", "() => JSON.parse(qa) === ascii",
    };
    VM v = vm_new();
    size_t i;

    eval_ok(v.ctx,
            "globalThis.narrow = 'ab\\u00e9'.repeat(40000) + '\"';\n"
            "globalThis.wide = '\\u6f22x'.repeat(40000) + '\\ud83d\\ude00\\n';\n"
            "globalThis.qn = JSON.stringify(narrow);\n"
            "globalThis.qw = JSON.stringify(wide);\n"
            "globalThis.ascii = 'x'.repeat(120000);\n"
            "globalThis.qa = JSON.stringify(ascii); 0");
    for (i = 0; i < countof(fns); i++) {
        JSValue fn = JS_Eval(v.ctx, fns[i], strlen(fns[i]), "<unit>", JS_EVAL_TYPE_GLOBAL);
        size_t extra;
        int ooms = 0;

        CHECK(JS_IsObject(fn), "%s: not a function", fns[i]);
        for (extra = 0;; extra += 4093) {
            JSValue r;

            if (extra > (size_t)4 << 20) {
                CHECK(0, "%s: does not return true with 4 MiB more", fns[i]);
                break;
            }
            room(&v, extra);
            r = JS_Call(v.ctx, fn, JS_UNDEFINED, 0, NULL);
            if (JS_IsException(r)) {
                int unc = JS_IsUncatchableException(v.ctx);
                char *t;

                unlimit(&v);
                t = exception_text(v.ctx);
                CHECK(unc, "%s: with %zu bytes more: a catchable exception: %s", fns[i], extra, t ? t : "?");
                free(t);
                ooms++;
                continue;
            }
            unlimit(&v);
            CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_BOOL && JS_VALUE_GET_INT(r) == 1, "%s: with %zu bytes more: tag %d, want true", fns[i], extra, JS_VALUE_GET_TAG(r));
            break;
        }
        JS_FreeValue(v.ctx, fn);
        CHECK(ooms >= 10, "%s: ran out of memory %d times, want the copies to run out", fns[i], ooms);
    }
    vm_free(&v);
}

/* ---- uncatchable ---- */

/* uncatchable_cases run fail(), which throws an error page code must not be
   able to catch, inside the constructs that catch an exception or run code on
   the way out of one. What page code saw is pushed onto globalThis.seen. */
static const struct { const char *name, *js; } uncatchable_cases[] = {
    { "catch", "try { fail() } catch (e) { seen.push('catch') }" },
    { "finally", "try { fail() } finally { seen.push('finally') }" },
    { "catch and rethrow", "try { fail() } catch (e) { seen.push('catch'); throw e }" },
    { "error boundary",
      "function render() { try { return child() } catch (e) { seen.push('boundary'); return 'fallback' } }\n"
      "function child() { try { fail() } catch (x) { if (typeof x?.then === 'function') return 'suspended'; throw x } }\n"
      "render()" },
    { "generator",
      "function* g() { try { yield 1; fail() } finally { seen.push('generator finally') } }\n"
      "try { for (const x of g()) {} } catch (e) { seen.push('catch') }" },
    { "for of return",
      "const it = { [Symbol.iterator]() { return this }, next() { return { value: 1, done: false } }, return() { seen.push('return'); return {} } }\n"
      "for (const x of it) fail()" },
    { "Array.from return",
      "const it = { [Symbol.iterator]() { return this }, next() { return { value: 1, done: false } }, return() { seen.push('return'); return {} } }\n"
      "try { Array.from(it, () => fail()) } catch (e) { seen.push('catch') }" },
    { "async function", "(async () => { try { await 0; fail() } catch (e) { seen.push('catch') } finally { seen.push('finally') } })()" },
    { "async rejection", "(async () => { await 0; fail() })().catch(() => seen.push('rejection'))" },
    { "async first step", "(async () => { fail() })().catch(() => seen.push('rejection'))" },
    { "then", "Promise.resolve().then(() => fail()).catch(() => seen.push('rejection'))" },
    { "Promise constructor", "new Promise(() => fail()).catch(() => seen.push('rejection'))" },
    { "Promise.try", "Promise.try(fail).catch(() => seen.push('rejection'))" },
    { "thenable job", "Promise.resolve({ then() { fail() } }).catch(() => seen.push('rejection'))" },
    { "thenable getter", "new Promise(r => r({ get then() { fail() } })).catch(() => seen.push('rejection'))" },
    { "Promise.all", "Promise.all({ [Symbol.iterator]() { fail() } }).catch(() => seen.push('rejection'))" },
    { "async generator", "(async function* () { try { await 0; fail() } finally { seen.push('finally') } })().next().catch(() => seen.push('rejection'))" },
    { "async from sync iterator", "(async () => { try { for await (const x of { [Symbol.iterator]() { return { next() { fail() } } } }) {} } catch (e) { seen.push('catch') } })()" },
};

/* interrupt_taken_text takes the pending exception out of the context as
   exception_text does, with the interrupt flag clear first: the host takes
   an interrupt back once the call that got it has ended, and the conversion
   of the error to a string runs code. */
static char *interrupt_taken_text(VM *v)
{
    *JS_GetInterruptFlag(v->rt) = 0;
    return exception_text(v->ctx);
}

/* run_uncatchable evaluates js and then runs the jobs it queued, and returns
   whether the first error was uncatchable. What page code pushed onto seen is
   put in seen (malloc'd). The error is taken out of the context, and the
   limit is lifted. */
static int run_uncatchable(VM *v, const char *js, char **seen, char **text)
{
    JSValue r = JS_Eval(v->ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL);
    JSContext *jc;
    int unc = 0, ret;

    *text = NULL;
    /* An uncatchable error can stay pending while the call returns a value
       (an async function that ran out of memory before its first await). */
    if (JS_IsException(r) || JS_HasException(v->ctx)) {
        unc = JS_IsUncatchableException(v->ctx);
        *text = interrupt_taken_text(v);
    }
    JS_FreeValue(v->ctx, r);
    /* An uncatchable error stays pending when a job ends with it: the host
       asks after each job, as VM.ExecutePendingJobs does. */
    while (!*text && (ret = JS_ExecutePendingJob(v->rt, &jc)) != 0) {
        if (ret < 0 || JS_HasException(v->ctx)) {
            unc = JS_IsUncatchableException(v->ctx);
            *text = interrupt_taken_text(v);
        }
    }
    unlimit(v);
    /* the host takes the interrupt back once the call that got it has ended */
    *JS_GetInterruptFlag(v->rt) = 0;
    /* nothing page code queued may run later either */
    while ((ret = JS_ExecutePendingJob(v->rt, &jc)) != 0) {
        if (ret < 0)
            free(exception_text(v->ctx));
    }
    *seen = eval_text(v->ctx, "seen.join()");
    return unc;
}

/* stop() sets the interrupt flag of the runtime, as a host does to end a
   script. */
static JSValue js_stop(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    (void)this_val;
    (void)argc;
    (void)argv;
    *JS_GetInterruptFlag(JS_GetRuntime(ctx)) = 1;
    return JS_UNDEFINED;
}

/* The two ways page code is ended that it cannot catch: fail() sets the
   interrupt flag and loops, or runs out of memory. */
typedef enum { END_INTERRUPT, END_OOM } End;

static const char *const fail_js[] = {
    "globalThis.fail = () => { stop(); for (;;); }",
    "globalThis.fail = () => { const a = []; for (;;) a.push('x'.repeat(256) + a.length) }",
};

/* check_uncatchable runs js in a new runtime where setup (a script) made
   fail() end the call as end says, and checks that page code cannot catch it
   in any construct: the error that ends the run is uncatchable and is the
   interrupt or the out of memory error, null or an error of that name, page
   code saw nothing of it, and no exception stays pending. */
static void check_uncatchable(const char *name, End end, const char *setup, const char *js)
{
    VM v = vm_new();
    JSValue g = JS_GetGlobalObject(v.ctx);
    char *seen, *text;
    const char *what = end == END_INTERRUPT ? "interrupt" : "out of memory error";
    int unc, ended;

    JS_SetPropertyStr(v.ctx, g, "stop", JS_NewCFunction(v.ctx, js_stop, "stop", 0));
    JS_FreeValue(v.ctx, g);
    eval_ok(v.ctx, "globalThis.seen = []");
    eval_ok(v.ctx, fail_js[end]);
    if (setup)
        eval_ok(v.ctx, setup);
    if (end == END_OOM)
        room(&v, 1 << 20);
    unc = run_uncatchable(&v, js, &seen, &text);
    ended = end == END_INTERRUPT ? text && strcmp(text, "InternalError: interrupted") == 0 :
        text && (strstr(text, "out of memory") || strcmp(text, "null") == 0);
    CHECK(unc, "%s: the error that ended the run is catchable (%s)", name, text ? text : "no error");
    CHECK(ended, "%s: ended with %s, want the %s", name, text ? text : "no error", what);
    CHECK(seen && seen[0] == 0, "%s: page code saw the %s: %s", name, what, seen ? seen : "?");
    CHECK(!JS_HasException(v.ctx), "%s: an exception is pending", name);
    free(seen);
    free(text);
    vm_free(&v);
}

/* An interrupt ends the call that was running however page code is written:
   no catch, finally, iterator return or rejection handler sees it, and the
   call returns it. */
static void test_interrupt_uncatchable(void)
{
    size_t i;

    for (i = 0; i < countof(uncatchable_cases); i++)
        check_uncatchable(uncatchable_cases[i].name, END_INTERRUPT, NULL, uncatchable_cases[i].js);
}

/* Running out of memory ends the call: page code cannot catch it in any
   construct that catches an exception or runs code on the way out of one, and
   what ends the call is the engine's out of memory error, null or an error of
   that name. */
static void test_oom_uncatchable(void)
{
    size_t i;

    for (i = 0; i < countof(uncatchable_cases); i++)
        check_uncatchable(uncatchable_cases[i].name, END_OOM, NULL, uncatchable_cases[i].js);
}

/* An interrupt or running out of memory in a getter or a method reached from
   a string, a number or an array (the reads by name that go straight to the
   prototype) ends the call: page code cannot catch it. */
static void test_field_chain_uncatchable(void)
{
    static const struct { const char *name, *setup; } cases[] = {
        { "string getter", "Object.defineProperty(String.prototype, 'boom', { get() { fail() }, configurable: true }); globalThis.go = () => 'abc'.boom" },
        { "number getter", "Object.defineProperty(Number.prototype, 'boom', { get() { 'use strict'; fail() }, configurable: true }); globalThis.go = () => (1.5).boom" },
        { "array getter", "Object.defineProperty(Array.prototype, 'boom', { get() { fail() }, configurable: true }); globalThis.go = () => [1].boom" },
        { "string method", "String.prototype.boom = function () { fail() }; globalThis.go = () => 'abc'.boom()" },
        { "array method", "Object.prototype.boom = function () { fail() }; globalThis.go = () => [1].boom()" },
    };
    size_t i;
    int end;

    for (i = 0; i < countof(cases); i++) {
        for (end = END_INTERRUPT; end <= END_OOM; end++) {
            char name[64];

            snprintf(name, sizeof(name), "%s, %s", cases[i].name, end == END_OOM ? "out of memory" : "interrupt");
            check_uncatchable(name, end, cases[i].setup,
                              "try { go() } catch (e) { seen.push('catch') } finally { seen.push('finally') }");
        }
    }
}

/* The regexp engine running out of memory, compiling a pattern or running it,
   is the out of memory error. */
static void test_regexp_oom(void)
{
    static const struct { const char *name, *js; } cases[] = {
        { "exec", "try { /(a)+$/.exec(s) } catch (e) { seen.push(e.message) }" },
        { "compile", "try { new RegExp(s) } catch (e) { seen.push(e.message) }" },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        VM v = vm_new();
        char *seen, *text;
        int unc;

        eval_ok(v.ctx, "globalThis.seen = []; globalThis.s = 'a'.repeat(1e6) + 'x'");
        room(&v, 64 << 10);
        unc = run_uncatchable(&v, cases[i].js, &seen, &text);
        CHECK(unc, "%s: the error that ended the run is catchable (%s)", cases[i].name, text ? text : "no error");
        CHECK(text && (strstr(text, "out of memory") || strcmp(text, "null") == 0), "%s: ended with %s", cases[i].name, text ? text : "no error");
        CHECK(seen && seen[0] == 0, "%s: page code saw the out of memory error: %s", cases[i].name, seen ? seen : "?");
        free(seen);
        free(text);
        vm_free(&v);
    }
}

/* What page code throws is never uncatchable, even when it copies the name and
   message of an error the engine ends a call with. */
static void test_thrown_is_not_uncatchable(void)
{
    static const char *const thrown[] = {
        "new InternalError('out of memory')",
        "new InternalError('interrupted')",
        "new InternalError('panic in Go function boom: boom')",
        "({ name: 'InternalError', message: 'out of memory' })",
        "({ name: 'InternalError', message: 'interrupted' })",
        "'InternalError: interrupted'",
        "null",
    };
    size_t i;

    for (i = 0; i < countof(thrown); i++) {
        int via;

        for (via = 0; via < 2; via++) {
            VM v = vm_new();
            char src[256];
            JSValue r;

            snprintf(src, sizeof(src), "globalThis.fail = () => { throw %s }", thrown[i]);
            eval_ok(v.ctx, src);
            if (via == 0)
                r = JS_Eval(v.ctx, "fail()", 6, "<unit>", JS_EVAL_TYPE_GLOBAL);
            else
                r = call_global(v.ctx, "fail");
            CHECK(JS_IsException(r), "%s: no exception", thrown[i]);
            CHECK(!JS_IsUncatchableException(v.ctx), "%s (%s): thrown by page code and uncatchable", thrown[i], via ? "call" : "eval");
            JS_FreeValue(v.ctx, r);
            free(exception_text(v.ctx));
            CHECK(!JS_HasException(v.ctx), "%s: an exception is pending", thrown[i]);
            vm_free(&v);
        }
    }
}

/* ---- the paths that no longer recurse ---- */

/* sweep runs setup and then src, both global scripts of the file name file, in
   a new runtime for each room: every 8 bytes from nothing to max bytes past
   what the runtime holds, and every room of a full heap (fill_rooms). The text
   of the value of src must be one of want (a list that ends with NULL), or
   src must fail uncatchable, as running out of memory does. Then, with the
   memory back, src must give want[0] in the same runtime, and the runtime
   must free with no block left. Some room must fail and some must work. */
static void sweep(const char *what, const char *file, const char *setup,
                  const char *src, const char *const *want, size_t max)
{
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes = max / 8 + 1, k;
    int failed = 0, worked = 0;

    for (k = 0; k < nbytes + nfill; k++) {
        VM v = vm_new();
        JSValue r;
        char *t;
        int unc, i, fail;

        if (setup && !eval_ok(v.ctx, setup)) {
            CHECK(0, "%s: the setup failed", what);
            vm_free(&v);
            return;
        }
        if (k < nbytes) {
            room(&v, k * 8);
        } else {
            fill_memory(&v);
            free_blocks(&v, rm[k - nbytes].min, rm[k - nbytes].max, rm[k - nbytes].n);
        }
        r = JS_Eval(v.ctx, src, strlen(src), file, JS_EVAL_TYPE_GLOBAL);
        fail = JS_IsException(r);
        unc = JS_IsUncatchableException(v.ctx);
        if (k < nbytes)
            unlimit(&v);
        else
            release_blocks(&v);
        if (fail) {
            failed++;
            t = exception_text(v.ctx);
            CHECK(unc, "%s: room %zu: a catchable exception: %s", what, k, t ? t : "?");
        } else {
            t = value_text(v.ctx, r);
            for (i = 0; want[i] && !(t && strcmp(t, want[i]) == 0); i++)
                ;
            CHECK(want[i], "%s: room %zu: got %s", what, k, t ? t : "(no text)");
            if (t && strcmp(t, want[0]) == 0)
                worked++;
        }
        free(t);
        JS_FreeValue(v.ctx, r);
        r = JS_Eval(v.ctx, src, strlen(src), file, JS_EVAL_TYPE_GLOBAL);
        t = value_text(v.ctx, r);
        CHECK(t && strcmp(t, want[0]) == 0, "%s: room %zu: with the memory back: %s", what, k, t ? t : "(exception)");
        free(t);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, r);
        fflush(stdout);
        vm_free(&v);
    }
    CHECK(failed > 0, "%s: no room made it fail", what);
    CHECK(worked > 0, "%s: no room let it work", what);
}

/* A TypeError that a built-in function throws gets its stack when it is made,
   and code of a script on the stack has it formatted at once: the names of the
   functions and of the file are converted to UTF-8 there, which needs memory
   when they are not ASCII. Without memory for it the stack is null, or the
   call fails with the out of memory error. */
static void test_new_error_stack_oom(void)
{
    static const char *const want[] = {
        "TypeError: empty array\n"
        "    at reduce (native)\n"
        "    at f\xc3\xa9 (\xc3\xa9.js:1:17)\n"
        "    at <eval> (\xc3\xa9.js:2:9)\n",
        "TypeError: empty array\nnull",
        NULL };

    sweep("new error", "\xc3\xa9.js", NULL,
          "function f\xc3\xa9() { return [].reduce(Math.max) }\n"
          "try { f\xc3\xa9() } catch (e) { e + '\\n' + e.stack }",
          want, 24576);
}

/* A syntax error gets the place in the source, then the stack of the
   functions that called eval, formatted at once with the memory that is left.
   Running out of it there throws, which frees the current exception: the
   syntax error itself, which was still being given its properties. The
   syntax error stays the exception. Its stack is null, or missing, which the
   interpreter then fills with the functions alone, without the place. */
static void test_syntax_error_stack_oom(void)
{
    static const char *const want[] = {
        "SyntaxError: unexpected token in expression: ''\n"
        "    at <input>:1:4\n"
        "    at f\xc3\xa9 (\xc3\xa9.js:1:28)\n"
        "    at <eval> (\xc3\xa9.js:2:9)\n",
        "SyntaxError: unexpected token in expression: ''\n"
        "    at f\xc3\xa9 (\xc3\xa9.js:1:28)\n"
        "    at <eval> (\xc3\xa9.js:2:9)\n",
        "SyntaxError: unexpected token in expression: ''\nundefined",
        "SyntaxError: unexpected token in expression: ''\nnull",
        NULL };

    sweep("syntax error", "\xc3\xa9.js", NULL,
          "function f\xc3\xa9() { return eval('1 +') }\n"
          "try { f\xc3\xa9() } catch (e) { e + '\\n' + e.stack }",
          want, 24576);
}

/* The paths split off the generic conversions and property paths, so that
   the call graph has no cycle through them, with each room: an element of a
   fast array read and written by a string key, through a prototype and as a
   descriptor (a BigInt element needs memory), a rope converted to a number,
   strings that are and are not canonical numeric keys of a typed array, a
   length set through valueOf, a rope made flat by a method and as a key, and
   objects converted to strings and keys. Each works or fails with the out of
   memory error. */
static void test_split_paths_oom(void)
{
    static const char *const want[] = {
        "-2,0,3,5,7,42,undefined,undefined,false,false,2,4206,233,3,k4206,k4206,1",
        NULL };

    sweep("split paths", "split.js",
          "var ta = new BigInt64Array([1n, -2n]), f64 = new Float64Array([0.5, -0]), a = [1, 2, 3];\n"
          "var p = 'ab\\u00e9'.repeat(200), rope = p;\n"
          "for (var i = 0; i < 6; i++) rope = rope + (p + i);\n"
          "var num = ' '.repeat(600) + (' '.repeat(600) + '42');\n"
          "var o = { toString() { return 'k' + rope.length } }, len = { valueOf() { return 2 } };\n",
          "var r = [];\n"
          "r.push(String(Reflect.get(ta, '1')), String(Object.getOwnPropertyDescriptor(f64, '1').value), Object.create(a)[2]);\n"
          "Reflect.set(a, '1', 5); Object.defineProperty(ta, '0', { value: 7n }); r.push(a[1], String(ta[0]));\n"
          "r.push(Number(num), String(ta['1.5']), String(ta['-0']), String('1e21' in f64), String('0.1' in f64));\n"
          "var b = [1, 2, 3, 4]; b.length = len; r.push(b.length);\n"
          "r.push(rope.length, rope.charCodeAt(1203), ({ [rope]: 3 })[rope]);\n"
          "r.push(String(o), `${o}`, { [o]: 1 }['k' + rope.length]);\n"
          "r.join(',')",
          want, 24576);
}

/* ---- atoms ---- */

/* The atom table of a runtime grows by half when it is full, a large block
   that a limit can refuse while small blocks still come. A runtime starts with
   about 500 atoms in a table of 711, so 600 new names make it grow. When it
   could not grow, the name was not made and no error was thrown: compiling
   failed with no exception pending, and a property key made from a string
   left the interpreter with no exception to throw. Now both fail with the out
   of memory error. */
#define ATOM_NAMES 600

static void test_compile_names_oom(void)
{
    static char js[ATOM_NAMES * 12 + 16];
    size_t len, n;
    int i, failed = 0;

    len = (size_t)snprintf(js, sizeof(js), "var ");
    for (i = 0; i < ATOM_NAMES; i++)
        len += (size_t)snprintf(js + len, sizeof(js) - len, "%sname%d", i ? ", " : "", i);
    snprintf(js + len, sizeof(js) - len, "; 1");
    for (n = 0; n <= 48 << 10; n += 32)
        failed += compile_with_room("many names", js, n);
    CHECK(failed > 0, "many names never ran out of memory");
}

static void test_key_names_oom(void)
{
    static const char *const want[] = { "600", NULL };

    sweep("keys made from strings", "keys.js", NULL,
          "var o = {}; for (var i = 0; i < 600; i++) o['key' + i] = i; String(Object.keys(o).length)",
          want, 65536);
}

/* The empty string is the string of an atom: making it takes no memory,
   from a full heap too, where a string of one character fails. The string
   of an index atom is made, with memory. */
static void test_empty_string_full_heap(void)
{
    VM v = vm_new();
    JSAtom seven = JS_NewAtomUInt32(v.ctx, 7);
    JSValue e1, e2, one, s7;
    char *t1, *t2, *t7;

    fill_memory(&v);
    e1 = JS_NewStringLen(v.ctx, "", 0);
    e2 = JS_NewString(v.ctx, "");
    one = JS_NewStringLen(v.ctx, "a", 1);
    CHECK(JS_IsString(e1) && JS_IsString(e2), "no empty string from a full heap");
    CHECK(JS_IsException(one), "a string of one character from a full heap");
    free(exception_text(v.ctx));
    release_blocks(&v);
    s7 = JS_AtomToString(v.ctx, seven);
    t1 = value_text(v.ctx, e1);
    t2 = value_text(v.ctx, e2);
    t7 = value_text(v.ctx, s7);
    CHECK(t1 && t2 && !strcmp(t1, "") && !strcmp(t2, ""), "the empty strings are %s and %s", t1 ? t1 : "?", t2 ? t2 : "?");
    CHECK(t7 && !strcmp(t7, "7"), "the string of the atom 7 is %s", t7 ? t7 : "?");
    free(t1);
    free(t2);
    free(t7);
    JS_FreeValue(v.ctx, e1);
    JS_FreeValue(v.ctx, e2);
    JS_FreeValue(v.ctx, s7);
    JS_FreeAtom(v.ctx, seven);
    vm_free(&v);
}

/* The collector queues the callback of a FinalizationRegistry when it
   frees a target. It cannot throw, so with no memory for the job the job is
   lost and the collection goes on. With memory the callback runs with the
   held value. Either way nothing is left behind (vm_free). The heap is
   filled after the target is let go: what it gave back, which the system
   malloc of the asan build counts at once, must not hold the job. */
static void test_finrec_job_full_heap(void)
{
    static const char setup[] =
        "var got = [];\n"
        "var reg = new FinalizationRegistry(h => got.push(h));\n"
        "(function () { var t = {}; reg.register(t, 'held'); return t })()";
    int full;

    for (full = 0; full < 2; full++) {
        VM v = vm_new();
        JSValue target = JS_Eval(v.ctx, setup, strlen(setup), "<unit>", JS_EVAL_TYPE_GLOBAL);
        JSContext *ctx1;

        CHECK(JS_IsObject(target), "the setup failed");
        JS_RunGC(v.rt);
        CHECK(!JS_IsJobPending(v.rt), "full %d: a job before the target is freed", full);
        JS_FreeValue(v.ctx, target);
        if (full)
            take_memory(&v);
        JS_RunGC(v.rt);
        CHECK(JS_IsJobPending(v.rt) == !full, "full %d: a job is %s", full, full ? "queued" : "missing");
        if (full)
            release_blocks(&v);
        while (JS_ExecutePendingJob(v.rt, &ctx1) > 0)
            ;
        CHECK(eval_is(v.ctx, "got.join()", full ? "" : "held"), "full %d: the callback", full);
        vm_free(&v);
    }
}

int main(void)
{
    nested_scopes();
    test_throw_error_oom();
    test_tocstring_oom();
    test_define_oom();
    test_function_name_oom();
    test_compile_oom();
    test_compile_oom_fine();
    test_write_bytecode_oom();
    test_buffer_after_failure();
    test_backtrace_read_oom();
    test_backtrace_overwrite();
    test_backtrace_print_oom();
    test_throw_stackless_oom();
    test_autoinit_oom();
    test_json_strings_oom();
    test_interrupt_uncatchable();
    test_oom_uncatchable();
    test_field_chain_uncatchable();
    test_regexp_oom();
    test_thrown_is_not_uncatchable();
    test_new_error_stack_oom();
    test_syntax_error_stack_oom();
    test_empty_string_full_heap();
    test_finrec_job_full_heap();
    test_split_paths_oom();
    test_compile_names_oom();
    test_key_names_oom();
    if (failures) {
        printf("unit-oom: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-oom: %ld checks passed\n", checks);
    return 0;
}
