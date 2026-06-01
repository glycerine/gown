package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func bindRestoreCapabilities(pkg *packages.Package, idx *OstampIndex, quals map[int]*CapQualifierAnnotation, restores map[int]*RestoreAnnotation, file *ast.File) {
	if pkg == nil || idx == nil || file == nil || len(restores) == 0 {
		return
	}
	path := gownSourcePath(pkg.Fset.Position(file.Pos()).Filename)
	for _, ann := range restores {
		ann.Path = path
		idx.RestoreAnnotations = append(idx.RestoreAnnotations, ann)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE {
			return true
		}
		if len(assign.Rhs) != 1 {
			return true
		}
		call, ok := unparenExpr(assign.Rhs[0]).(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := unparenExpr(call.Fun).(*ast.FuncLit)
		if !ok || fn.Type == nil {
			return true
		}
		funcOffset := pkg.Fset.Position(fn.Type.Func).Offset
		ann := restores[funcOffset]
		if ann == nil {
			return true
		}
		binding := newRestoreBinding(pkg, idx, quals, path, ann, assign, call, fn)
		idx.addRestoreBinding(binding)
		return false
	})
}

func newRestoreBinding(pkg *packages.Package, idx *OstampIndex, quals map[int]*CapQualifierAnnotation, path string, ann *RestoreAnnotation, assign *ast.AssignStmt, call *ast.CallExpr, fn *ast.FuncLit) RestoreBinding {
	pos := pkg.Fset.Position(call.Pos())
	binding := RestoreBinding{
		Annotation: ann,
		Offset:     pos.Offset,
		Line:       pos.Line,
		Col:        pos.Column,
		Path:       path,
		Assign:     assign,
		Call:       call,
		FuncLit:    fn,
		Lhs:        append([]ast.Expr(nil), assign.Lhs...),
		Args:       append([]ast.Expr(nil), call.Args...),
		LhsPlaces:  restorePlacesForExprs(idx, assign.Lhs),
		ArgPlaces:  restorePlacesForExprs(idx, call.Args),
	}
	bindRestoreFuncLiteralCapabilities(pkg, idx, quals, fn, &binding)
	binding.Return = restoreFinalReturn(fn.Body)
	bindRestoreLHSResultCaps(pkg, idx, assign, binding.ResultCaps)
	return binding
}

func bindRestoreFuncLiteralCapabilities(pkg *packages.Package, idx *OstampIndex, quals map[int]*CapQualifierAnnotation, fn *ast.FuncLit, binding *RestoreBinding) {
	if pkg == nil || idx == nil || fn == nil || binding == nil {
		return
	}
	sig, _ := pkg.TypesInfo.TypeOf(fn).(*types.Signature)
	if sig == nil {
		return
	}
	binding.ParamCaps = make([]Cap, sig.Params().Len())
	binding.ResultCaps = make([]Cap, sig.Results().Len())
	fillCaps(binding.ParamCaps, CapUntracked)
	fillCaps(binding.ResultCaps, CapUntracked)
	bindFieldListCapabilities(pkg, idx, quals, fn.Type.Params, sig.Params(), binding.ParamCaps)
	bindRestoreResultCapabilities(pkg, idx, quals, fn.Type.Results, sig.Results(), binding.ResultCaps)
}

func bindRestoreResultCapabilities(pkg *packages.Package, idx *OstampIndex, quals map[int]*CapQualifierAnnotation, fields *ast.FieldList, tuple *types.Tuple, out []Cap) {
	if fields == nil || tuple == nil {
		return
	}
	tupleIndex := 0
	for _, field := range fields.List {
		fieldCap := directCapForType(pkg, quals, field.Type)
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n && tupleIndex < len(out); i++ {
			obj := tuple.At(tupleIndex)
			if fieldCap != CapInvalid {
				out[tupleIndex] = fieldCap
			}
			bindObjectCaps(idx, obj, fieldCap, CapInvalid)
			tupleIndex++
		}
	}
}

func bindRestoreLHSResultCaps(pkg *packages.Package, idx *OstampIndex, assign *ast.AssignStmt, resultCaps []Cap) {
	if pkg == nil || idx == nil || assign == nil || assign.Tok != token.DEFINE {
		return
	}
	for i, lhs := range assign.Lhs {
		if i >= len(resultCaps) {
			continue
		}
		obj := assignedObject(pkg, lhs)
		if obj == nil {
			continue
		}
		bindObjectCaps(idx, obj, resultCaps[i], CapInvalid)
	}
}

func restorePlacesForExprs(idx *OstampIndex, exprs []ast.Expr) []Place {
	places := make([]Place, len(exprs))
	for i, expr := range exprs {
		if place, ok := idx.PlaceForExpr(expr); ok {
			places[i] = place
		}
	}
	return places
}

func restoreFinalReturn(body *ast.BlockStmt) *ast.ReturnStmt {
	if body == nil || len(body.List) == 0 {
		return nil
	}
	ret, _ := body.List[len(body.List)-1].(*ast.ReturnStmt)
	return ret
}
