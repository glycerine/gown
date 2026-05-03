package gown

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/packages"
)

// assignCreates walks the AST to find new(T), make(...), and &T{}
// creation points, recording each as a createAnew on gf.
func assignCreates(pkg *packages.Package, gf *gownFile) {
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
					pos := pkg.Fset.Position(x.Pos())
					ca = &createAnew{
						kind:     "ampersand",
						typeName: baseTypeName(cl.Type),
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
