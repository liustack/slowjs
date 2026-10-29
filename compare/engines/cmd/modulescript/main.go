// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Command modulescript makes a tifa SSR bundle a script.
//
//	go run ./cmd/modulescript <ssr.js> <out.js>
//
// tifa builds its SSR bundle as an ES module with no imports, whose only
// export is the `export{};` at its end. Engines without a module loader of the
// same kind (qjs --std, Node's vm, the Go engines' script entry) run it as a
// script: this drops that export and the source map comment and wraps the rest
// in a strict function, so its top-level names stay local and strict mode
// holds as in a module. A top-level await, import or other export does not
// compile in the function, so such a bundle fails at load rather than running
// differently.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

var sourceMap = regexp.MustCompile(`\n//# sourceMappingURL=\S+\s*$`)

const export = "export{};"

// script returns the bundle src, an ES module, as a script.
func script(src string) (string, error) {
	body := strings.TrimRightFunc(sourceMap.ReplaceAllString(src, "\n"), unicode.IsSpace)
	body, ok := strings.CutSuffix(body, export)
	if !ok {
		return "", fmt.Errorf("does not end with %s, not a tifa ES module bundle", export)
	}
	return `(function(){"use strict";` + "\n" + body + "\n})();\n", nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: modulescript <ssr.js> <out.js>")
		os.Exit(2)
	}
	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "modulescript:", err)
		os.Exit(1)
	}
	out, err := script(string(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "modulescript: %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[2], []byte(out), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "modulescript:", err)
		os.Exit(1)
	}
}
