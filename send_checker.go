package gown

import "fmt"

func checkSendCapabilities(caps *CapabilityIndex) CheckerErrors {
	if caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, binding := range caps.SendBindings {
		if err, ok := checkSendCapabilityMatch(binding); ok {
			errs = append(errs, err)
		}
		if !capSendable(binding.ValueCap) {
			name := "<unknown>"
			if root := binding.ValueKey().Root; root != nil {
				name = root.Name()
			}
			errs = append(errs, CheckerError{
				Code:    GWN003,
				Path:    binding.Path,
				Offset:  binding.Offset,
				Line:    binding.Line,
				Col:     binding.Col,
				Message: fmt.Sprintf("cannot send non-sendable %s value %q", binding.ValueCap, name),
			})
		}
	}
	return errs
}

func checkSendCapabilityMatch(binding SendBinding) (CheckerError, bool) {
	if !capTracked(binding.ChanElemCap) || binding.ValueCap == binding.ChanElemCap {
		return CheckerError{}, false
	}
	name := "<unknown>"
	if root := binding.ValueKey().Root; root != nil {
		name = root.Name()
	}
	return CheckerError{
		Code:    GWN010,
		Path:    binding.Path,
		Offset:  binding.Offset,
		Line:    binding.Line,
		Col:     binding.Col,
		Message: fmt.Sprintf("cannot send %s value %q on %s channel", binding.ValueCap, name, binding.ChanElemCap),
	}, true
}

func capSendable(cap Cap) bool {
	return cap != CapMub && cap != CapRob
}
