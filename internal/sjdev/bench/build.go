// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/liustack/slowjs/internal/sjdev"
)

// BuildOptions say which binaries to build.
type BuildOptions struct {
	Side      string     // label of the side, such as "base" or "new"
	Tree      string     // the slowjs checkout, with the engine generated into internal/cengine
	App       string     // build in this module instead (tifa), with a go.work that uses Tree
	Pkg       string     // the package to build, relative to App (or Tree)
	Test      bool       // build the test binary of Pkg (go test -c)
	Tags      string     // build tags
	PGO       string     // the -pgo flag: "off" or a profile
	Front     string     // the interpreter function whose body moves
	Positions []Position // the positions to build
	Out       string     // folder of the binaries
}

// Defaults fills what is not set.
func (o *BuildOptions) Defaults() {
	if o.Tree == "" {
		o.Tree = "."
	}
	if o.PGO == "" {
		o.PGO = "off"
	}
	if o.Front == "" {
		o.Front = "JS_CallInternal"
	}
	if o.Tags == "" {
		o.Tags = "slowjs_cengine"
	}
	if len(o.Positions) == 0 {
		o.Positions = Quick()
	}
}

var sideRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Check reports why the options cannot be used.
func (o BuildOptions) Check() error {
	if !sideRe.MatchString(o.Side) {
		return fmt.Errorf("side %q: use letters, digits, _ and - only", o.Side)
	}
	if o.Pkg == "" {
		return fmt.Errorf("no package to build: pass -pkg")
	}
	if o.Out == "" {
		return fmt.Errorf("no output folder: pass -out")
	}
	if !strings.Contains(o.Tags, "slowjs_cengine") {
		return fmt.Errorf("tags %q: positions are made for the C engine, which needs slowjs_cengine", o.Tags)
	}
	return nil
}

// Generated lists the files that scripts/gen-engine.sh writes into dir for the
// architecture (the .gitignore of internal/cengine lists the same), sorted.
func Generated(dir, arch string) ([]string, error) {
	var all []string
	for _, p := range []string{"*_" + arch + ".s", "engine_*_" + arch + ".go"} {
		m, err := filepath.Glob(filepath.Join(dir, p))
		if err != nil {
			return nil, err
		}
		all = append(all, m...)
	}
	slices.Sort(all)
	return all, nil
}

// Xform moves the code of the assembly files in dir.
type Xform func(ctx context.Context, dir string, ops []string) error

// Overlay is the JSON file of go build -overlay.
type Overlay struct {
	Replace map[string]string
}

// MakeOverlay copies the generated amd64 files of the tree to work, moves the
// code in the copies, and returns the overlay that puts the changed copies in
// place of the files of the tree. The tree is not touched.
func MakeOverlay(ctx context.Context, tree, work string, p Position, front string, xf Xform) (Overlay, error) {
	src := filepath.Join(tree, "internal", "cengine")
	files, err := Generated(src, "amd64")
	if err != nil {
		return Overlay{}, err
	}
	var asm []string
	for _, f := range files {
		if strings.HasSuffix(f, "_amd64.s") {
			asm = append(asm, f)
		}
	}
	if len(asm) == 0 {
		return Overlay{}, fmt.Errorf("no generated amd64 assembly in %s: run sjdev gen", src)
	}
	if err := os.RemoveAll(work); err != nil {
		return Overlay{}, err
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return Overlay{}, err
	}
	orig := map[string][]byte{}
	for _, f := range asm {
		b, err := os.ReadFile(f)
		if err != nil {
			return Overlay{}, err
		}
		orig[filepath.Base(f)] = b
		if err := os.WriteFile(filepath.Join(work, filepath.Base(f)), b, 0o644); err != nil {
			return Overlay{}, err
		}
	}
	if err := xf(ctx, work, p.XformOps(front)); err != nil {
		return Overlay{}, fmt.Errorf("position %s: %w", p, err)
	}
	ov := Overlay{Replace: map[string]string{}}
	for name, b := range orig {
		moved, err := os.ReadFile(filepath.Join(work, name))
		if err != nil {
			return Overlay{}, err
		}
		if string(moved) != string(b) {
			ov.Replace[filepath.Join(src, name)] = filepath.Join(work, name)
		}
	}
	if p != (Position{}) && len(ov.Replace) == 0 {
		return Overlay{}, fmt.Errorf("position %s changed no file", p)
	}
	return ov, nil
}

