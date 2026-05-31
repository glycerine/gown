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
