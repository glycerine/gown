# gownpls LSP Plan And Progress

## Goal

Build `gownpls`, a fresh Go/Gown language server. Do not fork `gopls` for v1.
The first usable version should be stable, bounded, and conservative: Gown
diagnostics, basic Go navigation helpers, formatting, and previewable
annotation transactions triggered by ordinary editor edits.

## Agreed Product Shape

- Public binary name: `gownpls`.
- Transport: stdio LSP with a small in-repo JSON-RPC/LSP implementation using
  the Go standard library.
- Primary UX: when a user adds, deletes, or converts one capability annotation
  in the editor, `gownpls` detects the source delta and offers a code action:
  "Propagate implied Gown annotations".
- Propagation is preview/apply, not automatic rewrite-on-type.
- Each propagation is applied as one grouped workspace edit so editor undo can
  undo the visible edit set as a unit.
- Semantic provenance should be persisted in `.gownpls/transactions.jsonl`.
  This is local editor state, not source truth.
- V1 should avoid completion, rename, references, import management, and broad
  `gopls` parity.

## V1 Features

- Track open `.go` and `.gown` documents, versions, line maps, and LSP position
  conversion.
- Debounce edits, cancel stale analysis, cap concurrency, and avoid whole
  workspace SSA on every keystroke.
- Analyze only affected package directories.
- Publish diagnostics:
  - Go syntax/type errors from package loading.
  - Gown checker errors from the existing checker pipeline.
- Provide:
  - `textDocument/definition`
  - `textDocument/documentSymbol`
  - `textDocument/formatting`
  - `textDocument/codeAction` for annotation propagation.

## Annotation Transaction Design

Root edit detection compares the last settled source snapshot to the current
editor snapshot. V1 recognizes one root annotation delta at a time:

- add: no qualifier -> `\iso`, `\mub`, `\rob`, or `\imm`
- delete: qualifier -> none
- convert: one qualifier -> another qualifier

The transaction graph should cover these site types first:

- function and method parameters
- function and method results
- struct fields
- channel element types
- explicit local value specs

V1 should generate only hard mechanical edits:

- direct call argument/signature propagation
- return/result propagation
- channel send/channel element propagation
- struct field store/field declaration propagation
- `\iso` downgrade to `\mub` or `\rob` only when all uses uniquely require
  that borrow form

Ambiguous API-design choices are reported as suggestions/frontiers, not applied
as source edits.

Deleting a root annotation removes only implied annotations owned solely by
that root. User-authored annotations and annotations also implied by another
active root must remain.

Each persisted transaction should record:

- transaction id
- root edit
- implied edits
- reason edges
- before/after text
- source file hashes
- status: planned, applied, undone, or redone

## Progress So Far

Implemented in the working tree:

- `check.go`
  - Added `CheckOptions.GownOverlay`.
  - Added `GownAnalysis`.
  - Added `AnalyzeWithOptions`, a non-writing analysis path that can accept
    unsaved `.gown` overlays and always forces check-only behavior when overlays
    are present.
  - Existing `CheckWithOptions` now delegates to `AnalyzeWithOptions`.
  - Checker errors can now return a populated `GownAnalysis` object where
    package/type/side-table state is available.
- `annotations.go`
  - Added exported `GownSourceViews`.
  - Added exported `ClassifyGownSource` wrapper around `scanAndClassify` for
    LSP/editor use.
- `annotation_transaction.go`
  - Added an initial conservative annotation transaction planner.
  - It detects add/delete/convert deltas for one capability annotation.
  - It builds a typed site graph from `GownAnalysis`.
  - It follows mechanical edges through direct calls, sends, assignments,
    value specs, returns, channel element sites, and struct fields.
  - It emits planned text edits plus reason edges and source hashes.
  - Delete propagation currently reports that applied transaction provenance is
    needed; semantic undo/redo over `.gownpls/transactions.jsonl` is still
    pending.
- `.gitignore`
  - Added `.gownpls/`.
- `cmd/gownpls`
  - Added `gownpls` public binary entry point.
  - Added a small stdio JSON-RPC/LSP transport using the standard library.
  - Added LSP data types, URI/path conversion, document state, and UTF-16
    line/column mapping.
  - Added workspace/package state with debounced diagnostics and a one-at-a-time
    analysis semaphore.
  - Added `.gown` overlay analysis using `AnalyzeWithOptions`.
  - Added `.go` package analysis fallback for packages without `.gown` files.
  - Added diagnostics publication for Gown checker errors and package loader
    errors.
  - Added formatting:
    - `.gown` via `FormatGown`
    - `.go` via `go/format`
  - Added document symbols for package-level funcs, methods, types, vars, and
    consts.
  - Added basic go-to-definition using `types.Info`.
  - Added code action generation for previewable annotation propagation.
  - Added `workspace/executeCommand` support for recording applied annotation
    transactions to `.gownpls/transactions.jsonl`.
- Tests
  - Added focused `cmd/gownpls` tests for JSON-RPC header parsing/writing,
    UTF-16 position conversion, formatting, annotation code actions, and
    transaction recording.
  - Added focused core transaction test for direct call propagation.

Validation already run before pausing:

```text
go test ./...
ok  	github.com/glycerine/gown	59.417s
ok  	github.com/glycerine/gown/cmd/gown	0.685s
?   	github.com/glycerine/gown/cmd/gownfmt	[no test files]
?   	github.com/glycerine/gown/vectors/iso0	[no test files]
?   	github.com/glycerine/gown/vectors/iso1	[no test files]
```

Latest focused validation:

```text
go test ./cmd/gownpls
ok  	github.com/glycerine/gown/cmd/gownpls	0.202s

go test -run 'TestPlanAnnotationTransactionPropagatesDirectCall|TestScanAndClassifyCapabilityQualifiers' ./
ok  	github.com/glycerine/gown	0.165s
```

## Next Implementation Steps

1. Add explicit semantic undo/redo commands backed by
   `.gownpls/transactions.jsonl`.
2. Expand annotation transaction tests:
   - return/result propagation
   - channel send/channel element propagation
   - struct field propagation
   - explicit local value specs
   - convert `\iso` to `\rob`/`\mub`
   - overlapping transaction ownership/refcount behavior
3. Add LSP integration tests that exercise full initialize/open/change/code
   action/execute command message flow over the JSON-RPC transport.
4. Improve package error diagnostics for `.gown` packages so `packages.Load`
   syntax/type errors are mapped to precise LSP ranges instead of one package
   error message.
5. Add explicit cancellation contexts to Gown analysis once the checker API can
   accept a context.
6. Add a tiny editor setup note for running `gownpls`.

## Heat-Safe Work Notes

- Avoid repeated `go test ./...` while iterating on `gownpls`; it currently
  takes about one minute and warms the laptop.
- Prefer focused tests such as:
  - `go test -run TestName ./`
  - `go test ./cmd/gownpls`
- Do broad `go test ./...` only at milestone boundaries.
