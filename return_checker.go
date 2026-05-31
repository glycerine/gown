package gown

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

func checkReturnBorrowEscapes(pkg *packages.Package, caps *OstampIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			for _, result := range ret.Results {
				if err, ok := checkReturnBorrowEscape(pkg, caps, result); ok {
					errs = append(errs, err)
				}
			}
			return true
		})
	}
	return errs
}

func checkReturnBorrowEscape(pkg *packages.Package, caps *OstampIndex, result ast.Expr) (CheckerError, bool) {
	place, ok := caps.PlaceForExpr(result)
	if !ok || place.Root == nil {
		return CheckerError{}, false
	}
	cap := caps.ObjectCap(place.Root)
	if cap != CapMub && cap != CapRob {
		return CheckerError{}, false
	}
	return newCheckerErrorAtNode(
		pkg,
		GWN007,
		result,
		fmt.Sprintf("cannot return %s borrow %q", cap, place.Root.Name()),
	), true
}
