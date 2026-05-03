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
	a := &payload{}
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

func TestRunReportsCheckerErrorWithoutPanic(t *testing.T) {
	dir := writeCLIGownDir(t, "failing.gown", cliFailingSource)
	var stderr bytes.Buffer

	code := run([]string{"-check", dir}, &stderr)
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

	code := run([]string{"-check", dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCheckOnlyDoesNotWriteGeneratedGo(t *testing.T) {
	dir := writeCLIGownDir(t, "passing.gown", cliPassingSource)
	var stderr bytes.Buffer

	code := run([]string{"-check", dir}, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	goPath := filepath.Join(dir, "passing.go")
	if _, err := os.Stat(goPath); err == nil {
		t.Fatalf("-check wrote generated file %s", goPath)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestRunFormatsOriginalGownLineForAnnotatedCheckerError(t *testing.T) {
	dir := writeCLIGownDir(t, "annotated.gown", cliAnnotatedCheckerErrorSource)
	var stderr bytes.Buffer

	code := run([]string{"-check", dir}, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	got := stderr.String()
	if !strings.Contains(got, "GWN003") {
		t.Fatalf("stderr %q does not contain GWN003", got)
	}
	if !strings.Contains(got, `var a \mub *payload`) {
		t.Fatalf("stderr %q does not contain original .gown annotation line", got)
	}
	if strings.Contains(got, "var a      *payload") {
		t.Fatalf("stderr %q appears to contain stripped .go line", got)
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
