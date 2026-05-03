package gown

import "testing"

const gownMubSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \mub *payload
	ch := make(chan *payload)
	ch <- a
}
`

const gownRobSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \rob *payload
	ch := make(chan *payload)
	ch <- a
}
`

const gownImmSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \imm *payload
	ch := make(chan \imm *payload)
	ch <- a
	println(a)
}
`

const gownUntrackedSendToIsoChannelSource = `package example

type payload struct {
	Data string
}

func main() {
	var a *payload
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownIsoSendToImmChannelSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	ch := make(chan \imm *payload)
	ch <- a
}
`

const gownFreshAllocationIsoSendUseAfterSource = `package example

type payload struct {
	Data string
}

func main() {
	ch := make(chan \iso *payload)
	a := &payload{Data: "fresh"}
	ch <- a
	println(a)
}
`

const gownFreshAllocationIsoSendSource = `package example

type payload struct {
	Data string
}

func main() {
	ch := make(chan \iso *payload)
	a := new(payload)
	ch <- a
}
`

func TestGWN003RejectsMutableBorrowSend(t *testing.T) {
	err := checkGownSource(t, "mub_send.gown", gownMubSendSource)
	requireCheckerCode(t, err, GWN003)
}

func TestGWN003RejectsReadBorrowSend(t *testing.T) {
	err := checkGownSource(t, "rob_send.gown", gownRobSendSource)
	requireCheckerCode(t, err, GWN003)
}

func TestGWN003AllowsImmutableSend(t *testing.T) {
	err := checkGownSource(t, "imm_send.gown", gownImmSendSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN010RejectsUntrackedValueSentToIsoChannel(t *testing.T) {
	err := checkGownSource(t, "untracked_to_iso.gown", gownUntrackedSendToIsoChannelSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN010RejectsIsoValueSentToImmChannel(t *testing.T) {
	err := checkGownSource(t, "iso_to_imm.gown", gownIsoSendToImmChannelSource)
	requireCheckerCode(t, err, GWN010)
}

func TestIsoSendInfersFreshAllocationOwnership(t *testing.T) {
	err := checkGownSource(t, "fresh_iso_send.gown", gownFreshAllocationIsoSendSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIsoSendOfFreshAllocationReportsUseAfterMove(t *testing.T) {
	err := checkGownSource(t, "fresh_iso_send_use.gown", gownFreshAllocationIsoSendUseAfterSource)
	requireCheckerCode(t, err, GWN001)
}
