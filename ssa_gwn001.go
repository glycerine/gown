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
	assignments := collectSSAAssignments(pkg, caps)
	flowAssignments := collectSSAFlowAssignments(pkg, caps)
	checker := &ssaGWN001Checker{
		pkg:                     pkg,
		caps:                    caps,
		places:                  places,
		assignments:             assignments,
		flowAssignments:         flowAssignments,
		immutableProjectionUses: collectSSAImmutableProjectionUseSpans(pkg, caps),
		namedBorrows:            collectSSANamedBorrows(pkg, caps),
		deferEffects:            collectSSADeferredClosureEffects(pkg, caps),
		returns:                 collectSSAReturnPlaces(pkg, caps),
		returnValues:            collectSSAReturnValues(pkg, caps),
		bindings:                NewSSABindingIndex(caps),
		reported:                make(map[string]bool),
	}
	for _, fn := range collectSSAFunctions(ssaPkg) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaGWN001Checker struct {
	pkg                     *packages.Package
	caps                    *CapabilityIndex
	places                  *SSAPlaceIndex
	assignments             map[ast.Expr]ssaAssignment
	flowAssignments         map[ast.Expr]ssaFlowAssignment
	immutableProjectionUses []ssaImmutableProjectionUseSpan
	namedBorrows            map[*types.Func]SSANamedBorrowInfo
	deferEffects            map[*types.Func]SSADeferredClosureEffectInfo
	returns                 map[sourcePosKey][]Place
	returnValues            map[sourcePosKey][]ValueCapability
	bindings                *SSABindingIndex
	activeNamedBorrows      SSANamedBorrowInfo
	activeDeferEffects      SSADeferredClosureEffectInfo
	namedBorrowLiveness     *SSANamedBorrowLiveness
	errs                    CheckerErrors
	reported                map[string]bool
}

type ssaAssignment struct {
	Dst   Place
	Value ValueCapability
	RHS   ast.Expr
}

type ssaFlowAssignment struct {
	Dst   Place
	Value ValueCapability
}

type ssaImmutableProjectionUseSpan struct {
	Root  types.Object
	Start token.Pos
	End   token.Pos
}

func (checker *ssaGWN001Checker) checkFunction(fn *ssa.Function) {
	if fn == nil || len(fn.Blocks) == 0 {
		return
	}
	checker.beginFunction(fn)
	defer checker.endFunction()

	checker.runFunctionBody(fn, NewSSAFunctionState())
}

