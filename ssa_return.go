package gown

import (
	"fmt"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkReturnBorrowEscapesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaReturnChecker{
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

type ssaReturnChecker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaReturnChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}
			checker.checkReturn(ret)
		}
	}
}

func (checker *ssaReturnChecker) checkReturn(ret *ssa.Return) {
	for _, result := range ret.Results {
		place, ok := checker.places.PlaceForValue(result)
		if !ok || place.Root == nil {
			continue
		}
		cap := capForSSAPlace(checker.caps, place)
		if cap != CapMub && cap != CapRob {
			continue
		}
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN007,
			checker.pkg.Fset.Position(ret.Pos()),
			fmt.Sprintf("cannot return %s borrow %q", cap, place.Root.Name()),
		))
	}
}

func (checker *ssaReturnChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
