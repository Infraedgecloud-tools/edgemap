package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRenderNoIconsWritesBothFormatsWithoutNetwork(t *testing.T) {
	var calls atomic.Int64
	replaceTransport(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, fmt.Errorf("unexpected network request to %s", req.URL.Host)
	})
	output := filepath.Join(t.TempDir(), "architecture.svg")
	_, stderr, err := executeRender("render", "../../testdata/two-tier", "--no-icons", "--out", output)
	if err != nil {
		t.Fatalf("render failed: %v\n%s", err, stderr)
	}
	if calls.Load() != 0 {
		t.Fatalf("--no-icons made %d network requests", calls.Load())
	}
	assertSVGFile(t, output)
	d2, err := os.ReadFile(strings.TrimSuffix(output, ".svg") + ".d2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Terraform architecture", "aws_instance", "aws_db_instance", "consumer → dependency"} {
		if !bytes.Contains(d2, []byte(want)) {
			t.Errorf("D2 missing %q", want)
		}
	}
	if bytes.Contains(d2, []byte("icon:")) {
		t.Error("--no-icons emitted an icon")
	}
}

func TestRenderRejectsInvalidOptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"layout", []string{"--layout", "unknown"}, "layout must be elk or dagre"},
		{"format", []string{"--format", "png"}, "format must be svg or d2"},
		{"extension", []string{"--out", "diagram.png"}, "output extension must match"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"render", "../../testdata/two-tier", "--no-icons"}, tc.args...)
			_, _, err := executeRender(args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRenderInvalidIconsFallBackToPlainShapes(t *testing.T) {
	// Linux honors XDG_CACHE_HOME. Keep the test independent of the user's
	// cache, without changing HOME or allowing any real network request.
	if runtime.GOOS != "linux" {
		t.Skip("isolated XDG icon cache test requires Linux")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	var calls atomic.Int64
	replaceTransport(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Scheme != "https" || req.URL.Host != "icons.terrastruct.com" {
			return nil, fmt.Errorf("unexpected icon origin %s", req.URL.Host)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
			Body:       io.NopCloser(strings.NewReader(`<svg><script>invalid icon</script></svg>`)),
			Request:    req,
		}, nil
	})
	output := filepath.Join(t.TempDir(), "architecture.svg")
	_, stderr, err := executeRender("render", "../../testdata/two-tier", "--out", output)
	if err != nil {
		t.Fatalf("invalid icons prevented render: %v\n%s", err, stderr)
	}
	if calls.Load() == 0 {
		t.Fatal("test did not exercise icon requests")
	}
	if !strings.Contains(stderr, "unavailable; using plain shape") {
		t.Fatalf("missing fallback warning: %s", stderr)
	}
	assertSVGFile(t, output)
	d2, err := os.ReadFile(strings.TrimSuffix(output, ".svg") + ".d2")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(d2, []byte("icon:")) || bytes.Contains(d2, []byte("invalid icon")) {
		t.Error("invalid icon propagated to D2")
	}
}

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func replaceTransport(t *testing.T, roundTrip testTransport) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = roundTrip
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func executeRender(args ...string) (string, string, error) {
	cmd := newCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func assertSVGFile(t *testing.T, filename string) {
	t.Helper()
	data, err := os.ReadFile(filename) // #nosec G304 -- test-generated output in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	var root struct{ XMLName xml.Name }
	if err := xml.Unmarshal(data, &root); err != nil {
		t.Fatalf("invalid SVG XML: %v", err)
	}
	if root.XMLName.Local != "svg" {
		t.Fatalf("root element = %q", root.XMLName.Local)
	}
}

func TestRenderHiddenDataVPC(t *testing.T) {
	dir := t.TempDir()
	config := `provider "aws" { region = "eu-west-2" }
 data "aws_vpc" "existing" { id = "vpc-example" }
 resource "aws_subnet" "app" {
 vpc_id = data.aws_vpc.existing.id
 cidr_block = "10.0.1.0/24"
 }`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "diagram.svg")
	if _, _, err := executeRender("render", dir, "--no-icons", "--out", output); err != nil {
		t.Fatal(err)
	}
	assertSVGFile(t, output)
}
