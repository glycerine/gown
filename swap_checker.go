package gown

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func checkSwapIntrinsics(ctx *CheckerContext) CheckerErrors {
	if ctx == nil || ctx.Pkg == nil || ctx.Caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, binding := range ctx.Caps.IntrinsicBindings {
		if binding.Kind != IntrinsicSwap {
			continue
		}
		if err, ok := checkSwapIntrinsic(ctx.Pkg, ctx.Caps, binding); ok {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkSwapIntrinsic(pkg *packages.Package, caps *OstampIndex, binding IntrinsicBinding) (CheckerError, bool) {
	if len(binding.Args) != 2 {
		return swapIntrinsicError(binding, fmt.Sprintf("\\swap requires exactly two arguments, got %d", len(binding.Args))), true
	}
	if len(binding.ArgPlaces) != len(binding.Args) {
		return swapIntrinsicError(binding, "\\swap arguments were not bound to ownerstamp places"), true
	}

	var argTypes [2]types.Type
	for i, arg := range binding.Args {
		if !isSwapAssignablePlace(arg) {
			return swapIntrinsicError(binding, fmt.Sprintf("\\swap argument %d must be an assignable local or field place", i+1)), true
		}
		place := binding.ArgPlaces[i]
		if place.Root == nil {
			return swapIntrinsicError(binding, fmt.Sprintf("\\swap argument %d is not a tracked place", i+1)), true
		}
		cap := capForSSAPlace(caps, place)
		if cap != CapIso {
			if message, ok := swapUntrackedRootIsoFieldMessage(caps, i+1, place); ok {
				return swapIntrinsicError(binding, message), true
			}
			return swapIntrinsicError(binding, fmt.Sprintf("\\swap argument %d %q has %s ownerstamp; \\swap requires \\iso", i+1, placeName(place), cap)), true
		}
		if pkg == nil || pkg.TypesInfo == nil {
			return swapIntrinsicError(binding, fmt.Sprintf("\\swap argument %d has invalid type", i+1)), true
		}
		argTypes[i] = pkg.TypesInfo.TypeOf(arg)
		if argTypes[i] == nil {
			return swapIntrinsicError(binding, fmt.Sprintf("\\swap argument %d has invalid type", i+1)), true
		}
	}
	if !types.Identical(argTypes[0], argTypes[1]) {
		return swapIntrinsicError(binding, fmt.Sprintf("\\swap arguments must have identical types, got %s and %s",
			swapTypeString(pkg, argTypes[0]), swapTypeString(pkg, argTypes[1]))), true
	}
	return CheckerError{}, false
}

func isSwapAssignablePlace(expr ast.Expr) bool {
	switch expr := unparenExpr(expr).(type) {
	case *ast.Ident:
		return expr.Name != "_"
	case *ast.SelectorExpr:
		return expr.Sel != nil && expr.Sel.Name != "_"
	default:
		return false
	}
}

func swapUntrackedRootIsoFieldMessage(caps *OstampIndex, index int, place Place) (string, bool) {
	if caps == nil || place.Root == nil || len(place.Projection) == 0 {
		return "", false
	}
	if caps.ObjectCap(place.Root) != CapUntracked {
		return "", false
	}
	field := place.Projection[len(place.Projection)-1].Field
	if field == nil || caps.ObjectCap(field) != CapIso {
		return "", false
	}
	return fmt.Sprintf("\\swap argument %d %q names an \\iso field through untracked root %q; \\swap requires \\iso ownership of the container root", index, placeName(place), place.Root.Name()), true
}

func swapTypeString(pkg *packages.Package, typ types.Type) string {
	return types.TypeString(typ, func(p *types.Package) string {
		if p == nil {
			return ""
		}
		if pkg != nil && pkg.Types != nil && p == pkg.Types {
			return ""
		}
		return p.Name()
	})
}

func swapIntrinsicError(binding IntrinsicBinding, message string) CheckerError {
	return newCheckerErrorAtSource(
		GWN010,
		binding.Path,
		binding.Offset,
		binding.Line,
		binding.Col,
		message,
	)
}
