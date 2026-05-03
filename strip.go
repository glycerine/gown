package gown

import (
	"fmt"
	"go/types"
)

type gownFile struct {
	path          string
	annotations   []*AnnotationToken
	capQualifiers []*CapQualifierAnnotation
	intrinsics    []*IntrinsicAnnotation
	unsafeUses    []*UnsafeBoundaryAnnotation
	iso           []*isoAnnotation
	create        []*createAnew
	boundary      []*boundaryCrossing
}

type Cap uint8

const (
	CapInvalid Cap = iota
	CapIso
	CapMub
	CapRob
	CapImm
	CapUntracked
)

type AnnotationKind uint8

const (
	AnnotationInvalid AnnotationKind = iota
	AnnotationCapQualifier
	AnnotationIntrinsic
	AnnotationUnsafeBoundary
)

type IntrinsicKind uint8

const (
	IntrinsicInvalid IntrinsicKind = iota
	IntrinsicMub
	IntrinsicRob
	IntrinsicNew
	IntrinsicClone
	IntrinsicFreeze
	IntrinsicUnsafe
)

type SourceSpan struct {
	Offset int
	End    int
	Line   int
	Col    int
	Lexeme string
}

type AnnotationToken struct {
	Span SourceSpan
	Kind AnnotationKind
}

type CapQualifierAnnotation struct {
	Span SourceSpan
	Cap  Cap
}

type IntrinsicAnnotation struct {
	Span      SourceSpan
	Intrinsic IntrinsicKind
}

type UnsafeBoundaryAnnotation struct {
	Span SourceSpan
}

type boundaryCrossing struct {
	offset   int        // 0-based byte offset
	line     int        // 1-based line number
	col      int        // 1-based column number
	kind     string     // "chan-send", "chan-recv", "go-arg", "go-capture", "pkg-var", "import-sig"
	typeName string     // human-readable type name
	goType   types.Type // resolved type from type checker
	funcName string     // containing function (empty for pkg-var)
	scope    *region    // innermost enclosing block
}

type createAnew struct {
	offset   int     // 0-based byte offset of the creation expression
	line     int     // 1-based line number
	col      int     // 1-based column number (bytes, not runes)
	kind     string  // "new", "make", or "ampersand" (for &T{})
	typeName string  // the type being created (e.g. "payload")
	funcName string  // containing function name (filled in after AST parse)
	scope    *region // innermost enclosing { } block (filled in after AST parse)
}

type region struct {
	beg  int // byte offset of '{' (inclusive)
	endx int // byte offset just past '}' (exclusive)
}

type isoAnnotation struct {
	offset   int     // 0-based byte offset of the '\' in \iso
	line     int     // 1-based line number
	col      int     // 1-based column number (bytes, not runes)
	funcName string  // containing function name (filled in after AST parse)
	scope    *region // innermost enclosing { } block (filled in after AST parse)
}

// scanAndStrip finds Gown annotations, records their positions, and returns
// the emit source view. It is kept as a compatibility wrapper for existing
// callers that predate the analysis-source view.
func scanAndStrip(path string, gownSrc []byte) (goSrc []byte, gf *gownFile) {
	goSrc, _, gf, _ = scanAndClassify(path, gownSrc)
	return goSrc, gf
}

// scanAndClassify records all Gown annotation tokens and produces two source
// views:
//   - emitSrc: annotations erased with spaces
//   - analysisSrc: type qualifiers erased, expression intrinsics rewritten to
//     length-preserving fake identifiers for later Go/SSA analysis
func scanAndClassify(path string, gownSrc []byte) (emitSrc, analysisSrc []byte, gf *gownFile, err error) {
	emit := make([]byte, len(gownSrc))
	copy(emit, gownSrc)
	analysis := make([]byte, len(gownSrc))
	copy(analysis, gownSrc)
	gf = &gownFile{path: path}

	lineStarts := buildLineStarts(gownSrc)

	for i := 0; i < len(gownSrc); {
		switch gownSrc[i] {
		case '/':
			if i+1 < len(gownSrc) && gownSrc[i+1] == '/' {
				i = skipLineComment(gownSrc, i+2)
				continue
			}
			if i+1 < len(gownSrc) && gownSrc[i+1] == '*' {
				i = skipBlockComment(gownSrc, i+2)
				continue
			}
		case '"':
			i = skipQuoted(gownSrc, i+1, '"')
			continue
		case '\'':
			i = skipQuoted(gownSrc, i+1, '\'')
			continue
		case '`':
			i = skipRawString(gownSrc, i+1)
			continue
		case '\\':
			next, scanErr := scanGownToken(gf, emit, analysis, gownSrc, lineStarts, i)
			if scanErr != nil {
				return nil, nil, nil, scanErr
			}
			i = next
			continue
		}
		i++
	}

	return emit, analysis, gf, nil
}

