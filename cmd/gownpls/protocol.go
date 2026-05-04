package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcConn struct {
	r  *bufio.Reader
	w  io.Writer
	mu sync.Mutex
}

func newRPCConn(in io.Reader, out io.Writer) *rpcConn {
	return &rpcConn{r: bufio.NewReader(in), w: out}
}

func (conn *rpcConn) read() (rpcMessage, error) {
	length := -1
	for {
		line, err := conn.r.ReadString('\n')
		if err != nil {
			return rpcMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return rpcMessage{}, fmt.Errorf("malformed LSP header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return rpcMessage{}, fmt.Errorf("invalid Content-Length %q", value)
			}
			length = n
		}
	}
	if length < 0 {
		return rpcMessage{}, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(conn.r, body); err != nil {
		return rpcMessage{}, err
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return rpcMessage{}, err
	}
	return msg, nil
}

func (conn *rpcConn) respond(id json.RawMessage, result any) error {
	return conn.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (conn *rpcConn) respondError(id json.RawMessage, code int, message string) error {
	return conn.write(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	})
}

func (conn *rpcConn) notify(method string, params any) error {
	return conn.write(rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
}

func (conn *rpcConn) write(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", len(body))
	buf.Write(body)

	conn.mu.Lock()
	defer conn.mu.Unlock()
	_, err = conn.w.Write(buf.Bytes())
	return err
}
