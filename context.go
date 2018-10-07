package lintpack

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/go-toolsmith/astfmt"
)

type Context struct {
	// TypesInfo carries parsed packages types information.
	TypesInfo *types.Info

	// SizesInfo carries alignment and type size information.
	// Arch-dependent.
	SizesInfo types.Sizes

	// FileSet is a file set that was used during the program loading.
	FileSet *token.FileSet

	// Pkg describes package that is being checked.
	Pkg *types.Package

	// Filename is a currently checked file name.
	Filename string
}

type CheckerContext struct {
	*Context

	// printer used to format warning text.
	printer *astfmt.Printer

	Params parameters

	warnings []Warning
}

// Warn adds a Warning to checker output.
func (ctx *CheckerContext) Warn(node ast.Node, format string, args ...interface{}) {
	ctx.warnings = append(ctx.warnings, Warning{
		Text: ctx.printer.Sprintf(format, args...),
		Node: node,
	})
}
