package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type goCapture struct {
	Ident *ast.Ident
	Place Place
	Cap   Cap
}

func goClosureCaptures(pkg *packages.Package, caps *OstampIndex, stmt *ast.GoStmt) []goCapture {
	if pkg == nil || caps == nil || stmt == nil {
		return nil
	}
	lit := goFuncLit(stmt)
	if lit == nil || lit.Body == nil {
		return nil
	}
	seen := make(map[PlaceKey]bool)
	var captures []goCapture
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj, _ := pkg.TypesInfo.Uses[id].(*types.Var)
		if obj == nil || posInNode(obj.Pos(), lit) {
			return true
		}
		place, ok := caps.PlaceForExpr(id)
		if !ok {
			return true
		}
		key := place.RegionKey()
		if key.Root == nil || seen[key] {
			return true
		}
		seen[key] = true
		captures = append(captures, goCapture{
			Ident: id,
			Place: place,
			Cap:   caps.ObjectCap(key.Root),
		})
		return true
	})
	return captures
}

func goFuncLit(stmt *ast.GoStmt) *ast.FuncLit {
	if stmt == nil || stmt.Call == nil {
		return nil
	}
	switch fun := stmt.Call.Fun.(type) {
	case *ast.FuncLit:
		return fun
	case *ast.ParenExpr:
		lit, _ := fun.X.(*ast.FuncLit)
		return lit
	default:
		return nil
	}
}

func posInNode(pos token.Pos, node ast.Node) bool {
	return node != nil && node.Pos() <= pos && pos <= node.End()
}
