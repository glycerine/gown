package gown

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type moveSite struct {
	name string
	kind string
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
			checker.checkExpr(rhs)
		}
		checker.recordIsoAssignMoves(stmt)
	case *ast.BlockStmt:
		checker.checkBlock(stmt)
	case *ast.DeclStmt:
		checker.checkDecl(stmt.Decl)
	case *ast.DeferStmt:
		checker.checkExpr(stmt.Call)
	case *ast.ExprStmt:
		checker.checkExpr(stmt.X)
	case *ast.ForStmt:
		checker.checkStmt(stmt.Init)
		checker.checkExpr(stmt.Cond)
		checker.checkBlock(stmt.Body)
		checker.checkStmt(stmt.Post)
	case *ast.GoStmt:
		checker.checkExpr(stmt.Call)
		checker.recordIsoGoCaptures(stmt)
	case *ast.IfStmt:
		checker.checkStmt(stmt.Init)
		checker.checkExpr(stmt.Cond)
		checker.checkBlock(stmt.Body)
		checker.checkStmt(stmt.Else)
	case *ast.IncDecStmt:
		checker.checkExpr(stmt.X)
	case *ast.RangeStmt:
		checker.checkExpr(stmt.X)
		checker.checkBlock(stmt.Body)
	case *ast.ReturnStmt:
		for _, result := range stmt.Results {
			checker.checkExpr(result)
		}
	case *ast.SelectStmt:
		checker.checkBlock(stmt.Body)
	case *ast.SendStmt:
		checker.checkExpr(stmt.Chan)
		checker.checkExpr(stmt.Value)
		checker.recordIsoSend(stmt)
	case *ast.SwitchStmt:
		checker.checkStmt(stmt.Init)
		checker.checkExpr(stmt.Tag)
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
			checker.checkExpr(value)
		}
	}
}

func (checker *gwn001Checker) checkExpr(expr ast.Expr) {
	checker.checkExprUses(expr)
	checker.recordIsoCallMoves(expr)
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
		consumed, ok := checker.consumed[place.RegionKey()]
		if !ok {
			return true
		}
		checker.reportUseAfterMove(use, consumed)
		return false
	})
}

func (checker *gwn001Checker) reportUseAfterMove(use ast.Expr, consumed moveSite) {
	checker.errs = append(checker.errs, newCheckerErrorAtNode(
		checker.pkg,
		GWN001,
		use,
		fmt.Sprintf("use of moved \\iso value %q after %s at %d:%d", consumed.name, consumed.kind, consumed.line, consumed.col),
	))
}

func (checker *gwn001Checker) recordIsoSend(stmt *ast.SendStmt) {
	binding, ok := checker.caps.SendBinding(stmt)
	if !ok || !binding.IsIsoMove() {
		return
	}
	key := binding.ValueKey()
	if key.Root == nil {
		return
	}
	checker.consumed[key] = moveSite{
		name: key.Root.Name(),
		kind: "send",
		line: binding.Line,
		col:  binding.Col,
	}
}

func (checker *gwn001Checker) recordIsoCallMoves(expr ast.Expr) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		checker.recordIsoCallMove(call)
		return true
	})
}

func (checker *gwn001Checker) recordIsoCallMove(call *ast.CallExpr) {
	binding, ok := checker.caps.CallBinding(call)
	if !ok {
		return
	}
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapIso || i >= len(binding.ArgPlaces) {
			continue
		}
		key := binding.ArgPlaces[i].RegionKey()
		if key.Root == nil || checker.caps.ObjectCap(key.Root) != CapIso {
			continue
		}
		checker.consumed[key] = moveSite{
			name: key.Root.Name(),
			kind: "call",
			line: binding.Line,
			col:  binding.Col,
		}
	}
}

func (checker *gwn001Checker) recordIsoGoCaptures(stmt *ast.GoStmt) {
	pos := checker.pkg.Fset.Position(stmt.Go)
	for _, capture := range goClosureCaptures(checker.pkg, checker.caps, stmt) {
		if capture.Cap != CapIso {
			continue
		}
		key := capture.Place.RegionKey()
		if key.Root == nil {
			continue
		}
		checker.consumed[key] = moveSite{
			name: key.Root.Name(),
			kind: "go",
			line: pos.Line,
			col:  pos.Column,
		}
	}
}

func (checker *gwn001Checker) recordIsoAssignMoves(stmt *ast.AssignStmt) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		dst, ok := checker.caps.PlaceForExpr(lhs)
		if !ok || dst.Root == nil {
			continue
		}
		src, ok := checker.caps.PlaceForExpr(stmt.Rhs[i])
		if !ok || src.Root == nil || src.Root == dst.Root {
			continue
		}
		if checker.caps.ObjectCap(src.Root) != CapIso {
			continue
		}
		dstKey := dst.RegionKey()
		srcKey := src.RegionKey()
		delete(checker.consumed, dstKey)
		pos := checker.pkg.Fset.Position(stmt.Rhs[i].Pos())
		checker.consumed[srcKey] = moveSite{
			name: src.Root.Name(),
			kind: "assignment",
			line: pos.Line,
			col:  pos.Column,
		}
	}
}
