package gown

import (
	"reflect"
	"runtime"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type CheckerContext struct {
	Pkg    *packages.Package
	SSAPkg *ssa.Package
	Caps   *OstampIndex
}

type CheckerPass func(*CheckerContext) CheckerErrors

var checkerPasses = []CheckerPass{
	checkRestoreRegions,
	checkCloneIntrinsics,
	checkSwapIntrinsics,
	checkChannelElementDeclarations,
	checkOstampErasure,
	checkMovedUses,
	checkCallBorrows,
	checkSends,
	checkGoEscapes,
	checkClosureEscapes,
	checkStores,
	checkReturns,
}

func runCheckerPasses(pkg *packages.Package, ssaPkg *ssa.Package, caps *OstampIndex) CheckerErrors {
	ctx := &CheckerContext{Pkg: pkg, SSAPkg: ssaPkg, Caps: caps}
	//vv("GOWN checker runner start passes=%d ssaPkgNil=%v", len(checkerPasses), ssaPkg == nil)
	for i, pass := range checkerPasses {
		_ = i

		//vv("GOWN checker pass[%d] begin name=%s", i, checkerPassName(pass))
		passErrs := pass(ctx)
		//vv("GOWN checker pass[%d] complete name=%s errors=%d", i, checkerPassName(pass), len(passErrs))
		if len(passErrs) > 0 {
			//vv("GOWN checker pass[%d] first error name=%s err=%v", i, checkerPassName(pass), passErrs[0])
			//vv("GOWN checker runner stop after first error pass[%d] name=%s", i, checkerPassName(pass))
			return CheckerErrors{passErrs[0]}
		}
	}
	//vv("GOWN checker runner complete totalErrors=0")
	return nil
}

func checkerPassName(pass CheckerPass) string {
	if pass == nil {
		return "<nil>"
	}
	fn := runtime.FuncForPC(reflect.ValueOf(pass).Pointer())
	if fn == nil {
		return "<unknown>"
	}
	return fn.Name()
}

func checkMovedUses(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkGWN001SSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkGWN001(ctx.Pkg, ctx.Caps)
}

func checkCallBorrows(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkGWN002SSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkCallBorrowConflicts(ctx.Caps)
}

func checkSends(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkSendCapabilitiesSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkSendCapabilities(ctx.Pkg, ctx.Caps)
}

func checkGoEscapes(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkGoBorrowEscapesSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkGoBorrowEscapes(ctx.Pkg, ctx.Caps)
}

func checkClosureEscapes(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkClosureEscapesSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return nil
}

func checkStores(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkStoreCapabilitiesSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	var errs CheckerErrors
	errs = append(errs, checkReadOnlyWrites(ctx.Pkg, ctx.Caps)...)
	errs = append(errs, checkBorrowStoreEscapes(ctx.Pkg, ctx.Caps)...)
	return errs
}

func checkReturns(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkReturnBorrowEscapesSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkReturnBorrowEscapes(ctx.Pkg, ctx.Caps)
}

func checkUntrackedCalls(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkUntrackedCallBoundariesSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkUntrackedCallBoundaries(ctx.Pkg, ctx.Caps)
}

func checkInterfaces(ctx *CheckerContext) CheckerErrors {
	if ctx.SSAPkg != nil {
		return checkInterfaceErasureSSA(ctx.Pkg, ctx.SSAPkg, ctx.Caps)
	}
	return checkInterfaceErasure(ctx.Pkg, ctx.Caps)
}
