#!/bin/sh
# Regenerate the proto package and build its tests. Run from the slowjs root
# with LLVM 22.1.8. ARCH=amd64 (the default) builds the test binary and
# verifies it after linking. ARCH=arm64 cross-builds the test binary as a
# compile check: copy the package to an arm64 machine to run it. MODE picks
# how unified calls keep registers (see cmd/gen). BIN is where the test
# binary goes.
set -e
G=internal/goasm/gen
ARCH=${ARCH:-amd64}
MODE=${MODE:-body}
OUT=${OUT:-$G/internal/proto}
BIN=${BIN:-/tmp/proto.test}
L=/usr/lib/llvm-22/bin
PASS=$G/internal/_shadow.so
[ $PASS -nt $G/pass/shadow.cpp ] || $L/clang++ -shared -fPIC -O1 $($L/llvm-config --cxxflags) $G/pass/shadow.cpp -o $PASS
rm -rf $OUT
mkdir -p $OUT
go run ./$G/cmd/gen -arch $ARCH -mode $MODE -llvm $L -pass $PASS -data $G/testdata -out $OUT
for f in $G/testdata/*.txt; do cp $f $OUT/$(basename $f .txt); done
if [ $ARCH = arm64 ]; then
	GOARCH=arm64 go test -vet=off -c -o $BIN ./$OUT
	exit
fi
go test -vet=off -c -o $BIN ./$OUT
go run ./$G/cmd/gen -arch $ARCH -mode $MODE -llvm $L -pass $PASS -data $G/testdata -out $OUT -bin $BIN 2>&1 | grep -E 'verified|checks|error'
