; Variadic calls with arguments C would have promoted: a float and an i8.
; The pass takes only promoted int, i64, pointer and double.
target datalayout = "e-m:e-p270:32:32-p271:32:32-p272:64:64-i64:64-i128:128-f80:128-n8:16:32:64-S128"
target triple = "x86_64-unknown-linux-gnu"

define i64 @v(i32 %n, ...) {
  ret i64 0
}

define i64 @passes_float() {
  %r = call i64 (i32, ...) @v(i32 1, float 1.0)
  ret i64 %r
}

define i64 @passes_byte() {
  %r = call i64 (i32, ...) @v(i32 1, i8 7)
  ret i64 %r
}

define i64 @passes_promoted(ptr %p) {
  %r = call i64 (i32, ...) @v(i32 4, i32 1, i64 2, ptr %p, double 1.0)
  ret i64 %r
}
