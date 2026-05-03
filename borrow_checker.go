package gown

import "fmt"

type callBorrow struct {
	key  PlaceKey
	cap  Cap
	name string
}

func checkCallBorrowConflicts(caps *CapabilityIndex) CheckerErrors {
	if caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, binding := range caps.CallBindings {
		if err, ok := checkCallBorrowConflict(binding); ok {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkCallBorrowConflict(binding CallBinding) (CheckerError, bool) {
	active := make(map[PlaceKey]callBorrow)
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapMub && paramCap != CapRob {
			continue
		}
		if i >= len(binding.ArgPlaces) {
			continue
		}
		key := binding.ArgPlaces[i].Key()
		if key.Root == nil {
			continue
		}
		next := callBorrow{
			key:  key,
			cap:  paramCap,
			name: key.Root.Name(),
		}
		if _, ok := conflictingActiveBorrow(active, next); ok {
			return CheckerError{
				Code:    GWN002,
				Path:    binding.Path,
				Offset:  binding.Offset,
				Line:    binding.Line,
				Col:     binding.Col,
				Message: fmt.Sprintf("conflicting inferred borrows of %q in call", next.name),
			}, true
		}
		active[key] = next
	}
	return CheckerError{}, false
}

func conflictingActiveBorrow(active map[PlaceKey]callBorrow, next callBorrow) (callBorrow, bool) {
	for _, prev := range active {
		if prev.key.Overlaps(next.key) && callBorrowsConflict(prev.cap, next.cap) {
			return prev, true
		}
	}
	return callBorrow{}, false
}

func callBorrowsConflict(a, b Cap) bool {
	return a == CapMub || b == CapMub
}
