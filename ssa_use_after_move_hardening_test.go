package gown

import "testing"

const gownHardeningPreamble = `package example

type payload struct {
	Data string
}

func (p *payload) Clone() *payload {
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
	Plain(y)
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
