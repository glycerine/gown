package gown

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func checkRestoreRegions(ctx *CheckerContext) CheckerErrors {
	if ctx == nil || ctx.Pkg == nil || ctx.Caps == nil {
		return nil
	}
	checker := &restoreChecker{
		pkg:      ctx.Pkg,
		caps:     ctx.Caps,
		reported: make(map[string]bool),
	}
	checker.checkAllAnnotationsBound()
	for _, binding := range ctx.Caps.RestoreBindings {
		checker.checkBinding(binding)
	}
	return checker.errs
}

type restoreChecker struct {
	pkg      *packages.Package
	caps     *OstampIndex
	errs     CheckerErrors
	reported map[string]bool
}

type restoreBodyCheck struct {
	*restoreChecker
	binding          RestoreBinding
	locals           map[types.Object]bool
	isoParams        map[types.Object]bool
	localTokens      map[types.Object]restoreToken
	movedProjections map[PlaceKey]ast.Node
	incorporated     map[types.Object]types.Object
}

type restoreToken struct {
	Root types.Object
	Path string
}

func (token restoreToken) Key() PlaceKey {
	return PlaceKey{Root: token.Root, Path: token.Path}
}

func (checker *restoreChecker) checkAllAnnotationsBound() {
	if checker == nil || checker.caps == nil {
		return
	}
	bound := make(map[*RestoreAnnotation]bool)
	for _, binding := range checker.caps.RestoreBindings {
		if binding.Annotation != nil {
			bound[binding.Annotation] = true
		}
	}
	for _, ann := range checker.caps.RestoreAnnotations {
		if bound[ann] {
			continue
		}
		checker.reportCheckerError(newCheckerErrorAtSource(
			GWN013,
			ann.Path,
			ann.Span.Offset,
			ann.Span.Line,
			ann.Span.Col,
			`\restore must appear as the single right-hand side of an assignment to a func literal call`,
		))
	}
}

func (checker *restoreChecker) checkBinding(binding RestoreBinding) {
	if checker == nil || binding.Call == nil || binding.FuncLit == nil || binding.Assign == nil {
		return
	}
	checker.checkRestoreAssignmentShape(binding)
	checker.checkRestoreSignature(binding)
	checker.checkRestoreArguments(binding)
	body := &restoreBodyCheck{
		restoreChecker:   checker,
		binding:          binding,
		locals:           restoreLocalObjects(checker.pkg, binding.FuncLit),
		isoParams:        restoreIsoParamObjects(checker.pkg, binding),
		localTokens:      make(map[types.Object]restoreToken),
		movedProjections: make(map[PlaceKey]ast.Node),
		incorporated:     make(map[types.Object]types.Object),
	}
	for param := range body.isoParams {
		body.localTokens[param] = restoreToken{Root: param}
	}
	body.checkBody()
	checker.checkRestoreBoundary(binding, body.locals)
	body.checkSymbolicBoundary()
}

func (checker *restoreChecker) checkRestoreAssignmentShape(binding RestoreBinding) {
	if binding.Assign.Tok != token.ASSIGN && binding.Assign.Tok != token.DEFINE {
		checker.errorAtNode(binding.Assign, `\restore must be assigned with = or :=`)
	}
	if len(binding.Assign.Rhs) != 1 {
		checker.errorAtNode(binding.Assign, `\restore assignment must have exactly one right-hand side expression`)
	}
	if len(binding.Assign.Lhs) == 0 {
		checker.errorAtNode(binding.Assign, `\restore assignment must have at least one left-hand side`)
	}
	if len(binding.Lhs) != len(binding.ResultCaps) {
		checker.errorAtNode(binding.Assign, fmt.Sprintf(`\restore returns %d result(s), but assignment has %d left-hand side value(s)`, len(binding.ResultCaps), len(binding.Lhs)))
	}
	for _, lhs := range binding.Lhs {
		place, ok := checker.caps.PlaceForExpr(lhs)
		if !ok || place.Root == nil || place.Key().Path != "" {
			checker.errorAtNode(lhs, `\restore result target must be a direct local root`)
			continue
		}
		if capForSSAPlace(checker.caps, place) != CapIso {
			checker.errorAtNode(lhs, `\restore result target must have \iso ownerstamp`)
		}
	}
}

