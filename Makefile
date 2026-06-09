.PHONY: all lean test

all:
	go install ./cmd/gown
	go install ./cmd/gownfmt

lean:
	lean Gown.lean > lean.run.log 2>&1
	lean restore.lean >> lean.run.log 2>&1

test: all
	go test ./...
