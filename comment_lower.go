package gown

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

const gownCommentPrefix = "//gown:"

type CommentLoweringResult struct {
	Path       string
	GownPath   string
	GownSrc    []byte
	Directives []GownCommentDirective
}

type GownCommentDirective struct {
	Span SourceSpan
	Body string
}

type gownCommentCommand struct {
	Name string
	Args []string
	Raw  string
}

type commentLoweringContext struct {
	path       string
	src        []byte
	fset       *token.FileSet
	file       *ast.File
	lineStarts []int
	edits      []EmitEdit
	directives []GownCommentDirective
}

func LowerGoCommentsToGown(path string, src []byte) (*CommentLoweringResult, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	ctx := &commentLoweringContext{
		path:       path,
		src:        src,
		fset:       fset,
		file:       file,
		lineStarts: buildLineStarts(src),
	}
	if err := ctx.collectDirectives(); err != nil {
		return nil, err
	}
	for _, directive := range ctx.directives {
		if err := ctx.applyDirective(directive); err != nil {
			return nil, err
		}
	}
	out, err := applyEmitEdits(src, ctx.edits)
	if err != nil {
		return nil, err
	}
	return &CommentLoweringResult{
		Path:       path,
		GownSrc:    out,
		Directives: append([]GownCommentDirective(nil), ctx.directives...),
	}, nil
}

func ContainsGownComment(src []byte) bool {
	return bytes.Contains(src, []byte(gownCommentPrefix))
}

func (ctx *commentLoweringContext) collectDirectives() error {
	for _, group := range ctx.file.Comments {
		for _, comment := range group.List {
			if !strings.HasPrefix(comment.Text, gownCommentPrefix) {
				continue
			}
			body, err := parseGownCommentBody(comment.Text)
			pos := ctx.fset.Position(comment.Pos())
			end := ctx.fset.Position(comment.End())
			span := SourceSpan{
				Offset: pos.Offset,
				End:    end.Offset,
				Line:   pos.Line,
				Col:    pos.Column,
				Lexeme: comment.Text,
			}
			if err != nil {
				return scanError(ctx.path, span, err.Error())
			}
			ctx.directives = append(ctx.directives, GownCommentDirective{
				Span: span,
				Body: body,
			})
		}
	}
	return nil
}

func parseGownCommentBody(text string) (string, error) {
	rest := strings.TrimPrefix(text, gownCommentPrefix)
	if rest == text {
		return "", fmt.Errorf("Gown comment must start with %q", gownCommentPrefix)
	}
	if len(rest) == 0 || rest[0] != ' ' {
		return "", fmt.Errorf("%s must be followed by one or more ASCII spaces", gownCommentPrefix)
	}
	i := 0
	for i < len(rest) && rest[i] == ' ' {
		i++
	}
	if i >= len(rest) {
		return "", fmt.Errorf("%s requires a directive", gownCommentPrefix)
	}
	if rest[i] == '\t' {
		return "", fmt.Errorf("%s spacing must use ASCII spaces, not tabs", gownCommentPrefix)
	}
	return rest[i:], nil
}

func parseGownCommentCommands(body string) []gownCommentCommand {
	parts := strings.Split(body, ";")
	var out []gownCommentCommand
	for _, part := range parts {
		raw := strings.TrimSpace(part)
		if raw == "" {
			continue
		}
		words := strings.Fields(raw)
		if len(words) == 0 {
			continue
		}
		out = append(out, gownCommentCommand{
			Name: words[0],
			Args: words[1:],
			Raw:  raw,
		})
	}
	return out
}

func (ctx *commentLoweringContext) applyDirective(directive GownCommentDirective) error {
	commands := parseGownCommentCommands(directive.Body)
	if len(commands) == 0 {
		return ctx.directiveError(directive, "empty Gown directive")
	}
	if len(commands) == 1 && commands[0].Name == "observer" {
		return ctx.applyObserverDirective(directive, commands[0])
	}
	if containsCommand(commands, "restore") {
		return ctx.applyRestoreDirective(directive, commands)
	}
	if ctx.isTrailingDirective(directive) {
		return ctx.applyTrailingDirective(directive, commands)
	}
	return ctx.applyLeadingDirective(directive, commands)
}

