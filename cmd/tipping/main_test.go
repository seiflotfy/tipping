package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunStdIO(t *testing.T) {
	in := strings.NewReader("a x1 x2 b\na x2 b\na x3 b\na x4 b\nc x1 d\nc x2 d\nc x3 d\nc x4 d\n")
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := run(
		[]string{"-templates", "-masks"},
		in,
		&out,
		&errOut,
	)
	if err != nil {
		t.Fatalf("run returned error: %v\nstderr:\n%s", err, errOut.String())
	}

	var decoded output
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode output json: %v\nraw:\n%s", err, out.String())
	}
	if len(decoded.Clusters) != 8 {
		t.Fatalf("clusters length mismatch: got %d", len(decoded.Clusters))
	}
	if len(decoded.Templates) == 0 {
		t.Fatalf("expected templates in output")
	}
	if len(decoded.Masks) != 8 {
		t.Fatalf("expected masks for all messages, got %d", len(decoded.Masks))
	}
}

func TestRunInvalidThreshold(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := run([]string{"-threshold", "1.2"}, strings.NewReader("x\n"), &out, &errOut)
	if err == nil {
		t.Fatalf("expected invalid threshold error")
	}
}

func TestRunInvalidPatternSample(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	err := run([]string{"-pattern-sample", "0"}, strings.NewReader("x\n"), &out, &errOut)
	if err == nil {
		t.Fatalf("expected invalid pattern sample error")
	}
}
