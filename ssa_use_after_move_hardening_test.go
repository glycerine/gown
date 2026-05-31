package gown

import (
	"strings"
	"testing"
)

const gownHardeningPreamble = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload {
	if p == nil {
		return nil
	}
	return &payload{Data: p.Data}
}

func Take(x \iso *payload) {}
func Read(x \rob *payload) {}
func Plain(x *payload) {}
func (p *payload) Touch() {}
`

func TestSSAGWN001RejectsSendThenSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	ch <- x
}`, GWN001)
}

func TestSSAGWN001RejectsSendThenIsoCall(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	Take(x)
}`, GWN001)
}

func TestSSAGWN001RejectsIsoCallThenSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	Take(x)
	ch <- x
}`, GWN001)
}

func TestSSAGWN001RejectsIsoCallThenIsoCall(t *testing.T) {
	requireHardeningCheckerCode(t, `func main() {
	var x \iso *payload
	Take(x)
	Take(x)
}`, GWN001)
}

func TestSSAGWN001RejectsSendThenReturn(t *testing.T) {
	requireHardeningCheckerCode(t, `func Move(ch chan \iso *payload, x \iso *payload) \iso *payload {
	ch <- x
	return x
}`, GWN001)
}

func TestSSAGWN001RejectsCallThenReturn(t *testing.T) {
	requireHardeningCheckerCode(t, `func Move(x \iso *payload) \iso *payload {
	Take(x)
	return x
}`, GWN001)
}

func TestSSAGWN001RejectsAssignmentMoveThenSourceSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	y := x
	ch <- x
	_ = y
}`, GWN001)
}

func TestSSAGWN001RejectsSelectorReadAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	println(x.Data)
}`, GWN001)
}

func TestSSAGWN001RejectsSelectorWriteAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	x.Data = "moved"
}`, GWN001)
}

func TestSSAGWN001RejectsDerefAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	_ = *x
}`, GWN001)
}

func TestSSAGWN001RejectsCompareAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	if x == nil {
	}
}`, GWN001)
}

func TestSSAGWN001RejectsInterfaceConversionAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	var y any = x
	_ = y
}`, GWN001)
}

func TestSSAGWN001RejectsMethodReceiverAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	x.Touch()
}`, GWN001)
}

func TestSSAGWN001RejectsClosureCaptureAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	fn := func() {
		println(x)
	}
	fn()
}`, GWN001)
}

func TestSSAGWN001RejectsGoUseAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	go println(x)
}`, GWN001)
}

func TestSSAGWN001RejectsDeferUseAfterSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	defer println(x)
}`, GWN001)
}

func TestSSAGWN001RejectsLoopSecondIterationSend(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	for i := 0; i < 2; i++ {
		ch <- x
	}
}`, GWN001)
}

func TestSSAGWN001AllowsBranchSendAndReturnThenOtherPathUse(t *testing.T) {
	requireHardeningOK(t, `func main(cond bool, ch chan \iso *payload) {
	var x \iso *payload
	if cond {
		ch <- x
		return
	}
	println(x)
}`)
}

func TestSSAGWN001AllowsRebindWithNewAfterSend(t *testing.T) {
	requireHardeningOK(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	x = \new(payload{Data: "fresh"})
	println(x)
}`)
}

func TestSSAGWN001AllowsRebindWithCloneAfterCall(t *testing.T) {
	requireHardeningOK(t, `func main(y \imm *payload) {
	var x \iso *payload
	Take(x)
	x = \clone(y)
	println(x)
}`)
}

func TestSSAGWN001AllowsRebindWithIsoReceiveAfterSend(t *testing.T) {
	requireHardeningOK(t, `func main(ch chan \iso *payload, in chan \iso *payload) {
	var x \iso *payload
	ch <- x
	x = <-in
	println(x)
}`)
}

