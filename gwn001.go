package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type rootPlace struct {
	ident *ast.Ident
	obj   types.Object
}

type moveSite struct {
	name string
	line int
	col  int
}

type gwn001Checker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	consumed map[types.Object]moveSite
	errs     CheckerErrors
}

func checkGWN001(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	checker := &gwn001Checker{
		pkg:      pkg,
		caps:     caps,
		consumed: make(map[types.Object]moveSite),
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
	checker.consumed = make(map[types.Object]moveSite)
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
		place, ok := checker.rootUse(n)
		if !ok {
			return true
		}
		consumed, ok := checker.consumed[place.obj]
		if !ok {
			return true
		}
		checker.reportUseAfterMove(place, consumed)
		return false
	})
}

func (checker *gwn001Checker) reportUseAfterMove(place rootPlace, consumed moveSite) {
	pos := checker.pkg.Fset.Position(place.ident.Pos())
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
	sent, ok := checker.isoSendRoot(stmt)
	if !ok {
		return
	}
	pos := checker.pkg.Fset.Position(sent.ident.Pos())
	checker.consumed[sent.obj] = moveSite{
		name: sent.ident.Name,
		line: pos.Line,
		col:  pos.Column,
	}
}

func (checker *gwn001Checker) isoSendRoot(stmt *ast.SendStmt) (rootPlace, bool) {
	ch, ok := checker.rootPlaceForExpr(stmt.Chan)
	if !ok || checker.caps.ChanElemCap(ch.obj) != CapIso {
		return rootPlace{}, false
	}
	sent, ok := checker.rootPlaceForExpr(stmt.Value)
	if !ok || checker.caps.ObjectCap(sent.obj) != CapIso {
		return rootPlace{}, false
	}
	return sent, true
}

func (checker *gwn001Checker) rootUse(n ast.Node) (rootPlace, bool) {
	id, ok := n.(*ast.Ident)
	if !ok {
		return rootPlace{}, false
	}
	obj := checker.pkg.TypesInfo.Uses[id]
	if obj == nil {
		return rootPlace{}, false
	}
	return rootPlace{ident: id, obj: obj}, true
}

func (checker *gwn001Checker) rootPlaceForExpr(expr ast.Expr) (rootPlace, bool) {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return rootPlace{}, false
	}
	obj := checker.pkg.TypesInfo.Uses[id]
	if obj == nil {
		obj = checker.pkg.TypesInfo.Defs[id]
	}
	if obj == nil {
		return rootPlace{}, false
	}
	return rootPlace{ident: id, obj: obj}, true
}
