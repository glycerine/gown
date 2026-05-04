package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

func TestRPCReadMessageParsesContentLength(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	input := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)))
	input = append(input, body...)
	conn := newRPCConn(bytes.NewReader(input), &bytes.Buffer{})

	msg, err := conn.read()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Method != "initialize" {
		t.Fatalf("method = %q, want initialize", msg.Method)
	}
	if string(msg.ID) != "1" {
		t.Fatalf("id = %s, want 1", msg.ID)
	}
}

func TestRPCWriteResponseUsesHeaderAndJSONBody(t *testing.T) {
	var out bytes.Buffer
	conn := newRPCConn(bytes.NewReader(nil), &out)
	if err := conn.respond(json.RawMessage(`"abc"`), map[string]bool{"ok": true}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !bytes.HasPrefix(out.Bytes(), []byte("Content-Length: ")) {
		t.Fatalf("missing Content-Length header in %q", got)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"id":"abc"`)) || !bytes.Contains(out.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("unexpected response %q", got)
	}
}
