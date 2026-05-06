package gown

func EffectivePlaceCap(caps *CapabilityIndex, place Place) Cap {
	if caps == nil || place.Root == nil {
		return CapInvalid
	}
	cap := caps.ObjectCap(place.Root)
	if len(place.Projection) == 0 {
		return cap
	}
	for _, projection := range place.Projection {
		fieldCap := CapUntracked
		if projection.Field != nil {
			fieldCap = caps.ObjectCap(projection.Field)
		}
		cap = adaptFieldCapability(cap, fieldCap)
	}
	return cap
}

func adaptFieldCapability(ownerCap, fieldCap Cap) Cap {
	switch ownerCap {
	case CapIso:
		if capTracked(fieldCap) {
			return fieldCap
		}
		return CapUntracked
	case CapMub:
		switch fieldCap {
		case CapIso, CapMub:
			return CapMub
		case CapRob:
			return CapRob
		case CapImm:
			return CapImm
		default:
			return CapUntracked
		}
	case CapRob:
		return CapRob
	case CapImm:
		return CapImm
	case CapUntracked:
		if fieldCap == CapRob || fieldCap == CapImm {
			return fieldCap
		}
		return CapUntracked
	default:
		return CapInvalid
	}
}

func placeCanTransferAsIso(caps *CapabilityIndex, place Place) bool {
	if capForSSAPlace(caps, place) == CapIso {
		return true
	}
	if place.Key().Path == "" || caps == nil || place.Root == nil {
		return false
	}
	if caps.ObjectCap(place.Root) == CapIso {
		return true
	}
	if len(place.Projection) == 0 {
		return false
	}
	field := place.Projection[len(place.Projection)-1].Field
	return field != nil && caps.ObjectCap(field) == CapIso
}
