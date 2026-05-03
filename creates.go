package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// assignCreates walks the AST to find new(T), make(...), and &T{}
// creation points, recording each as a createAnew on gf.
// Only types in the reachable set (or all types if poisoned) are tracked.
func assignCreates(pkg *packages.Package, gf *gownFile, reachable map[types.Type]bool, poisoned bool) {
	structs := collectStructNames(pkg)
	info := pkg.TypesInfo

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			bodyBeg := pkg.Fset.Position(fn.Body.Lbrace).Offset
			bodyEndx := pkg.Fset.Position(fn.Body.Rbrace).Offset + 1
			bodyRegion := &region{beg: bodyBeg, endx: bodyEndx}

			var blocks []*region
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				bs, ok := n.(*ast.BlockStmt)
				if !ok {
					return true
				}
				beg := pkg.Fset.Position(bs.Lbrace).Offset
				endx := pkg.Fset.Position(bs.Rbrace).Offset + 1
				blocks = append(blocks, &region{beg: beg, endx: endx})
				return true
			})

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				var ca *createAnew

				switch x := n.(type) {
				case *ast.CallExpr:
					ident, ok := x.Fun.(*ast.Ident)
					if !ok || len(x.Args) == 0 {
						return true
					}
					switch ident.Name {
					case "new":
						tv, has := info.Types[x.Args[0]]
						if has && !isReachable(tv.Type, reachable, poisoned) {
							return true
						}
						ca = &createAnew{
							kind:     "new",
							typeName: baseTypeName(x.Args[0]),
						}
					case "make":
						tv, has := info.Types[x.Args[0]]
						if !has || !canHoldPointers(tv.Type) {
							return true
						}
						if !isReachable(tv.Type, reachable, poisoned) {
							return true
						}
						ca = &createAnew{
							kind:     "make",
							typeName: makeArgTypeName(x.Args[0], tv.Type),
						}
					}
					if ca != nil {
						pos := pkg.Fset.Position(x.Pos())
						ca.offset = pos.Offset
						ca.line = pos.Line
						ca.col = pos.Column
					}

				case *ast.UnaryExpr:
					if x.Op != token.AND {
						return true
					}
					cl, ok := x.X.(*ast.CompositeLit)
					if !ok || cl.Type == nil {
						return true
					}
					name, ok := classifyCompositeLit(cl.Type, info, structs)
					if !ok {
						return true
					}
					if tv, has := info.Types[cl.Type]; has {
						if !isReachable(tv.Type, reachable, poisoned) {
							return true
						}
					}
					pos := pkg.Fset.Position(x.Pos())
					ca = &createAnew{
						kind:     "ampersand",
						typeName: name,
						offset:   pos.Offset,
						line:     pos.Line,
						col:      pos.Column,
					}
				}

				if ca == nil {
					return true
				}

				ca.funcName = fn.Name.Name
				for _, blk := range blocks {
					if ca.offset >= blk.beg && ca.offset < blk.endx {
						if ca.scope == nil || (blk.endx-blk.beg) < (ca.scope.endx-ca.scope.beg) {
							ca.scope = blk
						}
					}
				}
				if ca.scope == nil {
					ca.scope = bodyRegion
				}
				gf.create = append(gf.create, ca)
				return true
			})
		}
	}
}

func collectStructNames(pkg *packages.Package) map[string]bool {
	names := make(map[string]bool)
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if _, isStruct := tn.Type().Underlying().(*types.Struct); isStruct {
			names[name] = true
		}
	}
	return names
}

// canHoldPointers reports whether a Go type can contain pointer values.
// It resolves named types through their underlying type.
func canHoldPointers(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return true
	case *types.Interface:
		return true
	case *types.Signature:
		return true
	case *types.Struct:
		return true
	case *types.Slice:
		return canHoldPointers(u.Elem())
	case *types.Array:
		return canHoldPointers(u.Elem())
	case *types.Map:
		return canHoldPointers(u.Key()) || canHoldPointers(u.Elem())
	case *types.Chan:
		return canHoldPointers(u.Elem())
	default:
		return false
	}
}

// classifyCompositeLit decides whether a &T{} composite literal should
// be tracked. Uses the type checker to resolve named and container types.
func classifyCompositeLit(expr ast.Expr, info *types.Info, structs map[string]bool) (string, bool) {
	if ident, ok := expr.(*ast.Ident); ok {
		if structs[ident.Name] {
			return ident.Name, true
		}
	}
	tv, ok := info.Types[expr]
	if !ok {
		return "", false
	}
	if !canHoldPointers(tv.Type) {
		return "", false
	}
	if mt, ok := expr.(*ast.MapType); ok {
		if u, ok := tv.Type.Underlying().(*types.Map); ok && canHoldPointers(u.Key()) {
			return baseTypeName(mt.Key), true
		}
		return baseTypeName(mt.Value), true
	}
	return baseTypeName(expr), true
}

// makeArgTypeName extracts the type name from a make() call's first argument,
// using the resolved type to pick the pointer-containing part for maps.
func makeArgTypeName(expr ast.Expr, t types.Type) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	if mt, ok := expr.(*ast.MapType); ok {
		if u, ok := t.Underlying().(*types.Map); ok && canHoldPointers(u.Key()) {
			return baseTypeName(mt.Key)
		}
		return baseTypeName(mt.Value)
	}
	return baseTypeName(expr)
}

func baseTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.ChanType:
		return baseTypeName(t.Value)
	case *ast.ArrayType:
		return baseTypeName(t.Elt)
	case *ast.MapType:
		return baseTypeName(t.Value)
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}
