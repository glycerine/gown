package gown

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

type consumedIso struct {
	name string
	line int
	col  int
}

type gwn001State struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	consumed map[types.Object]consumedIso
	errs     CheckerErrors
}

func checkGWN001(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	state := &gwn001State{
		pkg:      pkg,
		caps:     caps,
		consumed: make(map[types.Object]consumedIso),
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			state.consumed = make(map[types.Object]consumedIso)
			state.checkBlock(fn.Body)
		}
	}
	return state.errs
}

func (state *gwn001State) checkBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		state.checkStmt(stmt)
	}
}

func (state *gwn001State) checkStmt(stmt ast.Stmt) {
	switch stmt := stmt.(type) {
	case *ast.AssignStmt:
		for _, rhs := range stmt.Rhs {
			state.checkExpr(rhs)
		}
	case *ast.BlockStmt:
		state.checkBlock(stmt)
	case *ast.DeclStmt:
		state.checkDecl(stmt.Decl)
	case *ast.DeferStmt:
		state.checkExpr(stmt.Call)
	case *ast.ExprStmt:
		state.checkExpr(stmt.X)
	case *ast.ForStmt:
		state.checkStmt(stmt.Init)
		state.checkExpr(stmt.Cond)
		state.checkBlock(stmt.Body)
		state.checkStmt(stmt.Post)
	case *ast.GoStmt:
		state.checkExpr(stmt.Call)
	case *ast.IfStmt:
		state.checkStmt(stmt.Init)
		state.checkExpr(stmt.Cond)
		state.checkBlock(stmt.Body)
		state.checkStmt(stmt.Else)
	case *ast.IncDecStmt:
		state.checkExpr(stmt.X)
	case *ast.RangeStmt:
		state.checkExpr(stmt.X)
		state.checkBlock(stmt.Body)
	case *ast.ReturnStmt:
		for _, result := range stmt.Results {
			state.checkExpr(result)
		}
	case *ast.SelectStmt:
		state.checkBlock(stmt.Body)
	case *ast.SendStmt:
		state.checkExpr(stmt.Chan)
		state.checkExpr(stmt.Value)
		state.recordIsoSend(stmt)
	case *ast.SwitchStmt:
		state.checkStmt(stmt.Init)
		state.checkExpr(stmt.Tag)
		state.checkBlock(stmt.Body)
	case *ast.TypeSwitchStmt:
		state.checkStmt(stmt.Init)
		state.checkStmt(stmt.Assign)
		state.checkBlock(stmt.Body)
	}
}

func (state *gwn001State) checkDecl(decl ast.Decl) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, value := range valueSpec.Values {
			state.checkExpr(value)
		}
	}
}

func (state *gwn001State) checkExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := state.pkg.TypesInfo.Uses[id]
		if obj == nil {
			return true
		}
		consumed, ok := state.consumed[obj]
		if !ok {
			return true
		}
		pos := state.pkg.Fset.Position(id.Pos())
		state.errs = append(state.errs, CheckerError{
			Code:    GWN001,
			Path:    gownSourcePath(pos.Filename),
			Offset:  pos.Offset,
			Line:    pos.Line,
			Col:     pos.Column,
			Message: fmt.Sprintf("use of moved \\iso value %q after send at %d:%d", consumed.name, consumed.line, consumed.col),
		})
		return false
	})
}

func (state *gwn001State) recordIsoSend(stmt *ast.SendStmt) {
	chObj := identObject(state.pkg, stmt.Chan)
	if state.caps.ChanElemCap(chObj) != CapIso {
		return
	}
	valueIdent, valueObj := identAndObject(state.pkg, stmt.Value)
	if valueObj == nil || state.caps.ObjectCap(valueObj) != CapIso {
		return
	}
	pos := state.pkg.Fset.Position(valueIdent.Pos())
	state.consumed[valueObj] = consumedIso{
		name: valueIdent.Name,
		line: pos.Line,
		col:  pos.Column,
	}
}

func identObject(pkg *packages.Package, expr ast.Expr) types.Object {
	_, obj := identAndObject(pkg, expr)
	return obj
}

func identAndObject(pkg *packages.Package, expr ast.Expr) (*ast.Ident, types.Object) {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return nil, nil
	}
	obj := pkg.TypesInfo.Uses[id]
	if obj == nil {
		obj = pkg.TypesInfo.Defs[id]
	}
	return id, obj
}

func gownSourcePath(path string) string {
	if strings.HasSuffix(path, ".go") {
		return strings.TrimSuffix(path, ".go") + ".gown"
	}
	return path
}
