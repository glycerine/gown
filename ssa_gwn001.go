package gown

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkGWN001SSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	places := buildSSAPlaceIndex(pkg, ssaPkg, caps)
	checker := &ssaGWN001Checker{
		pkg:          pkg,
		caps:         caps,
		places:       places,
		assigns:      collectSSAAssignmentMoves(pkg, caps),
		namedBorrows: collectSSANamedBorrows(pkg, caps),
		sendBindings: sendBindingsByPosition(caps),
		callBindings: callBindingsByPosition(caps),
		reported:     make(map[string]bool),
	}
	for _, fn := range collectSSAFunctions(ssaPkg) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaGWN001Checker struct {
	pkg                 *packages.Package
	caps                *CapabilityIndex
	places              *SSAPlaceIndex
	assigns             map[ast.Expr]ssaAssignmentMove
	namedBorrows        map[*types.Func]SSANamedBorrowInfo
	sendBindings        map[sourcePosKey]SendBinding
	callBindings        map[sourcePosKey]CallBinding
	activeNamedBorrows  SSANamedBorrowInfo
	namedBorrowLiveness *SSANamedBorrowLiveness
	errs                CheckerErrors
	reported            map[string]bool
}

type ssaAssignmentMove struct {
	Src Place
	Dst Place
}

func (checker *ssaGWN001Checker) checkFunction(fn *ssa.Function) {
	if fn == nil || len(fn.Blocks) == 0 {
		return
	}
	checker.beginFunction(fn)
	defer checker.endFunction()

	in := make(map[*ssa.BasicBlock]SSAFunctionState)
	queued := make(map[*ssa.BasicBlock]bool)
	in[fn.Blocks[0]] = NewSSAFunctionState()
	worklist := []*ssa.BasicBlock{fn.Blocks[0]}
	queued[fn.Blocks[0]] = true

	for len(worklist) > 0 {
		block := worklist[0]
		worklist = worklist[1:]
		queued[block] = false

		state := in[block].Clone()
		for _, instr := range block.Instrs {
			checker.checkInstructionUses(instr, &state)
			checker.applyInstructionTransfer(instr, &state)
		}

		for _, succ := range block.Succs {
			next, changed := checker.mergeSuccessorState(in[succ], state, succ)
			if !changed {
				continue
			}
			in[succ] = next
			if !queued[succ] {
				worklist = append(worklist, succ)
				queued[succ] = true
			}
		}
	}
}

func (checker *ssaGWN001Checker) beginFunction(fn *ssa.Function) {
	checker.activeNamedBorrows = checker.namedBorrowInfoForFunction(fn)
	checker.namedBorrowLiveness = buildSSANamedBorrowLiveness(fn, checker.caps, checker.activeNamedBorrows)
}

func (checker *ssaGWN001Checker) endFunction() {
	checker.activeNamedBorrows = SSANamedBorrowInfo{}
	checker.namedBorrowLiveness = nil
}

func (checker *ssaGWN001Checker) namedBorrowInfoForFunction(fn *ssa.Function) SSANamedBorrowInfo {
	if fn == nil {
		return SSANamedBorrowInfo{}
	}
	fnObj, _ := fn.Object().(*types.Func)
	if fnObj == nil {
		return SSANamedBorrowInfo{}
	}
	return checker.namedBorrows[fnObj]
}

func (checker *ssaGWN001Checker) mergeSuccessorState(existing, incoming SSAFunctionState, block *ssa.BasicBlock) (SSAFunctionState, bool) {
	if existing.Consumed == nil && len(existing.Borrows) == 0 {
		return incoming.Clone(), true
	}
	merged, violations := MergeSSAFunctionStates(existing, incoming)
	for _, violation := range violations {
		checker.reportViolation(blockPosition(checker.pkg, block), violation)
	}
	return merged, !equalSSAFunctionState(existing, merged)
}

func (checker *ssaGWN001Checker) checkInstructionUses(instr ssa.Instruction, state *SSAFunctionState) {
	if debug, ok := instr.(*ssa.DebugRef); ok {
		checker.checkDebugRefUse(debug, state)
	}
}

func (checker *ssaGWN001Checker) checkDebugRefUse(debug *ssa.DebugRef, state *SSAFunctionState) {
	if debug == nil || debug.IsAddr {
		return
	}
	place, ok := checker.caps.PlaceForExpr(debug.Expr)
	if !ok || place.Root == nil {
		return
	}
	site, moved := state.CheckUse(place.Key())
	if !moved {
		return
	}
	checker.reportUseAfterMove(debug, place, site)
}

