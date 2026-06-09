package gown

import (
	"errors"
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
	Done  chan *X //gown: imm; elem iso
	//gown: rob
	Read *Y
}

type LiteralHolder struct {
	Done  chan *X
	Other chan *X
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
	copy := b.clone() //gown: clone
	copy2 := (b).clone() //gown:clone
	ch := make(chan *X) //gown: iso
	ch2 := make(chan *X) //gown: elem imm
	x, b = b, x //gown: swap
	_, _, _, _, _, _, _, _, _, _, _ = r, m, f, u, copy, copy2, ch, ch2, x, y, c
}

func (x *X) clone() *X { return &X{} }

func MakeLiteralHolder() *LiteralHolder {
	return &LiteralHolder{
		Done:  make(chan *X), //gown:iso
		Other: make(chan *X), //gown: elem imm
	}
}

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
		`copy2 := \clone((b))`,
		`ch := make(chan \iso *X)`,
		`ch2 := make(chan \imm *X)`,
		`Done:  make(chan \iso *X)`,
		`Other: make(chan \imm *X)`,
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

func TestCommentModeTrailingDirectiveOnMultilineAssignmentStart(t *testing.T) {
	result, err := LowerGoCommentsToGown("comment.go", []byte(`package example

type Job struct {
	Input *int
}

func Use() {
	j := &Job{ //gown:new
		Input: new(int),
	}
	_ = j
}
`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result.GownSrc), `j := \new(Job{`) {
		t.Fatalf("lowered source missing multiline new directive:\n%s", result.GownSrc)
	}
}

const commentModeCheckSource = `package example

type payload struct {
	Data string
}

func main() {
	ch := make(chan *payload) //gown: iso
	a := &payload{} //gown: new
	ch <- a
	println(a)
}
`

const commentModeCompositeLiteralChannelSource = `package example

type ticket struct {
	done chan *ticket //gown: elem iso
}

func newTicket() *ticket {
	return &ticket{
		done: make(chan *ticket), //gown:iso
	}
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
	if !strings.Contains(gownSrc, `ch := make(chan \iso *payload)`) {
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

func TestCommentModeCompositeLiteralFieldDirective(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"main.go": commentModeCompositeLiteralChannelSource})

	if err := NewGownPackage(dir).Check(); err != nil {
		t.Fatal(err)
	}
	gownBytes, err := os.ReadFile(filepath.Join(dir, ".gown", "main.gown"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gownBytes), `done: make(chan \iso *ticket)`) {
		t.Fatalf("materialized .gown missing composite literal channel ownerstamp:\n%s", gownBytes)
	}
}

func TestCommentModeCheckerErrorsReportOriginGoPath(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"main.go": `package example

type wheel struct{}

type bicycle struct {
	front *wheel //gown: iso
}

func main() {
	j := &bicycle{front: &wheel{}}
	var a *wheel //gown: iso
	a, j.front = j.front, a
	_, _ = a, j
}
`})

	err := NewGownPackage(dir).Check()
	requireCheckerCode(t, err, GWN010)

	var checkerErrs CheckerErrors
	if !errors.As(err, &checkerErrs) {
		t.Fatalf("got error %T %v, want CheckerErrors", err, err)
	}
	if len(checkerErrs) == 0 {
		t.Fatal("expected at least one checker error")
	}
	wantPath, pathErr := canonicalFilePath(filepath.Join(dir, "main.go"))
	if pathErr != nil {
		t.Fatal(pathErr)
	}
	if got := checkerErrs[0].Path; got != wantPath {
		t.Fatalf("checker error path = %q, want origin %q", got, wantPath)
	}
	if got := FormatError(err); strings.Contains(got, string(filepath.Separator)+".gown"+string(filepath.Separator)) {
		t.Fatalf("formatted error still points at mirror path:\n%s", got)
	}
}

func TestCommentModeIsoCreationStampsCreatedObjects(t *testing.T) {
	cases := []struct {
		name    string
		varName string
		source  string
	}{
		{
			name:    "make map",
			varName: "m",
			source: `package example

type pointy *int

func Use() {
	m := make(map[pointy]int) //gown: iso
	_ = m
}
`,
		},
		{
			name:    "make slice",
			varName: "s",
			source: `package example

type trickySlice []*int

func Use() {
	s := make(trickySlice, 10) //gown: iso
	_ = s
}
`,
		},
		{
			name:    "make builtin slice",
			varName: "s",
			source: `package example

type payload struct{}

func Use() {
	s := make([]*payload, 0) //gown: iso
	_ = s
}
`,
		},
		{
			name:    "map literal",
			varName: "m",
			source: `package example

