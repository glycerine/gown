package gown

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

// assignRegions walks the AST to find the containing function and
// innermost { } block for each \iso annotation.
//
// Annotations in a function signature (parameters, return types)
// get the function body as their scope.
func assignRegions(pkg *packages.Package, gf *gownFile) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			fnBeg := pkg.Fset.Position(fn.Pos()).Offset
			fnEnd := pkg.Fset.Position(fn.End()).Offset

			bodyBeg := pkg.Fset.Position(fn.Body.Lbrace).Offset
			bodyEndx := pkg.Fset.Position(fn.Body.Rbrace).Offset + 1
			bodyRegion := &region{beg: bodyBeg, endx: bodyEndx}

			// Collect all block scopes inside this function.
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

			for _, ann := range gf.iso {
				if ann.offset < fnBeg || ann.offset >= fnEnd {
					continue
				}
				ann.funcName = fn.Name.Name

				// Find innermost block containing the annotation.
				for _, blk := range blocks {
					if ann.offset >= blk.beg && ann.offset < blk.endx {
						if ann.scope == nil || (blk.endx-blk.beg) < (ann.scope.endx-ann.scope.beg) {
							ann.scope = blk
						}
					}
				}
				// Signature-level annotations get the function body.
				if ann.scope == nil {
					ann.scope = bodyRegion
				}
			}
		}
	}
}
