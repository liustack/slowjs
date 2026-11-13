//go:build gointl_all

package intl_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
	"github.com/liustack/quickjs/internal/gointl/temporal"
)

// No exported package-level variable holds state an importer could change
// for every other: Und, Divergences, Embedded, temporal.Calendars,
// temporal.ISOCalendar and the temporal defaults had been variables any
// caller could reassign (ISSUES.md RU-4). An error sentinel, as io.EOF is,
// and Embedded, whose type has one value, are all that remain, with the
// hooks of a Limit, which internal/intlhook sets once when it starts, as
// this package cannot import it (qjs_limit.go).
func TestNoExportedMutableVariables(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "internal", "testdata", "data":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, spec := range g.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					switch {
					case !name.IsExported():
					case strings.HasPrefix(name.Name, "Err"):
					case name.Name == "Embedded" && f.Name.Name == "intl":
					case strings.HasPrefix(name.Name, "Limit") && path == "qjs_limit.go":
					default:
						t.Errorf("%s: %s.%s is an exported variable", fset.Position(name.Pos()), f.Name.Name, name.Name)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// What the functions return is the caller's own.
	d := intl.Divergences()
	d[0].Name, d[0].Flag = "Changed", intl.NodeICU
	if got := intl.Divergences()[0].Name; got != "NarrowSpace" {
		t.Errorf("Divergences()[0] is %s after a caller changed its copy", got)
	}
	if got := intl.NarrowSpace.String(); got != "NarrowSpace" {
		t.Errorf("NarrowSpace is %s after a caller changed its copy", got)
	}
	c := temporal.Calendars()
	c[0] = "changed"
	if got := temporal.Calendars()[0]; got != "buddhist" {
		t.Errorf("Calendars()[0] is %s after a caller changed its copy", got)
	}
	if temporal.ISOCalendar() != temporal.ISOCalendar() || temporal.ISOCalendar().ID() != "iso8601" {
		t.Error("ISOCalendar is not the one ISO calendar")
	}
	if o := temporal.DefaultToStringOptions(); o.Precision != temporal.PrecisionAuto || o.SmallestUnit != temporal.NoUnit {
		t.Errorf("DefaultToStringOptions() = %+v", o)
	}
}
