package gown

import "testing"

const gownIsoToInterfaceValueSpecSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var x any = a
	_ = x
}
`

const gownMubToInterfaceAssignSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \mub *payload
	var x any
	x = a
	_ = x
}
`

const gownUntrackedToInterfaceSource = `package example

type payload struct {
	Data string
}

func main() {
	var a *payload
	var x any = a
	_ = x
}
`

func TestGWN009AllowsIsoStoredIntoInterfaceAsFrontier(t *testing.T) {
	err := checkGownSource(t, "iso_interface.gown", gownIsoToInterfaceValueSpecSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN009AllowsBorrowAssignedIntoInterfaceAsFrontier(t *testing.T) {
	err := checkGownSource(t, "mub_interface.gown", gownMubToInterfaceAssignSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN009AllowsUntrackedValueStoredIntoInterface(t *testing.T) {
	err := checkGownSource(t, "untracked_interface.gown", gownUntrackedToInterfaceSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN009AllowsTrackedFieldStoredIntoInterfaceAsFrontier(t *testing.T) {
	err := checkGownSource(t, "iso_field_interface.gown", gownIsoFieldToInterfaceSource)
	if err != nil {
		t.Fatal(err)
	}
}
