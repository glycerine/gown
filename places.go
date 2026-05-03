package gown

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Place struct {
	Root       types.Object
	Projection Projection
}

type Projection []FieldProjection

type FieldProjection struct {
	Name  string
	Index int
	Field *types.Var
}

type PlaceKey struct {
	Root types.Object
	Path string
}

type PlaceIndex struct {
	exprPlaces map[ast.Expr]Place
}

func (place Place) Key() PlaceKey {
	return PlaceKey{
		Root: place.Root,
		Path: place.Projection.String(),
	}
}

func (projection Projection) String() string {
	if len(projection) == 0 {
		return ""
	}
	var b strings.Builder
	for _, field := range projection {
		b.WriteByte('.')
		b.WriteString(field.Name)
	}
	return b.String()
}

func (idx *PlaceIndex) PlaceForExpr(expr ast.Expr) (Place, bool) {
	if idx == nil || expr == nil {
		return Place{}, false
	}
	place, ok := idx.exprPlaces[expr]
	return place, ok
}

func buildPlaceIndex(pkg *packages.Package) *PlaceIndex {
	idx := &PlaceIndex{
		exprPlaces: make(map[ast.Expr]Place),
	}
	if pkg == nil {
		return idx
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			place, ok := resolveRootPlace(pkg, expr)
			if ok {
				idx.exprPlaces[expr] = place
			}
			return true
		})
	}
	return idx
}

func resolveRootPlace(pkg *packages.Package, expr ast.Expr) (Place, bool) {
	switch expr := expr.(type) {
	case *ast.Ident:
		obj := objectForIdent(pkg, expr)
		if obj == nil {
			return Place{}, false
		}
		return Place{Root: obj}, true
	case *ast.ParenExpr:
		return resolveRootPlace(pkg, expr.X)
	case *ast.SelectorExpr:
		return resolveRootPlace(pkg, expr.X)
	case *ast.IndexExpr:
		return resolveRootPlace(pkg, expr.X)
	default:
		return Place{}, false
	}
}

func directRootPlace(pkg *packages.Package, expr ast.Expr) (Place, bool) {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return Place{}, false
	}
	obj := objectForIdent(pkg, id)
	if obj == nil {
		return Place{}, false
	}
	return Place{Root: obj}, true
}

func objectForIdent(pkg *packages.Package, id *ast.Ident) types.Object {
	if pkg == nil || id == nil {
		return nil
	}
	obj := pkg.TypesInfo.Uses[id]
	if obj == nil {
		obj = pkg.TypesInfo.Defs[id]
	}
	if _, ok := obj.(*types.Var); !ok {
		return nil
	}
	return obj
}
