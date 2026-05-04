package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gown "github.com/glycerine/gown"
)

func TestHandleFormattingFormatsGownDocument(t *testing.T) {
	dir := writeGownPLSTestDir(t, map[string]string{
		"example.gown": "package example\n\nfunc f( x \\iso *int){\n_ = x\n}\n",
	})
	path := filepath.Join(dir, "example.gown")
	uri := pathToURI(path)
	srv := newServer(nil, &bytes.Buffer{}, nil)
	srv.didOpen(didOpenTextDocumentParams{TextDocument: textDocumentItem{
		URI:     uri,
		Version: 1,
		Text:    readTestFile(t, path),
	}})

	params, _ := json.Marshal(formattingParams{TextDocument: textDocumentIdentifier{URI: uri}})
	edits, err := srv.handleFormatting(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 {
		t.Fatalf("got %d edits, want 1", len(edits))
	}
	if edits[0].NewText == readTestFile(t, path) {
		t.Fatalf("formatting returned unchanged source")
	}
	if !strings.Contains(edits[0].NewText, `\iso *int`) {
		t.Fatalf("formatted source lost Gown annotation:\n%s", edits[0].NewText)
	}
}

func TestHandleCodeActionReturnsAnnotationTransaction(t *testing.T) {
	before := `package example

type Msg struct{}

func Take(x *Msg) {}

func Use(x *Msg) {
	Take(x)
}
`
	after := `package example

type Msg struct{}

func Take(x *Msg) {}

func Use(x \iso *Msg) {
	Take(x)
}
`
	dir := writeGownPLSTestDir(t, map[string]string{"example.gown": before})
	path := filepath.Join(dir, "example.gown")
	uri := pathToURI(path)
	srv := newServer(nil, &bytes.Buffer{}, nil)
	srv.root = dir
	srv.didOpen(didOpenTextDocumentParams{TextDocument: textDocumentItem{
		URI:     uri,
		Version: 1,
		Text:    before,
	}})
	srv.didChange(didChangeTextDocumentParams{
		TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{{
			Text: after,
		}},
	})

	params, _ := json.Marshal(codeActionParams{TextDocument: textDocumentIdentifier{URI: uri}})
	actions, err := srv.handleCodeAction(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Fatalf("actions = %#v, want one annotation action", actions)
	}
	edits := actions[0].Edit.Changes[uri]
	if len(edits) != 1 || edits[0].NewText != `\iso ` {
		t.Fatalf("workspace edits = %#v, want one \\iso insertion", actions[0].Edit.Changes)
	}
}

func TestExecuteCommandRecordsTransaction(t *testing.T) {
	dir := writeGownPLSTestDir(t, map[string]string{"example.gown": "package example\n"})
	srv := newServer(nil, &bytes.Buffer{}, nil)
	srv.root = dir
	txn := &gown.PlannedAnnotationTransaction{
		ID:     "txn-test",
		Status: "planned",
		Root: gown.AnnotationChange{
			Path: filepath.Join(dir, "example.gown"),
		},
	}
	srv.pending[txn.ID] = txn
	arg, _ := json.Marshal(txn.ID)
	params, _ := json.Marshal(executeCommandParams{
		Command:   recordTransactionCommand,
		Arguments: []json.RawMessage{arg},
	})
	if _, err := srv.handleExecuteCommand(params); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gownpls", "transactions.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"id":"txn-test"`) || !strings.Contains(string(data), `"status":"applied"`) {
		t.Fatalf("transaction log = %s", data)
	}
}

func writeGownPLSTestDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
