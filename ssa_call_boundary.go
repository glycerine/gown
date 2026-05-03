package gown

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkUntrackedCallBoundariesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaCallBoundaryChecker{
		pkg:      pkg,
		caps:     caps,
		places:   buildSSAPlaceIndex(pkg, ssaPkg, caps),
		reported: make(map[string]bool),
	}
	for _, fn := range collectSSAFunctions(ssaPkg) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaCallBoundaryChecker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaCallBoundaryChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			call, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}
			checker.checkCall(call)
		}
	}
}

func (checker *ssaCallBoundaryChecker) checkCall(call *ssa.Call) {
	callee := call.Call.StaticCallee()
	if callee == nil {
		return
	}
	fn, _ := callee.Object().(*types.Func)
	if checker.caps.FuncCap(fn) != nil {
		return
	}
	for _, arg := range call.Call.Args {
		place, ok := checker.places.PlaceForValue(arg)
		if !ok || place.Root == nil {
			continue
		}
		cap := capForSSAPlace(checker.caps, place)
		if !capTracked(cap) {
			continue
		}
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN008,
			checker.pkg.Fset.Position(call.Pos()),
			fmt.Sprintf("cannot pass %s value %q to untracked function", cap, place.Root.Name()),
		))
		return
	}
}

func (checker *ssaCallBoundaryChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
