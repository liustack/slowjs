/*
 * Copyright (c) 2026 The liustack/beastjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE at the
 * root of this repository.
 */

/* unit-oom runs the engine with little memory left, which scripts cannot do:
   a call that runs out of memory fails with the out of memory error (null,
   uncatchable), leaves the runtime working and loses nothing. They took over
   the tests that did this on the engine before it (out of memory,
   string conversion, backtraces, JSON, object literals, compiling, field
   chains, uncatchable errors). Each sweeps the memory it leaves: JS_SetMemoryLimit to what
   the runtime holds plus a room, for rooms from nothing up. A runtime that
   leaks a block ends the process with status 87 (host_free_runtime).

     unit-oom */
#include <unistd.h>

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

/* A function made by JS_NewHostFunction, JS_NewHostFunctionRelease or
   JS_NewCFunction2 with little memory left is either whole, with its name and
   length and no exception pending, or the call returns the exception with the
   out of memory error pending, and nothing it made is left (vm_free). One of
   JS_NewHostFunctionRelease that failed releases nothing, and one that was
   made releases its handle once, when it is freed. The name is a number,
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

static int function_name_releases;

static void function_name_release(uint32_t vm, uint32_t handle)
{
    function_name_releases += handle == 7 ? 1 : 1000;
}

static void test_function_name_oom(void)
{
    static const char *const kinds[] = { "JS_NewHostFunction", "JS_NewCFunction2", "JS_NewHostFunctionRelease" };
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), k;
    int kind;

    host_release_call = function_name_release;
    for (kind = 0; kind < 3; kind++) {
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
            function_name_releases = 0;
            if (kind == 0)
                f = JS_NewHostFunction(v.ctx, "12345", 5, 2, 7);
            else if (kind == 1)
                f = JS_NewCFunction2(v.ctx, function_name_noop, "12345", 2, JS_CFUNC_generic, 0);
            else
                f = JS_NewHostFunctionRelease(v.ctx, "12345", 5, 2, 7);
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
            CHECK(function_name_releases == (kind == 2 && !JS_IsException(f)),
                  "%s %s room %zu: %d releases", kinds[kind], sweep, n, function_name_releases);
            /* a lost object ends the process in vm_free: say what failed first */
            fflush(stdout);
            vm_free(&v);
        }
        CHECK(failed > 0, "%s: no room made it fail", kinds[kind]);
        CHECK(made > 0, "%s: no room let it work", kinds[kind]);
    }
    host_release_call = NULL;
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

/* new from the register loop of constructors whose register code is made at
   that new and runs out of memory: rv_new_frame must send them out of the
   loop as functions without register code, with more arguments than
   parameters too, and the new ends with the out of memory error. Each room
   gives the result or that error, and the runtime frees every block. */
static void test_new_convert_oom(void)
{
    static const char js[] =
        "function C(a) { this.a = a; }\n"
        "function D() { this.d = 1; }\n"
        "function f(n) {\n"
        "    var s = 0;\n"
        "    for (var i = 0; i < n; i++) s += new C(i, 1, 2).a + new D(1).d + new C(1).a;\n"
        "    return s;\n"
        "}\n"
        "f(0);\n";
    size_t n;
    int failed = 0, ran = 0;

    for (n = 0; n <= 16 << 10 && ran < 4; n += 8) {
        VM v = vm_new();
        JSContext *ctx = v.ctx;
        JSValue g, f, arg = JS_NewInt32(ctx, 3), val;
        int32_t r = 0;

        CHECK(eval_ok(ctx, js), "new_convert_oom: the script");
        g = JS_GetGlobalObject(ctx);
        f = JS_GetPropertyStr(ctx, g, "f");
        room(&v, n);
        val = JS_Call(ctx, f, JS_UNDEFINED, 1, &arg);
        if (JS_IsException(val)) {
            CHECK(JS_IsUncatchableException(ctx), "new_convert_oom, room %zu: fails without the out of memory error", n);
            free(exception_text(ctx));
            failed++;
        } else {
            CHECK(JS_ToInt32(ctx, &r, val) == 0 && r == 9, "new_convert_oom, room %zu: f(3) is %d, want 9", n, r);
            ran++;
        }
        JS_FreeValue(ctx, val);
        unlimit(&v);
        CHECK(eval_is(ctx, "f(3)", "9"), "new_convert_oom, room %zu: f(3) afterwards", n);
        JS_FreeValue(ctx, f);
        JS_FreeValue(ctx, g);
        vm_free(&v);
    }
    CHECK(failed > 0 && ran > 0, "new_convert_oom: %d rooms failed and %d ran", failed, ran);
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
        /* a script that fails for another reason never runs: the room
           stops growing at 1 MiB */
        for (n = 0; ran < 512 && n <= 1 << 20; n += 4) {
            if (compile_with_room(what, compile_fine_scripts[i], n)) {
                failed++;
                ran = 0;
            } else {
                ran++;
            }
        }
        CHECK(ran >= 512, "fine script %zu did not run with 1 MiB of room", i);
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

/* A built-in that meets the out of memory error can record it, in a promise
   it rejects, and return as if it had not failed: the error stays pending
   and the script goes on. A loop with no call in it must still end at its
   next backward jump with the out of memory error, without a host asking for
   a stop. LOOP_BOUND readings of the flag are far more than a run that ends
   there needs: past them the host stops the loop, and the case fails. The
   cases with "spin" run the loop in a function called twice before, so that
   it runs in the register code of that function, and the case checks that
   spin has it. */
#define LOOP_BOUND (1L << 22)
#define SPIN "function spin(n) { for (;;) if (n-- === 0) return; } spin(1); spin(1); "

static void test_uncatchable_then_loop(void)
{
    static const struct { const char *name, *js; } cases[] = {
        { "resolve with a thenable getter",
          "let res; new Promise(r => { res = r }); res({ get then() { fail() } }); for (;;) {}" },
        { "async first step", "(async () => { fail() })(); for (;;) {}" },
        { "async generator", "(async function* () { fail() })().next(); for (;;) {}" },
        { "dynamic import", "import({ toString() { fail() } }); for (;;) {}" },
        { "in a job",
          "Promise.resolve().then(() => { (async () => { fail() })(); for (;;) {} })" },
        { "resolve with a thenable getter, spin",
          SPIN "let res; new Promise(r => { res = r }); res({ get then() { fail() } }); spin(-1)" },
        { "async first step, spin", SPIN "(async () => { fail() })(); spin(-1)" },
        { "async generator, spin", SPIN "(async function* () { fail() })().next(); spin(-1)" },
        { "dynamic import, spin", SPIN "import({ toString() { fail() } }); spin(-1)" },
        { "in a job, spin",
          SPIN "Promise.resolve().then(() => { (async () => { fail() })(); spin(-1) })" },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        VM v = vm_new();
        JSValue r;
        JSContext *jc;
        char *text = NULL;
        long before;
        int ret, unc = 0;

        eval_ok(v.ctx, fail_js[END_OOM]);
        room(&v, 1 << 20);
        before = host_poll_reads;
        host_poll_stop_after(LOOP_BOUND);
        r = JS_Eval(v.ctx, cases[i].js, strlen(cases[i].js), "<unit>", JS_EVAL_TYPE_GLOBAL);
        if (JS_IsException(r)) {
            unc = JS_IsUncatchableException(v.ctx);
            text = exception_text(v.ctx);
        }
        JS_FreeValue(v.ctx, r);
        while (!text && (ret = JS_ExecutePendingJob(v.rt, &jc)) != 0) {
            if (ret < 0) {
                unc = JS_IsUncatchableException(v.ctx);
                text = exception_text(v.ctx);
            }
        }
        before = host_poll_reads - before;
        host_poll_stop_after(0);
        unlimit(&v);
        CHECK(before < LOOP_BOUND, "%s: the loop ran on until the host stopped it", cases[i].name);
        CHECK(unc, "%s: the error that ended the run is catchable (%s)", cases[i].name, text ? text : "no error");
        CHECK(text && (strstr(text, "out of memory") || strcmp(text, "null") == 0),
              "%s: ended with %s, want the out of memory error", cases[i].name, text ? text : "no error");
        /* the engine takes its stop back with the exception */
        CHECK(*JS_GetInterruptFlag(v.rt) == 0, "%s: the interrupt flag is %d after the exception was taken",
              cases[i].name, *JS_GetInterruptFlag(v.rt));
        if (strstr(cases[i].js, "spin(-1)")) {
            /* 1: register code */
            JSValue g = JS_GetGlobalObject(v.ctx), f = JS_GetPropertyStr(v.ctx, g, "spin");
            int st = JS_RegvmFuncStat(v.ctx, f, 0);
            CHECK(st == 1, "%s: spin is not in the register loop (state %d)", cases[i].name, st);
            JS_FreeValue(v.ctx, f);
            JS_FreeValue(v.ctx, g);
        }
        CHECK(eval_is(v.ctx, "let n = 0; for (let k = 0; k < 1000; k++) n += k; n", "499500"),
              "%s: a loop after the error", cases[i].name);
        free(text);
        vm_free(&v);
    }
}

/* A call the register loop answers itself, without the call (rv_fast_call,
   here charCodeAt of a string), fails while an uncatchable exception is
   pending, as the call does: the code after it does not run. */
static void test_uncatchable_fast_call(void)
{
    VM v = vm_new();
    JSValue r, g, f;
    char *text = NULL;
    int st, unc = 0;

    eval_ok(v.ctx, fail_js[END_OOM]);
    eval_ok(v.ctx, "var after = 0; function cc(s, k) { var n = s.charCodeAt(0);"
            " if (k) { (async () => { fail() })(); n += s.charCodeAt(1); after = n; } return n; }"
            " cc('ab', 0); cc('ab', 0);");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "cc");
    st = JS_RegvmFuncStat(v.ctx, f, 0);
    CHECK(st == 1, "fast call: cc is not in the register loop (state %d)", st);
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    room(&v, 1 << 20);
    r = JS_Eval(v.ctx, "cc('ab', 1)", strlen("cc('ab', 1)"), "<unit>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(r)) {
        unc = JS_IsUncatchableException(v.ctx);
        text = exception_text(v.ctx);
    }
    JS_FreeValue(v.ctx, r);
    unlimit(&v);
    CHECK(unc && text && (strstr(text, "out of memory") || strcmp(text, "null") == 0),
          "fast call: ended with %s, want the uncatchable out of memory error", text ? text : "no error");
    CHECK(eval_is(v.ctx, "after", "0"), "fast call: the code after charCodeAt ran with the exception pending");
    free(text);
    vm_free(&v);
}

/* x instanceof C that the register loop answers itself (rv_instof), without
   the call of Function.prototype[Symbol.hasInstance], fails while an
   uncatchable exception is pending, as that call does: the code after it
   does not run. The exception is left pending by an async function whose
   first run fails, by an interrupt or by running out of memory, and the left
   side is an object or a primitive. */
static void test_uncatchable_instanceof(void)
{
    static const char *const lefts[] = { "{}", "1" };
    End end;
    size_t i;

    for (end = END_INTERRUPT; end <= END_OOM; end++) {
        for (i = 0; i < sizeof(lefts) / sizeof(lefts[0]); i++) {
            const char *what = end == END_OOM ? "out of memory" : "interrupt";
            VM v = vm_new();
            JSValue r, g, f;
            char *text = NULL, call[64];
            int st, unc = 0, ended;

            g = JS_GetGlobalObject(v.ctx);
            JS_SetPropertyStr(v.ctx, g, "stop", JS_NewCFunction(v.ctx, js_stop, "stop", 0));
            eval_ok(v.ctx, fail_js[end]);
            eval_ok(v.ctx, "var after = 0; function io(x, k) { var r = x instanceof Object;"
                    " if (k) { (async () => { fail() })(); r = x instanceof Object; after = 1; } return r; }"
                    " void Function.prototype[Symbol.hasInstance]; io({}, 0); io(1, 0);");
            f = JS_GetPropertyStr(v.ctx, g, "io");
            st = JS_RegvmFuncStat(v.ctx, f, 0);
            CHECK(st == 1, "instanceof, %s, left %s: io is not in the register loop (state %d)",
                  what, lefts[i], st);
            JS_FreeValue(v.ctx, f);
            JS_FreeValue(v.ctx, g);
            if (end == END_OOM)
                room(&v, 1 << 20);
            snprintf(call, sizeof(call), "io(%s, 1)", lefts[i]);
            r = JS_Eval(v.ctx, call, strlen(call), "<unit>", JS_EVAL_TYPE_GLOBAL);
            if (JS_IsException(r)) {
                unc = JS_IsUncatchableException(v.ctx);
                /* the host takes its stop back to read the error */
                *JS_GetInterruptFlag(v.rt) &= ~1;
                text = exception_text(v.ctx);
            }
            JS_FreeValue(v.ctx, r);
            if (end == END_OOM)
                unlimit(&v);
            *JS_GetInterruptFlag(v.rt) = 0;
            ended = end == END_INTERRUPT ? text && strcmp(text, "InternalError: interrupted") == 0 :
                text && (strstr(text, "out of memory") || strcmp(text, "null") == 0);
            CHECK(unc && ended, "instanceof, %s, left %s: ended with %s, want the uncatchable %s",
                  what, lefts[i], text ? text : "no error", what);
            CHECK(eval_is(v.ctx, "after", "0"),
                  "instanceof, %s, left %s: the code after instanceof ran with the exception pending",
                  what, lefts[i]);
            free(text);
            vm_free(&v);
        }
    }
}

/* A RegExp method that does its work without the calls of the specification,
   on a regexp that nothing changed, fails as those calls do while an
   uncatchable exception is pending: here one left by the toString of an
   argument, which returned undefined. Called by the host, the method
   returns the exception, not a value. Called from a script, the code after
   it does not run. A replace first makes RegExp.prototype.exec, so that the
   replace of the case is not the first one. */
static void test_uncatchable_regexp_fast(void)
{
    static const struct { const char *name, *fn, *self, *args; int argc; } cases[] = {
        { "replace with a string", "RegExp.prototype[Symbol.replace]", "/a/g", "'aa', o", 2 },
        { "split", "RegExp.prototype[Symbol.split]", "/b/", "o", 1 },
        { "split with a limit", "RegExp.prototype[Symbol.split]", "/b/", "o, 1", 2 },
        { "replace with a function", "RegExp.prototype[Symbol.replace]", "/a/g", "o, () => (ran = 1, 'x')", 2 },
        { "match", "RegExp.prototype[Symbol.match]", "/a/", "o", 1 },
        { "global match", "RegExp.prototype[Symbol.match]", "/a/g", "o", 1 },
    };
    size_t i;
    int host;

    for (i = 0; i < countof(cases); i++) {
        for (host = 0; host < 2; host++) {
            VM v = vm_new();
            JSValue r, c = JS_UNDEFINED;
            char js[256], *text = NULL;
            const char *how = host ? "called by the host" : "called from a script";
            int unc = 0;

            eval_ok(v.ctx, fail_js[END_OOM]);
            /* the resolve function of a promise, given a thenable whose then
               getter runs out of memory, rejects the promise and returns */
            eval_ok(v.ctx, "/b/[Symbol.replace]('b', 'c'); var after = 0, ran = 0, res;"
                    " new Promise(r => { res = r });"
                    " var o = { toString: res.bind(undefined, { get then() { fail() } }) }");
            if (host) {
                snprintf(js, sizeof(js), "[%s, %s, %s]", cases[i].fn, cases[i].self, cases[i].args);
                c = JS_Eval(v.ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL);
                CHECK(!JS_IsException(c), "regexp %s: %s threw", cases[i].name, js);
            }
            room(&v, 1 << 20);
            if (host) {
                JSValue f = JS_GetPropertyUint32(v.ctx, c, 0), self = JS_GetPropertyUint32(v.ctx, c, 1);
                JSValue argv[2] = { JS_GetPropertyUint32(v.ctx, c, 2), JS_GetPropertyUint32(v.ctx, c, 3) };
                r = JS_Call(v.ctx, f, self, cases[i].argc, argv);
                JS_FreeValue(v.ctx, f);
                JS_FreeValue(v.ctx, self);
                JS_FreeValue(v.ctx, argv[0]);
                JS_FreeValue(v.ctx, argv[1]);
            } else {
                snprintf(js, sizeof(js), "%s.call(%s, %s); after = 1", cases[i].fn, cases[i].self, cases[i].args);
                r = JS_Eval(v.ctx, js, strlen(js), "<unit>", JS_EVAL_TYPE_GLOBAL);
            }
            if (JS_IsException(r)) {
                unc = JS_IsUncatchableException(v.ctx);
                text = exception_text(v.ctx);
            }
            JS_FreeValue(v.ctx, r);
            JS_FreeValue(v.ctx, c);
            unlimit(&v);
            CHECK(unc && text && (strstr(text, "out of memory") || strcmp(text, "null") == 0),
                  "regexp %s, %s: ended with %s, want the uncatchable out of memory error", cases[i].name,
                  how, text ? text : "no error");
            CHECK(eval_is(v.ctx, "after", "0"), "regexp %s, %s: the code after it ran with the exception pending",
                  cases[i].name, how);
            CHECK(eval_is(v.ctx, "ran", "0"), "regexp %s, %s: a function ran with the exception pending",
                  cases[i].name, how);
            free(text);
            vm_free(&v);
        }
    }
}

