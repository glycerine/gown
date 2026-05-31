package gown

import (
	"fmt"
	"go/types"
)

func checkCloneIntrinsics(ctx *CheckerContext) CheckerErrors {
	if ctx == nil || ctx.Pkg == nil || ctx.Caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, binding := range ctx.Caps.IntrinsicBindings {
		if !isCloneIntrinsic(binding.Kind) {
			continue
		}
		if err, ok := checkCloneIntrinsic(ctx.Pkg.TypesInfo, ctx.Pkg.Types, binding); ok {
			errs = append(errs, err)
		}
	}
	return errs
}

func isCloneIntrinsic(kind IntrinsicKind) bool {
	return kind == IntrinsicClone || kind == IntrinsicCloneExported
}

func cloneIntrinsicMethodName(kind IntrinsicKind) string {
	if kind == IntrinsicCloneExported {
		return "Clone"
	}
	return "clone"
}

func checkCloneIntrinsic(info *types.Info, currentPkg *types.Package, binding IntrinsicBinding) (CheckerError, bool) {
	argType, ok := cloneArgType(info, binding)
	if !ok {
		return cloneIntrinsicError(binding, "cannot clone expression with invalid type"), true
	}
	if !cloneArgIsNamedStructOrPointer(argType) {
		return cloneIntrinsicError(binding, fmt.Sprintf("cannot clone non-struct type %s", argType)), true
	}
	methodName := cloneIntrinsicMethodName(binding.Kind)
	if !hasSameTypeCloneMethod(argType, currentPkg, methodName) {
		return cloneIntrinsicError(binding, fmt.Sprintf("type %s must define %s() %s", argType, methodName, argType)), true
	}
	return CheckerError{}, false
}

func cloneArgType(info *types.Info, binding IntrinsicBinding) (types.Type, bool) {
	if info == nil || binding.Arg == nil {
		return nil, false
	}
	argType := info.TypeOf(binding.Arg)
	return argType, argType != nil
}

func cloneArgIsNamedStructOrPointer(argType types.Type) bool {
	switch typ := argType.(type) {
	case *types.Named:
		_, ok := typ.Underlying().(*types.Struct)
		return ok
	case *types.Pointer:
		named, ok := typ.Elem().(*types.Named)
		if !ok {
			return false
		}
		_, ok = named.Underlying().(*types.Struct)
		return ok
	default:
		return false
	}
}

func hasSameTypeCloneMethod(argType types.Type, currentPkg *types.Package, methodName string) bool {
	lookupPkg := currentPkg
	if isExportedName(methodName) {
		lookupPkg = nil
	}
	selection := types.NewMethodSet(argType).Lookup(lookupPkg, methodName)
	if selection == nil {
		return false
	}
	fn, _ := selection.Obj().(*types.Func)
	if fn == nil {
		return false
	}
	sig, _ := fn.Type().(*types.Signature)
	if sig == nil || sig.Params().Len() != 0 || sig.Results().Len() != 1 {
		return false
	}
	return types.Identical(sig.Results().At(0).Type(), argType)
}

func isExportedName(name string) bool {
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

func cloneIntrinsicError(binding IntrinsicBinding, message string) CheckerError {
	return newCheckerErrorAtSource(
		GWN010,
		binding.Path,
		binding.Offset,
		binding.Line,
		binding.Col,
		message,
	)
}
