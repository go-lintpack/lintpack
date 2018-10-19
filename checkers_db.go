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

type FileWalker interface {
	WalkFile(*ast.File)
}

var checkerNameRE = regexp.MustCompile(`(\w+)Checker$`)

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
