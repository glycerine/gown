package gown

import "testing"

const gownNewsSource = `package main

type payload struct {
	dirt string
}

var Shared *payload

func main() {
	ch := make(chan *payload)
	a := new(payload)
	b := &payload{dirt: "lots"}
	_ = ch
	_ = a
	_ = b
}
`

func TestCreationDetection(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"news.gown": gownNewsSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	if len(gf.create) != 3 {
		t.Fatalf("expected 3 creation points, got %d", len(gf.create))
	}

	wantKinds := []string{"make", "new", "ampersand"}
	wantTypes := []string{"payload", "payload", "payload"}

	for i, n := range gf.create {
		if n.kind != wantKinds[i] {
			t.Errorf("news[%d]: kind = %q, want %q", i, n.kind, wantKinds[i])
		}
		if n.typeName != wantTypes[i] {
			t.Errorf("news[%d]: typeName = %q, want %q", i, n.typeName, wantTypes[i])
		}
		if n.funcName != "main" {
			t.Errorf("news[%d]: funcName = %q, want %q", i, n.funcName, "main")
		}
		if n.scope == nil {
			t.Errorf("news[%d]: scope is nil", i)
		}
		if n.line == 0 {
			t.Errorf("news[%d]: line is 0", i)
		}
		t.Logf("news[%d]: kind=%s type=%s offset=%d line=%d col=%d func=%s",
			i, n.kind, n.typeName, n.offset, n.line, n.col, n.funcName)
	}
}

const gownFalsePositiveSource = `package main

type payload struct {
	dirt string
}

var Shared *payload

func main() {
	a := &payload{dirt: "lots"}
	b := &[]int{1, 2, 3}
	c := &map[string]int{"x": 1}
	d := !true
	_ = a
	_ = b
	_ = c
	_ = d
}
`

func TestCreationNoFalsePositives(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"fp.gown": gownFalsePositiveSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// Only &payload{} is a struct creation. &[]int{} and &map[string]int{}
	// are not structs and must not be detected. !true is not & at all.
	if len(gf.create) != 1 {
		for i, c := range gf.create {
			t.Logf("create[%d]: kind=%s type=%s", i, c.kind, c.typeName)
		}
		t.Fatalf("expected 1 struct creation, got %d", len(gf.create))
	}

	c := gf.create[0]
	if c.kind != "ampersand" || c.typeName != "payload" {
		t.Errorf("got kind=%q type=%q, want ampersand/payload", c.kind, c.typeName)
	}
}

const gownPointerContainerSource = `package main

type payload struct {
	dirt string
}

var Shared *payload

func main() {
	var p1, p2 *payload
	a := &[]*payload{p1, p2}
	b := &[]int{1, 2, 3}
	c := &map[string]*payload{"x": p1}
	d := &map[string]int{"x": 1}
	_ = a
	_ = b
	_ = c
	_ = d
}
`

func TestCreationPointerContainers(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"ptrcont.gown": gownPointerContainerSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// &[]*payload{} and &map[string]*payload{} contain pointers → tracked.
	// &[]int{} and &map[string]int{} do not → skipped.
	if len(gf.create) != 2 {
		for i, c := range gf.create {
			t.Logf("create[%d]: kind=%s type=%s line=%d", i, c.kind, c.typeName, c.line)
		}
		t.Fatalf("expected 2 pointer-container creations, got %d", len(gf.create))
	}

	if gf.create[0].kind != "ampersand" || gf.create[0].typeName != "payload" {
		t.Errorf("create[0]: got kind=%q type=%q, want ampersand/payload",
			gf.create[0].kind, gf.create[0].typeName)
	}
	if gf.create[1].kind != "ampersand" || gf.create[1].typeName != "payload" {
		t.Errorf("create[1]: got kind=%q type=%q, want ampersand/payload",
			gf.create[1].kind, gf.create[1].typeName)
	}
}