func (checker *ssaGWN001Checker) runFunctionBody(fn *ssa.Function, initial SSAFunctionState) (SSAFunctionState, bool) {
	in := make(map[*ssa.BasicBlock]SSAFunctionState)
	queued := make(map[*ssa.BasicBlock]bool)
	in[fn.Blocks[0]] = initial.Clone()
	worklist := []*ssa.BasicBlock{fn.Blocks[0]}
	queued[fn.Blocks[0]] = true
	var exit SSAFunctionState
	haveExit := false

	for len(worklist) > 0 {
		block := worklist[0]
		worklist = worklist[1:]
		queued[block] = false

		state := in[block].Clone()
		returned := false
		for _, instr := range block.Instrs {
			checker.checkInstructionUses(instr, &state)
			checker.applyInstructionTransfer(instr, &state)
			if _, ok := instr.(*ssa.Return); ok {
				returned = true
			}
		}
		if returned {
			if !haveExit {
				exit = state.Clone()
				haveExit = true
			} else {
				merged, violations := MergeSSAFunctionStates(exit, state)
				for _, violation := range violations {
					checker.reportViolation(blockPosition(checker.pkg, block), violation)
				}
				exit = merged
			}
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
	return exit, haveExit
}

func (checker *ssaGWN001Checker) beginFunction(fn *ssa.Function) {
	checker.activeNamedBorrows = checker.namedBorrowInfoForFunction(fn)
	checker.activeDeferEffects = checker.deferEffectInfoForFunction(fn)
	checker.namedBorrowLiveness = buildSSANamedBorrowLiveness(fn, checker.caps, checker.activeNamedBorrows)
}

func (checker *ssaGWN001Checker) endFunction() {
	checker.activeNamedBorrows = SSANamedBorrowInfo{}
	checker.activeDeferEffects = SSADeferredClosureEffectInfo{}
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

func (checker *ssaGWN001Checker) deferEffectInfoForFunction(fn *ssa.Function) SSADeferredClosureEffectInfo {
	if fn == nil {
		return SSADeferredClosureEffectInfo{}
	}
	fnObj, _ := fn.Object().(*types.Func)
	if fnObj == nil {
		return SSADeferredClosureEffectInfo{}
	}
	return checker.deferEffects[fnObj]
}

func (checker *ssaGWN001Checker) mergeSuccessorState(existing, incoming SSAFunctionState, block *ssa.BasicBlock) (SSAFunctionState, bool) {
	if existing.Consumed == nil && len(existing.Frontiered) == 0 && len(existing.FlowValues) == 0 && len(existing.Borrows) == 0 && len(existing.Deferred) == 0 {
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
		return
	}
	checker.checkInstructionOperandUses(instr, state)
}

func (checker *ssaGWN001Checker) checkDebugRefUse(debug *ssa.DebugRef, state *SSAFunctionState) {
	if debug == nil || debug.IsAddr {
		return
	}
	if checker.isAssignmentTargetDebugRef(debug) {
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
	if checker.allowsMovedRebindUse(debug.Pos(), place, state) {
		return
	}
	checker.reportUseAfterMoveAtPosition(checker.debugRefUsePosition(debug), place, site)
}

func (checker *ssaGWN001Checker) isAssignmentTargetDebugRef(debug *ssa.DebugRef) bool {
	if checker == nil || debug == nil {
		return false
	}
	key := debugRefExprKey(debug.Expr)
	if _, ok := checker.assignments[key]; ok {
		return true
	}
	_, ok := checker.flowAssignments[key]
	return ok
}

func (checker *ssaGWN001Checker) allowsMovedRebindUse(pos token.Pos, place Place, state *SSAFunctionState) bool {
	if checker == nil || state == nil || place.Root == nil {
		return false
	}
	if place.Key().Path != "" && capForSSAPlace(checker.caps, place) == CapImm {
		return true
	}
	if !pos.IsValid() {
		return false
	}
	for _, span := range checker.immutableProjectionUses {
		if span.Root != place.Root || pos < span.Start || pos >= span.End {
			continue
		}
		_, moved := state.CheckUse(PlaceKey{Root: span.Root})
		return moved
	}
	return false
}

func (checker *ssaGWN001Checker) checkInstructionOperandUses(instr ssa.Instruction, state *SSAFunctionState) {
	if instr == nil || state == nil {
		return
	}
	if instructionOperandsHaveSourceDebugRefs(instr) {
		return
	}
	for _, operand := range instr.Operands(nil) {
		if operand == nil || *operand == nil {
			continue
		}
		value := *operand
		if checker.operandIsDefinition(instr, value) {
			continue
		}
		if checker.places.ValuePlaceAmbiguous(value) {
			continue
		}
		place, ok := checker.places.PlaceForValue(value)
		if !ok || place.Root == nil {
			continue
		}
		site, moved := state.CheckUse(place.Key())
		if !moved {
			continue
		}
		if checker.allowsMovedRebindUse(instr.Pos(), place, state) {
			continue
		}
		checker.reportOperandUseAfterMove(instr, value, place, site)
	}
}

func instructionOperandsHaveSourceDebugRefs(instr ssa.Instruction) bool {
	switch instr.(type) {
	case *ssa.Call, *ssa.Defer, *ssa.Go, *ssa.Phi, *ssa.Return, *ssa.Select, *ssa.Send:
		return true
	default:
		return false
	}
}

func (checker *ssaGWN001Checker) operandIsDefinition(instr ssa.Instruction, value ssa.Value) bool {
	store, ok := instr.(*ssa.Store)
	if !ok || value != store.Addr {
		return false
	}
	place, ok := checker.places.PlaceForValue(store.Addr)
	if !ok || place.Root == nil || place.Key().Path != "" || place.Collapsed {
		return false
	}
	ptr, ok := store.Addr.Type().Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	return types.Identical(ptr.Elem(), place.Root.Type())
}

func (checker *ssaGWN001Checker) applyInstructionTransfer(instr ssa.Instruction, state *SSAFunctionState) {
	switch instr := instr.(type) {
	case *ssa.Send:
		checker.applySendTransfer(instr, state)
	case *ssa.Call:
		checker.applyCallTransfer(instr, state)
	case *ssa.Defer:
		checker.applyDeferTransfer(instr, state)
	case *ssa.DebugRef:
		checker.applyFlowAssignmentTransfer(instr, state)
		checker.applyAssignmentTransfer(instr, state)
	case *ssa.Go:
		checker.applyGoTransfer(instr, state)
	case *ssa.MakeInterface:
		checker.applyInterfaceFrontier(instr, state)
	case *ssa.Return:
		checker.applyReturnTransfer(instr, state)
	case *ssa.Select:
		checker.applySelectTransfer(instr, state)
	case *ssa.Store:
		checker.applyStoreFrontierTransfer(instr, state)
	case *ssa.MapUpdate:
		checker.applyMapUpdateFrontierTransfer(instr, state)
	}
}

func (checker *ssaGWN001Checker) applyStoreFrontierTransfer(instr *ssa.Store, state *SSAFunctionState) {
	if instr == nil {
		return
	}
	value, ok := checker.places.PlaceForValue(instr.Val)
	if !ok || value.Root == nil || !placeCanTransferAsIso(checker.caps, value) {
		return
	}
	target, ok := checker.places.PlaceForValue(instr.Addr)
	if !ok || target.Root == nil || !ssaStoreTargetEscapes(checker.pkg, target) {
		return
	}
	checker.enterFrontierAtInstruction(state, value, instr, "store")
}

func (checker *ssaGWN001Checker) applyMapUpdateFrontierTransfer(instr *ssa.MapUpdate, state *SSAFunctionState) {
	if instr == nil {
		return
	}
	value, ok := checker.places.PlaceForValue(instr.Value)
	if !ok || value.Root == nil || !placeCanTransferAsIso(checker.caps, value) {
		return
	}
	checker.enterFrontierAtInstruction(state, value, instr, "map store")
}

func (checker *ssaGWN001Checker) applySelectTransfer(instr *ssa.Select, state *SSAFunctionState) {
	if instr == nil {
		return
	}
	seen := make(map[PlaceKey]bool)
	for _, selectState := range instr.States {
		if selectState == nil || selectState.Dir != types.SendOnly {
			continue
		}
		chPlace, ok := checker.places.PlaceForValue(selectState.Chan)
		if !ok || chPlace.Root == nil {
			continue
		}
		chCap := chanElemCapForPlace(checker.caps, chPlace)
		if chCap != CapIso && chCap != CapImm {
			continue
		}
		valuePlace, ok := checker.places.PlaceForValue(selectState.Send)
		if !ok || valuePlace.Root == nil || !placeCanTransferAsIso(checker.caps, valuePlace) {
			continue
		}
		key := valuePlace.Key()
		if seen[key] {
			continue
		}
		seen[key] = true
		kind := "select send"
		if chCap == CapImm {
			kind = "select freeze send"
		}
		checker.consumeRootAtInstruction(state, valuePlace, instr, kind)
	}
}

func (checker *ssaGWN001Checker) applyFlowAssignmentTransfer(instr *ssa.DebugRef, state *SSAFunctionState) {
	if instr == nil || state == nil {
		return
	}
	assignment, ok := checker.flowAssignments[debugRefExprKey(instr.Expr)]
	if !ok || assignment.Dst.Root == nil {
		return
	}
	dst := assignment.Dst.Key()
	state.UnconsumeRoot(dst)
	state.UnfrontierRoot(dst)
	state.ClearFlowValue(dst)
	if capTracked(assignment.Value.Cap) {
		state.SetFlowValue(dst, SSAFlowValue{Cap: assignment.Value.Cap})
	}
}

func (checker *ssaGWN001Checker) applyAssignmentTransfer(instr *ssa.DebugRef, state *SSAFunctionState) {
	if instr == nil {
		return
	}
	assignment, ok := checker.assignments[debugRefExprKey(instr.Expr)]
	if !ok {
		return
	}
	pos := checker.pkg.Fset.Position(instr.Pos())
	assignment = checker.assignmentWithFlowValue(assignment, state)
	if assignment.Value.Cap != CapIso {
		checker.reportAssignmentCapabilityMismatch(pos, assignment)
		return
	}
	if assignment.Value.Fresh {
		state.UnconsumeRoot(assignment.Dst.Key())
		state.UnfrontierRoot(assignment.Dst.Key())
		return
	}
	src := assignment.Value.Place
	if src.Root == nil || !checker.placeCanTransferAsIsoInState(state, src) {
		checker.reportAssignmentCapabilityMismatch(pos, assignment)
		return
	}
	if site, frontiered := state.CheckFrontier(src.Key()); frontiered {
		checker.reportFrontierViolation(pos, src, site, "assignment")
		return
	}
	if site, moved := state.CheckUse(src.Key()); moved {
		checker.reportUseAfterMoveAtPosition(pos, src, site)
		return
	}
	if violation, ok := checker.namedBorrowMoveViolation(src.Key(), instr); ok {
		checker.reportViolation(pos, violation)
		return
	}
	violation, ok := state.ConsumeRoot(src.Key(), SSAMoveSite{
		Name:   src.Root.Name(),
		Kind:   "assignment",
		Path:   gownSourcePath(pos.Filename),
		Offset: pos.Offset,
		Line:   sourceLine(pos),
		Col:    sourceColumn(pos),
	})
	if ok {
		checker.reportViolation(pos, violation)
		return
	}
	state.UnconsumeRoot(assignment.Dst.Key())
	state.UnfrontierRoot(assignment.Dst.Key())
}

func (checker *ssaGWN001Checker) assignmentWithFlowValue(assignment ssaAssignment, state *SSAFunctionState) ssaAssignment {
	if state == nil || capTracked(assignment.Value.Cap) || assignment.Value.Place.Root == nil {
		return assignment
	}
	if value, ok := state.FlowValue(assignment.Value.Place.Key()); ok {
		assignment.Value.Cap = value.Cap
		assignment.Value.Fresh = false
	}
	return assignment
}

func (checker *ssaGWN001Checker) placeCanTransferAsIsoInState(state *SSAFunctionState, place Place) bool {
	if placeCanTransferAsIso(checker.caps, place) {
		return true
	}
	if state == nil || place.Root == nil {
		return false
	}
	value, ok := state.FlowValue(place.Key())
	return ok && value.Cap == CapIso
}

func (checker *ssaGWN001Checker) reportAssignmentCapabilityMismatch(pos token.Position, assignment ssaAssignment) {
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN010,
		pos,
		fmt.Sprintf(
			"cannot assign %s value of type %s to \\iso root %q: %s",
			assignment.Value.Cap,
			assignmentValueTypeString(checker.pkg, assignment.RHS),
			assignment.Dst.Root.Name(),
			assignmentValueInvalidReason(checker.caps, assignment),
		),
	))
}

func (checker *ssaGWN001Checker) applySendTransfer(instr *ssa.Send, state *SSAFunctionState) {
	if binding, ok := checker.bindings.Send(checker.pkg, instr); ok {
		if binding.Value.Root == nil {
			return
		}
		if !binding.IsIsoConsumingTransfer() &&
			!(binding.ValueIso && (binding.ChanElemCap == CapIso || binding.ChanElemCap == CapImm)) {
			return
		}
		checker.consumeRootAtInstruction(state, binding.Value, instr, binding.TransferKind())
		return
	}
	chPlace, ok := checker.places.PlaceForValue(instr.Chan)
	if !ok || chPlace.Root == nil {
		return
	}
	valuePlace, ok := checker.places.PlaceForValue(instr.X)
	if !ok || valuePlace.Root == nil || !placeCanTransferAsIso(checker.caps, valuePlace) {
		return
	}
	chCap := chanElemCapForPlace(checker.caps, chPlace)
	if chCap != CapIso && chCap != CapImm {
		return
	}
	kind := "send"
	if chCap == CapImm {
		kind = "freeze send"
	}
	checker.consumeRootAtInstruction(state, valuePlace, instr, kind)
}

func (checker *ssaGWN001Checker) applyCallTransfer(instr *ssa.Call, state *SSAFunctionState) {
	if binding, ok := checker.bindings.Intrinsic(checker.pkg, instr); ok {
		checker.applyIntrinsicTransfer(binding, state, instr)
		return
	}
	if binding, ok := checker.bindings.Call(checker.pkg, instr); ok {
		checker.applyBoundCallTransfer(binding, state, instr, "call")
		return
	}
	if checker.applyUntrackedCallFrontier(&instr.Call, state, instr) {
		return
	}
	checker.applyCallCommonTransfer(&instr.Call, state, instr, "call")
}

func (checker *ssaGWN001Checker) applyIntrinsicTransfer(binding IntrinsicBinding, state *SSAFunctionState, instr ssa.Instruction) {
	switch binding.Kind {
	case IntrinsicMub:
		checker.checkExplicitBorrowIntrinsic(binding, state, instr, CapMub)
	case IntrinsicRob:
		checker.checkExplicitBorrowIntrinsic(binding, state, instr, CapRob)
	case IntrinsicFreeze:
		checker.applyExplicitFreezeIntrinsic(binding, state, instr)
	case IntrinsicUnsafe:
		checker.applyUnsafeIntrinsic(binding, state, instr)
	}
}

func (checker *ssaGWN001Checker) applyUnsafeIntrinsic(binding IntrinsicBinding, state *SSAFunctionState, instr ssa.Instruction) {
	place := binding.ArgPlace
	if place.Root == nil || !capTracked(checker.capForPlace(place)) {
		return
	}
	checker.enterFrontierAtInstruction(state, place, instr, "unsafe")
}

func (checker *ssaGWN001Checker) applyExplicitFreezeIntrinsic(binding IntrinsicBinding, state *SSAFunctionState, instr ssa.Instruction) {
	place := binding.ArgPlace
	if place.Root == nil {
		return
	}
	pos := checker.pkg.Fset.Position(instr.Pos())
	if !placeCanTransferAsIso(checker.caps, place) {
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN010,
			pos,
			fmt.Sprintf("cannot freeze %s value %q", checker.capForPlace(place), place.Root.Name()),
		))
		return
	}
	checker.consumeRootAtInstruction(state, place, instr, "freeze")
}

