.phony: all lean

all:
	go install ./cmd/gown

lean:
	lean Gown.lean &> lean.run.log

test: all
	gown vectors/iso0/
	cat vectors/iso0/basic.go
