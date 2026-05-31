package gown

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatGownPreservesCapabilityAnnotations(t *testing.T) {
	src := []byte(`package example
type Msg struct{ Data string }
func Take( x \iso *Msg) \imm *Msg{ var y \mub *Msg; var z \rob *Msg; ch:=make(chan \iso *Msg); _,_,_=y,z,ch; return x}
`)

	formatted, err := FormatGown(src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(formatted)

	for _, want := range []string{
		`func Take(x \iso *Msg) \imm *Msg {`,
		`var y \mub *Msg`,
		`var z \rob *Msg`,
		`ch := make(chan \iso *Msg)`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted source missing %q:\n%s", want, got)
		}
	}
}

func TestFormatGownPreservesIntrinsicCalls(t *testing.T) {
	src := []byte(`package example
func Borrow(x *int){ b:=\mub(x); c:=\rob(x); d:=\clone(x); e:=\Clone(x); f:=\freeze(x); g:=\unsafe(x); _,_,_,_,_,_=b,c,d,e,f,g}
`)

	formatted, err := FormatGown(src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(formatted)

	for _, want := range []string{
		`b := \mub(x)`,
		`c := \rob(x)`,
		`d := \clone(x)`,
		`e := \Clone(x)`,
		`f := \freeze(x)`,
		`g := \unsafe(x)`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted source missing %q:\n%s", want, got)
		}
	}
}

func TestFormatGownLeavesCommentsAndStringsAlone(t *testing.T) {
	src := []byte("package example\n\n" +
		"const quoted = \"\\\\iso in a string\"\n" +
		"const raw = `\\rob in a raw string`\n" +
		"func F(){\n" +
		"// \\mub in a comment\n" +
		"var x \\iso *int\n" +
		"_ = x\n" +
		"}\n")

	formatted, err := FormatGown(src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(formatted)

	for _, want := range []string{
		`const quoted = "\\iso in a string"`,
		"const raw = `\\rob in a raw string`",
		`// \mub in a comment`,
		`var x \iso *int`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted source missing %q:\n%s", want, got)
		}
	}
}

func TestFormatGownFormatsOriginalComments(t *testing.T) {
	src := []byte(`package example
func F(){
if true{
// \iso in a real comment
x:=1
_ = x
}
}
`)

	formatted, err := FormatGown(src)
	if err != nil {
		t.Fatal(err)
	}
	got := string(formatted)

	if want := "\t\t// \\iso in a real comment"; !strings.Contains(got, want) {
		t.Fatalf("comment was not formatted by gofmt; missing %q:\n%s", want, got)
	}
}

func TestFormatGownReportsInvalidSource(t *testing.T) {
	_, err := FormatGown([]byte("package example\nfunc Broken( {\n"))
	if err == nil {
		t.Fatal("expected invalid source error")
	}
}

func TestRunGownfmtFormatsStdinStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunGownfmt([]string{}, strings.NewReader(`package example
func F( x \iso *int){_ = x}
`), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunGownfmt returned %d, stderr:\n%s", code, stderr.String())
	}

	if got, want := stdout.String(), `func F(x \iso *int) {`; !strings.Contains(got, want) {
		t.Fatalf("stdout missing %q:\n%s", want, got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr:\n%s", stderr.String())
	}
}

func TestRunGownfmtWriteFlagRewritesFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "example.gown")
	if err := os.WriteFile(path, []byte(`package example
func F( x \iso *int){_ = x}
`), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := RunGownfmt([]string{"-w", path}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunGownfmt returned %d, stderr:\n%s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout:\n%s", stdout.String())
	}

	formatted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(formatted), `func F(x \iso *int) {`; !strings.Contains(got, want) {
		t.Fatalf("file missing %q:\n%s", want, got)
	}
}