/* f.apply(this, arguments) in the register loop (CALLMA,
   engine/regvm-ops-args.h) calls f from the loop only when no uncatchable
   exception is pending: with one pending, after an async function ran out
   of memory, the call goes out of the loop and fails there, and f does not
   run. */
static void test_uncatchable_apply_forward(void)
{
    VM v = vm_new();
    JSValue r, g, f;
    char *text = NULL;
    int st, unc = 0;

    eval_ok(v.ctx, fail_js[END_OOM]);
    eval_ok(v.ctx, "var ran = 0, after = 0; function tg(a, b) { ran++; return a + b; }"
            " function fa(k) { var n = tg.apply(this, arguments);"
            " if (k) { (async () => { fail() })(); n += tg.apply(this, arguments); after = n; } return n; }"
            " fa(0, 1); fa(0, 1);");
    g = JS_GetGlobalObject(v.ctx);
    f = JS_GetPropertyStr(v.ctx, g, "fa");
    st = JS_RegvmFuncStat(v.ctx, f, 0);
    CHECK(st == 1, "apply forward: fa is not in the register loop (state %d)", st);
    JS_FreeValue(v.ctx, f);
    JS_FreeValue(v.ctx, g);
    eval_ok(v.ctx, "ran = 0");
    room(&v, 1 << 20);
    r = JS_Eval(v.ctx, "fa(1, 2)", strlen("fa(1, 2)"), "<unit>", JS_EVAL_TYPE_GLOBAL);
    if (JS_IsException(r)) {
        unc = JS_IsUncatchableException(v.ctx);
        text = exception_text(v.ctx);
    }
    JS_FreeValue(v.ctx, r);
    unlimit(&v);
    CHECK(unc && text && (strstr(text, "out of memory") || strcmp(text, "null") == 0),
          "apply forward: ended with %s, want the uncatchable out of memory error", text ? text : "no error");
    CHECK(eval_is(v.ctx, "ran", "1"), "apply forward: f ran with the exception pending");
    CHECK(eval_is(v.ctx, "after", "0"), "apply forward: the code after the call ran with the exception pending");
    free(text);
    vm_free(&v);
}

static VM *take_vm;

static JSValue js_take(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv)
{
    take_memory(take_vm);
    return JS_UNDEFINED;
}

/* lastIndex after a replace or a match by a regexp that fails, at each of
   the two stages of the rule of js_regexp_reset_lastIndex (quickjs.c), the
   same on the fast path and on the generic path, which a regexp with a
   property of its own takes. While the matches are found: the search of the
   4th match runs out of memory in the backtracking stack of (a|b)*, and
   lastIndex is the end of the 3rd, where the last call of exec left it (a
   match, a replace with a function, with a string), and a match runs out of
   memory as it makes the string of a match of 2 Mi characters, which exec
   makes after it set lastIndex to the end of the match (a stop in the
   search is in test_regexp_fast_stop_last_index of unit.c). After the
   matches are found, nothing writes lastIndex: a replace with a string runs
   out of memory as it adds a replacement of 16 Mi characters, and lastIndex
   is 0 with g, as the last call of exec left it, or the end of the match
   with y alone, and a replace with a function whose function set lastIndex
   to 77 and then took all memory, or threw, leaves it at 77 (on the fast
   path the strings of the next match fail, which the generic path made
   while it found the matches). */
static void test_regexp_oom_last_index(void)
{
    static const struct {
        const char *setup, *call, *want, *error;
        int room;
    } cases[] = {
        { "var s = ['ccc', 'ab'.repeat(1 << 20), 'c'].join(''), re = /(a|b)*c/g", "re[Symbol.match](s)", "3",
          "out of memory", 1 },
        { "var s = ['ccc', 'ab'.repeat(1 << 20), 'c'].join(''), re = /(a|b)*c/g", "re[Symbol.replace](s, () => '')",
          "3", "out of memory", 1 },
        { "var s = ['ccc', 'ab'.repeat(1 << 20), 'c'].join(''), re = /(a|b)*c/g", "re[Symbol.replace](s, 'x')", "3",
          "out of memory", 1 },
        { "var s = 'a'.repeat(2097152), re = /a{2097151}/g", "re[Symbol.match](s)", "2097151",
          "out of memory", 1 },
        { "var s = 'a'.repeat(2097152), re = /a{2097151}/y", "re[Symbol.match](s)", "2097151",
          "out of memory", 1 },
        { "var rep = 'x'.repeat(16 << 20), re = /a/g", "re[Symbol.replace]('aa', rep)", "0", "out of memory", 1 },
        { "var rep = 'x'.repeat(16 << 20), re = /a/gy", "re[Symbol.replace]('aa', rep)", "0", "out of memory", 1 },
        { "var rep = 'x'.repeat(16 << 20), re = /a/y", "re[Symbol.replace]('aa', rep)", "1", "out of memory", 1 },
        { "var s = ['a', 'b'.repeat(4096)].join(''), re = /a|b{4096}/g, big = 'x'.repeat(1 << 20), n = 0;"
          " function f() { re.lastIndex = 77; take(); return n++ ? big : '' }",
          "re[Symbol.replace](s, f)", "77", "out of memory", 0 },
        { "var s = 'aa', re = /a/g; function f() { re.lastIndex = 77; throw 1 }",
          "re[Symbol.replace](s, f)", "77", "1", 0 },
    };
    size_t i;
    int generic;

    for (i = 0; i < countof(cases); i++) {
        for (generic = 0; generic < 2; generic++) {
            VM v = vm_new();
            JSValue g, r;
            char js[512], *text = NULL;

            take_vm = &v;
            g = JS_GetGlobalObject(v.ctx);
            JS_SetPropertyStr(v.ctx, g, "take", JS_NewCFunction(v.ctx, js_take, "take", 0));
            JS_FreeValue(v.ctx, g);
            snprintf(js, sizeof(js), "%s; re.exec(''); if (%d) re.extra = 1", cases[i].setup,
                     generic);
            eval_ok(v.ctx, js);
            if (cases[i].room)
                room(&v, 1 << 20);
            r = JS_Eval(v.ctx, cases[i].call, strlen(cases[i].call), "<unit>", JS_EVAL_TYPE_GLOBAL);
            if (JS_IsException(r))
                text = exception_text(v.ctx);
            JS_FreeValue(v.ctx, r);
            release_blocks(&v);
            /* with no memory for the error, the uncatchable error is null */
            CHECK(text && (strstr(text, cases[i].error) ||
                           (!strcmp(cases[i].error, "out of memory") && !strcmp(text, "null"))),
                  "%s after %s, %s path: ended with %s, want %s",
                  cases[i].call, cases[i].setup, generic ? "generic" : "fast", text ? text : "no error",
                  cases[i].error);
            CHECK(eval_is(v.ctx, "re.lastIndex", cases[i].want), "%s after %s, %s path: lastIndex is not %s",
                  cases[i].call, cases[i].setup, generic ? "generic" : "fast", cases[i].want);
            free(text);
            vm_free(&v);
        }
    }
}

/* The engine's 2 in the interrupt flag follows the uncatchable exception
   alone: set when an exception is made uncatchable, cleared when it is taken
   or made catchable, never set for a catchable one or with nothing pending,
   and the host's stop (1) stays through all of it. */
static void test_uncatchable_flag_bit(void)
{
    VM v = vm_new();
    volatile int *flag = JS_GetInterruptFlag(v.rt);
    int host;

    for (host = 0; host <= 1; host++) {
        *flag = host;
        JS_ThrowInternalError(v.ctx, "catchable");
        CHECK(*flag == host, "host %d: a catchable exception set the flag to %d", host, *flag);
        JS_SetUncatchableException(v.ctx, TRUE);
        CHECK(*flag == (host | 2), "host %d: uncatchable, the flag is %d", host, *flag);
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
        CHECK(*flag == host, "host %d: the exception taken, the flag is %d", host, *flag);
        /* the uncatchable mark outlives the exception it was set for */
        JS_SetUncatchableException(v.ctx, TRUE);
        CHECK(*flag == host, "host %d: nothing pending, the flag is %d", host, *flag);
        JS_ThrowInternalError(v.ctx, "again");
        CHECK(*flag == host, "host %d: a new exception after the mark set the flag to %d", host, *flag);
        JS_SetUncatchableException(v.ctx, TRUE);
        CHECK(*flag == (host | 2), "host %d: uncatchable again, the flag is %d", host, *flag);
        JS_SetUncatchableException(v.ctx, FALSE);
        CHECK(*flag == host, "host %d: made catchable, the flag is %d", host, *flag);
        JS_FreeValue(v.ctx, JS_GetException(v.ctx));
    }
    *flag = 0;
    vm_free(&v);
}

/* The targets the WeakRef constructor and deref keep to the end of the job
   are in a set counted under the memory limit: making 200 WeakRefs and
   reading each, or reading 200 made in an earlier job, with every room up to
   64 KiB, gives the sum or the uncatchable out of memory error, and leaves
   no block behind once the job has ended and the runtime is freed. */
static void test_weakref_kept_oom(void)
{
    /* the first makes the WeakRefs in the job, so the constructor grows the
       set. The second reads WeakRefs made in a job before, so deref does */
    static const char *const js[] = {
        "const ws = objs.map(o => new WeakRef(o)); let k = 0;"
        "for (const w of ws) k += w.deref().i; String(k)",
        "let k = 0; for (const w of refs) k += w.deref().i; String(k)",
    };
    size_t n;
    int ok, oom, which;

    for (which = 0; which < 2; which++) {
        ok = oom = 0;
        for (n = 0; n <= 64 << 10; n += 256) {
            VM v = vm_new();
            JSValue r;
            char *t;

            eval_ok(v.ctx, "globalThis.objs = []; for (let i = 0; i < 200; i++) objs.push({ i });"
                    "globalThis.refs = objs.map(o => new WeakRef(o))");
            JS_ClearKeptObjects(v.rt);
            room(&v, n);
            r = JS_Eval(v.ctx, js[which], strlen(js[which]), "<unit>", JS_EVAL_TYPE_GLOBAL);
            unlimit(&v);
            if (JS_IsException(r)) {
                int unc = JS_IsUncatchableException(v.ctx);

                t = exception_text(v.ctx);
                CHECK(unc && t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                      "script %d, room %zu: failed with %s, want the out of memory error", which, n, t ? t : "nothing");
                oom++;
                free(t);
            } else {
                const char *s = JS_ToCString(v.ctx, r);

                CHECK(s && strcmp(s, "19900") == 0, "script %d, room %zu: got %s, want 19900", which, n, s ? s : "?");
                JS_FreeCString(v.ctx, s);
                ok++;
            }
            JS_FreeValue(v.ctx, r);
            JS_ClearKeptObjects(v.rt);
            vm_free(&v);
        }
        CHECK(ok > 0 && oom > 0, "script %d: %d rooms worked and %d ran out of memory, want some of each",
              which, ok, oom);
    }
}

/* A symbol gets a copy of a string that something else holds, and its
   description is a copy too: either copy running out of memory is the out of
   memory error, with nothing left behind. The first script makes symbols of
   held strings, the second reads the descriptions of symbols made before. */
