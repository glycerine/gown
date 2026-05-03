package gown

import "testing"

const gownGoMubCallSource = `package example

type payload struct {
	Data string
}

func Mutate(x \mub *payload) {}

func main() {
	var a \iso *payload
	go Mutate(a)
}
`

const gownGoRobCallSource = `package example

type payload struct {
	Data string
}

func Inspect(x \rob *payload) {}

func main() {
	var a \iso *payload
	go Inspect(a)
}
`

const gownGoIsoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	go Take(a)
}
`

const gownGoIsoClosureCaptureUseAfterSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	go func() {
		println(a)
	}()
	println(a)
}
`

const gownGoIsoClosureCaptureSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	go func() {
		println(a)
	}()
}
`

const gownGoMubClosureCaptureSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \mub *payload
	go func() {
		println(a)
	}()
}
`

const gownGoRobClosureCaptureSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \rob *payload
	go func() {
		println(a)
	}()
}
`

func TestGWN004RejectsMutableBorrowGoCall(t *testing.T) {
	err := checkGownSource(t, "go_mub.gown", gownGoMubCallSource)
	requireCheckerCode(t, err, GWN004)
}

func TestGWN004RejectsReadBorrowGoCall(t *testing.T) {
	err := checkGownSource(t, "go_rob.gown", gownGoRobCallSource)
	requireCheckerCode(t, err, GWN004)
}

func TestGWN004AllowsIsoMoveGoCall(t *testing.T) {
	err := checkGownSource(t, "go_iso.gown", gownGoIsoCallSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN001ReportsUseAfterIsoGoClosureCapture(t *testing.T) {
	err := checkGownSource(t, "go_iso_capture_use.gown", gownGoIsoClosureCaptureUseAfterSource)
	requireCheckerCode(t, err, GWN001)
}

func TestGWN001AllowsIsoGoClosureCaptureWithoutLaterUse(t *testing.T) {
	err := checkGownSource(t, "go_iso_capture.gown", gownGoIsoClosureCaptureSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN004RejectsMutableBorrowGoClosureCapture(t *testing.T) {
	err := checkGownSource(t, "go_mub_capture.gown", gownGoMubClosureCaptureSource)
	requireCheckerCode(t, err, GWN004)
}

func TestGWN004RejectsReadBorrowGoClosureCapture(t *testing.T) {
	err := checkGownSource(t, "go_rob_capture.gown", gownGoRobClosureCaptureSource)
	requireCheckerCode(t, err, GWN004)
}
