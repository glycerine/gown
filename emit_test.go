package gown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitNilAfterIsoSend(t *testing.T) {
	out := emitGownSource(t, "send.gown", `package example

type payload struct{ Data string }

func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
}
`)

	requireContains(t, out, "ch <- x\n\tx = nil")
}

func TestEmitDoesNotNilBeforeOwnChannelFieldReceiveRebind(t *testing.T) {
	out := emitGownSource(t, "receive_rebind.gown", `package example

type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	tkt = <-tkt.done
	_ = tkt
}
`)

	requireNotContains(t, out, "tkt = nil\n\ttkt = <-tkt.done")
	requireContains(t, out, "work <- tkt\n\ttkt = <-tkt.done")
}

func TestEmitDoesNotNilBeforeOtherMovedImmutableChannelFieldReceiveRebind(t *testing.T) {
	out := emitGownSource(t, "receive_other_rebind.gown", `package example

type ticket struct {
	done \imm chan \iso *ticket
}

func (t *ticket) clone() *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	tkt2 := \clone(tkt)
	work <- tkt2
	tkt = <-tkt2.done
	_ = tkt
}
`)

	requireNotContains(t, out, "tkt2 = nil\n\ttkt = <-tkt2.done")
	requireContains(t, out, "work <- tkt2\n\ttkt = <-tkt2.done")
}

func TestEmitNilAfterIsoCall(t *testing.T) {
	out := emitGownSource(t, "call.gown", `package example

type payload struct{ Data string }

func Take(x \iso *payload) {}

func main() {
	var x \iso *payload
	Take(x)
}
`)

	requireContains(t, out, "Take(x)\n\tx = nil")
}

func TestEmitNilAfterIsoAssignmentMove(t *testing.T) {
	out := emitGownSource(t, "assign.gown", `package example

type payload struct{ Data string }

func main() {
	var x \iso *payload
	y := x
	_ = y
}
`)

	requireContains(t, out, "y := x\n\tx = nil")
}

func TestEmitNilAfterDeferIsoCall(t *testing.T) {
	out := emitGownSource(t, "defer.gown", `package example

type payload struct{ Data string }

func Take(x \iso *payload) {}

func main() {
	var x \iso *payload
	defer Take(x)
}
`)

	requireContains(t, out, "defer Take(x)\n\tx = nil")
}

func TestEmitFreezeAssignment(t *testing.T) {
	out := emitGownSource(t, "freeze.gown", `package example

type payload struct{ Data string }

func main() {
	var x \iso *payload
	y := \freeze(x)
	_ = y
}
`)

	requireContains(t, out, "y := x\n\tx = nil")
	requireNotContains(t, out, `\freeze`)
}

func TestEmitMubRobUnsafeEraseToPlainAssignment(t *testing.T) {
	out := emitGownSource(t, "borrow.gown", `package example

type payload struct{ Data string }

func main() {
	var x \iso *payload
	b := \mub(x)
	r := \rob(x)
	u := \unsafe(x)
	_, _, _ = b, r, u
}
`)

	requireContains(t, out, "b := x")
	requireContains(t, out, "r := x")
	requireContains(t, out, "u := x")
	requireNotContains(t, out, `\mub`)
	requireNotContains(t, out, `\rob`)
	requireNotContains(t, out, `\unsafe`)
}

func TestEmitNewLowersToAddressOfComposite(t *testing.T) {
	out := emitGownSource(t, "new.gown", `package example

type payload struct{ Data string }

func main() {
	x := \new(payload{Data: "fresh"})
	_ = x
}
`)

	requireContains(t, out, `x := &payload{Data: "fresh"}`)
}

func TestEmitDoesNotNilFreshNewSource(t *testing.T) {
	out := emitGownSource(t, "fresh_new.gown", `package example

type payload struct{ Data string }

func main() {
	z := \new(payload{})
	_ = z
}
`)

	requireNotContains(t, out, "z = nil")
}

func TestEmitCloneLowersToSameTypeCloneMethod(t *testing.T) {
	out := emitGownSource(t, "clone.gown", `package example

type payload struct{ Data string }

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \clone(x)
	_, _ = x, y
}
`)

	requireContains(t, out, "y := (x).clone()")
	requireNotContains(t, out, `\clone`)
}

func TestEmitExportedCloneLowersToSameTypeCloneMethod(t *testing.T) {
	out := emitGownSource(t, "exported_clone.gown", `package example

type payload struct{ Data string }

func (p *payload) Clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \Clone(x)
	_, _ = x, y
}
`)

	requireContains(t, out, "y := (x).Clone()")
	requireNotContains(t, out, `\Clone`)
}

func TestEmitCloneCallArgumentLowersWithParens(t *testing.T) {
	out := emitGownSource(t, "clone_call.gown", `package example

type payload struct{ Data string }

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func Get() *payload { return &payload{} }

func main(ch chan \iso *payload) {
	ch <- \clone(Get())
}
`)

	requireContains(t, out, "ch <- (Get()).clone()")
}

func TestEmitCloneFailsClosedWhenInvalid(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"clone_invalid.gown": `package example

type payload struct{ Data string }

func main() {
	var x *payload
	y := \clone(x)
	_, _ = x, y
}
`})
	gp := NewGownPackage(dir)
	err := gp.Check()
	if err == nil {
		t.Fatal("expected clone checker error, got nil")
	}
	if !strings.Contains(err.Error(), "clone()") {
		t.Fatalf("clone checker error = %v", err)
	}
}

func TestRunCheckDoesNotWriteSemanticGo(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"check.gown": `package example

type payload struct{ Data string }

func main() {
	x := \new(payload{})
	_ = x
}
`})
	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "check.go")); !os.IsNotExist(err) {
		t.Fatalf("check-only wrote generated go file, stat err = %v", err)
	}
}

func emitGownSource(t *testing.T, name, source string) string {
	t.Helper()
	dir := writeGownDir(t, map[string]string{name: source})
	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	goPath := filepath.Join(dir, strings.TrimSuffix(name, ".gown")+".go")
	out, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func requireContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output does not contain %q:\n%s", needle, haystack)
	}
}

func requireNotContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Fatalf("output unexpectedly contains %q:\n%s", needle, haystack)
	}
}
