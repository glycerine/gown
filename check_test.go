package gown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckIgnoresHiddenGownEditorFiles(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.gown":   "package example\n\nfunc main() {}\n",
		".#main.gown": "this is editor state, not Gown source\n",
	})

	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIgnoresUnderscoreGownFiles(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.gown":  "package example\n\nfunc main() {}\n",
		"_skip.gown": "this is not package source\n",
	})

	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDoesNotUseHiddenGownOverlays(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.gown": "package example\n\nfunc main() {}\n",
	})
	hidden := filepath.Join(dir, ".#main.gown")

	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{
		CheckOnly: true,
		GownOverlay: map[string][]byte{
			hidden: []byte("this is editor state, not Gown source\n"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hidden); !os.IsNotExist(err) {
		t.Fatalf("hidden overlay unexpectedly materialized on disk: %v", err)
	}
}