static void test_symbol_copy_oom(void)
{
    static const char *const js[] = {
        "const ys = held.map(s => Symbol(s)); let k = 0;"
        "for (const y of ys) k += y.description.length; String(k)",
        "let k = 0; for (const y of syms) k += y.description.length; String(k)",
    };
    size_t n;
    int ok, oom, which;

    for (which = 0; which < 2; which++) {
        ok = oom = 0;
        for (n = 0; n <= 64 << 10; n += 256) {
            VM v = vm_new();
            JSValue r;
            char *t;

            eval_ok(v.ctx, "globalThis.held = []; for (let i = 0; i < 200; i++) held.push(String(i) + (i & 1 ? 'abcdefgh' : '\\u20acbcdefgh'));"
                    "globalThis.syms = held.map(s => Symbol(s))");
            room(&v, n);
            r = JS_Eval(v.ctx, js[which], strlen(js[which]), "<unit>", JS_EVAL_TYPE_GLOBAL);
            unlimit(&v);
            if (JS_IsException(r)) {
                int unc = JS_IsUncatchableException(v.ctx);

                t = exception_text(v.ctx);
                CHECK(unc && t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                      "script %d, room %zu: failed with %s, want the out of memory error", which, n, t ? t : "nothing");
                oom++;
                free(t);
            } else {
                const char *s = JS_ToCString(v.ctx, r);

                CHECK(s && strcmp(s, "2090") == 0, "script %d, room %zu: got %s, want 2090", which, n, s ? s : "?");
                JS_FreeCString(v.ctx, s);
                ok++;
            }
            JS_FreeValue(v.ctx, r);
            vm_free(&v);
        }
        CHECK(ok > 0 && oom > 0, "script %d: %d rooms worked and %d ran out of memory, want some of each",
              which, ok, oom);
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
   a new runtime for each room: every sweep_step bytes (8) from nothing to
   max bytes past what the runtime holds, and every room of a full heap
   (fill_rooms). The text
   of the value of src must be one of want (a list that ends with NULL), or
   src must fail uncatchable, as running out of memory does. Then, with the
   memory back, src must give want[0] in the same runtime, and the runtime
   must free with no block left. Some room must fail and some must work. */
/* sweep_register, when set, runs on the context of each runtime sweep makes,
   before the setup: it gives the scripts a function of the host. */
static void (*sweep_register)(JSContext *ctx);
/* sweep_done, when set, runs on the context before the runtime is freed */
static void (*sweep_done)(JSContext *ctx);
static size_t sweep_step = 8;

static void sweep(const char *what, const char *file, const char *setup,
                  const char *src, const char *const *want, size_t max)
{
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes, k;
    int failed = 0, worked = 0;

#ifdef RV_FOLD_TEST
    /* the test builds fold every function, which then takes more memory
       to convert and to run (a frame of all 127 slots, the stack model of
       the pass): up to 11 KiB more for the sweeps here, so that some room
       lets each work */
    max += 16384;
#endif
    nbytes = max / sweep_step + 1;
    for (k = 0; k < nbytes + nfill; k++) {
        VM v = vm_new();
        JSValue r;
        char *t;
        int unc, i, fail;

        if (sweep_register)
            sweep_register(v.ctx);
        if (setup && !eval_ok(v.ctx, setup)) {
            CHECK(0, "%s: the setup failed", what);
            if (sweep_done)
                sweep_done(v.ctx);
            vm_free(&v);
            return;
        }
        if (k < nbytes) {
            room(&v, k * sweep_step);
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
            CHECK(unc && t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                  "%s: room %zu: not the uncatchable out of memory error: %s", what, k, t ? t : "?");
        } else {
            /* a value, and no error left pending behind it */
            t = exception_text(v.ctx);
            CHECK(!t, "%s: room %zu: a value with %s pending", what, k, t);
            free(t);
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
        if (sweep_done)
            sweep_done(v.ctx);
        vm_free(&v);
    }
    CHECK(failed > 0, "%s: no room made it fail", what);
    CHECK(worked > 0, "%s: no room let it work", what);
}

/* copyProps (JS_NewCopyPropsFunction) adds keys to props one at a time, or
   takes the shape of a config and its values, and sets the keys props has:
   with too little memory it fails with the out of memory error, leaves
   props with the keys it had or a part of those it was given, and loses
   nothing. */
static void register_copy_props(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue f = JS_NewCopyPropsFunction(ctx, "copyProps", 9);

    CHECK(!JS_IsException(f), "no function");
    JS_SetPropertyStr(ctx, g, "copyProps", f);
    JS_FreeValue(ctx, g);
}

static void test_copy_props_oom(void)
{
    static const char setup[] =
        "var hop = Object.prototype.hasOwnProperty;\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 6; i++) {\n"
        "    var c = [{ a: 1, b: 2, key: 'k', c: 3 }, { a: 1, b: 2 }, Object.freeze({ x: 1, y: 2, z: 3 })][i % 3];\n"
        "    var p = {}, q = Object.assign({}, { a: 0, m: 9 });\n"
        "    if (!copyProps(c, p, hop, false) || !copyProps(c, q, hop, true)) return 'not copied';\n"
        "    out.push(Object.keys(p).join('+'), Object.keys(q).join('+'));\n"
        "  }\n"
        "  return out.join(' ');\n"
        "}\n"
        /* once before the sweep: with the register loop on, its register
           code is made at the first call, which would need more room */
        "run();\n";
    static const char *const want[] = {
        "a+b+c a+m+b+c a+b a+m+b x+y+z a+m+x+y+z a+b+c a+m+b+c a+b a+m+b x+y+z a+m+x+y+z", NULL
    };

    sweep_register = register_copy_props;
    sweep("copyProps", "copyprops.js", setup, "run()", want, 16384);
    sweep_register = NULL;
}

/* JS_HasProperty walks a prototype chain, through `in` with keys made at run
   time, the fields of property descriptors and the holes that every and
   concat look up: with too little memory (a key, the number a typed array
   reads from a key, a descriptor, a result) it fails with the out of memory
   error and frees each object of the chains once. */
static void test_has_property_oom(void)
{
    static const char setup[] =
        "var base = { a: 1 }, mid = Object.create(base), arr = [1, 2];\n"
        "mid.b = 2;\n"
        "var top = Object.create(mid), onArr = Object.create(arr);\n"
        "var onTa = Object.create(new Uint8Array(4));\n"
        "function run() {\n"
        "  var n = 0, out = [];\n"
        "  for (var i = 0; i < 8; i++) {\n"
        "    if (('a' + (i % 2 ? '' : 'x')) in top) n++;\n"
        "    if (i in onArr) n++;\n"
        "    if ((i + '.5') in onTa || ('' + i) in onTa) n++;\n"
        "    var d = Object.create({ enumerable: true, configurable: i % 2 == 0 });\n"
        "    d.value = i;\n"
        "    var o = Object.defineProperty({}, 'p' + i, d);\n"
        "    out.push(Object.keys(o).length);\n"
        "  }\n"
        "  var h = [1, , 3, , 5], seen = 0;\n"
        "  h.every(function () { seen++; return true });\n"
        "  return n + ' ' + out.join('') + ' ' + seen + ' ' + [0].concat(h).length;\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "10 11111111 3 6", NULL };

    sweep("has property", "hasprop.js", setup, "run()", want, 16384);
}

/* o[k] with the key in a local: with too little memory (the atom of a key
   made at run time, the string a key object gives, the result) it fails
   with the out of memory error and frees each key once. */
static void test_getel_local_key_oom(void)
{
    static const char setup[] =
        "var o = { k0: 'a', k1: 'b', k2: 'c', 5: 'five' }, arr = [1, 2, 3];\n"
        "function get(obj, key) { return obj[key] }\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 6; i++) {\n"
        "    var k = 'k' + (i % 4);\n"
        "    var ko = { toString: function () { return 'k' + (i % 3) } };\n"
        "    out.push(get(o, k), get(o, ko), get(arr, i % 4), get(arr, '' + (i % 3)), get(o, 5));\n"
        "  }\n"
        "  return out.join('');\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "aa11fivebb22fivecc33fivea1fiveab12fivebc23five", NULL };

    sweep("local key", "getel.js", setup, "run()", want, 16384);
}

/* o[k] = v with the key in a local: with too little memory (the atom of a
   key made at run time, the string a key object gives, the property and
   the room of the object or the array) it fails with the out of memory
   error and frees each key and value once. */
static void test_putel_local_key_oom(void)
{
    static const char setup[] =
        "function put(obj, key, v) { obj[key] = v }\n"
        "function run() {\n"
        "  var o = {}, arr = [];\n"
        "  for (var i = 0; i < 6; i++) {\n"
        "    var k = 'k' + (i % 4);\n"
        "    var ko = { toString: function () { return 'p' + (i % 3) } };\n"
        "    put(o, k, 'v' + i); put(o, ko, i); put(arr, i, 'a' + i); put(arr, '' + (i + 6), i);\n"
        "  }\n"
        "  return Object.keys(o).map(function (k) { return k + o[k] }).join() + '|' + arr.join();\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "k0v4,p03,k1v5,p14,k2v2,p25,k3v3|a0,a1,a2,a3,a4,a5,0,1,2,3,4,5", NULL };

    sweep("local key put", "putel.js", setup, "run()", want, 16384);
}

static void no_shape_cache(JSContext *ctx)
{
    JS_SetShapeCache(ctx, 0);
}

/* o[k] = v with a key that is its own atom (from Object.keys) on plain
   objects, empty, a literal and with a prototype, that grow one key at a time
   and get a key again: with too little memory (the copy of a shared shape,
   the next shape, the hash table of shapes, the property array) it fails
   with the out of memory error and frees each key and value once, with the
   shape cache at its default and off. */
static void test_putel_atom_oom(void)
{
    static const char setup[] =
        "var names = Object.keys({ pa0: 0, pa1: 0, pa2: 0, pa3: 0, pa4: 0, pa5: 0,\n"
        "  pa6: 0, pa7: 0, pa8: 0, pa9: 0, pa10: 0, pa11: 0 });\n"
        "function put(obj, key, v) { obj[key] = v }\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 4; i++) {\n"
        "    var o = i % 2 ? { lit: 1 } : {}, p = Object.create({ inh: 1 });\n"
        "    for (var j = 0; j < names.length; j++) {\n"
        "      put(o, names[j], 'v' + j); put(p, names[(j + i) % names.length], [j]);\n"
        "    }\n"
        "    put(o, names[0], 'again');\n"
        "    out.push(Object.keys(o).length + ':' + o.pa0 + ':' + Object.keys(p)[0] + ':' + p.pa11[0]);\n"
        "  }\n"
        "  return out.join(' ');\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "12:again:pa0:11 13:again:pa1:10 12:again:pa2:9 13:again:pa3:8", NULL };

    sweep("atom key put", "putelatom.js", setup, "run()", want, 16384);
    sweep_register = no_shape_cache;
    sweep("atom key put, no shape cache", "putelatom.js", setup, "run()", want, 16384);
    sweep_register = NULL;
}

static void shape_ring(JSContext *ctx)
{
    JS_SetShapeCache(ctx, 64);
}

/* Objects that get their keys one at a time find the shapes the shape cache
   keeps, and some of those have more room than the property array of the
   object, which add_property grows before it takes the shape. A request the
   heap refuses lets every kept shape go (js_shape_cache_drop), in some rooms
   while run() runs and in others before it (fill_memory): with too little
   memory the objects are whole or it fails with the out of memory error, and
   the runtime frees with no block left. unit-white sees the kept shapes go
   inside the request (test_shape_drop_slot) and inside the growth of the
   property array (test_shape_drop_in_resize). */
static void test_shape_cache_drop_oom(void)
{
    static const char setup[] =
        "function mk(n) { var o = {}; for (var k = 0; k < n; k++) o['q' + k] = k; return o }\n"
        "function run() {\n"
        "  var s = 0, o;\n"
        "  for (var i = 0; i < 6; i++) { o = mk(14); s += o.q13 + Object.keys(o).length }\n"
        "  return s + ':' + Object.keys(mk(5)).join();\n"
        "}\n"
        /* once before the sweep, so that its register code is made and the
           ring holds the shapes of mk */
        "run();\n";
    static const char *const want[] = { "162:q0,q1,q2,q3,q4", NULL };

    sweep_register = shape_ring;
    sweep_done = no_shape_cache;
    sweep("kept shapes let go", "shapedrop.js", setup, "run()", want, 16384);
    sweep_register = NULL;
    sweep_done = NULL;
}

/* The truth of strings, ropes and objects in locals and temps: with too
   little memory (the strings and objects made, the result) it fails with
   the out of memory error and frees each value once. */
static void test_truth_oom(void)
{
    static const char setup[] =
        "function run() {\n"
        "  var n = 0, s = '';\n"
        "  for (var i = 0; i < 40; i++) {\n"
        "    var t = i % 3 ? 'v' + i : '', o = i % 4 ? {} : null;\n"
        "    s += 'abcdefghijabcdefghij';\n"
        "    if (t) n++;\n"
        "    if (!o) n += 100;\n"
        "    if (s && t) n += 10000;\n"
        "    var r = t + s;\n"
        "    if (r || o) n += 1000000;\n"
        "  }\n"
        "  return String(n);\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "40261026", NULL };

    sweep("truth", "truth.js", setup, "run()", want, 16384);
}

/* An array literal takes the references of its elements (OP_array_from,
   js_create_array_free): with too little memory for the array or its
   elements it fails with the out of memory error and frees each element
   once, objects and strings made for it too. */
static void test_array_literal_oom(void)
{
    static const char setup[] =
        "function run() {\n"
        "  var keep = [], s = '';\n"
        "  for (var i = 0; i < 24; i++) {\n"
        "    var o = { i: i };\n"
        "    s += 'abcdefghij';\n"
        "    var a = [o, 'k' + i, s + i, [i, o], { o: o }, i];\n"
        "    if (i % 6 === 0) keep.push(a);\n"
        "  }\n"
        "  return keep.map(function (a) { return a[0].i + a[1] + a[2].length + a[3][1].i + a[4].o.i + a[5]; }).join();\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "0k011000,6k671666,12k12132121212,18k18192181818", NULL };

    sweep("array literal", "array_literal.js", setup, "run()", want, 16384);
}

/* The built-ins the register loop answers itself (rv_fast_call): a push that
   must grow its array is the call of push, so with too little memory it fails
   with the out of memory error of push and leaves the array as it was, and
   the answers that make nothing give the same result in every room. */
static void test_fast_builtins_oom(void)
{
    static const char setup[] =
        "function run() {\n"
        "  var a = [], m = new Map([[1, 'x']]), w = new WeakMap(), o = { k: 1 }, s = 'abcabd', n = 0;\n"
        "  w.set(o, 2);\n"
        "  for (var i = 0; i < 40; i++) {\n"
        "    a.push(i, m.get(1));\n"
        "    n += Math.clz32(i) + m.has(i) + w.get(o) + a.indexOf(i) + a.includes('x') + Array.isArray(a) +\n"
        "      s.indexOf('bd') + s.includes('ca') + s.startsWith('ab') + Object.hasOwn(o, 'k') + o.hasOwnProperty('j');\n"
        "  }\n"
        "  return a.length + ',' + n;\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "80,3104", NULL };

    sweep("fast built-ins", "fast_builtins.js", setup, "run()", want, 32768);
}

/* for in loops over objects, arrays, strings, null and a prototype, nested
   and in a generator: with too little memory (the iterator, its object, the
   empty shape the context then holds, the keys) they fail with the out of
   memory error and leave nothing behind. The first sweep makes the shape in
   its setup, the second makes it under the room. */
static void test_for_in_oom(void)
{
    static const char fns[] =
        "function keys(o) { var r = ''; for (var k in o) r += k; return r }\n"
        "function* gen(o) { for (var k in o) yield k }\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 4; i++) {\n"
        "    var g = gen({ g: i, h: i });\n"
        "    out.push(keys({ a: 1, b: 2 }), keys([1, 2]), keys('xy'), keys(null),\n"
        "      keys(Object.create({ p: 1 })), g.next().value);\n"
        "    for (var k in { n: 1 }) out.push(keys({ m: k }));\n"
        "    out.push(g.next().value);\n"
        "  }\n"
        "  return out.join();\n"
        "}\n";
    static const char *const want[] = {
        "ab,01,01,,p,g,m,h,ab,01,01,,p,g,m,h,ab,01,01,,p,g,m,h,ab,01,01,,p,g,m,h",
        NULL
    };
    char setup[sizeof(fns) + 16];

    snprintf(setup, sizeof(setup), "%srun();\n", fns);
    sweep("for in", "for_in.js", setup, "run()", want, 16384);
    sweep("for in, first loop", "for_in.js", fns, "run()", want, 32768);
}

/* Object.freeze and Object.seal in one pass over the shape
   (js_shape_set_integrity): of objects whose shape another object holds (the
   shape is copied first), of an object alone with its shape, of arrays whose
   elements are made properties first, of an empty array and of an object of
   40 keys, and Object.isFrozen and Object.isSealed of each. With too little
   memory a call fails with the out of memory error and loses nothing, and
   the same calls on new objects then give the same levels. */
static void test_integrity_oom(void)
{
    static const char setup[] =
        "function lv(o) { return (Object.isExtensible(o) ? '1' : '0') + (Object.isSealed(o) ? 1 : 0) +"
        " (Object.isFrozen(o) ? 1 : 0) }\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 4; i++) {\n"
        "    var a = { x: i, y: 2, get g() { return 1 } }, b = { x: i, y: 2, get g() { return 1 } };\n"
        "    var c = {}; c['k' + i] = 1; c.z = 2;\n"
        "    var d = [i, 2, 3], e = [], big = {};\n"
        "    for (var j = 0; j < 40; j++) big['p' + j] = j;\n"
        "    Object.freeze(a); Object.seal(b); Object.freeze(c); Object.freeze(d); Object.seal(e);\n"
        "    Object.freeze(big);\n"
        "    out.push(lv(a) + lv(b) + lv(c) + lv(d) + lv(e) + lv(big));\n"
        "  }\n"
        "  return out.join(' ');\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = {
        "011010011011010011 011010011011010011 011010011011010011 011010011011010011", NULL
    };

    sweep("integrity levels", "integrity.js", setup, "run()", want, 16384);
}

/* split, replace with a function and match by a regexp that nothing changed,
   which do their work without the calls of exec (js_regexp_split_fast,
   js_regexp_replace_fn, js_regexp_match_fast): with too little memory (the
   captures, the array and its pieces, the positions kept, the arguments of
   the function, the string made) they fail with the out of memory error, and
   leave nothing behind. */
static void test_regexp_fast_oom(void)
{
    static const char setup[] =
        "function run() {\n"
        "  var s = 'a1b22c333d4444e55555f'.repeat(4) + '\\u4e2d1\\u00e92', r = /(\\d)(\\d)?/g, out = [];\n"
        "  out.push(s.split(/(\\d)(x)?/).length, s.split(/\\d+/, 7).join('-'), s.split(/(?:)/u).length);\n"
        "  out.push(s.replace(r, (m, a, b, p) => (b === undefined ? '_' : b) + p).length, r.lastIndex);\n"
        "  out.push(s.replace(/[a-f]/y, m => m.toUpperCase()).slice(0, 3));\n"
        "  out.push(s.match(/\\d+/g).length, s.match(/(?:)/g).length, s.match(/(\\d)(\\d)/).index);\n"
        "  return out.join(',');\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = { "187,a-b-c-d-e-fa-b,88,136,0,A1b,22,89,3", NULL };

    sweep("regexp fast paths", "regexp_fast.js", setup, "run()", want, 32768);
}

/* Object.defineProperty, Object.defineProperties, Object.create and
   Reflect.defineProperty with plain descriptors, read from their shape
   (js_obj_to_desc_plain), and with descriptors that take the generic path (a
   getter, an inherited field, a proxy): with too little memory (the key, the
   property, the shape of the object, the result) they fail with the out of
   memory error and lose nothing. */
static void test_define_desc_oom(void)
{
    static const char setup[] =
        "var up = { enumerable: true }, fn = function () { return 1 };\n"
        "var px = new Proxy({ value: 'p', configurable: true }, {});\n"
        "function show(o) { return Object.keys(o).length + Object.getOwnPropertyNames(o).join('') }\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 6; i++) {\n"
        "    var o = {}, d = Object.create(up);\n"
        "    d.value = 'v' + i;\n"
        "    Object.defineProperty(o, 'a' + i, { value: i, writable: true, enumerable: i % 2 == 0 });\n"
        "    Object.defineProperty(o, 'b' + i, { get: fn, set: fn, configurable: true });\n"
        "    Object.defineProperty(o, 'c' + i, d);\n"
        "    Object.defineProperty(o, 'd' + i, { get value() { return i } });\n"
        "    Object.defineProperty(o, 'e' + i, px);\n"
        "    Object.defineProperties(o, { f: { value: 1, enumerable: true }, g: d });\n"
        "    Reflect.defineProperty(o, 'h' + i, { value: 'h', enumerable: true });\n"
        "    var c = Object.create(null, { x: { value: i, enumerable: true }, y: { get: fn } });\n"
        "    out.push(show(o), show(c), o['a' + i], o['b' + i], o['c' + i], o['d' + i], o['e' + i]);\n"
        "  }\n"
        "  return out.join(' ');\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = {
        "5a0b0c0d0e0fgh0 1xy 0 1 v0 0 p 4a1b1c1d1e1fgh1 1xy 1 1 v1 1 p "
        "5a2b2c2d2e2fgh2 1xy 2 1 v2 2 p 4a3b3c3d3e3fgh3 1xy 3 1 v3 3 p "
        "5a4b4c4d4e4fgh4 1xy 4 1 v4 4 p 4a5b5c5d5e5fgh5 1xy 5 1 v5 5 p",
        NULL
    };

    sweep("define descriptors", "define_desc.js", setup, "run()", want, 16384);
}

/* Function.prototype.bind makes the bound function, the copy of its
   arguments, its length and its name ("bound " and the name of the target):
   with too little memory it fails with the out of memory error, never
   returns a function without its length or name, and loses nothing. The
   targets: functions of each prototype, a C function, a class, a bound
   function, a name outside latin-1, a length that a getter gives, and a
   function of a new prototype, whose bound function needs new shapes. */
static void test_bind_oom(void)
{
    static const char setup[] =
        "function f(a, b, c) { return this.k + a + b + c }\n"
        "var g = function* gen(x) { }, h = async function () { }, ag = async function* agen(a, b) { };\n"
        "var named = function () { };\n"
        "Object.defineProperty(named, 'name', { value: 'n\\u00e9\\u4e2d' });\n"
        "var lenf = function () { };\n"
        "Object.defineProperty(lenf, 'length', { get: function () { return 2.5 } });\n"
        "function show(x) { return x.name + '/' + x.length + '/' + Object.getOwnPropertyNames(x).join('+') }\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 4; i++) {\n"
        "    var t = function (a) { };\n"
        "    Object.setPrototypeOf(t, Object.create(Function.prototype));\n"
        "    var b = f.bind({ k: i }, 1), bb = b.bind(null, 2);\n"
        "    var all = [b, bb, g.bind(), h.bind(), ag.bind(null, 1), Math.max.bind(null, i), named.bind(), lenf.bind(), (class C { }).bind(), t.bind()];\n"
        "    out.push(all.map(show).join(' '), bb(3));\n"
        "  }\n"
        "  return out.join(' ');\n"
        "}\n"
        /* once before the sweep, so that its register code is made */
        "run();\n";
    static const char *const want[] = {
        "bound f/2/length+name bound bound f/1/length+name bound gen/1/length+name "
        "bound h/0/length+name bound agen/1/length+name bound max/1/length+name "
        "bound n\xc3\xa9\xe4\xb8\xad/0/length+name bound lenf/2/length+name bound C/0/length+name bound t/1/length+name 6 "
        "bound f/2/length+name bound bound f/1/length+name bound gen/1/length+name "
        "bound h/0/length+name bound agen/1/length+name bound max/1/length+name "
        "bound n\xc3\xa9\xe4\xb8\xad/0/length+name bound lenf/2/length+name bound C/0/length+name bound t/1/length+name 7 "
        "bound f/2/length+name bound bound f/1/length+name bound gen/1/length+name "
        "bound h/0/length+name bound agen/1/length+name bound max/1/length+name "
        "bound n\xc3\xa9\xe4\xb8\xad/0/length+name bound lenf/2/length+name bound C/0/length+name bound t/1/length+name 8 "
        "bound f/2/length+name bound bound f/1/length+name bound gen/1/length+name "
        "bound h/0/length+name bound agen/1/length+name bound max/1/length+name "
        "bound n\xc3\xa9\xe4\xb8\xad/0/length+name bound lenf/2/length+name bound C/0/length+name bound t/1/length+name 9",
        NULL
    };

    sweep("bind", "bind.js", setup, "run()", want, 16384);
}

/* A host that calls bind sees either the bound function, whole, with no
   exception pending, or the exception with the out of memory error pending.
   (A script cannot see a bound function bind returned with the error
   pending: the next call fails and the next backward jump ends the loop.)
   The target has a new prototype whose empty shape another object shares,
   so the bound function gets its keys on copies of the shape, which are
   the requests that fail in the sweep of all sizes but one. */
#define BIND_BYTE_ROOMS (1 + 16384 / 8)
#define BIND_BUT_ONE (1024 / 8)

static void test_bind_host_oom(void)
{
    static const char setup[] =
        "var p = Object.create(Function.prototype), other = Object.create(p);\n"
        "var t = function (a, b) { };\n"
        "Object.setPrototypeOf(t, p);\n"
        "var bind = Function.prototype.bind;\n";
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), k;
    int failed = 0, made = 0;

    for (k = 0; k < BIND_BYTE_ROOMS + nfill + BIND_BUT_ONE; k++) {
        VM v = vm_new();
        JSValue g, bind, t, f, args[2], name, len;
        JSAtom alen, aname;
        int bytes = k < BIND_BYTE_ROOMS, butone = k >= BIND_BYTE_ROOMS + nfill, pending, unc;
        size_t n = bytes ? k * 8 : butone ? 8 * (k - BIND_BYTE_ROOMS - nfill + 1) : k - BIND_BYTE_ROOMS;
        const char *sweep = bytes ? "byte" : butone ? "all but" : "fill";
        char *s;

        CHECK(eval_ok(v.ctx, setup), "bind host: the setup failed");
        g = JS_GetGlobalObject(v.ctx);
        bind = JS_GetPropertyStr(v.ctx, g, "bind");
        t = JS_GetPropertyStr(v.ctx, g, "t");
        args[0] = JS_NULL;
        args[1] = JS_NewInt32(v.ctx, 1);
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
        f = JS_Call(v.ctx, bind, t, 2, args);
        pending = JS_HasException(v.ctx);
        unc = JS_IsUncatchableException(v.ctx);
        if (bytes)
            unlimit(&v);
        else
            release_blocks(&v);
        if (JS_IsException(f)) {
            failed++;
            CHECK(pending && unc, "bind host %s room %zu: pending %d, uncatchable %d", sweep, n, pending, unc);
            free(exception_text(v.ctx));
        } else {
            made++;
            CHECK(!pending, "bind host %s room %zu: a bound function came back with an exception pending", sweep, n);
            free(exception_text(v.ctx));
            alen = JS_NewAtom(v.ctx, "length");
            aname = JS_NewAtom(v.ctx, "name");
            CHECK(JS_GetOwnProperty(v.ctx, NULL, f, alen) == 1 &&
                  JS_GetOwnProperty(v.ctx, NULL, f, aname) == 1,
                  "bind host %s room %zu: no own length or name", sweep, n);
            JS_FreeAtom(v.ctx, alen);
            JS_FreeAtom(v.ctx, aname);
            name = JS_GetPropertyStr(v.ctx, f, "name");
            len = JS_GetPropertyStr(v.ctx, f, "length");
            s = value_text(v.ctx, name);
            CHECK(s && !strcmp(s, "bound t") && JS_VALUE_GET_TAG(len) == JS_TAG_INT && JS_VALUE_GET_INT(len) == 1,
                  "bind host %s room %zu: name %s, length tag %d", sweep, n, s ? s : "?", JS_VALUE_GET_TAG(len));
            free(s);
            JS_FreeValue(v.ctx, name);
            JS_FreeValue(v.ctx, len);
            JS_FreeValue(v.ctx, f);
        }
        JS_FreeValue(v.ctx, t);
        JS_FreeValue(v.ctx, bind);
        JS_FreeValue(v.ctx, g);
        /* a lost object ends the process in vm_free: say what failed first */
        fflush(stdout);
        vm_free(&v);
    }
    CHECK(failed > 0, "bind host: no room made it fail");
    CHECK(made > 0, "bind host: no room let it work");
}

