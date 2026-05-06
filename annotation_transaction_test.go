package gown

import (
	"errors"
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

const transactionReturnSource = `package example

type Msg struct{}

func Make() \imm *Msg {
	return &Msg{}
}

func Use() *Msg {
	return Make()
}
`

const transactionCallResultArgumentSource = `package example

type Msg struct{}

func Make() \iso *Msg {
	return &Msg{}
}

func Take(x *Msg) {}

func Use() {
	Take(Make())
}
`

const transactionConflictSource = `package example

type Msg struct{}

func Take(x \iso *Msg) {}

func Use(x \imm *Msg) {
	Take(x)
}
`

const transactionChannelSource = `package example

type Msg struct{}

func Send(ch chan \imm *Msg, x *Msg) {
	ch <- x
}
`

const transactionChannelCallResultSource = `package example

type Msg struct{}

func Make() \imm *Msg {
	return &Msg{}
}

func Send(ch chan *Msg) {
	ch <- Make()
}
`

const transactionFieldSource = `package example

type Msg struct{}

type Box struct {
	Value \rob *Msg
}

func Store(b *Box, x *Msg) {
	b.Value = x
}
`

const transactionLocalSource = `package example

type Msg struct{}

func Use(x \mub *Msg) {
	var y *Msg = x
	_ = y
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

func TestForcePropagateAnnotationsWritesDirectCall(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"example.gown": transactionCallAfter})
	path := filepath.Join(dir, "example.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one forced edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Take(x \iso *Msg)`) {
		t.Fatalf("forced source missing Take annotation:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "example.go")); !os.IsNotExist(err) {
		t.Fatalf("forced propagation wrote generated Go file; stat err=%v", err)
	}
}

func TestForcePropagateAnnotationsWritesReturnResult(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"return.gown": transactionReturnSource})
	path := filepath.Join(dir, "return.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one return-result edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Use() \imm *Msg`) {
		t.Fatalf("forced source missing Use result annotation:\n%s", got)
	}
}

func TestForcePropagateAnnotationsWritesCallResultArgument(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"arg_result.gown": transactionCallResultArgumentSource})
	path := filepath.Join(dir, "arg_result.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one call-result argument edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Take(x \iso *Msg)`) {
		t.Fatalf("forced source missing call-result argument annotation:\n%s", got)
	}
}

func TestForcePropagateAnnotationsWritesChannelSend(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"channel.gown": transactionChannelSource})
	path := filepath.Join(dir, "channel.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one channel-send edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Send(ch chan \imm *Msg, x \imm *Msg)`) {
		t.Fatalf("forced source missing send value annotation:\n%s", got)
	}
}

func TestForcePropagateAnnotationsWritesChannelCallResult(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"channel_result.gown": transactionChannelCallResultSource})
	path := filepath.Join(dir, "channel_result.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one channel call-result edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Send(ch chan \imm *Msg)`) {
		t.Fatalf("forced source missing channel call-result annotation:\n%s", got)
	}
}

func TestForcePropagateAnnotationsWritesFieldStore(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"field.gown": transactionFieldSource})
	path := filepath.Join(dir, "field.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one field-store edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `func Store(b *Box, x \rob *Msg)`) {
		t.Fatalf("forced source missing field store annotation:\n%s", got)
	}
}

func TestForcePropagateAnnotationsWritesExplicitLocal(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"local.gown": transactionLocalSource})
	path := filepath.Join(dir, "local.gown")

	result, err := ForcePropagateAnnotations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Edits) != 1 {
		t.Fatalf("edits = %#v, want exactly one local edit", result.Edits)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `var y \mub *Msg = x`) {
		t.Fatalf("forced source missing local annotation:\n%s", got)
	}
}

func TestForcePropagateAnnotationsReportsConflictingRoots(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"conflict.gown": transactionConflictSource})
	path := filepath.Join(dir, "conflict.gown")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	result, err := ForcePropagateAnnotations(dir)
	if err == nil {
		t.Fatal("ForcePropagateAnnotations succeeded, want conflict")
	}
	var conflicts ForcedAnnotationConflicts
	if !errors.As(err, &conflicts) {
		t.Fatalf("error = %T %v, want ForcedAnnotationConflicts", err, err)
	}
	if result == nil || len(result.Conflicts) == 0 {
		t.Fatalf("result = %#v, want conflict details", result)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("conflicting propagation changed source:\n%s", after)
	}
}

func applyAnnotationTestEdit(src []byte, edit AnnotationTextEdit) []byte {
	next := make([]byte, 0, len(src)-(edit.End-edit.Start)+len(edit.NewText))
	next = append(next, src[:edit.Start]...)
	next = append(next, edit.NewText...)
	next = append(next, src[edit.End:]...)
	return next
}
