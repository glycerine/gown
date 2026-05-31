package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkClosureEscapesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *OstampIndex) CheckerErrors {
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
	caps     *OstampIndex
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
	checker.checkSourceBlock(body, newClosureFlowEnv())
}

func appendSourceClosureAlias(lits []*ast.FuncLit, next *ast.FuncLit) []*ast.FuncLit {
	if next == nil {
		return lits
	}
	for _, lit := range lits {
		if lit == next {
			return lits
		}
	}
	return append(lits, next)
}

type closureFlowEnv map[types.Object][]*ast.FuncLit

func newClosureFlowEnv() closureFlowEnv {
	return make(closureFlowEnv)
}

func cloneClosureFlowEnv(env closureFlowEnv) closureFlowEnv {
	clone := newClosureFlowEnv()
	for obj, lits := range env {
		clone[obj] = append([]*ast.FuncLit(nil), lits...)
	}
	return clone
}

func mergeClosureFlowEnv(left, right closureFlowEnv) closureFlowEnv {
	merged := cloneClosureFlowEnv(left)
	for obj, lits := range right {
		for _, lit := range lits {
			merged[obj] = appendSourceClosureAlias(merged[obj], lit)
		}
	}
	return merged
}

func setClosureFlowValue(env closureFlowEnv, obj types.Object, lits []*ast.FuncLit) {
	if obj == nil {
		return
	}
	if len(lits) == 0 {
		delete(env, obj)
		return
	}
	env[obj] = append([]*ast.FuncLit(nil), lits...)
}

func (checker *ssaClosureEscapeChecker) checkSourceBlock(block *ast.BlockStmt, env closureFlowEnv) closureFlowEnv {
	if block == nil {
		return env
	}
	for _, stmt := range block.List {
		env = checker.checkSourceStmt(stmt, env)
	}
	return env
}

func (checker *ssaClosureEscapeChecker) checkSourceStmt(stmt ast.Stmt, env closureFlowEnv) closureFlowEnv {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		checker.checkSourceAssign(stmt, env)
	case *ast.BlockStmt:
		return checker.checkSourceBlock(stmt, env)
	case *ast.DeclStmt:
		checker.checkSourceDecl(stmt.Decl, env)
	case *ast.ExprStmt:
		checker.checkSourceExpr(stmt.X, env)
	case *ast.ForStmt:
		checker.checkSourceStmt(stmt.Init, env)
		checker.checkSourceExpr(stmt.Cond, env)
		bodyOut := checker.checkSourceBlock(stmt.Body, cloneClosureFlowEnv(env))
		env = mergeClosureFlowEnv(env, bodyOut)
		checker.checkSourceStmt(stmt.Post, env)
	case *ast.IfStmt:
		checker.checkSourceStmt(stmt.Init, env)
		checker.checkSourceExpr(stmt.Cond, env)
		thenEnv := checker.checkSourceBlock(stmt.Body, cloneClosureFlowEnv(env))
		elseEnv := cloneClosureFlowEnv(env)
		if stmt.Else != nil {
			elseEnv = checker.checkSourceStmt(stmt.Else, elseEnv)
		}
		env = mergeClosureFlowEnv(thenEnv, elseEnv)
	case *ast.ReturnStmt:
		checker.checkSourceReturn(stmt, env)
	case *ast.SendStmt:
		checker.checkSourceExpr(stmt.Chan, env)
		checker.checkSourceExpr(stmt.Value, env)
	case *ast.SwitchStmt:
		checker.checkSourceStmt(stmt.Init, env)
		checker.checkSourceExpr(stmt.Tag, env)
		env = checker.checkSourceCaseBlock(stmt.Body, env)
	case *ast.TypeSwitchStmt:
		checker.checkSourceStmt(stmt.Init, env)
		checker.checkSourceStmt(stmt.Assign, env)
		env = checker.checkSourceCaseBlock(stmt.Body, env)
	case *ast.RangeStmt:
		checker.checkSourceExpr(stmt.X, env)
		bodyOut := checker.checkSourceBlock(stmt.Body, cloneClosureFlowEnv(env))
		env = mergeClosureFlowEnv(env, bodyOut)
	}
	return env
}

