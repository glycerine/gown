package gown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const transactionCallBefore = `package example

type Msg struct{}

func Take(x *Msg) {}

func Use(x *Msg) {
	Take(x)
}
`

const transactionCallAfter = `package example

type Msg struct{}

func Take(x *Msg) {}

func Use(x \iso *Msg) {
	Take(x)
}
`

func TestPlanAnnotationTransactionPropagatesDirectCall(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"example.gown": transactionCallBefore})
	path := filepath.Join(dir, "example.gown")
	before := []byte(transactionCallBefore)
	after := []byte(transactionCallAfter)

	txn, err := PlanAnnotationTransaction(AnnotationTransactionOptions{
		Dir:    dir,
		Path:   path,
		Before: before,
		After:  after,
		Overlay: map[string][]byte{
			path: after,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(txn.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one propagated edit", txn.Edits)
	}
	edit := txn.Edits[0]
	if edit.Path != path || edit.NewText != `\iso ` {
		t.Fatalf("edit = %#v, want \\iso insertion in source file", edit)
	}
	next := applyAnnotationTestEdit(before, edit)
	if !strings.Contains(string(next), `func Take(x \iso *Msg)`) {
		t.Fatalf("propagated source missing Take annotation:\n%s", next)
	}
	if _, err := os.Stat(filepath.Join(dir, "example.go")); !os.IsNotExist(err) {
		t.Fatalf("transaction planning wrote generated Go file; stat err=%v", err)
	}
}

func applyAnnotationTestEdit(src []byte, edit AnnotationTextEdit) []byte {
	next := make([]byte, 0, len(src)-(edit.End-edit.Start)+len(edit.NewText))
	next = append(next, src[:edit.Start]...)
	next = append(next, edit.NewText...)
	next = append(next, src[edit.End:]...)
	return next
}
