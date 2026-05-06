package gown

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatErrorIncludesSourceContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.gown")
	src := "package example\n\nfunc main() {\n\tprintln(a)\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	out := FormatError(CheckerErrors{{
		Code:    GWN001,
		Path:    path,
		Line:    4,
		Col:     10,
		Message: `use of moved \iso value "a"`,
	}})

	if !strings.Contains(out, "GWN001") {
		t.Fatalf("formatted error %q does not contain GWN001", out)
	}
	if !strings.Contains(out, "\tprintln(a)") {
		t.Fatalf("formatted error %q does not contain source line", out)
	}
	if !strings.Contains(out, "\t        ^") {
		t.Fatalf("formatted error %q does not contain caret context", out)
	}
}

func TestFormatErrorFormatsMultipleCheckerErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.gown")
	src := "package example\n\nfunc main() {\n\tprintln(a)\n\tprintln(b)\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	out := FormatError(CheckerErrors{
		{Code: GWN001, Path: path, Line: 4, Col: 10, Message: "first"},
		{Code: GWN001, Path: path, Line: 5, Col: 10, Message: "second"},
	})

	if strings.Count(out, "GWN001") != 2 {
		t.Fatalf("formatted error %q should contain two diagnostics", out)
	}
	if !strings.Contains(out, "\tprintln(a)") || !strings.Contains(out, "\tprintln(b)") {
		t.Fatalf("formatted error %q missing source context", out)
	}
}

func TestFormatErrorIncludesRelatedNotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.gown")
	src := "package example\n\nfunc main() {\n\tPlain(a)\n\tch <- a\n}\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	out := FormatError(CheckerErrors{{
		Code:    GWN012,
		Path:    path,
		Line:    5,
		Col:     5,
		Message: `cannot use "a" as send after proof ended`,
		Notes: []CheckerNote{{
			Path:    path,
			Line:    4,
			Col:     2,
			Message: "proof ended here (untracked call)",
		}},
	}})

	if !strings.Contains(out, "note: proof ended here (untracked call)") {
		t.Fatalf("formatted error %q does not contain related note", out)
	}
	if !strings.Contains(out, "\tPlain(a)") {
		t.Fatalf("formatted error %q does not contain note source context", out)
	}
}

func TestFormatErrorFallsBackForNonCheckerError(t *testing.T) {
	out := FormatError(errors.New("plain failure"))
	if out != "plain failure" {
		t.Fatalf("formatted error = %q, want plain failure", out)
	}
}