/* startTag and endTag (JS_NewStartTagFunction, JS_NewEndTagFunction) make the
   names, styles and end tags they keep, the tables that hold them (made at the
   first entry and grown as they fill), the buffer the start tag keeps (freed
   after a long text), the string of the tag, the text of a number, and room
   in target: with too little memory they fail with the out of memory error,
   leave target as it was (the next run looks) and lose nothing. */
static void register_tags(JSContext *ctx)
{
    JSValue g = JS_GetGlobalObject(ctx);
    JSValue s = JS_NewStartTagFunction(ctx, "startTag", 8);
    JSValue e = JS_NewEndTagFunction(ctx, "endTag", 6);

    CHECK(!JS_IsException(s) && !JS_IsException(e), "no function");
    JS_SetPropertyStr(ctx, g, "startTag", s);
    JS_SetPropertyStr(ctx, g, "endTag", e);
    JS_FreeValue(ctx, g);
}

static void test_tags_oom(void)
{
    static const char setup[] =
        "var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null }, T = ['x'];\n"
        "var L = 'y'.repeat(1500);\n"
        /* pop is made at its first use: in the setup, not in a run with
           little memory, which would fail there with the tag pushed */
        "[0].pop();\n"
        "function run() {\n"
        "  var out = [];\n"
        "  for (var i = 0; i < 6; i++) {\n"
        "    if (T.length !== 1 || T[0] !== 'x') return 'target changed';\n"
        "    var p = { className: 'c' + i, ['data-' + i]: i, style: { marginTop: i, width: i + 0.5, ['--v' + i]: 'a&b' },\n"
        "      children: 'text <' + i + (i % 2 ? '\\u0100' : '') };\n"
        "    var r = startTag(T, p, 'div', fc, hop, d);\n"
        "    var s = T.length === 2 ? T.pop() : '';\n"
        "    if (r !== null) return 'returned ' + (r === d ? 'decline' : r);\n"
        "    out.push(s, endTag('div'), endTag('s' + i), endTag('u' + i), endTag('v' + i));\n"
        "  }\n"
        "  var q = { children: L };\n"
        "  if (startTag(T, q, 'p', fc, hop, d) !== null) return 'long declined';\n"
        "  if (T.length !== 2) return 'long text';\n"
        "  s = T.pop();\n"
        "  if (s !== '<p>' + L) return 'long text';\n"
        "  r = startTag(T, { id: 'z' }, 'i', fc, hop, d);\n"
        "  s = T.length === 2 ? T.pop() : '';\n"
        "  out.push('/' + r + s);\n"
        "  r = startTag(T, { alt: 'a<b', style: { width: 1 } }, 'img', fc, hop, d, 1);\n"
        "  s = T.length === 2 ? T.pop() : '';\n"
        "  out.push('/' + r + s);\n"
        "  r = startTag(T, { type: 'checkbox', value: 'v<', checked: true, name: 'n' + L.slice(0, 3) }, 'input', fc, hop, d, 2);\n"
        "  s = T.length === 2 ? T.pop() : '';\n"
        "  out.push('/' + r + s);\n"
        "  r = startTag(T, { 'data-emotion': 'css ' + L.slice(0, 3), dangerouslySetInnerHTML: { __html: '.a{}' + L.slice(0, 2) } }, 'style', fc, hop, d, 3);\n"
        "  s = T.length === 2 ? T.pop() : '';\n"
        "  out.push('/' + r + s);\n"
        "  return out.join('');\n"
        "}\n";
    static const char *const want[] = {
        "<div class=\"c0\" data-0=\"0\" style=\"margin-top:0;width:0.5px;--v0:a&amp;b\">text &lt;0</div></s0></u0></v0>"
        "<div class=\"c1\" data-1=\"1\" style=\"margin-top:1px;width:1.5px;--v1:a&amp;b\">text &lt;1\xc4\x80</div></s1></u1></v1>"
        "<div class=\"c2\" data-2=\"2\" style=\"margin-top:2px;width:2.5px;--v2:a&amp;b\">text &lt;2</div></s2></u2></v2>"
        "<div class=\"c3\" data-3=\"3\" style=\"margin-top:3px;width:3.5px;--v3:a&amp;b\">text &lt;3\xc4\x80</div></s3></u3></v3>"
        "<div class=\"c4\" data-4=\"4\" style=\"margin-top:4px;width:4.5px;--v4:a&amp;b\">text &lt;4</div></s4></u4></v4>"
        "<div class=\"c5\" data-5=\"5\" style=\"margin-top:5px;width:5.5px;--v5:a&amp;b\">text &lt;5\xc4\x80</div></s5></u5></v5>"
        "/null<i id=\"z\">/null<img alt=\"a&lt;b\" style=\"width:1px\"/>/null<input type=\"checkbox\" name=\"nyyy\" checked=\"\" value=\"v&lt;\"/>/null<style data-emotion=\"css yyy\">.a{}yy</style>",
        NULL
    };

    /* run first works with about 32.1 KiB of room (32888 bytes): the sweep
       goes well past it, so that a few more bytes of register code still
       leave a room that works */
    sweep_register = register_tags;
    sweep("startTag", "tags.js", setup, "run()", want, 49152);
    sweep_register = NULL;
}

/* Registering makes the cache of the tags, and Function.prototype.call and
   Array.prototype.push, which the engine makes at their first use. With too
   little memory it fails with the out of memory error. A failure before the
   cache is complete leaves no cache, and one after it, in making the function,
   leaves the whole cache to the next registration. Registering again with the
   memory back gives a function that works. */
