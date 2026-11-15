module github.com/liustack/slowjs/internal/measure/pages

go 1.26.6

// The tree it measures: this checkout, or the one go tool measure bench build
// -tree names through the go.work it writes.
replace github.com/liustack/slowjs => ../../..

// Replaced, so this version is not resolved and nothing reads it.
require github.com/liustack/slowjs v0.0.0-00010101000000-000000000000

require golang.org/x/sys v0.48.0 // indirect
