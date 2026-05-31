package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func checkInterfaceErasure(pkg *packages.Package, caps *OstampIndex) CheckerErrors {
	return checkOstampErasureInPackage(pkg, caps, capabilityErasureInterfaces)
}

func checkInterfaceAssign(pkg *packages.Package, caps *OstampIndex, stmt *ast.AssignStmt) CheckerErrors {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return nil
	}
	var errs CheckerErrors
	for i, lhs := range stmt.Lhs {
		if !isInterfaceExpr(pkg, lhs) {
			continue
		}
		if err, ok := checkInterfaceSource(pkg, caps, stmt.Rhs[i]); ok {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkInterfaceValueSpec(pkg *packages.Package, caps *OstampIndex, spec *ast.ValueSpec) CheckerErrors {
	if len(spec.Names) != len(spec.Values) {
		return nil
	}
	var errs CheckerErrors
	for i, name := range spec.Names {
		obj, _ := pkg.TypesInfo.Defs[name].(*types.Var)
		if obj == nil || !isInterfaceType(obj.Type()) {
			continue
		}
		if err, ok := checkInterfaceSource(pkg, caps, spec.Values[i]); ok {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkInterfaceSource(pkg *packages.Package, caps *OstampIndex, src ast.Expr) (CheckerError, bool) {
	place, ok := caps.PlaceForExpr(src)
	if !ok || place.Root == nil {
		return CheckerError{}, false
	}
	cap := caps.ObjectCap(place.Root)
	if !capTracked(cap) {
		return CheckerError{}, false
	}
	return newCheckerErrorAtNode(
		pkg,
		GWN009,
		src,
		fmt.Sprintf("cannot erase %s value %q into interface", cap, place.Root.Name()),
	), true
}

func isInterfaceExpr(pkg *packages.Package, expr ast.Expr) bool {
	tv, ok := pkg.TypesInfo.Types[expr]
	return ok && isInterfaceType(tv.Type)
}

func isInterfaceType(t types.Type) bool {
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Interface)
	return ok
}