static void test_tags_register_oom(void)
{
    static const char call[] =
        "var t = []; s(t, { a: 1 }, 'p', { viewTransition: null }, Object.prototype.hasOwnProperty, {}); t[0] + [].push.name";
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes = 4096 / 8 + 1, k;
    int failed = 0, worked = 0;

    for (k = 0; k < nbytes + nfill; k++) {
        VM v = vm_new();
        JSValue g, f;
        char *t;

        if (k < nbytes) {
            room(&v, k * 8);
        } else {
            fill_memory(&v);
            free_blocks(&v, rm[k - nbytes].min, rm[k - nbytes].max, rm[k - nbytes].n);
        }
        f = JS_NewStartTagFunction(v.ctx, "s", 1);
        if (k < nbytes)
            unlimit(&v);
        else
            release_blocks(&v);
        if (JS_IsException(f)) {
            failed++;
            CHECK(JS_IsUncatchableException(v.ctx), "register tags: room %zu: a catchable exception", k);
            free(exception_text(v.ctx));
            f = JS_NewStartTagFunction(v.ctx, "s", 1);
            CHECK(!JS_IsException(f), "register tags: room %zu: failed with the memory back", k);
        } else {
            worked++;
        }
        g = JS_GetGlobalObject(v.ctx);
        JS_SetPropertyStr(v.ctx, g, "s", f);
        JS_FreeValue(v.ctx, g);
        t = eval_text(v.ctx, call);
        CHECK(t && !strcmp(t, "<p a=\"1\">push"), "register tags: room %zu: %s", k, t ? t : "(exception)");
        free(t);
        free(exception_text(v.ctx));
        vm_free(&v);
    }
    CHECK(failed > 0 && worked > 0, "register tags: %d rooms failed, %d worked", failed, worked);
}

/* A script name in a regexp (\p{Script=...}, \p{Script_Extensions=...}) is
   turned into ranges of characters by unicode_script, which gives them back
   with an error when it runs out of memory. Its error path once jumped to
   itself, so a pattern compiled with too little memory never returned. With
   each room the compile either works or fails with the out of memory error.
   An alarm ends a run that hangs. */
static void test_regexp_script_oom(void)
{
    static const char *const want[] = { "true,false,true,false,false,false,false", NULL };

    alarm(300);
    sweep("regexp script", "script.js", NULL,
          "['\\\\p{Script=Greek}', '\\\\p{sc=Latn}', '\\\\p{scx=Greek}', '\\\\p{scx=Common}',"
          " '[\\\\p{Script=Zzzz}]', '\\\\p{scx=Latn}', '[^\\\\p{sc=Grek}]']"
          ".map(p => new RegExp(p, 'u').test('\\u03b1')).join()",
          want, 98304);
    alarm(0);
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

    /* the script first works with about 68 KiB of room: the hash table of
       the atoms keeps at most one atom for two buckets (62 KiB when it held
       two atoms a bucket). With the 4 bytes of key_atom in every string
       (engine/quickjs.c, JSString), the register loop's build no longer
       makes the 600 keys in 64 KiB */
    sweep("keys made from strings", "keys.js", NULL,
          "var o = {}; for (var i = 0; i < 600; i++) o['key' + i] = i; String(Object.keys(o).length)",
          want, 81920);
}

/* The decomposition of normalize grows its buffer (to_nfd), and out of
   memory there fails the call and frees the buffer. */
static void test_normalize_oom(void)
{
    static const char *const want[] = { "900", NULL };

    sweep("normalize", "normalize.js", NULL,
          "String('\\ufb03'.repeat(300).normalize('NFKD').length)", want, 16384);
}

/* A Map or Set whose table cannot grow keeps the table it has and goes on,
   with no exception left behind, and a record it adds has its value from the
   start, as a collection may run while the table grows. The 256th set
   doubles the table from 1 KiB to 2 KiB. f, called from here so that nothing
   is compiled, adds that record and returns the size, which needs no more
   memory. The setup calls f once with a key the map has, so that f is in
   register code before the limit: its first call converts it, which takes
   about 4 KiB. A room with no space for the growth (fewer than 1 KiB, or a
   full heap with no block of 2 KiB given back) still lets it work. */
#define MAP_GROW_BYTE_ROOMS (2048 / 8 + 1)
static void test_map_grow_oom(void)
{
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), k;
    int failed = 0, small_worked = 0;

    for (k = 0; k < nfill + MAP_GROW_BYTE_ROOMS; k++) {
        VM v = vm_new();
        JSValue g, f, r, key = JS_MKVAL(JS_TAG_INT, 255);
        int fail, unc, small;
        char *t;

        if (!eval_ok(v.ctx, "var m = new Map(); for (var i = 0; i < 255; i++) m.set(i, i);"
                            "function f(k) { m.set(k, k); return m.size } f(254);")) {
            CHECK(0, "Map grow: the setup failed");
            vm_free(&v);
            return;
        }
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, "f");
        JS_FreeValue(v.ctx, g);
        if (k < nfill) {
            fill_memory(&v);
            free_blocks(&v, rm[k].min, rm[k].max, rm[k].n);
            small = rm[k].n == 0 || rm[k].max < 2048;
        } else {
            room(&v, (k - nfill) * 8);
            small = (k - nfill) * 8 < 1024;
        }
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &key);
        fail = JS_IsException(r);
        unc = JS_IsUncatchableException(v.ctx);
        if (k < nfill)
            release_blocks(&v);
        else
            unlimit(&v);
        if (fail) {
            failed++;
            t = exception_text(v.ctx);
            CHECK(unc && t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                  "Map grow: room %zu: not the uncatchable out of memory error: %s", k, t ? t : "?");
            free(t);
        } else {
            CHECK(!JS_HasException(v.ctx), "Map grow: room %zu: it worked with an exception pending", k);
            free(exception_text(v.ctx));
            CHECK(JS_VALUE_GET_TAG(r) == JS_TAG_INT && JS_VALUE_GET_INT(r) == 256,
                  "Map grow: room %zu: f gave no 256", k);
            if (small)
                small_worked++;
        }
        JS_FreeValue(v.ctx, r);
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 1, &key);
        JS_FreeValue(v.ctx, r);
        t = eval_text(v.ctx, "var ok = m.size === 256;"
                             "for (var i = 0; i < 256; i++) ok = ok && m.get(i) === i; ok");
        CHECK(t && strcmp(t, "true") == 0, "Map grow: room %zu: with the memory back: %s", k, t ? t : "(exception)");
        free(t);
        free(exception_text(v.ctx));
        JS_FreeValue(v.ctx, f);
        vm_free(&v);
    }
    CHECK(failed > 0, "Map grow: no room made it fail");
    CHECK(small_worked > 0, "Map grow: no room without space for the growth let it work");
}

/* Appending to a variable grows its string in place or copies it with room
   to grow (JS_ConcatStringAppend). Out of memory at any copy fails the
   append and leaves the variable with the whole string it had: the closure
   variables are checked, after the failure, to hold whole pieces in order.
   Each kind of variable an add stores into is swept: a var and a let of a
   closure, a let and a var local, a var local that add_loc grows, and an
   argument. The pieces are 3 characters, 8-bit and 16-bit. */
static void test_string_append_oom(void)
{
    static const char setup[] =
        "function piece(k) { return k % 5 ? 'abc' : '\\u4e2d\\u6587x' }\n"
        "function whole(s) {\n"
        "  if (s.length % 3) return 'cut at ' + s.length\n"
        "  for (let k = 0; k < s.length / 3; k++) if (s.slice(3 * k, 3 * k + 3) !== piece(k)) return 'wrong at ' + k\n"
        "  return 'ok'\n"
        "}\n"
        "var N = 3000\n"
        "var closureVar = (() => { var s = ''; const add = x => { s = s + x }; return () => { while (s.length < N) add(piece(s.length / 3)); return whole(s) } })()\n"
        "var closureLet = (() => { let s = ''; const add = x => (s += x); return () => { while (s.length < N) add(piece(s.length / 3)); return whole(s) } })()\n"
        "function localLet() { let s = ''; for (let k = 0; k < N / 3; k++) s += piece(k); return whole(s) }\n"
        "function localVar() { var s = ''; for (var k = 0; k < N / 3; k++) s = s + piece(k); return whole(s) }\n"
        "function addLoc() { var s = '', p = 'abc'; for (var k = 0; k < N / 3; k++) s += p; return s.length === N ? 'ok' : 'cut' }\n"
        "function arg(s) { s = ''; for (let k = 0; k < N / 3; k++) s += piece(k); return whole(s) }\n";
    static const char *const want[] = { "ok", NULL };
    static const char *const calls[] = { "closureVar()", "closureLet()", "localLet()", "localVar()", "addLoc()", "arg()" };
    size_t i;

    for (i = 0; i < countof(calls); i++)
        sweep(calls[i], "append.js", setup, calls[i], want, 32768);
}

/* a + b of two strings in each form of the register loop's add (ADD, ADDL,
   ADDK, ADDKL, KADD, rv_add_str in regvm-ops-basic.h): with too little
   memory for the new string, the copy with room or the rope, it fails with
   the out of memory error and frees each operand once, and the variable it
   was to go into keeps its string. A variable appended to is written
   t = t + id(b): t = t + b of two locals is add_loc (ADDLOC), not ADDL.
   Three sweeps. run(): strings of 8 and 16 bits, short ones that are
   copied, long ones that make ropes, and self, which appends to a string
   another variable holds too, which must not change. It keeps each string
   it makes, about 118 KiB at its peak, so it is swept every 64 bytes up to
   128 KiB: each copy of 600 characters or more is then a new block that
   takes the count to a new peak. runArgs(): the adds
   into an argument, which the mapped arguments object still reads after
   the out of memory error has ended the call: check(), at the start of the
   next run, finds each argument holding the string it had or the whole new
   one. A function that has the arguments object reads its arguments into
   temps for an add, so the operands of an add into a temp are not seen
   here: r-stradd.js reads them after the length error, which leaves
   rv_add_str by the same path. runGrow(): grow appends to a string only
   its local holds, which js_add_string_into grows in place or copies with
   room (JS_ConcatStringAppend): it keeps only the old length, as a
   number, so that nothing else holds the string, and aGrow does it in an
   argument, which check() finds whole. Its answer is fixed, as the
   sweep's answer comes from the same engine: an append done wrong in
   place gives "cut".
   The strings are past 504 bytes, so that each new one is a large block
   the heap gives (docs/internals/allocator.md): a small one takes a free slot of an
   arena that is counted already, and would not fail. An add fails only
   when its block takes the count past the limit, so the runs free no large
   block before their adds: a freed one waits in the cache of large blocks,
   counted still, and the next add of its size takes it and adds nothing to
   the count. check() makes no string, run() and runArgs() keep the
   strings they make, and grow frees the block it copies from only for a
   copy half as large again, of another size. */
static void test_string_add_oom(void)
{
    static const char setup[] =
        "function sum(s) { let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0; return s.length + ':' + h }\n"
        "function hash(out) { let h = 0; for (const s of out) h = (h * 7 + s.length + s.charCodeAt(s.length - 1)) | 0; return out.length + ' ' + h }\n"
        "function id(x) { return x }\n"
        "function add(a, b) { return a + b }\n"
        "function addl(a, b) { let t = 0; t = a + b; return t }\n"
        "function addk(a) { return a + '<k>' }\n"
        "function addkl(a) { let t = 0; t = a + '\\u4e2d'; return t }\n"
        "function kadd(b) { return '<k>' + b }\n"
        "function kaddl(b) { let t = b; t = '\\u4e2d' + t; return t }\n"
        "function self(a, b) { let t = a; for (let i = 0; i < 8; i++) { const was = t; t = t + id(b); if (was.length !== t.length - b.length) return 'changed' } return t }\n"
        "function grow(b, n) { let t = 'p'.repeat(n); for (let i = 0; i < 40; i++) { const m = t.length; t = t + id(b); if (t.length !== m + b.length) return 'cut' } return t }\n"
        "function grown(v, n, b) {\n"
        "  if (typeof v !== 'string' || v.length < n || (v.length - n) % b.length) return false\n"
        "  for (let i = 0; i < v.length; i++) if (v.charCodeAt(i) !== (i < n ? 112 : b.charCodeAt((i - n) % b.length))) return false\n"
        "  return true\n"
        "}\n"
        "var seen = {}\n"
        "function rec(k, args, old, len) { seen[k] = { args, old, len } }\n"
        "function aAddl(t, a, b) { rec('addl', arguments, t, a.length + b.length); t = a + b; return t }\n"
        "function aAddkl(t, a) { rec('addkl', arguments, t, a.length + 1); t = a + '\\u4e2d'; return t }\n"
        "function aKadd(t, b) { rec('kadd', arguments, t, b.length + 1); t = '\\u4e2d' + b; return t }\n"
        "function aGrow(t, b) { rec('grow', arguments, [t.length, b], -2); for (let i = 0; i < 40; i++) t = t + id(b); return t }\n"
        "function check() {\n"
        "  for (const k in seen) {\n"
        "    const s = seen[k], v = s.args[0]\n"
        "    if (s.len === -2 ? !grown(v, s.old[0], s.old[1])\n"
        "        : !(v === s.old || (typeof v === 'string' && v.length === s.len))) return 'lost ' + k + ' ' + typeof v\n"
        "  }\n"
        "  return 'kept'\n"
        "}\n"
        "var parts = ['ab', '\\u4e2d\\u6587', 'x'.repeat(70), 'y'.repeat(600), '\\u00e9'.repeat(600)]\n"
        "function run() {\n"
        "  const out = []\n"
        "  for (const a of parts) {\n"
        "    for (const b of parts) out.push(add(a, b), addl(a, b), self(a, b))\n"
        "    out.push(addk(a), addkl(a), kadd(a), kaddl(a))\n"
        "  }\n"
        "  return hash(out.map(sum))\n"
        "}\n"
        "var l8 = 'y'.repeat(600), l16 = '\\u00e9'.repeat(600), m8 = 'x'.repeat(500)\n"
        "function runArgs() {\n"
        "  const was = check(), out = []\n"
        "  for (const [a, b] of [[l8, 'ab'], [l16, 'cd'], [l8, '\\u00e9'], [l16, m8]])\n"
        "    out.push(aAddl('old', a, b), aAddkl('old', a), aKadd('old', b === 'ab' ? m8 : b))\n"
        "  return was + ' ' + hash(out.map(sum))\n"
        "}\n"
        "function runGrow() {\n"
        "  const was = check(), b = 'q'.repeat(50), c = '\\u4e2d'.repeat(20)\n"
        "  const g = grow(b, 600), h = grow(c, 600), a = aGrow('p'.repeat(600), b)\n"
        "  return was + ' ' + (grown(g, 600, b) && grown(h, 600, c) && grown(a, 600, b) ? 'whole' : 'cut')\n"
        "}\n"
        /* once before the sweeps, so that their register code is made */
        "run(); runArgs(); runGrow()\n";
    static const struct { const char *what, *src, *answer; size_t step, max; } runs[] = {
        { "string add", "run()", NULL, 64, 131072 },
        { "string add into an argument", "runArgs()", NULL, 8, 32768 },
        { "string add grown in place", "runGrow()", "kept whole", 8, 32768 },
    };
    size_t i;

    for (i = 0; i < countof(runs); i++) {
        const char *want[] = { NULL, NULL };
        VM v = vm_new();
        JSValue r;

        /* the answer, from a run with all the memory it needs */
        CHECK(eval_ok(v.ctx, setup), "%s: the setup failed", runs[i].what);
        r = JS_Eval(v.ctx, runs[i].src, strlen(runs[i].src), "add.js", JS_EVAL_TYPE_GLOBAL);
        want[0] = value_text(v.ctx, r);
        JS_FreeValue(v.ctx, r);
        vm_free(&v);
        CHECK(want[0] != NULL, "%s: no answer", runs[i].what);
        CHECK(!runs[i].answer || (want[0] && strcmp(want[0], runs[i].answer) == 0),
              "%s: the answer is %s, not %s", runs[i].what, want[0] ? want[0] : "(none)", runs[i].answer);
        sweep_step = runs[i].step;
        if (want[0])
            sweep(runs[i].what, "add.js", setup, runs[i].src, want, runs[i].max);
        sweep_step = 8;
        free((char *)want[0]);
    }
}

