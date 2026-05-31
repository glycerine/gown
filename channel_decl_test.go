package gown

import (
	"errors"
	"strings"
	"testing"
)

const gownMubChannelElementSource = `package example

type payload struct{}

func main() {
	ch := make(chan \mub *payload)
	_ = ch
}
`

const gownRobChannelElementSource = `package example

type payload struct{}

func Take(ch chan \rob *payload) {
	_ = ch
}
`

func TestGWN010RejectsMutableBorrowChannelElement(t *testing.T) {
	err := checkGownSource(t, "mub_channel.gown", gownMubChannelElementSource)
	checkerErr := requireSingleChannelElementError(t, err)
	if checkerErr.Line != 6 || checkerErr.Col != 18 {
		t.Fatalf("error location = %d:%d, want 6:18", checkerErr.Line, checkerErr.Col)
	}
	text := FormatError(err)
	if !strings.Contains(text, "channel element ownerstamp must be \\iso or \\imm, not \\mub") {
		t.Fatalf("formatted error does not explain invalid channel element:\n%s", text)
	}
	if !strings.Contains(text, "ch := make(chan \\mub *payload)") {
		t.Fatalf("formatted error does not point at channel declaration:\n%s", text)
	}
}

func TestGWN010RejectsReadBorrowChannelElement(t *testing.T) {
	err := checkGownSource(t, "rob_channel.gown", gownRobChannelElementSource)
	checkerErr := requireSingleChannelElementError(t, err)
	if checkerErr.Line != 5 || checkerErr.Col != 19 {
		t.Fatalf("error location = %d:%d, want 5:19", checkerErr.Line, checkerErr.Col)
	}
	text := FormatError(err)
	if !strings.Contains(text, "channel element ownerstamp must be \\iso or \\imm, not \\rob") {
		t.Fatalf("formatted error does not explain invalid channel element:\n%s", text)
	}
	if !strings.Contains(text, "func Take(ch chan \\rob *payload)") {
		t.Fatalf("formatted error does not point at channel declaration:\n%s", text)
	}
}

func requireSingleChannelElementError(t *testing.T, err error) CheckerError {
	t.Helper()
	if err == nil {
		t.Fatal("expected GWN010, got nil")
	}
	var checkerErrs CheckerErrors
	if !errors.As(err, &checkerErrs) {
		t.Fatalf("got error %T %v, want CheckerErrors", err, err)
	}
	if len(checkerErrs) != 1 {
		t.Fatalf("checker errors = %v, want exactly one error", checkerErrs)
	}
	if checkerErrs[0].Code != GWN010 {
		t.Fatalf("checker error code = %s, want %s", checkerErrs[0].Code, GWN010)
	}
	return checkerErrs[0]
}
