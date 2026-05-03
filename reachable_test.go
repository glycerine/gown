package gown

import "testing"

const gownReachableSource = `package main

type Inner struct {
	Value int
}

type Outer struct {
	Field *Inner
}

func main() {
	ch := make(chan *Outer)
	go func() { <-ch }()
	o := &Outer{Field: &Inner{Value: 1}}
	ch <- o
	// Inner should be reachable transitively through Outer.Field
	i := &Inner{Value: 2}
	_ = i
}
`

func TestReachableTransitive(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"reach.gown": gownReachableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// Both &Outer{} and &Inner{} should be tracked because Inner is
	// reachable transitively from the channel element type *Outer.
	kinds := make(map[string]int)
	for _, c := range gf.create {
		kinds[c.typeName]++
		t.Logf("create: kind=%s type=%s line=%d", c.kind, c.typeName, c.line)
	}

	if kinds["Outer"] < 1 {
		t.Error("expected Outer creation to be tracked")
	}
	if kinds["Inner"] < 1 {
		t.Error("expected Inner creation to be tracked (reachable via Outer.Field)")
	}
}

const gownPoisonSource = `package main

type Safe struct {
	Data int
}

func main() {
	ch := make(chan any)
	go func() { <-ch }()
	ch <- &Safe{Data: 1}
	s := &Safe{Data: 2}
	_ = s
}
`

func TestReachablePoisonPill(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"poison.gown": gownPoisonSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// chan any → interface → poison pill → track everything.
	found := false
	for _, c := range gf.create {
		t.Logf("create: kind=%s type=%s line=%d", c.kind, c.typeName, c.line)
		if c.typeName == "Safe" {
			found = true
		}
	}
	if !found {
		t.Error("expected Safe creation to be tracked (poisoned by interface channel)")
	}
}

const gownUnreachableSource = `package main

type Tracked struct {
	Data int
}

type Untracked struct {
	Data int
}

func main() {
	ch := make(chan *Tracked)
	go func() { <-ch }()
	a := &Tracked{Data: 1}
	ch <- a
	b := &Untracked{Data: 2}
	_ = b
}
`

func TestReachableFiltersCreation(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"filter.gown": gownUnreachableSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	// Only Tracked is reachable (sent on channel). Untracked is not.
	for _, c := range gf.create {
		t.Logf("create: kind=%s type=%s line=%d", c.kind, c.typeName, c.line)
		if c.typeName == "Untracked" {
			t.Error("Untracked should not be in creation list — not reachable from any boundary")
		}
	}

	found := false
	for _, c := range gf.create {
		if c.typeName == "Tracked" {
			found = true
		}
	}
	if !found {
		t.Error("expected Tracked creation to be tracked")
	}
}
