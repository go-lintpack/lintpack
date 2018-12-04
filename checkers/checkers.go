package checkers

import (
	"go/ast"

	"github.com/go-lintpack/lintpack"
	"github.com/go-lintpack/lintpack/astwalk"
	"github.com/go-toolsmith/astfmt"
)

var collection = &lintpack.CheckerCollection{
	URL: "https://github.com/go-lintpack/lintpack",
}

func init() {
	var info lintpack.CheckerInfo
	info.Name = "panicNil"
	info.Tags = []string{"diagnostic"}
	info.Params = lintpack.CheckerParams{
		"skipNilEfaceLit": {
			Value: false,
			Usage: "whether to ignore interface{}(nil) arguments",
		},
	}
	info.Summary = "Detects panic(nil) calls"
	info.Details = "Such panic calls are hard to handle during recover."
	info.Before = `panic(nil)`
	info.After = `panic("something meaningful")`

	collection.AddChecker(&info, func(ctx *lintpack.CheckerContext) lintpack.FileWalker {
		c := &panicNilChecker{ctx: ctx}
		c.skipNilEfaceLit = info.Params.Bool("skipNilEfaceLit")
		return astwalk.WalkerForExpr(c)
	})
}

type panicNilChecker struct {
	astwalk.WalkHandler
	ctx             *lintpack.CheckerContext
	skipNilEfaceLit bool
}

func (c *panicNilChecker) VisitExpr(expr ast.Expr) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return
	}

	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "panic" {
		return
	}

	switch astfmt.Sprint(call.Args[0]) {
	case "nil":
		c.warn(expr)
	case "interface{}(nil)":
		if !c.skipNilEfaceLit {
			c.warn(expr)
		}
	}
}

func (c *panicNilChecker) warn(cause ast.Node) {
	c.ctx.Warn(cause, "%s calls are discouraged", cause)
}
