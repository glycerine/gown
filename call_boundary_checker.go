package gown

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/packages"
)

func checkUntrackedCallBoundaries(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if _, ok := caps.CallBinding(call); ok {
				return true
			}
			callee := callCallee(pkg, call)
			if callee == nil {
				return true
			}
			if err, ok := checkUntrackedCallBoundary(pkg, caps, call); ok {
				errs = append(errs, err)
			}
			return true
		})
	}
	return errs
}

func checkUntrackedCallBoundary(pkg *packages.Package, caps *CapabilityIndex, call *ast.CallExpr) (CheckerError, bool) {
	for _, arg := range call.Args {
		place, ok := caps.PlaceForExpr(arg)
		if !ok || place.Root == nil {
			continue
		}
		cap := caps.ObjectCap(place.Root)
		if !capTracked(cap) {
			continue
		}
		pos := pkg.Fset.Position(arg.Pos())
		return CheckerError{
			Code:    GWN008,
			Path:    gownSourcePath(pos.Filename),
			Offset:  pos.Offset,
			Line:    pos.Line,
			Col:     pos.Column,
			Message: fmt.Sprintf("cannot pass %s value %q to untracked function", cap, place.Root.Name()),
		}, true
	}
	return CheckerError{}, false
}

func capTracked(cap Cap) bool {
	return cap != CapInvalid && cap != CapUntracked
}
