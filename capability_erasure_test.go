package gown

import "testing"

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

func TestGWN008AndGWN010RejectPlainHelperBetweenIsoReceiveAndSend(t *testing.T) {
	err := checkGownSource(t, "plain_helper_between_iso.gown", gownPlainHelperBetweenIsoReceiveAndSendSource)
	requireCheckerCode(t, err, GWN008)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN008AndGWN010RejectPlainHelperBetweenIsoSelectReceiveAndSendInClosure(t *testing.T) {
	err := checkGownSource(t, "plain_helper_select_closure.gown", gownPlainHelperBetweenIsoSelectReceiveAndSendInClosureSource)
	requireCheckerCode(t, err, GWN008)
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
