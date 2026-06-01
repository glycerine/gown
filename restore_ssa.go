package gown

import (
	"go/ast"

	"golang.org/x/tools/go/ssa"
)

func collectSSAFunctionsForChecking(ssaPkg *ssa.Package, caps *OstampIndex) []*ssa.Function {
	funcs := collectSSAFunctions(ssaPkg)
	if caps == nil || len(caps.RestoreBindings) == 0 {
		return funcs
	}
	out := funcs[:0]
	for _, fn := range funcs {
		if isRestoreSSAFunction(caps, fn) {
			continue
		}
		out = append(out, fn)
	}
	return out
}

func isRestoreSSAFunction(caps *OstampIndex, fn *ssa.Function) bool {
	if caps == nil || fn == nil {
		return false
	}
	lit, _ := fn.Syntax().(*ast.FuncLit)
	return caps.IsRestoreFuncLit(lit)
}