func (checker *ssaGWN001Checker) checkExplicitBorrowIntrinsic(binding IntrinsicBinding, state *SSAFunctionState, instr ssa.Instruction, borrowCap Cap) {
	place := binding.ArgPlace
	if place.Root == nil {
		return
	}
	pos := checker.pkg.Fset.Position(instr.Pos())
	if site, frontiered := state.CheckFrontier(place.Key()); frontiered {
		checker.reportFrontierViolation(pos, place, site, borrowCap.String()+" borrow")
		return
	}
	if namedBorrowSourceAllowed(checker.caps, borrowCap, place) {
		return
	}
	sourceCap := checker.capForPlace(place)
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN010,
		pos,
		fmt.Sprintf("cannot create %s borrow from %s value %q", borrowCap, sourceCap, place.Root.Name()),
	))
}

func (checker *ssaGWN001Checker) applyBoundCallTransfer(binding CallBinding, state *SSAFunctionState, instr ssa.Instruction, kind string) {
	for i, paramCap := range binding.ParamCaps {
		if i >= len(binding.ArgPlaces) {
			continue
		}
		argPlace := binding.ArgPlaces[i]
		if argPlace.Root == nil {
			continue
		}
		switch paramCap {
		case CapIso:
			if placeCanTransferAsIso(checker.caps, argPlace) {
				checker.consumeRootAtInstruction(state, argPlace, instr, kind)
			}
		case CapMub, CapRob:
			checker.applyTemporaryBorrow(state, argPlace, paramCap, instr)
		}
	}
}

