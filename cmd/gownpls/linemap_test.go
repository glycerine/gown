package main

import "testing"

func TestLineMapUsesUTF16Columns(t *testing.T) {
	src := []byte("a😀b\nnext")
	lines := newLineMap(src)

	if got := lines.position(len("a😀")); got != (position{Line: 0, Character: 3}) {
		t.Fatalf("position after emoji = %+v, want line 0 char 3", got)
	}
	if got := lines.offset(position{Line: 0, Character: 3}); got != len("a😀") {
		t.Fatalf("offset at char 3 = %d, want %d", got, len("a😀"))
	}
	if got := lines.position(len("a😀b\nne")); got != (position{Line: 1, Character: 2}) {
		t.Fatalf("position on second line = %+v, want line 1 char 2", got)
	}
}
