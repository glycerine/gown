package gown

import (
	"errors"
	"testing"
)

const gownMubTwiceSameArgSource = `package example

type payload struct {
	Data string
}

func MutPair(x \mub *payload, y \mub *payload) {}

func main() {
	var a \iso *payload
	MutPair(a, a)
}
`

const gownMubRobSameArgSource = `package example

type payload struct {
	Data string
}

func MutInspect(x \mub *payload, y \rob *payload) {}

func main() {
	var a \iso *payload
	MutInspect(a, a)
}
`

const gownRobTwiceSameArgSource = `package example

type payload struct {
	Data string
}

func InspectPair(x \rob *payload, y \rob *payload) {}

func main() {
	var a \iso *payload
	InspectPair(a, a)
}
`

const gownMubDistinctArgsSource = `package example

type payload struct {
	Data string
}

func MutPair(x \mub *payload, y \mub *payload) {}

func main() {
	var a \iso *payload
	var b \iso *payload
	MutPair(a, b)
}
`

const gownMubSiblingFieldsSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Left  *payload
	Right *payload
}

func MutPair(x \mub *payload, y \mub *payload) {}

func main() {
	var h \iso *holder
	MutPair(h.Left, h.Right)
}
`

const gownMubSameFieldSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Left *payload
}

func MutPair(x \mub *payload, y \mub *payload) {}

func main() {
	var h \iso *holder
	MutPair(h.Left, h.Left)
}
`

const gownMubRootAndFieldSource = `package example

type holder struct {
	Child *holder
}

func MutRoot(x \mub *holder, y \mub *holder) {}

func main() {
	var h \iso *holder
	MutRoot(h, h.Child)
}
`

func TestGWN002RejectsTwoMutableBorrowsInOneCall(t *testing.T) {
	err := checkGownSource(t, "mub_twice.gown", gownMubTwiceSameArgSource)
	requireCheckerCode(t, err, GWN002)
}

func TestGWN002RejectsMutableAndReadBorrowInOneCall(t *testing.T) {
	err := checkGownSource(t, "mub_rob.gown", gownMubRobSameArgSource)
	requireCheckerCode(t, err, GWN002)
}

func TestGWN002AllowsTwoReadBorrowsInOneCall(t *testing.T) {
	err := checkGownSource(t, "rob_twice.gown", gownRobTwiceSameArgSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN002AllowsMutableBorrowsOfDistinctRoots(t *testing.T) {
	err := checkGownSource(t, "mub_distinct.gown", gownMubDistinctArgsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN002AllowsMutableBorrowsOfSiblingFields(t *testing.T) {
	err := checkGownSource(t, "mub_sibling_fields.gown", gownMubSiblingFieldsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN002RejectsMutableBorrowsOfSameField(t *testing.T) {
	err := checkGownSource(t, "mub_same_field.gown", gownMubSameFieldSource)
	requireCheckerCode(t, err, GWN002)
}

func TestGWN002RejectsMutableBorrowsOfRootAndField(t *testing.T) {
	err := checkGownSource(t, "mub_root_field.gown", gownMubRootAndFieldSource)
	requireCheckerCode(t, err, GWN002)
}

func checkGownSource(t *testing.T, name, source string) error {
	t.Helper()
	dir := writeGownDir(t, map[string]string{name: source})
	return NewGownPackage(dir).Check()
}

func requireCheckerCode(t *testing.T, err error, code CheckerErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	var checkerErrs CheckerErrors
	if !errors.As(err, &checkerErrs) {
		t.Fatalf("got error %T %v, want CheckerErrors", err, err)
	}
	for _, checkerErr := range checkerErrs {
		if checkerErr.Code == code {
			return
		}
	}
	t.Fatalf("checker errors %v do not include %s", checkerErrs, code)
}
