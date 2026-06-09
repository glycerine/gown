package gown

import (
	"strings"
	"testing"
)

const gownReturnFreezeToPlainResultSource = `package example

type ticket struct{}

func helper(tkt \iso *ticket) *ticket {
	return \freeze(tkt)
}
`

const gownReturnUnsafeFreezeToPlainResultSource = `package example

type ticket struct{}

func helper(tkt \iso *ticket) *ticket {
	frozen := \freeze(tkt)
	return \unsafe(frozen)
}
`

const gownTrackedToExplicitPlainLocalSource = `package example

type payload struct{}

func main() {
	var x \iso *payload
	var y *payload = x
	_ = y
}
`

const gownUnsafeTrackedToExplicitPlainLocalSource = `package example

type payload struct{}

func main() {
	var x \iso *payload
	var y *payload = \unsafe(x)
	_ = y
}
`

const gownTrackedToUntrackedFieldSource = `package example

type node struct {
	prev *node
	next \iso *node
}

func main() {
	n := \new(node{next: &node{}})
	n.prev = n.next
}
`

const gownUnsafeTrackedToUntrackedFieldSource = `package example

type node struct {
	prev *node
	next \iso *node
}

func main() {
	n := \new(node{next: &node{}})
	n.prev = \unsafe(n.next)
}
`

const gownIsoMoveFromIntrinsicResultToShortDeclSource = `package example

type payload struct{}

func main() {
	j := \new(payload{})
	a := j
	_ = a
}
`

const gownPlainFreshAllocationToPlainChannelSource = `package example

type payload struct{}

func main(ch chan *payload) {
	ch <- &payload{}
}
`

const gownFreshAllocationToIsoContextsSource = `package example

type payload struct{}

func main(ch chan \iso *payload) {
	var x \iso *payload = &payload{}
	ch <- &payload{}
	_ = x
}
`

const gownFreshAllocationToOstampFieldsSource = `package example

type payload struct{}

type holder struct {
	Item \iso *payload
	Done \imm chan \iso *payload
}

func main() {
	h := \new(holder{
		Item: &payload{},
		Done: make(chan \iso *payload),
	})
	_ = h
}
`

const gownPlainValueToIsoCompositeFieldSource = `package example

type payload struct{}

type holder struct {
	Item \iso *payload
}

func main(p *payload) {
	h := \new(holder{
		Item: p,
	})
	_ = h
}
`

const gownPlainValueToIsoAssignedFieldSource = `package example

type payload struct{}

type holder struct {
	Item \iso *payload
}

func main(p *payload) {
	h := \new(holder{})
	h.Item = p
}
`

const gownCloneResultToIsoFieldSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload {
	return &payload{Data: p.Data}
}

type holder struct {
	Item \iso *payload
}

func main(p *payload) {
	h := \new(holder{
		Item: p.clone(),
	})
	_ = h
}
`

const gownSelectReceivedIsoToPlainHelperSource = `package example

type ticket struct{}

type worker struct {
	getJob chan \iso *ticket
}

func helper(tkt *ticket) *ticket {
	return tkt
}

func (w *worker) runWorker() {
	select {
	case tkt := <-w.getJob:
		helper(tkt)
	}
}
`

const gownPlainHelperResultToIsoChannelSource = `package example

type ticket struct{}

func helper() *ticket {
	return nil
}

func main(done chan \iso *ticket) {
	done <- helper()
}
`

const gownPlainHelperBetweenIsoReceiveAndSendSource = `package example

type ticket struct{}

func helper(tkt *ticket) *ticket {
	return tkt
}

func main(in chan \iso *ticket, done chan \iso *ticket) {
	tkt := <-in
	done <- helper(tkt)
}
`

const gownPlainHelperBetweenIsoSelectReceiveAndSendInClosureSource = `package example

type ticket struct{}

type worker struct {
	getJob chan \iso *ticket
	done chan \iso *ticket
}

func helper(tkt *ticket) *ticket {
	return tkt
}

func (w *worker) runWorker() {
	go func() {
		select {
		case tkt := <-w.getJob:
			w.done <- helper(tkt)
		}
	}()
}
`

const gownObserverFmtPrintfTrackedArgumentSource = `package example

import "fmt"

\\\\observer fmt.Printf()

type payload struct{}

func main() {
	var x \iso *payload
	fmt.Printf("x = %p\n", x)
}
`

const gownObserverDebugTicketTrackedArgumentSource = `package example

\\\\observer debugTicket

type ticket struct{}

func debugTicket(tkt *ticket) {}

func main(in chan \iso *ticket) {
	tkt := <-in
	debugTicket(tkt)
}
`

const gownObserverHelperStillRejectsPlainResultToIsoChannelSource = `package example

\\\\observer helper

type ticket struct{}

func helper(tkt *ticket) *ticket {
	return tkt
}

