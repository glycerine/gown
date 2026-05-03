package gown

import (
	"fmt"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func (gp *GownPackage) buildSSA() error {
	if gp.pkg == nil {
		return fmt.Errorf("cannot build SSA before package load")
	}
	prog, pkgs := ssautil.Packages([]*packages.Package{gp.pkg}, ssa.SanityCheckFunctions|ssa.GlobalDebug)
	if len(pkgs) == 0 || pkgs[0] == nil {
		return fmt.Errorf("SSA package not found for %s", gp.path)
	}
	prog.Build()
	gp.ssaProg = prog
	gp.ssaPkg = pkgs[0]
	return nil
}
