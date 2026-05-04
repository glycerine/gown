package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	goformat "go/format"
	"go/token"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gown "github.com/glycerine/gown"
	"golang.org/x/tools/go/packages"
)

const recordTransactionCommand = "gownpls.recordTransaction"

type document struct {
	URI     string
	Path    string
	Version int
	Text    []byte
	Settled []byte
	Lines   *lineMap
}

type packageState struct {
	Generation int
	Timer      *time.Timer
	Analysis   *gown.GownAnalysis
	Err        error
}

type packageDiagnostic struct {
	Path    string
	Line    int
	Col     int
	Message string
}

type packageDiagnostics []packageDiagnostic

func (diags packageDiagnostics) Error() string {
	if len(diags) == 0 {
		return ""
	}
	if len(diags) == 1 {
		return diags[0].Message
	}
	return fmt.Sprintf("%s and %d more package errors", diags[0].Message, len(diags)-1)
}

type server struct {
	conn   *rpcConn
	stderr io.Writer

	mu       sync.Mutex
	root     string
	shutdown bool
	docs     map[string]*document
	pkgs     map[string]*packageState
	pending  map[string]*gown.PlannedAnnotationTransaction
	sem      chan struct{}
}

func newServer(in io.Reader, out io.Writer, stderr io.Writer) *server {
	return &server{
		conn:    newRPCConn(in, out),
		stderr:  stderr,
		docs:    make(map[string]*document),
		pkgs:    make(map[string]*packageState),
		pending: make(map[string]*gown.PlannedAnnotationTransaction),
		sem:     make(chan struct{}, 1),
	}
}

func (s *server) serve() error {
	for {
		msg, err := s.conn.read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg.Method == "exit" {
			return nil
		}
		if len(msg.ID) == 0 {
			s.handleNotification(msg)
			continue
		}
		s.handleRequest(msg)
	}
}

func (s *server) handleRequest(msg rpcMessage) {
	var result any
	var err error
	switch msg.Method {
	case "initialize":
		result, err = s.handleInitialize(msg.Params)
	case "shutdown":
		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()
		result = nil
	case "textDocument/formatting":
		result, err = s.handleFormatting(msg.Params)
	case "textDocument/documentSymbol":
		result, err = s.handleDocumentSymbol(msg.Params)
	case "textDocument/definition":
		result, err = s.handleDefinition(msg.Params)
	case "textDocument/codeAction":
		result, err = s.handleCodeAction(msg.Params)
	case "workspace/executeCommand":
		result, err = s.handleExecuteCommand(msg.Params)
	default:
		err = fmt.Errorf("method %s not implemented", msg.Method)
	}
	if err != nil {
		_ = s.conn.respondError(msg.ID, -32603, err.Error())
		return
	}
	_ = s.conn.respond(msg.ID, result)
}

func (s *server) handleNotification(msg rpcMessage) {
	switch msg.Method {
	case "initialized":
	case "textDocument/didOpen":
		var params didOpenTextDocumentParams
		if json.Unmarshal(msg.Params, &params) == nil {
			s.didOpen(params)
		}
	case "textDocument/didChange":
		var params didChangeTextDocumentParams
		if json.Unmarshal(msg.Params, &params) == nil {
			s.didChange(params)
		}
	case "textDocument/didSave":
		var params didSaveTextDocumentParams
		if json.Unmarshal(msg.Params, &params) == nil {
			s.didSave(params)
		}
	case "textDocument/didClose":
		var params didCloseTextDocumentParams
		if json.Unmarshal(msg.Params, &params) == nil {
			s.didClose(params)
		}
	case "$/cancelRequest":
		// Analysis jobs are generation-gated; stale results are dropped.
	}
}

