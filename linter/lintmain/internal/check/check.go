package check

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/go-lintpack/lintpack"
	"github.com/logrusorgru/aurora"
	"golang.org/x/tools/go/loader"
)

// Main implements sub-command entry point.
func Main() {
	var l linter

	steps := []struct {
		name string
		fn   func() error
	}{
		{"parse args", l.parseArgs},
		{"load program", l.loadProgram},
		{"init checkers", l.initCheckers},
		{"run checkers", l.runCheckers},
		{"exit if found issues", l.exit},
	}

	for _, step := range steps {
		if err := step.fn(); err != nil {
			log.Fatalf("%s: %v", step.name, err)
		}
	}
}

type linter struct {
	ctx *lintpack.Context

	prog *loader.Program

	checkers []*lintpack.Checker

	packages []string

	foundIssues bool

	exitCode           int
	checkTests         bool
	checkGenerated     bool
	shorterErrLocation bool
	coloredOutput      bool
}

func (l *linter) exit() error {
	if l.foundIssues {
		os.Exit(l.exitCode)
	}
	return nil
}

func (l *linter) runCheckers() error {
	pkgInfoMap := make(map[string]*loader.PackageInfo)
	for _, pkgInfo := range l.prog.AllPackages {
		pkgInfoMap[pkgInfo.Pkg.Path()] = pkgInfo
	}
	for _, pkgPath := range l.packages {
		pkgInfo := pkgInfoMap[pkgPath]
		if pkgInfo == nil || !pkgInfo.TransitivelyErrorFree {
			log.Fatalf("%s package is not properly loaded", pkgPath)
		}
		// Check the package itself.
		l.checkPackage(pkgPath, pkgInfo)
		// Check package external test (if any).
		pkgInfo = pkgInfoMap[pkgPath+"_test"]
		if pkgInfo != nil {
			l.checkPackage(pkgPath+"_test", pkgInfo)
		}
	}

	return nil
}

func (l *linter) checkPackage(pkgPath string, pkgInfo *loader.PackageInfo) {
	l.ctx.TypesInfo = &pkgInfo.Info
	l.ctx.Pkg = pkgInfo.Pkg
	for _, f := range pkgInfo.Files {
		filename := l.getFilename(f)
		if !l.checkTests && strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if !l.checkGenerated && l.isGenerated(f) {
			continue
		}
		l.ctx.Filename = filename
		l.checkFile(f)
	}
}

func (l *linter) checkFile(f *ast.File) {
	var wg sync.WaitGroup
	wg.Add(len(l.checkers))
	for _, c := range l.checkers {
		// All checkers are expected to use *lint.Context
		// as read-only structure, so no copying is required.
		go func(c *lintpack.Checker) {
			defer func() {
				wg.Done()
				// Checker signals unexpected error with panic(error).
				r := recover()
				if r == nil {
					return // There were no panic
				}
				if err, ok := r.(error); ok {
					log.Printf("%s: error: %v\n", c.Name, err)
					panic(err)
				} else {
					// Some other kind of run-time panic.
					// Undo the recover and resume panic.
					panic(r)
				}
			}()

			for _, warn := range c.Check(f) {
				l.foundIssues = true
				loc := l.ctx.FileSet.Position(warn.Node.Pos()).String()
				if l.shorterErrLocation {
					loc = shortenLocation(loc)
				}

				printWarning(l, c.Name, loc, warn.Text)
			}
		}(c)
	}
	wg.Wait()
}

func (l *linter) initCheckers() error {
	for _, c := range l.checkers {
		c.Init(l.ctx)
	}
	return nil
}

func (l *linter) loadProgram() error {
	sizes := types.SizesFor("gc", runtime.GOARCH)
	if sizes == nil {
		return fmt.Errorf("can't find sizes info for %s", runtime.GOARCH)
	}

	conf := loader.Config{
		ParserMode: parser.ParseComments,
		TypeChecker: types.Config{
			Sizes: sizes,
		},
	}

	if _, err := conf.FromArgs(l.packages, true); err != nil {
		log.Fatalf("resolve packages: %v", err)
	}
	prog, err := conf.Load()
	if err != nil {
		log.Fatalf("load program: %v", err)
	}

	l.prog = prog
	l.ctx = &lintpack.Context{
		SizesInfo: sizes,
		FileSet:   prog.Fset,
	}

	return nil
}

func (l *linter) parseArgs() error {
	disableTags := flag.String("disableTags", `^experimental$`,
		`regexp that excludes checkers that have matching tag`)
	disable := flag.String("disable", `<none>`,
		`regexp that disables unwanted checks`)
	enable := flag.String("enable", `.*`,
		`regexp that selects what checkers are being run. Applied after all other filters`)
	flag.IntVar(&l.exitCode, "exitCode", 1,
		`exit code to be used when lint issues are found`)
	flag.BoolVar(&l.checkTests, "checkTests", true,
		`whether to check test files`)
	flag.BoolVar(&l.shorterErrLocation, `shorterErrLocation`, true,
		`whether to replace error location prefix with $GOROOT and $GOPATH`)
	flag.BoolVar(&l.coloredOutput, `coloredOutput`, true,
		`whether to use colored output`)

	flag.Parse()

	l.packages = flag.Args()
	disableTagsRE, err := regexp.Compile(*disableTags)
	if err != nil {
		return fmt.Errorf("-disableTags: %v", err)
	}
	disableRE, err := regexp.Compile(*disable)
	if err != nil {
		return fmt.Errorf("-disable: %v", err)
	}
	enableRE, err := regexp.Compile(*enable)
	if err != nil {
		return fmt.Errorf("-enable: %v", err)
	}

	disabledByTags := func(c *lintpack.Checker) bool {
		for _, tag := range c.Tags {
			if disableTagsRE.MatchString(tag) {
				return true
			}
		}
		return false
	}
	for _, c := range lintpack.Checkers {
		if disabledByTags(c) || disableRE.MatchString(c.Name) {
			continue
		}
		if enableRE.MatchString(c.Name) {
			l.checkers = append(l.checkers, c)
		}
	}

	return nil
}

var generatedFileCommentRE = regexp.MustCompile("Code generated .* DO NOT EDIT.")

func (l *linter) isGenerated(f *ast.File) bool {
	return len(f.Comments) != 0 &&
		generatedFileCommentRE.MatchString(f.Comments[0].Text())
}

func (l *linter) getFilename(f *ast.File) string {
	// See https://github.com/golang/go/issues/24498.
	return filepath.Base(l.prog.Fset.Position(f.Pos()).Filename)
}

func shortenLocation(loc string) string {
	switch {
	case strings.HasPrefix(loc, build.Default.GOPATH):
		return strings.Replace(loc, build.Default.GOPATH, "$GOPATH", 1)
	case strings.HasPrefix(loc, build.Default.GOROOT):
		return strings.Replace(loc, build.Default.GOROOT, "$GOROOT", 1)
	default:
		return loc
	}
}

func printWarning(l *linter, rule, loc, warn string) {
	switch {
	case l.coloredOutput:
		log.Printf("%v: %v: %v\n",
			aurora.Magenta(aurora.Bold(loc)),
			aurora.Red(rule),
			warn)

	default:
		log.Printf("%s: %s: %s\n", loc, rule, warn)
	}
}