func (checker *ssaGWN001Checker) applyGoTransfer(instr *ssa.Go, state *SSAFunctionState) {
	if binding, ok := checker.bindings.GoCall(checker.pkg, instr); ok {
		checker.applyBoundCallTransfer(binding, state, instr, "go")
	} else {
		checker.applyUntrackedCallFrontier(&instr.Call, state, instr)
		checker.applyCallCommonTransfer(&instr.Call, state, instr, "go")
	}
	checker.applyGoClosureCaptureTransfer(instr, state)
}

func (checker *ssaGWN001Checker) applyDeferTransfer(instr *ssa.Defer, state *SSAFunctionState) {
	if binding, ok := checker.bindings.DeferCall(checker.pkg, instr); ok {
		checker.applyBoundCallTransfer(binding, state, instr, "defer")
		checker.applyDeferredInferredBorrowArgs(binding, state, instr)
		for _, place := range binding.ArgPlaces {
			checker.activateDeferredNamedBorrow(state, place, instr)
		}
	} else {
		checker.applyUntrackedCallFrontier(&instr.Call, state, instr)
		checker.applyCallCommonTransfer(&instr.Call, state, instr, "defer")
		checker.applyDeferredCallCommonTransfer(&instr.Call, state, instr)
		for _, arg := range instr.Call.Args {
			place, ok := checker.places.PlaceForValue(arg)
			if !ok {
				continue
			}
			checker.activateDeferredNamedBorrow(state, place, instr)
		}
	}
	checker.applyDeferClosureCaptureTransfer(instr, state)
	checker.applySourceDeferClosureCaptureTransfer(instr, state)
	checker.recordSourceDeferClosureEffects(instr, state)
}

func (checker *ssaGWN001Checker) applyDeferredInferredBorrowArgs(binding CallBinding, state *SSAFunctionState, instr ssa.Instruction) {
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapMub && paramCap != CapRob {
			continue
		}
		if i >= len(binding.ArgPlaces) {
			continue
		}
		place := binding.ArgPlaces[i]
		if place.Root == nil {
			continue
		}
		if violation, ok := state.BeginBorrow(place.Key(), paramCap); ok {
			checker.reportViolation(checker.pkg.Fset.Position(instr.Pos()), violation)
		}
	}
}

func (checker *ssaGWN001Checker) applyUntrackedCallFrontier(call *ssa.CallCommon, state *SSAFunctionState, instr ssa.Instruction) bool {
	return false
}

func (checker *ssaGWN001Checker) applyDeferredCallCommonTransfer(call *ssa.CallCommon, state *SSAFunctionState, instr ssa.Instruction) {
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
		if paramCap != CapMub && paramCap != CapRob {
			continue
		}
		if i >= len(call.Args) {
			continue
		}
		argPlace, ok := checker.places.PlaceForValue(call.Args[i])
		if !ok || argPlace.Root == nil {
			continue
		}
		if violation, ok := state.BeginBorrow(argPlace.Key(), paramCap); ok {
			checker.reportViolation(checker.pkg.Fset.Position(instr.Pos()), violation)
		}
	}
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
		if i >= len(call.Args) {
			continue
		}
		argPlace, ok := checker.places.PlaceForValue(call.Args[i])
		if !ok || argPlace.Root == nil {
			continue
		}
		switch paramCap {
		case CapIso:
			if placeCanTransferAsIso(checker.caps, argPlace) {
				checker.consumeRootAtInstruction(state, argPlace, instr, kind)
			}
		case CapMub, CapRob:
			checker.applyTemporaryBorrow(state, argPlace, paramCap, instr)
		}
	}
}