func (checker *restoreChecker) checkRestoreSignature(binding RestoreBinding) {
	fn := binding.FuncLit
	if fn.Type == nil {
		checker.errorAtNode(binding.Call, `\restore must wrap a func literal`)
		return
	}
	if len(binding.ResultCaps) == 0 {
		checker.errorAtNode(fn.Type, `\restore func literal must declare at least one \iso result`)
	}
	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			if len(field.Names) > 0 {
				checker.errorAtNode(field, `\restore v1 does not allow named result parameters`)
			}
		}
	}
	for i, cap := range binding.ResultCaps {
		if cap != CapIso {
			checker.errorAtNode(fn.Type, fmt.Sprintf(`\restore result %d must be declared \iso`, i+1))
		}
	}
	isoParams := 0
	sig, _ := checker.pkg.TypesInfo.TypeOf(fn).(*types.Signature)
	if sig == nil {
		return
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		cap := CapUntracked
		if i < len(binding.ParamCaps) {
			cap = binding.ParamCaps[i]
		}
		if cap == CapIso {
			isoParams++
		}
		if isPointerLike(param.Type()) && cap != CapIso && cap != CapImm {
			checker.errorAtObject(param, `pointer parameters in \restore must be declared \iso or \imm`)
		}
		if cap == CapMub || cap == CapRob {
			checker.errorAtObject(param, `\restore v1 does not allow borrow parameters`)
		}
	}
	if isoParams == 0 {
		checker.errorAtNode(fn.Type, `\restore must open at least one \iso parameter`)
	}
}

func (checker *restoreChecker) checkRestoreArguments(binding RestoreBinding) {
	if len(binding.Args) != len(binding.ParamCaps) {
		checker.errorAtNode(binding.Call, `\restore argument count must match the func literal parameters`)
		return
	}
	for i, cap := range binding.ParamCaps {
		if i >= len(binding.Args) {
			continue
		}
		arg := binding.Args[i]
		switch cap {
		case CapIso:
			place, ok := checker.caps.PlaceForExpr(arg)
			if !ok || place.Root == nil || place.Key().Path != "" {
				checker.errorAtNode(arg, `\restore \iso arguments must be direct local roots in v1`)
				continue
			}
			if !placeCanTransferAsIso(checker.caps, place) {
				checker.errorAtNode(arg, `\restore \iso argument must be a live \iso root`)
			}
		case CapImm:
			if !restoreExpressionIsImmutable(checker.pkg, checker.caps, arg) {
				checker.errorAtNode(arg, `\restore \imm argument must be an immutable value`)
			}
		default:
			if isPointerLike(checker.pkg.TypesInfo.TypeOf(arg)) {
				checker.errorAtNode(arg, `pointer arguments to \restore must be tracked as \iso or \imm`)
			}
		}
	}
}

func (body *restoreBodyCheck) checkBody() {
	if body == nil || body.binding.FuncLit == nil || body.binding.FuncLit.Body == nil {
		return
	}
	stmts := body.binding.FuncLit.Body.List
	if len(stmts) == 0 {
		body.errorAtNode(body.binding.FuncLit.Body, `\restore body must end with a return`)
		return
	}
	for i, stmt := range stmts {
		if i == len(stmts)-1 {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok {
				body.errorAtNode(stmt, `\restore body must have one final return statement`)
				continue
			}
			body.checkReturn(ret)
			continue
		}
		body.checkStatement(stmt, false)
	}
	body.checkCaptures()
}

func (body *restoreBodyCheck) checkStatement(stmt ast.Stmt, allowReturn bool) {
	switch stmt := stmt.(type) {
	case *ast.DeclStmt:
		body.checkDecl(stmt)
	case *ast.AssignStmt:
		body.checkAssign(stmt)
	case *ast.IfStmt:
		body.checkIf(stmt)
	case *ast.ReturnStmt:
		if !allowReturn {
			body.errorAtNode(stmt, `\restore body may only return in the final top-level statement`)
		}
	default:
		body.errorAtNode(stmt, fmt.Sprintf(`%s is not allowed inside \restore`, restoreStmtName(stmt)))
	}
}