func (s *server) handleInitialize(raw json.RawMessage) (initializeResult, error) {
	var params initializeParams
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &params)
	}
	root := params.RootPath
	if root == "" && params.RootURI != "" {
		if path, err := uriToPath(params.RootURI); err == nil {
			root = path
		}
	}
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	s.mu.Lock()
	s.root = root
	s.mu.Unlock()
	return initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync:           1,
			DefinitionProvider:         true,
			DocumentSymbolProvider:     true,
			DocumentFormattingProvider: true,
			CodeActionProvider:         true,
			ExecuteCommandProvider:     executeCommandProvider{Commands: []string{recordTransactionCommand}},
		},
		ServerInfo: serverInfo{Name: "gownpls", Version: "0.1.0"},
	}, nil
}

func (s *server) didOpen(params didOpenTextDocumentParams) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return
	}
	doc := &document{
		URI:     params.TextDocument.URI,
		Path:    path,
		Version: params.TextDocument.Version,
		Text:    []byte(params.TextDocument.Text),
		Settled: []byte(params.TextDocument.Text),
	}
	doc.Lines = newLineMap(doc.Text)
	s.mu.Lock()
	s.docs[path] = doc
	s.mu.Unlock()
	s.scheduleDiagnostics(packageDir(path))
}

func (s *server) didChange(params didChangeTextDocumentParams) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return
	}
	s.mu.Lock()
	doc := s.docs[path]
	if doc == nil {
		doc = &document{URI: params.TextDocument.URI, Path: path}
		s.docs[path] = doc
	}
	text := doc.Text
	lineMap := doc.Lines
	for _, change := range params.ContentChanges {
		if change.Range == nil {
			text = []byte(change.Text)
			lineMap = newLineMap(text)
			continue
		}
		if lineMap == nil {
			lineMap = newLineMap(text)
		}
		start := lineMap.offset(change.Range.Start)
		end := lineMap.offset(change.Range.End)
		next := make([]byte, 0, len(text)-(end-start)+len(change.Text))
		next = append(next, text[:start]...)
		next = append(next, change.Text...)
		next = append(next, text[end:]...)
		text = next
		lineMap = newLineMap(text)
	}
	doc.Text = text
	doc.Version = params.TextDocument.Version
	doc.Lines = newLineMap(text)
	s.mu.Unlock()
	s.scheduleDiagnostics(packageDir(path))
}

func (s *server) didSave(params didSaveTextDocumentParams) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return
	}
	s.mu.Lock()
	if doc := s.docs[path]; doc != nil {
		if params.Text != nil {
			doc.Text = []byte(*params.Text)
			doc.Lines = newLineMap(doc.Text)
		}
		doc.Settled = append(doc.Settled[:0], doc.Text...)
	}
	s.mu.Unlock()
	s.scheduleDiagnostics(packageDir(path))
}

func (s *server) didClose(params didCloseTextDocumentParams) {
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return
	}
	s.mu.Lock()
	delete(s.docs, path)
	s.mu.Unlock()
	_ = s.conn.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: params.TextDocument.URI})
}

func (s *server) scheduleDiagnostics(dir string) {
	if dir == "" {
		return
	}
	s.mu.Lock()
	state := s.pkgs[dir]
	if state == nil {
		state = &packageState{}
		s.pkgs[dir] = state
	}
	state.Generation++
	gen := state.Generation
	if state.Timer != nil {
		state.Timer.Stop()
	}
	state.Timer = time.AfterFunc(150*time.Millisecond, func() {
		s.runDiagnostics(dir, gen)
	})
	s.mu.Unlock()
}

func (s *server) runDiagnostics(dir string, gen int) {
	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	analysis, err := s.analyzeDir(dir)
	s.mu.Lock()
	state := s.pkgs[dir]
	if state == nil || state.Generation != gen {
		s.mu.Unlock()
		return
	}
	state.Analysis = analysis
	state.Err = err
	docs := s.docsInDirLocked(dir)
	s.mu.Unlock()

	diagsByURI := make(map[string][]diagnostic)
	if err != nil {
		diagsByURI = s.diagnosticsForError(dir, err, docs)
	}
	for _, doc := range docs {
		_ = s.conn.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
			URI:         doc.URI,
			Diagnostics: diagsByURI[doc.URI],
		})
	}
}

