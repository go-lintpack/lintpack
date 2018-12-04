package linttest

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/go-lintpack/lintpack"
	"github.com/go-toolsmith/pkgload"
	"golang.org/x/tools/go/packages"
)

var sizes = types.SizesFor("gc", runtime.GOARCH)

func saneCheckersList(t *testing.T) []*lintpack.CheckerInfo {
	var saneList []*lintpack.CheckerInfo

	for _, info := range lintpack.GetCheckersInfo() {
		pkgPath := "github.com/go-lintpack/lintpack/linttest/testdata/sanity"
		t.Run(info.Name+"/sanity", func(t *testing.T) {
			fset := token.NewFileSet()
			pkgs := newPackages(t, pkgPath, fset)
			for _, pkg := range pkgs {
				ctx := &lintpack.Context{
					SizesInfo: sizes,
					FileSet:   fset,
					TypesInfo: pkg.TypesInfo,
					Pkg:       pkg.Types,
				}
				c := lintpack.NewChecker(ctx, info)
				defer func() {
					r := recover()
					if r != nil {
						t.Errorf("unexpected panic: %v\n%s", r, debug.Stack())
					} else {
						saneList = append(saneList, info)
					}
				}()
				for _, f := range pkg.Syntax {
					ctx.SetFileInfo(getFilename(fset, f), f)
					_ = c.Check(f)
				}
			}
		})
	}

	return saneList
}

// IntegrationTest specifies integration test options.
type IntegrationTest struct {
	// Packages list which packages tested linter should import.
	Packages []string

	// Dir specifies a path to integration tests.
	Dir string
}

// TestIntegration runs linter integration tests using default options.
func TestIntegration(t *testing.T) {
	defaultIntegrationTest().Run(t)
}

// TestCheckers runs end2end tests over all registered checkers using default options.
//
// TODO(Quasilyte): document default options.
// TODO(Quasilyte): make it possible to run tests with different options.
func TestCheckers(t *testing.T) {
	for _, info := range saneCheckersList(t) {
		t.Run(info.Name, func(t *testing.T) {
			pkgPath := "./testdata/" + info.Name

			fset := token.NewFileSet()
			pkgs := newPackages(t, pkgPath, fset)
			for _, pkg := range pkgs {
				ctx := &lintpack.Context{
					SizesInfo: sizes,
					FileSet:   fset,
					TypesInfo: pkg.TypesInfo,
					Pkg:       pkg.Types,
				}
				c := lintpack.NewChecker(ctx, info)
				checkFiles(t, c, ctx, pkg)
			}
		})
	}
}

func checkFiles(t *testing.T, c *lintpack.Checker, ctx *lintpack.Context, pkg *packages.Package) {
	for _, f := range pkg.Syntax {
		filename := getFilename(ctx.FileSet, f)
		testFilename := filepath.Join("testdata", c.Info.Name, filename)
		goldenWarns := newGoldenFile(t, testFilename)

		stripDirectives(f)
		ctx.SetFileInfo(filename, f)

		for _, warn := range c.Check(f) {
			line := ctx.FileSet.Position(warn.Node.Pos()).Line

			if w := goldenWarns.find(line, warn.Text); w != nil {
				if w.matched {
					t.Errorf("%s:%d: multiple matches for %s",
						testFilename, line, w)
				}
				w.matched = true
			} else {
				t.Errorf("%s:%d: unexpected warn: %s",
					testFilename, line, warn.Text)
			}
		}

		goldenWarns.checkUnmatched(t, testFilename)
	}
}

// stripDirectives replaces "///" comments with empty single-line
// comments, so the checkers that inspect comments see ordinary
// comment groups (with extra newlines, but that's not important).
func stripDirectives(f *ast.File) {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, "/// ") {
				c.Text = "//"
			}
		}
	}
}

func getFilename(fset *token.FileSet, f *ast.File) string {
	// see https://github.com/golang/go/issues/24498
	return filepath.Base(fset.Position(f.Pos()).Filename)
}

func newPackages(t *testing.T, pattern string, fset *token.FileSet) []*packages.Package {
	cfg := packages.Config{
		Mode:  packages.LoadSyntax,
		Tests: true,
		Fset:  fset,
	}
	pkgs, err := loadPackages(&cfg, []string{pattern})
	if err != nil {
		t.Fatalf("load package: %v", err)
	}
	return pkgs
}

// TODO(Quasilyte): copied from check.go. Should it be added to pkgload?
func loadPackages(cfg *packages.Config, patterns []string) ([]*packages.Package, error) {
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}

	result := pkgs[:0]
	pkgload.VisitUnits(pkgs, func(u *pkgload.Unit) {
		if u.ExternalTest != nil {
			result = append(result, u.ExternalTest)
		}

		if u.Test != nil {
			// Prefer tests to the base package, if present.
			result = append(result, u.Test)
		} else {
			result = append(result, u.Base)
		}
	})
	return result, nil
}
