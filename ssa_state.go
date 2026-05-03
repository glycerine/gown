package gown

type SSAFunctionState struct {
	Consumed map[PlaceKey]moveSite
	Borrows  []SSABorrow
}

type SSABorrow struct {
	Place PlaceKey
	Cap   Cap
}

type SSAStateViolation struct {
	Code    CheckerErrorCode
	Place   PlaceKey
	Message string
}

func NewSSAFunctionState() SSAFunctionState {
	return SSAFunctionState{
		Consumed: make(map[PlaceKey]moveSite),
	}
}

func (state *SSAFunctionState) ConsumeRoot(place PlaceKey, site moveSite) (SSAStateViolation, bool) {
	if place.Root == nil {
		return SSAStateViolation{}, false
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

func (state *SSAFunctionState) CheckUse(place PlaceKey) (moveSite, bool) {
	for consumed, site := range state.Consumed {
		if consumed.Overlaps(place) {
			return site, true
		}
	}
	return moveSite{}, false
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

func borrowsConflict(a, b SSABorrow) bool {
	return a.Place.Overlaps(b.Place) && callBorrowsConflict(a.Cap, b.Cap)
}
