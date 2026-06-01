package gown

import "testing"

const gownReturnMubSource = `package example

type payload struct {
	Data string
}

func Leak(x \mub *payload) *payload {
	return x
}
`

const gownReturnRobSource = `package example

type payload struct {
	Data string
}

func Leak(x \rob *payload) *payload {
	return x
}
`

const gownReturnIsoSource = `package example

type payload struct {
	Data string
}

func Move(x \iso *payload) \iso *payload {
	return x
}
`

func TestGWN010RejectsReturnedMutableBorrowToUntrackedResult(t *testing.T) {
	err := checkGownSource(t, "return_mub.gown", gownReturnMubSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN010RejectsReturnedReadBorrowToUntrackedResult(t *testing.T) {
	err := checkGownSource(t, "return_rob.gown", gownReturnRobSource)
	requireCheckerCode(t, err, GWN010)
}

func TestGWN007AllowsReturnedIso(t *testing.T) {
	err := checkGownSource(t, "return_iso.gown", gownReturnIsoSource)
	if err != nil {
		t.Fatal(err)
	}
}