func (s *server) analyzeDir(dir string) (*gown.GownAnalysis, error) {
	if !s.dirHasGown(dir) {
		return s.analyzeGoDir(dir)
	}
	overlay := s.gownOverlayForDir(dir)
	gp := gown.NewGownPackage(dir)
	return gp.AnalyzeWithOptions(gown.CheckOptions{CheckOnly: true, GownOverlay: overlay})
}

func (s *server) analyzeGoDir(dir string) (*gown.GownAnalysis, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedName |
			packages.NeedImports | packages.NeedTypesSizes,
		Dir:     dir,
		Context: ctx,
		Overlay: s.goOverlayForDir(dir),
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", dir)
	}
	analysis := &gown.GownAnalysis{Path: dir, Package: pkgs[0]}
	if len(pkgs[0].Errors) > 0 {
		return analysis, packageDiagnosticsFromErrors(pkgs[0].Errors)
	}
	return analysis, nil
}

func (s *server) dirHasGown(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".gown") {
				return true
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for path := range s.docs {
		if packageDir(path) == dir && strings.HasSuffix(path, ".gown") {
			return true
		}
	}
	return false
}

func (s *server) docsInDirLocked(dir string) []*document {
	var docs []*document
	for _, doc := range s.docs {
		if packageDir(doc.Path) == dir {
			docs = append(docs, doc)
		}
	}
	return docs
}

func (s *server) gownOverlayForDir(dir string) map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	overlay := make(map[string][]byte)
	for path, doc := range s.docs {
		if packageDir(path) == dir && strings.HasSuffix(path, ".gown") {
			overlay[path] = append([]byte(nil), doc.Text...)
		}
	}
	return overlay
}

func (s *server) allOverlayForDir(dir string) map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	overlay := make(map[string][]byte)
	for path, doc := range s.docs {
		if packageDir(path) == dir {
			overlay[path] = append([]byte(nil), doc.Text...)
		}
	}
	return overlay
}

func (s *server) goOverlayForDir(dir string) map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	overlay := make(map[string][]byte)
	for path, doc := range s.docs {
		if packageDir(path) == dir && strings.HasSuffix(path, ".go") {
			overlay[path] = append([]byte(nil), doc.Text...)
		}
	}
	return overlay
}

func (s *server) diagnosticsForError(dir string, err error, docs []*document) map[string][]diagnostic {
	out := make(map[string][]diagnostic)
	var pkgDiags packageDiagnostics
	if errors.As(err, &pkgDiags) {
		for _, pkgDiag := range pkgDiags {
			path := normalizePath(dir, pkgDiag.Path)
			uri := pathToURI(path)
			doc := s.documentForPath(path)
			rng := lspRange{
				Start: position{Line: max(0, pkgDiag.Line-1), Character: max(0, pkgDiag.Col-1)},
				End:   position{Line: max(0, pkgDiag.Line-1), Character: max(1, pkgDiag.Col)},
			}
			if doc != nil {
				start := offsetFromLineCol(doc.Text, pkgDiag.Line, pkgDiag.Col)
				rng = doc.Lines.rangeForOffsets(start, start+1)
			}
			out[uri] = append(out[uri], diagnostic{
				Range:    rng,
				Severity: 1,
				Source:   "go",
				Message:  pkgDiag.Message,
			})
		}
		return out
	}
	var checkerErrs gown.CheckerErrors
	if errors.As(err, &checkerErrs) {
		for _, checkerErr := range checkerErrs {
			path := normalizePath(dir, checkerErr.Path)
			uri := pathToURI(path)
			doc := s.documentForPath(path)
			rng := lspRange{Start: position{}, End: position{Character: 1}}
			if doc != nil {
				start := checkerErr.Offset
				if start <= 0 && checkerErr.Line > 0 {
					start = offsetFromLineCol(doc.Text, checkerErr.Line, checkerErr.Col)
				}
				rng = doc.Lines.rangeForOffsets(start, start+1)
			} else if checkerErr.Line > 0 {
				rng = lspRange{
					Start: position{Line: checkerErr.Line - 1, Character: max(0, checkerErr.Col-1)},
					End:   position{Line: checkerErr.Line - 1, Character: max(1, checkerErr.Col)},
				}
			}
			out[uri] = append(out[uri], diagnostic{
				Range:    rng,
				Severity: 1,
				Code:     string(checkerErr.Code),
				Source:   "gown",
				Message:  checkerErr.Message,
			})
		}
		return out
	}
	msg := err.Error()
	for _, doc := range docs {
		out[doc.URI] = append(out[doc.URI], diagnostic{
			Range:    lspRange{Start: position{}, End: position{Character: 1}},
			Severity: 1,
			Source:   "gown",
			Message:  msg,
		})
	}
	return out
}

