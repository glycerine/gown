package gown

import (
	"strings"
	"testing"
)

const gownFrontierUntrackedCallAllowsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a \iso *payload
	Plain(a)
	println(a)
}
`

const gownFrontierUntrackedCallThenSendRejectsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a \iso *payload
	Plain(a)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownFrontierBranchThenSendRejectsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main(cond bool) {
	var a \iso *payload
	if cond {
		Plain(a)
	}
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownFrontierUntrackedCallThenIsoCallRejectsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}
func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	Plain(a)
	Take(a)
}
`

const gownFrontierUntrackedCallThenReadBorrowRejectsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}
func Read(x \rob *payload) {}

func main() {
	var a \iso *payload
	Plain(a)
	Read(a)
}
`

const gownFrontierInterfaceAllowsSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var x any = a
	_ = x
	println(a)
}
`

const gownFrontierInterfaceThenSendRejectsSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var x any = a
	_ = x
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownFrontierReturnTrackedRejectsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func Make(a \iso *payload) \iso *payload {
	Plain(a)
	return a
}
`

const gownFrontierTrackedFunctionUntrackedParamRejectsSource = `package example

type payload struct {
	Data string
}

func Mixed(x *payload, y \rob *payload) {}

func main() {
	var a \iso *payload
	var b \iso *payload
	Mixed(a, b)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownUnsafeCallThenSendRejectsSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a \iso *payload
	Plain(\unsafe(a))
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownUnsafeCallAllowsOrdinaryUntrackedUseSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a \iso *payload
	Plain(\unsafe(a))
	println(a)
}
`

const gownUnsafeStandaloneThenSendRejectsSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	_ = \unsafe(a)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownInterfaceErasureFieldProjectionFrontierSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \iso *payload
}

func main(ch chan \iso *payload) {
	var h \iso *holder
	var x any = h.Item
	_ = x
	ch <- h.Item
}
`

const gownTypeAssertFromInterfaceUntrackedSource = `package example

type payload struct {
	Data string
}

func main(x any) {
	y := x.(*payload)
	ch := make(chan \iso *payload)
	ch <- y
}
`

func TestSSAGWN001IgnoresUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call.gown", gownFrontierUntrackedCallAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly handled untracked-call boundary: %#v", errs)
	}
}

func TestGWN008RejectsUntrackedCallBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call.gown", gownFrontierUntrackedCallAllowsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN008RejectsIsoSendAfterUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call_send.gown", gownFrontierUntrackedCallThenSendRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsIsoSendAfterUntrackedCallBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call_send.gown", gownFrontierUntrackedCallThenSendRejectsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN008RejectsIsoSendAfterBranchUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_branch_send.gown", gownFrontierBranchThenSendRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsIsoSendAfterBranchUntrackedCallBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_branch_send.gown", gownFrontierBranchThenSendRejectsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN008RejectsIsoCallAfterUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call_take.gown", gownFrontierUntrackedCallThenIsoCallRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsIsoCallAfterUntrackedCallBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call_take.gown", gownFrontierUntrackedCallThenIsoCallRejectsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN008RejectsBorrowAfterUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call_read.gown", gownFrontierUntrackedCallThenReadBorrowRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsBorrowAfterUntrackedCallBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call_read.gown", gownFrontierUntrackedCallThenReadBorrowRejectsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN001IgnoresInterfaceErasureBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_interface.gown", gownFrontierInterfaceAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly handled interface boundary: %#v", errs)
	}
}

func TestGWN009RejectsInterfaceErasureBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_interface.gown", gownFrontierInterfaceAllowsSource)
	requireCheckerCode(t, err, GWN009)
}

func TestSSAGWN009RejectsIsoSendAfterInterfaceBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_interface_send.gown", gownFrontierInterfaceThenSendRejectsSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN009)
}

func TestGWN009RejectsIsoSendAfterInterfaceBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_interface_send.gown", gownFrontierInterfaceThenSendRejectsSource)
	requireCheckerCode(t, err, GWN009)
}

func TestSSAGWN008RejectsTrackedReturnAfterUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_return.gown", gownFrontierReturnTrackedRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsTrackedReturnAfterUntrackedCallBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_return.gown", gownFrontierReturnTrackedRejectsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN008RejectsTrackedFunctionUntrackedParamBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_mixed_call.gown", gownFrontierTrackedFunctionUntrackedParamRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsTrackedFunctionUntrackedParamBoundary(t *testing.T) {
	err := checkGownSource(t, "frontier_mixed_call.gown", gownFrontierTrackedFunctionUntrackedParamRejectsSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAUnsafeCallCreatesExplicitFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "unsafe_call_send.gown", gownUnsafeCallThenSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
	if !strings.Contains(errs[0].Message, "unsafe") {
		t.Fatalf("GWN012 message = %q, want unsafe frontier mention", errs[0].Message)
	}
}

func TestSSAUnsafeCallAllowsOrdinaryUntrackedUse(t *testing.T) {
	gp := loadGownForSSACheck(t, "unsafe_call_use.gown", gownUnsafeCallAllowsOrdinaryUntrackedUseSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected ordinary use after unsafe frontier: %#v", errs)
	}
}

func TestSSAUnsafeDoesNotRequireUntrackedCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "unsafe_standalone_send.gown", gownUnsafeStandaloneThenSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
	if !strings.Contains(errs[0].Message, "unsafe") {
		t.Fatalf("GWN012 message = %q, want unsafe frontier mention", errs[0].Message)
	}
}

func TestSSAInterfaceErasureBoundaryIncludesFieldProjection(t *testing.T) {
	gp := loadGownForSSACheck(t, "interface_field_frontier.gown", gownInterfaceErasureFieldProjectionFrontierSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN009)
}

func TestSSATypeAssertFromInterfaceIsUntracked(t *testing.T) {
	gp := loadGownForSSACheck(t, "type_assert_untracked.gown", gownTypeAssertFromInterfaceUntrackedSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN010)
}

func TestGWN008ReportsUntrackedCallBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_note_call.gown", gownFrontierUntrackedCallThenSendRejectsSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN009ReportsInterfaceErasureBoundary(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_note_interface.gown", gownFrontierInterfaceThenSendRejectsSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN009)
}

func TestGWN012ReportsFrontierNoteForUnsafe(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_note_unsafe.gown", gownUnsafeStandaloneThenSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
	requireFrontierNote(t, errs[0], "unsafe")
}

func requireFrontierNote(t *testing.T, err CheckerError, want string) {
	t.Helper()
	if len(err.Notes) == 0 {
		t.Fatalf("GWN012 error has no related notes: %#v", err)
	}
	if !strings.Contains(err.Notes[0].Message, want) {
		t.Fatalf("frontier note message = %q, want %q", err.Notes[0].Message, want)
	}
}
