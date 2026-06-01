package gown

import (
	"strings"
	"testing"
)

const gownSwapRootsSource = `package example

type payload struct{}

func main() {
	x := \new(payload{})
	y := \new(payload{})
	\swap(x, y)
	_, _ = x, y
}
`

const gownSwapFieldAndRootSource = `package example

type node struct {
	Next \iso *node
}

func main() {
	n := \new(node{Next: &node{}})
	\swap(n.Next, n)
	_ = n
}
`

const gownSwapFieldTakeSource = `package example

type wheel struct{}

type bicycle struct {
	front \iso *wheel
}

func main() {
	b := \new(bicycle{front: &wheel{}})
	var w \iso *wheel
	\swap(w, b.front)
	_ = w
}
`

const gownSwapRejectsUntrackedSource = `package example

type payload struct{}

func main() {
	var x *payload
	y := \new(payload{})
	\swap(x, y)
}
`

const gownSwapRejectsTypeMismatchSource = `package example

type left struct{}
type right struct{}

func main() {
	x := \new(left{})
	y := \new(right{})
	\swap(x, y)
}
`

const gownSwapRejectsNonPlaceSource = `package example

type payload struct{}

func makePayload() \iso *payload {
	return \new(payload{})
}

func main() {
	y := \new(payload{})
	\swap(makePayload(), y)
}
`

const gownSwapRejectsMovedPlaceSource = `package example

type payload struct{}

func main(ch chan \iso *payload) {
	x := \new(payload{})
	y := \new(payload{})
	ch <- x
	\swap(x, y)
}
`

func TestSwapAllowsIsoRoots(t *testing.T) {
	err := checkGownSource(t, "swap_roots.gown", gownSwapRootsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSwapAllowsOverlappingFieldAndRoot(t *testing.T) {
	err := checkGownSource(t, "swap_overlap.gown", gownSwapFieldAndRootSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSwapAllowsTakingIsoFieldIntoIsoRoot(t *testing.T) {
	err := checkGownSource(t, "swap_field_take.gown", gownSwapFieldTakeSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSwapRejectsUntrackedPlace(t *testing.T) {
	err := checkGownSource(t, "swap_untracked.gown", gownSwapRejectsUntrackedSource)
	requireCheckerCode(t, err, GWN010)
	if !strings.Contains(err.Error(), `\swap requires \iso`) {
		t.Fatalf("swap error = %v, want \\iso requirement", err)
	}
}

func TestSwapRejectsTypeMismatch(t *testing.T) {
	err := checkGownSource(t, "swap_type_mismatch.gown", gownSwapRejectsTypeMismatchSource)
	requireCheckerCode(t, err, GWN010)
	if !strings.Contains(err.Error(), "identical types") {
		t.Fatalf("swap error = %v, want type mismatch detail", err)
	}
}

func TestSwapRejectsNonPlace(t *testing.T) {
	err := checkGownSource(t, "swap_non_place.gown", gownSwapRejectsNonPlaceSource)
	requireCheckerCode(t, err, GWN010)
	if !strings.Contains(err.Error(), "assignable local or field place") {
		t.Fatalf("swap error = %v, want place detail", err)
	}
}

func TestSwapRejectsMovedPlace(t *testing.T) {
	err := checkGownSource(t, "swap_moved.gown", gownSwapRejectsMovedPlaceSource)
	requireCheckerCode(t, err, GWN001)
}
