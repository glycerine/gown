package gown

import "testing"

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

func TestSSAGWN012AllowsUntrackedCallAsProofFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call.gown", gownFrontierUntrackedCallAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected frontier-only untracked call: %#v", errs)
	}
}

func TestGWN012AllowsUntrackedCallAsProofFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call.gown", gownFrontierUntrackedCallAllowsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN012RejectsIsoSendAfterUntrackedCallFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call_send.gown", gownFrontierUntrackedCallThenSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsIsoSendAfterUntrackedCallFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call_send.gown", gownFrontierUntrackedCallThenSendRejectsSource)
	requireCheckerCode(t, err, GWN012)
}

func TestSSAGWN012RejectsIsoSendAfterBranchFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_branch_send.gown", gownFrontierBranchThenSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsIsoSendAfterBranchFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_branch_send.gown", gownFrontierBranchThenSendRejectsSource)
	requireCheckerCode(t, err, GWN012)
}

func TestSSAGWN012RejectsIsoCallAfterUntrackedCallFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call_take.gown", gownFrontierUntrackedCallThenIsoCallRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsIsoCallAfterUntrackedCallFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call_take.gown", gownFrontierUntrackedCallThenIsoCallRejectsSource)
	requireCheckerCode(t, err, GWN012)
}

func TestSSAGWN012RejectsBorrowAfterUntrackedCallFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_untracked_call_read.gown", gownFrontierUntrackedCallThenReadBorrowRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsBorrowAfterUntrackedCallFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_untracked_call_read.gown", gownFrontierUntrackedCallThenReadBorrowRejectsSource)
	requireCheckerCode(t, err, GWN012)
}

func TestSSAGWN012AllowsInterfaceErasureAsProofFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_interface.gown", gownFrontierInterfaceAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected frontier-only interface erasure: %#v", errs)
	}
}

func TestGWN012AllowsInterfaceErasureAsProofFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_interface.gown", gownFrontierInterfaceAllowsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN012RejectsIsoSendAfterInterfaceFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_interface_send.gown", gownFrontierInterfaceThenSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsIsoSendAfterInterfaceFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_interface_send.gown", gownFrontierInterfaceThenSendRejectsSource)
	requireCheckerCode(t, err, GWN012)
}

func TestSSAGWN012RejectsTrackedReturnAfterFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_return.gown", gownFrontierReturnTrackedRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsTrackedReturnAfterFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_return.gown", gownFrontierReturnTrackedRejectsSource)
	requireCheckerCode(t, err, GWN012)
}

func TestSSAGWN012RejectsTrackedFunctionUntrackedParamFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "frontier_mixed_call.gown", gownFrontierTrackedFunctionUntrackedParamRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestGWN012RejectsTrackedFunctionUntrackedParamFrontier(t *testing.T) {
	err := checkGownSource(t, "frontier_mixed_call.gown", gownFrontierTrackedFunctionUntrackedParamRejectsSource)
	requireCheckerCode(t, err, GWN012)
}