func (s *server) documentForPath(path string) *document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docs[path]
}

func (s *server) handleFormatting(raw json.RawMessage) ([]textEdit, error) {
	var params formattingParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	doc, err := s.documentOrDisk(path, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	var formatted []byte
	if strings.HasSuffix(path, ".gown") {
		formatted, err = gown.FormatGown(doc.Text)
	} else {
		formatted, err = goformat.Source(doc.Text)
	}
	if err != nil {
		return nil, err
	}
	return []textEdit{{
		Range:   doc.Lines.rangeForOffsets(0, len(doc.Text)),
		NewText: string(formatted),
	}}, nil
}

func (s *server) handleDocumentSymbol(raw json.RawMessage) ([]documentSymbol, error) {
	var params documentSymbolParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	analysis, err := s.analysisForRequest(packageDir(path))
	if err != nil && analysis == nil {
		return nil, err
	}
	if analysis == nil || analysis.Package == nil {
		return nil, nil
	}
	file := astFileForPath(analysis, path)
	if file == nil {
		return nil, nil
	}
	doc, _ := s.documentOrDisk(path, params.TextDocument.URI)
	var symbols []documentSymbol
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			kind := 12
			if d.Recv != nil {
				kind = 6
			}
			symbols = append(symbols, documentSymbol{
				Name:           d.Name.Name,
				Kind:           kind,
				Range:          nodeRange(analysis, doc, d),
				SelectionRange: nodeRange(analysis, doc, d.Name),
			})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					kind := 5
					if _, ok := s.Type.(*ast.StructType); ok {
						kind = 23
					}
					symbols = append(symbols, documentSymbol{
						Name:           s.Name.Name,
						Kind:           kind,
						Range:          nodeRange(analysis, doc, s),
						SelectionRange: nodeRange(analysis, doc, s.Name),
					})
				case *ast.ValueSpec:
					kind := 13
					if d.Tok == token.CONST {
						kind = 14
					}
					for _, name := range s.Names {
						symbols = append(symbols, documentSymbol{
							Name:           name.Name,
							Kind:           kind,
							Range:          nodeRange(analysis, doc, s),
							SelectionRange: nodeRange(analysis, doc, name),
						})
					}
				}
			}
		}
	}
	return symbols, nil
}

