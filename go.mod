// Deprecated: the module is github.com/liustack/quickjs again, from v0.30.0 on.
// Change the import path and the package name slowjs to quickjs.
module github.com/liustack/slowjs

go 1.26.6

require (
	golang.org/x/arch v0.32.0
	golang.org/x/sys v0.48.0
)

tool github.com/liustack/slowjs/internal/sjdev/cmd/sjdev