func TestSSAGWN001RejectsRebindWithIsoReceiveFromMovedMutableChannelField(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	tkt = <-tkt.done
	println(tkt)
}`, GWN001)
}

func TestSSAGWN001UseAfterAssignmentReportsUseBeforeMoveNote(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package example

type payload struct{}

func main() {
	var x \iso *payload
	var y \iso *payload
	x = y
	println(y)
	println(x)
}
`)
	if err == nil {
		t.Fatal("expected GWN001, got nil")
	}
	text := FormatError(err)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], ":9:") || !strings.Contains(lines[0], "GWN001") {
		t.Fatalf("first formatted line = %q, want primary error at use line 9; full error:\n%s", firstLine(text), text)
	}
	useIndex := strings.Index(text, "println(y)")
	noteIndex := strings.Index(text, "note: moved by assignment here")
	assignIndex := strings.Index(text, "x = y")
	if useIndex < 0 || noteIndex < 0 || assignIndex < 0 {
		t.Fatalf("formatted error missing use line, assignment note, or assignment source:\n%s", text)
	}
	if !(useIndex < noteIndex && noteIndex < assignIndex) {
		t.Fatalf("formatted error should report use first, then move note; got:\n%s", text)
	}
}

func TestSSAGWN001MovedFieldUseReportsActualUseLine(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package example

import "fmt"

type ticket struct {
	done chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	done := \unsafe(tkt.done)
	fmt.Printf("done: %p\n", done)
}
`)
	if err == nil {
		t.Fatal("expected GWN001, got nil")
	}
	text := FormatError(err)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], ":16:") || !strings.Contains(lines[0], "GWN001") {
		t.Fatalf("first formatted line = %q, want primary error at use line 16; full error:\n%s", firstLine(text), text)
	}
	useIndex := strings.Index(text, `fmt.Printf("done: %p\n", tkt.done)`)
	noteIndex := strings.Index(text, "note: moved by send here")
	sendIndex := strings.Index(text, "work <- tkt")
	if useIndex < 0 || noteIndex < 0 || sendIndex < 0 {
		t.Fatalf("formatted error missing use line, send note, or send source:\n%s", text)
	}
	if !(useIndex < noteIndex && noteIndex < sendIndex) {
		t.Fatalf("formatted error should report use first, then move note; got:\n%s", text)
	}
}

func TestSSAGWN001AllowsMovedImmutableFieldReadInCall(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package example

import "fmt"

type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	fmt.Printf("done: %p\n", tkt.done)
}
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001AllowsRebindWithIsoReceiveFromMovedImmutableChannelField(t *testing.T) {
	requireHardeningOK(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	tkt = <-tkt.done
	println(tkt)
}`)
}

func TestSSAGWN001AllowsIntermediateRebindWithIsoReceiveFromMovedImmutableChannelField(t *testing.T) {
	requireHardeningOK(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	tkt3 := <-tkt.done
	tkt = tkt3
	println(tkt)
}`)
}

func TestSSAGWN001AllowsBranchIntermediateRebindWithIsoReceiveFromMovedImmutableChannelField(t *testing.T) {
	requireHardeningOK(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket, cond bool) {
	tkt := newTicket()
	if cond {
		work <- tkt
		tkt = <-tkt.done
	} else {
		work <- tkt
		tkt3 := <-tkt.done
		tkt = tkt3
	}
	println(tkt)
}`)
}

func TestSSAGWN001AllowsSelectReceiveTemporaryRebindFromMovedImmutableChannelField(t *testing.T) {
	requireHardeningOK(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	var tkt3 *ticket
	select {
	case tkt3 = <-tkt.done:
		tkt = tkt3
	}
	println(tkt)
}`)
}

func TestSSAGWN001RejectsSelectReceiveTemporaryRebindAfterUnassignedPath(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	var tkt3 *ticket
	select {
	case tkt3 = <-tkt.done:
	default:
	}
	tkt = tkt3
	println(tkt)
}`, GWN010)
}

func TestSSAGWN001AllowsRebindFromOtherMovedImmutableChannelField(t *testing.T) {
	requireHardeningOK(t, `type ticket struct {
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
	println(tkt)
	println(tkt.done)
}`)
}

func TestSSAGWN001AllowsUseBeforeRebindFromOtherMovedImmutableChannelField(t *testing.T) {
	requireHardeningOK(t, `type ticket struct {
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
	println(tkt)
	tkt = <-tkt2.done
	println(tkt)
	println(tkt.done)
}`)
}

func TestSSAGWN001AllowsUntrackedUseBeforeRebindFromOtherMovedImmutableChannelField(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package example

import "fmt"

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
	plainTkt := \unsafe(tkt)
	fmt.Printf("tkt is: %#v\n", plainTkt)
	tkt = <-tkt2.done
	plainDone := \unsafe(tkt.done)
	fmt.Printf("tkt done: %#v\n", plainDone)
}
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN005RejectsReassignImmutableChannelField(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main() {
	tkt := newTicket()
	tkt.done = make(chan \iso *ticket)
}`, GWN005)
}

func TestSSAGWN005RejectsNilAssignmentToImmutableChannelField(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main() {
	tkt := newTicket()
	tkt.done = nil
}`, GWN005)
}

