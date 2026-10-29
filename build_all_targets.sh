# Builds every package and compiles the tests of every package for the two
# targets the engine runs on, plain and with the test hooks. (Any other target
# is refused on purpose, see TestEngineUnsupportedTarget.)
set -e
targets="darwin/arm64 linux/amd64"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
for tag in none testhooks
do
	echo "-tags=$tag"
	for target in $targets
	do
		goos=${target%/*}
		goarch=${target#*/}
		echo "GOOS=$goos GOARCH=$goarch"
		GOOS=$goos GOARCH=$goarch go build -tags=$tag -v ./...
		# The tests of every package, not only the root one. Each package gets
		# its own file, as two packages share the name gen.
		for pkg in $(GOOS=$goos GOARCH=$goarch go list -tags=$tag -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
		do
			GOOS=$goos GOARCH=$goarch go test -tags=$tag -c -o "$out/$(echo "$pkg" | tr / _).test" "$pkg"
		done
	done
done
