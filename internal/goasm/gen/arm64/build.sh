#!/bin/sh
# Generate the arm64 proof package from testdata/engine.c. Run from this dir
# (internal/goasm/gen/arm64) on a host with LLVM 22.1.8 whose clang/llvm-mc
# can target aarch64 (native aarch64, or x86 cross with the aarch64 target
# built in). Output goes to the module under internal/mini, which builds and
# runs on any arm64 Go (darwin or linux). Set CONV_NO_SAVES=1 to build the
# stub without the R19-R25/F8-F15 saves, for the isolation timing.
set -e
L=${SLOWJS_LLVM:-/usr/lib/llvm-22/bin}
OUT=${OUT:-internal/mini}
F="--target=aarch64-linux-gnu -O2 -fwrapv -ffp-contract=off -ffreestanding -nostdinc -fno-stack-protector -fno-asynchronous-unwind-tables -fno-unwind-tables -mno-outline-atomics -mllvm -enable-shrink-wrap=false -ffixed-x18 -ffixed-x26 -ffixed-x27 -ffixed-x28 -Xclang -mframe-pointer=reserved -Xclang -target-feature -Xclang +reserve-lr-for-ra"
mkdir -p _work $OUT
$L/clang $F -S -o _work/engine.s testdata/engine.c
(cd conv && go build -o ../_work/conv .)
_work/conv lower _work/engine.s testdata/spec.json _work/lowered.s _work/frames.json
$L/llvm-mc -triple=aarch64-linux-gnu -filetype=obj -o _work/lowered.o _work/lowered.s
_work/conv emit _work/lowered.o testdata/spec.json _work/frames.json $OUT/gen_arm64.s $OUT/gen_decl.go
for f in testdata/*.go.txt; do cp $f $OUT/$(basename $f .txt); done
for f in testdata/*.s.txt; do cp $f $OUT/$(basename $f .txt); done
echo "generated $OUT"