func TestSSAGWN005ReportsImmutableFieldNotImmutableRoot(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package example

type ticket struct {
	done \imm chan \iso *ticket
}

func newTicket() \iso *ticket {
	return &ticket{done: make(chan \iso *ticket)}
}

func main(work chan \iso *ticket) {
	tkt := newTicket()
	work <- tkt
	tkt.done = nil
}
`)
	if err == nil {
		t.Fatal("expected GWN005, got nil")
	}
	text := FormatError(err)
	if !strings.Contains(text, `cannot assign to \imm field "done" of \iso value "tkt"`) {
		t.Fatalf("formatted error does not explain immutable field cause:\n%s", text)
	}
	if strings.Contains(text, `cannot write through \imm value "tkt"`) {
		t.Fatalf("formatted error incorrectly describes root as immutable:\n%s", text)
	}
}

func TestSSAGWN005RejectsNilAssignmentToImmutableChannelFieldFromSelectReceive(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

type worker struct {
	get chan \iso *ticket
}

func main(w *worker) {
	go func() {
		select {
		case tkt := <-w.get:
			tkt.done = nil
		}
	}()
}`, GWN005)
}

func TestSSAGWN005RejectsNilAssignmentToImmutableChannelFieldBeforeSelfSend(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

type worker struct {
	get chan \iso *ticket
}

func (w *worker) runWorker() {
	go func() {
		select {
		case tkt := <-w.get:
			tkt.done = nil
			tkt.done <- tkt
		}
	}()
}`, GWN005)
}

func TestSSAGWN005RejectsNilAssignmentToImmutableChannelFieldAfterSelfSend(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

type worker struct {
	get chan \iso *ticket
}

func (w *worker) runWorker() {
	go func() {
		select {
		case tkt := <-w.get:
			tkt.done <- tkt
			tkt.done = nil
		}
	}()
}`, GWN005)
}

func TestSSAGWN005RejectsMakeAssignmentToImmutableChannelFieldAfterSelfSend(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

type worker struct {
	get chan \iso *ticket
}

func (w *worker) runWorker() {
	go func() {
		select {
		case tkt := <-w.get:
			tkt.done <- tkt
			tkt.done = make(chan \iso *ticket)
		}
	}()
}`, GWN005)
}

func TestSSAGWN005RejectsMakeAssignmentToImmutableChannelFieldInWorkerLoop(t *testing.T) {
	requireHardeningCheckerCode(t, `type ticket struct {
	done \imm chan \iso *ticket
}

type worker struct {
	get chan \iso *ticket
	end chan struct{}
}

func (w *worker) runWorker() {
	go func() {
		for {
			select {
			case tkt := <-w.get:
				tkt.done <- tkt
				tkt.done = make(chan \iso *ticket)
			case <-w.end:
				return
			}
		}
	}()
}`, GWN005)
}

func TestSSAGWN005RejectsMakeAssignmentToImmutableChannelFieldAfterOtherFieldStores(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package example

import "fmt"

type bigTree struct {
	name string
}

type ticket struct {
	tree \iso *bigTree
	outcome string
	done \imm chan \iso *ticket
}

type worker struct {
	get chan \iso *ticket
	end chan struct{}
}

func (w *worker) runWorker() {
	go func() {
		for {
			select {
			case tkt := <-w.get:
				fmt.Printf("processing: %v\n", tkt.tree.name)
				tkt.outcome = "ok"
				tkt.done <- tkt
				tkt.done = make(chan \iso *ticket)
			case <-w.end:
				return
			}
		}
	}()
}
`)
	requireCheckerCode(t, err, GWN005)
}

