; Copyright (c) 2026 The liustack/beastjs Authors.
; SPDX-License-Identifier: MIT
; Use of this source code is governed by the MIT license in LICENSE at the
; root of this repository.
;
; Phis of locals that name one block several times: three cases of the switch
; in %sw go to %join, so %p and %q each have three entries for %sw. This is
; what SROA makes of a small array of two locals indexed by a counter
; (next[n++] = ...), in code with a switch. Each entry for one block must be
; the same value, so the pass must give every phi one address per block.
; %p takes %a from %sw and %b from %other, %q the other way round.
target datalayout = "e-m:e-p270:32:32-p271:32:32-p272:64:64-i64:64-i128:128-f80:128-n8:16:32:64-S128"
target triple = "x86_64-unknown-linux-gnu"

define i32 @pick(i32 %k, i32 %v) {
entry:
  %a = alloca i32, align 4
  %b = alloca i32, align 4
  store i32 1, ptr %a, align 4
  store i32 2, ptr %b, align 4
  br label %sw

sw:
  switch i32 %k, label %other [
    i32 1, label %join
    i32 2, label %join
    i32 3, label %join
  ]

other:
  br label %join

join:
  %p = phi ptr [ %a, %sw ], [ %a, %sw ], [ %a, %sw ], [ %b, %other ]
  %q = phi ptr [ %b, %sw ], [ %b, %sw ], [ %b, %sw ], [ %a, %other ]
  store i32 %v, ptr %p, align 4
  %x = load i32, ptr %q, align 4
  %y = load i32, ptr %a, align 4
  %r = add i32 %x, %y
  ret i32 %r
}
