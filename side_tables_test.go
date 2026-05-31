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

const gownNestedSelectorPlaceSource = `package example

type payload struct {
	Data string
}

type inner struct {
	Item *payload
}

type holder struct {
	Inner inner
}

func main() {
	var x \iso *holder
	_ = x.Inner.Item
}
`

const gownParenPlaceSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	_ = (a)
}
`

const gownParenSelectorPlaceSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main() {
	var x \iso *holder
	_ = (x).Item
}
`

const gownIndexPlaceSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Items []*payload
}

func main() {
	var x \iso *holder
	i := 0
	_ = x.Items[i]
}
`

const gownNonValueIdentifierSource = `package example

type payload struct {
	Data string
}

func helper(x *payload) {}

func main() {
	var a *payload
	helper(a)
	_ = payload{Data: "x"}
}
`

const gownPlainChannelSendBindingSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	ch := make(chan *payload)
	ch <- a
}
`

const gownIsoChannelUntrackedValueSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a *payload
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownMultipleSendBindingSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var b \iso *payload
	ch := make(chan \iso *payload)
	ch <- a
	ch <- b
}
`

const gownCallSideTableSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func Inspect(x \rob *payload) {}

func Plain(x *payload) {}

func main() {
	var a \iso *payload
	var b \iso *payload
	var c *payload
	Take(a)
	Inspect(b)
	Plain(c)
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

func TestPlaceIndexRecordsSelectorProjection(t *testing.T) {
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
	if key := place.Key(); key.Root != root || key.Path != ".Item" {
		t.Fatalf("selector place key = %#v, want root x with path .Item", key)
	}
	if key := place.RegionKey(); key.Root != root || key.Path != "" {
		t.Fatalf("selector region key = %#v, want root x with empty path", key)
	}
}

func TestPlaceIndexRecordsNestedSelectorProjection(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"nested_selector.gown": gownNestedSelectorPlaceSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	selector := findFirstSelectorExprBySelName(t, gp, "Item")
	place, ok := gp.caps.PlaceForExpr(selector)
	if !ok {
		t.Fatal("nested selector has no place")
	}
	root := lookupLocalVar(t, gp, "main", "x")
	if key := place.Key(); key.Root != root || key.Path != ".Inner.Item" {
		t.Fatalf("nested selector place key = %#v, want root x with path .Inner.Item", key)
	}
}

func TestPlaceIndexResolvesParenthesizedIdentifier(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"paren.gown": gownParenPlaceSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	paren := findFirstParenExpr(t, gp)
	place, ok := gp.caps.PlaceForExpr(paren)
	if !ok {
		t.Fatal("parenthesized identifier has no place")
	}
	root := lookupLocalVar(t, gp, "main", "a")
	if place.Key().Root != root || place.Key().Path != "" {
		t.Fatalf("parenthesized identifier place key = %#v, want root a", place.Key())
	}
}

func TestPlaceIndexCollapsesParenthesizedSelectorToRoot(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"paren_selector.gown": gownParenSelectorPlaceSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	selector := findFirstSelectorExpr(t, gp)
	place, ok := gp.caps.PlaceForExpr(selector)
	if !ok {
		t.Fatal("parenthesized selector has no place")
	}
	root := lookupLocalVar(t, gp, "main", "x")
	if place.Key().Root != root || place.Key().Path != ".Item" {
		t.Fatalf("parenthesized selector place key = %#v, want root x path .Item", place.Key())
	}
}

func TestPlaceIndexCollapsesIndexExprToRoot(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"index.gown": gownIndexPlaceSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	index := findFirstIndexExpr(t, gp)
	place, ok := gp.caps.PlaceForExpr(index)
	if !ok {
		t.Fatal("index expression has no place")
	}
	root := lookupLocalVar(t, gp, "main", "x")
	if place.Key().Root != root || place.Key().Path != "" {
		t.Fatalf("index expression place key = %#v, want root x", place.Key())
	}
	if !place.Collapsed {
		t.Fatal("index expression place should be collapsed")
	}
}

func TestPlaceIndexIgnoresNonValueIdentifiers(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"non_value.gown": gownNonValueIdentifierSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	for _, id := range findNonValueIdentifiers(t, gp) {
		if place, ok := gp.caps.PlaceForExpr(id); ok {
			t.Fatalf("non-value identifier %q resolved to place %#v", id.Name, place)
		}
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
	if !binding.IsIsoMove() {
		t.Fatal("send binding should be an iso move")
	}
	if binding.Value.Key().Root != lookupLocalVar(t, gp, "main", "a") {
		t.Fatalf("send binding value place = %#v, want root a", binding.Value)
	}
	if binding.Line != 10 {
		t.Fatalf("send binding line = %d, want 10", binding.Line)
	}
}

func TestSendBindingRecordsNonIsoSendAsNonMove(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"plain_send.gown": gownPlainChannelSendBindingSource})

	gp := NewGownPackage(dir)
	err := gp.Check()
	requireCheckerCode(t, err, GWN010)

	send := findFirstSendStmt(t, gp)
	binding, ok := gp.caps.SendBinding(send)
	if !ok {
		t.Fatal("missing send binding")
	}
	if binding.ChanElemCap != CapUntracked {
		t.Fatalf("plain channel elem cap = %v, want %v", binding.ChanElemCap, CapUntracked)
	}
	if binding.ValueCap != CapIso {
		t.Fatalf("plain send value cap = %v, want %v", binding.ValueCap, CapIso)
	}
	if binding.IsIsoMove() {
		t.Fatal("plain channel send should not be an iso move")
	}
}

func TestSendBindingRecordsIsoChannelUntrackedValueAsNonMove(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"iso_untracked.gown": gownIsoChannelUntrackedValueSendSource})

	gp := NewGownPackage(dir)
	err := gp.Check()
	requireCheckerCode(t, err, GWN010)

	send := findFirstSendStmt(t, gp)
	binding, ok := gp.caps.SendBinding(send)
	if !ok {
		t.Fatal("missing send binding")
	}
	if binding.ChanElemCap != CapIso {
		t.Fatalf("iso channel elem cap = %v, want %v", binding.ChanElemCap, CapIso)
	}
	if binding.ValueCap != CapUntracked {
		t.Fatalf("untracked send value cap = %v, want %v", binding.ValueCap, CapUntracked)
	}
	if binding.IsIsoMove() {
		t.Fatal("untracked value send should not be an iso move")
	}
}

func TestSendBindingRecordsMultipleDirectSends(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"multiple_sends.gown": gownMultipleSendBindingSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	sends := findSendStmts(t, gp)
	if len(sends) != 2 {
		t.Fatalf("got %d sends, want 2", len(sends))
	}
	roots := []string{"a", "b"}
	for i, send := range sends {
		binding, ok := gp.caps.SendBinding(send)
		if !ok {
			t.Fatalf("send %d missing binding", i)
		}
		if !binding.IsIsoMove() {
			t.Fatalf("send %d should be an iso move", i)
		}
		if binding.ValueKey().Root != lookupLocalVar(t, gp, "main", roots[i]) {
			t.Fatalf("send %d value key = %#v, want root %s", i, binding.ValueKey(), roots[i])
		}
	}
}

func TestCallBindingLookupRecordsAnnotatedCallArgPlace(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"calls.gown": gownCallSideTableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	call := findFirstCallExprByName(t, gp, "Take")
	binding, ok := gp.caps.CallBinding(call)
	if !ok {
		t.Fatal("missing call binding")
	}
	if binding.Call != call {
		t.Fatal("call binding does not point at original call expression")
	}
	if binding.Callee == nil || binding.Callee.Name() != "Take" {
		t.Fatalf("call binding callee = %v, want Take", binding.Callee)
	}
	if len(binding.ParamCaps) != 1 || binding.ParamCaps[0] != CapIso {
		t.Fatalf("call binding param caps = %v, want [%v]", binding.ParamCaps, CapIso)
	}
	if len(binding.ArgPlaces) != 1 {
		t.Fatalf("call binding arg places len = %d, want 1", len(binding.ArgPlaces))
	}
	if binding.ArgPlaces[0].Key().Root != lookupLocalVar(t, gp, "main", "a") {
		t.Fatalf("call binding arg place = %#v, want root a", binding.ArgPlaces[0])
	}
}

func TestCallBindingIgnoresUnannotatedCallee(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"plain_call.gown": gownCallSideTableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	call := findFirstCallExprByName(t, gp, "Plain")
	if binding, ok := gp.caps.CallBinding(call); ok {
		t.Fatalf("plain call unexpectedly has binding %#v", binding)
	}
}

func TestCallBindingRecordsMultipleAnnotatedCalls(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"multiple_calls.gown": gownCallSideTableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		cap  Cap
		root string
	}{
		"Take":    {cap: CapIso, root: "a"},
		"Inspect": {cap: CapRob, root: "b"},
	}
	for name, w := range want {
		call := findFirstCallExprByName(t, gp, name)
		binding, ok := gp.caps.CallBinding(call)
		if !ok {
			t.Fatalf("%s call missing binding", name)
		}
		if binding.Callee == nil || binding.Callee.Name() != name {
			t.Fatalf("%s call callee = %v, want %s", name, binding.Callee, name)
		}
		if len(binding.ParamCaps) != 1 || binding.ParamCaps[0] != w.cap {
			t.Fatalf("%s call param caps = %v, want [%v]", name, binding.ParamCaps, w.cap)
		}
		if len(binding.ArgPlaces) != 1 || binding.ArgPlaces[0].Key().Root != lookupLocalVar(t, gp, "main", w.root) {
			t.Fatalf("%s call arg places = %#v, want root %s", name, binding.ArgPlaces, w.root)
		}
	}
}

func findFirstSendStmt(t *testing.T, gp *GownPackage) *ast.SendStmt {
	t.Helper()
	sends := findSendStmts(t, gp)
	if len(sends) == 0 {
		t.Fatal("could not find send statement")
	}
	return sends[0]
}

func findSendStmts(t *testing.T, gp *GownPackage) []*ast.SendStmt {
	t.Helper()
	var sends []*ast.SendStmt
	for _, file := range gp.pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			send, ok := n.(*ast.SendStmt)
			if !ok {
				return true
			}
			sends = append(sends, send)
			return true
		})
	}
	return sends
}

func findFirstCallExprByName(t *testing.T, gp *GownPackage, name string) *ast.CallExpr {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		var found *ast.CallExpr
		ast.Inspect(file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || callName(call) != name {
				return true
			}
			found = call
			return false
		})
		if found != nil {
			return found
		}
	}
	t.Fatalf("could not find call %s", name)
	return nil
}

func callName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
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

func findFirstSelectorExprBySelName(t *testing.T, gp *GownPackage, name string) *ast.SelectorExpr {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		var found *ast.SelectorExpr
		ast.Inspect(file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			selector, ok := n.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != name {
				return true
			}
			found = selector
			return false
		})
		if found != nil {
			return found
		}
	}
	t.Fatalf("could not find selector expression %s", name)
	return nil
}

func findFirstParenExpr(t *testing.T, gp *GownPackage) *ast.ParenExpr {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		var found *ast.ParenExpr
		ast.Inspect(file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			paren, ok := n.(*ast.ParenExpr)
			if !ok {
				return true
			}
			found = paren
			return false
		})
		if found != nil {
			return found
		}
	}
	t.Fatal("could not find parenthesized expression")
	return nil
}

func findFirstIndexExpr(t *testing.T, gp *GownPackage) *ast.IndexExpr {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		var found *ast.IndexExpr
		ast.Inspect(file, func(n ast.Node) bool {
			if found != nil {
				return false
			}
			index, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			found = index
			return false
		})
		if found != nil {
			return found
		}
	}
	t.Fatal("could not find index expression")
	return nil
}

func findNonValueIdentifiers(t *testing.T, gp *GownPackage) []*ast.Ident {
	t.Helper()
	var ids []*ast.Ident
	for _, file := range gp.pkg.Syntax {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == "payload" {
						ids = append(ids, ts.Name)
					}
				}
			case *ast.FuncDecl:
				if decl.Name.Name == "helper" {
					ids = append(ids, decl.Name)
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "helper" {
					ids = append(ids, id)
				}
			case *ast.CompositeLit:
				if id, ok := n.Type.(*ast.Ident); ok && id.Name == "payload" {
					ids = append(ids, id)
				}
			}
			return true
		})
	}
	if len(ids) == 0 {
		t.Fatal("could not find non-value identifiers")
	}
	return ids
}
