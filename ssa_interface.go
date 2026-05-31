package gown

import (
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkInterfaceErasureSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	return checkInterfaceErasure(pkg, caps)
}