func (checker *ssaGWN001Checker) applyTemporaryBorrow(state *SSAFunctionState, place Place, cap Cap, instr ssa.Instruction) {
	if site, frontiered := state.CheckFrontier(place.Key()); frontiered {
		checker.reportFrontierViolation(checker.pkg.Fset.Position(instr.Pos()), place, site, cap.String())
		return
	}
	violation, ok := state.BeginBorrow(place.Key(), cap)
	if ok {
		checker.reportViolation(checker.pkg.Fset.Position(instr.Pos()), violation)
		return
	}
	state.EndBorrow(place.Key(), cap)
}

func (checker *ssaGWN001Checker) applyInterfaceFrontier(instr *ssa.MakeInterface, state *SSAFunctionState) {
}

func (checker *ssaGWN001Checker) applyDeferClosureCaptureTransfer(instr *ssa.Defer, state *SSAFunctionState) {
	closure, _ := instr.Call.Value.(*ssa.MakeClosure)
	if closure == nil {
		return
	}
	for _, binding := range closure.Bindings {
		place, ok := checker.places.PlaceForValue(binding)
		if !ok {
			continue
		}
		checker.activateDeferredNamedBorrow(state, place, instr)
	}
}

func (checker *ssaGWN001Checker) applySourceDeferClosureCaptureTransfer(instr *ssa.Defer, state *SSAFunctionState) {
	key := sourcePositionKey(checker.pkg.Fset.Position(instr.Pos()))
	for _, obj := range checker.activeNamedBorrows.DeferredCaptures[key] {
		borrow, ok := checker.activeNamedBorrows.Borrows[obj]
		if !ok {
			continue
		}
		checker.activateDeferredBorrow(state, borrow, instr)
	}
}

func (checker *ssaGWN001Checker) recordSourceDeferClosureEffects(instr *ssa.Defer, state *SSAFunctionState) {
	key := sourcePositionKey(checker.pkg.Fset.Position(instr.Pos()))
	effects := checker.activeDeferEffects.Effects[key]
	closure := deferredClosureFunction(instr)
	if len(effects) == 0 && closure == nil {
		return
	}
	group := SSADeferredGroup{
		Key:     key,
		Pos:     checker.pkg.Fset.Position(instr.Pos()),
		Closure: closure,
	}
	for _, effect := range effects {
		group.Effects = append(group.Effects, SSADeferredEffect{
			Place: effect.Place,
			Cap:   effect.Cap,
			Pos:   effect.Pos,
		})
	}
	state.AddDeferred(group)
}

func (checker *ssaGWN001Checker) applyReturnTransfer(instr *ssa.Return, state *SSAFunctionState) {
	if instr == nil {
		return
	}
	checker.checkReturnFrontierConflicts(instr, state)
	checker.applyReturnOwnershipTransfers(instr, state)
	if len(state.Deferred) == 0 {
		return
	}
	checker.checkReturnDeferredConflicts(instr, state)
	exitState := state.Clone()
	for i := len(exitState.Deferred) - 1; i >= 0; i-- {
		checker.applyDeferredGroupAtExit(exitState.Deferred[i], &exitState)
	}
	*state = exitState
}

func (checker *ssaGWN001Checker) applyReturnOwnershipTransfers(instr *ssa.Return, state *SSAFunctionState) {
	resultCaps := checker.resultCapsForReturn(instr)
	if len(resultCaps) == 0 {
		return
	}
	values := checker.valueCapabilitiesForReturn(instr)
	pos := checker.pkg.Fset.Position(instr.Pos())
	for i, resultCap := range resultCaps {
		if i >= len(values) || !capTracked(resultCap) {
			continue
		}
		value := values[i]
		switch resultCap {
		case CapIso:
			if value.Cap != CapIso {
				checker.reportReturnCapabilityMismatch(pos, value, resultCap)
				continue
			}
			if !value.Fresh && value.Place.Root != nil {
				checker.consumeRootAtInstruction(state, value.Place, instr, "return")
			}
		case CapImm:
			switch value.Cap {
			case CapImm:
				continue
			case CapIso:
				if !value.Fresh && value.Place.Root != nil {
					checker.consumeRootAtInstruction(state, value.Place, instr, "return freeze")
				}
			default:
				checker.reportReturnCapabilityMismatch(pos, value, resultCap)
			}
		}
	}
}

func (checker *ssaGWN001Checker) reportReturnCapabilityMismatch(pos token.Position, value ValueCapability, resultCap Cap) {
	if value.Cap == CapInvalid {
		return
	}
	name := "<expression>"
	if value.Place.Root != nil {
		name = value.Place.Root.Name()
	}
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN010,
		pos,
		fmt.Sprintf("cannot return %s value %q as %s result", value.Cap, name, resultCap),
	))
}

func (checker *ssaGWN001Checker) valueCapabilitiesForReturn(instr *ssa.Return) []ValueCapability {
	key := sourcePositionKey(checker.pkg.Fset.Position(instr.Pos()))
	values := append([]ValueCapability(nil), checker.returnValues[key]...)
	for len(values) < len(instr.Results) {
		i := len(values)
		value := ValueCapability{Cap: CapInvalid}
		if place, ok := checker.places.PlaceForValue(instr.Results[i]); ok && place.Root != nil {
			value = ValueCapability{
				Cap:   checker.capForPlace(place),
				Place: place,
			}
		}
		values = append(values, value)
	}
	return values
}

