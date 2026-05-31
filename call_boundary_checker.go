package gown

import "golang.org/x/tools/go/packages"

func checkUntrackedCallBoundaries(pkg *packages.Package, caps *OstampIndex) CheckerErrors {
	return checkOstampErasureInPackage(pkg, caps, capabilityErasureCalls)
}

func capTracked(cap Cap) bool {
	return cap != CapInvalid && cap != CapUntracked
}
