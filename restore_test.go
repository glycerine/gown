package gown

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const restoreTestPrelude = `package example

type node struct {
	next  \iso *node
	alt   \iso *node
	prev  *node
	value int
}

type holder struct {
	child \iso *node
}

var global *node

func helper(x *node) *node { return x }

func (n *node) touch() {}

`

func restoreSource(fn string) string {
	return restoreTestPrelude + fn
}

func TestScanAndClassifyRestore(t *testing.T) {
	src := restoreSource(`func f(src \iso *node) \iso *node {
	dst := \restore func(x \iso *node) \iso *node {
		return x
	}(src)
	return dst
}
`)
	emitSrc, analysisSrc, gf, err := scanAndClassify("restore.gown", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(gf.restores) != 1 {
		t.Fatalf("expected 1 restore annotation, got %d", len(gf.restores))
	}
	foundRestore := false
	for _, ann := range gf.annotations {
		if ann.Kind == AnnotationRestoreRegion {
			foundRestore = true
		}
	}
	if !foundRestore {
		t.Fatalf("annotations do not include restore: %#v", gf.annotations)
	}
	if bytes.Contains(emitSrc, []byte(`\restore`)) {
		t.Fatalf("emit source still contains restore marker:\n%s", emitSrc)
	}
	if bytes.Contains(analysisSrc, []byte(`\restore`)) {
		t.Fatalf("analysis source still contains restore marker:\n%s", analysisSrc)
	}
	if !bytes.Contains(analysisSrc, []byte(`func(x      *node)      *node`)) {
		t.Fatalf("analysis source does not preserve func literal shape:\n%s", analysisSrc)
	}
}

func TestRestoreAcceptsConservativeV1Programs(t *testing.T) {
	tests := []struct {
		name string
		fn   string
	}{
		{
			name: "single result define",
			fn: `func f(a \iso *node) \iso *node {
	dst := \restore func(x \iso *node) \iso *node {
		return x
	}(a)
	return dst
}
`,
		},
		{
			name: "single result assign",
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "multi result pointer swap",
			fn: `func rotate(a \iso *node, b \iso *node) (\iso *node, \iso *node) {
	a, b = \restore func(x \iso *node, y \iso *node) (\iso *node, \iso *node) {
		tmp := x.next
		x.next = y.next
		y.next = tmp
		return x, y
	}(a, b)
	return a, b
}
`,
		},
		{
			name: "linked list insertion",
			fn: `func insert(head \iso *node, item \iso *node) \iso *node {
	head = \restore func(h \iso *node, n \iso *node) \iso *node {
		n.next = h.next
		h.next = n
		return h
	}(head, item)
	return head
}
`,
		},
		{
			name: "nil field write",
			fn: `func clear(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		x.next = nil
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "if with scalar capture",
			fn: `func maybeClear(a \iso *node, clear bool) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		if clear {
			x.next = nil
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "if with imm capture",
			fn: `func maybeClearFrozen(a \iso *node, frozen \imm *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		if frozen == nil {
			x.next = nil
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "explicit imm parameter",
			fn: `func inspectImm(a \iso *node, frozen \imm *node) \iso *node {
	a = \restore func(x \iso *node, r \imm *node) \iso *node {
		if r == nil {
			x.next = nil
		}
		return x
	}(a, frozen)
	return a
}
`,
		},
		{
			name: "branch projection overwritten on both paths",
			fn: `func branchInsert(a \iso *node, b \iso *node, cond bool) \iso *node {
	a = \restore func(x \iso *node, y \iso *node) \iso *node {
		if cond {
			x.next = y
		} else {
			tmp := x.next
			x.next = y
			y.next = tmp
		}
		return x
	}(a, b)
	return a
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := checkGownSource(t, "restore_accept.gown", restoreSource(tt.fn)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRestoreRejectsInvalidRegions(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		code CheckerErrorCode
	}{
		{
			name: "not assignment",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	return \restore func(x \iso *node) \iso *node {
		return x
	}(a)
}
`,
		},
		{
			name: "expression statement",
			code: GWN013,
			fn: `func f(a \iso *node) {
	\restore func(x \iso *node) {
		return
	}(a)
}
`,
		},
		{
			name: "non iso result",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) *node {
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "named result",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) (out \iso *node) {
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "untracked pointer parameter",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x *node) \iso *node {
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "borrow parameter",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \mub *node) \iso *node {
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "no iso parameter",
			code: GWN013,
			fn: `func f(a \iso *node, frozen \imm *node) \iso *node {
	a = \restore func(x \imm *node) \iso *node {
		return x
	}(frozen)
	return a
}
`,
		},
		{
			name: "field projection argument",
			code: GWN013,
			fn: `func f(h \iso *holder) \iso *node {
	out := \restore func(x \iso *node) \iso *node {
		return x
	}(h.child)
	return out
}
`,
		},
		{
			name: "ordinary call",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		x = helper(x)
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "method call",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		x.touch()
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "builtin call",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		println(x)
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "channel send",
			code: GWN013,
			fn: `func f(a \iso *node, ch chan \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		ch <- x
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "channel receive",
			code: GWN013,
			fn: `func f(a \iso *node, ch chan \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		_ = <-ch
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "select",
			code: GWN013,
			fn: `func f(a \iso *node, ch chan \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		select {
		case ch <- x:
		default:
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "go statement",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		go helper(x)
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "defer statement",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		defer helper(x)
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "goto and label",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		goto done
	done:
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "loop",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		for x != nil {
			x.next = nil
			break
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "range",
			code: GWN013,
			fn: `func f(a \iso *node, xs []int) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		for range xs {
			x.next = nil
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "switch",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		switch {
		case x == nil:
			return x
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "early return",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		if x == nil {
			return x
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "address of",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		_ = &x
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "composite literal",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		_ = node{}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "index expression",
			code: GWN013,
			fn: `func f(a \iso *node, xs []int) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		_ = xs[0]
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "type assertion",
			code: GWN013,
			fn: `func f(a \iso *node, v any) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		_ = v.(*node)
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "mutate global",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		global = x
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "mutable pointer capture",
			code: GWN013,
			fn: `func f(a \iso *node, outer *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		if outer == nil {
			x.next = nil
		}
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "scalar capture mutation",
			code: GWN013,
			fn: `func f(a \iso *node, scalar int) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		scalar = 2
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "untracked backlink store",
			code: GWN013,
			fn: `func f(a \iso *node, b \iso *node) \iso *node {
	a = \restore func(x \iso *node, y \iso *node) \iso *node {
		x.prev = y
		return x
	}(a, b)
	return a
}
`,
		},
		{
			name: "interface boxing",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		var boxed any
		boxed = x
		_ = boxed
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "imm value into iso field",
			code: GWN013,
			fn: `func f(a \iso *node, frozen \imm *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		x.next = frozen
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "projection moved but not overwritten",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		tmp := x.next
		_ = tmp
		return x
	}(a)
	return a
}
`,
		},
		{
			name: "return local alias",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		tmp := x
		return tmp
	}(a)
	return a
}
`,
		},
		{
			name: "duplicate returned root",
			code: GWN013,
			fn: `func f(a \iso *node) (\iso *node, \iso *node) {
	a, a = \restore func(x \iso *node) (\iso *node, \iso *node) {
		return x, x
	}(a)
	return a, a
}
`,
		},
		{
			name: "incorporated root also returned",
			code: GWN013,
			fn: `func f(head \iso *node, item \iso *node) (\iso *node, \iso *node) {
	head, item = \restore func(h \iso *node, n \iso *node) (\iso *node, \iso *node) {
		h.next = n
		return h, n
	}(head, item)
	return head, item
}
`,
		},
		{
			name: "same root incorporated into two returned graphs",
			code: GWN013,
			fn: `func f(a \iso *node, b \iso *node, c \iso *node) (\iso *node, \iso *node) {
	a, b = \restore func(x \iso *node, y \iso *node, z \iso *node) (\iso *node, \iso *node) {
		x.next = z
		y.next = z
		return x, y
	}(a, b, c)
	return a, b
}
`,
		},
		{
			name: "root incorporated into unreturned graph",
			code: GWN013,
			fn: `func f(a \iso *node, b \iso *node, c \iso *node) \iso *node {
	a = \restore func(x \iso *node, y \iso *node, z \iso *node) \iso *node {
		y.next = z
		return x
	}(a, b, c)
	return a
}
`,
		},
		{
			name: "reassign iso parameter root",
			code: GWN013,
			fn: `func f(a \iso *node, b \iso *node) \iso *node {
	a = \restore func(x \iso *node, y \iso *node) \iso *node {
		x = y
		return x
	}(a, b)
	return a
}
`,
		},
		{
			name: "unsafe intrinsic",
			code: GWN013,
			fn: `func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		x = \unsafe(x)
		return x
	}(a)
	return a
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkGownSource(t, "restore_reject.gown", restoreSource(tt.fn))
			requireCheckerCode(t, err, tt.code)
		})
	}
}

func TestRestoreConsumesMovedArgument(t *testing.T) {
	src := restoreSource(`func f(src \iso *node) \iso *node {
	dst := \restore func(x \iso *node) \iso *node {
		return x
	}(src)
	_ = src
	return dst
}
`)
	err := checkGownSource(t, "restore_use_after.gown", src)
	requireCheckerCode(t, err, GWN001)
}

func TestRestoreEmissionNilsConsumedArgument(t *testing.T) {
	src := restoreSource(`func f(src \iso *node) \iso *node {
	dst := \restore func(x \iso *node) \iso *node {
		return x
	}(src)
	return dst
}
`)
	dir := writeGownDir(t, map[string]string{"restore_emit.gown": src})
	if err := NewGownPackage(dir).Check(); err != nil {
		t.Fatal(err)
	}
	goBytes, err := os.ReadFile(filepath.Join(dir, "restore_emit.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(goBytes, []byte(`\restore`)) {
		t.Fatalf("generated Go still contains restore marker:\n%s", goBytes)
	}
	if !bytes.Contains(goBytes, []byte(`src = nil`)) {
		t.Fatalf("generated Go does not nil consumed restore argument:\n%s", goBytes)
	}
}

func TestRestoreEmissionDoesNotNilReassignedRoots(t *testing.T) {
	src := restoreSource(`func f(a \iso *node, b \iso *node) (\iso *node, \iso *node) {
	a, b = \restore func(x \iso *node, y \iso *node) (\iso *node, \iso *node) {
		return x, y
	}(a, b)
	return a, b
}
`)
	dir := writeGownDir(t, map[string]string{"restore_emit_reassign.gown": src})
	if err := NewGownPackage(dir).Check(); err != nil {
		t.Fatal(err)
	}
	goBytes, err := os.ReadFile(filepath.Join(dir, "restore_emit_reassign.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(goBytes), "a = nil") || strings.Contains(string(goBytes), "b = nil") {
		t.Fatalf("generated Go nilled roots that were restored on the LHS:\n%s", goBytes)
	}
}
