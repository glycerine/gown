package gown

import "golang.org/x/tools/go/packages"

func checkUntrackedCallBoundaries(pkg *packages.Package, caps *OstampIndex) CheckerErrors {
	return checkOstampErasureInPackage(pkg, caps, ostampErasureCalls)
}

func capTracked(cap Cap) bool {
	return cap != CapInvalid && cap != CapUntracked
}
