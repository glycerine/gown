package gown

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type moveSite struct {
	name string
	line int
	col  int
}

type gwn001Checker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	consumed map[PlaceKey]moveSite
	errs     CheckerErrors
}

func checkGWN001(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	checker := &gwn001Checker{
		pkg:      pkg,
		caps:     caps,
		consumed: make(map[PlaceKey]moveSite),
	}
	checker.checkPackage()
	return checker.errs
}

func (checker *gwn001Checker) checkPackage() {
	for _, file := range checker.pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checker.checkFunc(fn)
		}
	}
}

func (checker *gwn001Checker) checkFunc(fn *ast.FuncDecl) {
	checker.consumed = make(map[PlaceKey]moveSite)
	checker.checkBlock(fn.Body)
}

func (checker *gwn001Checker) checkBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		checker.checkStmt(stmt)
	}
}

func (checker *gwn001Checker) checkStmt(stmt ast.Stmt) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		for _, rhs := range stmt.Rhs {
			checker.checkExprUses(rhs)
		}
	case *ast.BlockStmt:
		checker.checkBlock(stmt)
	case *ast.DeclStmt:
		checker.checkDecl(stmt.Decl)
	case *ast.DeferStmt:
		checker.checkExprUses(stmt.Call)
	case *ast.ExprStmt:
		checker.checkExprUses(stmt.X)
	case *ast.ForStmt:
		checker.checkStmt(stmt.Init)
		checker.checkExprUses(stmt.Cond)
		checker.checkBlock(stmt.Body)
		checker.checkStmt(stmt.Post)
	case *ast.GoStmt:
		checker.checkExprUses(stmt.Call)
	case *ast.IfStmt:
		checker.checkStmt(stmt.Init)
		checker.checkExprUses(stmt.Cond)
		checker.checkBlock(stmt.Body)
		checker.checkStmt(stmt.Else)
	case *ast.IncDecStmt:
		checker.checkExprUses(stmt.X)
	case *ast.RangeStmt:
		checker.checkExprUses(stmt.X)
		checker.checkBlock(stmt.Body)
	case *ast.ReturnStmt:
		for _, result := range stmt.Results {
			checker.checkExprUses(result)
		}
	case *ast.SelectStmt:
		checker.checkBlock(stmt.Body)
	case *ast.SendStmt:
		checker.checkExprUses(stmt.Chan)
		checker.checkExprUses(stmt.Value)
		checker.recordIsoSend(stmt)
	case *ast.SwitchStmt:
		checker.checkStmt(stmt.Init)
		checker.checkExprUses(stmt.Tag)
		checker.checkBlock(stmt.Body)
	case *ast.TypeSwitchStmt:
		checker.checkStmt(stmt.Init)
		checker.checkStmt(stmt.Assign)
		checker.checkBlock(stmt.Body)
	}
}

func (checker *gwn001Checker) checkDecl(decl ast.Decl) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, value := range valueSpec.Values {
			checker.checkExprUses(value)
		}
	}
}

func (checker *gwn001Checker) checkExprUses(expr ast.Expr) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		use, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		place, ok := checker.caps.PlaceForExpr(use)
		if !ok {
			return true
		}
		consumed, ok := checker.consumed[place.Key()]
		if !ok {
			return true
		}
		checker.reportUseAfterMove(use, consumed)
		return false
	})
}

func (checker *gwn001Checker) reportUseAfterMove(use ast.Expr, consumed moveSite) {
	pos := checker.pkg.Fset.Position(use.Pos())
	checker.errs = append(checker.errs, CheckerError{
		Code:    GWN001,
		Path:    gownSourcePath(pos.Filename),
		Offset:  pos.Offset,
		Line:    pos.Line,
		Col:     pos.Column,
		Message: fmt.Sprintf("use of moved \\iso value %q after send at %d:%d", consumed.name, consumed.line, consumed.col),
	})
}

func (checker *gwn001Checker) recordIsoSend(stmt *ast.SendStmt) {
	binding, ok := checker.caps.SendBinding(stmt)
	if !ok || binding.ChanElemCap != CapIso || binding.ValueCap != CapIso {
		return
	}
	key := binding.Value.Key()
	if key.Root == nil {
		return
	}
	checker.consumed[key] = moveSite{
		name: key.Root.Name(),
		line: binding.Line,
		col:  binding.Col,
	}
}
