package gown

import "testing"

const gownCloneDirectSendSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x \imm *payload
	ch := make(chan \iso *payload)
	ch <- \clone(x)
	println(x)
}
`

const gownNewDirectSendSource = `package example

type payload struct {
	Data string
}

func main() {
	ch := make(chan \iso *payload)
	ch <- \new(payload{Data: "fresh"})
}
`

const gownReceiveIsoThenSendUseAfterSource = `package example

type payload struct {
	Data string
}

func main(in chan \iso *payload, out chan \iso *payload) {
	x := <-in
	out <- x
	println(x)
}
`

const gownReceiveImmShareSource = `package example

type payload struct {
	Data string
}

func main(in chan \imm *payload, out chan \imm *payload) {
	x := <-in
	out <- x
	println(x)
}
`

const gownSelectIsoSendUseAfterSource = `package example

type payload struct {
	Data string
}

func main(ch chan \iso *payload) {
	var x \iso *payload
	select {
	case ch <- x:
	default:
	}
	println(x)
}
`

const gownSelectRepeatedIsoSendSource = `package example

type payload struct {
	Data string
}

func main(ch1 chan \iso *payload, ch2 chan \iso *payload) {
	var x \iso *payload
	select {
	case ch1 <- x:
	case ch2 <- x:
	}
}
`

const gownSelectCloneSendSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func main(ch chan \iso *payload) {
	var x \imm *payload
	select {
	case ch <- \clone(x):
	default:
	}
	println(x)
}
`

const gownSelectImmSendRetainsValueSource = `package example

type payload struct {
	Data string
}

func main(ch chan \imm *payload) {
	var y \imm *payload
	select {
	case ch <- y:
	default:
	}
	println(y)
}
`

const gownSendFieldThroughRobRootSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \iso *payload
}

func main(ch chan *payload) {
	var h \rob *holder
	ch <- h.Item
}
`

const gownSendImmRootFieldOnImmChannelSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main(ch chan \imm *payload) {
	var h \imm *holder
	ch <- h.Item
	println(h)
}
`

func TestSSAGWN003RejectsMutableBorrowSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_send.gown", gownMubSendSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN003)
}

func TestSSAGWN003RejectsReadBorrowSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_send.gown", gownRobSendSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN003)
}

func TestSSAGWN003AllowsImmutableSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "imm_send.gown", gownImmSendSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected immutable send: %#v", errs)
	}
}

func TestSSAGWN010RejectsUntrackedValueToIsoChannel(t *testing.T) {
	gp := loadGownForSSACheck(t, "untracked_to_iso.gown", gownUntrackedSendToIsoChannelSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN010)
}

func TestSSAInferredFreezeAllowsIsoValueToImmChannel(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_to_imm.gown", gownIsoSendToImmChannelSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected inferred freeze send: %#v", errs)
	}
}

func TestSSACloneSendDoesNotConsumeOriginal(t *testing.T) {
	gp := loadGownForSSACheck(t, "clone_send.gown", gownCloneDirectSendSource)

	if errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected clone send: %#v", errs)
	}
	if errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly consumed clone source: %#v", errs)
	}
}

func TestSSANewSendConsumesFreshResultOnly(t *testing.T) {
	gp := loadGownForSSACheck(t, "new_send.gown", gownNewDirectSendSource)

	if errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected new send: %#v", errs)
	}
	if errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected new send: %#v", errs)
	}
}

func TestSSAReceiveIsoThenSendConsumesReceivedValue(t *testing.T) {
	gp := loadGownForSSACheck(t, "receive_iso_send.gown", gownReceiveIsoThenSendUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSAReceiveImmCanBeShared(t *testing.T) {
	gp := loadGownForSSACheck(t, "receive_imm_share.gown", gownReceiveImmShareSource)

	if errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected imm receive send: %#v", errs)
	}
	if errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly consumed imm receive: %#v", errs)
	}
}

func TestSSASelectIsoSendConsumesAfterSelect(t *testing.T) {
	gp := loadGownForSSACheck(t, "select_iso_send.gown", gownSelectIsoSendUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSASelectRepeatedSameIsoSendAllowedBeforePostUse(t *testing.T) {
	gp := loadGownForSSACheck(t, "select_repeated_iso_send.gown", gownSelectRepeatedIsoSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected repeated select send without later use: %#v", errs)
	}
}

func TestSSASelectCloneSendDoesNotConsumeOriginal(t *testing.T) {
	gp := loadGownForSSACheck(t, "select_clone_send.gown", gownSelectCloneSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly consumed select clone source: %#v", errs)
	}
}

func TestSSASelectImmSendRetainsValue(t *testing.T) {
	gp := loadGownForSSACheck(t, "select_imm_send.gown", gownSelectImmSendRetainsValueSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly consumed select imm send value: %#v", errs)
	}
}

func TestSSASendFieldThroughRobRootRejectedAsNonSendable(t *testing.T) {
	gp := loadGownForSSACheck(t, "send_rob_root_field.gown", gownSendFieldThroughRobRootSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN003)
}

func TestSSASendImmRootFieldOnImmChannelAllowed(t *testing.T) {
	gp := loadGownForSSACheck(t, "send_imm_root_field.gown", gownSendImmRootFieldOnImmChannelSource)

	if errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected imm root field: %#v", errs)
	}
	if errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly consumed imm root field: %#v", errs)
	}
}
