package check

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/go-lintpack/lintpack"
	"github.com/go-lintpack/lintpack/linter/lintmain/internal/hotload"
	"github.com/go-toolsmith/pkgload"
	"github.com/logrusorgru/aurora"
	"golang.org/x/tools/go/packages"
)

// Main implements sub-command entry point.
func Main() {
	var l linter
	l.infoList = lintpack.GetCheckersInfo()

	steps := []struct {
		name string
		fn   func() error
	}{
		{"load plugin", l.loadPlugin},
		{"bind checker params", l.bindCheckerParams},
		{"bind default enabled list", l.bindDefaultEnabledList},
		{"parse args", l.parseArgs},
		{"assign checker params", l.assignCheckerParams},
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

	fset *token.FileSet

	loadedPackages []*packages.Package

	infoList []*lintpack.CheckerInfo

	checkers []*lintpack.Checker

	packages []string

	foundIssues bool

	checkerParams boundCheckerParams

	filters struct {
		enableAll       bool
		enable          []string
		disable         []string
		defaultCheckers []string
	}

	workDir string

	exitCode           int
	checkTests         bool
	checkGenerated     bool
	shorterErrLocation bool
	coloredOutput      bool
	verbose            bool
}

func (l *linter) exit() error {
	if l.foundIssues {
		os.Exit(l.exitCode)
	}
	return nil
}

func (l *linter) runCheckers() error {
	for _, pkg := range l.loadedPackages {
		if l.verbose {
			log.Printf("\tdebug: checking %q package (%d files)",
				pkg.String(), len(pkg.Syntax))
		}
		l.checkPackage(pkg)
	}

	return nil
}

func (l *linter) checkPackage(pkg *packages.Package) {
	l.ctx.SetPackageInfo(pkg.TypesInfo, pkg.Types)
	for _, f := range pkg.Syntax {
		filename := l.getFilename(f)
		if !l.checkTests && strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if !l.checkGenerated && l.isGenerated(f) {
			continue
		}
		l.ctx.SetFileInfo(filename, f)
		l.checkFile(f)
	}
}

func (l *linter) checkFile(f *ast.File) {
	warnings := make([][]lintpack.Warning, len(l.checkers))

	var wg sync.WaitGroup
	wg.Add(len(l.checkers))
	for i, c := range l.checkers {
		// All checkers are expected to use *lint.Context
		// as read-only structure, so no copying is required.
		go func(i int, c *lintpack.Checker) {
			defer func() {
				wg.Done()
				// Checker signals unexpected error with panic(error).
				r := recover()
				if r == nil {
					return // There were no panic
				}
				if err, ok := r.(error); ok {
					log.Printf("%s: error: %v\n", c.Info.Name, err)
					panic(err)
				} else {
					// Some other kind of run-time panic.
					// Undo the recover and resume panic.
					panic(r)
				}
			}()

			for _, warn := range c.Check(f) {
				warnings[i] = append(warnings[i], warn)
			}
		}(i, c)
	}
	wg.Wait()

	for i, c := range l.checkers {
		for _, warn := range warnings[i] {
			l.foundIssues = true
			loc := l.ctx.FileSet.Position(warn.Node.Pos()).String()
			if l.shorterErrLocation {
				loc = l.shortenLocation(loc)
			}
			printWarning(l, c.Info.Name, loc, warn.Text)
		}
	}

}

func (l *linter) initCheckers() error {
	parseKeys := func(keys []string, byName, byTag map[string]bool) {
		for _, key := range keys {
			if strings.HasPrefix(key, "#") {
				byTag[key[len("#"):]] = true
			} else {
				byName[key] = true
			}
		}
	}

	enabledByName := make(map[string]bool)
	enabledTags := make(map[string]bool)
	parseKeys(l.filters.enable, enabledByName, enabledTags)
	disabledByName := make(map[string]bool)
	disabledTags := make(map[string]bool)
	parseKeys(l.filters.disable, disabledByName, disabledTags)

	enabledByTag := func(info *lintpack.CheckerInfo) bool {
		for _, tag := range info.Tags {
			if enabledTags[tag] {
				return true
			}
		}
		return false
	}
	disabledByTag := func(info *lintpack.CheckerInfo) string {
		for _, tag := range info.Tags {
			if disabledTags[tag] {
				return tag
			}
		}
		return ""
	}

	for _, info := range l.infoList {
		enabled := l.filters.enableAll ||
			enabledByName[info.Name] ||
			enabledByTag(info)
		notice := ""

		switch {
		case !enabled:
			notice = "not enabled by name or tag (-enable)"
		case disabledByName[info.Name]:
			enabled = false
			notice = "disabled by name (-disable)"
		default:
			if tag := disabledByTag(info); tag != "" {
				enabled = false
				notice = fmt.Sprintf("disabled by %q tag (-disable)", tag)
			}
		}

		if l.verbose && !enabled {
			log.Printf("\tdebug: %s: %s", info.Name, notice)
		}
		if enabled {
			l.checkers = append(l.checkers, lintpack.NewChecker(l.ctx, info))
		}
	}
	if l.verbose {
		for _, c := range l.checkers {
			log.Printf("\tdebug: %s is enabled", c.Info.Name)
		}
	}

	if len(l.checkers) == 0 {
		return errors.New("empty checkers set selected")
	}
	return nil
}

func (l *linter) loadProgram() error {
	sizes := types.SizesFor("gc", runtime.GOARCH)
	if sizes == nil {
		return fmt.Errorf("can't find sizes info for %s", runtime.GOARCH)
	}

	l.fset = token.NewFileSet()
	cfg := packages.Config{
		Mode:  packages.LoadSyntax,
		Tests: true,
		Fset:  l.fset,
	}
	pkgs, err := loadPackages(&cfg, l.packages)
	if err != nil {
		log.Fatalf("load packages: %v", err)
	}
	sort.SliceStable(pkgs, func(i, j int) bool {
		return pkgs[i].PkgPath < pkgs[j].PkgPath
	})

	l.loadedPackages = pkgs
	l.ctx = lintpack.NewContext(l.fset, sizes)

	return nil
}

func (l *linter) loadPlugin() error {
	const pluginFilename = "lintpack-plugin.so"
	if _, err := os.Stat(pluginFilename); os.IsNotExist(err) {
		return nil
	}
	infoList, err := hotload.CheckersFromDylib(l.infoList, pluginFilename)
	l.infoList = infoList
	return err
}

type boundCheckerParams struct {
	ints    map[string]*int
	bools   map[string]*bool
	strings map[string]*string
}

// bindCheckerParams registers command-line flags for every checker parameter.
func (l *linter) bindCheckerParams() error {
	intParams := make(map[string]*int)
	boolParams := make(map[string]*bool)
	stringParams := make(map[string]*string)

	for _, info := range l.infoList {
		for pname, param := range info.Params {
			key := l.checkerParamKey(info, pname)
			switch v := param.Value.(type) {
			case int:
				intParams[key] = flag.Int(key, v, param.Usage)
			case bool:
				boolParams[key] = flag.Bool(key, v, param.Usage)
			case string:
				stringParams[key] = flag.String(key, v, param.Usage)
			default:
				panic("unreachable") // Checked in AddChecker
			}
		}
	}

	l.checkerParams.ints = intParams
	l.checkerParams.bools = boolParams
	l.checkerParams.strings = stringParams

	return nil
}

func (l *linter) checkerParamKey(info *lintpack.CheckerInfo, pname string) string {
	return "@" + info.Name + "." + pname
}

// bindDefaultEnabledList calculates the default value for -enable param.
func (l *linter) bindDefaultEnabledList() error {
	var enabled []string
	for _, info := range l.infoList {
		enable := !info.HasTag("experimental") &&
			!info.HasTag("opinionated") &&
			!info.HasTag("performance")
		if enable {
			enabled = append(enabled, info.Name)
		}
	}
	l.filters.defaultCheckers = enabled
	return nil
}

func (l *linter) parseArgs() error {
	flag.BoolVar(&l.filters.enableAll, "enableAll", false,
		`identical to -enable with all checkers listed. If true, -enable is ignored`)
	enable := flag.String("enable", strings.Join(l.filters.defaultCheckers, ","),
		`comma-separated list of enabled checkers. Can include #tags`)
	disable := flag.String("disable", "",
		`comma-separated list of checkers to be disabled. Can include #tags`)
	flag.IntVar(&l.exitCode, "exitCode", 1,
		`exit code to be used when lint issues are found`)
	flag.BoolVar(&l.checkTests, "checkTests", true,
		`whether to check test files`)
	flag.BoolVar(&l.shorterErrLocation, `shorterErrLocation`, true,
		`whether to replace error location prefix with $GOROOT and $GOPATH`)
	flag.BoolVar(&l.coloredOutput, `coloredOutput`, false,
		`whether to use colored output`)
	flag.BoolVar(&l.verbose, "v", false,
		`whether to print output useful during linter debugging`)

	flag.Parse()

	l.packages = flag.Args()
	l.filters.enable = strings.Split(*enable, ",")
	l.filters.disable = strings.Split(*disable, ",")

	if l.shorterErrLocation {
		wd, err := os.Getwd()
		if err != nil {
			log.Printf("getwd: %v", err)
		}
		l.workDir = wd
	}

	return nil
}

// assignCheckerParams initializes checker parameter values using
// values that are coming from the command-line arguments.
func (l *linter) assignCheckerParams() error {
	intParams := l.checkerParams.ints
	boolParams := l.checkerParams.bools
	stringParams := l.checkerParams.strings

	for _, info := range l.infoList {
		for pname, param := range info.Params {
			key := l.checkerParamKey(info, pname)
			switch param.Value.(type) {
			case int:
				info.Params[pname].Value = *intParams[key]
			case bool:
				info.Params[pname].Value = *boolParams[key]
			case string:
				info.Params[pname].Value = *stringParams[key]
			default:
				panic("unreachable") // Checked in AddChecker
			}
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
	return filepath.Base(l.fset.Position(f.Pos()).Filename)
}

func (l *linter) shortenLocation(loc string) string {
	// If possible, construct relative path.
	relLoc := loc
	if l.workDir != "" {
		relLoc = strings.Replace(loc, l.workDir, ".", 1)
	}

	switch {
	case strings.HasPrefix(loc, build.Default.GOPATH):
		loc = strings.Replace(loc, build.Default.GOPATH, "$GOPATH", 1)
	case strings.HasPrefix(loc, build.Default.GOROOT):
		loc = strings.Replace(loc, build.Default.GOROOT, "$GOROOT", 1)
	}

	// Return the representation that is shorter.
	if len(relLoc) < len(loc) {
		return relLoc
	}
	return loc
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
