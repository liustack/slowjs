set -e
targets="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
for tag in none libc.memgrind slowjs_nogoasm slowjs_nogoasm,libc.memgrind
do
	echo "-tags=$tag"
	for target in $targets
	do
		# slowjs_nogoasm changes the build of linux/amd64 alone.
		case $tag,$target in slowjs_nogoasm*,linux/amd64) ;; slowjs_nogoasm*) continue ;; esac
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
