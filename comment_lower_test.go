package gown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const commentLoweringSource = `package example

//gown: param x iso; param y rob; result 0 imm
func F(x *X, y *Y) *Z { return nil }

type X struct{}
type Y struct{}
type Z struct{}

type Holder struct {
	Owned *X      //gown: iso
	Done  chan *X //gown: cap imm; elem iso
	//gown: rob
	Read *Y
}

func Use() {
	var x *X //gown: iso
	//gown: imm
	var y *Y
	b := &X{} //gown: new
	//gown: new
	c := &X{}
	r := b //gown: rob
	m := b //gown: mub
	f := b //gown: freeze
	u := b //gown: unsafe
	copy := b.clone() //gown: clone b
	x, b = b, x //gown: swap
	_, _, _, _, _, _, _, _ = r, m, f, u, copy, x, y, c
}

func (x *X) clone() *X { return &X{} }

func Restore(a *node, b *node) (*node, *node) {
	//gown: restore; param a iso; param b iso; result 0 iso; result 1 iso
	a, b = func(a *node, b *node) (*node, *node) {
		old := a.next
		a.next = b.next
		b.next = old
		return a, b
	}(a, b)
	return a, b
}

type node struct {
	next *node //gown: iso
}

//gown: observer fmt.Printf
`

func TestLowerGoCommentsToGown(t *testing.T) {
	result, err := LowerGoCommentsToGown("comment.go", []byte(commentLoweringSource))
	if err != nil {
		t.Fatal(err)
	}
	got := string(result.GownSrc)
	for _, want := range []string{
		`func F(x \iso *X, y \rob *Y) \imm *Z`,
		`Owned \iso *X`,
		`Done  \imm chan \iso *X`,
		`Read \rob *Y`,
		`var x \iso *X`,
		`var y \imm *Y`,
		`b := \new(X{})`,
		`c := \new(X{})`,
		`r := \rob(b)`,
		`m := \mub(b)`,
		`f := \freeze(b)`,
		`u := \unsafe(b)`,
		`copy := \clone(b)`,
		`\swap(x, b)`,
		`a, b = \restore func(a \iso *node, b \iso *node) (\iso *node, \iso *node)`,
		`next \iso *node`,
		observerDirectiveLexeme + ` fmt.Printf`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("lowered source missing %q:\n%s", want, got)
		}
	}
}

const commentModeCheckSource = `package example

type payload struct {
	Data string
}

func main() {
	var ch chan *payload //gown: elem iso
	a := &payload{} //gown: new
	ch <- a
	println(a)
}
`

func TestCommentModeMaterializesMirrorAndChecks(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"main.go": commentModeCheckSource})

	err := NewGownPackage(dir).Check()
	requireCheckerCode(t, err, GWN001)

	gownPath := filepath.Join(dir, ".gown", "main.gown")
	gownBytes, readErr := os.ReadFile(gownPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	gownSrc := string(gownBytes)
	if !strings.Contains(gownSrc, `var ch chan \iso *payload`) {
		t.Fatalf("materialized .gown missing channel ownerstamp:\n%s", gownSrc)
	}
	if !strings.Contains(gownSrc, `a := \new(payload{})`) {
		t.Fatalf("materialized .gown missing new intrinsic:\n%s", gownSrc)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".gown", "main.go")); statErr != nil {
		t.Fatalf("materialized generated .go missing: %v", statErr)
	}
	originalBytes, readErr := os.ReadFile(filepath.Join(dir, "main.go"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(originalBytes) != commentModeCheckSource {
		t.Fatalf("original .go source was modified:\n%s", originalBytes)
	}
}

func TestGownCommentRequiresAsciiSpaceAfterPrefix(t *testing.T) {
	_, err := LowerGoCommentsToGown("bad.go", []byte("package p\nvar x *int //gown:\tiso\n"))
	if err == nil {
		t.Fatal("expected malformed //gown: spacing to fail")
	}
	if !strings.Contains(err.Error(), "ASCII spaces") {
		t.Fatalf("error = %q, want ASCII spaces diagnostic", err)
	}
}