func main(in chan \iso *ticket, done chan \iso *ticket) {
	tkt := <-in
	done <- helper(tkt)
}
`

func TestGWN010RejectsFreezeReturnToPlainResult(t *testing.T) {
	err := checkGownSource(t, "return_freeze_plain.gown", gownReturnFreezeToPlainResultSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN010AllowsUnsafeFreezeReturnToPlainResult(t *testing.T) {
	err := checkGownSource(t, "return_unsafe_freeze_plain.gown", gownReturnUnsafeFreezeToPlainResultSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN008RejectsSelectReceivedIsoPassedToUntrackedHelper(t *testing.T) {
	err := checkGownSource(t, "select_iso_plain_helper.gown", gownSelectReceivedIsoToPlainHelperSource)
	requireCheckerCode(t, err, GWN008)
}

func TestGWN010RejectsPlainHelperResultSentToIsoChannel(t *testing.T) {
	err := checkGownSource(t, "plain_helper_result_iso_send.gown", gownPlainHelperResultToIsoChannelSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN008RejectsPlainHelperBetweenIsoReceiveAndSend(t *testing.T) {
	err := checkGownSource(t, "plain_helper_between_iso.gown", gownPlainHelperBetweenIsoReceiveAndSendSource)
	requireCheckerCode(t, err, GWN008)
}

func TestGWN008RejectsPlainHelperBetweenIsoSelectReceiveAndSendInClosure(t *testing.T) {
	err := checkGownSource(t, "plain_helper_select_closure.gown", gownPlainHelperBetweenIsoSelectReceiveAndSendInClosureSource)
	requireCheckerCode(t, err, GWN008)
}

func TestObserverAllowsFmtPrintfTrackedArgument(t *testing.T) {
	err := checkGownSource(t, "observer_fmt_printf.gown", gownObserverFmtPrintfTrackedArgumentSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserverAllowsDebugTicketTrackedArgument(t *testing.T) {
	err := checkGownSource(t, "observer_debug_ticket.gown", gownObserverDebugTicketTrackedArgumentSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserverHelperStillRejectsPlainResultToIsoChannel(t *testing.T) {
	err := checkGownSource(t, "observer_helper_result.gown", gownObserverHelperStillRejectsPlainResultToIsoChannelSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN010RejectsTrackedValueToExplicitPlainLocal(t *testing.T) {
	err := checkGownSource(t, "tracked_plain_local.gown", gownTrackedToExplicitPlainLocalSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN010AllowsUnsafeTrackedValueToExplicitPlainLocal(t *testing.T) {
	err := checkGownSource(t, "unsafe_tracked_plain_local.gown", gownUnsafeTrackedToExplicitPlainLocalSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN010RejectsTrackedValueToUntrackedField(t *testing.T) {
	err := checkGownSource(t, "tracked_untracked_field.gown", gownTrackedToUntrackedFieldSource)
	requireCheckerCode(t, err, GWN010)
	text := FormatError(err)
	if !strings.Contains(text, `cannot store \iso value "n.next" in untracked field prev`) {
		t.Fatalf("diagnostic %q does not name projected source n.next", text)
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "n.prev = n.next") {
			continue
		}
		if i+1 >= len(lines) {
			t.Fatalf("diagnostic %q has source line without caret line", text)
		}
		gotCaret := strings.Index(lines[i+1], "^")
		wantCaret := strings.Index(line, "next")
		if gotCaret != wantCaret {
			t.Fatalf("caret column = %d, want %d in diagnostic:\n%s", gotCaret, wantCaret, text)
		}
		return
	}
	t.Fatalf("diagnostic %q does not include source line", text)
}

func TestGWN010AllowsUnsafeTrackedValueToUntrackedField(t *testing.T) {
	err := checkGownSource(t, "unsafe_tracked_untracked_field.gown", gownUnsafeTrackedToUntrackedFieldSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOstampErasureAllowsIsoMoveFromIntrinsicResultToShortDecl(t *testing.T) {
	err := checkGownSource(t, "intrinsic_move_short_decl.gown", gownIsoMoveFromIntrinsicResultToShortDeclSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOstampErasureAllowsPlainFreshAllocationToPlainChannel(t *testing.T) {
	err := checkGownSource(t, "plain_fresh_channel.gown", gownPlainFreshAllocationToPlainChannelSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOstampErasureAllowsFreshAllocationInIsoContexts(t *testing.T) {
	err := checkGownSource(t, "fresh_iso_contexts.gown", gownFreshAllocationToIsoContextsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOstampErasureAllowsFreshAllocationInOstampFields(t *testing.T) {
	err := checkGownSource(t, "fresh_ostamp_fields.gown", gownFreshAllocationToOstampFieldsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN010RejectsPlainValueToIsoCompositeField(t *testing.T) {
	err := checkGownSource(t, "plain_iso_composite_field.gown", gownPlainValueToIsoCompositeFieldSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN010RejectsPlainValueToIsoAssignedField(t *testing.T) {
	err := checkGownSource(t, "plain_iso_assigned_field.gown", gownPlainValueToIsoAssignedFieldSource)
	requireCheckerCode(t, err, GWN010)
}

func TestOstampErasureAllowsCloneResultToIsoField(t *testing.T) {
	err := checkGownSource(t, "clone_iso_field.gown", gownCloneResultToIsoFieldSource)
	if err != nil {
		t.Fatal(err)
	}
}
