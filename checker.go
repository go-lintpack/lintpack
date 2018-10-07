package lintpack

import (
	"go/ast"

	"github.com/go-lintpack/lintpack/internal/astwalk"
)

type Checker struct {
	Name string

	Doc CheckerDoc

	Tags []string

	ctx CheckerContext

	proto abstractChecker

	Init func(ctx *Context)

	walker astwalk.FileWalker

	warnings []Warning
}

// Check runs rule checker over file f.
func (c *Checker) Check(f *ast.File) []Warning {
	c.ctx.warnings = c.ctx.warnings[:0]
	c.walker.WalkFile(f)
	return c.ctx.warnings
}
