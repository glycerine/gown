package gown

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func bindIntrinsicCapabilities(pkg *packages.Package, idx *CapabilityIndex, intrinsics map[int]*IntrinsicAnnotation, file *ast.File) {
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
				bindIntrinsicCallsInExpr(pkg, idx, intrinsics, value, result)
			}
		case *ast.AssignStmt:
			if len(n.Lhs) != len(n.Rhs) {
				return true
			}
			for i, rhs := range n.Rhs {
				bindIntrinsicCallsInExpr(pkg, idx, intrinsics, rhs, assignedObject(pkg, n.Lhs[i]))
			}
		case *ast.CallExpr:
			bindIntrinsicCall(pkg, idx, intrinsics, n, nil)
		}
		return true
	})
}

func bindIntrinsicCallsInExpr(pkg *packages.Package, idx *CapabilityIndex, intrinsics map[int]*IntrinsicAnnotation, expr ast.Expr, result types.Object) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var callResult types.Object
		if call == expr {
			callResult = result
		}
		bindIntrinsicCall(pkg, idx, intrinsics, call, callResult)
		return true
	})
}

func bindIntrinsicCall(pkg *packages.Package, idx *CapabilityIndex, intrinsics map[int]*IntrinsicAnnotation, call *ast.CallExpr, result types.Object) {
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
	if len(call.Args) > 0 {
		arg = call.Args[0]
		argPlace, _ = idx.PlaceForExpr(arg)
	}
	idx.addIntrinsicBinding(IntrinsicBinding{
		Kind:     ann.Intrinsic,
		Offset:   pos.Offset,
		Line:     pos.Line,
		Col:      pos.Column,
		Path:     gownSourcePath(pos.Filename),
		Call:     call,
		Arg:      arg,
		ArgPlace: argPlace,
		Result:   result,
	})
	bindIntrinsicResultCapability(idx, ann.Intrinsic, result)
}

func (idx *CapabilityIndex) addIntrinsicBinding(binding IntrinsicBinding) {
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
		idx.IntrinsicBindings[i] = existing
		return
	}
	idx.IntrinsicBindings = append(idx.IntrinsicBindings, binding)
	idx.intrinsicByCall[binding.Call] = len(idx.IntrinsicBindings) - 1
}

func bindIntrinsicResultCapability(idx *CapabilityIndex, kind IntrinsicKind, result types.Object) {
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
