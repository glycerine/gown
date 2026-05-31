# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Gown

Gown is an ownerstamp-typed preprocessor for Go. An ownerstamp is Gown's term for an ownership annotation such as `\iso`, `\mub`, `\rob`, or `\imm`. It accepts `.gown` files, rejects invalid ownership with a type error, or emits plain `.go` files with ownerstamps erased and code additions that assign nil to \iso pointers that have been consumed. The core guarantee is race freedom: if all source passes the Gown checker, no execution has a data race (except through explicit `\unsafe`).

## Build and Test

```bash
make all        # go install ./cmd/gown
make test       # builds, then runs: gown vectors/iso0/
make lean       # runs Lean 4 proof verification on Gown.lean
```

Run Go unit tests:
```bash
go test ./...
go test -run TestPositionPrecision ./
go test -run TestRegionDetection ./
```

The `gown` binary takes directory paths as arguments (each directory is one package):
```bash
gown [-check] vectors/iso0/
```
`-check` means typecheck only, do not overwrite `.go` files.

## Architecture

### Processing pipeline

1. **CLI** (`cmd/gown/gown.go`) — parses flags and directory arguments, creates a `GownPackage` per directory.
2. **Scan & strip** (`strip.go`) — `scanAndStrip()` finds `\iso` (and future `\mub`, `\rob`, `\imm`) keywords in `.gown` source, records each as an `isoAnnotation` (byte offset, line, column), and replaces them with spaces to produce valid `.go` source. Byte offsets are preserved so AST positions map back to annotation positions.
3. **Write `.go`** (`check.go`) — stripped source is written alongside each `.gown` file.
4. **Load package** (`check.go`) — uses `golang.org/x/tools/go/packages` to parse and type-check the generated `.go` files.
5. **Assign regions** (`regions.go`) — `assignRegions()` walks the AST to populate each annotation's containing function name and innermost enclosing block scope (stored as a `region` byte range).

### Key types

| Type | File | Purpose |
|------|------|---------|
| `GownPackage` | `check.go` | Per-directory orchestrator: holds the loaded package and per-file metadata |
| `gownFile` | `strip.go` | Per-file list of `isoAnnotation`s with their positions |
| `isoAnnotation` | `strip.go` | One annotation: byte offset, line, col, function name, enclosing block scope |
| `region` | `strip.go` | Byte range `[beg, endx)` representing a block scope |

### The four ownerstamps

| Ownerstamp | Meaning | Mutable | Sendable cross-goroutine |
|------------|---------|---------|--------------------------|
| `\iso` | Isolated, uniquely owned | Yes | Yes (move semantics) |
| `\mub` | Mutable borrow | Yes | No |
| `\rob` | Read-only borrow | No | No |
| `\imm` | Deeply immutable | No | Yes (freely shareable) |

All annotations use the `\` prefix so they cause a Go compiler error if they leak into output — fail-fast by design.

## Specification and Proofs

- `gown-spec.md` — full language specification (ownerstamps, type rules, coercions, borrowing, channel semantics)
- `theory-proof.md` — formal proof of the Race Freedom Theorem
- `Gown.lean` — Lean 4 formalization of the type system

## Dependencies

- `golang.org/x/tools` — AST parsing and package loading
- `4d63.com/tz` — timezone utilities for debug/logging timestamps
