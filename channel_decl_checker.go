package gown

import "fmt"

func checkChannelElementDeclarations(ctx *CheckerContext) CheckerErrors {
	if ctx == nil || ctx.Caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, qualifier := range ctx.Caps.InvalidChannelElementQualifiers {
		errs = append(errs, newCheckerErrorAtSource(
			GWN010,
			qualifier.Path,
			qualifier.Offset,
			qualifier.Line,
			qualifier.Col,
			fmt.Sprintf("channel element ownerstamp must be \\iso or \\imm, not %s", qualifier.Cap),
		))
	}
	return errs
}
