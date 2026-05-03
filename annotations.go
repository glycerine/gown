package gown

type Cap uint8

const (
	CapInvalid Cap = iota
	CapIso
	CapMub
	CapRob
	CapImm
	CapUntracked
)

func (cap Cap) String() string {
	switch cap {
	case CapIso:
		return `\iso`
	case CapMub:
		return `\mub`
	case CapRob:
		return `\rob`
	case CapImm:
		return `\imm`
	case CapUntracked:
		return "untracked"
	default:
		return "invalid"
	}
}

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
	Span         SourceSpan
	Cap          Cap
	TargetOffset int
}

type IntrinsicAnnotation struct {
	Span      SourceSpan
	Intrinsic IntrinsicKind
}

type UnsafeBoundaryAnnotation struct {
	Span SourceSpan
}