func (ctx *commentLoweringContext) applyObserverDirective(directive GownCommentDirective, cmd gownCommentCommand) error {
	if len(cmd.Args) != 1 {
		return ctx.directiveError(directive, "observer directive requires exactly one target")
	}
	ctx.edits = append(ctx.edits, EmitEdit{
		Start:   directive.Span.Offset,
		End:     directive.Span.End,
		NewText: []byte(observerDirectiveLexeme + " " + cmd.Args[0]),
		Reason:  "lower gown observer comment",
	})
	return nil
}

func (ctx *commentLoweringContext) applyTrailingDirective(directive GownCommentDirective, commands []gownCommentCommand) error {
	line := directive.Span.Line
	if field := ctx.fieldEndingOnLine(line); field != nil {
		return ctx.applyTypeCommands(directive, commands, field.Type)
	}
	if spec := ctx.valueSpecEndingOnLine(line); spec != nil {
		return ctx.applyValueSpecDirective(directive, commands, spec)
	}
	if assign := ctx.assignEndingOnLine(line); assign != nil {
		return ctx.applyAssignDirective(directive, commands, assign)
	}
	return ctx.directiveError(directive, "could not find a same-line declaration or assignment for Gown directive")
}

func (ctx *commentLoweringContext) applyLeadingDirective(directive GownCommentDirective, commands []gownCommentCommand) error {
	if fn := ctx.funcStartingOnLine(directive.Span.Line + 1); fn != nil {
		return ctx.applyFuncSignatureCommands(directive, commands, fn.Type.Params, fn.Type.Results)
	}
	if field := ctx.fieldStartingOnLine(directive.Span.Line + 1); field != nil {
		return ctx.applyTypeCommands(directive, commands, field.Type)
	}
	if spec := ctx.valueSpecStartingOnLine(directive.Span.Line + 1); spec != nil {
		return ctx.applyValueSpecDirective(directive, commands, spec)
	}
	if assign := ctx.assignStartingOnLine(directive.Span.Line + 1); assign != nil {
		return ctx.applyAssignDirective(directive, commands, assign)
	}
	return ctx.directiveError(directive, "could not find a declaration on the line after Gown directive")
}

func (ctx *commentLoweringContext) applyValueSpecDirective(directive GownCommentDirective, commands []gownCommentCommand, spec *ast.ValueSpec) error {
	if len(commands) == 1 && isCapWord(commands[0].Name) {
		if spec.Type == nil {
			return ctx.directiveError(directive, "ownerstamp comments on var declarations require an explicit Go type")
		}
		return ctx.insertDirectCap(directive, spec.Type, capFromWord(commands[0].Name))
	}
	return ctx.applyTypeCommands(directive, commands, spec.Type)
}

