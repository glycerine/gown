package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type ostampErasureMode uint8

const (
	ostampErasureCalls ostampErasureMode = 1 << iota
	ostampErasureInterfaces
	ostampErasureReturns
	ostampErasureAssignments
	ostampErasureSends

	ostampErasureAll = ostampErasureCalls |
		ostampErasureInterfaces |
		ostampErasureReturns |
		ostampErasureAssignments |
		ostampErasureSends
)

func checkOstampErasure(ctx *CheckerContext) CheckerErrors {
	if ctx == nil {
		return nil
	}
	return checkOstampErasureInPackage(ctx.Pkg, ctx.Caps, ostampErasureAll)
}

func checkOstampErasureInPackage(pkg *packages.Package, caps *OstampIndex, mode ostampErasureMode) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	checker := ostampErasureChecker{
		pkg:      pkg,
		caps:     caps,
		mode:     mode,
		reported: make(map[string]bool),
	}
	for _, file := range pkg.Syntax {
		checker.checkFile(file)
	}
	return checker.errs
}

type ostampErasureChecker struct {
	pkg      *packages.Package
	caps     *OstampIndex
	mode     ostampErasureMode
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ostampErasureChecker) checkFile(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			checker.checkFuncReturns(node)
		case *ast.CallExpr:
			checker.checkCall(node)
		case *ast.AssignStmt:
			checker.checkAssign(node)
		case *ast.ValueSpec:
			checker.checkValueSpec(node)
		case *ast.SendStmt:
			checker.checkSend(node)
		case *ast.CompositeLit:
			checker.checkCompositeLit(node)
		}
		return true
	})
}

func (checker *ostampErasureChecker) checkFuncReturns(fn *ast.FuncDecl) {
	if checker.mode&ostampErasureReturns == 0 || fn == nil || fn.Body == nil {
		return
	}
	resultCaps := checker.funcResultCaps(fn)
	if len(resultCaps) == 0 {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			checker.checkReturn(node, resultCaps)
			return false
		}
		return true
	})
}

func (checker *ostampErasureChecker) checkReturn(ret *ast.ReturnStmt, resultCaps []Cap) {
	if ret == nil {
		return
	}
	for i, result := range ret.Results {
		if i >= len(resultCaps) || resultCaps[i] != CapUntracked {
			continue
		}
		value, ok := checker.valueOstamp(result)
		if !ok || !valueRequiresUnsafeErasure(value) {
			continue
		}
		checker.reportAtNode(
			GWN010,
			result,
			fmt.Sprintf("cannot return %s value %q as untracked result", value.Cap, valueName(value)),
		)
	}
}

func (checker *ostampErasureChecker) checkCall(call *ast.CallExpr) {
	if checker.mode&ostampErasureCalls == 0 || call == nil {
		return
	}
	if _, ok := checker.caps.IntrinsicBinding(call); ok {
		return
	}
	if isObserverCall(checker.pkg, checker.caps, call) {
		return
	}
	if isBuiltinCall(checker.pkg, call) {
		return
	}
	checker.checkCallArguments(call)
	checker.checkCallReceiver(call)
}

func (checker *ostampErasureChecker) checkCallArguments(call *ast.CallExpr) {
	paramCaps, ok := checker.callParamCaps(call)
	if !ok {
		return
	}
	for i, arg := range call.Args {
		paramCap := callCapAt(paramCaps, i)
		if paramCap != CapUntracked {
			continue
		}
		value, ok := checker.valueOstamp(arg)
		if !ok || !valueRequiresUnsafeErasure(value) {
			continue
		}
		checker.reportAtNode(
			GWN008,
			arg,
			fmt.Sprintf("cannot pass %s value %q to untracked parameter", value.Cap, valueName(value)),
		)
	}
}

func (checker *ostampErasureChecker) checkCallReceiver(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	fn := callCallee(checker.pkg, call)
	if fn == nil || checker.caps.FuncCap(fn) != nil {
		return
	}
	value, ok := checker.valueOstamp(sel.X)
	if !ok || !valueRequiresUnsafeErasure(value) {
		return
	}
	checker.reportAtNode(
		GWN008,
		sel.X,
		fmt.Sprintf("cannot call untracked method with %s receiver %q", value.Cap, valueName(value)),
	)
}

