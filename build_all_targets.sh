set -e
targets="darwin/amd64 darwin/arm64 freebsd/amd64 freebsd/arm64 linux/386 linux/amd64 linux/arm linux/arm64 linux/loong64 linux/ppc64le linux/riscv64 linux/s390x windows/amd64 windows/arm64"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
for tag in none libc.memgrind
do
	echo "-tags=$tag"
	for target in $targets
	do
		goos=${target%/*}
		goarch=${target#*/}
		echo "GOOS=$goos GOARCH=$goarch"
		GOOS=$goos GOARCH=$goarch go build -tags=$tag -v ./...
		# The tests of every package, not only the root one: the tests of
		# lib/ use the value representation, which is not a struct on 32-bit
		# targets.
		GOOS=$goos GOARCH=$goarch go test -tags=$tag -c -o "$out/" ./...
	done
done
