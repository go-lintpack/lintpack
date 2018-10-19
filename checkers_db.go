package lintpack

import (
	"go/ast"
	"regexp"
	"strings"

	"github.com/go-toolsmith/astfmt"
)

// Checkers is a list of registered checkers.
// Registration should be done with AddChecker function.
//
// TODO(quasilyte): can this be done without exported global var?
var Checkers []*Checker

// FileWalker is an interface every checker should implement.
//
// The WalkFile method is executed for every Go file inside the
// package that is being checked.
type FileWalker interface {
	WalkFile(*ast.File)
}

var checkerNameRE = regexp.MustCompile(`(\w+)Checker$`)

// AddChecker registers a new checker into a checker pool.
// constructor is used to create a new checker instance.
//
// If checker is never needed, for example if it is disabled,
// constructor will not be called.
func AddChecker(info *CheckerInfo, constructor func(*CheckerContext) FileWalker) {
	trimDocumentation := func(d *CheckerInfo) {
		fields := []*string{
			&d.Summary,
			&d.Details,
			&d.Before,
			&d.After,
			&d.Note,
		}
		for _, f := range fields {
			*f = strings.TrimSpace(*f)
		}
	}
	validateDocumentation := func(d *CheckerInfo) {
		// TODO(quasilyte): validate documentation.
	}

	checker := &Checker{
		Info: info,
	}
	trimDocumentation(checker.Info)
	validateDocumentation(checker.Info)
	checker.Init = func(ctx *Context) {
		checker.ctx = CheckerContext{
			Context: ctx,
			printer: astfmt.NewPrinter(ctx.FileSet),
		}
		checker.fileWalker = constructor(&checker.ctx)
	}

	Checkers = append(Checkers, checker)
}