/* An array that is appended to takes its first room in one allocation (push,
   an index at the length, spread, splice and the arrays of the engine's own
   builtins), and out of memory there fails the call with the out of memory
   error and loses nothing. Each sweep runs the script for every room of
   memory from nothing up and in a full heap. */
static void test_array_room_oom(void)
{
    static const struct { const char *name, *src, *want; } cases[] = {
        { "push", "var a = []; for (var i = 0; i < 10; i++) a.push(i); a.length + ':' + a.join(',')", "10:0,1,2,3,4,5,6,7,8,9" },
        { "push many", "var a = []; a.push(1, 2, 3, 4, 5, 6); a.length + ':' + a.join(',')", "6:1,2,3,4,5,6" },
        { "index", "var a = []; for (var i = 0; i < 9; i++) a[i] = i * 2; a.length + ':' + a.join(',')", "9:0,2,4,6,8,10,12,14,16" },
        { "spread", "var s = [1, 2, 3, 4, 5], a = [...s, ...s]; a.length + ':' + a.join(',')", "10:1,2,3,4,5,1,2,3,4,5" },
        { "splice", "var a = []; for (var i = 0; i < 6; i++) a.splice(a.length, 0, i); a.length + ':' + a.join(',')", "6:0,1,2,3,4,5" },
        { "entries", "var o = { a: 1, b: 2, c: 3 }; Object.entries(o).join(';')", "a,1;b,2;c,3" },
        { "string split", "'a,b,c,d,e'.split(',').join('')", "abcde" },
        { "JSON.parse", "JSON.stringify(JSON.parse('[1,[2,3],[4]]'))", "[1,[2,3],[4]]" },
    };
    size_t i;

    for (i = 0; i < countof(cases); i++) {
        const char *want[2];

        want[0] = cases[i].want;
        want[1] = NULL;
        /* up to 16 KiB and an arena of small blocks (4 KiB): where a
           script needs a new arena for a size class depends on the room
           the blocks made before it left in the arenas the runtime has
           (push many needs 12,872 bytes with shapes of 64 bytes before
           their keys and 16,968 with shapes of 72) */
        sweep(cases[i].name, "arrayroom.js", NULL, cases[i].src, want, 20480);
    }
}

/* An element added at the end of an array whose block must grow, which
   fails for want of memory, leaves the length as it was (upstream b25da59):
   the run with the memory back finds the last element where the length says
   it is. Before, the length counted the element that was not added. */
static void test_array_length_oom(void)
{
    static const char setup[] = "var a = 'x'.repeat(200).split('');\n";
    static const char src[] =
        "var n = a.length, last = (n - 1) in a;\n"
        "while (a.length < 1000) a[a.length] = 0;\n"
        "last ? 'no hole' : 'a hole at ' + (n - 1)";
    static const char *const want[] = { "no hole", NULL };

    sweep_step = 64;
    sweep("the length of a failed append", "arraylength.js", setup, src, want, 65536);
    sweep_step = 8;
}

/* An element stored after the last one of an array whose length cannot be
   written does nothing (a[0] = 1 in sloppy code) or gives false
   (Reflect.set), with a full heap too: the length is tested before room is
   made for the element. Before, the room came first, and with no memory the
   call ended with the uncatchable out of memory error. */
static void test_array_readonly_length_oom(void)
{
    static const char setup[] =
        "var a = Object.defineProperty([], 'length', { writable: false });\n"
        "function put() { a[0] = 1; return 7; }\n"
        "function reflect() { return Reflect.set(a, 0, 1); }\n"
        "put(); reflect();\n"
        /* a new array, with no room yet */
        "a = Object.defineProperty([], 'length', { writable: false });\n";
    static const char *const names[] = { "put", "reflect" };
    static const char *const want[] = { "7", "false" };
    int i;

    for (i = 0; i < 2; i++) {
        VM v = vm_new();
        JSValue g, f, r;
        char *t;
        int fail, unc;

        CHECK(eval_ok(v.ctx, setup), "%s: the setup", names[i]);
        g = JS_GetGlobalObject(v.ctx);
        f = JS_GetPropertyStr(v.ctx, g, names[i]);
        fill_memory(&v);
        r = JS_Call(v.ctx, f, JS_UNDEFINED, 0, NULL);
        fail = JS_IsException(r);
        unc = JS_IsUncatchableException(v.ctx);
        release_blocks(&v);
        t = fail ? exception_text(v.ctx) : value_text(v.ctx, r);
        CHECK(!fail && t && strcmp(t, want[i]) == 0, "%s on a length that cannot be written, a full heap: %s%s, want %s",
              names[i], fail ? (unc ? "uncatchable " : "exception ") : "", t ? t : "?", want[i]);
        free(t);
        JS_FreeValue(v.ctx, r);
        CHECK(eval_is(v.ctx, "a.length === 0 && !(0 in a)", "true"), "%s: the array changed", names[i]);
        JS_FreeValue(v.ctx, f);
        JS_FreeValue(v.ctx, g);
        vm_free(&v);
    }
}

/* Values read back from bytecode with little memory left, from a full heap
   with a few blocks back and with each room up to 4 KiB: the read gives the
   value whole or fails with the out of memory error. A frozen array is
   written as a template object. When the array or the object could not be
   made, the reader went on with the exception as if it were one, and sealed
   it, which crashed for an array with no element and no raw strings. */
static void test_read_object_oom(void)
{
    static const struct { const char *make, *check; } cases[] = {
        { "Object.freeze([])", "Array.isArray(r) && Object.isFrozen(r) && r.length === 0" },
        { "(function tag(s) { return s; })`a${1}b`", "Object.isFrozen(r) && Object.isFrozen(r.raw) && r.join() === 'a,b'" },
        { "({ a: 1, b: [2] })", "r.a === 1 && r.b[0] === 2" },
    };
    FillRoom rm[FILL_ROOMS];
    size_t nrooms = fill_rooms(rm), k, size;
    int i, made, failed, unc;

    for (i = 0; i < (int)countof(cases); i++) {
        VM v = vm_new();
        JSValue val = JS_Eval(v.ctx, cases[i].make, strlen(cases[i].make), "<unit>", JS_EVAL_TYPE_GLOBAL);
        uint8_t *buf;

        CHECK(!JS_IsException(val), "%s: the eval", cases[i].make);
        buf = JS_WriteObject(v.ctx, &size, val, JS_WRITE_OBJ_BYTECODE);
        JS_FreeValue(v.ctx, val);
        CHECK(buf != NULL, "%s: written", cases[i].make);
        made = failed = 0;
        for (k = 0; buf && k < nrooms + 4096 / 8 + 1; k++) {
            JSValue r, g;

            if (k < nrooms) {
                fill_memory(&v);
                free_blocks(&v, rm[k].min, rm[k].max, rm[k].n);
            } else {
                room(&v, (k - nrooms) * 8);
            }
            r = JS_ReadObject(v.ctx, buf, size, JS_READ_OBJ_BYTECODE);
            unc = JS_IsException(r) && JS_IsUncatchableException(v.ctx);
            if (k < nrooms)
                release_blocks(&v);
            else
                unlimit(&v);
            if (JS_IsException(r)) {
                char *t = exception_text(v.ctx);

                failed++;
                CHECK(unc, "%s, room %zu: the read fails with %s, not the out of memory error", cases[i].make, k, t ? t : "?");
                free(t);
                continue;
            }
            made++;
            g = JS_GetGlobalObject(v.ctx);
            CHECK(JS_SetPropertyStr(v.ctx, g, "r", r) >= 0, "global r");
            JS_FreeValue(v.ctx, g);
            CHECK(eval_is(v.ctx, cases[i].check, "true"), "%s, room %zu: the value read is not whole", cases[i].make, k);
            CHECK(eval_ok(v.ctx, "delete globalThis.r"), "delete r");
        }
        CHECK(made > 0 && failed > 0, "%s: %d reads made the value and %d failed, want both", cases[i].make, made, failed);
        js_free(v.ctx, buf);
        vm_free(&v);
    }
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

/* Built-ins and steps of the interpreter that define a property on the
   object they made, with too little memory: each fails with the out of
   memory error. They went on with the error pending and returned the object
   without the property: the prototype of a generator function or of a
   function, the length of a class, the length of a String object, the
   message, cause and errors of an error, the result of Proxy.revocable and
   the descriptor a defineProperty trap gets. f is called from C, made and
   converted with memory, so that its first allocations are the ones of the
   step and no compile before it takes more memory than the step needs. */
static void test_define_after_make_oom(void)
{
    static const struct {
        const char *what, *expr, *want;
    } cases[] = {
        { "generator function", "function* () {}", "function* () {}" },
        { "function", "function () {}", "function () {}" },
        { "class", "class { constructor(a, b) {} }", "class { constructor(a, b) {} }" },
        { "Object of a string", "Object('abc')", "abc" },
        { "String", "new String('xy')", "xy" },
        { "Error", "new Error('m', { cause: 1 })", "Error: m" },
        { "AggregateError", "new AggregateError([1, 2], 'n')", "AggregateError: n" },
        { "Proxy.revocable", "Proxy.revocable({}, {})", "[object Object]" },
        { "defineProperty trap",
          "(Object.defineProperty(p, 'x', { value: 1, writable: true, enumerable: true, configurable: true }), got)",
          "value,writable,enumerable,configurable" },
    };
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes = 4096 / 8 + 1, i, k;
    char setup[512];

    for (i = 0; i < countof(cases); i++) {
        int failed = 0, worked = 0;

        snprintf(setup, sizeof setup,
                 "var go = 0, got, p = new Proxy({}, { defineProperty(t, k, d) { got = Object.keys(d).join(); return true; } });\n"
                 "Object.defineProperty(p, 'y', { value: 0 });\n"
                 "var f = () => go ? (%s) : 0;\n"
                 "f(); go = 1;\n", cases[i].expr);
        for (k = 0; k < nbytes + nfill; k++) {
            VM v = vm_new();
            JSValue r;
            char *t;

            CHECK(eval_ok(v.ctx, setup), "%s: the setup failed", cases[i].what);
            if (k < nbytes) {
                room(&v, k * 8);
            } else {
                fill_memory(&v);
                free_blocks(&v, rm[k - nbytes].min, rm[k - nbytes].max, rm[k - nbytes].n);
            }
            r = call_global(v.ctx, "f");
            if (k < nbytes)
                unlimit(&v);
            else
                release_blocks(&v);
            if (JS_IsException(r)) {
                failed++;
                CHECK(JS_IsUncatchableException(v.ctx), "%s: room %zu: a catchable error", cases[i].what, k);
                t = exception_text(v.ctx);
                CHECK(t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                      "%s: room %zu: not the out of memory error: %s", cases[i].what, k, t ? t : "?");
            } else {
                t = exception_text(v.ctx);
                CHECK(!t, "%s: room %zu: a value with %s pending", cases[i].what, k, t);
                free(t);
                t = value_text(v.ctx, r);
                CHECK(t && strcmp(t, cases[i].want) == 0, "%s: room %zu: got %s", cases[i].what, k, t ? t : "(no text)");
                if (t && strcmp(t, cases[i].want) == 0)
                    worked++;
            }
            free(t);
            JS_FreeValue(v.ctx, r);
            vm_free(&v);
        }
        CHECK(failed > 0, "%s: no room made it fail", cases[i].what);
        CHECK(worked > 0, "%s: no room let it work", cases[i].what);
    }
}

/* A RegExp made from a RegExp with a new target C, called from C as a host
   does, with too little memory: the object takes the shape of C.prototype,
   which keep, made with that prototype, holds too, so its lastIndex is a
   key added to a shape of its own, which takes memory. With none, the call
   fails with the out of memory error. It went on with the error pending and
   gave the RegExp without lastIndex. A call that works gives the RegExp,
   with lastIndex 0 and no error pending. A room where the object itself
   cannot be made leaves no block (vm_free): it kept a reference to the
   code of the seed. */
static void test_regexp_new_target_oom(void)
{
    static const char setup[] =
        "var seed = /x/g; function C() {} C.prototype = Object.create(RegExp.prototype);\n"
        "var keep = Object.create(C.prototype); 0";
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes = 4096 / 8 + 1, k;
    int failed = 0, worked = 0;

    for (k = 0; k < nbytes + nfill; k++) {
        VM v = vm_new();
        JSValue g, re, c, seed, r;
        char *t;

        CHECK(eval_ok(v.ctx, setup), "the setup failed");
        g = JS_GetGlobalObject(v.ctx);
        re = JS_GetPropertyStr(v.ctx, g, "RegExp");
        c = JS_GetPropertyStr(v.ctx, g, "C");
        seed = JS_GetPropertyStr(v.ctx, g, "seed");
        if (k < nbytes) {
            room(&v, k * 8);
        } else {
            fill_memory(&v);
            free_blocks(&v, rm[k - nbytes].min, rm[k - nbytes].max, rm[k - nbytes].n);
        }
        r = JS_CallConstructor2(v.ctx, re, c, 1, (JSValueConst *)&seed);
        if (k < nbytes)
            unlimit(&v);
        else
            release_blocks(&v);
        if (JS_IsException(r)) {
            failed++;
            CHECK(JS_IsUncatchableException(v.ctx), "room %zu: a catchable error", k);
            t = exception_text(v.ctx);
            CHECK(t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                  "room %zu: not the out of memory error: %s", k, t ? t : "?");
        } else {
            t = exception_text(v.ctx);
            CHECK(!t, "room %zu: a RegExp with %s pending", k, t);
            free(t);
            JS_SetPropertyStr(v.ctx, g, "r", JS_DupValue(v.ctx, r));
            t = eval_text(v.ctx, "[r.source, r.flags, Object.hasOwn(r, 'lastIndex'), r.lastIndex, r instanceof C].join()");
            CHECK(t && strcmp(t, "x,g,true,0,true") == 0, "room %zu: got %s", k, t ? t : "(exception)");
            if (t && strcmp(t, "x,g,true,0,true") == 0)
                worked++;
        }
        free(t);
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, seed);
        JS_FreeValue(v.ctx, c);
        JS_FreeValue(v.ctx, re);
        JS_FreeValue(v.ctx, g);
        vm_free(&v);
    }
    CHECK(failed > 0, "RegExp with a new target: no room made it fail");
    CHECK(worked > 0, "RegExp with a new target: no room let it work");
}

/* The jobs of promises with too little memory, the resolving functions of a
   promise called from C as a host calls them, with a reaction waiting on
   the promise: a resolve with a thenable, a resolve and a reject each
   queue a job, and a job with no memory fails the call with the out of
   memory error, with the promise pending or settled with its reaction
   lost. A call that works
   settles the promise and leaves no error pending. The resolve with a
   thenable dropped the failure: it returned undefined, with the error
   pending and the promise pending for good. */