func (checker *ssaGWN001Checker) checkReturnFrontierConflicts(instr *ssa.Return, state *SSAFunctionState) {
	resultCaps := checker.resultCapsForReturn(instr)
	if len(resultCaps) == 0 {
		return
	}
	sourcePlaces := checker.returns[sourcePositionKey(checker.pkg.Fset.Position(instr.Pos()))]
	for i, resultCap := range resultCaps {
		if !capTracked(resultCap) {
			continue
		}
		var place Place
		var ok bool
		if i < len(sourcePlaces) {
			place, ok = sourcePlaces[i], sourcePlaces[i].Root != nil
		}
		if !ok && i < len(instr.Results) {
			place, ok = checker.places.PlaceForValue(instr.Results[i])
		}
		if !ok || place.Root == nil {
			continue
		}
		if site, frontiered := state.CheckFrontier(place.Key()); frontiered {
			checker.reportFrontierViolation(checker.pkg.Fset.Position(instr.Pos()), place, site, resultCap.String()+" return")
			return
		}
	}
}

func (checker *ssaGWN001Checker) resultCapsForReturn(instr *ssa.Return) []Cap {
	if instr == nil || instr.Block() == nil || instr.Block().Parent() == nil {
		return nil
	}
	fnObj, _ := instr.Block().Parent().Object().(*types.Func)
	funcCap := checker.caps.FuncCap(fnObj)
	if funcCap == nil {
		return nil
	}
	return funcCap.Results
}

func (checker *ssaGWN001Checker) checkReturnDeferredConflicts(instr *ssa.Return, state *SSAFunctionState) {
	places := checker.returns[sourcePositionKey(checker.pkg.Fset.Position(instr.Pos()))]
	for _, result := range instr.Results {
		returned, ok := checker.places.PlaceForValue(result)
		if ok && returned.Root != nil {
			places = append(places, returned)
		}
	}
	for _, returned := range places {
		if returned.Root == nil {
			continue
		}
		returnedKey := returned.Key()
		for _, group := range state.Deferred {
			for _, effect := range group.Effects {
				if !returnedKey.Overlaps(effect.Place.Key()) {
					continue
				}
				checker.reportCheckerError(newCheckerErrorAtPosition(
					GWN001,
					checker.pkg.Fset.Position(instr.Pos()),
					fmt.Sprintf("cannot return %q while deferred closure uses it", returned.Root.Name()),
				))
				return
			}
		}
	}
}

func (checker *ssaGWN001Checker) applyDeferredGroupAtExit(group SSADeferredGroup, state *SSAFunctionState) {
	if checker.reportRepeatedDeferredMoves(group) {
		return
	}
	if group.Closure != nil {
		checker.applyDeferredClosureAtExit(group, state)
		return
	}
	for _, effect := range group.Effects {
		checker.applyDeferredEffectAtExit(effect, state)
	}
}

func (checker *ssaGWN001Checker) applyDeferredClosureAtExit(group SSADeferredGroup, state *SSAFunctionState) {
	if group.Closure == nil || len(group.Closure.Blocks) == 0 {
		return
	}
	outerDeferred := state.Deferred
	initial := state.Clone()
	initial.Deferred = nil
	exit, ok := checker.runFunctionBody(group.Closure, initial)
	if !ok {
		return
	}
	exit.Deferred = outerDeferred
	*state = exit
}

func (checker *ssaGWN001Checker) reportRepeatedDeferredMoves(group SSADeferredGroup) bool {
	if !group.Repeat {
		return false
	}
	for _, effect := range group.Effects {
		if effect.Cap != CapIso || effect.Place.Root == nil || effect.Place.Key().Path != "" {
			continue
		}
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN001,
			effect.Pos,
			fmt.Sprintf("deferred closure may move \\iso value %q more than once", effect.Place.Root.Name()),
		))
		return true
	}
	return false
}

func (checker *ssaGWN001Checker) applyDeferredEffectAtExit(effect SSADeferredEffect, state *SSAFunctionState) {
	key := effect.Place.Key()
	switch effect.Cap {
	case CapIso:
		if site, moved := state.CheckUse(key); moved {
			checker.reportUseAfterMoveAtPosition(effect.Pos, effect.Place, site)
			return
		}
		violation, ok := state.ConsumeRoot(key, SSAMoveSite{
			Name:   effect.Place.Root.Name(),
			Kind:   "deferred closure",
			Path:   gownSourcePath(effect.Pos.Filename),
			Offset: effect.Pos.Offset,
			Line:   sourceLine(effect.Pos),
			Col:    sourceColumn(effect.Pos),
		})
		if ok {
			checker.reportViolation(effect.Pos, violation)
		}
	case CapMub, CapRob:
		violation, ok := state.BeginBorrow(key, effect.Cap)
		if ok {
			checker.reportViolation(effect.Pos, violation)
			return
		}
		state.EndBorrow(key, effect.Cap)
	}
}

func (checker *ssaGWN001Checker) activateDeferredNamedBorrow(state *SSAFunctionState, place Place, instr ssa.Instruction) {
	borrow, ok := checker.activeNamedBorrows.Borrows[place.Root]
	if !ok {
		return
	}
	checker.activateDeferredBorrow(state, borrow, instr)
}

func (checker *ssaGWN001Checker) activateDeferredBorrow(state *SSAFunctionState, borrow SSANamedBorrow, instr ssa.Instruction) {
	if state.HasBorrow(borrow.Source.Key(), borrow.Cap) {
		return
	}
	violation, violated := state.BeginBorrow(borrow.Source.Key(), borrow.Cap)
	if !violated {
		return
	}
	checker.reportViolation(checker.pkg.Fset.Position(instr.Pos()), violation)
}

