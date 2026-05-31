package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func bindIntrinsicCapabilities(pkg *packages.Package, idx *OstampIndex, intrinsics map[int]*IntrinsicAnnotation, file *ast.File) {
	if pkg == nil || idx == nil || len(intrinsics) == 0 || file == nil {
		return
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ValueSpec:
			for i, value := range n.Values {
				var result types.Object
				if i < len(n.Names) {
					result = pkg.TypesInfo.Defs[n.Names[i]]
				}
				bindIntrinsicCallsInExpr(pkg, idx, intrinsics, value, result, true)
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, rhs := range n.Rhs {
				result, bindResult := intrinsicAssignResultObject(pkg, n, i)
				bindIntrinsicCallsInExpr(pkg, idx, intrinsics, rhs, result, bindResult)
			}
		case *ast.CallExpr:
			bindIntrinsicCall(pkg, idx, intrinsics, n, nil, false)
		}
		return true
	})
}

func intrinsicAssignResultObject(pkg *packages.Package, stmt *ast.AssignStmt, index int) (types.Object, bool) {
	if pkg == nil || stmt == nil || index >= len(stmt.Lhs) {
		return nil, false
	}
	if stmt.Tok == token.DEFINE {
		if name, ok := stmt.Lhs[index].(*ast.Ident); ok {
			if obj := pkg.TypesInfo.Defs[name]; obj != nil {
				return obj, true
			}
		}
	}
	return assignedObject(pkg, stmt.Lhs[index]), false
}

func bindIntrinsicCallsInExpr(pkg *packages.Package, idx *OstampIndex, intrinsics map[int]*IntrinsicAnnotation, expr ast.Expr, result types.Object, bindResult bool) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var callResult types.Object
		callBindResult := false
		if call == expr {
			callResult = result
			callBindResult = bindResult
		}
		bindIntrinsicCall(pkg, idx, intrinsics, call, callResult, callBindResult)
		return true
	})
}

func bindIntrinsicCall(pkg *packages.Package, idx *OstampIndex, intrinsics map[int]*IntrinsicAnnotation, call *ast.CallExpr, result types.Object, bindResult bool) {
	if call == nil {
		return
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return
	}
	pos := pkg.Fset.Position(fun.Pos())
	ann := intrinsics[pos.Offset]
	if ann == nil {
		return
	}
	var arg ast.Expr
	var argPlace Place
	args := append([]ast.Expr(nil), call.Args...)
	argPlaces := make([]Place, len(call.Args))
	for i, callArg := range call.Args {
		argPlaces[i], _ = idx.PlaceForExpr(callArg)
	}
	if len(call.Args) > 0 {
		arg = call.Args[0]
		argPlace = argPlaces[0]
	}
	idx.addIntrinsicBinding(IntrinsicBinding{
		Kind:      ann.Intrinsic,
		Offset:    pos.Offset,
		Line:      pos.Line,
		Col:       pos.Column,
		Path:      gownSourcePath(pos.Filename),
		Call:      call,
		Arg:       arg,
		ArgPlace:  argPlace,
		Args:      args,
		ArgPlaces: argPlaces,
		Result:    result,
	})
	if bindResult {
		bindIntrinsicResultOstamp(idx, ann.Intrinsic, result)
	}
}

func (idx *OstampIndex) addIntrinsicBinding(binding IntrinsicBinding) {
	if idx == nil || binding.Call == nil {
		return
	}
	if i, ok := idx.intrinsicByCall[binding.Call]; ok {
		existing := idx.IntrinsicBindings[i]
		if existing.Result == nil && binding.Result != nil {
			existing.Result = binding.Result
		}
		if existing.Arg == nil {
			existing.Arg = binding.Arg
			existing.ArgPlace = binding.ArgPlace
		}
		if len(existing.Args) == 0 && len(binding.Args) > 0 {
			existing.Args = binding.Args
			existing.ArgPlaces = binding.ArgPlaces
		}
		idx.IntrinsicBindings[i] = existing
		return
	}
	idx.IntrinsicBindings = append(idx.IntrinsicBindings, binding)
	idx.intrinsicByCall[binding.Call] = len(idx.IntrinsicBindings) - 1
}

func bindIntrinsicResultOstamp(idx *OstampIndex, kind IntrinsicKind, result types.Object) {
	if idx == nil || result == nil {
		return
	}
	switch kind {
	case IntrinsicMub:
		bindObjectCaps(idx, result, CapMub, CapInvalid)
	case IntrinsicRob:
		bindObjectCaps(idx, result, CapRob, CapInvalid)
	case IntrinsicNew, IntrinsicClone, IntrinsicCloneExported:
		bindObjectCaps(idx, result, CapIso, CapInvalid)
	case IntrinsicFreeze:
		bindObjectCaps(idx, result, CapImm, CapInvalid)
	}
}
