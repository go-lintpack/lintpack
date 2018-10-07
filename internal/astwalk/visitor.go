package astwalk

import (
	"go/ast"
)

// Visitor interfaces.
type (
	// FileVisitor visits every package file.
	FileVisitor interface {
		VisitFile(*ast.File)
	}

	// FuncDeclVisitor visits every top-level function declaration.
	FuncDeclVisitor interface {
		walkerEvents
		VisitFuncDecl(*ast.FuncDecl)
	}

	// ExprVisitor visits every expression inside AST file.
	ExprVisitor interface {
		walkerEvents
		VisitExpr(ast.Expr)
	}

	// LocalExprVisitor visits every expression inside function body.
	LocalExprVisitor interface {
		walkerEvents
		VisitLocalExpr(ast.Expr)
	}

	// StmtListVisitor visits every statement list inside function body.
	// This includes block statement bodies as well as implicit blocks
	// introduced by case clauses and alike.
	StmtListVisitor interface {
		walkerEvents
		VisitStmtList([]ast.Stmt)
	}

	// StmtVisitor visits every statement inside function body.
	StmtVisitor interface {
		walkerEvents
		VisitStmt(ast.Stmt)
	}

	// TypeExprVisitor visits every type describing expression.
	// It also traverses struct types and interface types to run
	// checker over their fields/method signatures.
	TypeExprVisitor interface {
		walkerEvents
		VisitTypeExpr(ast.Expr)
	}

	// LocalCommentVisitor visits every comment inside function body.
	LocalCommentVisitor interface {
		walkerEvents
		VisitLocalComment(*ast.CommentGroup)
	}

	// DocCommentVisitor visits every doc-comment.
	// Does not visit doc-comments for function-local definitions (types, etc).
	// Also does not visit package doc-comment (file-level doc-comments).
	DocCommentVisitor interface {
		walkerEvents
		VisitDocComment(*ast.CommentGroup)
	}
)

// walkerEvents describes common hooks available for most visitor types.
type walkerEvents interface {
	// EnterFunc is called for every function declaration that is about
	// to be traversed. If false is returned, function is not visited.
	EnterFunc(*ast.FuncDecl) bool
}