func (s *server) handleDefinition(raw json.RawMessage) (*location, error) {
	var params definitionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	analysis, err := s.analysisForRequest(packageDir(path))
	if err != nil && analysis == nil {
		return nil, err
	}
	if analysis == nil {
		return nil, nil
	}
	doc, err := s.documentOrDisk(path, params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	offset := doc.Lines.offset(params.Position)
	file := astFileForPath(analysis, path)
	if file == nil {
		return nil, nil
	}
	var ident *ast.Ident
	ast.Inspect(file, func(n ast.Node) bool {
		if ident != nil {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		pos := analysis.Package.Fset.Position(id.Pos()).Offset
		end := analysis.Package.Fset.Position(id.End()).Offset
		if pos <= offset && offset <= end {
			ident = id
			return false
		}
		return true
	})
	if ident == nil {
		return nil, nil
	}
	obj := analysis.Package.TypesInfo.Uses[ident]
	if obj == nil {
		obj = analysis.Package.TypesInfo.Defs[ident]
	}
	if obj == nil || !obj.Pos().IsValid() {
		return nil, nil
	}
	pos := analysis.Package.Fset.Position(obj.Pos())
	targetPath := gownSourcePath(pos.Filename)
	targetDoc, _ := s.documentOrDisk(targetPath, pathToURI(targetPath))
	length := len(obj.Name())
	rng := lspRange{
		Start: position{Line: max(0, pos.Line-1), Character: max(0, pos.Column-1)},
		End:   position{Line: max(0, pos.Line-1), Character: max(0, pos.Column-1+length)},
	}
	if targetDoc != nil {
		rng = targetDoc.Lines.rangeForOffsets(pos.Offset, pos.Offset+length)
	}
	return &location{URI: pathToURI(targetPath), Range: rng}, nil
}

func (s *server) handleCodeAction(raw json.RawMessage) ([]codeAction, error) {
	var params codeActionParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	path, err := uriToPath(params.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	doc := s.docs[path]
	if doc == nil || len(doc.Settled) == 0 {
		s.mu.Unlock()
		return nil, nil
	}
	before := append([]byte(nil), doc.Settled...)
	after := append([]byte(nil), doc.Text...)
	s.mu.Unlock()
	if string(before) == string(after) {
		return nil, nil
	}
	txn, err := gown.PlanAnnotationTransaction(gown.AnnotationTransactionOptions{
		Dir:     packageDir(path),
		Path:    path,
		Before:  before,
		After:   after,
		Overlay: s.allOverlayForDir(packageDir(path)),
	})
	if err != nil || (len(txn.Edits) == 0 && len(txn.Suggestions) == 0) {
		return nil, nil
	}
	s.mu.Lock()
	s.pending[txn.ID] = txn
	s.mu.Unlock()
	title := "Propagate implied Gown annotations"
	if len(txn.Edits) > 0 {
		title = fmt.Sprintf("%s (%d edit%s)", title, len(txn.Edits), plural(len(txn.Edits)))
	}
	edit := workspaceEdit{Changes: make(map[string][]textEdit)}
	for _, annEdit := range txn.Edits {
		uri := pathToURI(annEdit.Path)
		doc, _ := s.documentOrDisk(annEdit.Path, uri)
		if doc == nil {
			continue
		}
		edit.Changes[uri] = append(edit.Changes[uri], textEdit{
			Range:   doc.Lines.rangeForOffsets(annEdit.Start, annEdit.End),
			NewText: annEdit.NewText,
		})
	}
	return []codeAction{{
		Title: title,
		Kind:  "quickfix",
		Edit:  edit,
		Command: &command{
			Title:     "Record Gown annotation transaction",
			Command:   recordTransactionCommand,
			Arguments: []any{txn.ID},
		},
	}}, nil
}

func (s *server) handleExecuteCommand(raw json.RawMessage) (any, error) {
	var params executeCommandParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, err
	}
	if params.Command != recordTransactionCommand {
		return nil, fmt.Errorf("unknown command %s", params.Command)
	}
	if len(params.Arguments) == 0 {
		return nil, fmt.Errorf("missing transaction id")
	}
	var id string
	if err := json.Unmarshal(params.Arguments[0], &id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	txn := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if txn == nil {
		return nil, nil
	}
	txn.Status = "applied"
	if err := s.appendTransaction(txn); err != nil {
		return nil, err
	}
	s.mu.Lock()
	for _, edit := range txn.Edits {
		if doc := s.docs[edit.Path]; doc != nil {
			doc.Settled = nil
		}
	}
	s.mu.Unlock()
	return nil, nil
}

func (s *server) appendTransaction(txn *gown.PlannedAnnotationTransaction) error {
	root := s.root
	if root == "" {
		root = packageDir(txn.Root.Path)
	}
	dir := filepath.Join(root, ".gownpls")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "transactions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(txn)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *server) analysisForRequest(dir string) (*gown.GownAnalysis, error) {
	s.mu.Lock()
	if state := s.pkgs[dir]; state != nil && state.Analysis != nil {
		analysis, err := state.Analysis, state.Err
		s.mu.Unlock()
		return analysis, err
	}
	s.mu.Unlock()
	analysis, err := s.analyzeDir(dir)
	s.mu.Lock()
	state := s.pkgs[dir]
	if state == nil {
		state = &packageState{}
		s.pkgs[dir] = state
	}
	state.Analysis = analysis
	state.Err = err
	s.mu.Unlock()
	return analysis, err
}

func (s *server) documentOrDisk(path, uri string) (*document, error) {
	s.mu.Lock()
	if doc := s.docs[path]; doc != nil {
		s.mu.Unlock()
		return doc, nil
	}
	s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return &document{URI: uri, Path: path, Text: data, Lines: newLineMap(data)}, nil
}

func astFileForPath(analysis *gown.GownAnalysis, path string) *ast.File {
	if analysis == nil || analysis.Package == nil {
		return nil
	}
	for _, file := range analysis.Package.Syntax {
		pos := analysis.Package.Fset.Position(file.Pos())
		if gownSourcePath(pos.Filename) == path || pos.Filename == path {
			return file
		}
	}
	return nil
}

func nodeRange(analysis *gown.GownAnalysis, doc *document, node ast.Node) lspRange {
	if analysis == nil || analysis.Package == nil || node == nil {
		return lspRange{}
	}
	start := analysis.Package.Fset.Position(node.Pos())
	end := analysis.Package.Fset.Position(node.End())
	if doc != nil {
		return doc.Lines.rangeForOffsets(start.Offset, end.Offset)
	}
	return lspRange{
		Start: position{Line: max(0, start.Line-1), Character: max(0, start.Column-1)},
		End:   position{Line: max(0, end.Line-1), Character: max(0, end.Column-1)},
	}
}

func packageDir(path string) string {
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

func uriToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported URI scheme %q", u.Scheme)
	}
	path, err := url.PathUnescape(u.Path)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("empty file URI path")
	}
	return filepath.Clean(path), nil
}

func pathToURI(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return u.String()
}

func normalizePath(dir, path string) string {
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

func gownSourcePath(path string) string {
	if strings.HasSuffix(path, ".go") {
		return strings.TrimSuffix(path, ".go") + ".gown"
	}
	return path
}

func offsetFromLineCol(src []byte, line, col int) int {
	if line <= 0 {
		return 0
	}
	lines := newLineMap(src)
	return lines.offset(position{Line: line - 1, Character: max(0, col-1)})
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func packageDiagnosticsFromErrors(errs []packages.Error) packageDiagnostics {
	diags := make(packageDiagnostics, 0, len(errs))
	for _, pkgErr := range errs {
		path, line, col := parsePackagePosition(pkgErr.Pos)
		diags = append(diags, packageDiagnostic{
			Path:    path,
			Line:    line,
			Col:     col,
			Message: pkgErr.Msg,
		})
	}
	return diags
}

func parsePackagePosition(pos string) (string, int, int) {
	if pos == "" {
		return "", 1, 1
	}
	parts := strings.Split(pos, ":")
	if len(parts) < 3 {
		return pos, 1, 1
	}
	path := strings.Join(parts[:len(parts)-2], ":")
	line := atoiDefault(parts[len(parts)-2], 1)
	col := atoiDefault(parts[len(parts)-1], 1)
	return path, line, col
}

func atoiDefault(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return def
	}
	return n
}
