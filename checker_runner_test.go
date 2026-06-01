package gown

import "testing"

func TestRunCheckerPassesStopsAtFirstCheckerError(t *testing.T) {
	oldPasses := checkerPasses
	defer func() { checkerPasses = oldPasses }()

	ranFirst := false
	ranSecond := false
	ranThird := false
	checkerPasses = []CheckerPass{
		func(*CheckerContext) CheckerErrors {
			ranFirst = true
			return nil
		},
		func(*CheckerContext) CheckerErrors {
			ranSecond = true
			return CheckerErrors{
				{Code: GWN010, Path: "first.gown", Line: 1, Col: 2, Message: "first"},
				{Code: GWN012, Path: "second.gown", Line: 3, Col: 4, Message: "second"},
			}
		},
		func(*CheckerContext) CheckerErrors {
			ranThird = true
			return CheckerErrors{{Code: GWN001, Path: "third.gown", Line: 5, Col: 6, Message: "third"}}
		},
	}

	errs := runCheckerPasses(nil, nil, nil)
	if !ranFirst || !ranSecond {
		t.Fatalf("expected first two passes to run; ranFirst=%v ranSecond=%v", ranFirst, ranSecond)
	}
	if ranThird {
		t.Fatalf("third pass ran after the first checker error")
	}
	if len(errs) != 1 {
		t.Fatalf("len(errs) = %d, want 1: %#v", len(errs), errs)
	}
	if errs[0].Code != GWN010 || errs[0].Message != "first" {
		t.Fatalf("first surfaced error = %#v, want first error from failing pass", errs[0])
	}
}
