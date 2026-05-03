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
	Collapsed  bool
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
	if place.Collapsed {
		return place.RegionKey()
	}
	return PlaceKey{
		Root: place.Root,
		Path: place.Projection.String(),
	}
}

func (place Place) RegionKey() PlaceKey {
	return PlaceKey{Root: place.Root}
}

func (key PlaceKey) Overlaps(other PlaceKey) bool {
	if key.Root == nil || key.Root != other.Root {
		return false
	}
	if key.Path == "" || other.Path == "" || key.Path == other.Path {
		return true
	}
	return strings.HasPrefix(key.Path, other.Path+".") ||
		strings.HasPrefix(other.Path, key.Path+".")
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
		return resolveSelectorPlace(pkg, expr)
	case *ast.IndexExpr:
		return collapseRootPlace(pkg, expr.X)
	default:
		return Place{}, false
	}
}

func resolveSelectorPlace(pkg *packages.Package, expr *ast.SelectorExpr) (Place, bool) {
	base, ok := resolveRootPlace(pkg, expr.X)
	if !ok {
		return Place{}, false
	}
	if base.Collapsed {
		return base, true
	}
	selection := pkg.TypesInfo.Selections[expr]
	if selection == nil || selection.Kind() != types.FieldVal {
		return collapsePlace(base), true
	}
	field, _ := selection.Obj().(*types.Var)
	if field == nil {
		return collapsePlace(base), true
	}
	index := -1
	if path := selection.Index(); len(path) > 0 {
		index = path[len(path)-1]
	}
	base.Projection = append(base.Projection, FieldProjection{
		Name:  field.Name(),
		Index: index,
		Field: field,
	})
	return base, true
}

func collapseRootPlace(pkg *packages.Package, expr ast.Expr) (Place, bool) {
	place, ok := resolveRootPlace(pkg, expr)
	if !ok {
		return Place{}, false
	}
	return collapsePlace(place), true
}

func collapsePlace(place Place) Place {
	place.Projection = nil
	place.Collapsed = true
	return place
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