func (body *restoreBodyCheck) checkDecl(stmt *ast.DeclStmt) {
	decl, ok := stmt.Decl.(*ast.GenDecl)
	if !ok || decl.Tok != token.VAR {
		body.errorAtNode(stmt, `\restore only allows local var declarations`)
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			body.errorAtNode(spec, `\restore only allows local var declarations`)
			continue
		}
		for _, value := range valueSpec.Values {
			body.checkExpr(value)
			body.rejectUntrackedPointerValue(value)
		}
		for i, name := range valueSpec.Names {
			if i < len(valueSpec.Values) {
				body.rejectInterfaceBoxing(name, valueSpec.Values[i])
				body.recordLocalToken(name, valueSpec.Values[i])
			}
		}
		if len(valueSpec.Values) == 0 && valueSpec.Type != nil && isPointerLike(body.pkg.TypesInfo.TypeOf(valueSpec.Type)) {
			cap := directCapForType(body.pkg, nil, valueSpec.Type)
			if cap == CapInvalid || cap == CapUntracked {
				body.errorAtNode(valueSpec, `\restore pointer locals must be initialized from tracked values`)
			}
		}
	}
}

func (body *restoreBodyCheck) checkAssign(stmt *ast.AssignStmt) {
	if stmt.Tok != token.ASSIGN && stmt.Tok != token.DEFINE {
		body.errorAtNode(stmt, `\restore only allows = and := assignments`)
		return
	}
	if len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		body.errorAtNode(stmt, `\restore v1 only allows single-target assignments inside the body`)
		return
	}
	lhs := stmt.Lhs[0]
	rhs := stmt.Rhs[0]
	body.checkExpr(lhs)
	body.checkExpr(rhs)
	body.checkAssignmentTarget(lhs, rhs)
	body.checkPointerFieldStoreValue(lhs, rhs)
	body.rejectInterfaceBoxing(lhs, rhs)
	body.rejectUntrackedPointerValue(rhs)
	body.recordPointerAssignment(lhs, rhs)
}

func (body *restoreBodyCheck) checkAssignmentTarget(lhs, rhs ast.Expr) {
	place, ok := body.caps.PlaceForExpr(lhs)
	if !ok || place.Root == nil {
		body.errorAtNode(lhs, `\restore assignment target must be a tracked local place`)
		return
	}
	if !body.locals[place.Root] {
		body.errorAtNode(lhs, `\restore may not mutate captured locals or package globals`)
		return
	}
	if place.Key().Path == "" {
		if body.isoParams[place.Root] {
			body.errorAtNode(lhs, `\restore v1 may not reassign \iso parameter roots; mutate their fields instead`)
		}
		return
	}
	if isPointerLike(body.pkg.TypesInfo.TypeOf(lhs)) && capForSSAPlace(body.caps, place) != CapIso {
		body.errorAtNode(lhs, `\restore v1 rejects stores to untracked pointer fields`)
	}
	if capForSSAPlace(body.caps, place) == CapImm || capForSSAPlace(body.caps, place) == CapRob {
		body.errorAtNode(lhs, `\restore may not mutate read-only fields`)
	}
}

func (body *restoreBodyCheck) checkPointerFieldStoreValue(lhs, rhs ast.Expr) {
	if body == nil || lhs == nil || rhs == nil || isNilIdent(rhs) || !isPointerLike(body.pkg.TypesInfo.TypeOf(lhs)) {
		return
	}
	lhsPlace, ok := body.caps.PlaceForExpr(lhs)
	if !ok || lhsPlace.Root == nil || lhsPlace.Key().Path == "" {
		return
	}
	if capForSSAPlace(body.caps, lhsPlace) != CapIso {
		return
	}
	if _, ok := body.tokenForExpr(rhs); ok {
		return
	}
	body.errorAtNode(rhs, `\restore pointer fields may only store nil or restore-local aliases`)
}

func (body *restoreBodyCheck) rejectInterfaceBoxing(dst ast.Expr, src ast.Expr) {
	if body == nil || dst == nil || src == nil || !isInterfaceType(body.pkg.TypesInfo.TypeOf(dst)) {
		return
	}
	if isNilIdent(src) {
		return
	}
	if value, ok := valueOstampForExpr(body.pkg, body.caps, src); ok && capTracked(value.Cap) {
		body.errorAtNode(src, `\restore may not box restore-local aliases into interfaces`)
	}
}

