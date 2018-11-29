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
		disableTags *regexp.Regexp
		disable     *regexp.Regexp
		enableTags  *regexp.Regexp
		enable      *regexp.Regexp
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
	matchAnyTag := func(re *regexp.Regexp, info *lintpack.CheckerInfo) bool {
		for _, tag := range info.Tags {
			if re.MatchString(tag) {
				return true
			}
		}
		return false
	}
	disabledByTags := func(info *lintpack.CheckerInfo) bool {
		if len(info.Tags) == 0 {
			return false
		}
		return matchAnyTag(l.filters.disableTags, info)
	}
	enabledByTags := func(info *lintpack.CheckerInfo) bool {
		if len(info.Tags) == 0 {
			return true
		}
		return matchAnyTag(l.filters.enableTags, info)
	}

	for _, info := range l.infoList {
		enabled := false
		notice := ""

		switch {
		case !l.filters.enable.MatchString(info.Name):
			notice = "not enabled by name (-enable)"
		case !enabledByTags(info):
			notice = "not enabled by tags (-enableTags)"
		case l.filters.disable.MatchString(info.Name):
			notice = "disabled by name (-disable)"
		case disabledByTags(info):
			notice = "disabled by tags (-disableTags)"
		default:
			enabled = true
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

func (l *linter) parseArgs() error {
	disableTags := flag.String("disableTags", `^experimental$|^performance$|^opinionated$`,
		`regexp that excludes checkers that have matching tag`)
	disable := flag.String("disable", `<none>`,
		`regexp that disables unwanted checks`)
	enableTags := flag.String("enableTags", `.*`,
		`regexp that includes checkers that have matching tag`)
	enable := flag.String("enable", `.*`,
		`regexp that selects what checkers are being run. Applied after all other filters`)
	flag.IntVar(&l.exitCode, "exitCode", 1,
		`exit code to be used when lint issues are found`)
	flag.BoolVar(&l.checkTests, "checkTests", true,
		`whether to check test files`)
	flag.BoolVar(&l.shorterErrLocation, `shorterErrLocation`, true,
		`whether to replace error location prefix with $GOROOT and $GOPATH`)
	flag.BoolVar(&l.coloredOutput, `coloredOutput`, false,
		`whether to use colored output`)
	flag.BoolVar(&l.verbose, `verbose`, false,
		`whether to print output useful during linter debugging`)

	flag.Parse()

	var err error

	l.packages = flag.Args()
	l.filters.disableTags, err = regexp.Compile(*disableTags)
	if err != nil {
		return fmt.Errorf("-disableTags: %v", err)
	}
	l.filters.disable, err = regexp.Compile(*disable)
	if err != nil {
		return fmt.Errorf("-disable: %v", err)
	}
	l.filters.enableTags, err = regexp.Compile(*enableTags)
	if err != nil {
		return fmt.Errorf("-enableTags: %v", err)
	}
	l.filters.enable, err = regexp.Compile(*enable)
	if err != nil {
		return fmt.Errorf("-enable: %v", err)
	}

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