const gownPtrToNonStructSource = `package main

var Shared *int

func main() {
	x := 42
	a := &[]*int{&x}
	b := &map[string]*int{"x": &x}
	c := &[]int{1, 2}
	d := &map[string]int{"x": 1}
	_ = a
	_ = b
	_ = c
	_ = d
}
`

func TestCreationPtrToNonStruct(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"ptrnonstruct.gown": gownPtrToNonStructSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// &[]*int{} and &map[string]*int{} contain pointers → must be tracked.
	// &[]int{} and &map[string]int{} do not → skipped.
	// The nested &x inside the literals are plain address-of-variable, not
	// composite literals, so they are not detected here.
	if len(gf.create) != 2 {
		for i, c := range gf.create {
			t.Logf("create[%d]: kind=%s type=%s line=%d", i, c.kind, c.typeName, c.line)
		}
		t.Fatalf("expected 2 pointer-container creations, got %d", len(gf.create))
	}

	if gf.create[0].kind != "ampersand" || gf.create[0].typeName != "int" {
		t.Errorf("create[0]: got kind=%q type=%q, want ampersand/int",
			gf.create[0].kind, gf.create[0].typeName)
	}
	if gf.create[1].kind != "ampersand" || gf.create[1].typeName != "int" {
		t.Errorf("create[1]: got kind=%q type=%q, want ampersand/int",
			gf.create[1].kind, gf.create[1].typeName)
	}
}

const gownPtrKeyMapSource = `package main

type node struct {
	id int
}

var Shared *node

func main() {
	var n1, n2 node
	a := &map[*node]string{&n1: "a", &n2: "b"}
	b := &map[*node]*node{&n1: &n2}
	c := &map[string]string{"x": "y"}
	_ = a
	_ = b
	_ = c
}
`

func TestCreationPtrKeyMaps(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"ptrkey.gown": gownPtrKeyMapSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// &map[*node]string{} has pointer key → tracked.
	// &map[*node]*node{} has pointer key and value → tracked.
	// &map[string]string{} has no pointers → skipped.
	if len(gf.create) != 2 {
		for i, c := range gf.create {
			t.Logf("create[%d]: kind=%s type=%s line=%d", i, c.kind, c.typeName, c.line)
		}
		t.Fatalf("expected 2 pointer-key map creations, got %d", len(gf.create))
	}

	if gf.create[0].typeName != "node" {
		t.Errorf("create[0]: typeName = %q, want %q", gf.create[0].typeName, "node")
	}
	if gf.create[1].typeName != "node" {
		t.Errorf("create[1]: typeName = %q, want %q", gf.create[1].typeName, "node")
	}
}

const gownNamedTypeSource = `package main

type pointy *int

type trickySlice []*int

var Shared any

func main() {
	a := make(map[pointy]int)
	b := make(trickySlice, 10)
	c := make(map[string]any)
	d := make([]int, 5)
	_ = a
	_ = b
	_ = c
	_ = d
}
`

func TestCreationNamedTypes(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"named.gown": gownNamedTypeSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// make(map[pointy]int): pointy is *int → pointer key → tracked
	// make(trickySlice, 10): trickySlice is []*int → pointer element → tracked
	// make(map[string]any): any is interface → tracked
	// make([]int, 5): no pointers → skipped
	if len(gf.create) != 3 {
		for i, c := range gf.create {
			t.Logf("create[%d]: kind=%s type=%s line=%d", i, c.kind, c.typeName, c.line)
		}
		t.Fatalf("expected 3 creations, got %d", len(gf.create))
	}

	want := []struct {
		kind, typeName string
	}{
		{"make", "pointy"},
		{"make", "trickySlice"},
		{"make", "any"},
	}
	for i, w := range want {
		if gf.create[i].kind != w.kind {
			t.Errorf("create[%d]: kind = %q, want %q", i, gf.create[i].kind, w.kind)
		}
		if gf.create[i].typeName != w.typeName {
			t.Errorf("create[%d]: typeName = %q, want %q", i, gf.create[i].typeName, w.typeName)
		}
	}
}
