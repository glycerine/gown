package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkClosureEscapesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaClosureEscapeChecker{
		pkg:      pkg,
		caps:     caps,
		places:   buildSSAPlaceIndex(pkg, ssaPkg, caps),
		reported: make(map[string]bool),
	}
	checker.checkSourceClosures()
	for _, fn := range collectSSAFunctions(ssaPkg) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaClosureEscapeChecker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaClosureEscapeChecker) checkSourceClosures() {
	for _, file := range checker.pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checker.checkSourceFuncBody(fn.Body)
		}
	}
}

func (checker *ssaClosureEscapeChecker) checkSourceFuncBody(body *ast.BlockStmt) {
	aliases := checker.sourceClosureAliases(body)
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			checker.checkSourceReturn(n, aliases)
		case *ast.AssignStmt:
			checker.checkSourceAssign(n, aliases)
		case *ast.CallExpr:
			checker.checkSourceCall(n, aliases)
		}
		return true
	})
}

func (checker *ssaClosureEscapeChecker) sourceClosureAliases(body *ast.BlockStmt) map[types.Object]*ast.FuncLit {
	aliases := make(map[types.Object]*ast.FuncLit)
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ValueSpec:
			if len(n.Names) != len(n.Values) {
				return true
			}
			for i, name := range n.Names {
				lit, ok := sourceFuncLit(n.Values[i])
				if !ok {
					continue
				}
				if obj := objectForIdent(checker.pkg, name); obj != nil {
					aliases[obj] = lit
				}
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, lhs := range n.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok {
					continue
				}
				lit, ok := sourceFuncLit(n.Rhs[i])
				if !ok {
					continue
				}
				if obj := objectForIdent(checker.pkg, id); obj != nil {
					aliases[obj] = lit
				}
			}
		}
		return true
	})
	return aliases
}

func (checker *ssaClosureEscapeChecker) checkSourceReturn(ret *ast.ReturnStmt, aliases map[types.Object]*ast.FuncLit) {
	for _, result := range ret.Results {
		lit, ok := checker.sourceClosureForExpr(result, aliases)
		if !ok {
			continue
		}
		checker.reportSourceBorrowCaptures(lit, result, GWN007, "cannot return closure capturing %s borrow %q")
	}
}

func (checker *ssaClosureEscapeChecker) checkSourceAssign(stmt *ast.AssignStmt, aliases map[types.Object]*ast.FuncLit) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, rhs := range stmt.Rhs {
		lit, ok := checker.sourceClosureForExpr(rhs, aliases)
		if !ok || !storeTargetEscapes(checker.pkg, stmt.Lhs[i]) {
			continue
		}
		checker.reportSourceBorrowCaptures(lit, rhs, GWN006, "cannot store closure capturing %s borrow %q into escaping location")
	}
}

func (checker *ssaClosureEscapeChecker) checkSourceCall(call *ast.CallExpr, aliases map[types.Object]*ast.FuncLit) {
	if _, ok := checker.caps.CallBinding(call); ok {
		return
	}
	if callee := callCallee(checker.pkg, call); callee == nil {
		return
	}
	for _, arg := range call.Args {
		lit, ok := checker.sourceClosureForExpr(arg, aliases)
		if !ok {
			continue
		}
		checker.reportSourceBorrowCaptures(lit, arg, GWN008, "cannot pass closure capturing %s borrow %q to untracked function")
	}
}

func (checker *ssaClosureEscapeChecker) sourceClosureForExpr(expr ast.Expr, aliases map[types.Object]*ast.FuncLit) (*ast.FuncLit, bool) {
	if lit, ok := sourceFuncLit(expr); ok {
		return lit, true
	}
	id, ok := expr.(*ast.Ident)
	if !ok || aliases == nil {
		return nil, false
	}
	obj := objectForIdent(checker.pkg, id)
	if obj == nil {
		return nil, false
	}
	lit, ok := aliases[obj]
	return lit, ok
}

func (checker *ssaClosureEscapeChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			switch instr := instr.(type) {
			case *ssa.Return:
				checker.checkReturn(instr)
			case *ssa.Store:
				checker.checkStore(instr)
			}
		}
	}
}

func (checker *ssaClosureEscapeChecker) checkReturn(ret *ssa.Return) {
	for _, result := range ret.Results {
		closure, ok := result.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		checker.reportBorrowCaptures(closure, ret, GWN007, "cannot return closure capturing %s borrow %q")
	}
}

func (checker *ssaClosureEscapeChecker) checkStore(store *ssa.Store) {
	closure, ok := store.Val.(*ssa.MakeClosure)
	if !ok {
		return
	}
	target, ok := checker.places.PlaceForValue(store.Addr)
	if !ok || target.Root == nil || !ssaStoreTargetEscapes(checker.pkg, target) {
		return
	}
	checker.reportBorrowCaptures(closure, store, GWN006, "cannot store closure capturing %s borrow %q into escaping location")
}

func (checker *ssaClosureEscapeChecker) reportBorrowCaptures(closure *ssa.MakeClosure, instr ssa.Instruction, code CheckerErrorCode, format string) {
	for _, binding := range closure.Bindings {
		place, ok := checker.places.PlaceForValue(binding)
		if !ok || place.Root == nil {
			continue
		}
		cap := capForSSAPlace(checker.caps, place)
		if !closureCaptureEscapes(cap) {
			continue
		}
		checker.reportCheckerError(newCheckerErrorAtPosition(
			code,
			checker.pkg.Fset.Position(instr.Pos()),
			fmt.Sprintf(format, cap, place.Root.Name()),
		))
	}
}

func (checker *ssaClosureEscapeChecker) reportSourceBorrowCaptures(lit *ast.FuncLit, pos ast.Node, code CheckerErrorCode, format string) {
	for _, capture := range sourceFuncLitBorrowCaptures(checker.pkg, checker.caps, lit) {
		checker.reportCheckerError(newCheckerErrorAtNode(
			checker.pkg,
			code,
			pos,
			fmt.Sprintf(format, capture.Cap, capture.Place.Root.Name()),
		))
	}
}

func (checker *ssaClosureEscapeChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}

func sourceFuncLit(expr ast.Expr) (*ast.FuncLit, bool) {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = paren.X
	}
	lit, ok := expr.(*ast.FuncLit)
	return lit, ok
}

func sourceFuncLitBorrowCaptures(pkg *packages.Package, caps *CapabilityIndex, lit *ast.FuncLit) []goCapture {
	if pkg == nil || caps == nil || lit == nil || lit.Body == nil {
		return nil
	}
	seen := make(map[PlaceKey]bool)
	var captures []goCapture
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := objectForIdent(pkg, id)
		if obj == nil || posInNode(obj.Pos(), lit) {
			return true
		}
		place, ok := caps.PlaceForExpr(id)
		if !ok || place.Root == nil {
			return true
		}
		cap := caps.ObjectCap(place.Root)
		if !closureCaptureEscapes(cap) {
			return true
		}
		key := place.Key()
		if seen[key] {
			return true
		}
		seen[key] = true
		captures = append(captures, goCapture{
			Ident: id,
			Place: place,
			Cap:   cap,
		})
		return true
	})
	return captures
}

func closureCaptureEscapes(cap Cap) bool {
	return cap == CapIso || cap == CapMub || cap == CapRob
}