func (checker *ssaGWN001Checker) applyGoClosureCaptureTransfer(instr *ssa.Go, state *SSAFunctionState) {
	closure, _ := instr.Call.Value.(*ssa.MakeClosure)
	if closure == nil {
		return
	}
	for _, binding := range closure.Bindings {
		place, ok := checker.places.PlaceForValue(binding)
		if !ok || place.Root == nil || !placeCanTransferAsIso(checker.caps, place) {
			continue
		}
		checker.consumeRootAtInstruction(state, place, instr, "go")
	}
}

func deferredClosureFunction(instr *ssa.Defer) *ssa.Function {
	if instr == nil {
		return nil
	}
	closure, _ := instr.Call.Value.(*ssa.MakeClosure)
	if closure == nil {
		return nil
	}
	fn, _ := closure.Fn.(*ssa.Function)
	return fn
}

func (checker *ssaGWN001Checker) consumeRootAtInstruction(state *SSAFunctionState, place Place, instr ssa.Instruction, kind string) {
	pos := checker.pkg.Fset.Position(instr.Pos())
	key := place.Key()
	if site, frontiered := state.CheckFrontier(key); frontiered {
		checker.reportFrontierViolation(pos, place, site, kind)
		return
	}
	site := SSAMoveSite{
		Name:   place.Root.Name(),
		Kind:   kind,
		Path:   gownSourcePath(pos.Filename),
		Offset: pos.Offset,
		Line:   sourceLine(pos),
		Col:    sourceColumn(pos),
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

func (checker *ssaGWN001Checker) debugRefUsePosition(debug *ssa.DebugRef) token.Position {
	if checker == nil || checker.pkg == nil || debug == nil {
		return token.Position{}
	}
	if debug.Expr != nil && debug.Expr.Pos().IsValid() {
		return checker.pkg.Fset.Position(debug.Expr.Pos())
	}
	return checker.pkg.Fset.Position(debug.Pos())
}

func (checker *ssaGWN001Checker) reportOperandUseAfterMove(instr ssa.Instruction, value ssa.Value, place Place, site SSAMoveSite) {
	checker.reportUseAfterMoveAtPosition(checker.operandUsePosition(instr, value), place, site)
}

func (checker *ssaGWN001Checker) operandUsePosition(instr ssa.Instruction, value ssa.Value) token.Position {
	if checker == nil || checker.pkg == nil {
		return token.Position{}
	}
	pos := checker.pkg.Fset.Position(instr.Pos())
	if validSourcePosition(pos) {
		return pos
	}
	if source, ok := checker.places.SourceForValue(value); ok {
		return source.Position(checker.pkg.Fset)
	}
	return pos
}

func validSourcePosition(pos token.Position) bool {
	return pos.Filename != "" && pos.Line > 0
}

func (checker *ssaGWN001Checker) reportUseAfterMove(instr ssa.Instruction, place Place, site SSAMoveSite) {
	checker.reportUseAfterMoveAtPosition(checker.pkg.Fset.Position(instr.Pos()), place, site)
}

func (checker *ssaGWN001Checker) reportUseAfterMoveAtPosition(pos token.Position, place Place, site SSAMoveSite) {
	name := "<unknown>"
	if place.Root != nil {
		name = place.Root.Name()
	}
	message := fmt.Sprintf("use of moved \\iso value %q after %s at %d:%d",
		name, site.Kind, site.Line, site.Col)
	err := newCheckerErrorAtPosition(
		GWN001,
		pos,
		message,
	)
	if site.Line > 0 {
		path := site.Path
		if path == "" {
			path = gownSourcePath(pos.Filename)
		}
		err.Notes = []CheckerNote{{
			Path:    path,
			Offset:  site.Offset,
			Line:    site.Line,
			Col:     site.Col,
			Message: fmt.Sprintf("moved by %s here", site.Kind),
		}}
	}
	checker.reportCheckerError(err)
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

func collectSSAAssignments(pkg *packages.Package, caps *CapabilityIndex) map[ast.Expr]ssaAssignment {
	assignments := make(map[ast.Expr]ssaAssignment)
	if pkg == nil || caps == nil {
		return assignments
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			if assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE {
				return true
			}
			if len(assign.Lhs) != len(assign.Rhs) {
				return true
			}
			for i, lhs := range assign.Lhs {
				dst, ok := caps.PlaceForExpr(lhs)
				if !ok || dst.Root == nil || dst.Key().Path != "" || capForSSAPlace(caps, dst) != CapIso {
					continue
				}
				rhs := assign.Rhs[i]
				value := assignmentValueCapability(pkg, caps, rhs)
				if value.Place.Root == dst.Root && !value.Fresh {
					continue
				}
				assignments[debugRefExprKey(lhs)] = ssaAssignment{Dst: dst, Value: value, RHS: rhs}
			}
			return true
		})
	}
	return assignments
}

func collectSSAFlowAssignments(pkg *packages.Package, caps *CapabilityIndex) map[ast.Expr]ssaFlowAssignment {
	assignments := make(map[ast.Expr]ssaFlowAssignment)
	if pkg == nil || caps == nil {
		return assignments
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			if assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE {
				return true
			}
			if len(assign.Lhs) != len(assign.Rhs) {
				return true
			}
			for i, lhs := range assign.Lhs {
				dst, ok := caps.PlaceForExpr(lhs)
				if !ok || !flowAssignmentDstAllowed(pkg, caps, dst) {
					continue
				}
				value := ValueCapability{Cap: CapInvalid}
				if cap := receiveCapForValueExpr(caps, assign.Rhs[i]); cap != CapInvalid {
					value.Cap = cap
				}
				assignments[debugRefExprKey(lhs)] = ssaFlowAssignment{Dst: dst, Value: value}
			}
			return true
		})
	}
	return assignments
}

