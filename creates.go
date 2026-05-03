package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// assignCreates walks the AST to find new(T), make(...), and &T{}
// creation points, recording each as a createAnew on gf.
func assignCreates(pkg *packages.Package, gf *gownFile) {
	structs := collectStructNames(pkg)

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
						ca = &createAnew{
							kind:     "new",
							typeName: baseTypeName(x.Args[0]),
						}
					case "make":
						ca = &createAnew{
							kind:     "make",
							typeName: baseTypeName(x.Args[0]),
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
					name, ok := classifyCompositeLit(cl.Type, structs)
					if !ok {
						return true
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

// classifyCompositeLit decides whether a &T{} composite literal should
// be tracked. It returns the base type name and true if:
//   - T is a known struct name, or
//   - T is a slice/array whose element type is a pointer, or
//   - T is a map whose key or value type is a pointer.
//
// For maps with pointer keys, the key's base type name is returned.
func classifyCompositeLit(expr ast.Expr, structs map[string]bool) (string, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		if structs[t.Name] {
			return t.Name, true
		}
	case *ast.ArrayType:
		if _, isStar := t.Elt.(*ast.StarExpr); isStar {
			return baseTypeName(t.Elt), true
		}
	case *ast.MapType:
		if _, isStar := t.Key.(*ast.StarExpr); isStar {
			return baseTypeName(t.Key), true
		}
		if _, isStar := t.Value.(*ast.StarExpr); isStar {
			return baseTypeName(t.Value), true
		}
	}
	return "", false
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