func scanGownToken(gf *gownFile, emit, analysis, gownSrc []byte, lineStarts []int, offset int) (int, error) {
	end := offset + 1
	if end >= len(gownSrc) || !isIdentStart(gownSrc[end]) {
		return offset + 1, fmt.Errorf("%s:%d:%d: invalid Gown token",
			gf.path, lineColFromStarts(lineStarts, offset).line, lineColFromStarts(lineStarts, offset).col)
	}
	for end < len(gownSrc) && isIdentPart(gownSrc[end]) {
		end++
	}

	lexeme := string(gownSrc[offset:end])
	name := lexeme[1:]
	lc := lineColFromStarts(lineStarts, offset)
	span := SourceSpan{
		Offset: offset,
		End:    end,
		Line:   lc.line,
		Col:    lc.col,
		Lexeme: lexeme,
	}
	hasCall := followedByCall(gownSrc, end)

	switch name {
	case "iso":
		if hasCall {
			return end, scanError(gf.path, span, `\iso is a type qualifier, not an intrinsic`)
		}
		recordCapQualifier(gf, emit, analysis, span, CapIso)
	case "imm":
		if hasCall {
			return end, scanError(gf.path, span, `\imm is a type qualifier, not an intrinsic`)
		}
		recordCapQualifier(gf, emit, analysis, span, CapImm)
	case "mub":
		if hasCall {
			recordIntrinsic(gf, emit, analysis, span, IntrinsicMub)
		} else {
			recordCapQualifier(gf, emit, analysis, span, CapMub)
		}
	case "rob":
		if hasCall {
			recordIntrinsic(gf, emit, analysis, span, IntrinsicRob)
		} else {
			recordCapQualifier(gf, emit, analysis, span, CapRob)
		}
	case "new":
		if !hasCall {
			return end, scanError(gf.path, span, `\new must be used as a call`)
		}
		recordIntrinsic(gf, emit, analysis, span, IntrinsicNew)
	case "clone":
		if !hasCall {
			return end, scanError(gf.path, span, `\clone must be used as a call`)
		}
		recordIntrinsic(gf, emit, analysis, span, IntrinsicClone)
	case "freeze":
		if !hasCall {
			return end, scanError(gf.path, span, `\freeze must be used as a call`)
		}
		recordIntrinsic(gf, emit, analysis, span, IntrinsicFreeze)
	case "unsafe":
		if !hasCall {
			return end, scanError(gf.path, span, `\unsafe must be used as a call`)
		}
		recordIntrinsic(gf, emit, analysis, span, IntrinsicUnsafe)
	default:
		return end, scanError(gf.path, span, fmt.Sprintf("unknown Gown token %q", lexeme))
	}
	return end, nil
}

func recordCapQualifier(gf *gownFile, emit, analysis []byte, span SourceSpan, cap Cap) {
	token := &AnnotationToken{Span: span, Kind: AnnotationCapQualifier}
	gf.annotations = append(gf.annotations, token)
	gf.capQualifiers = append(gf.capQualifiers, &CapQualifierAnnotation{Span: span, Cap: cap})
	if cap == CapIso {
		gf.iso = append(gf.iso, &isoAnnotation{
			offset: span.Offset,
			line:   span.Line,
			col:    span.Col,
		})
	}
	replaceWithSpaces(emit, span)
	replaceWithSpaces(analysis, span)
}

func recordIntrinsic(gf *gownFile, emit, analysis []byte, span SourceSpan, intrinsic IntrinsicKind) {
	kind := AnnotationIntrinsic
	if intrinsic == IntrinsicUnsafe {
		kind = AnnotationUnsafeBoundary
		gf.unsafeUses = append(gf.unsafeUses, &UnsafeBoundaryAnnotation{Span: span})
	}
	token := &AnnotationToken{Span: span, Kind: kind}
	gf.annotations = append(gf.annotations, token)
	gf.intrinsics = append(gf.intrinsics, &IntrinsicAnnotation{Span: span, Intrinsic: intrinsic})

	replaceWithSpaces(emit, span)
	copy(analysis[span.Offset:span.End], []byte(intrinsicAnalysisName(intrinsic)))
}

func intrinsicAnalysisName(intrinsic IntrinsicKind) string {
	switch intrinsic {
	case IntrinsicMub:
		return "mub_"
	case IntrinsicRob:
		return "rob_"
	case IntrinsicNew:
		return "new_"
	case IntrinsicClone:
		return "clone_"
	case IntrinsicFreeze:
		return "freeze_"
	case IntrinsicUnsafe:
		return "unsafe_"
	default:
		return ""
	}
}

func replaceWithSpaces(src []byte, span SourceSpan) {
	for i := span.Offset; i < span.End; i++ {
		src[i] = ' '
	}
}

func scanError(path string, span SourceSpan, msg string) error {
	return fmt.Errorf("%s:%d:%d: %s", path, span.Line, span.Col, msg)
}

func buildLineStarts(src []byte) []int {
	lineStarts := []int{0}
	for i, b := range src {
		if b == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	return lineStarts
}

type lineCol struct {
	line int
	col  int
}

func lineColFromStarts(lineStarts []int, offset int) lineCol {
	lo, hi := 0, len(lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if lineStarts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lineCol{
		line: lo + 1,
		col:  offset - lineStarts[lo] + 1,
	}
}

func followedByCall(src []byte, offset int) bool {
	for offset < len(src) {
		switch src[offset] {
		case ' ', '\t', '\n', '\r':
			offset++
			continue
		case '(':
			return true
		default:
			return false
		}
	}
	return false
}

func skipLineComment(src []byte, offset int) int {
	for offset < len(src) && src[offset] != '\n' {
		offset++
	}
	return offset
}

func skipBlockComment(src []byte, offset int) int {
	for offset+1 < len(src) {
		if src[offset] == '*' && src[offset+1] == '/' {
			return offset + 2
		}
		offset++
	}
	return len(src)
}

func skipRawString(src []byte, offset int) int {
	for offset < len(src) {
		if src[offset] == '`' {
			return offset + 1
		}
		offset++
	}
	return len(src)
}

func skipQuoted(src []byte, offset int, quote byte) int {
	for offset < len(src) {
		if src[offset] == '\\' {
			offset += 2
			continue
		}
		if src[offset] == quote {
			return offset + 1
		}
		offset++
	}
	return len(src)
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isIdentPart(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}
