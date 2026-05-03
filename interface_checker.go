package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func checkInterfaceErasure(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			switch stmt := n.(type) {
			case *ast.AssignStmt:
				errs = append(errs, checkInterfaceAssign(pkg, caps, stmt)...)
			case *ast.ValueSpec:
				errs = append(errs, checkInterfaceValueSpec(pkg, caps, stmt)...)
			}
			return true
		})
	}
	return errs
}

func checkInterfaceAssign(pkg *packages.Package, caps *CapabilityIndex, stmt *ast.AssignStmt) CheckerErrors {
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

func checkInterfaceValueSpec(pkg *packages.Package, caps *CapabilityIndex, spec *ast.ValueSpec) CheckerErrors {
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

func checkInterfaceSource(pkg *packages.Package, caps *CapabilityIndex, src ast.Expr) (CheckerError, bool) {
	place, ok := caps.PlaceForExpr(src)
	if !ok || place.Root == nil {
		return CheckerError{}, false
	}
	cap := caps.ObjectCap(place.Root)
	if !capTracked(cap) {
		return CheckerError{}, false
	}
	pos := pkg.Fset.Position(src.Pos())
	return CheckerError{
		Code:    GWN009,
		Path:    gownSourcePath(pos.Filename),
		Offset:  pos.Offset,
		Line:    pos.Line,
		Col:     pos.Column,
		Message: fmt.Sprintf("cannot erase %s value %q into interface", cap, place.Root.Name()),
	}, true
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
