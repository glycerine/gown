package gown

import "testing"

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