type payload struct{}

func Use() {
	m := map[string]*payload{} //gown: iso
	_ = m
}
`,
		},
		{
			name:    "slice literal",
			varName: "s",
			source: `package example

type payload struct{}

func Use() {
	s := []*payload{} //gown: iso
	_ = s
}
`,
		},
		{
			name:    "struct literal",
			varName: "v",
			source: `package example

type payload struct {
	next *payload
}

func Use() {
	v := payload{} //gown: iso
	_ = v
}
`,
		},
		{
			name:    "address struct literal",
			varName: "p",
			source: `package example

type payload struct{}

func Use() {
	p := &payload{} //gown: iso
	_ = p
}
`,
		},
		{
			name:    "new struct pointer",
			varName: "p",
			source: `package example

type payload struct{}

func Use() {
	p := new(payload) //gown: iso
	_ = p
}
`,
		},
		{
			name:    "new primitive pointer",
			varName: "p",
			source: `package example

func Use() {
	p := new(int) //gown: iso
	_ = p
}
`,
		},
		{
			name:    "address slice literal",
			varName: "p",
			source: `package example

type payload struct{}

func Use() {
	p := &[]*payload{} //gown: iso
	_ = p
}
`,
		},
		{
			name:    "address map literal",
			varName: "p",
			source: `package example

type payload struct{}

func Use() {
	p := &map[string]*payload{} //gown: iso
	_ = p
}
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeGownDir(t, map[string]string{"main.go": tt.source})
			gp := NewGownPackage(dir)
			if err := gp.Check(); err != nil {
				t.Fatalf("//gown: iso creation should check: %v", err)
			}
			if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Use", tt.varName)); got != CapIso {
				t.Fatalf("%s cap = %v, want %v", tt.varName, got, CapIso)
			}
			gownBytes, err := os.ReadFile(filepath.Join(dir, ".gown", "main.gown"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(gownBytes), "var "+tt.varName+` \iso `) {
				t.Fatalf("materialized .gown missing creation ownerstamp:\n%s", gownBytes)
			}
		})
	}
}

func TestGownCommentAllowsOptionalWhitespaceAfterPrefix(t *testing.T) {
	cases := []struct {
		name    string
		spacing string
	}{
		{name: "none", spacing: ""},
		{name: "one space", spacing: " "},
		{name: "tab", spacing: "\t"},
		{name: "twenty spaces", spacing: strings.Repeat(" ", maxGownCommentPrefixWhitespace)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := LowerGoCommentsToGown("comment.go", []byte("package p\nvar x *int //gown:"+tt.spacing+"iso\n"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(result.GownSrc), `var x \iso *int`) {
				t.Fatalf("lowered source missing ownerstamp:\n%s", result.GownSrc)
			}
		})
	}
}

func TestGownCommentAllowsGofmtSpaceAfterSlashes(t *testing.T) {
	source := []byte(`package p

// gown: param goner iso
func puncture(goner *wheel) {}

func use() {
	var x *int // gown:iso
	_ = x
}

type wheel struct{}
`)
	if !ContainsGownComment(source) {
		t.Fatal("ContainsGownComment did not recognize // gown:")
	}
	result, err := LowerGoCommentsToGown("comment.go", source)
	if err != nil {
		t.Fatal(err)
	}
	got := string(result.GownSrc)
	for _, want := range []string{
		`func puncture(goner \iso *wheel)`,
		`var x \iso *int`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("lowered source missing %q:\n%s", want, got)
		}
	}
}

func TestGownCommentLimitsWhitespaceAfterPrefix(t *testing.T) {
	_, err := LowerGoCommentsToGown("bad.go", []byte("package p\nvar x *int //gown:"+strings.Repeat(" ", maxGownCommentPrefixWhitespace+1)+"iso\n"))
	if err == nil {
		t.Fatal("expected too much //gown: spacing to fail")
	}
	if !strings.Contains(err.Error(), "at most 20 whitespace") {
		t.Fatalf("error = %q, want whitespace limit diagnostic", err)
	}
}

func TestGownCommentRejectsCapDirectiveAlias(t *testing.T) {
	_, err := LowerGoCommentsToGown("bad.go", []byte("package p\ntype Ticket struct {\n\tDone chan *Ticket //gown: cap imm; elem iso\n}\n"))
	if err == nil {
		t.Fatal("expected cap directive alias to fail")
	}
	if !strings.Contains(err.Error(), `unsupported type directive "cap"`) {
		t.Fatalf("error = %q, want unsupported cap diagnostic", err)
	}
}