func TestSSAGWN005RejectsExampleWorkerImmutableChannelReassign(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), `package main

import (
	"fmt"
)

type bigTree struct {
	name string
}

type ticket struct {
	tree \iso *bigTree
	outcome string
	done \imm chan \iso *ticket
}

func newTicket(name string) \iso *ticket {
	return &ticket{
		tree: &bigTree{name: name},
		done: make(chan \iso *ticket),
	}
}

type worker struct {
	getJob chan \iso *ticket
	end    chan struct{}
}

func (w *worker) runWorker() {
	go func() {
		for {
			select {
			case tkt := <-w.getJob:
				fmt.Printf("processing: %v\n", tkt.tree.name)
				tkt.outcome = "ok"
				tkt.done <- tkt
				tkt.done = make(chan \iso *ticket)
			case <-w.end:
				return
			}
		}
	}()
}

func main() {}

func (t *ticket) clone() *ticket {
	return &ticket{
		tree: t.tree.clone(),
		outcome: t.outcome,
		done: make(chan *ticket),
	}
}

func (t *bigTree) clone() *bigTree {
	return &bigTree{name: t.name}
}
`)
	requireCheckerCode(t, err, GWN005)
}

func TestSSAGWN001AllowsRebindByMovingOtherIsoAndRejectsOldSource(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	var y \iso *payload
	ch <- x
	x = y
	println(x)
	println(y)
}`, GWN001)
}

func TestSSAGWN010RejectsRebindFromUntracked(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload, z *payload) {
	var x \iso *payload
	ch <- x
	x = z
}`, GWN010)
}

func TestSSAGWN010RebindFromUntrackedReportsValueTypeAndReason(t *testing.T) {
	err := checkGownSource(t, hardeningTestName(t), gownHardeningPreamble+`
func main(ch chan \iso *payload, z *payload) {
	var x \iso *payload
	ch <- x
	x = z
}
`)
	if err == nil {
		t.Fatal("expected GWN010, got nil")
	}
	text := err.Error()
	for _, want := range []string{
		"type *example.payload",
		"requires a fresh or moved \\iso value",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("error %q does not contain %q", text, want)
		}
	}
}

func TestSSAGWN010RejectsRebindFromBorrow(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload, y \iso *payload) {
	var x \iso *payload
	ch <- x
	x = \rob(y)
}`, GWN010)
}

func TestSSAGWN001RejectsSelfRebindAfterMove(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	x = x
}`, GWN001)
}

func TestSSAGWN001RejectsFieldWriteInsteadOfRootRebind(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	x.Data = "not a root rebind"
}`, GWN001)
}

func TestSSAGWN012RejectsRebindFromFrontieredIso(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(ch chan \iso *payload, y \iso *payload) {
	var x \iso *payload
	Plain(\unsafe(y))
	ch <- x
	x = y
}`, GWN012)
}

func TestSSAGWN001RejectsPostMergeUseWhenOnlyOneBranchRebinds(t *testing.T) {
	requireHardeningCheckerCode(t, `func main(cond bool, ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	if cond {
		x = \new(payload{})
	}
	println(x)
}`, GWN001)
}

func TestSSAGWN001AllowsPostMergeUseWhenAllBranchesRebind(t *testing.T) {
	requireHardeningOK(t, `func main(cond bool, ch chan \iso *payload) {
	var x \iso *payload
	ch <- x
	if cond {
		x = \new(payload{Data: "left"})
	} else {
		x = \new(payload{Data: "right"})
	}
	println(x)
}`)
}

func TestSSAGWN001CloneSendStillDoesNotConsumeOriginal(t *testing.T) {
	requireHardeningOK(t, `func main(ch chan \iso *payload, x \imm *payload) {
	ch <- \clone(x)
	println(x)
}`)
}

func TestSSAGWN001NewSendStillConsumesNoSource(t *testing.T) {
	requireHardeningOK(t, `func main(ch chan \iso *payload) {
	ch <- \new(payload{})
}`)
}

func TestSSAGWN001ImmSendRemainsShareable(t *testing.T) {
	requireHardeningOK(t, `func main(ch chan \imm *payload) {
	var x \imm *payload
	ch <- x
	println(x)
}`)
}

func requireHardeningOK(t *testing.T, body string) {
	t.Helper()
	if err := checkGownSource(t, hardeningTestName(t), gownHardeningPreamble+"\n"+body+"\n"); err != nil {
		t.Fatal(err)
	}
}

func requireHardeningCheckerCode(t *testing.T, body string, code CheckerErrorCode) {
	t.Helper()
	err := checkGownSource(t, hardeningTestName(t), gownHardeningPreamble+"\n"+body+"\n")
	requireCheckerCode(t, err, code)
}

func hardeningTestName(t *testing.T) string {
	t.Helper()
	return t.Name() + ".gown"
}

func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}
