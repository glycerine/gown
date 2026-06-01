package gown

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

const gownRestoreMultiResultSource = `package example

type node struct {
	next \iso *node
	value int
}

func rotate(a \iso *node, b \iso *node) (\iso *node, \iso *node) {
	a, b = \restore func(x \iso *node, y \iso *node) (\iso *node, \iso *node) {
		tmp := x.next
		x.next = y.next
		y.next = tmp
		return x, y
	}(a, b)
	return a, b
}
`

const gownRestoreRejectsCallSource = `package example

type node struct {
	next \iso *node
}

func helper(x *node) *node { return x }

func f(a \iso *node) \iso *node {
	a = \restore func(x \iso *node) \iso *node {
		x = helper(x)
		return x
	}(a)
	return a
}
`

const gownRestoreRejectsUntrackedBacklinkSource = `package example

type node struct {
	next \iso *node
	prev *node
}

func f(a \iso *node, b \iso *node) \iso *node {
	a = \restore func(x \iso *node, y \iso *node) \iso *node {
		x.prev = y
		return x
	}(a, b)
	return a
}
`

const gownRestoreInsertSource = `package example

type node struct {
	next \iso *node
}

func insert(head \iso *node, item \iso *node) \iso *node {
	head = \restore func(h \iso *node, n \iso *node) \iso *node {
		n.next = h.next
		h.next = n
		return h
	}(head, item)
	return head
}
`

const gownRestoreRejectsDuplicateReturnedGraphSource = `package example

type node struct {
	next \iso *node
}

func bad(head \iso *node, item \iso *node) (\iso *node, \iso *node) {
	head, item = \restore func(h \iso *node, n \iso *node) (\iso *node, \iso *node) {
		h.next = n
		return h, n
	}(head, item)
	return head, item
}
`

const gownRestoreUseAfterSource = `package example

type node struct {
	next \iso *node
}

func f(src \iso *node) \iso *node {
	dst := \restore func(x \iso *node) \iso *node {
		return x
	}(src)
	_ = src
	return dst
}
`

const gownRestoreEmitNilSource = `package example

type node struct {
	next \iso *node
}

func f(src \iso *node) \iso *node {
	dst := \restore func(x \iso *node) \iso *node {
		return x
	}(src)
	return dst
}
`

func TestScanAndClassifyRestore(t *testing.T) {
	emitSrc, analysisSrc, gf, err := scanAndClassify("restore.gown", []byte(gownRestoreEmitNilSource))
	if err != nil {
		t.Fatal(err)
	}
	if len(gf.restores) != 1 {
		t.Fatalf("expected 1 restore annotation, got %d", len(gf.restores))
	}
	if len(gf.annotations) == 0 || gf.annotations[0].Kind != AnnotationCapQualifier {
		t.Fatalf("expected ordinary annotations to remain ordered, got %#v", gf.annotations)
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
}

func TestRestoreAllowsMultiResultPointerSurgery(t *testing.T) {
	err := checkGownSource(t, "restore_multi.gown", gownRestoreMultiResultSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRestoreAllowsLinkedListInsertion(t *testing.T) {
	err := checkGownSource(t, "restore_insert.gown", gownRestoreInsertSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRejectsCalls(t *testing.T) {
	err := checkGownSource(t, "restore_call.gown", gownRestoreRejectsCallSource)
	requireCheckerCode(t, err, GWN013)
}

func TestRestoreRejectsUntrackedBacklinkStores(t *testing.T) {
	err := checkGownSource(t, "restore_backlink.gown", gownRestoreRejectsUntrackedBacklinkSource)
	requireCheckerCode(t, err, GWN013)
}

func TestRestoreRejectsDuplicateReturnedGraph(t *testing.T) {
	err := checkGownSource(t, "restore_duplicate.gown", gownRestoreRejectsDuplicateReturnedGraphSource)
	requireCheckerCode(t, err, GWN013)
}

func TestRestoreConsumesMovedArgument(t *testing.T) {
	err := checkGownSource(t, "restore_use_after.gown", gownRestoreUseAfterSource)
	requireCheckerCode(t, err, GWN001)
}

func TestRestoreEmissionNilsConsumedArgument(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"restore_emit.gown": gownRestoreEmitNilSource})
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
