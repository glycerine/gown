package gown

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

func checkGoBorrowEscapes(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			stmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}
			if err, ok := checkGoCallBorrowEscape(caps, stmt); ok {
				errs = append(errs, err)
			}
			errs = append(errs, checkGoClosureBorrowCaptures(pkg, caps, stmt)...)
			return true
		})
	}
	return errs
}

func checkGoCallBorrowEscape(caps *CapabilityIndex, stmt *ast.GoStmt) (CheckerError, bool) {
	binding, ok := caps.CallBinding(stmt.Call)
	if !ok {
		return CheckerError{}, false
	}
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapMub && paramCap != CapRob {
			continue
		}
		name := "<unknown>"
		if i < len(binding.ArgPlaces) {
			if root := binding.ArgPlaces[i].RegionKey().Root; root != nil {
				name = root.Name()
			}
		}
		return newCheckerErrorAtSource(
			GWN004,
			binding.Path,
			binding.Offset,
			binding.Line,
			binding.Col,
			fmt.Sprintf("cannot pass inferred %s borrow of %q to goroutine", paramCap, name),
		), true
	}
	return CheckerError{}, false
}

func checkGoClosureBorrowCaptures(pkg *packages.Package, caps *CapabilityIndex, stmt *ast.GoStmt) CheckerErrors {
	var errs CheckerErrors
	pos := pkg.Fset.Position(stmt.Go)
	for _, capture := range goClosureCaptures(pkg, caps, stmt) {
		if capture.Cap != CapMub && capture.Cap != CapRob {
			continue
		}
		name := "<unknown>"
		if root := capture.Place.RegionKey().Root; root != nil {
			name = root.Name()
		}
		errs = append(errs, newCheckerErrorAtPosition(
			GWN004,
			pos,
			fmt.Sprintf("cannot capture non-sendable %s value %q in goroutine", capture.Cap, name),
		))
	}
	return errs
}
