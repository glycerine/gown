package gown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckIgnoresHiddenGownEditorFiles(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.gown":   "package example\n\nfunc main() {}\n",
		".#main.gown": "this is editor state, not Gown source\n",
	})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckIgnoresUnderscoreGownFiles(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.gown":  "package example\n\nfunc main() {}\n",
		"_skip.gown": "this is not package source\n",
	})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
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

func TestCheckRejectsGoGownBasenameCollision(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.go":   "package example\n\nfunc main() {}\n",
		"main.gown": "package example\n\nfunc main() {}\n",
	})

	err := NewGownPackage(dir).Check()
	if err == nil {
		t.Fatal("expected basename collision error")
	}
	if !strings.Contains(err.Error(), "main.go") || !strings.Contains(err.Error(), "main.gown") {
		t.Fatalf("collision error = %q, want both filenames", err)
	}
}

func TestCheckRejectsGoGownBasenameOverlayCollision(t *testing.T) {
	dir := writeGownDir(t, map[string]string{
		"main.gown": "package example\n\nfunc main() {}\n",
	})

	err := NewGownPackage(dir).CheckWithOptions(CheckOptions{
		GoOverlay: map[string][]byte{
			filepath.Join(dir, "main.go"): []byte("package example\n\nfunc main() {}\n"),
		},
	})
	if err == nil {
		t.Fatal("expected basename collision error")
	}
	if !strings.Contains(err.Error(), "main.go") || !strings.Contains(err.Error(), "main.gown") {
		t.Fatalf("collision error = %q, want both filenames", err)
	}
}