func (body *restoreBodyCheck) checkIf(stmt *ast.IfStmt) {
	if stmt.Init != nil {
		body.errorAtNode(stmt.Init, `\restore if statements may not have init clauses`)
	}
	body.checkExpr(stmt.Cond)
	before := cloneRestoreMoved(body.movedProjections)
	thenMoved := body.checkNestedBlockResult(stmt.Body, before)
	elseMoved := cloneRestoreMoved(before)
	switch elseStmt := stmt.Else.(type) {
	case nil:
	case *ast.BlockStmt:
		elseMoved = body.checkNestedBlockResult(elseStmt, before)
	case *ast.IfStmt:
		saved := body.movedProjections
		body.movedProjections = cloneRestoreMoved(before)
		body.checkIf(elseStmt)
		elseMoved = cloneRestoreMoved(body.movedProjections)
		body.movedProjections = saved
	default:
		body.errorAtNode(elseStmt, `\restore only allows block or if else clauses`)
	}
	body.movedProjections = unionRestoreMoved(thenMoved, elseMoved)
}

func (body *restoreBodyCheck) checkNestedBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		body.checkStatement(stmt, false)
	}
}

func (body *restoreBodyCheck) checkNestedBlockResult(block *ast.BlockStmt, moved map[PlaceKey]ast.Node) map[PlaceKey]ast.Node {
	saved := body.movedProjections
	body.movedProjections = cloneRestoreMoved(moved)
	body.checkNestedBlock(block)
	out := cloneRestoreMoved(body.movedProjections)
	body.movedProjections = saved
	return out
}

func (body *restoreBodyCheck) checkReturn(ret *ast.ReturnStmt) {
	if len(ret.Results) != len(body.binding.ResultCaps) {
		body.errorAtNode(ret, `\restore final return count must match declared results`)
	}
	for _, result := range ret.Results {
		body.checkExpr(result)
	}
}

func (body *restoreBodyCheck) checkExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch n := n.(type) {
		case nil:
			return true
		case *ast.CallExpr:
			body.errorAtNode(n, `function calls are not allowed inside \restore`)
			return false
		case *ast.FuncLit:
			body.errorAtNode(n, `nested func literals are not allowed inside \restore`)
			return false
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				body.errorAtNode(n, `address-of is not allowed inside \restore`)
			}
			if n.Op == token.ARROW {
				body.errorAtNode(n, `channel receive is not allowed inside \restore`)
			}
		case *ast.SendStmt:
			body.errorAtNode(n, `channel send is not allowed inside \restore`)
			return false
		case *ast.SelectorExpr:
			if selectorUsesPackage(body.pkg, n, "unsafe") || selectorUsesPackage(body.pkg, n, "reflect") {
				body.errorAtNode(n, `unsafe and reflection are not allowed inside \restore`)
			}
		case *ast.IndexExpr, *ast.SliceExpr, *ast.TypeAssertExpr, *ast.CompositeLit:
			body.errorAtNode(n, fmt.Sprintf(`%s is not allowed inside \restore`, restoreExprName(n.(ast.Expr))))
			return false
		}
		return true
	})
}

func (body *restoreBodyCheck) rejectUntrackedPointerValue(expr ast.Expr) {
	if expr == nil || isNilIdent(expr) || !isPointerLike(body.pkg.TypesInfo.TypeOf(expr)) {
		return
	}
	if value, ok := valueOstampForExpr(body.pkg, body.caps, expr); ok && value.Cap != CapInvalid && value.Cap != CapUntracked {
		return
	}
	body.errorAtNode(expr, `\restore v1 rejects untracked pointer aliases`)
}

func (body *restoreBodyCheck) recordLocalToken(name *ast.Ident, rhs ast.Expr) {
	if body == nil || name == nil || rhs == nil || !isPointerLike(body.pkg.TypesInfo.TypeOf(rhs)) {
		return
	}
	obj, _ := body.pkg.TypesInfo.Defs[name].(*types.Var)
	if obj == nil {
		obj, _ = body.pkg.TypesInfo.Uses[name].(*types.Var)
	}
	if obj == nil || body.isoParams[obj] {
		return
	}
	token, ok := body.tokenForExpr(rhs)
	if !ok {
		return
	}
	body.localTokens[obj] = token
	body.markMovedProjectionFromExpr(rhs, token)
}

