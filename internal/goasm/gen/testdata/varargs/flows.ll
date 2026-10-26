; Where a va_list may go. The pass follows it through calls to local
; functions that read it (the take functions read the overflow field the way
; clang's va_arg does once va_start has used up the registers), and refuses
; every other flow: through a select, a phi, or memory. direct is the control
; that stays.
target datalayout = "e-m:e-p270:32:32-p271:32:32-p272:64:64-i64:64-i128:128-f80:128-n8:16:32:64-S128"
target triple = "x86_64-unknown-linux-gnu"

declare void @llvm.va_start.p0(ptr)
declare void @llvm.va_end.p0(ptr)

define i64 @take(ptr %ap) {
  %f = getelementptr inbounds i8, ptr %ap, i64 8
  %ov = load ptr, ptr %f, align 8
  %v = load i64, ptr %ov, align 8
  %nx = getelementptr inbounds i8, ptr %ov, i64 8
  store ptr %nx, ptr %f, align 8
  ret i64 %v
}

define i64 @take_select(ptr %ap) {
  %f = getelementptr inbounds i8, ptr %ap, i64 8
  %ov = load ptr, ptr %f, align 8
  %v = load i64, ptr %ov, align 8
  %nx = getelementptr inbounds i8, ptr %ov, i64 8
  store ptr %nx, ptr %f, align 8
  ret i64 %v
}

define i64 @take_phi(ptr %ap) {
  %f = getelementptr inbounds i8, ptr %ap, i64 8
  %ov = load ptr, ptr %f, align 8
  %v = load i64, ptr %ov, align 8
  %nx = getelementptr inbounds i8, ptr %ov, i64 8
  store ptr %nx, ptr %f, align 8
  ret i64 %v
}

define i64 @take_memory(ptr %ap) {
  %f = getelementptr inbounds i8, ptr %ap, i64 8
  %ov = load ptr, ptr %f, align 8
  %v = load i64, ptr %ov, align 8
  %nx = getelementptr inbounds i8, ptr %ov, i64 8
  store ptr %nx, ptr %f, align 8
  ret i64 %v
}

define i64 @direct(i32 %n, ...) {
  %a = alloca [24 x i8], align 16
  call void @llvm.va_start.p0(ptr %a)
  %v = call i64 @take(ptr %a)
  call void @llvm.va_end.p0(ptr %a)
  ret i64 %v
}

; One of two lists, by a select.
define i64 @via_select(i32 %n, i1 %c, ...) {
  %a = alloca [24 x i8], align 16
  %b = alloca [24 x i8], align 16
  call void @llvm.va_start.p0(ptr %a)
  call void @llvm.va_start.p0(ptr %b)
  %p = select i1 %c, ptr %a, ptr %b
  %v = call i64 @take_select(ptr %p)
  call void @llvm.va_end.p0(ptr %a)
  call void @llvm.va_end.p0(ptr %b)
  ret i64 %v
}

; One of two lists, by a phi where two paths meet.
define i64 @via_phi(i32 %n, i1 %c, ...) {
entry:
  %a = alloca [24 x i8], align 16
  %b = alloca [24 x i8], align 16
  call void @llvm.va_start.p0(ptr %a)
  call void @llvm.va_start.p0(ptr %b)
  br i1 %c, label %left, label %right
left:
  br label %join
right:
  br label %join
join:
  %p = phi ptr [ %a, %left ], [ %b, %right ]
  %v = call i64 @take_phi(ptr %p)
  call void @llvm.va_end.p0(ptr %a)
  call void @llvm.va_end.p0(ptr %b)
  ret i64 %v
}

; The list's address stored in a local and read back.
define i64 @via_memory(i32 %n, ...) {
  %a = alloca [24 x i8], align 16
  %slot = alloca ptr, align 8
  call void @llvm.va_start.p0(ptr %a)
  store ptr %a, ptr %slot, align 8
  %q = load ptr, ptr %slot, align 8
  %v = call i64 @take_memory(ptr %q)
  call void @llvm.va_end.p0(ptr %a)
  ret i64 %v
}