func (checker *ostampErasureChecker) checkAssign(stmt *ast.AssignStmt) {
	if checker.mode&(ostampErasureAssignments|ostampErasureInterfaces) == 0 || stmt == nil {
		return
	}
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i := range stmt.Lhs {
		checker.checkValueIntoDestination(stmt.Rhs[i], stmt.Lhs[i])
	}
}

func (checker *ostampErasureChecker) checkValueSpec(spec *ast.ValueSpec) {
	if checker.mode&(ostampErasureAssignments|ostampErasureInterfaces) == 0 || spec == nil {
		return
	}
	if len(spec.Names) != len(spec.Values) {
		return
	}
	for i, name := range spec.Names {
		checker.checkValueIntoObject(spec.Values[i], pkgDef(checker.pkg, name), name)
	}
}

func (checker *ostampErasureChecker) checkValueIntoDestination(src ast.Expr, dst ast.Expr) {
	if isBlankIdent(dst) {
		return
	}
	if place, ok := checker.caps.PlaceForExpr(dst); ok && place.Root != nil {
		checker.checkValueIntoObject(src, capObjectForSSAPlace(checker.caps, place), dst)
		return
	}
	if checker.mode&ostampErasureInterfaces != 0 && isInterfaceExpr(checker.pkg, dst) {
		checker.checkValueIntoCap(src, CapUntracked, GWN009, dst, "cannot erase %s value %q into interface")
	}
}

func (checker *ostampErasureChecker) checkValueIntoObject(src ast.Expr, obj types.Object, dst ast.Node) {
	if obj == nil {
		return
	}
	dstCap := checker.caps.ObjectCap(obj)
	if dstCap != CapUntracked {
		return
	}
	code := GWN010
	message := "cannot store %s value %q in untracked destination"
	if v, ok := obj.(*types.Var); ok && isInterfaceType(v.Type()) {
		code = GWN009
		message = "cannot erase %s value %q into interface"
	}
	checker.checkValueIntoCap(src, dstCap, code, dst, message)
}

func (checker *ostampErasureChecker) checkValueIntoCap(src ast.Expr, dstCap Cap, code CheckerErrorCode, dst ast.Node, message string) {
	if dstCap != CapUntracked {
		return
	}
	value, ok := checker.valueOstamp(src)
	if !ok || !valueRequiresUnsafeErasure(value) {
		return
	}
	checker.reportAtNode(code, src, fmt.Sprintf(message, value.Cap, valueName(value)))
}

func (checker *ostampErasureChecker) checkSend(send *ast.SendStmt) {
	if checker.mode&ostampErasureSends == 0 || send == nil {
		return
	}
	ch, ok := checker.caps.PlaceForExpr(send.Chan)
	if !ok || ch.Root == nil || chanElemCapForPlace(checker.caps, ch) != CapUntracked {
		return
	}
	value, ok := checker.valueOstamp(send.Value)
	if !ok || !valueRequiresUnsafeErasure(value) {
		return
	}
	checker.reportAtNode(
		GWN010,
		send.Value,
		fmt.Sprintf("cannot send %s value %q on untracked channel", value.Cap, valueName(value)),
	)
}

func (checker *ostampErasureChecker) checkCompositeLit(lit *ast.CompositeLit) {
	if checker.mode&(ostampErasureAssignments|ostampErasureInterfaces) == 0 || lit == nil {
		return
	}
	structType := compositeStructType(checker.pkg, lit)
	if structType == nil {
		return
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		field := fieldByName(structType, name.Name)
		if field == nil || checker.caps.ObjectCap(field) != CapUntracked {
			continue
		}
		code := GWN010
		message := "cannot store %s value %q in untracked field " + name.Name + ""
		if isInterfaceType(field.Type()) {
			code = GWN009
			message = "cannot erase %s value %q into interface field " + name.Name
		}
		checker.checkValueIntoCap(kv.Value, CapUntracked, code, kv.Value, message)
	}
}

