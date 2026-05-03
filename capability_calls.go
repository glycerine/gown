package gown

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func bindCallCapabilities(pkg *packages.Package, idx *CapabilityIndex, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if fn.Body == nil {
			return false
		}
		bindFunctionCallCapabilities(pkg, idx, fn.Name.Name, fn.Body)
		return false
	})
}

func bindFunctionCallCapabilities(pkg *packages.Package, idx *CapabilityIndex, funcName string, body *ast.BlockStmt) {
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		bindCallCapability(pkg, idx, funcName, call)
		return true
	})
}

func bindCallCapability(pkg *packages.Package, idx *CapabilityIndex, funcName string, call *ast.CallExpr) {
	callee := callCallee(pkg, call)
	if callee == nil {
		return
	}
	sig := idx.FuncCap(callee)
	if sig == nil {
		return
	}
	pos := pkg.Fset.Position(call.Pos())
	idx.CallBindings = append(idx.CallBindings, CallBinding{
		Offset:     pos.Offset,
		Line:       pos.Line,
		Col:        pos.Column,
		FuncName:   funcName,
		Callee:     callee,
		ParamCaps:  append([]Cap(nil), sig.Params...),
		ResultCaps: append([]Cap(nil), sig.Results...),
		Args:       append([]ast.Expr(nil), call.Args...),
	})
}

func callCallee(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		fn, _ := pkg.TypesInfo.Uses[fun].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pkg.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	default:
		return nil
	}
}
