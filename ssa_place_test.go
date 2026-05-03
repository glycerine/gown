package gown

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func TestSSAPlaceIndexPropagatesFieldAddrChains(t *testing.T) {
	facts := loadSSAPlaceFacts(t)

	for _, want := range []struct {
		kind string
		path string
	}{
		{kind: "FieldAddr", path: ".Inner"},
		{kind: "FieldAddr", path: ".Inner.Item"},
		{kind: "UnOp", path: ".Inner.Item"},
		{kind: "Store", path: ".Inner.Item"},
	} {
		if !ssaPlaceFactHas(facts, "main", want.kind, "h", want.path, false) {
			t.Fatalf("SSA place facts missing %s h%s; facts:\n%s",
				want.kind, want.path, formatSSAPlaceFacts(facts))
		}
	}
}

func TestSSAPlaceIndexCollapsesDynamicAndErasedPaths(t *testing.T) {
	facts := loadSSAPlaceFacts(t)

	for _, want := range []string{"IndexAddr", "Lookup", "MakeInterface"} {
		if !ssaPlaceFactHas(facts, "main", want, "h", "", true) {
			t.Fatalf("SSA place facts missing collapsed %s for root h; facts:\n%s",
				want, formatSSAPlaceFacts(facts))
		}
	}
}

func loadSSAPlaceFacts(t *testing.T) []ssaPlaceFact {
	t.Helper()
	dir := writeGownDir(t, map[string]string{"places.gown": gownSSAInventorySource})
	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	places := buildSSAPlaceIndex(gp.pkg, gp.ssaPkg, gp.caps)
	return collectSSAPlaceFacts(gp.pkg, gp.ssaPkg, places)
}

type ssaPlaceFact struct {
	Function  string
	Kind      string
	Root      string
	Path      string
	Collapsed bool
	Line      int
	Col       int
	Block     int
	Index     int
	Text      string
}

func collectSSAPlaceFacts(pkg *packages.Package, ssaPkg *ssa.Package, places *SSAPlaceIndex) []ssaPlaceFact {
	if pkg == nil || ssaPkg == nil || places == nil {
		return nil
	}
	var facts []ssaPlaceFact
	for _, fn := range collectSSAFunctions(ssaPkg) {
		for blockIndex, block := range fn.Blocks {
			for instrIndex, instr := range block.Instrs {
				place, ok := places.PlaceForInstruction(instr)
				if !ok || place.Root == nil {
					continue
				}
				pos := pkg.Fset.Position(instr.Pos())
				facts = append(facts, ssaPlaceFact{
					Function:  fn.Name(),
					Kind:      ssaInventoryKind(instr),
					Root:      place.Root.Name(),
					Path:      place.Key().Path,
					Collapsed: place.Collapsed,
					Line:      sourceLine(pos),
					Col:       sourceColumn(pos),
					Block:     blockIndex,
					Index:     instrIndex,
					Text:      instr.String(),
				})
			}
		}
	}
	return facts
}

func ssaPlaceFactHas(facts []ssaPlaceFact, function, kind, root, path string, collapsed bool) bool {
	for _, fact := range facts {
		if fact.Function == function &&
			fact.Kind == kind &&
			fact.Root == root &&
			fact.Path == path &&
			fact.Collapsed == collapsed {
			return true
		}
	}
	return false
}

func formatSSAPlaceFacts(facts []ssaPlaceFact) string {
	if len(facts) == 0 {
		return "<none>"
	}
	var b strings.Builder
	for _, fact := range facts {
		fmt.Fprintf(&b, "%s %s root=%s path=%q collapsed=%t line=%d col=%d block=%d index=%d %s\n",
			fact.Function, fact.Kind, fact.Root, fact.Path, fact.Collapsed,
			fact.Line, fact.Col, fact.Block, fact.Index, fact.Text)
	}
	return b.String()
}
