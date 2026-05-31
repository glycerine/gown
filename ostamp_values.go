package gown

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type ValueOstamp struct {
	Cap         Cap
	ChanElemCap Cap
	Place       Place
	Fresh       bool
	Source      Place
	Intrinsic   IntrinsicKind
}

func (idx *OstampIndex) ValueOstamp(expr ast.Expr) (ValueOstamp, bool) {
	return valueOstampForExpr(nil, idx, expr)
}

func valueOstampForExpr(pkg *packages.Package, idx *OstampIndex, expr ast.Expr) (ValueOstamp, bool) {
	if idx == nil || expr == nil {
		return ValueOstamp{}, false
	}
	expr = unparenExpr(expr)
	if call, ok := expr.(*ast.CallExpr); ok {
		if binding, ok := idx.IntrinsicBinding(call); ok {
			return intrinsicValueOstamp(binding), true
		}
		if value, ok := callResultValueOstamp(pkg, idx, call); ok {
			return value, true
		}
	}
	if place, ok := idx.PlaceForExpr(expr); ok {
		return ValueOstamp{
			Cap:   capForSSAPlace(idx, place),
			Place: place,
		}, true
	}
	return ValueOstamp{}, false
}

func callResultValueOstamp(pkg *packages.Package, idx *OstampIndex, call *ast.CallExpr) (ValueOstamp, bool) {
	if pkg == nil || idx == nil || call == nil {
		return ValueOstamp{}, false
	}
	callee := callCallee(pkg, call)
	funcCap := idx.FuncCap(callee)
	if funcCap == nil || len(funcCap.Results) != 1 || !capTracked(funcCap.Results[0]) {
		return ValueOstamp{}, false
	}
	return ValueOstamp{
		Cap:   funcCap.Results[0],
		Fresh: funcCap.Results[0] == CapIso,
	}, true
}

func intrinsicValueOstamp(binding IntrinsicBinding) ValueOstamp {
	value := ValueOstamp{
		Source:    binding.ArgPlace,
		Intrinsic: binding.Kind,
	}
	switch binding.Kind {
	case IntrinsicMub:
		value.Cap = CapMub
	case IntrinsicRob:
		value.Cap = CapRob
	case IntrinsicFreeze:
		value.Cap = CapImm
	case IntrinsicNew, IntrinsicClone, IntrinsicCloneExported:
		value.Cap = CapIso
		value.Fresh = true
	case IntrinsicUnsafe:
		value.Cap = CapUntracked
	case IntrinsicSwap:
		value.Cap = CapInvalid
	default:
		value.Cap = CapInvalid
	}
	return value
}

func unparenExpr(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
