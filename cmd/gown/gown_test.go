package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cliFailingSource = `package example

type payload struct {
	Data string
}

func main() {
	ch := make(chan \iso *payload)
	a := \new(payload{})
	ch <- a
	println(a)
}
`

const cliPassingSource = `package example

func main() {
	println("ok")
}
`

const cliAnnotatedCheckerErrorSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \mub *payload
	ch := make(chan *payload)
	ch <- a
}
`

const cliPropagateSource = `package example

type payload struct {
	Data string
}

func Take(x *payload) {}

func Use(x \iso *payload) {
	Take(x)
}
`

const cliIntrinsicSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func main(x \iso *payload) {
	b := \mub(x)
	r := \rob(x)
	z := \clone(r)
	u := \unsafe(x)
	p := \new(payload{})
	_, _, _, _, _ = b, r, z, u, p
}
`

func TestRunReportsCheckerErrorWithoutPanic(t *testing.T) {
	dir := writeCLIGownDir(t, "failing.gown", cliFailingSource)
	var stderr bytes.Buffer

	code := run([]string{dir}, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "GWN001") {
		t.Fatalf("stderr %q does not contain GWN001", got)
	}
	if !strings.Contains(got, "println(a)") || !strings.Contains(got, "^") {
		t.Fatalf("stderr %q does not contain source context", got)
	}
	if strings.Contains(got, "panic") || strings.Contains(got, "goroutine") {
		t.Fatalf("stderr contains panic output: %q", got)
	}
}

func TestRunReturnsZeroForPassingPackage(t *testing.T) {
	dir := writeCLIGownDir(t, "passing.gown", cliPassingSource)
	var stderr bytes.Buffer

	code := run([]string{dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunDoesNotWriteGeneratedGo(t *testing.T) {
	dir := writeCLIGownDir(t, "passing.gown", cliPassingSource)
	var stderr bytes.Buffer

	code := run([]string{dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	goPath := filepath.Join(dir, "passing.go")
	if _, err := os.Stat(goPath); err == nil {
		t.Fatalf("gown wrote generated file %s", goPath)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRunFormatsOriginalGownLineForAnnotatedCheckerError(t *testing.T) {
	dir := writeCLIGownDir(t, "annotated.gown", cliAnnotatedCheckerErrorSource)
	var stderr bytes.Buffer

	code := run([]string{dir}, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "GWN010") {
		t.Fatalf("stderr %q does not contain GWN010", got)
	}
	if !strings.Contains(got, `ch <- a`) {
		t.Fatalf("stderr %q does not contain source context", got)
	}
	if strings.Contains(got, "ch <- a") && strings.Contains(got, "and ") && strings.Contains(got, "more checker errors") {
		t.Fatalf("stderr %q includes cascaded checker errors", got)
	}
	if strings.Contains(got, `var a      *payload`) {
		t.Fatalf("stderr %q appears to contain stripped .go line", got)
	}
}

func TestRunPropagateRewritesGownThenChecks(t *testing.T) {
	dir := writeCLIGownDir(t, "propagate.gown", cliPropagateSource)
	path := filepath.Join(dir, "propagate.gown")
	var stderr bytes.Buffer

	code := run([]string{"-propagate", dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Take(x \iso *payload)`) {
		t.Fatalf("-propagate did not rewrite Take parameter:\n%s", got)
	}
	if !strings.Contains(stderr.String(), "propagated 1 annotation edit") {
		t.Fatalf("stderr = %q, want propagation summary", stderr.String())
	}
	goPath := filepath.Join(dir, "propagate.go")
	if _, err := os.Stat(goPath); err == nil {
		t.Fatalf("-propagate wrote generated file %s", goPath)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRunCheckAcceptsIntrinsicSyntaxForAnalysis(t *testing.T) {
	dir := writeCLIGownDir(t, "intrinsics.gown", cliIntrinsicSource)
	var stderr bytes.Buffer

	code := run([]string{dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
}

func TestRunMaterializesCommentModeView(t *testing.T) {
	dir := writeCLIGownDir(t, "comment.go", `package example

type payload struct{}

func main() {
	p := &payload{} //gown: new
	_ = p
}
`)
	var stderr bytes.Buffer

	code := run([]string{dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	gownPath := filepath.Join(dir, ".gown", "comment.gown")
	got, err := os.ReadFile(gownPath)
	if err != nil {
		t.Fatalf("gown did not materialize %s: %v", gownPath, err)
	}
	if !strings.Contains(string(got), `p := \new(payload{})`) {
		t.Fatalf("materialized .gown missing lowered source:\n%s", got)
	}
}

func TestRunVersionPrintsBuildInfoWithoutPackageArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithWriters([]string{"-version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"go\t", "path\t"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output %q does not contain %q", got, want)
		}
	}
}

func TestRunRejectsRemovedFlags(t *testing.T) {
	for _, flagName := range []string{"-check", "-print-gown"} {
		var stderr bytes.Buffer
		code := run([]string{flagName}, &stderr)
		if code != 2 {
			t.Fatalf("run(%q) exit code = %d, want 2", flagName, code)
		}
		if !strings.Contains(stderr.String(), "flag provided but not defined") {
			t.Fatalf("run(%q) stderr = %q, want flag parse error", flagName, stderr.String())
		}
	}
}

func writeCLIGownDir(t *testing.T, name, source string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}
