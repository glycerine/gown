package gown

import (
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"
)

type ssaInventoryFact struct {
	Function string
	Kind     string
	Line     int
	Col      int
	Block    int
	Index    int
	Text     string
}

func collectSSAInventoryFacts(pkg *packages.Package, ssaPkg *ssa.Package) []ssaInventoryFact {
	if pkg == nil || ssaPkg == nil {
		return nil
	}

	funcs := collectSSAFunctions(ssaPkg)
	var facts []ssaInventoryFact
	for _, fn := range funcs {
		for blockIndex, block := range fn.Blocks {
			for instrIndex, instr := range block.Instrs {
				kind := ssaInventoryKind(instr)
				if kind == "" {
					continue
				}
				pos := pkg.Fset.Position(instr.Pos())
				facts = append(facts, ssaInventoryFact{
					Function: fn.Name(),
					Kind:     kind,
					Line:     sourceLine(pos),
					Col:      sourceColumn(pos),
					Block:    blockIndex,
					Index:    instrIndex,
					Text:     instr.String(),
				})
			}
		}
	}
	return facts
}

func collectSSAFunctions(ssaPkg *ssa.Package) []*ssa.Function {
	seen := make(map[*ssa.Function]bool)
	var funcs []*ssa.Function
	var collect func(*ssa.Function)
	collect = func(fn *ssa.Function) {
		if fn == nil || seen[fn] {
			return
		}
		seen[fn] = true
		if len(fn.Blocks) > 0 {
			funcs = append(funcs, fn)
		}
		for _, anon := range fn.AnonFuncs {
			collect(anon)
		}
	}

	for _, member := range ssaPkg.Members {
		switch member := member.(type) {
		case *ssa.Function:
			collect(member)
		case *ssa.Type:
			if ssaPkg.Prog == nil || types.IsInterface(member.Type()) {
				continue
			}
			for _, method := range typeutil.IntuitiveMethodSet(member.Type(), &ssaPkg.Prog.MethodSets) {
				if method.Obj() == nil || method.Obj().Pkg() != ssaPkg.Pkg {
					continue
				}
				collect(ssaPkg.Prog.MethodValue(method))
			}
		default:
			continue
		}
	}

	sort.Slice(funcs, func(i, j int) bool {
		if funcs[i].Name() != funcs[j].Name() {
			return funcs[i].Name() < funcs[j].Name()
		}
		return funcs[i].String() < funcs[j].String()
	})
	return funcs
}

func ssaInventoryKind(instr ssa.Instruction) string {
	switch instr.(type) {
	case *ssa.Call:
		return "Call"
	case *ssa.Defer:
		return "Defer"
	case *ssa.Field:
		return "Field"
	case *ssa.FieldAddr:
		return "FieldAddr"
	case *ssa.Go:
		return "Go"
	case *ssa.IndexAddr:
		return "IndexAddr"
	case *ssa.Lookup:
		return "Lookup"
	case *ssa.MakeClosure:
		return "MakeClosure"
	case *ssa.MakeInterface:
		return "MakeInterface"
	case *ssa.MapUpdate:
		return "MapUpdate"
	case *ssa.Phi:
		return "Phi"
	case *ssa.Send:
		return "Send"
	case *ssa.Select:
		return "Select"
	case *ssa.Store:
		return "Store"
	case *ssa.UnOp:
		return "UnOp"
	default:
		return ""
	}
}

func sourceLine(pos token.Position) int {
	if !pos.IsValid() {
		return 0
	}
	return pos.Line
}

func sourceColumn(pos token.Position) int {
	if !pos.IsValid() {
		return 0
	}
	return pos.Column
}
