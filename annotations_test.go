package gown

import (
	"bytes"
	"strings"
	"testing"
)

const gownAllQualifiersSource = `package example

type Msg struct {
	Data []byte
}

type Holder struct {
	Owned  \iso *Msg
	Borrow \mub *Msg
	Read   \rob *Msg
	Frozen \imm *Msg
}

func Use(ch chan \iso *Msg, x \mub *Msg) \rob *Msg {
	return nil
}
`

func TestScanAndClassifyOstampQualifiers(t *testing.T) {
	emitSrc, analysisSrc, gf, err := scanAndClassify("qualifiers.gown", []byte(gownAllQualifiersSource))
	if err != nil {
		t.Fatal(err)
	}

	wantCaps := []Cap{CapIso, CapMub, CapRob, CapImm, CapIso, CapMub, CapRob}
	if len(gf.capQualifiers) != len(wantCaps) {
		t.Fatalf("expected %d ownerstamp qualifiers, got %d", len(wantCaps), len(gf.capQualifiers))
	}
	if len(gf.annotations) != len(wantCaps) {
		t.Fatalf("expected %d annotation tokens, got %d", len(wantCaps), len(gf.annotations))
	}

	for i, want := range wantCaps {
		got := gf.capQualifiers[i]
		if got.Cap != want {
			t.Fatalf("cap qualifier %d: got %v, want %v", i, got.Cap, want)
		}
		if got.Span.End != got.Span.Offset+len(got.Span.Lexeme) {
			t.Fatalf("cap qualifier %d: invalid span [%d,%d) for %q",
				i, got.Span.Offset, got.Span.End, got.Span.Lexeme)
		}
		wantLine, wantCol := testLineCol(gownAllQualifiersSource, got.Span.Offset)
		if got.Span.Line != wantLine || got.Span.Col != wantCol {
			t.Fatalf("cap qualifier %d: got line/col %d/%d, want %d/%d",
				i, got.Span.Line, got.Span.Col, wantLine, wantCol)
		}
		for off := got.Span.Offset; off < got.Span.End; off++ {
			if emitSrc[off] != ' ' || analysisSrc[off] != ' ' {
				t.Fatalf("cap qualifier %d: expected whitespace replacement at offset %d", i, off)
			}
		}
	}

	for _, token := range [][]byte{[]byte(`\iso`), []byte(`\mub`), []byte(`\rob`), []byte(`\imm`)} {
		if bytes.Contains(emitSrc, token) {
			t.Fatalf("emit source still contains %q", token)
		}
		if bytes.Contains(analysisSrc, token) {
			t.Fatalf("analysis source still contains %q", token)
		}
	}
}

const gownIntrinsicSource = `package example

type Msg struct{}

func (m *Msg) clone() *Msg { return &Msg{} }
func (m *Msg) Clone() *Msg { return &Msg{} }

func UseMub(x \iso *Msg) {
	b := \mub(x)
	_ = b
}

func UseRob(x \imm *Msg) {
	r := \rob(x)
	_ = r
}

func UseFreeze(x \iso *Msg) {
	y := \freeze(x)
	_ = y
}

func UseClone(x \imm *Msg) {
	z := \clone(x)
	_ = z
}

func UseExportedClone(x \imm *Msg) {
	z := \Clone(x)
	_ = z
}

func UseUnsafe(x *Msg) {
	u := \unsafe(x)
	_ = u
}

func UseNew() {
	p := \new(Msg{})
	_ = p
}

func UseSwap(x \iso *Msg, y \iso *Msg) {
	\swap(x, y)
}
`