func (body *restoreBodyCheck) recordPointerAssignment(lhs, rhs ast.Expr) {
	if body == nil || lhs == nil || rhs == nil || !isPointerLike(body.pkg.TypesInfo.TypeOf(rhs)) {
		return
	}
	lhsPlace, lhsOK := body.caps.PlaceForExpr(lhs)
	if !lhsOK || lhsPlace.Root == nil {
		return
	}
	if isNilIdent(rhs) {
		body.clearMovedProjection(lhsPlace)
		return
	}
	token, ok := body.tokenForExpr(rhs)
	if !ok {
		return
	}
	body.markMovedProjectionFromExpr(rhs, token)
	if lhsPlace.Key().Path == "" {
		if !body.isoParams[lhsPlace.Root] {
			body.localTokens[lhsPlace.Root] = token
		}
		return
	}
	body.clearMovedProjection(lhsPlace)
	body.recordIncorporation(lhsPlace, token, lhs)
}

func (body *restoreBodyCheck) tokenForExpr(expr ast.Expr) (restoreToken, bool) {
	place, ok := body.caps.PlaceForExpr(expr)
	if !ok || place.Root == nil {
		return restoreToken{}, false
	}
	if place.Key().Path == "" {
		token, ok := body.localTokens[place.Root]
		if ok {
			return token, true
		}
		if body.isoParams[place.Root] {
			return restoreToken{Root: place.Root}, true
		}
		return restoreToken{}, false
	}
	if body.isoParams[place.Root] {
		return restoreToken{Root: place.Root, Path: place.Projection.String()}, true
	}
	base, ok := body.localTokens[place.Root]
	if !ok || base.Path != "" {
		return restoreToken{}, false
	}
	return restoreToken{Root: base.Root, Path: place.Projection.String()}, true
}

func (body *restoreBodyCheck) markMovedProjectionFromExpr(expr ast.Expr, token restoreToken) {
	if token.Root == nil || token.Path == "" {
		return
	}
	place, ok := body.caps.PlaceForExpr(expr)
	if !ok || place.Root == nil || place.Key().Path == "" {
		return
	}
	body.movedProjections[token.Key()] = expr
}

func (body *restoreBodyCheck) clearMovedProjection(place Place) {
	if place.Root == nil || place.Key().Path == "" {
		return
	}
	token := restoreToken{Root: place.Root, Path: place.Projection.String()}
	if !body.isoParams[place.Root] {
		base, ok := body.localTokens[place.Root]
		if !ok || base.Path != "" {
			return
		}
		token.Root = base.Root
	}
	delete(body.movedProjections, token.Key())
}

func (body *restoreBodyCheck) recordIncorporation(target Place, token restoreToken, at ast.Node) {
	if token.Root == nil || token.Path != "" || !body.isoParams[token.Root] {
		return
	}
	targetRoot := body.incorporationTargetRoot(target)
	if targetRoot == nil || targetRoot == token.Root {
		return
	}
	if prior := body.incorporated[token.Root]; prior != nil && prior != targetRoot {
		body.errorAtNode(at, `\restore cannot incorporate the same \iso root into multiple returned graphs`)
		return
	}
	body.incorporated[token.Root] = targetRoot
}

func (body *restoreBodyCheck) incorporationTargetRoot(target Place) types.Object {
	if target.Root == nil {
		return nil
	}
	if body.isoParams[target.Root] {
		return target.Root
	}
	token, ok := body.localTokens[target.Root]
	if !ok || token.Path != "" {
		return nil
	}
	return token.Root
}

func (body *restoreBodyCheck) checkCaptures() {
	ast.Inspect(body.binding.FuncLit.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj, _ := body.pkg.TypesInfo.Uses[id].(*types.Var)
		if obj == nil || obj.IsField() || body.locals[obj] {
			return true
		}
		if restoreSafeCapturedVar(body.pkg, body.caps, obj) {
			return true
		}
		body.errorAtNode(id, `\restore may only capture \imm values and read-only non-pointer scalar locals`)
		return true
	})
}