func flowAssignmentDstAllowed(pkg *packages.Package, caps *CapabilityIndex, dst Place) bool {
	if pkg == nil || caps == nil || dst.Root == nil || dst.Key().Path != "" {
		return false
	}
	if capForSSAPlace(caps, dst) != CapUntracked {
		return false
	}
	obj, ok := dst.Root.(*types.Var)
	if !ok || obj.IsField() {
		return false
	}
	return obj.Parent() != pkg.Types.Scope()
}

func collectSSAImmutableProjectionUseSpans(pkg *packages.Package, caps *CapabilityIndex) []ssaImmutableProjectionUseSpan {
	var spans []ssaImmutableProjectionUseSpan
	if pkg == nil || caps == nil {
		return spans
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			place, ok := caps.PlaceForExpr(expr)
			if !ok || place.Root == nil || place.Key().Path == "" {
				return true
			}
			if capForSSAPlace(caps, place) != CapImm {
				return true
			}
			spans = append(spans, ssaImmutableProjectionUseSpan{
				Root:  place.Root,
				Start: expr.Pos(),
				End:   expr.End(),
			})
			return true
		})
	}
	return spans
}

func assignmentValueCapability(pkg *packages.Package, caps *CapabilityIndex, expr ast.Expr) ValueCapability {
	if value, ok := valueCapabilityForExpr(pkg, caps, expr); ok {
		return value
	}
	if cap := receiveCapForValueExpr(caps, expr); cap != CapInvalid {
		return ValueCapability{Cap: cap, Fresh: cap == CapIso}
	}
	return ValueCapability{Cap: CapInvalid}
}

func assignmentValueTypeString(pkg *packages.Package, expr ast.Expr) string {
	if pkg == nil || pkg.TypesInfo == nil || expr == nil {
		return "<unknown>"
	}
	typ := pkg.TypesInfo.TypeOf(expr)
	if typ == nil {
		return "<unknown>"
	}
	return types.TypeString(typ, func(p *types.Package) string {
		if p == nil {
			return ""
		}
		return p.Name()
	})
}

func assignmentValueInvalidReason(caps *CapabilityIndex, assignment ssaAssignment) string {
	expr := unparenExpr(assignment.RHS)
	if recv, ok := expr.(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
		place, ok := caps.PlaceForExpr(recv.X)
		if !ok || place.Root == nil {
			return "receive channel expression is not a tracked place"
		}
		chCap := chanElemCapForPlace(caps, place)
		if !capTracked(chCap) {
			return fmt.Sprintf("receive channel %s has %s element capability", placeName(place), chCap)
		}
		if chCap != CapIso {
			return fmt.Sprintf("receive channel %s produces %s, but \\iso root rebinding requires \\iso", placeName(place), chCap)
		}
	}
	switch assignment.Value.Cap {
	case CapInvalid:
		return "expression is not known to produce an ownerstamp-tracked value"
	case CapUntracked:
		return "\\iso root rebinding requires a fresh or moved \\iso value; this expression is untracked"
	default:
		return fmt.Sprintf("\\iso root rebinding requires a fresh or moved \\iso value, not %s", assignment.Value.Cap)
	}
}

func placeName(place Place) string {
	if place.Root == nil {
		return "<unknown>"
	}
	return place.Root.Name() + place.Projection.String()
}

func collectSSAReturnPlaces(pkg *packages.Package, caps *CapabilityIndex) map[sourcePosKey][]Place {
	returns := make(map[sourcePosKey][]Place)
	if pkg == nil || caps == nil {
		return returns
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			key := sourcePositionKey(pkg.Fset.Position(ret.Return))
			for _, result := range ret.Results {
				place, ok := caps.PlaceForExpr(result)
				if !ok || place.Root == nil {
					continue
				}
				returns[key] = append(returns[key], place)
			}
			return true
		})
	}
	return returns
}

func collectSSAReturnValues(pkg *packages.Package, caps *CapabilityIndex) map[sourcePosKey][]ValueCapability {
	returns := make(map[sourcePosKey][]ValueCapability)
	if pkg == nil || caps == nil {
		return returns
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			key := sourcePositionKey(pkg.Fset.Position(ret.Return))
			values := make([]ValueCapability, len(ret.Results))
			for i, result := range ret.Results {
				if value, ok := valueCapabilityForExpr(pkg, caps, result); ok {
					values[i] = value
				} else {
					values[i] = ValueCapability{Cap: CapInvalid}
				}
			}
			returns[key] = values
			return true
		})
	}
	return returns
}

func capForSSAPlace(caps *CapabilityIndex, place Place) Cap {
	return EffectivePlaceCap(caps, place)
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

func (checker *ssaGWN001Checker) enterFrontierAtInstruction(state *SSAFunctionState, place Place, instr ssa.Instruction, kind string) {
	if place.Root == nil {
		return
	}
	pos := checker.pkg.Fset.Position(instr.Pos())
	state.EnterFrontier(place.Key(), SSAFrontierSite{
		Name:   place.Root.Name(),
		Kind:   kind,
		Path:   gownSourcePath(pos.Filename),
		Offset: pos.Offset,
		Line:   sourceLine(pos),
		Col:    sourceColumn(pos),
	})
}

func (checker *ssaGWN001Checker) reportFrontierViolation(pos token.Position, place Place, site SSAFrontierSite, operation string) {
	name := "<unknown>"
	if place.Root != nil {
		name = place.Root.Name()
	}
	message := fmt.Sprintf("cannot use %q as %s after proof ended at %d:%d (%s)",
		name, operation, site.Line, site.Col, site.Kind)
	err := newCheckerErrorAtPosition(
		GWN012,
		pos,
		message,
	)
	err.Notes = []CheckerNote{{
		Path:    site.Path,
		Offset:  site.Offset,
		Line:    site.Line,
		Col:     site.Col,
		Message: fmt.Sprintf("proof ended here (%s)", site.Kind),
	}}
	checker.reportCheckerError(err)
}