func (checker *ssaGWN001Checker) applyInstructionTransfer(instr ssa.Instruction, state *SSAFunctionState) {
	switch instr := instr.(type) {
	case *ssa.Send:
		checker.applySendTransfer(instr, state)
	case *ssa.Call:
		checker.applyCallTransfer(instr, state)
	case *ssa.DebugRef:
		checker.applyAssignmentMove(instr, state)
	case *ssa.Go:
		checker.applyGoTransfer(instr, state)
	}
}

func (checker *ssaGWN001Checker) applyAssignmentMove(instr *ssa.DebugRef, state *SSAFunctionState) {
	if instr == nil {
		return
	}
	move, ok := checker.assigns[debugRefExprKey(instr.Expr)]
	if !ok {
		return
	}
	if move.Dst.Root != nil {
		state.UnconsumeRoot(move.Dst.Key())
	}
	pos := checker.pkg.Fset.Position(instr.Pos())
	violation, ok := state.ConsumeRoot(move.Src.Key(), SSAMoveSite{
		Name: move.Src.Root.Name(),
		Kind: "assignment",
		Line: sourceLine(pos),
		Col:  sourceColumn(pos),
	})
	if !ok {
		return
	}
	checker.reportViolation(pos, violation)
}

func (checker *ssaGWN001Checker) applySendTransfer(instr *ssa.Send, state *SSAFunctionState) {
	if binding, ok := ssaSendBinding(checker.pkg, checker.sendBindings, instr); ok {
		if !binding.IsIsoMove() {
			return
		}
		checker.consumeRootAtInstruction(state, binding.Value, instr, "send")
		return
	}
	chPlace, ok := checker.places.PlaceForValue(instr.Chan)
	if !ok || chPlace.Root == nil || checker.caps.ChanElemCap(chPlace.Root) != CapIso {
		return
	}
	valuePlace, ok := checker.places.PlaceForValue(instr.X)
	if !ok || valuePlace.Root == nil || checker.capForPlace(valuePlace) != CapIso {
		return
	}
	checker.consumeRootAtInstruction(state, valuePlace, instr, "send")
}

func (checker *ssaGWN001Checker) applyCallTransfer(instr *ssa.Call, state *SSAFunctionState) {
	if binding, ok := ssaCallBinding(checker.pkg, checker.callBindings, instr); ok {
		checker.applyBoundCallTransfer(binding, state, instr, "call")
		return
	}
	checker.applyCallCommonTransfer(&instr.Call, state, instr, "call")
}

func (checker *ssaGWN001Checker) applyBoundCallTransfer(binding CallBinding, state *SSAFunctionState, instr ssa.Instruction, kind string) {
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapIso || i >= len(binding.ArgPlaces) {
			continue
		}
		argPlace := binding.ArgPlaces[i]
		if argPlace.Root == nil || checker.capForPlace(argPlace) != CapIso {
			continue
		}
		checker.consumeRootAtInstruction(state, argPlace, instr, kind)
	}
}

func (checker *ssaGWN001Checker) applyGoTransfer(instr *ssa.Go, state *SSAFunctionState) {
	if binding, ok := ssaGoCallBinding(checker.pkg, checker.callBindings, instr); ok {
		checker.applyBoundCallTransfer(binding, state, instr, "go")
	} else {
		checker.applyCallCommonTransfer(&instr.Call, state, instr, "go")
	}
	checker.applyGoClosureCaptureTransfer(instr, state)
}

func (checker *ssaGWN001Checker) applyCallCommonTransfer(call *ssa.CallCommon, state *SSAFunctionState, instr ssa.Instruction, kind string) {
	callee := call.StaticCallee()
	if callee == nil {
		return
	}
	fn, _ := callee.Object().(*types.Func)
	funcCap := checker.caps.FuncCap(fn)
	if funcCap == nil {
		return
	}
	for i, paramCap := range funcCap.Params {
		if paramCap != CapIso || i >= len(call.Args) {
			continue
		}
		argPlace, ok := checker.places.PlaceForValue(call.Args[i])
		if !ok || argPlace.Root == nil || checker.capForPlace(argPlace) != CapIso {
			continue
		}
		checker.consumeRootAtInstruction(state, argPlace, instr, kind)
	}
}

