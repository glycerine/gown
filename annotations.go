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
	IntrinsicCloneExported
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

type GownSourceViews struct {
	EmitSrc       []byte
	AnalysisSrc   []byte
	Annotations   []*AnnotationToken
	CapQualifiers []*CapQualifierAnnotation
	Intrinsics    []*IntrinsicAnnotation
	UnsafeUses    []*UnsafeBoundaryAnnotation
}

func ClassifyGownSource(path string, src []byte) (*GownSourceViews, error) {
	emitSrc, analysisSrc, gf, err := scanAndClassify(path, src)
	if err != nil {
		return nil, err
	}
	return &GownSourceViews{
		EmitSrc:       emitSrc,
		AnalysisSrc:   analysisSrc,
		Annotations:   gf.annotations,
		CapQualifiers: gf.capQualifiers,
		Intrinsics:    gf.intrinsics,
		UnsafeUses:    gf.unsafeUses,
	}, nil
}