func (checker *restoreChecker) checkRestoreBoundary(binding RestoreBinding, locals map[types.Object]bool) {
	ret := binding.Return
	if ret == nil {
		checker.errorAtNode(binding.FuncLit.Body, `\restore body must end with a return`)
		return
	}
	seen := make(map[PlaceKey]ast.Expr)
	for i, result := range ret.Results {
		if i >= len(binding.ResultCaps) {
			continue
		}
		place, ok := checker.caps.PlaceForExpr(result)
		if !ok || place.Root == nil {
			checker.errorAtNode(result, `\restore results must be restore-local pointer expressions`)
			continue
		}
		if !locals[place.Root] {
			checker.errorAtNode(result, `\restore may only return restore-local aliases`)
			continue
		}
		if place.Key().Path != "" {
			checker.errorAtNode(result, `\restore v1 results must be direct \iso parameter roots`)
			continue
		}
		isoParams := restoreIsoParamObjects(checker.pkg, binding)
		if !isoParams[place.Root] {
			checker.errorAtNode(result, `\restore v1 does not return local aliases; return an opened \iso parameter root`)
			continue
		}
		if !isPointerLike(checker.pkg.TypesInfo.TypeOf(result)) {
			checker.errorAtNode(result, `\restore result expression must be a pointer`)
		}
		key := place.Key()
		if prior := seen[key]; prior != nil {
			err := newCheckerErrorAtNode(checker.pkg, GWN013, result, `\restore results must name distinct roots`)
			err.Notes = []CheckerNote{{
				Path:    gownSourcePath(checker.pkg.Fset.Position(prior.Pos()).Filename),
				Offset:  checker.pkg.Fset.Position(prior.Pos()).Offset,
				Line:    checker.pkg.Fset.Position(prior.Pos()).Line,
				Col:     checker.pkg.Fset.Position(prior.Pos()).Column,
				Message: `same restore result root returned here`,
			}}
			checker.reportCheckerError(err)
			continue
		}
		seen[key] = result
	}
}

func (body *restoreBodyCheck) checkSymbolicBoundary() {
	if body == nil || body.binding.Return == nil {
		return
	}
	returned := make(map[types.Object]bool)
	for _, result := range body.binding.Return.Results {
		place, ok := body.caps.PlaceForExpr(result)
		if ok && place.Root != nil && place.Key().Path == "" && body.isoParams[place.Root] {
			returned[place.Root] = true
		}
	}
	for root, target := range body.incorporated {
		if returned[root] {
			body.errorAtObject(root, `\restore cannot return an \iso parameter after incorporating it into another returned graph`)
			continue
		}
		if target != nil && !returned[target] {
			body.errorAtObject(root, `\restore incorporated \iso parameter must be incorporated into a returned root`)
		}
	}
	for key, at := range body.movedProjections {
		if !body.tokenRootEscapes(key.Root, returned) {
			continue
		}
		err := newCheckerErrorAtNode(body.pkg, GWN013, at, `\restore moved a tracked field projection that was not overwritten before the graph was returned`)
		body.reportCheckerError(err)
	}
}

func (body *restoreBodyCheck) tokenRootEscapes(root types.Object, returned map[types.Object]bool) bool {
	if root == nil {
		return false
	}
	if returned[root] {
		return true
	}
	target := body.incorporated[root]
	return target != nil && returned[target]
}

func restoreLocalObjects(pkg *packages.Package, fn *ast.FuncLit) map[types.Object]bool {
	locals := make(map[types.Object]bool)
	if pkg == nil || fn == nil {
		return locals
	}
	if sig, _ := pkg.TypesInfo.TypeOf(fn).(*types.Signature); sig != nil {
		for i := 0; i < sig.Params().Len(); i++ {
			locals[sig.Params().At(i)] = true
		}
		for i := 0; i < sig.Results().Len(); i++ {
			locals[sig.Results().At(i)] = true
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if obj, ok := pkg.TypesInfo.Defs[id].(*types.Var); ok && obj != nil {
			locals[obj] = true
		}
		return true
	})
	return locals
}

