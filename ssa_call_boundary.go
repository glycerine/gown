package gown

import (
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkUntrackedCallBoundariesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	return nil
}