static void test_promise_jobs_oom(void)
{
    static const char setup[] =
        "var r = 0, t = { then(f) { f(1); } };\n"
        "function watch(p) { p.then(v => { r = v; }, e => { r = -e; }); }\n";
    static const struct {
        const char *what;
        int reject, thenable;
        const char *want;
    } cases[] = {
        { "resolve with a thenable", 0, 1, "1" },
        { "resolve with reactions", 0, 0, "5" },
        { "reject with reactions", 1, 0, "-5" },
    };
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes = 4096 / 8 + 1, i, k;

    for (i = 0; i < countof(cases); i++) {
        int failed = 0, worked = 0;

        for (k = 0; k < nbytes + nfill; k++) {
            VM v = vm_new();
            JSValue funcs[2], p, g, w, arg, r;
            char *t;

            eval_ok(v.ctx, setup);
            p = JS_NewPromiseCapability(v.ctx, funcs);
            CHECK(!JS_IsException(p), "%s: no promise", cases[i].what);
            g = JS_GetGlobalObject(v.ctx);
            w = JS_GetPropertyStr(v.ctx, g, "watch");
            JS_FreeValue(v.ctx, JS_Call(v.ctx, w, JS_UNDEFINED, 1, (JSValueConst *)&p));
            JS_FreeValue(v.ctx, w);
            if (cases[i].thenable)
                arg = JS_GetPropertyStr(v.ctx, g, "t");
            else
                arg = JS_NewInt32(v.ctx, 5);
            if (k < nbytes) {
                room(&v, k * 8);
            } else {
                fill_memory(&v);
                free_blocks(&v, rm[k - nbytes].min, rm[k - nbytes].max, rm[k - nbytes].n);
            }
            r = JS_Call(v.ctx, funcs[cases[i].reject], JS_UNDEFINED, 1, (JSValueConst *)&arg);
            if (k < nbytes)
                unlimit(&v);
            else
                release_blocks(&v);
            if (JS_IsException(r)) {
                failed++;
                CHECK(JS_IsUncatchableException(v.ctx), "%s: room %zu: a catchable error", cases[i].what, k);
                t = exception_text(v.ctx);
                CHECK(t && (strstr(t, "out of memory") || strcmp(t, "null") == 0),
                      "%s: room %zu: not the out of memory error: %s", cases[i].what, k, t ? t : "?");
                free(t);
            } else {
                t = exception_text(v.ctx);
                CHECK(!t, "%s: room %zu: the call worked with %s pending", cases[i].what, k, t);
                free(t);
                drain_jobs(&v);
                if (eval_is(v.ctx, "String(r)", cases[i].want))
                    worked++;
                else
                    CHECK(0, "%s: room %zu: the reaction did not run", cases[i].what, k);
            }
            JS_FreeValue(v.ctx, r);
            JS_FreeValue(v.ctx, arg);
            JS_FreeValue(v.ctx, g);
            JS_FreeValue(v.ctx, p);
            JS_FreeValue(v.ctx, funcs[0]);
            JS_FreeValue(v.ctx, funcs[1]);
            drain_jobs(&v);
            vm_free(&v);
        }
        CHECK(failed > 0, "%s: no room made it fail", cases[i].what);
        CHECK(worked > 0, "%s: no room let it work", cases[i].what);
    }
}

/* Running out of memory in a step of async work leaves that work as a stop
   does: it may never settle, and the VM is kept only to be read and
   closed. What must hold is that no call or job ends with a value and an
   error left pending, and that closing the runtime frees every block
   (vm_free): a return on a finished generator whose job of the await gets
   no memory leaves the generator waiting on that return for good, and an
   await whose promise is resolved with a thenable can lose the job of the
   thenable or its resolving functions. go runs under each room, from C,
   and the jobs after it too. Each room is run twice: once the VM is closed
   as soon as a call or a job failed, as a host that drops it does, and
   once after a call of after and the rest of the jobs with no limit. No
   promise is checked. */
static void test_async_oom_frees(void)
{
    static const struct {
        const char *what, *setup;
    } cases[] = {
        { "a return of a settled value on a finished generator",
          "var g = (async function* () {})(); g.next();\n"
          "var v = Promise.resolve(1);\n"
          "function go() { g.return(v); g.next(); g.next(); }\n"
          "function after() { return g.next(); }\n" },
        { "an await of a generator resolved with a thenable",
          "var res, p = new Promise(r => { res = r; });\n"
          "var g = (async function* () { await p; return 1; })();\n"
          "g.next(); g.next();\n"
          "var t = { then(f) { f(1); } };\n"
          "function go() { res(t); }\n"
          "function after() { return g.next(); }\n" },
        { "an await of an async function resolved with a thenable",
          "var res, p = new Promise(r => { res = r; });\n"
          "(async function () { await p; return 1; })();\n"
          "var t = { then(f) { f(1); } };\n"
          "function go() { res(t); }\n"
          "function after() { return 0; }\n" },
    };
    FillRoom rm[FILL_ROOMS];
    size_t nfill = fill_rooms(rm), nbytes = 4096 / 8 + 1, i, k;
    int drop;

    for (i = 0; i < countof(cases); i++) {
      for (drop = 0; drop < 2; drop++) {
        int failed = 0;

        for (k = 0; k < nbytes + nfill; k++) {
            VM v = vm_new();
            JSContext *jctx;
            JSValue r;
            int ret, fail = 0;
            char *t;

            eval_ok(v.ctx, cases[i].setup);
            if (k < nbytes) {
                room(&v, k * 8);
            } else {
                fill_memory(&v);
                free_blocks(&v, rm[k - nbytes].min, rm[k - nbytes].max, rm[k - nbytes].n);
            }
            r = call_global(v.ctx, "go");
            CHECK(JS_IsException(r) == JS_HasException(v.ctx),
                  "%s: room %zu: the call returned %s with %s pending", cases[i].what, k,
                  JS_IsException(r) ? "an exception" : "a value",
                  JS_HasException(v.ctx) ? "an exception" : "nothing");
            if (JS_IsException(r))
                fail = 1;
            free(exception_text(v.ctx));
            JS_FreeValue(v.ctx, r);
            while (!(drop && fail) && (ret = JS_ExecutePendingJob(v.rt, &jctx)) != 0) {
                CHECK((ret < 0) == JS_HasException(v.ctx),
                      "%s: room %zu: a job returned %d with %s pending", cases[i].what, k,
                      ret, JS_HasException(v.ctx) ? "an exception" : "nothing");
                if (ret < 0)
                    fail = 1;
                free(exception_text(v.ctx));
            }
            failed += fail;
            /* the blocks fill_memory took go back to the runtime before it
               is closed, as they are the test's and no part of the VM */
            if (k < nbytes)
                unlimit(&v);
            else
                release_blocks(&v);
            if (!(drop && fail)) {
                JS_FreeValue(v.ctx, call_global(v.ctx, "after"));
                t = exception_text(v.ctx);
                CHECK(!t, "%s: room %zu: after threw %s", cases[i].what, k, t);
                free(t);
                drain_jobs(&v);
            }
            vm_free(&v);
        }
        CHECK(failed > 0, "%s, drop %d: no room made it fail", cases[i].what, drop);
      }
    }
}

/* ---- the rejections kept for the host ---- */

/* The two allocations of keeping rejections, each with no memory: the list,
   which JS_TrackRejections makes and fails with -1 for, keeping nothing and
   changing nothing, and the entry of a rejection, which the tracker counts
   as dropped when it gets no memory: the rejection itself goes through, the
   promise is rejected, and nothing leaks (vm_free). With memory, the same
   rejection is kept. */
static void test_rejections_oom(void)
{
    static const char setup[] = "var rej; var p = new Promise((_, r) => { rej = r }); p";
    int full;

    for (full = 0; full < 2; full++) {
        VM v = vm_new();
        JSValue arg = JS_NewInt32(v.ctx, 7), g, rej, p, r;
        int32_t n = 0;

        if (full) {
            fill_memory(&v);
            CHECK(JS_TrackRejections(v.rt, 4) == -1, "the list made with a full heap");
            release_blocks(&v);
            CHECK(eval_ok(v.ctx, "Promise.reject(1)"), "the script threw");
            r = JS_TakeRejection(v.rt, JS_RejectionMark(v.rt));
            CHECK(JS_IsUninitialized(r), "a rejection kept with no list");
            JS_FreeValue(v.ctx, r);
            CHECK(JS_TakeDroppedRejections(v.rt) == 0, "dropped with no list");
        }
        CHECK(JS_TrackRejections(v.rt, 4) == 0, "full %d: no list", full);
        p = JS_Eval(v.ctx, setup, strlen(setup), "<unit>", JS_EVAL_TYPE_GLOBAL);
        CHECK(JS_IsObject(p), "the setup failed");
        g = JS_GetGlobalObject(v.ctx);
        rej = JS_GetPropertyStr(v.ctx, g, "rej");
        JS_FreeValue(v.ctx, g);
        if (full)
            fill_memory(&v);
        r = JS_Call(v.ctx, rej, JS_UNDEFINED, 1, &arg);
        if (full)
            release_blocks(&v);
        CHECK(!JS_IsException(r), "full %d: the rejection threw", full);
        JS_FreeValue(v.ctx, r);
        CHECK(JS_PromiseState(v.ctx, p) == JS_PROMISE_REJECTED, "full %d: not rejected", full);
        CHECK(JS_TakeDroppedRejections(v.rt) == (uint32_t)full, "full %d: dropped", full);
        r = JS_TakeRejection(v.rt, JS_RejectionMark(v.rt));
        if (full) {
            CHECK(JS_IsUninitialized(r), "full %d: a rejection kept", full);
        } else {
            JS_ToInt32(v.ctx, &n, r);
            CHECK(n == 7, "full %d: the reason %d", full, n);
        }
        JS_FreeValue(v.ctx, r);
        JS_FreeValue(v.ctx, rej);
        JS_FreeValue(v.ctx, p);
        vm_free(&v);
    }
}

/* ---- the register loop ---- */

/* A function converts to register code at its first call, with the memory
   the call has: under a memory limit with no room for the register code
   the call fails with the out of memory error, and gives its answer at a
   limit with room. f(3) converts f and g, which takes about 12.3 KiB at its
   peak, with the scratch of the pass (engine/regvm-pass.h), and 4 KiB more
   for an arena of small blocks of a size the setup asked for none of: f
   holds g itself, not a JSVarRef (capture_by_value), so the setup made no
   block of that size. So the rooms go up to 20 KiB. The register stack is
   made by JS_NewRuntime, before the limit (test_regvm_stack_oom).
   Each allocation of the pass failing in turn is test_regvm_convert_alloc
   of unit-white.c. */
static void test_regvm_convert_oom(void)
{
    static const char *const want[] = { "5493", NULL };
    char setup[1024];
    size_t n;
    int i;

    n = snprintf(setup, sizeof(setup), "var f = (function () { \"use strict\";\n"
                 "function g(x) { return x");
    for (i = 1; i <= 60; i++)
        n += snprintf(setup + n, sizeof(setup) - n, " + %d", i);
    snprintf(setup + n, sizeof(setup) - n, "; }\n"
             "return function (k) { var s = 0, i; for (i = 0; i < k; i++) s = s + g(i); return s; };\n"
             "})();");
    sweep("conversion to register code", "<regvm>", setup, "f(3)", want, 20480);
}

/* The conversion of a function with a switch of 31 string cases, whose
   CASES gets a table (rv_cases_table in engine/regvm-ops-cmp.h): the table
   is made in the code of the pass, which goes into the register code in one
   block at the end, so a conversion that runs out of memory leaves no part
   of a table behind, and the call that converts it later answers as the
   chain does. The call takes about 22.1 KiB at its peak, so the rooms go up
   to 32 KiB. */
static void test_regvm_cases_oom(void)
{
    static const char *const want[] = { "16", NULL };
    char setup[2048];
    size_t n;
    int i;

    n = snprintf(setup, sizeof(setup), "var f = Function('s', \"'use strict'; switch (s) {");
    for (i = 0; i < 31; i++)
        n += snprintf(setup + n, sizeof(setup) - n, " case 'c%d': return %d;", i, i);
    snprintf(setup + n, sizeof(setup) - n, " } return -1;\");");
    sweep("conversion of a case table", "<cases>", setup, "f('c17') + f('zz')", want, 32768);
}

/* The arguments object of a function that reads only arguments.length and
   arguments[key] is built at the first key that is not an index of an
   argument (ARGGET, engine/regvm-ops-args.h), into its local: each
   allocation of the build failing in turn ends in the out of memory error
   with nothing lost (the local is written only with the whole object), and
   the reads after the build in the same call see the object. The same for
   a function that also lets the object go, where it is built at that read
   (ARGMAT). The same for the mapped object of a sloppy function, whose
   parameters may be written first, and for f.apply(t, arguments) (CALLMA):
   a call of a function of bytecode makes no object, and a call of a C
   function builds it, where each allocation fails in turn. */
static void test_regvm_args_oom(void)
{
    static const char setup[] =
        "var f = (function () { 'use strict';\n"
        "  function g(a, b) {\n"
        "    var r = arguments.length + ':' + arguments[0] + ':' + arguments[3];\n"
        "    return r + ':' + arguments['1'] + ':' + arguments.length + ':' + arguments[2];\n"
        "  }\n"
        "  return function () { return g('x', 'y', 'z') + '|' + g('w'); };\n"
        "})();\n";
    static const char *const want[] = { "3:x:undefined:y:3:z|1:w:undefined:undefined:1:undefined", NULL };

    static const char setup_escape[] =
        "var h = (function () { 'use strict';\n"
        "  function g(a) {\n"
        "    var n = arguments.length;\n"
        "    var s = n > 1 ? Array.prototype.join.call(arguments, '-') : '';\n"
        "    return n + ':' + s + ':' + arguments[0] + ':' + arguments[n - 1];\n"
        "  }\n"
        "  return function () { return g('x', 'y', 'z') + '|' + g('w'); };\n"
        "})();\n";
    static const char *const want_escape[] = { "3:x-y-z:x:z|1::w:w", NULL };

    sweep("arguments built on a read", "<args>", setup, "f()", want, 16384);
    static const char setup_mapped[] =
        "var m = (function () {\n"
        "  function g(a, b) {\n"
        "    a = 'A';\n"
        "    var r = arguments.length + ':' + arguments[0] + ':' + arguments[3];\n"
        "    return r + ':' + arguments[1] + ':' + a;\n"
        "  }\n"
        "  return function () { return g('x', 'y', 'z') + '|' + g('w'); };\n"
        "})();\n";
    static const char *const want_mapped[] = { "3:A:undefined:y:A|1:A:undefined:undefined:A", NULL };

#define SETUP_APPLY \
        "var fw = (function () {\n" \
        "  function show() {\n" \
        "    var s = this.k + ':' + Array.prototype.join.call(arguments, ',');\n" \
        "    return s + ':' + s.repeat(1000).length;\n" \
        "  }\n" \
        "  function f(a) { a = 'A'; return show.apply(this, arguments); }\n" \
        "  function m(a) { return Math.max.apply(null, arguments) + ':' + arguments[5]; }\n" \
        "  function s(a) { 'use strict'; return Math.min.apply(null, arguments); }\n" \
        "  var o = { k: 'o' };\n" \
        "  var r = function () { return f.call(o, 1, 2) + '|' + m(3, 9, 4) + '|' + s(5, 1); };\n" \
        "  r.show = show;\n" \
        "  return r;\n" \
        "})();\n"
    /* cold, show is called first under the limit: not converted yet, the
       call goes out of the loop to the C apply, which builds the object.
       Warm, show was called before the limit and is converted: f calls it
       in the loop, and running out of memory in show leaves its frame,
       which keeps apply (RV_DST_APPLY), with apply freed. show makes a
       string of 5000 bytes, more than the compile of fw() takes and gives
       back, so that some rooms run out there. Math.max and Math.min are C
       functions, called by the C apply either way. The rooms go to 24 KiB:
       cold, fw() works from about 17 KiB (13 KiB before a closure was one
       block, js_new_function_object): a closure made there is a block of a
       size class with no arena yet, which takes one of 4 KiB. */
    static const char setup_apply[] = SETUP_APPLY;
    static const char setup_apply_warm[] = SETUP_APPLY "fw();\n";
#undef SETUP_APPLY
    static const char *const want_apply[] = { "o:A,2:5000|9:undefined|1", NULL };
    VM v;
    JSValue show;

    sweep("arguments built where they escape", "<args>", setup_escape, "h()", want_escape, 16384);
    sweep("mapped arguments built on a read", "<args>", setup_mapped, "m()", want_mapped, 16384);
    /* cold, the conversion of the five functions is in the room too: past
       16 KiB once a read of a name keeps its slot in the code */
    sweep("arguments passed on by apply", "<args>", setup_apply, "fw()", want_apply, 24576);
    v = vm_new();
    CHECK(eval_ok(v.ctx, setup_apply_warm), "apply, warm: the setup failed");
    show = JS_Eval(v.ctx, "fw.show", strlen("fw.show"), "<args>", JS_EVAL_TYPE_GLOBAL);
    CHECK(JS_RegvmFuncStat(v.ctx, show, 0) == 1, "apply, warm: show is not in the register loop");
    JS_FreeValue(v.ctx, show);
    vm_free(&v);
    sweep("arguments passed on by apply to a converted function", "<args>", setup_apply_warm,
          "fw()", want_apply, 24576);
}

