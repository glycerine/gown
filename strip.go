package gown

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	observerDirectiveLexeme  = `\\\\observer`
	observerDirectiveComment = `//\\observer`
)

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
	if bytes.HasPrefix(gownSrc[offset:], []byte(observerDirectiveLexeme)) {
		return scanObserverDirective(gf, emit, analysis, gownSrc, lineStarts, offset)
	}

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
	case "Clone":
		if !hasCall {
			return end, scanError(gf.path, span, `\Clone must be used as a call`)
		}
		recordIntrinsic(gf, emit, analysis, span, IntrinsicCloneExported)
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
	case "swap":
		if !hasCall {
			return end, scanError(gf.path, span, `\swap must be used as a call`)
		}
		recordIntrinsic(gf, emit, analysis, span, IntrinsicSwap)
	case "observer":
		return end, scanError(gf.path, span, fmt.Sprintf(`observer directive must be spelled %s`, observerDirectiveLexeme))
	default:
		return end, scanError(gf.path, span, fmt.Sprintf("unknown Gown token %q", lexeme))
	}
	return end, nil
}

func scanObserverDirective(gf *gownFile, emit, analysis, gownSrc []byte, lineStarts []int, offset int) (int, error) {
	end := offset + len(observerDirectiveLexeme)
	lc := lineColFromStarts(lineStarts, offset)
	span := SourceSpan{
		Offset: offset,
		End:    end,
		Line:   lc.line,
		Col:    lc.col,
		Lexeme: observerDirectiveLexeme,
	}
	lineStart, lineEnd := sourceLineRange(gownSrc, offset)
	if !linePrefixWhitespace(gownSrc[lineStart:offset]) {
		return end, scanError(gf.path, span, fmt.Sprintf(`%s must appear on its own line`, observerDirectiveLexeme))
	}
	if end < lineEnd && gownSrc[end] != ' ' && gownSrc[end] != '\t' {
		return end, scanError(gf.path, span, fmt.Sprintf(`%s target must be separated by whitespace`, observerDirectiveLexeme))
	}
	target := parseObserverTarget(gownSrc[end:lineEnd])
	if target == "" {
		return end, scanError(gf.path, span, fmt.Sprintf(`%s requires a target`, observerDirectiveLexeme))
	}
	recordObserverDirective(gf, emit, analysis, span, target)
	return lineEnd, nil
}

func recordCapQualifier(gf *gownFile, emit, analysis []byte, span SourceSpan, cap Cap) {
	token := &AnnotationToken{Span: span, Kind: AnnotationCapQualifier}
	gf.annotations = append(gf.annotations, token)
	gf.capQualifiers = append(gf.capQualifiers, &CapQualifierAnnotation{
		Span:         span,
		Cap:          cap,
		TargetOffset: firstNonSpaceOffset(emit, span.End),
	})
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

func recordObserverDirective(gf *gownFile, emit, analysis []byte, span SourceSpan, target string) {
	token := &AnnotationToken{Span: span, Kind: AnnotationObserverDirective}
	gf.annotations = append(gf.annotations, token)
	gf.observers = append(gf.observers, &ObserverAnnotation{Span: span, Target: target})
	copy(emit[span.Offset:span.End], observerDirectiveComment)
	copy(analysis[span.Offset:span.End], observerDirectiveComment)
}

func intrinsicAnalysisName(intrinsic IntrinsicKind) string {
	switch intrinsic {
	case IntrinsicMub:
		return "mub_"
	case IntrinsicRob:
		return "rob_"
	case IntrinsicNew:
		return "new_"
	case IntrinsicClone, IntrinsicCloneExported:
		return "clone_"
	case IntrinsicFreeze:
		return "freeze_"
	case IntrinsicUnsafe:
		return "unsafe_"
	case IntrinsicSwap:
		return "swap_"
	default:
		return ""
	}
}

func replaceWithSpaces(src []byte, span SourceSpan) {
	replaceRangeWithSpaces(src, span.Offset, span.End)
}

func replaceRangeWithSpaces(src []byte, start, end int) {
	for i := start; i < end; i++ {
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

func firstNonSpaceOffset(src []byte, offset int) int {
	for offset < len(src) {
		switch src[offset] {
		case ' ', '\t', '\n', '\r':
			offset++
			continue
		default:
			return offset
		}
	}
	return 0
}

func sourceLineRange(src []byte, offset int) (int, int) {
	start := offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := offset
	for end < len(src) && src[end] != '\n' {
		end++
	}
	return start, end
}

func linePrefixWhitespace(prefix []byte) bool {
	for _, b := range prefix {
		if b != ' ' && b != '\t' {
			return false
		}
	}
	return true
}

func parseObserverTarget(src []byte) string {
	if idx := bytes.Index(src, []byte("//")); idx >= 0 {
		src = src[:idx]
	}
	target := strings.TrimSpace(string(src))
	target = strings.TrimSuffix(target, "()")
	return strings.TrimSpace(target)
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