func (checker *ssaGWN001Checker) applyGoClosureCaptureTransfer(instr *ssa.Go, state *SSAFunctionState) {
	closure, _ := instr.Call.Value.(*ssa.MakeClosure)
	if closure == nil {
		return
	}
	for _, binding := range closure.Bindings {
		place, ok := checker.places.PlaceForValue(binding)
		if !ok || place.Root == nil || checker.capForPlace(place) != CapIso {
			continue
		}
		checker.consumeRootAtInstruction(state, place, instr, "go")
	}
}

func (checker *ssaGWN001Checker) consumeRootAtInstruction(state *SSAFunctionState, place Place, instr ssa.Instruction, kind string) {
	pos := checker.pkg.Fset.Position(instr.Pos())
	key := place.Key()
	site := SSAMoveSite{
		Name: place.Root.Name(),
		Kind: kind,
		Line: sourceLine(pos),
		Col:  sourceColumn(pos),
	}
	if key.Path != "" {
		violation, ok := state.ConsumeRoot(key, site)
		if ok {
			checker.reportViolation(pos, violation)
		}
		return
	}
	if violation, ok := checker.namedBorrowMoveViolation(key, instr); ok {
		checker.reportViolation(pos, violation)
		return
	}
	violation, ok := state.ConsumeRoot(key, site)
	if !ok {
		return
	}
	checker.reportViolation(pos, violation)
}

func (checker *ssaGWN001Checker) namedBorrowMoveViolation(moved PlaceKey, instr ssa.Instruction) (SSAStateViolation, bool) {
	for obj := range checker.namedBorrowLiveness.LiveAfterInstruction(instr) {
		borrow, ok := checker.activeNamedBorrows.Borrows[obj]
		if !ok || !borrow.Source.Key().Overlaps(moved) {
			continue
		}
		return SSAStateViolation{
			Code:    GWN002,
			Place:   moved,
			Message: "cannot move root while named borrow is live",
		}, true
	}
	return SSAStateViolation{}, false
}

func (checker *ssaGWN001Checker) capForPlace(place Place) Cap {
	return capForSSAPlace(checker.caps, place)
}

func (checker *ssaGWN001Checker) reportUseAfterMove(instr ssa.Instruction, place Place, site SSAMoveSite) {
	name := "<unknown>"
	if place.Root != nil {
		name = place.Root.Name()
	}
	message := fmt.Sprintf("use of moved \\iso value %q after %s at %d:%d",
		name, site.Kind, site.Line, site.Col)
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN001,
		checker.pkg.Fset.Position(instr.Pos()),
		message,
	))
}

func (checker *ssaGWN001Checker) reportViolation(pos token.Position, violation SSAStateViolation) {
	code := violation.Code
	message := violation.Message
	if code == GWN011 {
		message = "cannot move field projection"
	}
	checker.reportCheckerError(newCheckerErrorAtPosition(code, pos, message))
}

func (checker *ssaGWN001Checker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}

func blockPosition(pkg *packages.Package, block *ssa.BasicBlock) token.Position {
	if pkg == nil || block == nil {
		return token.Position{}
	}
	for _, instr := range block.Instrs {
		if instr.Pos().IsValid() {
			return pkg.Fset.Position(instr.Pos())
		}
	}
	return token.Position{}
}

func collectSSAAssignmentMoves(pkg *packages.Package, caps *CapabilityIndex) map[ast.Expr]ssaAssignmentMove {
	moves := make(map[ast.Expr]ssaAssignmentMove)
	if pkg == nil || caps == nil {
		return moves
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			if len(assign.Lhs) != len(assign.Rhs) {
				return true
			}
			for i, rhs := range assign.Rhs {
				src, ok := caps.PlaceForExpr(rhs)
				if !ok || src.Root == nil || capForSSAPlace(caps, src) != CapIso {
					continue
				}
				dst, ok := caps.PlaceForExpr(assign.Lhs[i])
				if !ok || dst.Root == nil || dst.Root == src.Root || capForSSAPlace(caps, dst) != CapIso {
					continue
				}
				moves[debugRefExprKey(rhs)] = ssaAssignmentMove{Src: src, Dst: dst}
			}
			return true
		})
	}
	return moves
}

func capForSSAPlace(caps *CapabilityIndex, place Place) Cap {
	if caps == nil || place.Root == nil {
		return CapInvalid
	}
	return caps.ObjectCap(capObjectForSSAPlace(caps, place))
}

func debugRefExprKey(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}
