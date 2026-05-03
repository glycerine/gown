package gown

import (
	"fmt"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkStoreCapabilitiesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaStoreChecker{
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

type ssaStoreChecker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaStoreChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			store, ok := instr.(*ssa.Store)
			if !ok {
				continue
			}
			checker.checkStore(store)
		}
	}
}

func (checker *ssaStoreChecker) checkStore(store *ssa.Store) {
	checker.checkReadOnlyStore(store)
	checker.checkBorrowStoreEscape(store)
}

func (checker *ssaStoreChecker) checkReadOnlyStore(store *ssa.Store) {
	target, ok := checker.places.PlaceForValue(store.Addr)
	if !ok || target.Root == nil || !ssaStoreTargetIsProjected(target) {
		return
	}
	cap := capForSSAPlace(checker.caps, target)
	if cap != CapRob && cap != CapImm {
		return
	}
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN005,
		checker.pkg.Fset.Position(store.Pos()),
		fmt.Sprintf("cannot write through %s value %q", cap, target.Root.Name()),
	))
}

func (checker *ssaStoreChecker) checkBorrowStoreEscape(store *ssa.Store) {
	value, ok := checker.places.PlaceForValue(store.Val)
	if !ok || value.Root == nil {
		return
	}
	valueCap := capForSSAPlace(checker.caps, value)
	if valueCap != CapMub && valueCap != CapRob {
		return
	}
	target, ok := checker.places.PlaceForValue(store.Addr)
	if !ok || target.Root == nil || !ssaStoreTargetEscapes(checker.pkg, target) {
		return
	}
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN006,
		checker.pkg.Fset.Position(store.Pos()),
		fmt.Sprintf("cannot store %s borrow %q into escaping location", valueCap, value.Root.Name()),
	))
}

func ssaStoreTargetIsProjected(place Place) bool {
	return place.Collapsed || len(place.Projection) > 0
}

func ssaStoreTargetEscapes(pkg *packages.Package, place Place) bool {
	if ssaStoreTargetIsProjected(place) {
		return true
	}
	return pkg != nil && place.Root != nil && place.Root.Parent() == pkg.Types.Scope()
}

func (checker *ssaStoreChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
