package lintpack

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-lintpack/lintpack/internal/astwalk"
	"github.com/go-toolsmith/astfmt"
)

// Checkers is a list of registered checkers.
// Registration should be done with AddChecker function.
//
// TODO(quasilyte): can this be done without exported global var?
var Checkers []*Checker

// abstractChecker is a proxy interface to forward checkerBase
// embedding checker into addChecker.
//
// abstractChecker is implemented by checkerBase directly and completely,
// making any checker that embeds it a valid argument to addChecker.
//
// See checkerBase and its implementation of this interface for more info.
type abstractChecker interface {
	BindContext(*CheckerContext) // See CheckerBase.BindContext
	Init()                       // See CheckerBase.Init

	// InitDocumentation fills Documentation object associated with checker.
	// Mandatory fields are Summary, Before and After.
	// See other checkers implementation for examples.
	InitDocumentation(*CheckerDoc)
}

var checkerNameRE = regexp.MustCompile(`(\w+)Checker$`)

func AddChecker(proto abstractChecker) {
	trimDocumentation := func(d *CheckerDoc) {
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

	validateDocumentation := func(d *CheckerDoc) {
		// TODO(quasilyte): validate documentation.
	}

	checker := &Checker{}
	{
		typeName := reflect.ValueOf(proto).Type().String()
		m := checkerNameRE.FindStringSubmatch(typeName)
		if m == nil {
			panic(fmt.Sprintf("bad checker type name: %q", typeName))
		}
		checker.Name = m[1]
	}

	newFileWalker := func(ctx *Context) astwalk.FileWalker {
		// Infer proper AST traversing wrapper (walker).
		switch v := proto.(type) {
		case astwalk.FileVisitor:
			return astwalk.WalkerForFile(v)
		case astwalk.FuncDeclVisitor:
			return astwalk.WalkerForFuncDecl(v)
		case astwalk.ExprVisitor:
			return astwalk.WalkerForExpr(v)
		case astwalk.LocalExprVisitor:
			return astwalk.WalkerForLocalExpr(v)
		case astwalk.StmtListVisitor:
			return astwalk.WalkerForStmtList(v)
		case astwalk.StmtVisitor:
			return astwalk.WalkerForStmt(v)
		case astwalk.TypeExprVisitor:
			return astwalk.WalkerForTypeExpr(v, ctx.TypesInfo)
		case astwalk.LocalCommentVisitor:
			return astwalk.WalkerForLocalComment(v)
		case astwalk.DocCommentVisitor:
			return astwalk.WalkerForDocComment(v)
		default:
			panic(fmt.Sprintf("%T does not implement known visitor interface", proto))
		}
	}

	proto.InitDocumentation(&checker.Doc)
	trimDocumentation(&checker.Doc)
	validateDocumentation(&checker.Doc)

	checker.Init = func(ctx *Context) {
		checker.ctx = CheckerContext{
			Context: ctx,
			printer: astfmt.NewPrinter(ctx.FileSet),
		}
		proto.BindContext(&checker.ctx)
		proto.Init()
		checker.walker = newFileWalker(ctx)
	}

	Checkers = append(Checkers, checker)
}
