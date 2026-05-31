package gown

import "golang.org/x/tools/go/packages"

func checkUntrackedCallBoundaries(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	return checkCapabilityErasureInPackage(pkg, caps, capabilityErasureCalls)
}

func capTracked(cap Cap) bool {
	return cap != CapInvalid && cap != CapUntracked
}
