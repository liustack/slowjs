#!/bin/sh
# Regenerate the proto package, build its test binary and verify it after
# linking. Run from the slowjs root on linux/amd64 with LLVM 22.1.8.
set -e
G=internal/goasm/gen
OUT=$G/internal/proto
mkdir -p $OUT
L=/usr/lib/llvm-22/bin
PASS=$G/internal/_shadow.so
[ $PASS -nt $G/pass/shadow.cpp ] || $L/clang++ -shared -fPIC -O1 $($L/llvm-config --cxxflags) $G/pass/shadow.cpp -o $PASS
go run ./$G/cmd/gen -llvm $L -pass $PASS -data $G/testdata -out $OUT
for f in $G/testdata/*.txt; do b=$(basename $f .txt); cp $f $OUT/$b; done
go test -vet=off -c -o /tmp/proto.test ./$OUT
go run ./$G/cmd/gen -llvm $L -pass $PASS -data $G/testdata -out $OUT -bin /tmp/proto.test 2>&1 | grep -E 'verified|checks|error'
