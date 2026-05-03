package gown

import "testing"

const gownIsoToPlainCallSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a \iso *payload
	Plain(a)
}
`

const gownMubToPlainCallSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a \mub *payload
	Plain(a)
}
`

const gownUntrackedToPlainCallSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func main() {
	var a *payload
	Plain(a)
}
`

const gownTrackedToBuiltinSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	println(a)
}
`

func TestGWN008RejectsIsoPassedToUntrackedUserCall(t *testing.T) {
	err := checkGownSource(t, "iso_plain_call.gown", gownIsoToPlainCallSource)
	requireCheckerCode(t, err, GWN008)
}

func TestGWN008RejectsBorrowPassedToUntrackedUserCall(t *testing.T) {
	err := checkGownSource(t, "mub_plain_call.gown", gownMubToPlainCallSource)
	requireCheckerCode(t, err, GWN008)
}

func TestGWN008AllowsUntrackedValuePassedToUntrackedUserCall(t *testing.T) {
	err := checkGownSource(t, "untracked_plain_call.gown", gownUntrackedToPlainCallSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN008AllowsTrackedValuePassedToBuiltin(t *testing.T) {
	err := checkGownSource(t, "tracked_builtin.gown", gownTrackedToBuiltinSource)
	if err != nil {
		t.Fatal(err)
	}
}
