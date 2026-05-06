package gown

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

func checkReadOnlyWrites(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			switch stmt := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range stmt.Lhs {
					if err, ok := checkWriteTarget(pkg, caps, lhs); ok {
						errs = append(errs, err)
					}
				}
			case *ast.IncDecStmt:
				if err, ok := checkWriteTarget(pkg, caps, stmt.X); ok {
					errs = append(errs, err)
				}
			}
			return true
		})
	}
	return errs
}

func checkBorrowStoreEscapes(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			stmt, ok := n.(*ast.AssignStmt)
			if !ok || len(stmt.Lhs) != len(stmt.Rhs) {
				return true
			}
			for i, lhs := range stmt.Lhs {
				if err, ok := checkBorrowStoreEscape(pkg, caps, lhs, stmt.Rhs[i]); ok {
					errs = append(errs, err)
				}
			}
			return true
		})
	}
	return errs
}

func checkBorrowStoreEscape(pkg *packages.Package, caps *CapabilityIndex, lhs, rhs ast.Expr) (CheckerError, bool) {
	rhsPlace, ok := caps.PlaceForExpr(rhs)
	if !ok || rhsPlace.Root == nil {
		return CheckerError{}, false
	}
	rhsCap := caps.ObjectCap(rhsPlace.Root)
	if rhsCap != CapMub && rhsCap != CapRob {
		return CheckerError{}, false
	}
	if !storeTargetEscapes(pkg, lhs) {
		return CheckerError{}, false
	}
	return newCheckerErrorAtNode(
		pkg,
		GWN006,
		rhs,
		fmt.Sprintf("cannot store %s borrow %q into escaping location", rhsCap, rhsPlace.Root.Name()),
	), true
}

func checkWriteTarget(pkg *packages.Package, caps *CapabilityIndex, expr ast.Expr) (CheckerError, bool) {
	if !isProjectedWrite(expr) {
		return CheckerError{}, false
	}
	place, ok := caps.PlaceForExpr(expr)
	if !ok || place.Root == nil {
		return CheckerError{}, false
	}
	cap := EffectivePlaceCap(caps, place)
	if cap != CapRob && cap != CapImm {
		return CheckerError{}, false
	}
	return newCheckerErrorAtNode(
		pkg,
		GWN005,
		expr,
		fmt.Sprintf("cannot write through %s value %q", cap, place.Root.Name()),
	), true
}

func isProjectedWrite(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.SelectorExpr, *ast.IndexExpr:
		return true
	default:
		return false
	}
}

func storeTargetEscapes(pkg *packages.Package, expr ast.Expr) bool {
	if isProjectedWrite(expr) {
		return true
	}
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	obj := objectForIdent(pkg, id)
	return obj != nil && obj.Parent() == pkg.Types.Scope()
}
