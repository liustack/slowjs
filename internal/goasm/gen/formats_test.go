// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"os"
	"strings"
	"testing"
)

// FormatError agrees with testdata/formats/cases.txt, the table that
// mirrors engine/libc/stdio.c.
func TestFormatTable(t *testing.T) {
	b, err := os.ReadFile("testdata/formats/cases.txt")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || l[0] == '#' {
			continue
		}
		verdict, f, ok := strings.Cut(l, "\t")
		if !ok || (verdict != "ok" && verdict != "bad") {
			t.Fatalf("bad line %q", l)
		}
		err := FormatError(f)
		if (err == nil) != (verdict == "ok") {
			t.Errorf("%q: the table says %s, FormatError says %v", f, verdict, err)
		}
		n++
	}
	if n < 90 {
		t.Fatalf("only %d cases read", n)
	}
}

// The calls that reach the formatter are found in IR the way clang writes it
// without optimization, and each one's format is held to the table.
func TestCheckFormats(t *testing.T) {
	const strs = `@.ok = private unnamed_addr constant [9 x i8] c"%s: %ld\00", align 1
@.pct = private unnamed_addr constant [6 x i8] c"100%%\00", align 1
@.float = private unnamed_addr constant [6 x i8] c"x=%f\0A\00", align 1
@.long = private unnamed_addr constant [6 x i8] c"%lf!\00\00", align 1
@.cut = private unnamed_addr constant [4 x i8] c"ab%\00", align 1
@.tail = private unnamed_addr constant [10 x i8] c"%f then %d\00", align 1
@.arr = internal constant [2 x ptr] [ptr @.ok, ptr @.float], align 8
`
	wrapper := func(body string) string { return strs + body }
	ok := map[string]string{
		"a constant format to a variadic function": `define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef @.ok, ptr noundef %ctx, i64 noundef 3)
  ret void
}`,
		"snprintf": `define void @a(ptr %b) {
  %r = call i32 (ptr, i64, ptr, ...) @snprintf(ptr noundef %b, i64 noundef 8, ptr noundef @.ok, ptr noundef %b, i64 noundef 1)
  ret void
}`,
		"vsnprintf with a constant": `define i32 @a(ptr %b, ptr %ap) {
  %r = call i32 @vsnprintf(ptr noundef %b, i64 noundef 8, ptr noundef @.ok, ptr noundef %ap)
  ret i32 %r
}`,
		"a suffix of a constant (merged strings)": `define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr getelementptr inbounds nuw (i8, ptr @.ok, i64 3), i64 noundef 3)
  ret void
}`,
		"percent signs": `define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef @.pct)
  ret void
}`,
		"a wrapper that forwards its own format": `define i64 @wrap(ptr %ctx, ptr %fmt, ...) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %fmt, i64 noundef 3)
  ret i64 %r
}`,
		"a wrapper that stores its format and reads it back": `define i64 @wrap(ptr %0, ptr %1, ...) {
  %3 = alloca ptr, align 8
  %4 = alloca ptr, align 8
  store ptr %0, ptr %3, align 8
  store ptr %1, ptr %4, align 8
  %5 = load ptr, ptr %3, align 8
  %6 = load ptr, ptr %4, align 8
  %7 = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %5, ptr noundef %6, i64 noundef 3)
  ret i64 %7
}`,
		"one of two constants by a branch": `define void @a(ptr %ctx, i1 %c) {
entry:
  br i1 %c, label %l, label %r
l:
  br label %j
r:
  br label %j
j:
  %f = phi ptr [ @.ok, %l ], [ @.pct, %r ]
  %x = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %f)
  ret void
}`,
		"one of two constants by a select": `define void @a(ptr %ctx, i1 %c) {
  %f = select i1 %c, ptr @.ok, ptr @.pct
  %x = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %f)
  ret void
}`,
		"a wrapper whose fixed arguments are not the format alone": `define i64 @wrap(ptr %ctx, i32 %atom, ptr %fmt, ...) {
  %r = call i64 (ptr, i32, ptr, ...) @inner(ptr noundef %ctx, i32 noundef %atom, ptr noundef %fmt, i64 noundef 3)
  ret i64 %r
}`,
		"the formatter of a va_list": `define i64 @JS_ThrowError2(ptr %ctx, i32 %k, ptr %fmt, ptr %ap) {
  %r = call i32 @vsnprintf(ptr noundef %ctx, i64 noundef 8, ptr noundef %fmt, ptr noundef %ap)
  ret i64 0
}`,
		"a variadic function whose arguments are opcodes": `define void @a(ptr %s) {
  %r = call i32 (ptr, i32, ...) @code_match(ptr noundef %s, i32 noundef 3, i32 noundef 4)
  call void (ptr, ...) @unicode_prop_ops(ptr noundef %s, i32 noundef 1)
  ret void
}`,
		"a call of something else": `define void @a(ptr %p) {
  %r = call i32 @puts(ptr noundef @.float)
  ret void
}`,
	}
	for name, body := range ok {
		if vs := CheckFormats(wrapper(body), EngineNonFormatVariadic); len(vs) != 0 {
			t.Errorf("%s: %+v", name, vs)
		}
	}
	bad := map[string]struct{ ir, want string }{
		"a float conversion to a variadic function": {`define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef @.float, double noundef 1.0)
  ret void
}`, `the conversion "f"`},
		"a float conversion to snprintf": {`define void @a(ptr %b) {
  %r = call i32 (ptr, i64, ptr, ...) @snprintf(ptr noundef %b, i64 noundef 8, ptr noundef @.float, double noundef 1.0)
  ret void
}`, `the conversion "f"`},
		"a float conversion to vsnprintf": {`define i32 @a(ptr %b, ptr %ap) {
  %r = call i32 @vsnprintf(ptr noundef %b, i64 noundef 8, ptr noundef @.long, ptr noundef %ap)
  ret i32 %r
}`, `the conversion "f"`},
		"a conversion behind a suffix": {`define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr getelementptr inbounds nuw (i8, ptr @.tail, i64 0))
  ret void
}`, `the conversion "f"`},
		"a format that ends inside a conversion": {`define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef @.cut)
  ret void
}`, "ends inside"},
		"a format that is not a constant": {`define void @a(ptr %ctx, ptr %fmt) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %fmt)
  ret void
}`, "is not a constant"},
		"a format from memory": {`define void @a(ptr %ctx, ptr %t) {
  %f = load ptr, ptr %t, align 8
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %f)
  ret void
}`, "not a string constant the check can read"},
		"a format that may be a float one, by a phi": {`define void @a(ptr %ctx, i1 %c) {
entry:
  br i1 %c, label %l, label %j
l:
  br label %j
j:
  %f = phi ptr [ @.ok, %entry ], [ @.float, %l ]
  %x = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %f)
  ret void
}`, `the conversion "f"`},
		"a format that may be a parameter or a constant, in a function that forwards no format": {`define void @a(ptr %ctx, ptr %fmt, i1 %c) {
  %f = select i1 %c, ptr @.ok, ptr %fmt
  %x = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %f)
  ret void
}`, "is not a constant"},
		"a format stored from a parameter that does not forward": {`define void @a(ptr %0, ptr %1) {
  %3 = alloca ptr, align 8
  store ptr %1, ptr %3, align 8
  %4 = load ptr, ptr %3, align 8
  %5 = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %0, ptr noundef %4)
  ret void
}`, "is not a constant"},
		"a format from a table": {`define void @a(ptr %ctx) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef @.arr)
  ret void
}`, "not a string constant the check can read"},
		"a variadic function's other parameter as the format": {`define i64 @wrap(ptr %ctx, ptr %fmt, ptr %other, ...) {
  %r = call i64 (ptr, ptr, ...) @JS_ThrowTypeError(ptr noundef %ctx, ptr noundef %fmt)
  ret i64 %r
}`, "is not a constant"},
		"a function that is not variadic forwarding a format": {`define i32 @a(ptr %b, ptr %fmt, ptr %ap) {
  %r = call i32 @vsnprintf(ptr noundef %b, i64 noundef 8, ptr noundef %fmt, ptr noundef %ap)
  ret i32 %r
}`, "is not a constant"},
		"a call with no format": {`define void @a(ptr %ctx) {
  %r = call i64 (...) @weird(ptr noundef %ctx)
  ret void
}`, "no format argument"},
	}
	// Another list: the same call is no longer left out, and a name that is
	// left out is left out only by name.
	custom := wrapper(`define void @a(ptr %s) {
  %r = call i32 (ptr, i32, ...) @code_match(ptr noundef %s, i32 noundef 3, i32 noundef 4)
  %q = call i32 (ptr, i32, ...) @sum_up(ptr noundef %s, i32 noundef 3, i32 noundef 4)
  ret void
}`)
	if vs := CheckFormats(custom, []string{"sum_up"}); len(vs) != 1 || !strings.Contains(vs[0].Line, "code_match") {
		t.Errorf("a list of its own: %+v", vs)
	}
	for name, c := range bad {
		vs := CheckFormats(wrapper(c.ir), EngineNonFormatVariadic)
		if len(vs) != 1 || vs[0].Func != "a" && vs[0].Func != "wrap" || !strings.Contains(vs[0].Rule, c.want) {
			t.Errorf("%s: %+v, want one violation saying %q", name, vs, c.want)
		}
	}
}