func (checker *ssaClosureEscapeChecker) checkSourceCaseBlock(body *ast.BlockStmt, env closureFlowEnv) closureFlowEnv {
	if body == nil {
		return env
	}
	merged := cloneClosureFlowEnv(env)
	for _, stmt := range body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		caseEnv := cloneClosureFlowEnv(env)
		for _, expr := range clause.List {
			checker.checkSourceExpr(expr, caseEnv)
		}
		for _, bodyStmt := range clause.Body {
			caseEnv = checker.checkSourceStmt(bodyStmt, caseEnv)
		}
		merged = mergeClosureFlowEnv(merged, caseEnv)
	}
	return merged
}

func (checker *ssaClosureEscapeChecker) checkSourceDecl(decl ast.Decl, env closureFlowEnv) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) != len(valueSpec.Values) {
			continue
		}
		for i, name := range valueSpec.Names {
			obj := objectForIdent(checker.pkg, name)
			setClosureFlowValue(env, obj, checker.sourceClosuresForExpr(valueSpec.Values[i], env))
		}
	}
}

func (checker *ssaClosureEscapeChecker) checkSourceExpr(expr ast.Expr, env closureFlowEnv) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case nil:
			return true
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			checker.checkSourceCall(n, env)
		}
		return true
	})
}

func (checker *ssaClosureEscapeChecker) checkSourceReturn(ret *ast.ReturnStmt, env closureFlowEnv) {
	for _, result := range ret.Results {
		for _, lit := range checker.sourceClosuresForExpr(result, env) {
			checker.reportSourceBorrowCaptures(lit, result, GWN007, "cannot return closure capturing %s borrow %q")
		}
	}
}

func (checker *ssaClosureEscapeChecker) checkSourceAssign(stmt *ast.AssignStmt, env closureFlowEnv) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	rhsClosures := make([][]*ast.FuncLit, len(stmt.Rhs))
	for i, rhs := range stmt.Rhs {
		checker.checkSourceExpr(rhs, env)
		rhsClosures[i] = checker.sourceClosuresForExpr(rhs, env)
	}
	for i, rhs := range stmt.Rhs {
		if !storeTargetEscapes(checker.pkg, stmt.Lhs[i]) {
			continue
		}
		for _, lit := range rhsClosures[i] {
			checker.reportSourceBorrowCaptures(lit, rhs, GWN006, "cannot store closure capturing %s borrow %q into escaping location")
		}
	}
	for i, lhs := range stmt.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		obj := objectForIdent(checker.pkg, id)
		setClosureFlowValue(env, obj, rhsClosures[i])
	}
}

func (checker *ssaClosureEscapeChecker) checkSourceCall(call *ast.CallExpr, env closureFlowEnv) {
	if _, ok := checker.caps.CallBinding(call); ok {
		return
	}
	if callee := callCallee(checker.pkg, call); callee == nil {
		return
	}
	for _, arg := range call.Args {
		for _, lit := range checker.sourceClosuresForExpr(arg, env) {
			checker.reportSourceBorrowCaptures(lit, arg, GWN008, "cannot pass closure capturing %s borrow %q to untracked function")
		}
	}
}

func (checker *ssaClosureEscapeChecker) sourceClosuresForExpr(expr ast.Expr, env closureFlowEnv) []*ast.FuncLit {
	if lit, ok := sourceFuncLit(expr); ok {
		return []*ast.FuncLit{lit}
	}
	id, ok := expr.(*ast.Ident)
	if !ok || env == nil {
		return nil
	}
	obj := objectForIdent(checker.pkg, id)
	if obj == nil {
		return nil
	}
	return env[obj]
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

func sourceFuncLitBorrowCaptures(pkg *packages.Package, caps *OstampIndex, lit *ast.FuncLit) []goCapture {
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
