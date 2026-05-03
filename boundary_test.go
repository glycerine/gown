package gown

import "testing"

const gownBoundarySource = `package main

type Config struct {
	Name string
}

var GlobalCfg *Config

func main() {
	ch := make(chan *Config)
	go func(c *Config) {
		ch <- c
	}(GlobalCfg)
	go func() {
		r := <-ch
		_ = r
	}()
}
`

func TestBoundaryDetection(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"boundary.gown": gownBoundarySource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	kindCounts := make(map[string]int)
	for _, bc := range gf.boundary {
		kindCounts[bc.kind]++
		t.Logf("boundary: kind=%s type=%s line=%d func=%s",
			bc.kind, bc.typeName, bc.line, bc.funcName)
	}

	if kindCounts["pkg-var"] < 1 {
		t.Error("expected at least 1 pkg-var boundary (GlobalCfg)")
	}
	if kindCounts["chan-send"] < 1 {
		t.Error("expected at least 1 chan-send boundary")
	}
	if kindCounts["chan-recv"] < 1 {
		t.Error("expected at least 1 chan-recv boundary")
	}
	if kindCounts["go-arg"] < 1 {
		t.Error("expected at least 1 go-arg boundary")
	}
	if kindCounts["go-capture"] < 1 {
		t.Error("expected at least 1 go-capture boundary (ch captured in second goroutine)")
	}
}

const gownCaptureSource = `package main

type Payload struct {
	Data []byte
}

func main() {
	p := &Payload{Data: []byte("hi")}
	go func() {
		_ = p.Data
	}()
}
`

func TestBoundaryCaptureOnly(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"capture.gown": gownCaptureSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	found := false
	for _, bc := range gf.boundary {
		t.Logf("boundary: kind=%s type=%s line=%d", bc.kind, bc.typeName, bc.line)
		if bc.kind == "go-capture" {
			found = true
		}
	}
	if !found {
		t.Error("expected go-capture boundary for p")
	}
}
