package gown

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func isObserverCall(pkg *packages.Package, caps *CapabilityIndex, call *ast.CallExpr) bool {
	if caps == nil || call == nil {
		return false
	}
	for _, target := range observerCallTargets(pkg, call) {
		if caps.Observer(target) {
			return true
		}
	}
	return false
}

func observerCallTargets(pkg *packages.Package, call *ast.CallExpr) []string {
	var targets []string
	if call == nil {
		return targets
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		targets = append(targets, fun.Name)
	case *ast.SelectorExpr:
		targets = append(targets, fun.Sel.Name)
		if x, ok := fun.X.(*ast.Ident); ok {
			targets = append(targets, x.Name+"."+fun.Sel.Name)
		}
	}
	if fn := callCallee(pkg, call); fn != nil {
		targets = appendFunctionObserverTargets(targets, fn)
	}
	return targets
}

func appendFunctionObserverTargets(targets []string, fn *types.Func) []string {
	if fn == nil {
		return targets
	}
	targets = append(targets, fn.Name())
	if full := fn.FullName(); full != "" {
		targets = append(targets, full)
	}
	return targets
}
