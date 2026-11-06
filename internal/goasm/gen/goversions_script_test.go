// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoVersionsScriptLogs runs scripts/go-versions.sh with a go command, a
// gen-engine.sh and a check.sh that only make the files asked for, and a
// folder of logs whose name is a glob pattern. The script removes the test
// binaries and engines it made for the post-link check, and nothing else: not
// the files of another folder the pattern matches.
func TestGoVersionsScriptLogs(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "go-versions.sh"))
	if err != nil {
		t.Fatal(err)
	}
	write("repo/scripts/go-versions.sh", string(script))
	write("repo/scripts/gen-engine.sh", "#!/bin/sh\nset -eu\n[ \"$1\" = pass ] && exit 0\nmkdir -p \"$GEN_OUT\"\n: > \"$GEN_OUT/engine_$1.s\"\n")
	write("repo/scripts/check.sh", "#!/bin/sh\nexit 0\n")
	write("llvm/clang", "#!/bin/sh\nexit 0\n")
	// go version and go env GOVERSION give the release asked for, go test -c
	// makes its -o file, and the rest does nothing.
	write("bin/go", `#!/bin/sh
case "$1" in
version) echo "go version $GOTOOLCHAIN" ;;
env) echo "$GOTOOLCHAIN" ;;
test)
	while [ $# -gt 0 ]; do
		if [ "$1" = -o ]; then : > "$2"; fi
		shift
	done ;;
esac
exit 0
`)
	other := filepath.Join(dir, "abc", "go1.27.2.linux-amd64.test")
	write("abc/go1.27.2.linux-amd64.test", "another folder's file\n")
	write("abc/go1.27.2.amd64.gen/engine_amd64.s", "another folder's engine\n")
	logs := filepath.Join(dir, "a*")
	cmd := exec.Command("sh", "scripts/go-versions.sh", "go1.27.2")
	cmd.Dir = filepath.Join(dir, "repo")
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GO_VERSIONS_LOGS="+logs, "GO_VERSIONS_LLVM=1", "LLVM_BIN="+filepath.Join(dir, "llvm"))
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "ok    go1.27.2") {
		log, _ := os.ReadFile(filepath.Join(logs, "go1.27.2.log"))
		t.Fatalf("go-versions.sh: %v\n%s\nlog:\n%s", err, out, log)
	}
	for _, p := range []string{other, filepath.Join(dir, "abc", "go1.27.2.amd64.gen", "engine_amd64.s")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("the file of another folder is gone: %v", err)
		}
	}
	left, err := filepath.Glob(filepath.Join(dir, "a[*]", "*"))
	if err != nil {
		t.Fatal(err)
	}
	var made []string
	for _, p := range left {
		if strings.HasSuffix(p, ".test") || strings.HasSuffix(p, ".gen") {
			made = append(made, filepath.Base(p))
		}
	}
	if len(made) != 0 {
		t.Errorf("the script left what it made for the post-link check: %v", made)
	}
}
