package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// assignBoundary walks the AST to find all goroutine boundary crossings:
// channel sends/receives, go-statement arguments, closure captures,
// package-level variables, and imported function return types.
func assignBoundary(pkg *packages.Package, gf *gownFile) {
	info := pkg.TypesInfo

	// Package-level variables.
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		v, ok := obj.(*types.Var)
		if !ok {
			continue
		}
		if !canHoldPointers(v.Type()) {
			continue
		}
		pos := pkg.Fset.Position(v.Pos())
		gf.boundary = append(gf.boundary, &boundaryCrossing{
			offset:   pos.Offset,
			line:     pos.Line,
			col:      pos.Column,
			kind:     "pkg-var",
			typeName: typeNameFromGoType(v.Type()),
			goType:   v.Type(),
		})
	}

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
				switch x := n.(type) {
				case *ast.SendStmt:
					addChanBoundary(pkg, gf, x.Chan, "chan-send", fn.Name.Name, blocks, bodyRegion)

				case *ast.UnaryExpr:
					if x.Op == token.ARROW {
						addChanBoundary(pkg, gf, x.X, "chan-recv", fn.Name.Name, blocks, bodyRegion)
					}

				case *ast.GoStmt:
					addGoBoundary(pkg, info, gf, x, fn.Name.Name, blocks, bodyRegion)

				case *ast.CallExpr:
					addImportSigBoundary(pkg, info, gf, x, fn.Name.Name, blocks, bodyRegion)
				}
				return true
			})
		}
	}
}

func addChanBoundary(pkg *packages.Package, gf *gownFile, chanExpr ast.Expr, kind, funcName string, blocks []*region, bodyRegion *region) {
	tv, ok := pkg.TypesInfo.Types[chanExpr]
	if !ok {
		return
	}
	ch, ok := tv.Type.Underlying().(*types.Chan)
	if !ok {
		return
	}
	elem := ch.Elem()
	if !canHoldPointers(elem) {
		return
	}
	pos := pkg.Fset.Position(chanExpr.Pos())
	bc := &boundaryCrossing{
		offset:   pos.Offset,
		line:     pos.Line,
		col:      pos.Column,
		kind:     kind,
		typeName: typeNameFromGoType(elem),
		goType:   elem,
		funcName: funcName,
	}
	bc.scope = findInnermostBlock(bc.offset, blocks, bodyRegion)
	gf.boundary = append(gf.boundary, bc)
}

func addGoBoundary(pkg *packages.Package, info *types.Info, gf *gownFile, goStmt *ast.GoStmt, funcName string, blocks []*region, bodyRegion *region) {
	call := goStmt.Call

	// Go-statement arguments.
	if len(call.Args) > 0 {
		var sig *types.Signature
		if tv, ok := info.Types[call.Fun]; ok {
			sig, _ = tv.Type.Underlying().(*types.Signature)
		}
		for i, arg := range call.Args {
			var argType types.Type
			if sig != nil && i < sig.Params().Len() {
				argType = sig.Params().At(i).Type()
			} else if tv, ok := info.Types[arg]; ok {
				argType = tv.Type
			}
			if argType == nil || !canHoldPointers(argType) {
				continue
			}
			pos := pkg.Fset.Position(arg.Pos())
			bc := &boundaryCrossing{
				offset:   pos.Offset,
				line:     pos.Line,
				col:      pos.Column,
				kind:     "go-arg",
				typeName: typeNameFromGoType(argType),
				goType:   argType,
				funcName: funcName,
			}
			bc.scope = findInnermostBlock(bc.offset, blocks, bodyRegion)
			gf.boundary = append(gf.boundary, bc)
		}
	}

	// Closure captures: find free variables in the FuncLit.
	funcLit, ok := call.Fun.(*ast.FuncLit)
	if !ok {
		return
	}
	// Collect objects declared inside the FuncLit.
	localObjs := make(map[types.Object]bool)
	ast.Inspect(funcLit, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, lhs := range x.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						if obj := info.Defs[id]; obj != nil {
							localObjs[obj] = true
						}
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				for _, v := range []ast.Expr{x.Key, x.Value} {
					if id, ok := v.(*ast.Ident); ok {
						if obj := info.Defs[id]; obj != nil {
							localObjs[obj] = true
						}
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				if obj := info.Defs[id]; obj != nil {
					localObjs[obj] = true
				}
			}
		}
		return true
	})
	// FuncLit parameters are local too.
	if funcLit.Type.Params != nil {
		for _, field := range funcLit.Type.Params.List {
			for _, id := range field.Names {
				if obj := info.Defs[id]; obj != nil {
					localObjs[obj] = true
				}
			}
		}
	}

	// Walk uses inside the FuncLit body looking for captures.
	seen := make(map[types.Object]bool)
	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.Uses[id]
		if obj == nil {
			return true
		}
		if localObjs[obj] {
			return true
		}
		if _, isVar := obj.(*types.Var); !isVar {
			return true
		}
		if seen[obj] {
			return true
		}
		seen[obj] = true
		if !canHoldPointers(obj.Type()) {
			return true
		}
		pos := pkg.Fset.Position(id.Pos())
		bc := &boundaryCrossing{
			offset:   pos.Offset,
			line:     pos.Line,
			col:      pos.Column,
			kind:     "go-capture",
			typeName: typeNameFromGoType(obj.Type()),
			goType:   obj.Type(),
			funcName: funcName,
		}
		bc.scope = findInnermostBlock(bc.offset, blocks, bodyRegion)
		gf.boundary = append(gf.boundary, bc)
		return true
	})
}

func addImportSigBoundary(pkg *packages.Package, info *types.Info, gf *gownFile, call *ast.CallExpr, funcName string, blocks []*region, bodyRegion *region) {
	var callee types.Object

	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		callee = info.Uses[fun.Sel]
	case *ast.Ident:
		callee = info.Uses[fun]
	default:
		return
	}
	if callee == nil {
		return
	}
	fn, ok := callee.(*types.Func)
	if !ok {
		return
	}
	// Only track imported functions (different package).
	if fn.Pkg() == nil || fn.Pkg() == pkg.Types {
		return
	}
	sig := fn.Type().(*types.Signature)
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		rt := results.At(i).Type()
		if !canHoldPointers(rt) {
			continue
		}
		pos := pkg.Fset.Position(call.Pos())
		bc := &boundaryCrossing{
			offset:   pos.Offset,
			line:     pos.Line,
			col:      pos.Column,
			kind:     "import-sig",
			typeName: typeNameFromGoType(rt),
			goType:   rt,
			funcName: funcName,
		}
		bc.scope = findInnermostBlock(bc.offset, blocks, bodyRegion)
		gf.boundary = append(gf.boundary, bc)
		break // one boundary per call site is sufficient
	}
}

func findInnermostBlock(offset int, blocks []*region, bodyRegion *region) *region {
	var best *region
	for _, blk := range blocks {
		if offset >= blk.beg && offset < blk.endx {
			if best == nil || (blk.endx-blk.beg) < (best.endx-best.beg) {
				best = blk
			}
		}
	}
	if best == nil {
		best = bodyRegion
	}
	return best
}

func typeNameFromGoType(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return typeNameFromGoType(u.Elem())
	case *types.Slice:
		return typeNameFromGoType(u.Elem())
	case *types.Map:
		if canHoldPointers(u.Key()) {
			return typeNameFromGoType(u.Key())
		}
		return typeNameFromGoType(u.Elem())
	case *types.Chan:
		return typeNameFromGoType(u.Elem())
	case *types.Interface:
		return "interface"
	case *types.Basic:
		return u.Name()
	default:
		return t.String()
	}
}
