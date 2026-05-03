package gown

import (
	"go/ast"
	"testing"
)

const gownSideTableSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownSelectorPlaceSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main() {
	var x \iso *holder
	_ = x.Item
}
`

func TestPlaceIndexResolvesRootIdentifier(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"places.gown": gownSideTableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	send := findFirstSendStmt(t, gp)
	place, ok := gp.caps.PlaceForExpr(send.Value)
	if !ok {
		t.Fatal("send value has no place")
	}
	root := lookupLocalVar(t, gp, "main", "a")
	if place.Root != root {
		t.Fatalf("place root = %v, want %v", place.Root, root)
	}
	if key := place.Key(); key.Root != root || key.Path != "" {
		t.Fatalf("place key = %#v, want root a with empty path", key)
	}
}

func TestPlaceIndexCollapsesSelectorToRoot(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"selector.gown": gownSelectorPlaceSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	selector := findFirstSelectorExpr(t, gp)
	place, ok := gp.caps.PlaceForExpr(selector)
	if !ok {
		t.Fatal("selector has no place")
	}
	root := lookupLocalVar(t, gp, "main", "x")
	if place.Root != root {
		t.Fatalf("selector place root = %v, want %v", place.Root, root)
	}
	if key := place.Key(); key.Root != root || key.Path != "" {
		t.Fatalf("selector place key = %#v, want root x with empty path", key)
	}
}

func TestSendBindingRecordsDirectIsoSend(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"send.gown": gownSideTableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	send := findFirstSendStmt(t, gp)
	binding, ok := gp.caps.SendBinding(send)
	if !ok {
		t.Fatal("missing send binding")
	}
	if binding.Stmt != send {
		t.Fatal("send binding does not point at original send statement")
	}
	if binding.ChanElemCap != CapIso {
		t.Fatalf("send binding chan elem cap = %v, want %v", binding.ChanElemCap, CapIso)
	}
	if binding.ValueCap != CapIso {
		t.Fatalf("send binding value cap = %v, want %v", binding.ValueCap, CapIso)
	}
	if binding.Value.Key().Root != lookupLocalVar(t, gp, "main", "a") {
		t.Fatalf("send binding value place = %#v, want root a", binding.Value)
	}
	if binding.Line != 10 {
		t.Fatalf("send binding line = %d, want 10", binding.Line)
	}
}

func findFirstSendStmt(t *testing.T, gp *GownPackage) *ast.SendStmt {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		var found *ast.SendStmt
		ast.Inspect(file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			send, ok := n.(*ast.SendStmt)
			if !ok {
				return true
			}
			found = send
			return false
		})
		if found != nil {
			return found
		}
	}
	t.Fatal("could not find send statement")
	return nil
}

func findFirstSelectorExpr(t *testing.T, gp *GownPackage) *ast.SelectorExpr {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		var found *ast.SelectorExpr
		ast.Inspect(file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			found = selector
			return false
		})
		if found != nil {
			return found
		}
	}
	t.Fatal("could not find selector expression")
	return nil
}
