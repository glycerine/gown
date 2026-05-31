package gown

type SSAFunctionState struct {
	Consumed   map[PlaceKey]SSAMoveSite
	Frontiered map[PlaceKey]SSAFrontierSite
	FlowValues map[PlaceKey]SSAFlowValue
	Borrows    []SSABorrow
	Deferred   []SSADeferredGroup
}

type SSAFlowValue struct {
	Cap Cap
}

type SSAMoveSite struct {
	Name   string
	Kind   string
	Path   string
	Offset int
	Line   int
	Col    int
}

type SSABorrow struct {
	Place PlaceKey
	Cap   Cap
}

type SSAFrontierSite struct {
	Name   string
	Kind   string
	Path   string
	Offset int
	Line   int
	Col    int
}

type SSAStateViolation struct {
	Code    CheckerErrorCode
	Place   PlaceKey
	Message string
}

func NewSSAFunctionState() SSAFunctionState {
	return SSAFunctionState{
		Consumed:   make(map[PlaceKey]SSAMoveSite),
		Frontiered: make(map[PlaceKey]SSAFrontierSite),
		FlowValues: make(map[PlaceKey]SSAFlowValue),
	}
}

func (state SSAFunctionState) Clone() SSAFunctionState {
	clone := NewSSAFunctionState()
	for place, site := range state.Consumed {
		clone.Consumed[place] = site
	}
	for place, site := range state.Frontiered {
		clone.Frontiered[place] = site
	}
	for place, value := range state.FlowValues {
		clone.FlowValues[place] = value
	}
	clone.Borrows = append(clone.Borrows, state.Borrows...)
	clone.Deferred = append(clone.Deferred, state.Deferred...)
	return clone
}

func (state *SSAFunctionState) ConsumeRoot(place PlaceKey, site SSAMoveSite) (SSAStateViolation, bool) {
	if place.Root == nil {
		return SSAStateViolation{}, false
	}
	if _, moved := state.CheckUse(place); moved {
		return SSAStateViolation{
			Code:    GWN001,
			Place:   place,
			Message: "cannot move already moved place",
		}, true
	}
	if place.Path != "" {
		return SSAStateViolation{
			Code:    GWN011,
			Place:   place,
			Message: "cannot move field projection",
		}, true
	}
	for _, borrow := range state.Borrows {
		if borrow.Place.Overlaps(place) {
			return SSAStateViolation{
				Code:    GWN002,
				Place:   place,
				Message: "cannot move root while borrow is active",
			}, true
		}
	}
	state.Consumed[place] = site
	return SSAStateViolation{}, false
}

func (state *SSAFunctionState) CheckUse(place PlaceKey) (SSAMoveSite, bool) {
	for consumed, site := range state.Consumed {
		if consumed.Overlaps(place) {
			return site, true
		}
	}
	return SSAMoveSite{}, false
}

func (state *SSAFunctionState) UnconsumeRoot(place PlaceKey) {
	if place.Root == nil {
		return
	}
	delete(state.Consumed, PlaceKey{Root: place.Root})
}

func (state *SSAFunctionState) EnterFrontier(place PlaceKey, site SSAFrontierSite) {
	if place.Root == nil {
		return
	}
	state.Frontiered[place] = site
}

func (state *SSAFunctionState) CheckFrontier(place PlaceKey) (SSAFrontierSite, bool) {
	for frontiered, site := range state.Frontiered {
		if frontiered.Overlaps(place) {
			return site, true
		}
	}
	return SSAFrontierSite{}, false
}

func (state *SSAFunctionState) UnfrontierRoot(place PlaceKey) {
	if place.Root == nil {
		return
	}
	for frontiered := range state.Frontiered {
		if frontiered.Root == place.Root {
			delete(state.Frontiered, frontiered)
		}
	}
}

func (state *SSAFunctionState) SetFlowValue(place PlaceKey, value SSAFlowValue) {
	if place.Root == nil || place.Path != "" || !capTracked(value.Cap) {
		return
	}
	if state.FlowValues == nil {
		state.FlowValues = make(map[PlaceKey]SSAFlowValue)
	}
	state.FlowValues[place] = value
}

func (state *SSAFunctionState) ClearFlowValue(place PlaceKey) {
	if place.Root == nil {
		return
	}
	delete(state.FlowValues, PlaceKey{Root: place.Root})
}

func (state *SSAFunctionState) FlowValue(place PlaceKey) (SSAFlowValue, bool) {
	if place.Root == nil {
		return SSAFlowValue{}, false
	}
	value, ok := state.FlowValues[PlaceKey{Root: place.Root}]
	return value, ok
}

func (state *SSAFunctionState) BeginBorrow(place PlaceKey, cap Cap) (SSAStateViolation, bool) {
	if cap != CapMub && cap != CapRob {
		return SSAStateViolation{}, false
	}
	if _, moved := state.CheckUse(place); moved {
		return SSAStateViolation{
			Code:    GWN001,
			Place:   place,
			Message: "cannot borrow moved place",
		}, true
	}
	next := SSABorrow{Place: place, Cap: cap}
	for _, active := range state.Borrows {
		if borrowsConflict(active, next) {
			return SSAStateViolation{
				Code:    GWN002,
				Place:   place,
				Message: "conflicting borrow",
			}, true
		}
	}
	state.Borrows = append(state.Borrows, next)
	return SSAStateViolation{}, false
}

