package gown

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type SSADeferredClosureEffect struct {
	Place Place
	Cap   Cap
}

type SSADeferredClosureEffectInfo struct {
	Effects map[sourcePosKey][]SSADeferredClosureEffect
}

func collectSSADeferredClosureEffects(pkg *packages.Package, caps *CapabilityIndex) map[*types.Func]SSADeferredClosureEffectInfo {
	byFunc := make(map[*types.Func]SSADeferredClosureEffectInfo)
	if pkg == nil || caps == nil {
		return byFunc
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fnDecl, ok := decl.(*ast.FuncDecl)
			if !ok || fnDecl.Body == nil {
				continue
			}
			fnObj, _ := pkg.TypesInfo.Defs[fnDecl.Name].(*types.Func)
			if fnObj == nil {
				continue
			}
			info := byFunc[fnObj]
			ast.Inspect(fnDecl.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncLit:
					return false
				case *ast.DeferStmt:
					collectDeferredClosureEffects(pkg, caps, &info, n)
				}
				return true
			})
			if len(info.Effects) != 0 {
				byFunc[fnObj] = info
			}
		}
	}
	return byFunc
}

func collectDeferredClosureEffects(pkg *packages.Package, caps *CapabilityIndex, info *SSADeferredClosureEffectInfo, stmt *ast.DeferStmt) {
	if pkg == nil || caps == nil || info == nil || stmt == nil || stmt.Call == nil {
		return
	}
	lit, ok := stmt.Call.Fun.(*ast.FuncLit)
	if !ok || lit.Body == nil {
		return
	}
	key := sourcePositionKey(pkg.Fset.Position(stmt.Defer))
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			collectDeferredClosureCallEffects(caps, info, key, n)
		}
		return true
	})
}

func collectDeferredClosureCallEffects(caps *CapabilityIndex, info *SSADeferredClosureEffectInfo, key sourcePosKey, call *ast.CallExpr) {
	binding, ok := caps.CallBinding(call)
	if !ok {
		return
	}
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapIso && paramCap != CapMub && paramCap != CapRob {
			continue
		}
		if i >= len(binding.ArgPlaces) {
			continue
		}
		place := binding.ArgPlaces[i]
		if place.Root == nil {
			continue
		}
		if paramCap == CapIso && capForSSAPlace(caps, place) != CapIso {
			continue
		}
		if info.Effects == nil {
			info.Effects = make(map[sourcePosKey][]SSADeferredClosureEffect)
		}
		info.Effects[key] = append(info.Effects[key], SSADeferredClosureEffect{
			Place: place,
			Cap:   paramCap,
		})
	}
}
