package main

import "encoding/json"

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version,omitempty"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId,omitempty"`
	Version    int    `json:"version,omitempty"`
	Text       string `json:"text"`
}

type didOpenTextDocumentParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type textDocumentContentChangeEvent struct {
	Range *lspRange `json:"range,omitempty"`
	Text  string    `json:"text"`
}

type didChangeTextDocumentParams struct {
	TextDocument   versionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent `json:"contentChanges"`
}

type didSaveTextDocumentParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

type didCloseTextDocumentParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type formattingParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type documentSymbolParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type definitionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Position     position               `json:"position"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        lspRange               `json:"range"`
	Context      codeActionContext      `json:"context"`
}

type codeActionContext struct {
	Diagnostics []diagnostic `json:"diagnostics,omitempty"`
}

type executeCommandParams struct {
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments,omitempty"`
}

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity,omitempty"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type textEdit struct {
	Range   lspRange `json:"range"`
	NewText string   `json:"newText"`
}

type workspaceEdit struct {
	Changes map[string][]textEdit `json:"changes"`
}

type codeAction struct {
	Title       string        `json:"title"`
	Kind        string        `json:"kind,omitempty"`
	Diagnostics []diagnostic  `json:"diagnostics,omitempty"`
	Edit        workspaceEdit `json:"edit,omitempty"`
	Command     *command      `json:"command,omitempty"`
}

type command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments,omitempty"`
}

type location struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

type documentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          lspRange         `json:"range"`
	SelectionRange lspRange         `json:"selectionRange"`
	Children       []documentSymbol `json:"children,omitempty"`
}

type initializeParams struct {
	RootURI  string `json:"rootUri,omitempty"`
	RootPath string `json:"rootPath,omitempty"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type serverCapabilities struct {
	TextDocumentSync           int                    `json:"textDocumentSync"`
	DefinitionProvider         bool                   `json:"definitionProvider"`
	DocumentSymbolProvider     bool                   `json:"documentSymbolProvider"`
	DocumentFormattingProvider bool                   `json:"documentFormattingProvider"`
	CodeActionProvider         bool                   `json:"codeActionProvider"`
	ExecuteCommandProvider     executeCommandProvider `json:"executeCommandProvider"`
}

type executeCommandProvider struct {
	Commands []string `json:"commands"`
}