func (state *SSAFunctionState) EndBorrow(place PlaceKey, cap Cap) {
	for i, borrow := range state.Borrows {
		if borrow.Place == place && borrow.Cap == cap {
			state.Borrows = append(state.Borrows[:i], state.Borrows[i+1:]...)
			return
		}
	}
}

func (state *SSAFunctionState) HasBorrow(place PlaceKey, cap Cap) bool {
	for _, borrow := range state.Borrows {
		if borrow.Place == place && borrow.Cap == cap {
			return true
		}
	}
	return false
}

func (state *SSAFunctionState) AddDeferred(group SSADeferredGroup) {
	if group.Closure == nil && len(group.Effects) == 0 {
		return
	}
	for i := range state.Deferred {
		if state.Deferred[i].Key == group.Key {
			state.Deferred[i].Repeat = true
			return
		}
	}
	state.Deferred = append(state.Deferred, group)
}

func MergeSSAFunctionStates(left, right SSAFunctionState) (SSAFunctionState, []SSAStateViolation) {
	merged := NewSSAFunctionState()
	for place, site := range left.Consumed {
		merged.Consumed[place] = site
	}
	for place, site := range right.Consumed {
		if _, ok := merged.Consumed[place]; !ok {
			merged.Consumed[place] = site
		}
	}
	for place, site := range left.Frontiered {
		merged.Frontiered[place] = site
	}
	for place, site := range right.Frontiered {
		if _, ok := merged.Frontiered[place]; !ok {
			merged.Frontiered[place] = site
		}
	}
	for place, leftValue := range left.FlowValues {
		if rightValue, ok := right.FlowValues[place]; ok && rightValue == leftValue {
			merged.FlowValues[place] = leftValue
		}
	}

	var violations []SSAStateViolation
	for _, borrow := range left.Borrows {
		if violation, ok := merged.addMergedBorrow(borrow); ok {
			violations = append(violations, violation)
		}
	}
	for _, borrow := range right.Borrows {
		if violation, ok := merged.addMergedBorrow(borrow); ok {
			violations = append(violations, violation)
		}
	}
	for _, group := range left.Deferred {
		merged.addMergedDeferred(group)
	}
	for _, group := range right.Deferred {
		merged.addMergedDeferred(group)
	}
	return merged, violations
}

func (state *SSAFunctionState) addMergedBorrow(next SSABorrow) (SSAStateViolation, bool) {
	for _, active := range state.Borrows {
		if active == next {
			return SSAStateViolation{}, false
		}
		if borrowsConflict(active, next) {
			return SSAStateViolation{
				Code:    GWN002,
				Place:   next.Place,
				Message: "conflicting borrows at merge",
			}, true
		}
	}
	state.Borrows = append(state.Borrows, next)
	return SSAStateViolation{}, false
}

func (state *SSAFunctionState) addMergedDeferred(next SSADeferredGroup) {
	for i := range state.Deferred {
		if state.Deferred[i].Key == next.Key {
			state.Deferred[i].Repeat = state.Deferred[i].Repeat || next.Repeat
			return
		}
	}
	state.Deferred = append(state.Deferred, next)
}

func borrowsConflict(a, b SSABorrow) bool {
	return a.Place.Overlaps(b.Place) && callBorrowsConflict(a.Cap, b.Cap)
}

func equalSSAFunctionState(a, b SSAFunctionState) bool {
	if len(a.Consumed) != len(b.Consumed) ||
		len(a.Frontiered) != len(b.Frontiered) ||
		len(a.FlowValues) != len(b.FlowValues) ||
		len(a.Borrows) != len(b.Borrows) ||
		len(a.Deferred) != len(b.Deferred) {
		return false
	}
	for place, site := range a.Consumed {
		if b.Consumed[place] != site {
			return false
		}
	}
	for place, site := range a.Frontiered {
		if b.Frontiered[place] != site {
			return false
		}
	}
	for place, value := range a.FlowValues {
		if b.FlowValues[place] != value {
			return false
		}
	}
	for _, borrow := range a.Borrows {
		if !hasSSABorrow(b.Borrows, borrow) {
			return false
		}
	}
	for _, group := range a.Deferred {
		if !hasSSADeferredGroup(b.Deferred, group) {
			return false
		}
	}
	return true
}

func hasSSABorrow(borrows []SSABorrow, want SSABorrow) bool {
	for _, borrow := range borrows {
		if borrow == want {
			return true
		}
	}
	return false
}

func hasSSADeferredGroup(groups []SSADeferredGroup, want SSADeferredGroup) bool {
	for _, group := range groups {
		if group.Key == want.Key && group.Repeat == want.Repeat && group.Closure == want.Closure {
			return true
		}
	}
	return false
}
