/*
 * QuickJS Javascript Engine
 *
 * Copyright (c) 2017-2021 Fabrice Bellard
 * Copyright (c) 2017-2021 Charlie Gordon
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 * THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */
#ifndef QUICKJS_H
#define QUICKJS_H

#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "heap.h"

#ifdef __cplusplus
extern "C" {
#endif

#if defined(__GNUC__) || defined(__clang__)
#define js_likely(x)          __builtin_expect(!!(x), 1)
#define js_unlikely(x)        __builtin_expect(!!(x), 0)
#define js_force_inline       inline __attribute__((always_inline))
#define __js_printf_like(f, a)   __attribute__((format(printf, f, a)))
#else
#define js_likely(x)     (x)
#define js_unlikely(x)   (x)
#define js_force_inline  inline
#define __js_printf_like(a, b)
#endif

#define JS_BOOL int

typedef struct JSRuntime JSRuntime;
typedef struct JSContext JSContext;
typedef struct JSClass JSClass;
typedef uint32_t JSClassID;
typedef uint32_t JSAtom;

#if INTPTR_MAX >= INT64_MAX
#define JS_PTR64
#define JS_PTR64_DEF(a) a
#else
#define JS_PTR64_DEF(a)
#endif

#ifndef JS_PTR64
#define JS_NAN_BOXING
#endif

#if defined(__SIZEOF_INT128__) && (INTPTR_MAX >= INT64_MAX)
#define JS_LIMB_BITS 64
#else
#define JS_LIMB_BITS 32
#endif

#define JS_SHORT_BIG_INT_BITS JS_LIMB_BITS
    
enum {
    /* all tags with a reference count are negative */
    JS_TAG_FIRST       = -9, /* first negative tag */
    JS_TAG_BIG_INT     = -9,
    JS_TAG_SYMBOL      = -8,
    JS_TAG_STRING      = -7,
    JS_TAG_STRING_ROPE = -6,
    JS_TAG_MODULE      = -3, /* used internally */
    JS_TAG_FUNCTION_BYTECODE = -2, /* used internally */
    JS_TAG_OBJECT      = -1,

    JS_TAG_INT         = 0,
    JS_TAG_BOOL        = 1,
    JS_TAG_NULL        = 2,
    JS_TAG_UNDEFINED   = 3,
    JS_TAG_UNINITIALIZED = 4,
    JS_TAG_CATCH_OFFSET = 5,
    JS_TAG_EXCEPTION   = 6,
    JS_TAG_SHORT_BIG_INT = 7,
    JS_TAG_FLOAT64     = 8,
    /* any larger tag is FLOAT64 if JS_NAN_BOXING */
};

/* must match the layout of 'JSMallocBlockHeader' */
typedef struct JSRefCountHeader {
    int ref_count;
} JSRefCountHeader;

/* The reference count of every block of the allocator, small or large, is
   this many bytes before the user data. */
#define JS_BLOCK_REF_COUNT_OFFSET 4

#define JS_FLOAT64_NAN NAN

#ifdef CONFIG_CHECK_JSVALUE
/* JSValue consistency : it is not possible to run the code in this
   mode, but it is useful to detect simple reference counting
   errors. It would be interesting to modify a static C analyzer to
   handle specific annotations (clang has such annotations but only
   for objective C) */
typedef struct __JSValue *JSValue;
typedef const struct __JSValue *JSValueConst;

#define JS_VALUE_GET_TAG(v) (int)((uintptr_t)(v) & 0xf)
/* same as JS_VALUE_GET_TAG, but return JS_TAG_FLOAT64 with NaN boxing */
#define JS_VALUE_GET_NORM_TAG(v) JS_VALUE_GET_TAG(v)
#define JS_VALUE_GET_INT(v) (int)((intptr_t)(v) >> 4)
#define JS_VALUE_GET_BOOL(v) JS_VALUE_GET_INT(v)
#define JS_VALUE_GET_FLOAT64(v) (double)JS_VALUE_GET_INT(v)
#define JS_VALUE_GET_SHORT_BIG_INT(v) JS_VALUE_GET_INT(v)
#define JS_VALUE_GET_PTR(v) (void *)((intptr_t)(v) & ~0xf)

#define JS_MKVAL(tag, val) (JSValue)(intptr_t)(((val) << 4) | (tag))
#define JS_MKPTR(tag, p) (JSValue)((intptr_t)(p) | (tag))

#define JS_TAG_IS_FLOAT64(tag) ((unsigned)(tag) == JS_TAG_FLOAT64)

#define JS_NAN JS_MKVAL(JS_TAG_FLOAT64, 1)

static inline JSValue __JS_NewFloat64(JSContext *ctx, double d)
{
    return JS_MKVAL(JS_TAG_FLOAT64, (int)d);
}

static inline JS_BOOL JS_VALUE_IS_NAN(JSValue v)
{
    return 0;
}

static inline JSValue __JS_NewShortBigInt(JSContext *ctx, int32_t d)
{
    return JS_MKVAL(JS_TAG_SHORT_BIG_INT, d);
}

#elif defined(JS_NAN_BOXING)

typedef uint64_t JSValue;

#define JSValueConst JSValue

#define JS_VALUE_GET_TAG(v) (int)((v) >> 32)
#define JS_VALUE_GET_INT(v) (int)(v)
#define JS_VALUE_GET_BOOL(v) (int)(v)
#define JS_VALUE_GET_SHORT_BIG_INT(v) (int)(v)
#define JS_VALUE_GET_PTR(v) (void *)(intptr_t)(v)

#define JS_MKVAL(tag, val) (((uint64_t)(tag) << 32) | (uint32_t)(val))
#define JS_MKPTR(tag, ptr) (((uint64_t)(tag) << 32) | (uintptr_t)(ptr))

#define JS_FLOAT64_TAG_ADDEND (0x7ff80000 - JS_TAG_FIRST + 1) /* quiet NaN encoding */

static inline double JS_VALUE_GET_FLOAT64(JSValue v)
{
    union {
        JSValue v;
        double d;
    } u;
    u.v = v;
    u.v += (uint64_t)JS_FLOAT64_TAG_ADDEND << 32;
    return u.d;
}

#define JS_NAN (0x7ff8000000000000 - ((uint64_t)JS_FLOAT64_TAG_ADDEND << 32))

static inline JSValue __JS_NewFloat64(JSContext *ctx, double d)
{
    union {
        double d;
        uint64_t u64;
    } u;
    JSValue v;
    u.d = d;
    /* normalize NaN */
    if (js_unlikely((u.u64 & 0x7fffffffffffffff) > 0x7ff0000000000000))
        v = JS_NAN;
    else
        v = u.u64 - ((uint64_t)JS_FLOAT64_TAG_ADDEND << 32);
    return v;
}

#define JS_TAG_IS_FLOAT64(tag) ((unsigned)((tag) - JS_TAG_FIRST) >= (JS_TAG_FLOAT64 - JS_TAG_FIRST))

/* same as JS_VALUE_GET_TAG, but return JS_TAG_FLOAT64 with NaN boxing */
static inline int JS_VALUE_GET_NORM_TAG(JSValue v)
{
    uint32_t tag;
    tag = JS_VALUE_GET_TAG(v);
    if (JS_TAG_IS_FLOAT64(tag))
        return JS_TAG_FLOAT64;
    else
        return tag;
}

static inline JS_BOOL JS_VALUE_IS_NAN(JSValue v)
{
    uint32_t tag;
    tag = JS_VALUE_GET_TAG(v);
    return tag == (JS_NAN >> 32);
}

static inline JSValue __JS_NewShortBigInt(JSContext *ctx, int32_t d)
{
    return JS_MKVAL(JS_TAG_SHORT_BIG_INT, d);
}

#else /* !JS_NAN_BOXING */

typedef union JSValueUnion {
    uint64_t uint64;
    double float64;
    void *ptr;
#if JS_SHORT_BIG_INT_BITS == 32
    int32_t short_big_int;
#else
    int64_t short_big_int;
#endif
} JSValueUnion;

typedef struct JSValue {
    JSValueUnion u;
    int64_t tag;
} JSValue;

#define JSValueConst JSValue

#define JS_VALUE_GET_TAG(v) ((int32_t)(v).tag)
/* same as JS_VALUE_GET_TAG, but return JS_TAG_FLOAT64 with NaN boxing */
#define JS_VALUE_GET_NORM_TAG(v) JS_VALUE_GET_TAG(v)
#define JS_VALUE_GET_INT(v) ((int)(v).u.uint64)
#define JS_VALUE_GET_BOOL(v) ((int)(v).u.uint64)
#define JS_VALUE_GET_FLOAT64(v) ((v).u.float64)
#define JS_VALUE_GET_SHORT_BIG_INT(v) ((v).u.short_big_int)
#define JS_VALUE_GET_PTR(v) ((v).u.ptr)

/* avoid uninitialized data by using a 64 bit field even if only 32
   bits are needed because some compilers generate slower code */
#define JS_MKVAL(tag, val) (JSValue){ (JSValueUnion){ .uint64 = (uint32_t)(val) }, tag }
#define JS_MKPTR(tag, p) (JSValue){ (JSValueUnion){ .ptr = p }, tag }

#define JS_TAG_IS_FLOAT64(tag) ((unsigned)(tag) == JS_TAG_FLOAT64)

#define JS_NAN (JSValue){ .u.float64 = JS_FLOAT64_NAN, JS_TAG_FLOAT64 }

static inline JSValue __JS_NewFloat64(JSContext *ctx, double d)
{
    JSValue v;
    v.tag = JS_TAG_FLOAT64;
    v.u.float64 = d;
    return v;
}

static inline JS_BOOL JS_VALUE_IS_NAN(JSValue v)
{
    union {
        double d;
        uint64_t u64;
    } u;
    if (v.tag != JS_TAG_FLOAT64)
        return 0;
    u.d = v.u.float64;
    return (u.u64 & 0x7fffffffffffffff) > 0x7ff0000000000000;
}

static inline JSValue __JS_NewShortBigInt(JSContext *ctx, int64_t d)
{
    JSValue v;
    v.tag = JS_TAG_SHORT_BIG_INT;
    v.u.short_big_int = d;
    return v;
}

#endif /* !JS_NAN_BOXING */

#define JS_VALUE_IS_BOTH_INT(v1, v2) ((JS_VALUE_GET_TAG(v1) | JS_VALUE_GET_TAG(v2)) == 0)
#define JS_VALUE_IS_BOTH_FLOAT(v1, v2) (JS_TAG_IS_FLOAT64(JS_VALUE_GET_TAG(v1)) && JS_TAG_IS_FLOAT64(JS_VALUE_GET_TAG(v2)))

#define JS_VALUE_HAS_REF_COUNT(v) ((unsigned)JS_VALUE_GET_TAG(v) >= (unsigned)JS_TAG_FIRST)

/* special values */
#define JS_NULL      JS_MKVAL(JS_TAG_NULL, 0)
#define JS_UNDEFINED JS_MKVAL(JS_TAG_UNDEFINED, 0)
#define JS_FALSE     JS_MKVAL(JS_TAG_BOOL, 0)
#define JS_TRUE      JS_MKVAL(JS_TAG_BOOL, 1)
#define JS_EXCEPTION JS_MKVAL(JS_TAG_EXCEPTION, 0)
#define JS_UNINITIALIZED JS_MKVAL(JS_TAG_UNINITIALIZED, 0)

/* flags for object properties */
#define JS_PROP_CONFIGURABLE  (1 << 0)
#define JS_PROP_WRITABLE      (1 << 1)
#define JS_PROP_ENUMERABLE    (1 << 2)
#define JS_PROP_C_W_E         (JS_PROP_CONFIGURABLE | JS_PROP_WRITABLE | JS_PROP_ENUMERABLE)
#define JS_PROP_LENGTH        (1 << 3) /* used internally in Arrays */
#define JS_PROP_TMASK         (3 << 4) /* mask for NORMAL, GETSET, VARREF, AUTOINIT */
#define JS_PROP_NORMAL         (0 << 4)
#define JS_PROP_GETSET         (1 << 4)
#define JS_PROP_VARREF         (2 << 4) /* used internally */
#define JS_PROP_AUTOINIT       (3 << 4) /* used internally */

/* flags for JS_DefineProperty */
#define JS_PROP_HAS_SHIFT        8
#define JS_PROP_HAS_CONFIGURABLE (1 << 8)
#define JS_PROP_HAS_WRITABLE     (1 << 9)
#define JS_PROP_HAS_ENUMERABLE   (1 << 10)
#define JS_PROP_HAS_GET          (1 << 11)
#define JS_PROP_HAS_SET          (1 << 12)
#define JS_PROP_HAS_VALUE        (1 << 13)

/* throw an exception if false would be returned
   (JS_DefineProperty/JS_SetProperty) */
#define JS_PROP_THROW            (1 << 14)
/* throw an exception if false would be returned in strict mode
   (JS_SetProperty) */
#define JS_PROP_THROW_STRICT     (1 << 15)

#define JS_PROP_NO_EXOTIC        (1 << 16) /* internal use */

#ifndef JS_DEFAULT_STACK_SIZE
#define JS_DEFAULT_STACK_SIZE (1024 * 1024)
#endif

/* JS_Eval() flags */
#define JS_EVAL_TYPE_GLOBAL   (0 << 0) /* global code (default) */
#define JS_EVAL_TYPE_MODULE   (1 << 0) /* module code */
#define JS_EVAL_TYPE_DIRECT   (2 << 0) /* direct call (internal use) */
#define JS_EVAL_TYPE_INDIRECT (3 << 0) /* indirect call (internal use) */
#define JS_EVAL_TYPE_MASK     (3 << 0)

#define JS_EVAL_FLAG_STRICT   (1 << 3) /* force 'strict' mode */
/* compile but do not run. The result is an object with a
   JS_TAG_FUNCTION_BYTECODE or JS_TAG_MODULE tag. It can be executed
   with JS_EvalFunction(). */
#define JS_EVAL_FLAG_COMPILE_ONLY (1 << 5)
/* don't include the stack frames before this eval in the Error() backtraces */
#define JS_EVAL_FLAG_BACKTRACE_BARRIER (1 << 6)
/* allow top-level await in normal script. JS_Eval() returns a
   promise. Only allowed with JS_EVAL_TYPE_GLOBAL */
#define JS_EVAL_FLAG_ASYNC (1 << 7)

typedef JSValue JSCFunction(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv);
typedef JSValue JSCFunctionMagic(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
typedef JSValue JSCFunctionData(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic, JSValue *func_data);

typedef struct JSGCObjectHeader JSGCObjectHeader;

/* The runtime lives in the memory the host gives the VM vm
   (host_memory_grow in heap.h). seed is where Math.random starts, in every
   context of the runtime: the engine reads no clock for it. 0 is allowed. */
JSRuntime *JS_NewRuntime(uint32_t vm, uint64_t seed);
/* info lifetime must exceed that of rt */
void JS_SetRuntimeInfo(JSRuntime *rt, const char *info);
void JS_SetMemoryLimit(JSRuntime *rt, size_t limit);
void JS_SetGCThreshold(JSRuntime *rt, size_t gc_threshold);
/* bytes the host may be asked for in all, 0 for no limit. It only stops
   growth. */
void JS_SetFootprintLimit(JSRuntime *rt, size_t limit);
/* what the memory limit and the GC threshold count, and the threshold */
size_t JS_GetMemorySize(JSRuntime *rt);
/* The host calls this right after JS_NewRuntime when it can remap a chunk (see
   HOST_CALL_MEMORY_REMAP in host.h). Off by default. */
void JS_SetHostRemap(JSRuntime *rt, int on);
size_t JS_GetGCThreshold(JSRuntime *rt);
/* give the free memory back to the host except keep bytes: JS_ReleaseStart,
   then JS_ReleaseNextChunk until its length is 0 and JS_ReleaseNextRange
   until its length is 0 (see heap.h). The runtime may not be used in
   between. */
void JS_ReleaseStart(JSRuntime *rt, size_t keep);
JSMemRange JS_ReleaseNextChunk(JSRuntime *rt);
JSMemRange JS_ReleaseNextRange(JSRuntime *rt);
/* the most freed small blocks of a size class the allocator keeps for the
   next allocation, 0 to 256. 0 also turns off the cache of freed large
   blocks (blocks of 505 to 4096 bytes, see docs/allocator.md), and any other
   value leaves it on. For tests and measuring */
void JS_SetFrontCache(JSRuntime *rt, int max);
/* The call depth: how many calls may run at once, each call of a function
   (built-in functions included) and each entry of the host that runs code
   (JS_Call, JS_Eval and the like) counting one. Past it a script gets a
   RangeError. It starts at JS_DEFAULT_MAX_CALL_DEPTH. JS_SetMaxCallDepth
   returns 0, or -1 and changes nothing when depth is not 1 to
   JS_MAX_CALL_DEPTH_LIMIT or when the runtime is running code. */
#define JS_DEFAULT_MAX_CALL_DEPTH 5000
#define JS_MAX_CALL_DEPTH_LIMIT 10000
int JS_SetMaxCallDepth(JSRuntime *rt, int depth);
int JS_GetMaxCallDepth(JSRuntime *rt);
/* The recursions of C that make no call (the parsers, JSON, modules, proxies
   without a trap and the like) have a depth of their own, separate from the
   call depth: at most this many levels at once. Past it a script gets a
   RangeError (a SyntaxError from a regexp). Unlike the call depth it is a
   constant of the build, not a setting: the proof of the bound on the stack the
   engine may use (docs/changes/c-engine.md) multiplies it by the stack
   of the deepest recursion, so it needs one fixed biggest value. The parser
   counts 3 levels for each nested parenthesis, call or array. Of 79525 files
   of real code (npm packages, compiled JSX, the output of code generators)
   the deepest parse reaches 632, in an asm.js build of sql.js, no other goes
   past 282, and one in a thousand reaches 100: 2000 is more than three times
   the deepest. */
#define JS_MAX_INTERNAL_DEPTH 2000
/* The depths now: how many calls run (the call depth), and how many levels the
   recursions of C that make no call are in (the internal depth). They are for
   a host whose code in one runtime calls into another, which counts its own
   depths: to hold the runtimes together to one limit, the host reads the
   depths of the runtime that calls, sets those of the other to them, makes the
   call, and sets the old depths back. The Get functions return a depth. The Set
   functions return 0, or -1 and change nothing when depth is below 0 or above
   JS_MAX_CALL_DEPTH_LIMIT (JS_MAX_INTERNAL_DEPTH for the internal depth). A
   depth above the limit of the runtime is accepted: nothing more runs in it
   until the host sets the depth back, and the next call is a RangeError. They
   work while the runtime runs code, which is when they are needed, and then
   every depth set must be set back before the code that was running goes on:
   a call that ends counts one level off whatever the depth is by then. */
int JS_GetCallDepth(JSRuntime *rt);
int JS_SetCallDepth(JSRuntime *rt, int depth);
int JS_GetInternalDepth(JSRuntime *rt);
int JS_SetInternalDepth(JSRuntime *rt, int depth);
/* Returns the number of blocks of the allocator still in use, which is 0 when
   nothing leaked. The host takes the chunks of memory back after it. An
   object a reference is still held to, such as one the host never freed,
   stops it before it frees the rest: it returns the blocks in use then. */
size_t JS_FreeRuntime(JSRuntime *rt);
void *JS_GetRuntimeOpaque(JSRuntime *rt);
void JS_SetRuntimeOpaque(JSRuntime *rt, void *opaque);
typedef void JS_MarkFunc(JSRuntime *rt, JSGCObjectHeader *gp);
void JS_MarkValue(JSRuntime *rt, JSValueConst val, JS_MarkFunc *mark_func);
void JS_RunGC(JSRuntime *rt);
void JS_RunYoungGC(JSRuntime *rt);
JS_BOOL JS_IsLiveObject(JSRuntime *rt, JSValueConst obj);

JSContext *JS_NewContext(JSRuntime *rt);
void JS_FreeContext(JSContext *s);
JSContext *JS_DupContext(JSContext *ctx);
void *JS_GetContextOpaque(JSContext *ctx);
void JS_SetContextOpaque(JSContext *ctx, void *opaque);
JSRuntime *JS_GetRuntime(JSContext *ctx);
void JS_SetClassProto(JSContext *ctx, JSClassID class_id, JSValue obj);
JSValue JS_GetClassProto(JSContext *ctx, JSClassID class_id);

/* the following functions are used to select the intrinsic object to
   save memory */
JSContext *JS_NewContextRaw(JSRuntime *rt);
int JS_AddIntrinsicBaseObjects(JSContext *ctx);
int JS_AddIntrinsicDate(JSContext *ctx);
int JS_AddIntrinsicEval(JSContext *ctx);
int JS_AddIntrinsicStringNormalize(JSContext *ctx);
void JS_AddIntrinsicRegExpCompiler(JSContext *ctx);
int JS_AddIntrinsicRegExp(JSContext *ctx);
int JS_AddIntrinsicJSON(JSContext *ctx);
int JS_AddIntrinsicProxy(JSContext *ctx);
int JS_AddIntrinsicMapSet(JSContext *ctx);
int JS_AddIntrinsicTypedArrays(JSContext *ctx);
int JS_AddIntrinsicPromise(JSContext *ctx);
int JS_AddIntrinsicWeakRef(JSContext *ctx);

JSValue js_string_codePointRange(JSContext *ctx, JSValueConst this_val,
                                 int argc, JSValueConst *argv);

void *js_malloc_rt(JSRuntime *rt, size_t size);
void js_free_rt(JSRuntime *rt, void *ptr);
void *js_realloc_rt(JSRuntime *rt, void *ptr, size_t size);
size_t js_malloc_usable_size_rt(JSRuntime *rt, const void *ptr);
void *js_mallocz_rt(JSRuntime *rt, size_t size);

void *js_malloc(JSContext *ctx, size_t size);
void js_free(JSContext *ctx, void *ptr);
void *js_realloc(JSContext *ctx, void *ptr, size_t size);
size_t js_malloc_usable_size(JSContext *ctx, const void *ptr);
void *js_realloc2(JSContext *ctx, void *ptr, size_t size, size_t *pslack);
void *js_mallocz(JSContext *ctx, size_t size);
char *js_strdup(JSContext *ctx, const char *str);
char *js_strndup(JSContext *ctx, const char *s, size_t n);

typedef struct JSMemoryUsage {
    int64_t malloc_size, malloc_limit, memory_used_size;
    int64_t malloc_count;
    int64_t memory_used_count;
    int64_t atom_count, atom_size;
    int64_t str_count, str_size;
    int64_t obj_count, obj_size;
    int64_t prop_count, prop_size;
    int64_t shape_count, shape_size;
    int64_t js_func_count, js_func_size, js_func_code_size;
    int64_t js_func_pc2line_count, js_func_pc2line_size;
    int64_t c_func_count, array_count;
    int64_t fast_array_count, fast_array_elements;
    int64_t binary_object_count, binary_object_size;
    /* what the runtime took from the host, its limit (0 for none), the
       chunks, and the bytes offered back so far */
    int64_t footprint, footprint_limit, chunk_count, release_candidate;
} JSMemoryUsage;

void JS_ComputeMemoryUsage(JSRuntime *rt, JSMemoryUsage *s);
/* write a report of s to buf, cut to size bytes, and return its whole length */
size_t JS_DumpMemoryUsage(char *buf, size_t size, const JSMemoryUsage *s, JSRuntime *rt);
#ifdef JS_HEAP_CHECK
/* test builds: NULL if the allocator is consistent, else a message */
const char *JS_CheckMemory(JSRuntime *rt);
/* test builds: how many freed blocks the front cache holds */
size_t JS_FrontCacheBlocks(JSRuntime *rt);
/* test builds: how many freed large blocks the cache of large blocks holds,
   and the sum of the sizes the heap gave them (JS_LARGE_CACHE_BYTES at most) */
size_t JS_LargeCacheBlocks(JSRuntime *rt);
size_t JS_LargeCacheBytes(JSRuntime *rt);
/* test builds: how many requests the cache of large blocks has served */
size_t JS_LargeCacheHits(JSRuntime *rt);
/* test builds: the most blocks a class of the large cache keeps, and the most
   bytes the whole cache keeps */
void JS_LargeCacheLimits(size_t *per_class, size_t *bytes);
#endif

/* atom support */
#define JS_ATOM_NULL 0

JSAtom JS_NewAtomLen(JSContext *ctx, const char *str, size_t len);
JSAtom JS_NewAtom(JSContext *ctx, const char *str);
JSAtom JS_NewAtomUInt32(JSContext *ctx, uint32_t n);
JSAtom JS_DupAtom(JSContext *ctx, JSAtom v);
void JS_FreeAtom(JSContext *ctx, JSAtom v);
void JS_FreeAtomRT(JSRuntime *rt, JSAtom v);
JSValue JS_AtomToValue(JSContext *ctx, JSAtom atom);
JSValue JS_AtomToString(JSContext *ctx, JSAtom atom);
const char *JS_AtomToCStringLen(JSContext *ctx, size_t *plen, JSAtom atom);
static inline const char *JS_AtomToCString(JSContext *ctx, JSAtom atom)
{
    return JS_AtomToCStringLen(ctx, NULL, atom);
}
JSAtom JS_ValueToAtom(JSContext *ctx, JSValueConst val);

/* object class support */

typedef struct JSPropertyEnum {
    JS_BOOL is_enumerable;
    JSAtom atom;
} JSPropertyEnum;

typedef struct JSPropertyDescriptor {
    int flags;
    JSValue value;
    JSValue getter;
    JSValue setter;
} JSPropertyDescriptor;

typedef struct JSClassExoticMethods {
    /* Return -1 if exception (can only happen in case of Proxy object),
       FALSE if the property does not exists, TRUE if it exists. If 1 is
       returned, the property descriptor 'desc' is filled if != NULL. */
    int (*get_own_property)(JSContext *ctx, JSPropertyDescriptor *desc,
                             JSValueConst obj, JSAtom prop);
    /* '*ptab' should hold the '*plen' property keys. Return 0 if OK,
       -1 if exception. The 'is_enumerable' field is ignored.
    */
    int (*get_own_property_names)(JSContext *ctx, JSPropertyEnum **ptab,
                                  uint32_t *plen,
                                  JSValueConst obj);
    /* return < 0 if exception, or TRUE/FALSE */
    int (*delete_property)(JSContext *ctx, JSValueConst obj, JSAtom prop);
    /* return < 0 if exception or TRUE/FALSE */
    int (*define_own_property)(JSContext *ctx, JSValueConst this_obj,
                               JSAtom prop, JSValueConst val,
                               JSValueConst getter, JSValueConst setter,
                               int flags);
    /* The following methods can be emulated with the previous ones,
       so they are usually not needed */
    /* return < 0 if exception or TRUE/FALSE */
    int (*has_property)(JSContext *ctx, JSValueConst obj, JSAtom atom);
    JSValue (*get_property)(JSContext *ctx, JSValueConst obj, JSAtom atom,
                            JSValueConst receiver);
    /* return < 0 if exception or TRUE/FALSE */
    int (*set_property)(JSContext *ctx, JSValueConst obj, JSAtom atom,
                        JSValueConst value, JSValueConst receiver, int flags);

    /* There is no get_prototype: a proxy is the one object whose prototype
       is not that of its shape, and JS_GetPrototype calls it by name. */
    /* return < 0 if exception or TRUE/FALSE */
    int (*set_prototype)(JSContext *ctx, JSValueConst obj, JSValueConst proto_val);
    /* return < 0 if exception or TRUE/FALSE */
    int (*is_extensible)(JSContext *ctx, JSValueConst obj);
    /* return < 0 if exception or TRUE/FALSE */
    int (*prevent_extensions)(JSContext *ctx, JSValueConst obj);
} JSClassExoticMethods;

typedef void JSClassFinalizer(JSRuntime *rt, JSValue val);
typedef void JSClassGCMark(JSRuntime *rt, JSValueConst val,
                           JS_MarkFunc *mark_func);
#define JS_CALL_FLAG_CONSTRUCTOR (1 << 0)
typedef JSValue JSClassCall(JSContext *ctx, JSValueConst func_obj,
                            JSValueConst this_val, int argc, JSValueConst *argv,
                            int flags);

typedef struct JSClassDef {
    const char *class_name;
    JSClassFinalizer *finalizer;
    JSClassGCMark *gc_mark;
    /* if call != NULL, the object is a function. If (flags &
       JS_CALL_FLAG_CONSTRUCTOR) != 0, the function is called as a
       constructor. In this case, 'this_val' is new.target. A
       constructor call only happens if the object constructor bit is
       set (see JS_SetConstructorBit()). */
    JSClassCall *call;
    /* XXX: suppress this indirection ? It is here only to save memory
       because only a few classes need these methods */
    JSClassExoticMethods *exotic;
} JSClassDef;

#define JS_INVALID_CLASS_ID 0
/* Gives a class ID of rt, the next one it has not given out, and keeps it in
   *pclass_id when that is 0. The IDs are counted by each runtime on its own, so
   two runtimes give the same numbers to different classes, and the engine
   keeps no state outside its runtimes. Keep *pclass_id per runtime: one global
   variable shared by two runtimes holds the ID of the first and is a different
   class in the second. A nonzero *pclass_id is returned as it is, and rt does
   not give that number out again. The numbers of the built-in classes are
   never given out. */
JSClassID JS_NewClassID(JSRuntime *rt, JSClassID *pclass_id);
/* Returns the class ID if `v` is an object, otherwise returns JS_INVALID_CLASS_ID. */
JSClassID JS_GetClassID(JSValue v);
int JS_NewClass(JSRuntime *rt, JSClassID class_id, const JSClassDef *class_def);
int JS_IsRegisteredClass(JSRuntime *rt, JSClassID class_id);

/* value handling */

static js_force_inline JSValue JS_NewBool(JSContext *ctx, JS_BOOL val)
{
    return JS_MKVAL(JS_TAG_BOOL, (val != 0));
}

static js_force_inline JSValue JS_NewInt32(JSContext *ctx, int32_t val)
{
    return JS_MKVAL(JS_TAG_INT, val);
}

static js_force_inline JSValue JS_NewCatchOffset(JSContext *ctx, int32_t val)
{
    return JS_MKVAL(JS_TAG_CATCH_OFFSET, val);
}

static js_force_inline JSValue JS_NewInt64(JSContext *ctx, int64_t val)
{
    JSValue v;
    if (val == (int32_t)val) {
        v = JS_NewInt32(ctx, val);
    } else {
        v = __JS_NewFloat64(ctx, val);
    }
    return v;
}

static js_force_inline JSValue JS_NewUint32(JSContext *ctx, uint32_t val)
{
    JSValue v;
    if (val <= 0x7fffffff) {
        v = JS_NewInt32(ctx, val);
    } else {
        v = __JS_NewFloat64(ctx, val);
    }
    return v;
}

JSValue JS_NewBigInt64(JSContext *ctx, int64_t v);
JSValue JS_NewBigUint64(JSContext *ctx, uint64_t v);

static js_force_inline JSValue JS_NewFloat64(JSContext *ctx, double d)
{
    int32_t val;
    union {
        double d;
        uint64_t u;
    } u, t;
    if (d >= INT32_MIN && d <= INT32_MAX) {
        u.d = d;
        val = (int32_t)d;
        t.d = val;
        /* -0 cannot be represented as integer, so we compare the bit
           representation */
        if (u.u == t.u)
            return JS_MKVAL(JS_TAG_INT, val);
    }
    return __JS_NewFloat64(ctx, d);
}

static inline JS_BOOL JS_IsNumber(JSValueConst v)
{
    int tag = JS_VALUE_GET_TAG(v);
    return tag == JS_TAG_INT || JS_TAG_IS_FLOAT64(tag);
}

static inline JS_BOOL JS_IsBigInt(JSContext *ctx, JSValueConst v)
{
    int tag = JS_VALUE_GET_TAG(v);
    return tag == JS_TAG_BIG_INT || tag == JS_TAG_SHORT_BIG_INT;
}

static inline JS_BOOL JS_IsBool(JSValueConst v)
{
    return JS_VALUE_GET_TAG(v) == JS_TAG_BOOL;
}

static inline JS_BOOL JS_IsNull(JSValueConst v)
{
    return JS_VALUE_GET_TAG(v) == JS_TAG_NULL;
}

static inline JS_BOOL JS_IsUndefined(JSValueConst v)
{
    return JS_VALUE_GET_TAG(v) == JS_TAG_UNDEFINED;
}

static inline JS_BOOL JS_IsException(JSValueConst v)
{
    return js_unlikely(JS_VALUE_GET_TAG(v) == JS_TAG_EXCEPTION);
}

static inline JS_BOOL JS_IsUninitialized(JSValueConst v)
{
    return js_unlikely(JS_VALUE_GET_TAG(v) == JS_TAG_UNINITIALIZED);
}

static inline JS_BOOL JS_IsString(JSValueConst v)
{
    return JS_VALUE_GET_TAG(v) == JS_TAG_STRING ||
        JS_VALUE_GET_TAG(v) == JS_TAG_STRING_ROPE;
}

static inline JS_BOOL JS_IsSymbol(JSValueConst v)
{
    return JS_VALUE_GET_TAG(v) == JS_TAG_SYMBOL;
}

static inline JS_BOOL JS_IsObject(JSValueConst v)
{
    return JS_VALUE_GET_TAG(v) == JS_TAG_OBJECT;
}

JSValue JS_Throw(JSContext *ctx, JSValue obj);
void JS_SetUncatchableException(JSContext *ctx, JS_BOOL flag);
JS_BOOL JS_IsUncatchableException(JSContext *ctx);
JSValue JS_GetException(JSContext *ctx);
JS_BOOL JS_HasException(JSContext *ctx);
JS_BOOL JS_IsError(JSContext *ctx, JSValueConst val);
JSValue JS_NewError(JSContext *ctx);
JSValue __js_printf_like(2, 3) JS_ThrowSyntaxError(JSContext *ctx, const char *fmt, ...);
JSValue __js_printf_like(2, 3) JS_ThrowTypeError(JSContext *ctx, const char *fmt, ...);
JSValue __js_printf_like(2, 3) JS_ThrowReferenceError(JSContext *ctx, const char *fmt, ...);
JSValue __js_printf_like(2, 3) JS_ThrowRangeError(JSContext *ctx, const char *fmt, ...);
JSValue __js_printf_like(2, 3) JS_ThrowInternalError(JSContext *ctx, const char *fmt, ...);
JSValue JS_ThrowOutOfMemory(JSContext *ctx);
/* The kinds of JS_ThrowErrorKind, one per native error constructor. */
typedef enum JSErrorKind {
    JS_ERROR_KIND_EVAL,
    JS_ERROR_KIND_RANGE,
    JS_ERROR_KIND_REFERENCE,
    JS_ERROR_KIND_SYNTAX,
    JS_ERROR_KIND_TYPE,
    JS_ERROR_KIND_URI,
    JS_ERROR_KIND_INTERNAL,
    JS_ERROR_KIND_AGGREGATE,
} JSErrorKind;
/* Throws a new error of kind whose message is the len bytes at msg, as
   JS_ThrowTypeError(ctx, "%s", msg) and the like do: the message stops at a
   NUL byte and at 255 bytes. It takes no variable arguments, so a host that
   cannot pass them calls it. A kind that is not a JSErrorKind throws an
   InternalError instead. */
JSValue JS_ThrowErrorKind(JSContext *ctx, int kind, const char *msg, size_t len);

void __JS_FreeValue(JSContext *ctx, JSValue v);

static inline JSRefCountHeader *__js_rc(void *ptr)
{
    return (JSRefCountHeader *)((uint32_t *)ptr - 1);
}

static inline void JS_FreeValue(JSContext *ctx, JSValue v)
{
    if (JS_VALUE_HAS_REF_COUNT(v)) {
        JSRefCountHeader *p = __js_rc(JS_VALUE_GET_PTR(v));
        if (--p->ref_count <= 0) {
            __JS_FreeValue(ctx, v);
        }
    }
}
void __JS_FreeValueRT(JSRuntime *rt, JSValue v);
static inline void JS_FreeValueRT(JSRuntime *rt, JSValue v)
{
    if (JS_VALUE_HAS_REF_COUNT(v)) {
        JSRefCountHeader *p = __js_rc(JS_VALUE_GET_PTR(v));
        if (--p->ref_count <= 0) {
            __JS_FreeValueRT(rt, v);
        }
    }
}

static inline JSValue JS_DupValue(JSContext *ctx, JSValueConst v)
{
    if (JS_VALUE_HAS_REF_COUNT(v)) {
        JSRefCountHeader *p = __js_rc(JS_VALUE_GET_PTR(v));
        p->ref_count++;
    }
    return (JSValue)v;
}

static inline JSValue JS_DupValueRT(JSRuntime *rt, JSValueConst v)
{
    if (JS_VALUE_HAS_REF_COUNT(v)) {
        JSRefCountHeader *p = __js_rc(JS_VALUE_GET_PTR(v));
        p->ref_count++;
    }
    return (JSValue)v;
}

JS_BOOL JS_StrictEq(JSContext *ctx, JSValueConst op1, JSValueConst op2);
JS_BOOL JS_SameValue(JSContext *ctx, JSValueConst op1, JSValueConst op2);
JS_BOOL JS_SameValueZero(JSContext *ctx, JSValueConst op1, JSValueConst op2);

int JS_ToBool(JSContext *ctx, JSValueConst val); /* return -1 for JS_EXCEPTION */
int JS_ToInt32(JSContext *ctx, int32_t *pres, JSValueConst val);
static inline int JS_ToUint32(JSContext *ctx, uint32_t *pres, JSValueConst val)
{
    return JS_ToInt32(ctx, (int32_t*)pres, val);
}
int JS_ToInt64(JSContext *ctx, int64_t *pres, JSValueConst val);
int JS_ToIndex(JSContext *ctx, uint64_t *plen, JSValueConst val);
int JS_ToFloat64(JSContext *ctx, double *pres, JSValueConst val);
/* return an exception if 'val' is a Number */
int JS_ToBigInt64(JSContext *ctx, int64_t *pres, JSValueConst val);
/* same as JS_ToInt64() but allow BigInt */
int JS_ToInt64Ext(JSContext *ctx, int64_t *pres, JSValueConst val);

JSValue JS_NewStringLen(JSContext *ctx, const char *str1, size_t len1);
/* What a host reads and writes in place, by index into js_host_layout, a
   table of int64_t that quickjs.c defines. Nothing in the engine reads the
   table: internal/goasm/gen copies it into Go constants (engine_decl.go). */
enum {
    /* where the int32 reference count of a value with one is, from the
       address the value holds */
    JS_HOST_LAYOUT_REFCOUNT_OFFSET,
    /* the first uint32 of a string (JS_TAG_STRING) holds its length under
       this mask, and this bit when its characters are 16-bit units */
    JS_HOST_LAYOUT_STRING_LEN_MASK,
    JS_HOST_LAYOUT_STRING_WIDE_BIT,
    /* where the characters of a string start, from the address the value holds */
    JS_HOST_LAYOUT_STRING_CHARS,
    /* the most characters a string holds */
    JS_HOST_LAYOUT_STRING_LEN_MAX,
    /* the numbers of the built-in atoms a host names */
    JS_HOST_LAYOUT_ATOM_EMPTY_STRING,
    JS_HOST_LAYOUT_ATOM_FILE_NAME,
    JS_HOST_LAYOUT_ATOM_LINE_NUMBER,
    JS_HOST_LAYOUT_ATOM_COLUMN_NUMBER,
    JS_HOST_LAYOUT_ATOM_MESSAGE,
    JS_HOST_LAYOUT_ATOM_STACK,
    JS_HOST_LAYOUT_ATOM_NAME,
    JS_HOST_LAYOUT_ATOM_TO_STRING,
    JS_HOST_LAYOUT_ATOM_THEN,
    JS_HOST_LAYOUT_COUNT,
};
/* Returns a new string of len characters, 16-bit units when wide is set and
   bytes (Latin-1) when not, whose characters are not written yet: the host
   writes all of them in place before the string is used (JS_HOST_LAYOUT_*
   says where they are). An 8-bit string has its NUL after them. len 0 gives
   the empty string, len above JS_HOST_LAYOUT_STRING_LEN_MAX throws a
   RangeError, and no memory throws the out of memory error. */
JSValue JS_NewStringChars(JSContext *ctx, uint32_t len, int wide);
static inline JSValue JS_NewString(JSContext *ctx, const char *str)
{
    return JS_NewStringLen(ctx, str, strlen(str));
}
JSValue JS_NewAtomString(JSContext *ctx, const char *str);
JSValue JS_ToString(JSContext *ctx, JSValueConst val);
JSValue JS_ToPropertyKey(JSContext *ctx, JSValueConst val);
const char *JS_ToCStringLen2(JSContext *ctx, size_t *plen, JSValueConst val1, JS_BOOL cesu8);
static inline const char *JS_ToCStringLen(JSContext *ctx, size_t *plen, JSValueConst val1)
{
    return JS_ToCStringLen2(ctx, plen, val1, 0);
}
static inline const char *JS_ToCString(JSContext *ctx, JSValueConst val1)
{
    return JS_ToCStringLen2(ctx, NULL, val1, 0);
}
void JS_FreeCString(JSContext *ctx, const char *ptr);

JSValue JS_NewObjectProtoClass(JSContext *ctx, JSValueConst proto, JSClassID class_id);
JSValue JS_NewObjectClass(JSContext *ctx, int class_id);
JSValue JS_NewObjectProto(JSContext *ctx, JSValueConst proto);
JSValue JS_NewObject(JSContext *ctx);

JS_BOOL JS_IsFunction(JSContext* ctx, JSValueConst val);
JS_BOOL JS_IsConstructor(JSContext* ctx, JSValueConst val);
JS_BOOL JS_SetConstructorBit(JSContext *ctx, JSValueConst func_obj, JS_BOOL val);

JSValue JS_NewArray(JSContext *ctx);
int JS_IsArray(JSContext *ctx, JSValueConst val);

JSValue JS_NewDate(JSContext *ctx, double epoch_ms);

JSValue JS_GetPropertyInternal(JSContext *ctx, JSValueConst obj,
                               JSAtom prop, JSValueConst receiver,
                               JS_BOOL throw_ref_error);
static js_force_inline JSValue JS_GetProperty(JSContext *ctx, JSValueConst this_obj,
                                              JSAtom prop)
{
    return JS_GetPropertyInternal(ctx, this_obj, prop, this_obj, 0);
}
JSValue JS_GetPropertyStr(JSContext *ctx, JSValueConst this_obj,
                          const char *prop);
JSValue JS_GetPropertyUint32(JSContext *ctx, JSValueConst this_obj,
                             uint32_t idx);

int JS_SetPropertyInternal(JSContext *ctx, JSValueConst obj,
                           JSAtom prop, JSValue val, JSValueConst this_obj,
                           int flags);
static inline int JS_SetProperty(JSContext *ctx, JSValueConst this_obj,
                                 JSAtom prop, JSValue val)
{
    return JS_SetPropertyInternal(ctx, this_obj, prop, val, this_obj, JS_PROP_THROW);
}
int JS_SetPropertyUint32(JSContext *ctx, JSValueConst this_obj,
                         uint32_t idx, JSValue val);
int JS_SetPropertyInt64(JSContext *ctx, JSValueConst this_obj,
                        int64_t idx, JSValue val);
int JS_SetPropertyStr(JSContext *ctx, JSValueConst this_obj,
                      const char *prop, JSValue val);
int JS_HasProperty(JSContext *ctx, JSValueConst this_obj, JSAtom prop);
int JS_IsExtensible(JSContext *ctx, JSValueConst obj);
int JS_PreventExtensions(JSContext *ctx, JSValueConst obj);
int JS_DeleteProperty(JSContext *ctx, JSValueConst obj, JSAtom prop, int flags);
int JS_SetPrototype(JSContext *ctx, JSValueConst obj, JSValueConst proto_val);
JSValue JS_GetPrototype(JSContext *ctx, JSValueConst val);

#define JS_GPN_STRING_MASK  (1 << 0)
#define JS_GPN_SYMBOL_MASK  (1 << 1)
#define JS_GPN_PRIVATE_MASK (1 << 2)
/* only include the enumerable properties */
#define JS_GPN_ENUM_ONLY    (1 << 4)
/* set theJSPropertyEnum.is_enumerable field */
#define JS_GPN_SET_ENUM     (1 << 5)

int JS_GetOwnPropertyNames(JSContext *ctx, JSPropertyEnum **ptab,
                           uint32_t *plen, JSValueConst obj, int flags);
void JS_FreePropertyEnum(JSContext *ctx, JSPropertyEnum *tab,
                         uint32_t len);
int JS_GetOwnProperty(JSContext *ctx, JSPropertyDescriptor *desc,
                      JSValueConst obj, JSAtom prop);

JSValue JS_Call(JSContext *ctx, JSValueConst func_obj, JSValueConst this_obj,
                int argc, JSValueConst *argv);
JSValue JS_Invoke(JSContext *ctx, JSValueConst this_val, JSAtom atom,
                  int argc, JSValueConst *argv);
JSValue JS_CallConstructor(JSContext *ctx, JSValueConst func_obj,
                           int argc, JSValueConst *argv);
JSValue JS_CallConstructor2(JSContext *ctx, JSValueConst func_obj,
                            JSValueConst new_target,
                            int argc, JSValueConst *argv);
JS_BOOL JS_DetectModule(const char *input, size_t input_len);
/* 'input' must be zero terminated i.e. input[input_len] = '\0'. */
JSValue JS_Eval(JSContext *ctx, const char *input, size_t input_len,
                const char *filename, int eval_flags);
/* same as JS_Eval() but with an explicit 'this_obj' parameter */
JSValue JS_EvalThis(JSContext *ctx, JSValueConst this_obj,
                    const char *input, size_t input_len,
                    const char *filename, int eval_flags);
JSValue JS_GetGlobalObject(JSContext *ctx);
int JS_IsInstanceOf(JSContext *ctx, JSValueConst val, JSValueConst obj);
int JS_DefineProperty(JSContext *ctx, JSValueConst this_obj,
                      JSAtom prop, JSValueConst val,
                      JSValueConst getter, JSValueConst setter, int flags);
int JS_DefinePropertyValue(JSContext *ctx, JSValueConst this_obj,
                           JSAtom prop, JSValue val, int flags);
int JS_DefinePropertyValueUint32(JSContext *ctx, JSValueConst this_obj,
                                 uint32_t idx, JSValue val, int flags);
int JS_DefinePropertyValueStr(JSContext *ctx, JSValueConst this_obj,
                              const char *prop, JSValue val, int flags);
int JS_DefinePropertyGetSet(JSContext *ctx, JSValueConst this_obj,
                            JSAtom prop, JSValue getter, JSValue setter,
                            int flags);
void JS_SetOpaque(JSValue obj, void *opaque);
void *JS_GetOpaque(JSValueConst obj, JSClassID class_id);
void *JS_GetOpaque2(JSContext *ctx, JSValueConst obj, JSClassID class_id);
void *JS_GetAnyOpaque(JSValueConst obj, JSClassID *class_id);

/* 'buf' must be zero terminated i.e. buf[buf_len] = '\0'. */
JSValue JS_ParseJSON(JSContext *ctx, const char *buf, size_t buf_len,
                     const char *filename);
#define JS_PARSE_JSON_EXT (1 << 0) /* allow extended JSON */
JSValue JS_ParseJSON2(JSContext *ctx, const char *buf, size_t buf_len,
                      const char *filename, int flags);
JSValue JS_JSONStringify(JSContext *ctx, JSValueConst obj,
                         JSValueConst replacer, JSValueConst space0);

typedef void JSFreeArrayBufferDataFunc(JSRuntime *rt, void *opaque, void *ptr);
JSValue JS_NewArrayBuffer(JSContext *ctx, uint8_t *buf, size_t len,
                          JSFreeArrayBufferDataFunc *free_func, void *opaque,
                          JS_BOOL is_shared);
JSValue JS_NewArrayBufferCopy(JSContext *ctx, const uint8_t *buf, size_t len);
void JS_DetachArrayBuffer(JSContext *ctx, JSValueConst obj);
uint8_t *JS_GetArrayBuffer(JSContext *ctx, size_t *psize, JSValueConst obj);

typedef enum JSTypedArrayEnum {
    JS_TYPED_ARRAY_UINT8C = 0,
    JS_TYPED_ARRAY_INT8,
    JS_TYPED_ARRAY_UINT8,
    JS_TYPED_ARRAY_INT16,
    JS_TYPED_ARRAY_UINT16,
    JS_TYPED_ARRAY_INT32,
    JS_TYPED_ARRAY_UINT32,
    JS_TYPED_ARRAY_BIG_INT64,
    JS_TYPED_ARRAY_BIG_UINT64,
    JS_TYPED_ARRAY_FLOAT16,
    JS_TYPED_ARRAY_FLOAT32,
    JS_TYPED_ARRAY_FLOAT64,
} JSTypedArrayEnum;

JSValue JS_NewTypedArray(JSContext *ctx, int argc, JSValueConst *argv,
                         JSTypedArrayEnum array_type);
JSValue JS_GetTypedArrayBuffer(JSContext *ctx, JSValueConst obj,
                               size_t *pbyte_offset,
                               size_t *pbyte_length,
                               size_t *pbytes_per_element);
typedef struct {
    void *(*sab_alloc)(void *opaque, size_t size);
    void (*sab_free)(void *opaque, void *ptr);
    void (*sab_dup)(void *opaque, void *ptr);
    void *sab_opaque;
} JSSharedArrayBufferFunctions;
void JS_SetSharedArrayBufferFunctions(JSRuntime *rt,
                                      const JSSharedArrayBufferFunctions *sf);

typedef enum JSPromiseStateEnum {
    JS_PROMISE_PENDING,
    JS_PROMISE_FULFILLED,
    JS_PROMISE_REJECTED,
} JSPromiseStateEnum;

JSValue JS_NewPromiseCapability(JSContext *ctx, JSValue *resolving_funcs);
JSPromiseStateEnum JS_PromiseState(JSContext *ctx, JSValue promise);
JSValue JS_PromiseResult(JSContext *ctx, JSValue promise);

/* is_handled = TRUE means that the rejection is handled */
typedef void JSHostPromiseRejectionTracker(JSContext *ctx, JSValueConst promise,
                                           JSValueConst reason,
                                           JS_BOOL is_handled, void *opaque);
void JS_SetHostPromiseRejectionTracker(JSRuntime *rt, JSHostPromiseRejectionTracker *cb, void *opaque);
/* the async context of promise jobs: an object that promise reactions keep
   and run with, or none (undefined) */
JSValue JS_GetAsyncContext(JSContext *ctx);
void JS_SetAsyncContext(JSContext *ctx, JSValueConst v);
/* the regexp cache of a runtime keeps compiled regexps by pattern and flags:
   at most n entries in at most bytes bytes, 0 for none. A new runtime keeps
   JS_REGEXP_CACHE_ENTRIES in JS_REGEXP_CACHE_BYTES. */
#define JS_REGEXP_CACHE_ENTRIES 256
#define JS_REGEXP_CACHE_BYTES (1 << 20)
void JS_SetRegExpCache(JSRuntime *rt, int n, size_t bytes);
/* the shape cache of the runtime of ctx keeps the last n shapes of plain
   objects of ctx after their objects are gone, 0 for none, the default.
   Return -1 when there is no memory for it. Turn it off before freeing ctx
   while the runtime lives on. */
int JS_SetShapeCache(JSContext *ctx, int n);

/* The interrupt flag of rt. The host sets it to a value other than 0 to
   stop the script, from the thread that runs it (in a poll or a callback):
   the engine reads it with plain loads. It reads it where it can throw, in the
   interpreter (at calls and backward jumps), in regexps, in long string
   searches, in the long loops of typed arrays, repeat and JSON, and throws
   the error "interrupted", which no try catches. The engine never clears
   it: the host does, before it runs code in rt again. */
volatile int *JS_GetInterruptFlag(JSRuntime *rt);
/* select which debug info is stripped from the compiled code */
#define JS_STRIP_SOURCE (1 << 0) /* strip source code */
#define JS_STRIP_DEBUG  (1 << 1) /* strip all debug info including source code */
void JS_SetStripInfo(JSRuntime *rt, int flags);
int JS_GetStripInfo(JSRuntime *rt);

/* set the [IsHTMLDDA] internal slot */
void JS_SetIsHTMLDDA(JSContext *ctx, JSValueConst obj);

typedef struct JSModuleDef JSModuleDef;

/* return the module specifier (allocated with js_malloc()) or NULL if
   exception */
typedef char *JSModuleNormalizeFunc(JSContext *ctx,
                                    const char *module_base_name,
                                    const char *module_name, void *opaque);
typedef JSModuleDef *JSModuleLoaderFunc(JSContext *ctx,
                                        const char *module_name, void *opaque);
typedef JSModuleDef *JSModuleLoaderFunc2(JSContext *ctx,
                                         const char *module_name, void *opaque,
                                         JSValueConst attributes);
/* return -1 if exception, 0 if OK */
typedef int JSModuleCheckSupportedImportAttributes(JSContext *ctx, void *opaque,
                                                   JSValueConst attributes);
                                                   
/* module_normalize = NULL is allowed and invokes the default module
   filename normalizer */
void JS_SetModuleLoaderFunc(JSRuntime *rt,
                            JSModuleNormalizeFunc *module_normalize,
                            JSModuleLoaderFunc *module_loader, void *opaque);
/* same as JS_SetModuleLoaderFunc but with attributes. if
   module_check_attrs = NULL, no attribute checking is done. */
void JS_SetModuleLoaderFunc2(JSRuntime *rt,
                             JSModuleNormalizeFunc *module_normalize,
                             JSModuleLoaderFunc2 *module_loader,
                             JSModuleCheckSupportedImportAttributes *module_check_attrs,
                             void *opaque);
/* return the import.meta object of a module */
JSValue JS_GetImportMeta(JSContext *ctx, JSModuleDef *m);
JSAtom JS_GetModuleName(JSContext *ctx, JSModuleDef *m);
JSValue JS_GetModuleNamespace(JSContext *ctx, JSModuleDef *m);
/* Frees the function of the top level of m once m has run to its end without
   an exception, with the code of the top level and the variables that neither
   an export nor a function of m holds. It does nothing to a module that has
   not run, still runs (a top-level await) or threw. */
void JS_FreeModuleTopLevel(JSContext *ctx, JSModuleDef *m);

/* JS Job support */

typedef JSValue JSJobFunc(JSContext *ctx, int argc, JSValueConst *argv);
int JS_EnqueueJob(JSContext *ctx, JSJobFunc *job_func, int argc, JSValueConst *argv);

JS_BOOL JS_IsJobPending(JSRuntime *rt);
int JS_ExecutePendingJob(JSRuntime *rt, JSContext **pctx);

/* Object Writer/Reader (currently only used to handle precompiled code) */
#define JS_WRITE_OBJ_BYTECODE  (1 << 0) /* allow function/module */
#define JS_WRITE_OBJ_BSWAP     (1 << 1) /* byte swapped output */
#define JS_WRITE_OBJ_SAB       (1 << 2) /* allow SharedArrayBuffer */
#define JS_WRITE_OBJ_REFERENCE (1 << 3) /* allow object references to
                                           encode arbitrary object
                                           graph */
uint8_t *JS_WriteObject(JSContext *ctx, size_t *psize, JSValueConst obj,
                        int flags);
uint8_t *JS_WriteObject2(JSContext *ctx, size_t *psize, JSValueConst obj,
                         int flags, uint8_t ***psab_tab, size_t *psab_tab_len);

#define JS_READ_OBJ_BYTECODE  (1 << 0) /* allow function/module */
#define JS_READ_OBJ_ROM_DATA  (1 << 1) /* avoid duplicating 'buf' data */
#define JS_READ_OBJ_SAB       (1 << 2) /* allow SharedArrayBuffer */
#define JS_READ_OBJ_REFERENCE (1 << 3) /* allow object references */
/* the source text and line table of each function point into 'buf',
   which must stay unchanged until the runtime is freed */
#define JS_READ_OBJ_SHARED_DEBUG (1 << 4)
JSValue JS_ReadObject(JSContext *ctx, const uint8_t *buf, size_t buf_len,
                      int flags);
/* instantiate and evaluate a bytecode function. Only used when
   reading a script or module with JS_ReadObject() */
JSValue JS_EvalFunction(JSContext *ctx, JSValue fun_obj);
/* load the dependencies of the module 'obj'. Useful when JS_ReadObject()
   returns a module. */
int JS_ResolveModule(JSContext *ctx, JSValueConst obj);

/* only exported for os.Worker() */
JSAtom JS_GetScriptOrModuleName(JSContext *ctx, int n_stack_levels);
/* only exported for os.Worker() */
JSValue JS_LoadModule(JSContext *ctx, const char *basename,
                      const char *filename);

/* C function definition */
typedef enum JSCFunctionEnum {  /* XXX: should rename for namespace isolation */
    JS_CFUNC_generic,
    JS_CFUNC_generic_magic,
    JS_CFUNC_constructor,
    JS_CFUNC_constructor_magic,
    JS_CFUNC_constructor_or_func,
    JS_CFUNC_constructor_or_func_magic,
    JS_CFUNC_f_f,
    JS_CFUNC_f_f_f,
    JS_CFUNC_getter,
    JS_CFUNC_setter,
    JS_CFUNC_getter_magic,
    JS_CFUNC_setter_magic,
    JS_CFUNC_iterator_next,
    JS_CFUNC_host, /* JS_NewHostFunction */
} JSCFunctionEnum;

typedef union JSCFunctionType {
    JSCFunction *generic;
    JSValue (*generic_magic)(JSContext *ctx, JSValueConst this_val, int argc, JSValueConst *argv, int magic);
    JSCFunction *constructor;
    JSValue (*constructor_magic)(JSContext *ctx, JSValueConst new_target, int argc, JSValueConst *argv, int magic);
    JSCFunction *constructor_or_func;
    double (*f_f)(double);
    double (*f_f_f)(double, double);
    JSValue (*getter)(JSContext *ctx, JSValueConst this_val);
    JSValue (*setter)(JSContext *ctx, JSValueConst this_val, JSValueConst val);
    JSValue (*getter_magic)(JSContext *ctx, JSValueConst this_val, int magic);
    JSValue (*setter_magic)(JSContext *ctx, JSValueConst this_val, JSValueConst val, int magic);
    JSValue (*iterator_next)(JSContext *ctx, JSValueConst this_val,
                             int argc, JSValueConst *argv, int *pdone, int magic);
} JSCFunctionType;

JSValue JS_NewCFunction2(JSContext *ctx, JSCFunction *func,
                         const char *name,
                         int length, JSCFunctionEnum cproto, int magic);
JSValue JS_NewCFunctionData(JSContext *ctx, JSCFunctionData *func,
                            int length, int magic, int data_len,
                            JSValueConst *data);
/* Returns a function of the host, made as the built-in functions are: its
   name is the name_len bytes at name, its length is length (0 to 255), it
   has no prototype property, cannot be called with new, and toString shows
   [native code]. A call of it is host_call(vm, &this, argc, argv, id) (see
   host.h), with at least length arguments readable in argv. id is 0 or
   more: a negative one throws a RangeError. */
JSValue JS_NewHostFunction(JSContext *ctx, const char *name, size_t name_len,
                           int length, int32_t id);
/* JS_NewHostFunction of a function that holds the handle id of the host:
   when the engine frees the function, at the drop of its last reference, in
   a collection or when the runtime is freed, it calls host_release(vm, id)
   (see host.h), once. When it returns the exception it made no function and
   calls nothing: the handle stays the host's. */
JSValue JS_NewHostFunctionRelease(JSContext *ctx, const char *name, size_t name_len,
                                  int length, int32_t id);

static inline JSValue JS_NewCFunction(JSContext *ctx, JSCFunction *func, const char *name,
                                      int length)
{
    return JS_NewCFunction2(ctx, func, name, length, JS_CFUNC_generic, 0);
}

static inline JSValue JS_NewCFunctionMagic(JSContext *ctx, JSCFunctionMagic *func,
                                           const char *name,
                                           int length, JSCFunctionEnum cproto, int magic)
{
    /* Used to squelch a -Wcast-function-type warning. */
    JSCFunctionType ft = { .generic_magic = func };
    return JS_NewCFunction2(ctx, ft.generic, name, length, cproto, magic);
}
int JS_SetConstructor(JSContext *ctx, JSValueConst func_obj,
                      JSValueConst proto);

/* C property definition */

typedef struct JSCFunctionListEntry {
    const char *name;
    uint8_t prop_flags;
    uint8_t def_type;
    int16_t magic;
    union {
        struct {
            uint8_t length; /* XXX: should move outside union */
            uint8_t cproto; /* XXX: should move outside union */
            JSCFunctionType cfunc;
        } func;
        struct {
            JSCFunctionType get;
            JSCFunctionType set;
        } getset;
        struct {
            const char *name;
            int base;
        } alias;
        struct {
            const struct JSCFunctionListEntry *tab;
            int len;
        } prop_list;
        const char *str;
        int32_t i32;
        int64_t i64;
        double f64;
    } u;
} JSCFunctionListEntry;

#define JS_DEF_CFUNC          0
#define JS_DEF_CGETSET        1
#define JS_DEF_CGETSET_MAGIC  2
#define JS_DEF_PROP_STRING    3
#define JS_DEF_PROP_INT32     4
#define JS_DEF_PROP_INT64     5
#define JS_DEF_PROP_DOUBLE    6
#define JS_DEF_PROP_UNDEFINED 7
#define JS_DEF_OBJECT         8
#define JS_DEF_ALIAS          9
#define JS_DEF_PROP_ATOM     10
#define JS_DEF_PROP_BOOL     11

/* Note: c++ does not like nested designators */
#define JS_CFUNC_DEF(name, length, func1) { name, JS_PROP_WRITABLE | JS_PROP_CONFIGURABLE, JS_DEF_CFUNC, 0, .u = { .func = { length, JS_CFUNC_generic, { .generic = func1 } } } }
#define JS_CFUNC_MAGIC_DEF(name, length, func1, magic) { name, JS_PROP_WRITABLE | JS_PROP_CONFIGURABLE, JS_DEF_CFUNC, magic, .u = { .func = { length, JS_CFUNC_generic_magic, { .generic_magic = func1 } } } }
#define JS_CFUNC_SPECIAL_DEF(name, length, cproto, func1) { name, JS_PROP_WRITABLE | JS_PROP_CONFIGURABLE, JS_DEF_CFUNC, 0, .u = { .func = { length, JS_CFUNC_ ## cproto, { .cproto = func1 } } } }
#define JS_ITERATOR_NEXT_DEF(name, length, func1, magic) { name, JS_PROP_WRITABLE | JS_PROP_CONFIGURABLE, JS_DEF_CFUNC, magic, .u = { .func = { length, JS_CFUNC_iterator_next, { .iterator_next = func1 } } } }
#define JS_CGETSET_DEF(name, fgetter, fsetter) { name, JS_PROP_CONFIGURABLE, JS_DEF_CGETSET, 0, .u = { .getset = { .get = { .getter = fgetter }, .set = { .setter = fsetter } } } }
#define JS_CGETSET_MAGIC_DEF(name, fgetter, fsetter, magic) { name, JS_PROP_CONFIGURABLE, JS_DEF_CGETSET_MAGIC, magic, .u = { .getset = { .get = { .getter_magic = fgetter }, .set = { .setter_magic = fsetter } } } }
#define JS_PROP_STRING_DEF(name, cstr, prop_flags) { name, prop_flags, JS_DEF_PROP_STRING, 0, .u = { .str = cstr } }
#define JS_PROP_INT32_DEF(name, val, prop_flags) { name, prop_flags, JS_DEF_PROP_INT32, 0, .u = { .i32 = val } }
#define JS_PROP_INT64_DEF(name, val, prop_flags) { name, prop_flags, JS_DEF_PROP_INT64, 0, .u = { .i64 = val } }
#define JS_PROP_DOUBLE_DEF(name, val, prop_flags) { name, prop_flags, JS_DEF_PROP_DOUBLE, 0, .u = { .f64 = val } }
#define JS_PROP_UNDEFINED_DEF(name, prop_flags) { name, prop_flags, JS_DEF_PROP_UNDEFINED, 0, .u = { .i32 = 0 } }
#define JS_PROP_ATOM_DEF(name, val, prop_flags) { name, prop_flags, JS_DEF_PROP_ATOM, 0, .u = { .i32 = val } }
#define JS_PROP_BOOL_DEF(name, val, prop_flags) { name, prop_flags, JS_DEF_PROP_BOOL, 0, .u = { .i32 = val } }
#define JS_OBJECT_DEF(name, tab, len, prop_flags) { name, prop_flags, JS_DEF_OBJECT, 0, .u = { .prop_list = { tab, len } } }
#define JS_ALIAS_DEF(name, from) { name, JS_PROP_WRITABLE | JS_PROP_CONFIGURABLE, JS_DEF_ALIAS, 0, .u = { .alias = { from, -1 } } }
#define JS_ALIAS_BASE_DEF(name, from, base) { name, JS_PROP_WRITABLE | JS_PROP_CONFIGURABLE, JS_DEF_ALIAS, 0, .u = { .alias = { from, base } } }

int JS_SetPropertyFunctionList(JSContext *ctx, JSValueConst obj,
                               const JSCFunctionListEntry *tab,
                               int len);

/* C module definition */

typedef int JSModuleInitFunc(JSContext *ctx, JSModuleDef *m);

JSModuleDef *JS_NewCModule(JSContext *ctx, const char *name_str,
                           JSModuleInitFunc *func);
/* can only be called before the module is instantiated */
int JS_AddModuleExport(JSContext *ctx, JSModuleDef *m, const char *name_str);
int JS_AddModuleExportList(JSContext *ctx, JSModuleDef *m,
                           const JSCFunctionListEntry *tab, int len);
/* can only be called after the module is instantiated */
int JS_SetModuleExport(JSContext *ctx, JSModuleDef *m, const char *export_name,
                       JSValue val);
int JS_SetModuleExportList(JSContext *ctx, JSModuleDef *m,
                           const JSCFunctionListEntry *tab, int len);
/* associate a JSValue to a C module */
int JS_SetModulePrivateValue(JSContext *ctx, JSModuleDef *m, JSValue val);
JSValue JS_GetModulePrivateValue(JSContext *ctx, JSModuleDef *m);
                        
/* debug value output, for the hosts that define CONFIG_PRINT_VALUE */
#ifdef CONFIG_PRINT_VALUE

typedef struct {
    JS_BOOL show_hidden : 8; /* only show enumerable properties */
    JS_BOOL raw_dump : 8; /* avoid doing autoinit and avoid any malloc() call (for internal use) */
    uint32_t max_depth; /* recurse up to this depth, 0 = no limit */
    uint32_t max_string_length; /* print no more than this length for
                                   strings, 0 = no limit */
    uint32_t max_item_count; /*  print no more than this count for
                                 arrays or objects, 0 = no limit */
} JSPrintValueOptions;

typedef void JSPrintValueWrite(void *opaque, const char *buf, size_t len);

void JS_PrintValueSetDefaultOptions(JSPrintValueOptions *options);
void JS_PrintValueRT(JSRuntime *rt, JSPrintValueWrite *write_func, void *write_opaque,
                     JSValueConst val, const JSPrintValueOptions *options);
void JS_PrintValue(JSContext *ctx, JSPrintValueWrite *write_func, void *write_opaque,
                   JSValueConst val, const JSPrintValueOptions *options);
#endif /* CONFIG_PRINT_VALUE */

#undef js_unlikely
#undef js_force_inline

#ifdef __cplusplus
} /* extern "C" { */
#endif

#endif /* QUICKJS_H */
