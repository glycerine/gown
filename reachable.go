package gown

import "go/types"

// computeReachableTypes performs a BFS from boundary crossing types
// (and iso annotation types as fallback) to build the set of types
// that can participate in cross-goroutine ownership transfer.
//
// If an interface type is encountered, poisoned is set to true,
// meaning all pointer-containing types must be tracked.
func computeReachableTypes(boundaries []*boundaryCrossing, isoTypes []types.Type) (reachable map[types.Type]bool, poisoned bool) {
	reachable = make(map[types.Type]bool)

	var queue []types.Type
	if len(boundaries) > 0 {
		for _, bc := range boundaries {
			if bc.goType != nil {
				queue = append(queue, bc.goType)
			}
		}
	} else {
		queue = append(queue, isoTypes...)
	}

	visited := make(map[types.Type]bool)

	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]

		if t == nil {
			continue
		}

		u := t.Underlying()
		if visited[u] {
			continue
		}
		visited[u] = true
		reachable[t] = true
		reachable[u] = true

		switch x := u.(type) {
		case *types.Pointer:
			queue = append(queue, x.Elem())
		case *types.Struct:
			reachable[t] = true
			for i := 0; i < x.NumFields(); i++ {
				queue = append(queue, x.Field(i).Type())
			}
		case *types.Slice:
			queue = append(queue, x.Elem())
		case *types.Array:
			queue = append(queue, x.Elem())
		case *types.Map:
			queue = append(queue, x.Key())
			queue = append(queue, x.Elem())
		case *types.Chan:
			queue = append(queue, x.Elem())
		case *types.Interface:
			poisoned = true
			return
		case *types.Signature:
			params := x.Params()
			for i := 0; i < params.Len(); i++ {
				queue = append(queue, params.At(i).Type())
			}
			results := x.Results()
			for i := 0; i < results.Len(); i++ {
				queue = append(queue, results.At(i).Type())
			}
		}
	}
	return
}

// isReachable reports whether a type is in the reachable set,
// either directly or because it contains reachable element types.
func isReachable(t types.Type, reachable map[types.Type]bool, poisoned bool) bool {
	if poisoned {
		return true
	}
	if reachable[t] || reachable[t.Underlying()] {
		return true
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		return isReachable(u.Elem(), reachable, false)
	case *types.Chan:
		return isReachable(u.Elem(), reachable, false)
	case *types.Slice:
		return isReachable(u.Elem(), reachable, false)
	case *types.Array:
		return isReachable(u.Elem(), reachable, false)
	case *types.Map:
		return isReachable(u.Key(), reachable, false) || isReachable(u.Elem(), reachable, false)
	}
	return false
}
