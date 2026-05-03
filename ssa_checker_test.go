package gown

import (
	"errors"
	"testing"
)

const gownSSASmokeSource = `package example

func main() {
	println("ssa")
}
`

const gownUseAfterIsoSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	ch := make(chan \iso *payload)
	ch <- a
	println(a)
}
`

const gownUseBeforeIsoSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	ch := make(chan \iso *payload)
	println(a)
	ch <- a
}
`

const gownPlainChannelSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	ch := make(chan *payload)
	ch <- a
	println(a)
}
`

const gownUseAfterIsoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	Take(a)
	println(a)
}
`

const gownUseBeforeIsoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	println(a)
	Take(a)
}
`

const gownRobCallDoesNotMoveSource = `package example

type payload struct {
	Data string
}

func Inspect(x \rob *payload) {}

func main() {
	var a \iso *payload
	Inspect(a)
	println(a)
}
`

const gownUseAfterIsoAssignSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	b := a
	println(b)
	println(a)
}
`

const gownFreshIsoAssignThenSendSource = `package example

type payload struct {
	Data string
}

func main() {
	ch := make(chan \iso *payload)
	a := &payload{}
	b := a
	ch <- b
	println(a)
}
`

func TestBuildsSSAPackage(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"ssa.gown": gownSSASmokeSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if gp.ssaProg == nil {
		t.Fatal("SSA program is nil")
	}
	if gp.ssaPkg == nil {
		t.Fatal("SSA package is nil")
	}
	if gp.ssaPkg.Members["main"] == nil {
		t.Fatal("SSA package is missing main function")
	}
}

func TestGWN001ReportsDirectUseAfterIsoSend(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"use_after_send.gown": gownUseAfterIsoSendSource})

	gp := NewGownPackage(dir)
	err := gp.Check()
	if err == nil {
		t.Fatal("expected GWN001, got nil")
	}

	var checkerErrs CheckerErrors
	if !errors.As(err, &checkerErrs) {
		t.Fatalf("got error %T %v, want CheckerErrors", err, err)
	}
	if len(checkerErrs) != 1 {
		t.Fatalf("got %d checker errors, want 1: %v", len(checkerErrs), checkerErrs)
	}
	if checkerErrs[0].Code != GWN001 {
		t.Fatalf("checker error code = %s, want %s", checkerErrs[0].Code, GWN001)
	}
	if checkerErrs[0].Line != 11 {
		t.Fatalf("GWN001 line = %d, want 11", checkerErrs[0].Line)
	}
}

func TestGWN001AllowsUseBeforeIsoSend(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"use_before_send.gown": gownUseBeforeIsoSendSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestGWN001IgnoresPlainChannelSend(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"plain_channel.gown": gownPlainChannelSendSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestGWN001ReportsDirectUseAfterIsoCall(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"use_after_call.gown": gownUseAfterIsoCallSource})

	gp := NewGownPackage(dir)
	err := gp.Check()
	if err == nil {
		t.Fatal("expected GWN001, got nil")
	}

	var checkerErrs CheckerErrors
	if !errors.As(err, &checkerErrs) {
		t.Fatalf("got error %T %v, want CheckerErrors", err, err)
	}
	if len(checkerErrs) != 1 {
		t.Fatalf("got %d checker errors, want 1: %v", len(checkerErrs), checkerErrs)
	}
	if checkerErrs[0].Code != GWN001 {
		t.Fatalf("checker error code = %s, want %s", checkerErrs[0].Code, GWN001)
	}
	if checkerErrs[0].Line != 12 {
		t.Fatalf("GWN001 line = %d, want 12", checkerErrs[0].Line)
	}
}

func TestGWN001AllowsUseBeforeIsoCall(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"use_before_call.gown": gownUseBeforeIsoCallSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestGWN001IgnoresRobCall(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"rob_call.gown": gownRobCallDoesNotMoveSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestGWN001ReportsDirectUseAfterIsoAssign(t *testing.T) {
	err := checkGownSource(t, "use_after_assign.gown", gownUseAfterIsoAssignSource)
	requireCheckerCode(t, err, GWN001)
}

func TestGWN001MovesFreshIsoThroughAssignmentBeforeSend(t *testing.T) {
	err := checkGownSource(t, "fresh_assign_send.gown", gownFreshIsoAssignThenSendSource)
	requireCheckerCode(t, err, GWN001)
}