func TestScanAndClassifyIntrinsics(t *testing.T) {
	emitSrc, analysisSrc, gf, err := scanAndClassify("intrinsics.gown", []byte(gownIntrinsicSource))
	if err != nil {
		t.Fatal(err)
	}

	wantIntrinsics := []IntrinsicKind{
		IntrinsicMub,
		IntrinsicRob,
		IntrinsicFreeze,
		IntrinsicClone,
		IntrinsicCloneExported,
		IntrinsicUnsafe,
		IntrinsicNew,
		IntrinsicSwap,
	}
	if len(gf.intrinsics) != len(wantIntrinsics) {
		t.Fatalf("expected %d intrinsics, got %d", len(wantIntrinsics), len(gf.intrinsics))
	}
	if len(gf.unsafeUses) != 1 {
		t.Fatalf("expected 1 unsafe use, got %d", len(gf.unsafeUses))
	}
	for i, want := range wantIntrinsics {
		got := gf.intrinsics[i]
		if got.Intrinsic != want {
			t.Fatalf("intrinsic %d: got %v, want %v", i, got.Intrinsic, want)
		}
	}

	for _, want := range []string{"mub_(x)", "rob_(x)", "freeze_(x)", "clone_(x)", "unsafe_(x)", "new_(Msg{})", "swap_(x, y)"} {
		if !strings.Contains(string(analysisSrc), want) {
			t.Fatalf("analysis source does not contain %q:\n%s", want, analysisSrc)
		}
	}
	if strings.Count(string(analysisSrc), "clone_(x)") != 2 {
		t.Fatalf("analysis source should contain both clone intrinsic calls:\n%s", analysisSrc)
	}
	if bytes.Contains(emitSrc, []byte(`\`)) {
		t.Fatalf("emit source still contains a Gown backslash token:\n%s", emitSrc)
	}
}

func TestAnalyzeIntrinsicCallsTypeCheckWithSyntheticHelpers(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"intrinsics.gown": gownIntrinsicSource})
	gp := NewGownPackage(dir)

	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}
}

func TestScanAndClassifyMixedOrderAndUnsafeMarker(t *testing.T) {
	src := `package example

type Msg struct{}

func Use(x \iso *Msg) {
	y := \unsafe(x)
	_ = \mub(y)
}
`

	_, _, gf, err := scanAndClassify("mixed.gown", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(gf.annotations) != 3 {
		t.Fatalf("expected 3 annotation tokens, got %d", len(gf.annotations))
	}
	wantLexemes := []string{`\iso`, `\unsafe`, `\mub`}
	wantKinds := []AnnotationKind{
		AnnotationCapQualifier,
		AnnotationUnsafeBoundary,
		AnnotationIntrinsic,
	}
	for i := range wantLexemes {
		if gf.annotations[i].Span.Lexeme != wantLexemes[i] {
			t.Fatalf("annotation %d: got lexeme %q, want %q",
				i, gf.annotations[i].Span.Lexeme, wantLexemes[i])
		}
		if gf.annotations[i].Kind != wantKinds[i] {
			t.Fatalf("annotation %d: got kind %v, want %v",
				i, gf.annotations[i].Kind, wantKinds[i])
		}
	}
	if len(gf.unsafeUses) != 1 {
		t.Fatalf("expected 1 unsafe boundary marker, got %d", len(gf.unsafeUses))
	}
	if len(gf.intrinsics) != 2 {
		t.Fatalf("expected unsafe and mub to both be intrinsics, got %d", len(gf.intrinsics))
	}
	if gf.intrinsics[0].Intrinsic != IntrinsicUnsafe {
		t.Fatalf("first intrinsic = %v, want %v", gf.intrinsics[0].Intrinsic, IntrinsicUnsafe)
	}
}

func TestScanAndClassifyObserverDirective(t *testing.T) {
	src := `package example

\\\\observer fmt.Printf()

func main() {}
`

	emitSrc, analysisSrc, gf, err := scanAndClassify("observer.gown", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(gf.observers) != 1 {
		t.Fatalf("expected 1 observer directive, got %d", len(gf.observers))
	}
	if gf.observers[0].Target != "fmt.Printf" {
		t.Fatalf("observer target = %q, want fmt.Printf", gf.observers[0].Target)
	}
	if len(gf.annotations) != 1 || gf.annotations[0].Kind != AnnotationObserverDirective {
		t.Fatalf("annotations = %#v, want one observer directive", gf.annotations)
	}
	if !strings.Contains(string(emitSrc), observerDirectiveComment+" fmt.Printf()") {
		t.Fatalf("emit source does not contain observer comment:\n%s", emitSrc)
	}
	if !strings.Contains(string(analysisSrc), observerDirectiveComment+" fmt.Printf()") {
		t.Fatalf("analysis source does not contain observer comment:\n%s", analysisSrc)
	}
	if len(emitSrc) != len(src) || len(analysisSrc) != len(src) {
		t.Fatalf("observer source view changed byte count: emit=%d analysis=%d original=%d", len(emitSrc), len(analysisSrc), len(src))
	}
}

func TestScanAndClassifyRejectsSingleSlashObserverDirective(t *testing.T) {
	src := `package example

\observer fmt.Printf()

func main() {}
`

	_, _, _, err := scanAndClassify("observer.gown", []byte(src))
	if err == nil {
		t.Fatal("expected single-slash observer directive to be rejected")
	}
	if !strings.Contains(err.Error(), observerDirectiveLexeme) {
		t.Fatalf("error %q does not mention required observer spelling %q", err, observerDirectiveLexeme)
	}
}

func TestScanAndClassifyIgnoresCommentsAndStrings(t *testing.T) {
	src := `package example

type Msg struct{}

// \iso and \freeze(x) in a line comment
/* \mub and \rob(x) in a block comment */
func Use() {
	_ = "\imm"
	_ = '\x5c'
	_ = ` + "`\\clone(x)`" + `
	var x \iso *Msg
	_ = x
}
`

	_, _, gf, err := scanAndClassify("ignored.gown", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(gf.annotations) != 1 {
		t.Fatalf("expected exactly 1 annotation outside comments/strings, got %d", len(gf.annotations))
	}
	if len(gf.capQualifiers) != 1 || gf.capQualifiers[0].Cap != CapIso {
		t.Fatalf("expected one iso qualifier, got %#v", gf.capQualifiers)
	}
	if len(gf.intrinsics) != 0 {
		t.Fatalf("expected no intrinsics, got %d", len(gf.intrinsics))
	}
}

func TestScanAndClassifyRejectsMalformedTokens(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{
			name: "unknown",
			src:  `package p; func f(x *T) { _ = \wat(x) }`,
		},
		{
			name: "iso-call",
			src:  `package p; func f(x *T) { _ = \iso(x) }`,
		},
		{
			name: "freeze-without-call",
			src:  `package p; func f(x *T) { _ = \freeze }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := scanAndClassify(tt.name+".gown", []byte(tt.src))
			if err == nil {
				t.Fatal("expected scanner error")
			}
		})
	}
}

func testLineCol(src string, offset int) (line, col int) {
	line = 1
	lineStart := 0
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return line, offset - lineStart + 1
}
