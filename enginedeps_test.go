// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The package runs on the C engine of internal/cengine and its Go host.
func TestEngineDeps(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}

	const (
		cengine = "github.com/liustack/slowjs/internal/cengine"
		gohost  = "github.com/liustack/slowjs/internal/gohost"
		hostmem = "github.com/liustack/slowjs/internal/hostmem"
	)
	for _, c := range []struct {
		name string
		args []string
		env  []string
		want []string
	}{
		{"linux/amd64", nil, []string{"GOOS=linux", "GOARCH=amd64"}, []string{cengine, gohost, hostmem}},
		{"darwin/arm64", nil, []string{"GOOS=darwin", "GOARCH=arm64"}, []string{cengine, gohost, hostmem}},
		{"linux/arm64", nil, []string{"GOOS=linux", "GOARCH=arm64"}, []string{cengine, gohost, hostmem}},
		{"darwin/amd64", nil, []string{"GOOS=darwin", "GOARCH=amd64"}, []string{cengine, gohost, hostmem}},
		{"windows/amd64", nil, []string{"GOOS=windows", "GOARCH=amd64"}, []string{cengine, gohost, hostmem}},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(gobin, append(append([]string{"list", "-deps"}, c.args...), ".")...)
			// The flags of the run that started the test are not those of
			// the row.
			cmd.Env = append(append(os.Environ(), "GOFLAGS="), c.env...)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("go list: %v", err)
			}
			deps := strings.Fields(string(out))
			for _, p := range c.want {
				if !slices.Contains(deps, p) {
					t.Errorf("does not depend on %s", p)
				}
			}

		})
	}
}

// A build on a target the C engine does not run on fails with the reason as
// its only error, not under errors of the files that need the
// engine. The engine runs on linux and darwin, on amd64 and arm64, and on
// windows/amd64, only.
func TestEngineUnsupportedTarget(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	for _, target := range []string{"freebsd/amd64", "windows/arm64", "linux/386", "linux/riscv64"} {
		t.Run(target, func(t *testing.T) {
			goos, goarch, _ := strings.Cut(target, "/")
			cmd := exec.Command(gobin, "build", "-o", os.DevNull, ".")
			cmd.Env = append(os.Environ(), "GOFLAGS=", "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("the build succeeded")
			}
			var errs []string
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if !strings.HasPrefix(l, "#") {
					errs = append(errs, l)
				}
			}
			if len(errs) != 1 || !strings.Contains(errs[0], "the_C_engine_runs_only_on_linux_and_darwin_on_amd64_and_arm64_and_on_windows_amd64_this_target_is_not_supported") {
				t.Fatalf("the build said\n%s\nwant only the error that names the targets", out)
			}
		})
	}
}

// A Go newer than the one the engine was checked with stops the build with
// the error of internal/goversion and no other, and -tags slowjs_anygo builds
// anyway. The go1.N tag given by hand stands for the newer Go: with it, the go
// command also says the file of the error asks for a newer Go.
func TestEngineNewerGo(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	src, err := os.ReadFile("internal/goversion/newer.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^//go:build (go1\.\d+) && !slowjs_anygo$`).FindSubmatch(src)
	if m == nil {
		t.Fatalf("internal/goversion/newer.go has no build line of the form the test reads:\n%s", src)
	}
	newer := string(m[1])
	build := func(tags string) ([]byte, error) {
		cmd := exec.Command(gobin, "build", "-tags", tags, "-o", os.DevNull, ".")
		cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0")
		return cmd.CombinedOutput()
	}
	out, err := build(newer)
	if err == nil {
		t.Fatalf("the build with %s succeeded", newer)
	}
	named := false
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(l, "#") {
			continue
		}
		if !strings.HasPrefix(l, "internal/goversion/newer.go:") {
			t.Fatalf("the build with %s said\n%s\nwant only the errors of internal/goversion", newer, out)
		}
		named = named || strings.Contains(l, "slowjs_is_checked_up_to_go1_")
	}
	if !named {
		t.Fatalf("the build with %s said\n%s\nwant the error that names the Go the engine was checked with", newer, out)
	}
	if out, err := build(newer + ",slowjs_anygo"); err != nil {
		t.Fatalf("the build with %s and slowjs_anygo failed:\n%s", newer, out)
	}
}

// A build option that makes the assembler encode the calls and the loads of
// symbol addresses of the engine with other lengths stops the build, with the
// error of the tripwire in internal/cengine and no other. Without the stop the
// build succeeds and the first NewVM crashes. -asmflags=PKG=... limits the
// option to the engine, as -buildmode=plugin and -buildmode=shared would pass
// -dynlink to every package. Options that leave the encodings of the engine
// alone still build.
func TestEngineAsmOptions(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	const cengine = "github.com/liustack/slowjs/internal/cengine"
	for _, c := range []struct {
		name     string
		goos     string
		goarch   string
		asmflags string
		want     string // empty: the build must succeed
	}{
		{"spectre=ret/linux/amd64", "linux", "amd64", "-spectre=ret", "non-retpoline-compatible"},
		{"spectre=all/darwin/amd64", "darwin", "amd64", "-spectre=all", "non-retpoline-compatible"},
		{"spectre=ret/windows/amd64", "windows", "amd64", "-spectre=ret", "non-retpoline-compatible"},
		{"dynlink/linux/amd64", "linux", "amd64", "-dynlink", "R15 is clobbered"},
		{"dynlink/linux/arm64", "linux", "arm64", "-dynlink", "do not know how to handle TYPE_ADDR"},
		{"dynlink/darwin/arm64", "darwin", "arm64", "-dynlink", "do not know how to handle TYPE_ADDR"},
		// What -buildmode=pie passes on linux and darwin, and -spectre=index
		// (for the compiler only).
		{"shared/linux/amd64", "linux", "amd64", "-shared", ""},
		{"shared/linux/arm64", "linux", "arm64", "-shared", ""},
		{"spectre=index/linux/amd64", "linux", "amd64", "-spectre=index", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(gobin, "build", "-asmflags="+cengine+"="+c.asmflags, "-o", os.DevNull, ".")
			cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0", "GOOS="+c.goos, "GOARCH="+c.goarch)
			out, err := cmd.CombinedOutput()
			if c.want == "" {
				if err != nil {
					t.Fatalf("the build failed:\n%s", out)
				}
				return
			}
			if err == nil {
				t.Fatalf("the build with %s succeeded", c.asmflags)
			}
			named := false
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if strings.HasPrefix(l, "#") || l == "asm: assembly failed" {
					continue
				}
				if !strings.Contains(l, "goasm_tripwire.") {
					t.Fatalf("the build said\n%s\nwant only the errors of the tripwire", out)
				}
				named = named || strings.Contains(l, c.want)
			}
			if !named {
				t.Fatalf("the build said\n%s\nwant the error %q", out, c.want)
			}
		})
	}
}

// -buildmode=shared passes -dynlink to the assembler of every package, and is
// the way to reach it without cgo (-buildmode=plugin needs cgo). The build
// stops at the tripwire of the engine.
func TestEngineBuildmodeShared(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	if testing.Short() {
		t.Skip("builds the standard library again for -buildmode=shared")
	}
	cmd := exec.Command(gobin, "build", "-buildmode=shared", "./internal/cengine")
	cmd.Env = append(os.Environ(), "GOFLAGS=", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("the build succeeded")
	}
	if !strings.Contains(string(out), "R15 is clobbered") {
		t.Fatalf("the build said\n%s\nwant the error of the tripwire", out)
	}
}
