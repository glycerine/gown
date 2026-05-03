package gown

import "fmt"

func reportCheckerErrorOnce(errs *CheckerErrors, reported map[string]bool, err CheckerError) {
	if reported == nil {
		*errs = append(*errs, err)
		return
	}
	key := fmt.Sprintf("%s:%d:%d:%s:%s", err.Path, err.Line, err.Col, err.Code, err.Message)
	if reported[key] {
		return
	}
	reported[key] = true
	*errs = append(*errs, err)
}