func restoreIsoParamObjects(pkg *packages.Package, binding RestoreBinding) map[types.Object]bool {
	out := make(map[types.Object]bool)
	if pkg == nil || binding.FuncLit == nil {
		return out
	}
	sig, _ := pkg.TypesInfo.TypeOf(binding.FuncLit).(*types.Signature)
	if sig == nil {
		return out
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if i < len(binding.ParamCaps) && binding.ParamCaps[i] == CapIso {
			out[sig.Params().At(i)] = true
		}
	}
	return out
}

func cloneRestoreMoved(in map[PlaceKey]ast.Node) map[PlaceKey]ast.Node {
	out := make(map[PlaceKey]ast.Node, len(in))
	for key, node := range in {
		out[key] = node
	}
	return out
}

func unionRestoreMoved(left, right map[PlaceKey]ast.Node) map[PlaceKey]ast.Node {
	out := cloneRestoreMoved(left)
	for key, node := range right {
		if _, ok := out[key]; !ok {
			out[key] = node
		}
	}
	return out
}

func restoreExpressionIsImmutable(pkg *packages.Package, caps *OstampIndex, expr ast.Expr) bool {
	if pkg == nil || caps == nil || expr == nil {
		return false
	}
	if place, ok := caps.PlaceForExpr(expr); ok && place.Root != nil {
		return capForSSAPlace(caps, place) == CapImm
	}
	if isNonPointerScalar(pkg.TypesInfo.TypeOf(expr)) {
		return true
	}
	return false
}

func restoreSafeCapturedVar(pkg *packages.Package, caps *OstampIndex, obj *types.Var) bool {
	if obj == nil {
		return false
	}
	if pkg != nil && obj.Parent() == pkg.Types.Scope() {
		return false
	}
	if caps != nil && caps.ObjectCap(obj) == CapImm {
		return true
	}
	return isNonPointerScalar(obj.Type())
}

func isPointerLike(typ types.Type) bool {
	if typ == nil {
		return false
	}
	switch typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	default:
		return false
	}
}

func isNonPointerScalar(typ types.Type) bool {
	if typ == nil {
		return false
	}
	_, ok := typ.Underlying().(*types.Basic)
	return ok
}

func isNilIdent(expr ast.Expr) bool {
	id, ok := unparenExpr(expr).(*ast.Ident)
	return ok && id.Name == "nil"
}

func selectorUsesPackage(pkg *packages.Package, selector *ast.SelectorExpr, path string) bool {
	if pkg == nil || selector == nil {
		return false
	}
	id, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	pkgName, _ := pkg.TypesInfo.Uses[id].(*types.PkgName)
	return pkgName != nil && pkgName.Imported() != nil && pkgName.Imported().Path() == path
}

func restoreStmtName(stmt ast.Stmt) string {
	switch stmt.(type) {
	case *ast.ForStmt:
		return "loops"
	case *ast.RangeStmt:
		return "range"
	case *ast.SwitchStmt, *ast.TypeSwitchStmt:
		return "switch"
	case *ast.GoStmt:
		return "go"
	case *ast.DeferStmt:
		return "defer"
	case *ast.BranchStmt:
		return "goto/break/continue/fallthrough"
	case *ast.LabeledStmt:
		return "labels"
	case *ast.SendStmt:
		return "channel send"
	case *ast.SelectStmt:
		return "select"
	default:
		return fmt.Sprintf("%T", stmt)
	}
}

func restoreExprName(expr ast.Expr) string {
	switch expr.(type) {
	case *ast.IndexExpr:
		return "index expressions"
	case *ast.SliceExpr:
		return "slice expressions"
	case *ast.TypeAssertExpr:
		return "type assertions"
	case *ast.CompositeLit:
		return "composite literals"
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func (checker *restoreChecker) errorAtNode(node ast.Node, message string) {
	checker.reportCheckerError(newCheckerErrorAtNode(checker.pkg, GWN013, node, message))
}

func (checker *restoreChecker) errorAtObject(obj types.Object, message string) {
	checker.reportCheckerError(newCheckerErrorAtObject(checker.pkg, GWN013, obj, token.Position{}, message))
}

func (checker *restoreChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
