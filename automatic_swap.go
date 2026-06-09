package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type automaticSwap struct {
	Places [2]Place
	Pos    token.Position
}

func automaticSwapForAssign(pkg *packages.Package, caps *OstampIndex, assign *ast.AssignStmt) (automaticSwap, bool) {
	if pkg == nil || pkg.TypesInfo == nil || caps == nil || assign == nil {
		return automaticSwap{}, false
	}
	if assign.Tok != token.ASSIGN || len(assign.Lhs) != 2 || len(assign.Rhs) != 2 {
		return automaticSwap{}, false
	}
	if !isSwapAssignablePlace(assign.Lhs[0]) || !isSwapAssignablePlace(assign.Lhs[1]) {
		return automaticSwap{}, false
	}

	lhs0, ok := caps.PlaceForExpr(assign.Lhs[0])
	if !ok || lhs0.Root == nil {
		return automaticSwap{}, false
	}
	lhs1, ok := caps.PlaceForExpr(assign.Lhs[1])
	if !ok || lhs1.Root == nil {
		return automaticSwap{}, false
	}
	rhs0, ok := caps.PlaceForExpr(assign.Rhs[0])
	if !ok || rhs0.Root == nil {
		return automaticSwap{}, false
	}
	rhs1, ok := caps.PlaceForExpr(assign.Rhs[1])
	if !ok || rhs1.Root == nil {
		return automaticSwap{}, false
	}
	if lhs0.Key() != rhs1.Key() || lhs1.Key() != rhs0.Key() {
		return automaticSwap{}, false
	}
	if capForSSAPlace(caps, lhs0) != CapIso || capForSSAPlace(caps, lhs1) != CapIso {
		return automaticSwap{}, false
	}
	if !automaticSwapTypesIdentical(pkg, assign.Lhs[0], assign.Lhs[1]) {
		return automaticSwap{}, false
	}
	return automaticSwap{
		Places: [2]Place{lhs0, lhs1},
		Pos:    pkg.Fset.Position(assign.Pos()),
	}, true
}

func automaticSwapTypesIdentical(pkg *packages.Package, left, right ast.Expr) bool {
	if pkg == nil || pkg.TypesInfo == nil {
		return false
	}
	leftType := pkg.TypesInfo.TypeOf(left)
	rightType := pkg.TypesInfo.TypeOf(right)
	return leftType != nil && rightType != nil && types.Identical(leftType, rightType)
}
