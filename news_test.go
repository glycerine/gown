package gown

import "testing"

const gownNewsSource = `package main

type payload struct {
	dirt string
}

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
