package gown

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type ValueCapability struct {
	Cap         Cap
	ChanElemCap Cap
	Place       Place
	Fresh       bool
	Source      Place
	Intrinsic   IntrinsicKind
}

func (idx *CapabilityIndex) ValueCapability(expr ast.Expr) (ValueCapability, bool) {
	return valueCapabilityForExpr(nil, idx, expr)
}

func valueCapabilityForExpr(pkg *packages.Package, idx *CapabilityIndex, expr ast.Expr) (ValueCapability, bool) {
	if idx == nil || expr == nil {
		return ValueCapability{}, false
	}
	expr = unparenExpr(expr)
	if call, ok := expr.(*ast.CallExpr); ok {
		if binding, ok := idx.IntrinsicBinding(call); ok {
			return intrinsicValueCapability(binding), true
		}
		if value, ok := callResultValueCapability(pkg, idx, call); ok {
			return value, true
		}
	}
	if isFreshOwnedValueExpr(expr) {
		return ValueCapability{Cap: CapIso, Fresh: true}, true
	}
	if place, ok := idx.PlaceForExpr(expr); ok {
		return ValueCapability{
			Cap:   capForSSAPlace(idx, place),
			Place: place,
		}, true
	}
	return ValueCapability{}, false
}

func callResultValueCapability(pkg *packages.Package, idx *CapabilityIndex, call *ast.CallExpr) (ValueCapability, bool) {
	if pkg == nil || idx == nil || call == nil {
		return ValueCapability{}, false
	}
	callee := callCallee(pkg, call)
	funcCap := idx.FuncCap(callee)
	if funcCap == nil || len(funcCap.Results) != 1 || !capTracked(funcCap.Results[0]) {
		return ValueCapability{}, false
	}
	return ValueCapability{
		Cap:   funcCap.Results[0],
		Fresh: funcCap.Results[0] == CapIso,
	}, true
}

func intrinsicValueCapability(binding IntrinsicBinding) ValueCapability {
	value := ValueCapability{
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
