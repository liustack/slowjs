module github.com/liustack/quickjs/lib/internal/run-test262

go 1.27.0

// The test262 runner is a module of its own, so its transpiled sources
// (about 69 MiB) stay out of the github.com/liustack/quickjs module zip.
// It always builds against the engine in this work tree.
replace github.com/liustack/quickjs => ../../..

require (
	github.com/liustack/quickjs v0.0.0-00010101000000-000000000000
	modernc.org/libc v1.77.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.48.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