func (ctx *commentLoweringContext) applyAssignDirective(directive GownCommentDirective, commands []gownCommentCommand, assign *ast.AssignStmt) error {
	if len(commands) != 1 {
		return ctx.directiveError(directive, "assignment Gown directives support exactly one command")
	}
	cmd := commands[0]
	switch cmd.Name {
	case "new":
		return ctx.lowerNewDirective(directive, assign)
	case "iso", "imm":
		if len(cmd.Args) != 0 {
			return ctx.directiveError(directive, fmt.Sprintf("%s directive does not take arguments", cmd.Name))
		}
		if assignmentRHSIsMakeChannel(assign) {
			return ctx.lowerMakeChannelElemCapDirective(directive, assign, capFromWord(cmd.Name))
		}
		return ctx.lowerCreationObjectCapDirective(directive, assign, capFromWord(cmd.Name))
	case "elem":
		if len(cmd.Args) != 1 || !isCapWord(cmd.Args[0]) {
			return ctx.directiveError(directive, "elem directive requires one ownerstamp")
		}
		return ctx.lowerMakeChannelElemCapDirective(directive, assign, capFromWord(cmd.Args[0]))
	case "mub", "rob", "freeze", "unsafe":
		if len(cmd.Args) != 0 {
			return ctx.directiveError(directive, fmt.Sprintf("%s directive does not take arguments", cmd.Name))
		}
		return ctx.wrapSingleRHS(directive, assign, `\`+cmd.Name)
	case "clone", "Clone":
		if len(cmd.Args) != 1 {
			return ctx.directiveError(directive, fmt.Sprintf("%s directive requires the receiver name", cmd.Name))
		}
		return ctx.lowerCloneDirective(directive, assign, cmd.Name, cmd.Args[0])
	case "swap":
		if len(cmd.Args) != 0 {
			return ctx.directiveError(directive, "swap directive does not take arguments")
		}
		return ctx.lowerSwapDirective(directive, assign)
	default:
		return ctx.directiveError(directive, fmt.Sprintf("unsupported assignment directive %q", cmd.Name))
	}
}

func assignmentRHSIsMakeChannel(assign *ast.AssignStmt) bool {
	if assign == nil || len(assign.Rhs) != 1 {
		return false
	}
	call, ok := unparenExpr(assign.Rhs[0]).(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	name, ok := unparenExpr(call.Fun).(*ast.Ident)
	if !ok || name.Name != "make" {
		return false
	}
	_, ok = unparenExpr(call.Args[0]).(*ast.ChanType)
	return ok
}

func (ctx *commentLoweringContext) lowerMakeChannelElemCapDirective(directive GownCommentDirective, assign *ast.AssignStmt, cap Cap) error {
	rhs, err := singleRHS(assign)
	if err != nil {
		return ctx.directiveError(directive, err.Error())
	}
	call, ok := unparenExpr(rhs).(*ast.CallExpr)
	if !ok {
		return ctx.directiveError(directive, "ownerstamp assignment directive requires make(chan ...)")
	}
	name, ok := unparenExpr(call.Fun).(*ast.Ident)
	if !ok || name.Name != "make" {
		return ctx.directiveError(directive, "ownerstamp assignment directive requires make(chan ...)")
	}
	if len(call.Args) == 0 {
		return ctx.directiveError(directive, "ownerstamp assignment directive requires make(chan ...)")
	}
	ch, ok := unparenExpr(call.Args[0]).(*ast.ChanType)
	if !ok {
		return ctx.directiveError(directive, "ownerstamp assignment directive requires make(chan ...)")
	}
	return ctx.insertDirectCap(directive, ch.Value, cap)
}

func (ctx *commentLoweringContext) lowerCreationObjectCapDirective(directive GownCommentDirective, assign *ast.AssignStmt, cap Cap) error {
	if assign.Tok != token.DEFINE {
		return ctx.directiveError(directive, "ownerstamp creation directive requires a short variable declaration")
	}
	if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return ctx.directiveError(directive, "ownerstamp creation directive requires exactly one assigned identifier and one right-hand side")
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name == "_" {
		return ctx.directiveError(directive, "ownerstamp creation directive requires an assigned identifier")
	}
	typeText, err := ctx.creationResultTypeText(directive, assign.Rhs[0])
	if err != nil {
		return err
	}
	rhsText := strings.TrimSpace(string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(assign.Rhs[0]))))
	if rhsText == "" {
		return ctx.directiveError(directive, "invalid creation right-hand side range")
	}
	replacement := []byte("var " + lhs.Name + " " + cap.String() + " " + typeText + " = " + rhsText)
	start, end := ctx.nodeOffsets(assign)
	ctx.replace(start, end, replacement, "lower gown creation ownerstamp comment")
	return nil
}

func (ctx *commentLoweringContext) creationResultTypeText(directive GownCommentDirective, rhs ast.Expr) (string, error) {
	switch expr := unparenExpr(rhs).(type) {
	case *ast.CallExpr:
		name, ok := unparenExpr(expr.Fun).(*ast.Ident)
		if !ok {
			break
		}
		switch name.Name {
		case "make":
			if len(expr.Args) == 0 {
				return "", ctx.directiveError(directive, "make creation directive requires a type argument")
			}
			return ctx.typeExprText(directive, expr.Args[0])
		case "new":
			if len(expr.Args) != 1 {
				return "", ctx.directiveError(directive, "new creation directive requires exactly one type argument")
			}
			typeText, err := ctx.typeExprText(directive, expr.Args[0])
			if err != nil {
				return "", err
			}
			return "*" + typeText, nil
		}
	case *ast.UnaryExpr:
		if expr.Op == token.AND {
			if lit, ok := unparenExpr(expr.X).(*ast.CompositeLit); ok && lit.Type != nil {
				typeText, err := ctx.typeExprText(directive, lit.Type)
				if err != nil {
					return "", err
				}
				return "*" + typeText, nil
			}
		}
	case *ast.CompositeLit:
		if expr.Type != nil {
			return ctx.typeExprText(directive, expr.Type)
		}
	}
	return "", ctx.directiveError(directive, "ownerstamp creation directive requires make(...), new(...), T{...}, or &T{...}")
}

func (ctx *commentLoweringContext) typeExprText(directive GownCommentDirective, typ ast.Expr) (string, error) {
	text := strings.TrimSpace(string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(typ))))
	if text == "" {
		return "", ctx.directiveError(directive, "invalid creation type range")
	}
	return text, nil
}

func (ctx *commentLoweringContext) applyTypeCommands(directive GownCommentDirective, commands []gownCommentCommand, typ ast.Expr) error {
	if typ == nil {
		return ctx.directiveError(directive, "Gown ownerstamp directive requires an explicit Go type")
	}
	for _, cmd := range commands {
		switch {
		case isCapWord(cmd.Name):
			if len(cmd.Args) != 0 {
				return ctx.directiveError(directive, fmt.Sprintf("%s directive does not take arguments", cmd.Name))
			}
			if err := ctx.insertDirectCap(directive, typ, capFromWord(cmd.Name)); err != nil {
				return err
			}
		case cmd.Name == "elem":
			if len(cmd.Args) != 1 || !isCapWord(cmd.Args[0]) {
				return ctx.directiveError(directive, "elem directive requires one ownerstamp")
			}
			ch, ok := typ.(*ast.ChanType)
			if !ok {
				return ctx.directiveError(directive, "elem directive requires a channel type")
			}
			if err := ctx.insertDirectCap(directive, ch.Value, capFromWord(cmd.Args[0])); err != nil {
				return err
			}
		default:
			return ctx.directiveError(directive, fmt.Sprintf("unsupported type directive %q", cmd.Name))
		}
	}
	return nil
}

func (ctx *commentLoweringContext) applyFuncSignatureCommands(directive GownCommentDirective, commands []gownCommentCommand, params, results *ast.FieldList) error {
	for _, cmd := range commands {
		switch cmd.Name {
		case "param":
			if len(cmd.Args) != 2 || !isCapWord(cmd.Args[1]) {
				return ctx.directiveError(directive, "param directive must be: param NAME OWNERSTAMP")
			}
			field, err := fieldListFieldByName(params, cmd.Args[0])
			if err != nil {
				return ctx.directiveError(directive, err.Error())
			}
			if err := ctx.insertDirectCap(directive, field.Type, capFromWord(cmd.Args[1])); err != nil {
				return err
			}
		case "result":
			if len(cmd.Args) != 2 || !isCapWord(cmd.Args[1]) {
				return ctx.directiveError(directive, "result directive must be: result INDEX OWNERSTAMP")
			}
			index, err := strconv.Atoi(cmd.Args[0])
			if err != nil || index < 0 {
				return ctx.directiveError(directive, "result directive requires a zero-based result index")
			}
			field, err := fieldListFieldByIndex(results, index)
			if err != nil {
				return ctx.directiveError(directive, err.Error())
			}
			if err := ctx.insertDirectCap(directive, field.Type, capFromWord(cmd.Args[1])); err != nil {
				return err
			}
		default:
			return ctx.directiveError(directive, fmt.Sprintf("unsupported function directive %q", cmd.Name))
		}
	}
	return nil
}

func (ctx *commentLoweringContext) applyRestoreDirective(directive GownCommentDirective, commands []gownCommentCommand) error {
	assign := ctx.assignStartingOnLine(directive.Span.Line + 1)
	if assign == nil {
		return ctx.directiveError(directive, "restore directive must immediately precede an assignment")
	}
	if len(assign.Rhs) != 1 {
		return ctx.directiveError(directive, "restore assignment must have exactly one right-hand side")
	}
	call, ok := unparenExpr(assign.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return ctx.directiveError(directive, "restore assignment right-hand side must be an IIFE call")
	}
	fn, ok := unparenExpr(call.Fun).(*ast.FuncLit)
	if !ok || fn.Type == nil {
		return ctx.directiveError(directive, "restore assignment must call a func literal")
	}
	ctx.insertAt(ctx.offset(fn.Type.Func), `\restore `, "lower gown restore comment")
	for _, cmd := range commands {
		switch cmd.Name {
		case "restore":
			if len(cmd.Args) != 0 {
				return ctx.directiveError(directive, "restore directive does not take arguments")
			}
		case "param":
			if len(cmd.Args) != 2 || !isCapWord(cmd.Args[1]) {
				return ctx.directiveError(directive, "restore param directive must be: param NAME OWNERSTAMP")
			}
			field, err := fieldListFieldByName(fn.Type.Params, cmd.Args[0])
			if err != nil {
				return ctx.directiveError(directive, err.Error())
			}
			if err := ctx.insertDirectCap(directive, field.Type, capFromWord(cmd.Args[1])); err != nil {
				return err
			}
		case "result":
			if len(cmd.Args) != 2 || !isCapWord(cmd.Args[1]) {
				return ctx.directiveError(directive, "restore result directive must be: result INDEX OWNERSTAMP")
			}
			index, err := strconv.Atoi(cmd.Args[0])
			if err != nil || index < 0 {
				return ctx.directiveError(directive, "restore result directive requires a zero-based result index")
			}
			field, err := fieldListFieldByIndex(fn.Type.Results, index)
			if err != nil {
				return ctx.directiveError(directive, err.Error())
			}
			if err := ctx.insertDirectCap(directive, field.Type, capFromWord(cmd.Args[1])); err != nil {
				return err
			}
		default:
			return ctx.directiveError(directive, fmt.Sprintf("unsupported restore directive %q", cmd.Name))
		}
	}
	return nil
}

func (ctx *commentLoweringContext) lowerNewDirective(directive GownCommentDirective, assign *ast.AssignStmt) error {
	rhs, err := singleRHS(assign)
	if err != nil {
		return ctx.directiveError(directive, err.Error())
	}
	unary, ok := unparenExpr(rhs).(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return ctx.directiveError(directive, "new directive requires a right-hand side of the form &T{...}")
	}
	lit, ok := unparenExpr(unary.X).(*ast.CompositeLit)
	if !ok {
		return ctx.directiveError(directive, "new directive requires a right-hand side of the form &T{...}")
	}
	start, end := ctx.nodeOffsets(rhs)
	litStart, litEnd := ctx.nodeOffsets(lit)
	if !validRange(ctx.src, litStart, litEnd) {
		return ctx.directiveError(directive, "invalid composite literal range for new directive")
	}
	replacement := append([]byte(`\new(`), ctx.src[litStart:litEnd]...)
	replacement = append(replacement, ')')
	ctx.replace(start, end, replacement, "lower gown new comment")
	return nil
}

func (ctx *commentLoweringContext) wrapSingleRHS(directive GownCommentDirective, assign *ast.AssignStmt, intrinsic string) error {
	rhs, err := singleRHS(assign)
	if err != nil {
		return ctx.directiveError(directive, err.Error())
	}
	start, end := ctx.nodeOffsets(rhs)
	if !validRange(ctx.src, start, end) {
		return ctx.directiveError(directive, "invalid right-hand side range")
	}
	replacement := append([]byte(intrinsic+"("), ctx.src[start:end]...)
	replacement = append(replacement, ')')
	ctx.replace(start, end, replacement, "lower gown intrinsic comment")
	return nil
}

func (ctx *commentLoweringContext) lowerCloneDirective(directive GownCommentDirective, assign *ast.AssignStmt, name, receiver string) error {
	rhs, err := singleRHS(assign)
	if err != nil {
		return ctx.directiveError(directive, err.Error())
	}
	call, ok := unparenExpr(rhs).(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return ctx.directiveError(directive, fmt.Sprintf("%s directive requires a zero-argument method call", name))
	}
	sel, ok := unparenExpr(call.Fun).(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != cloneIntrinsicMethodName(commentCloneIntrinsicKind(name)) {
		return ctx.directiveError(directive, fmt.Sprintf("%s directive must mark a matching method call", name))
	}
	if string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(sel.X))) != receiver {
		return ctx.directiveError(directive, fmt.Sprintf("%s directive receiver does not match %q", name, receiver))
	}
	start, end := ctx.nodeOffsets(rhs)
	replacement := []byte(`\` + name + `(` + receiver + `)`)
	ctx.replace(start, end, replacement, "lower gown clone comment")
	return nil
}

func (ctx *commentLoweringContext) lowerSwapDirective(directive GownCommentDirective, assign *ast.AssignStmt) error {
	if len(assign.Lhs) != 2 || len(assign.Rhs) != 2 {
		return ctx.directiveError(directive, "swap directive requires a two-place assignment")
	}
	left0 := strings.TrimSpace(string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(assign.Lhs[0]))))
	left1 := strings.TrimSpace(string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(assign.Lhs[1]))))
	right0 := strings.TrimSpace(string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(assign.Rhs[0]))))
	right1 := strings.TrimSpace(string(nodeTextFromSrc(ctx.src, ctx.nodeOffsetsPair(assign.Rhs[1]))))
	if left0 == "" || left1 == "" || right0 != left1 || right1 != left0 {
		return ctx.directiveError(directive, "swap directive requires assignment shape a, b = b, a")
	}
	start, end := ctx.nodeOffsets(assign)
	ctx.replace(start, end, []byte(`\swap(`+left0+`, `+left1+`)`), "lower gown swap comment")
	return nil
}

func (ctx *commentLoweringContext) insertDirectCap(directive GownCommentDirective, typ ast.Expr, cap Cap) error {
	if typ == nil || cap == CapInvalid {
		return ctx.directiveError(directive, "invalid ownerstamp insertion target")
	}
	ctx.insertAt(ctx.offset(typ.Pos()), cap.String()+" ", "lower gown ownerstamp comment")
	return nil
}

func fieldListFieldByName(fields *ast.FieldList, name string) (*ast.Field, error) {
	if fields == nil {
		return nil, fmt.Errorf("no parameter list available for %q", name)
	}
	for _, field := range fields.List {
		for _, fieldName := range field.Names {
			if fieldName.Name == name {
				if len(field.Names) > 1 {
					return nil, fmt.Errorf("cannot ownerstamp %q in a multi-name field; split the declaration first", name)
				}
				return field, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find parameter %q", name)
}

func fieldListFieldByIndex(fields *ast.FieldList, index int) (*ast.Field, error) {
	if fields == nil {
		return nil, fmt.Errorf("no result list available")
	}
	tupleIndex := 0
	for _, field := range fields.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			if tupleIndex == index {
				if n > 1 {
					return nil, fmt.Errorf("cannot ownerstamp result %d in a multi-name result field; split the declaration first", index)
				}
				return field, nil
			}
			tupleIndex++
		}
	}
	return nil, fmt.Errorf("could not find result %d", index)
}

func singleRHS(assign *ast.AssignStmt) (ast.Expr, error) {
	if len(assign.Rhs) != 1 {
		return nil, fmt.Errorf("directive requires exactly one right-hand side")
	}
	return assign.Rhs[0], nil
}

func containsCommand(commands []gownCommentCommand, name string) bool {
	for _, cmd := range commands {
		if cmd.Name == name {
			return true
		}
	}
	return false
}

func capFromWord(word string) Cap {
	switch word {
	case "iso":
		return CapIso
	case "mub":
		return CapMub
	case "rob":
		return CapRob
	case "imm":
		return CapImm
	default:
		return CapInvalid
	}
}

func isCapWord(word string) bool {
	return capFromWord(word) != CapInvalid
}

func commentCloneIntrinsicKind(name string) IntrinsicKind {
	if name == "Clone" {
		return IntrinsicCloneExported
	}
	return IntrinsicClone
}

func (ctx *commentLoweringContext) isTrailingDirective(directive GownCommentDirective) bool {
	lineStart, _ := sourceLineRange(ctx.src, directive.Span.Offset)
	return !linePrefixWhitespace(ctx.src[lineStart:directive.Span.Offset])
}

func (ctx *commentLoweringContext) fieldEndingOnLine(line int) *ast.Field {
	var out *ast.Field
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || ctx.line(field.End()) != line {
			return true
		}
		if out == nil || ctx.offset(field.Pos()) > ctx.offset(out.Pos()) {
			out = field
		}
		return true
	})
	return out
}

func (ctx *commentLoweringContext) fieldStartingOnLine(line int) *ast.Field {
	var out *ast.Field
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if !ok || ctx.line(field.Pos()) != line {
			return true
		}
		if out == nil || ctx.offset(field.Pos()) < ctx.offset(out.Pos()) {
			out = field
		}
		return true
	})
	return out
}

func (ctx *commentLoweringContext) valueSpecEndingOnLine(line int) *ast.ValueSpec {
	var out *ast.ValueSpec
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || ctx.line(spec.End()) != line {
			return true
		}
		if out == nil || ctx.offset(spec.Pos()) > ctx.offset(out.Pos()) {
			out = spec
		}
		return true
	})
	return out
}

func (ctx *commentLoweringContext) valueSpecStartingOnLine(line int) *ast.ValueSpec {
	var out *ast.ValueSpec
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || ctx.line(spec.Pos()) != line {
			return true
		}
		if out == nil || ctx.offset(spec.Pos()) < ctx.offset(out.Pos()) {
			out = spec
		}
		return true
	})
	return out
}

func (ctx *commentLoweringContext) assignEndingOnLine(line int) *ast.AssignStmt {
	var out *ast.AssignStmt
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || ctx.line(assign.End()) != line {
			return true
		}
		if out == nil || ctx.offset(assign.Pos()) > ctx.offset(out.Pos()) {
			out = assign
		}
		return true
	})
	return out
}

func (ctx *commentLoweringContext) assignStartingOnLine(line int) *ast.AssignStmt {
	var out *ast.AssignStmt
	ast.Inspect(ctx.file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || ctx.line(assign.Pos()) != line {
			return true
		}
		if out == nil || ctx.offset(assign.Pos()) < ctx.offset(out.Pos()) {
			out = assign
		}
		return true
	})
	return out
}

func (ctx *commentLoweringContext) funcStartingOnLine(line int) *ast.FuncDecl {
	for _, decl := range ctx.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && ctx.line(fn.Pos()) == line {
			return fn
		}
	}
	return nil
}

func (ctx *commentLoweringContext) insertAt(offset int, text string, reason string) {
	ctx.edits = append(ctx.edits, EmitEdit{
		Start:   offset,
		End:     offset,
		NewText: []byte(text),
		Reason:  reason,
	})
}

func (ctx *commentLoweringContext) replace(start, end int, text []byte, reason string) {
	ctx.edits = append(ctx.edits, EmitEdit{
		Start:   start,
		End:     end,
		NewText: text,
		Reason:  reason,
	})
}

func (ctx *commentLoweringContext) nodeOffsets(node ast.Node) (int, int) {
	return ctx.offset(node.Pos()), ctx.offset(node.End())
}

func (ctx *commentLoweringContext) nodeOffsetsPair(node ast.Node) [2]int {
	start, end := ctx.nodeOffsets(node)
	return [2]int{start, end}
}

func (ctx *commentLoweringContext) offset(pos token.Pos) int {
	return ctx.fset.Position(pos).Offset
}

func (ctx *commentLoweringContext) line(pos token.Pos) int {
	return ctx.fset.Position(pos).Line
}

func (ctx *commentLoweringContext) directiveError(directive GownCommentDirective, msg string) error {
	return scanError(ctx.path, directive.Span, msg)
}

func nodeTextFromSrc(src []byte, span [2]int) []byte {
	if !validRange(src, span[0], span[1]) {
		return nil
	}
	return src[span[0]:span[1]]
}

func sortLoweringResults(results []CommentLoweringResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})
}
