package gown

import "golang.org/x/tools/go/packages"

type CheckerContext struct {
	Pkg  *packages.Package
	Caps *CapabilityIndex
}

type CheckerPass func(*CheckerContext) CheckerErrors

var checkerPasses = []CheckerPass{
	checkMovedUses,
	checkCallBorrows,
	checkSends,
	checkGoEscapes,
	checkReadOnlyStores,
	checkBorrowStores,
	checkReturns,
	checkUntrackedCalls,
	checkInterfaces,
}

func runCheckerPasses(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	ctx := &CheckerContext{Pkg: pkg, Caps: caps}
	var errs CheckerErrors
	for _, pass := range checkerPasses {
		errs = append(errs, pass(ctx)...)
	}
	return errs
}

func checkMovedUses(ctx *CheckerContext) CheckerErrors {
	return checkGWN001(ctx.Pkg, ctx.Caps)
}

func checkCallBorrows(ctx *CheckerContext) CheckerErrors {
	return checkCallBorrowConflicts(ctx.Caps)
}

func checkSends(ctx *CheckerContext) CheckerErrors {
	return checkSendCapabilities(ctx.Pkg, ctx.Caps)
}

func checkGoEscapes(ctx *CheckerContext) CheckerErrors {
	return checkGoBorrowEscapes(ctx.Pkg, ctx.Caps)
}

func checkReadOnlyStores(ctx *CheckerContext) CheckerErrors {
	return checkReadOnlyWrites(ctx.Pkg, ctx.Caps)
}

func checkBorrowStores(ctx *CheckerContext) CheckerErrors {
	return checkBorrowStoreEscapes(ctx.Pkg, ctx.Caps)
}

func checkReturns(ctx *CheckerContext) CheckerErrors {
	return checkReturnBorrowEscapes(ctx.Pkg, ctx.Caps)
}

func checkUntrackedCalls(ctx *CheckerContext) CheckerErrors {
	return checkUntrackedCallBoundaries(ctx.Pkg, ctx.Caps)
}

func checkInterfaces(ctx *CheckerContext) CheckerErrors {
	return checkInterfaceErasure(ctx.Pkg, ctx.Caps)
}
