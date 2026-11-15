module github.com/liustack/slowjs

go 1.26.6

// v0.30.0 to v0.30.7 were published as github.com/liustack/quickjs. Their go.mod
// declares that path, so the go command cannot use them as github.com/liustack/slowjs.
retract [v0.30.0, v0.30.7]

require (
	golang.org/x/arch v0.32.0
	golang.org/x/sys v0.48.0
)

tool github.com/liustack/slowjs/internal/measure/cmd/measure