// ResolveFront returns the name the interpreter has in the generated assembly of
// the tree. A function with many arguments is generated as NAME_block, which takes
// its arguments in a block, and that is where the body sits.
func ResolveFront(tree, name string) (string, error) {
	files, err := Generated(filepath.Join(tree, "internal", "cengine"), "amd64")
	if err != nil {
		return "", err
	}
	for _, cand := range []string{name, name + "_block"} {
		re := regexp.MustCompile(`(?m)^TEXT ·` + regexp.QuoteMeta(cand) + `\(SB\)`)
		for _, f := range files {
			if !strings.HasSuffix(f, "_amd64.s") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				return "", err
			}
			if re.Match(b) {
				return cand, nil
			}
		}
	}
	return "", fmt.Errorf("no function %s or %s_block in the generated assembly of %s", name, name, tree)
}

// BuildArgs returns the go command line of one variant.
func (o BuildOptions) BuildArgs(overlay, bin string) []string {
	args := []string{"build"}
	if o.Test {
		args = []string{"test", "-c"}
	}
	args = append(args, "-overlay", overlay, "-pgo="+o.PGO, "-buildvcs=false", "-tags", o.Tags, "-o", bin, o.Pkg)
	return args
}

var goVersionRe = regexp.MustCompile(`(?m)^go (\S+)$`)

// WorkFile returns the text of a go.work that builds the app against the tree.
func WorkFile(goVersion string, dirs ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "go %s\n\n", goVersion)
	for _, d := range dirs {
		fmt.Fprintf(&b, "use %s\n", d)
	}
	return b.String()
}

// Build makes one binary per position in o.Out.
func Build(ctx context.Context, o BuildOptions, xf Xform, run sjdev.Runner, log io.Writer) error {
	o.Defaults()
	if err := o.Check(); err != nil {
		return err
	}
	abs := func(p string) (string, error) {
		a, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(a)
	}
	tree, err := abs(o.Tree)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return err
	}
	out, err := abs(o.Out)
	if err != nil {
		return err
	}
	front, err := ResolveFront(tree, o.Front)
	if err != nil {
		return err
	}
	if front != o.Front {
		fmt.Fprintf(log, "the body of %s is %s in the generated assembly\n", o.Front, front)
	}
	dir, env := tree, []string{"CGO_ENABLED=0"}
	if o.App != "" {
		app, err := abs(o.App)
		if err != nil {
			return err
		}
		mod, err := os.ReadFile(filepath.Join(tree, "go.mod"))
		if err != nil {
			return err
		}
		m := goVersionRe.FindSubmatch(mod)
		if m == nil {
			return fmt.Errorf("no go line in %s/go.mod", tree)
		}
		wf := filepath.Join(out, ".work", o.Side+".work")
		if err := os.MkdirAll(filepath.Dir(wf), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(wf, []byte(WorkFile(string(m[1]), app, tree)), 0o644); err != nil {
			return err
		}
		dir, env = app, append(env, "GOWORK="+wf)
	}
	for _, p := range o.Positions {
		bin := filepath.Join(out, BinName(o.Side, p))
		work := filepath.Join(out, ".work", BinName(o.Side, p))
		ov, err := MakeOverlay(ctx, tree, work, p, front, xf)
		if err != nil {
			return err
		}
		ovFile := work + ".overlay.json"
		b, _ := json.MarshalIndent(ov, "", "\t")
		if err := os.WriteFile(ovFile, b, 0o644); err != nil {
			return err
		}
		c := sjdev.Cmd{Dir: dir, Env: env, Name: "go", Args: o.BuildArgs(ovFile, bin)}
		fmt.Fprintf(log, "+ %s\n", c)
		if r := run(ctx, c, nil); r.Err != nil {
			return fmt.Errorf("%w\n%s", r.Err, r.Out)
		}
		fmt.Fprintf(log, "built %s\n", filepath.Base(bin))
	}
	return nil
}

// XformCommand returns the Xform that runs the built xform binary.
func XformCommand(bin string, run sjdev.Runner) Xform {
	return func(ctx context.Context, dir string, ops []string) error {
		r := run(ctx, sjdev.Cmd{Name: bin, Args: append([]string{dir}, ops...)}, nil)
		if r.Err != nil {
			return fmt.Errorf("%w\n%s", r.Err, r.Out)
		}
		return nil
	}
}

// BuildXform builds internal/goasm/cmd/xform of the tree into out and returns
// the Xform that runs it.
func BuildXform(ctx context.Context, tree, out string, run sjdev.Runner) (Xform, error) {
	bin := filepath.Join(out, ".xform")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(bin)
	if err != nil {
		return nil, err
	}
	r := run(ctx, sjdev.Cmd{Dir: tree, Env: []string{"CGO_ENABLED=0"}, Name: "go", Args: []string{"build", "-o", abs, "./internal/goasm/cmd/xform"}}, nil)
	if r.Err != nil {
		return nil, fmt.Errorf("%w\n%s", r.Err, r.Out)
	}
	return XformCommand(abs, run), nil
}

// Sorted returns the names of the binaries in dir ("side@pos"), sorted.
func Sorted(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.Contains(e.Name(), "@") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}