/* Array destructuring with no iterator (engine/regvm-ops-dst.h) makes the
   Array Iterator only when it needs it: at a read the array cannot give
   (a hole, an array no longer fast), also after a default changed the next
   of %ArrayIteratorPrototype%, when the record gets a new built-in next, at
   a close that would read a return, and in the unwinding when an exception
   passes the destructuring and there is a return to call. The first
   destructuring makes the built-in next, which the engine makes at its
   first use. Each allocation of making them failing in turn
   ends in the out of memory error with nothing lost, and the call after it
   answers as one with room does. Running out of memory skips the finally
   blocks that put back the return and the next of %ArrayIteratorPrototype%,
   so each call puts them back first. */
static void test_regvm_dst_oom(void)
{
    static const char setup[] =
        "var f = (function () { 'use strict';\n"
        "  var AIP = Object.getPrototypeOf([][Symbol.iterator]()), next0, log = [];\n"
        "  function holes() { var a = [1, 2]; a.length = 4; const [x, y, z = 'z', w] = a; return [x, y, z, w].join(); }\n"
        "  function slow(a) { const [x = (a[50] = 5, 'd'), y, ...r] = a; return x + y + r.length; }\n"
        "  function swap() { var a = [undefined, 2], old = next0 = AIP.next; a.length = 3;\n"
        "    try { const [x = (AIP.next = function () { log.push('n'); return { done: true }; }, 'x'), y, z = 'z'] = a;\n"
        "      return [x, y, z].join(); } finally { AIP.next = old; } }\n"
        "  function close() { AIP.return = function () { log.push('r'); return {}; };\n"
        "    try { const [x] = [1, 2, 3]; return x; } finally { delete AIP.return; } }\n"
        "  function unwind() { AIP.return = function () { log.push('u'); return {}; };\n"
        "    try { const [x, y = (() => { throw 'e'; })()] = [1, undefined]; } catch (e) { return e; } finally { delete AIP.return; } }\n"
        "  return function () { log.length = 0; delete AIP.return; if (next0) AIP.next = next0;\n"
        "    return [holes(), slow([undefined, 2, 3]), swap(), close(), unwind(), log.join('')].join('|'); };\n"
        "})();\n";
    static const char *const want[] = { "1,2,z,|d249|x,2,z|1|e|ru", NULL };

    sweep("array destructuring that makes its iterator", "<dst>", setup, "f()", want, 16384);
}

/* Closures that hold the values of the variables they capture, which no
   code changes once they are captured (capture_by_value, engine/quickjs.c):
   the values go into the block of the closure, so each allocation of
   making such a closure, in a plain function, two functions down and in a
   generator that goes on after it made one, failing in turn ends in the
   out of memory error with nothing lost, and the call after it answers as
   one with room does. */
static void test_capture_value_oom(void)
{
    static const char setup[] =
        "var f = (function () { 'use strict';\n"
        "  function mk(a, b) { const c = a + ':' + b, o = { a }; let d = [a]; d.push(b); return () => c + o.a + d.length + a + b; }\n"
        "  function* gen(n) { const k = 's' + n; const g = () => k; yield g(); yield g; }\n"
        "  function deep(x) { return () => () => x + '!'; }\n"
        "  return function () { var r = [], i, it; for (i = 0; i < 3; i++) r.push(mk(i, 'v' + i)());\n"
        "    it = gen(4); r.push(it.next().value, it.next().value()); r.push(deep('q')()()); return r.join(','); };\n"
        "})();\n";
    static const char *const want[] = { "0:v0020v0,1:v1121v1,2:v2222v2,s4,s4,q!", NULL };

    sweep("closures that hold values", "<capture>", setup, "f()", want, 16384);
}

/* A function whose stack is too deep for the temps folds it into Arrays at
   run time (engine/regvm-fold.h), where QuickJS's interpreter allocates
   nothing: in two levels (calls of 32 arguments nested 40 deep), under the
   catch offset of a try in a for-in (the enumerator frozen into an Array of
   its own) and under that of an array destructuring. Each allocation of a
   fold, an unfold, a freeze or a thaw failing in turn ends in the out of
   memory error with nothing lost. */
static void test_regvm_fold_oom(void)
{
    static const char setup[] =
        "var f = (function () {\n"
        "  function plus(n, inner) { var s = '', i; for (i = 0; i < n; i++) s += 'a[' + i + '] + (';\n"
        "    s += inner; for (i = 0; i < n; i++) s += ')'; return s; }\n"
        "  function calls(n, last) { var s = '', i, j; for (i = 0; i < n; i++) { s += 'g('; for (j = 0; j < 31; j++) s += j + ', '; }\n"
        "    s += last; for (i = 0; i < n; i++) s += ')'; return s; }\n"
        "  var a = [], i, g = function () { return arguments.length + arguments[arguments.length - 1]; };\n"
        "  for (i = 0; i < 300; i++) a.push(i % 7);\n"
        "  var h = Function('a, g, o, b', \"'use strict'; var r = [], k, x, y;\\n\" +\n"
        "    'for (k in o) {\\n  try { r.push(' + plus(300, 'o[k]()') + '); } catch (e) { r.push(e.message); }\\n}\\n' +\n"
        "    'r.push(' + plus(200, '([x, y] = b, x + y)') + ', ' + calls(40, 'k') + ');\\nreturn r.join();');\n"
        "  var o = { p: function () { return 1; }, q: function () { throw new RangeError('q'); } }, b = [3, 4];\n"
        "  return function () { return h(a, g, o, b); };\n"
        "})();\n"
        /* once before the sweep: the call that converts it */
        "f();\n";
    static const char *const want[] = { "898,q,601,32323232323232323232323232323232323232323232323232323232323232323232323232323232q", NULL };
    /* the conversion of a function that folds, at its first call in the
       sweep: each allocation of the pass failing ends in the out of memory
       error, and a later call converts it (in the test builds, where every
       function folds, the stack model of a small one fails too: scripts 6
       and 11 of test_compile_oom) */
    static const char conv[] =
        "var a = [], i;\n"
        "for (i = 0; i < 130; i++) a.push(i % 7);\n"
        "var h = Function('a', \"'use strict'; return \" +\n"
        "  Array.from({ length: 130 }, function (_, i) { return 'a[' + i + '] + ('; }).join('') +\n"
        "  '1' + ')'.repeat(130) + ';');\n";
    static const char *const conv_want[] = { "385", NULL };

#ifdef RV_FOLD_TEST
    /* the test build folds into Arrays of 2 or 3 entries, many more of
       them: every 64 bytes up to 256 KiB */
    sweep_step = 64;
    sweep("fold", "<fold>", setup, "f()", want, 262144);
    sweep_step = 8;
#else
    sweep("fold", "<fold>", setup, "f()", want, 32768);
#endif
    /* the conversion takes more than the run: every 32 bytes up to 128 KiB */
    sweep_step = 32;
    sweep("fold conversion", "<fold>", conv, "h(a)", conv_want, 131072);
    sweep_step = 8;
}

/* the state of the function the script src returns in the register loop
   (JS_RegvmFuncStat 0), or -9 when src threw */
static int regvm_state_of(JSContext *ctx, const char *src)
{
    JSValue f = JS_Eval(ctx, src, strlen(src), "<regvm>", JS_EVAL_TYPE_GLOBAL);
    int st;

    if (JS_IsException(f)) {
        free(exception_text(ctx));
        return -9;
    }
    st = JS_RegvmFuncStat(ctx, f, 0);
    JS_FreeValue(ctx, f);
    return st;
}

/* JS_NewRuntime makes the first segment of register stack (about 32 KiB,
   RV_SEG_FIRST of engine/regvm.h), so that it counts under a memory limit
   from the start: a function converted under a tight limit later needs room
   for its code only, and runs in the register loop. */
static void test_regvm_stack_oom(void)
{
    static const char g[] =
        "var g = function (x) { 'use strict'; return x + 1; }; g(1); g(2); g";
    VM v = vm_new();
    int st;

    CHECK(JS_RegvmStat(v.ctx, 8) == 1, "a new runtime has %d segments of register stack, want 1", JS_RegvmStat(v.ctx, 8));
    room(&v, 24 * 1024);
    st = regvm_state_of(v.ctx, g);
    CHECK(st == 1, "a function converted under a tight limit: state %d, want 1", st);
    CHECK(eval_is(v.ctx, "g(41)", "42"), "the function converted under a tight limit gives its answer");
    unlimit(&v);
    vm_free(&v);
}

/* The pass keeps its scratch between conversions (engine/regvm-pass.h):
   up to 64 KiB, given back by a collection, at the end of a conversion
   that needed more, and when the heap fails a request, as the front cache
   is. So a kept scratch never makes a script run out of memory: a request
   that the limit leaves no room for gets the room of the scratch. A
   function that folds takes the larger buffers of a fold, so the test
   builds that fold every function (RV_FOLD_TEST) use one of 100 lines where
   the others use 300, to keep its scratch between 16 and 64 KiB. */
#ifdef RV_FOLD_TEST
#define SCRATCH_LINES 100
#else
#define SCRATCH_LINES 300
#endif
static void test_regvm_scratch(void)
{
    char src[16384], want[32];
    size_t n, kept, size;
    int i, big;
    VM v = vm_new();

    n = snprintf(src, sizeof(src), "var f = function (x) { 'use strict'; var s = 0;");
    for (i = 0; i < SCRATCH_LINES; i++)
        n += snprintf(src + n, sizeof(src) - n, " s = s + x * %d;", i);
    snprintf(src + n, sizeof(src) - n, " return s; }; f(1)");
    snprintf(want, sizeof(want), "%d", SCRATCH_LINES * (SCRATCH_LINES - 1) / 2);
    CHECK(eval_is(v.ctx, src, want), "the function of %d lines does not give %s", SCRATCH_LINES, want);
    kept = JS_RegvmStat(v.ctx, 11);
    CHECK(kept >= 16384 && kept <= 64 * 1024, "the scratch kept after a function of %d lines is %zu bytes", SCRATCH_LINES, kept);
    JS_RunGC(v.rt);
    CHECK(JS_RegvmStat(v.ctx, 11) == 0, "a collection leaves %d bytes of scratch", JS_RegvmStat(v.ctx, 11));
    /* a request the limit has room for only with the scratch given back */
    n = snprintf(src, sizeof(src), "var h = function (x) { 'use strict'; var s = 0;");
    for (i = 0; i < SCRATCH_LINES; i++)
        n += snprintf(src + n, sizeof(src) - n, " s = s - x * %d;", i);
    snprintf(src + n, sizeof(src) - n, " return s; }; h(1)");
    snprintf(want, sizeof(want), "-%d", SCRATCH_LINES * (SCRATCH_LINES - 1) / 2);
    CHECK(eval_is(v.ctx, src, want), "the second function of %d lines does not give %s", SCRATCH_LINES, want);
    kept = JS_RegvmStat(v.ctx, 11);
    CHECK(kept >= 16384, "the scratch kept is %zu bytes", kept);
    size = JS_GetMemorySize(v.rt);
    JS_SetMemoryLimit(v.rt, size + 8192);
    snprintf(src, sizeof(src), "new ArrayBuffer(%zu).byteLength", kept - 8192);
    n = kept - 8192;
    snprintf(want, sizeof(want), "%zu", n);
    CHECK(eval_is(v.ctx, src, want), "an ArrayBuffer of the bytes of the scratch less 8 KiB, 8 KiB under the limit, does not get them");
    /* the conversion of the script may have made a small one again */
    CHECK((size_t)JS_RegvmStat(v.ctx, 11) < kept, "the heap failed a request and the scratch kept %d bytes", JS_RegvmStat(v.ctx, 11));
    unlimit(&v);
    /* a function whose scratch is over 64 KiB gives it back at once */
    n = snprintf(src, sizeof(src), "var k = function (x) { 'use strict'; var s = 0;");
    for (big = 0; n + 64 < sizeof(src); big++)
        n += snprintf(src + n, sizeof(src) - n, " s = s + x * %d;", big);
    snprintf(src + n, sizeof(src) - n, " return s; }; k(1) > 0");
    CHECK(eval_is(v.ctx, src, "true"), "the function of %d lines does not run", big);
    CHECK(JS_RegvmStat(v.ctx, 11) == 0, "a conversion that needed more than 64 KiB keeps %d bytes of scratch", JS_RegvmStat(v.ctx, 11));
    vm_free(&v);
}

int main(void)
{
    nested_scopes();
    test_throw_error_oom();
    test_tocstring_oom();
    test_define_oom();
    test_function_name_oom();
    test_compile_oom();
    test_new_convert_oom();
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
    test_uncatchable_then_loop();
    test_uncatchable_fast_call();
    test_uncatchable_instanceof();
    test_uncatchable_apply_forward();
    test_uncatchable_regexp_fast();
    test_regexp_oom_last_index();
    test_uncatchable_flag_bit();
    test_weakref_kept_oom();
    test_symbol_copy_oom();
    test_regexp_oom();
    test_regexp_script_oom();
    test_thrown_is_not_uncatchable();
    test_new_error_stack_oom();
    test_syntax_error_stack_oom();
    test_empty_string_full_heap();
    test_finrec_job_full_heap();
    test_define_after_make_oom();
    test_regexp_new_target_oom();
    test_promise_jobs_oom();
    test_async_oom_frees();
    test_rejections_oom();
    test_split_paths_oom();
    test_compile_names_oom();
    test_key_names_oom();
    test_normalize_oom();
    test_map_grow_oom();
    test_array_room_oom();
    test_array_length_oom();
    test_array_readonly_length_oom();
    test_read_object_oom();
    test_string_append_oom();
    test_string_add_oom();
    test_copy_props_oom();
    test_has_property_oom();
    test_getel_local_key_oom();
    test_putel_local_key_oom();
    test_putel_atom_oom();
    test_shape_cache_drop_oom();
    test_truth_oom();
    test_array_literal_oom();
    test_fast_builtins_oom();
    test_for_in_oom();
    test_regexp_fast_oom();
    test_define_desc_oom();
    test_integrity_oom();
    test_bind_oom();
    test_bind_host_oom();
    test_tags_oom();
    test_tags_register_oom();
    test_regvm_convert_oom();
    test_regvm_cases_oom();
    test_regvm_args_oom();
    test_capture_value_oom();
    test_regvm_dst_oom();
    test_regvm_fold_oom();
    test_regvm_stack_oom();
    test_regvm_scratch();
    if (failures) {
        printf("unit-oom: %d of %ld checks failed\n", failures, checks);
        return 1;
    }
    printf("unit-oom: %ld checks passed\n", checks);
    return 0;
}