func (checker *ostampErasureChecker) funcResultCaps(fn *ast.FuncDecl) []Cap {
	obj, _ := checker.pkg.TypesInfo.Defs[fn.Name].(*types.Func)
	if obj != nil {
		if sig := checker.caps.FuncCap(obj); sig != nil {
			return append([]Cap(nil), sig.Results...)
		}
		if signature, _ := obj.Type().(*types.Signature); signature != nil {
			results := make([]Cap, signature.Results().Len())
			fillCaps(results, CapUntracked)
			return results
		}
	}
	return nil
}

func (checker *ostampErasureChecker) callParamCaps(call *ast.CallExpr) ([]Cap, bool) {
	callee := callCallee(checker.pkg, call)
	if sig := checker.caps.FuncCap(callee); sig != nil {
		return append([]Cap(nil), sig.Params...), true
	}
	signature := callSignature(checker.pkg, call)
	if signature == nil {
		return nil, false
	}
	params := make([]Cap, signature.Params().Len())
	fillCaps(params, CapUntracked)
	if signature.Variadic() && len(call.Args) > len(params) {
		for len(params) < len(call.Args) {
			params = append(params, CapUntracked)
		}
	}
	return params, true
}

func (checker *ostampErasureChecker) valueOstamp(expr ast.Expr) (ValueOstamp, bool) {
	return valueOstampForExpr(checker.pkg, checker.caps, expr)
}

func (checker *ostampErasureChecker) reportAtNode(code CheckerErrorCode, node ast.Node, message string) {
	err := newCheckerErrorAtNode(checker.pkg, code, node, message)
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}

func callCapAt(caps []Cap, i int) Cap {
	if i < 0 || len(caps) == 0 {
		return CapInvalid
	}
	if i < len(caps) {
		return caps[i]
	}
	return caps[len(caps)-1]
}

func callSignature(pkg *packages.Package, call *ast.CallExpr) *types.Signature {
	if pkg == nil || call == nil {
		return nil
	}
	if callee := callCallee(pkg, call); callee != nil {
		if sig, _ := callee.Type().(*types.Signature); sig != nil {
			return sig
		}
	}
	if typ := pkg.TypesInfo.TypeOf(call.Fun); typ != nil {
		if sig, _ := typ.Underlying().(*types.Signature); sig != nil {
			return sig
		}
	}
	return nil
}

func isBuiltinCall(pkg *packages.Package, call *ast.CallExpr) bool {
	if pkg == nil || call == nil {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = pkg.TypesInfo.Uses[id].(*types.Builtin)
	return ok
}

func pkgDef(pkg *packages.Package, id *ast.Ident) types.Object {
	if pkg == nil || id == nil {
		return nil
	}
	if obj := pkg.TypesInfo.Defs[id]; obj != nil {
		return obj
	}
	return pkg.TypesInfo.Uses[id]
}

func isBlankIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "_"
}

func valueName(value ValueOstamp) string {
	if value.Place.Root != nil {
		return value.Place.Root.Name()
	}
	if value.Source.Root != nil {
		return value.Source.Root.Name()
	}
	return "<expression>"
}

func valueRequiresUnsafeErasure(value ValueOstamp) bool {
	if !capTracked(value.Cap) {
		return false
	}
	if value.Fresh && value.Intrinsic == IntrinsicInvalid && value.Place.Root == nil && value.Source.Root == nil {
		return false
	}
	return true
}

func compositeStructType(pkg *packages.Package, lit *ast.CompositeLit) *types.Struct {
	if pkg == nil || lit == nil {
		return nil
	}
	typ := pkg.TypesInfo.TypeOf(lit)
	if typ == nil {
		return nil
	}
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	if named, ok := typ.(*types.Named); ok {
		typ = named.Underlying()
	}
	st, _ := typ.Underlying().(*types.Struct)
	return st
}

func fieldByName(st *types.Struct, name string) *types.Var {
	if st == nil {
		return nil
	}
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if field.Name() == name {
			return field
		}
	}
	return nil
}
