package render

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Infraedgecloud-tools/edgemap/internal/filter"
	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
	"github.com/Infraedgecloud-tools/edgemap/internal/parse"
)

func TestGolden(t *testing.T) {
	for _, fixture := range []string{"two-tier", "multi-az", "local-modules", "vpc-module", "for-each", "broken", "existing-vpc"} {
		t.Run(fixture, func(t *testing.T) {
			g, err := graph.Build(context.Background(), parse.Terraform{}, filepath.Join("../../testdata", fixture))
			if err != nil {
				t.Fatal(err)
			}
			g = filter.Apply(g, filter.Options{})
			got, err := Emit(g, Options{Title: "Terraform architecture"})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", fixture+".d2")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path) // #nosec G304 -- fixed test fixture names only.
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("D2 differs from %s; inspect and regenerate with UPDATE_GOLDEN=1", path)
			}
			again, err := Emit(g, Options{Title: "Terraform architecture"})
			if err != nil || again != got {
				t.Fatal("nondeterministic D2")
			}
		})
	}
}

func TestSVGLayouts(t *testing.T) {
	for _, layout := range []string{"elk", "dagre"} {
		t.Run(layout, func(t *testing.T) {
			svg, err := SVG(context.Background(), `a: "compute"; b: "data"; a -> b`, layout, "")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(svg), "<svg") {
				t.Fatal("not SVG")
			}
		})
	}
}

func TestIconValidation(t *testing.T) {
	for _, test := range []struct {
		svg   string
		valid bool
	}{
		{`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`, true},
		{`<svg><script>alert(1)</script></svg>`, false},
		{`<svg><image href="https://example.com/tracker"/></svg>`, false},
		{`<svg onload="alert(1)"/>`, false},
		{`<svg style="fill:u\72l(https://example.com/x)"/>`, false},
		{`<html>not an icon</html>`, false},
		{`<svg><style>.x {fill:url(https://example.com/x)}</style></svg>`, false},
		{`<svg><style>@import "https://example.com/x";</style></svg>`, false},
		{`<svg><style>.x {fill:url(#gradient)}</style></svg>`, true},
		{`<svg><defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs><rect fill="url(#g)"/></svg>`, true},
		{`<svg xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="#a"/></svg>`, true},
		{`<svg><style>.a{background:image-set("http://example.com/x" 1x)}</style></svg>`, false},
		{`<svg><style>.a{background:src("http://example.com/x")}</style></svg>`, false},
		{`<svg><video src="https://example.com/v.mp4"/></svg>`, false},
		{`<svg><iframe src="https://example.com"/></svg>`, false},
		{`<svg><a><set attributeName="href" to="javascript:alert(1)"/></a></svg>`, false},
		{`<svg><animate attributeName="href" values="http://example.com/"/></svg>`, false},
		{`<svg><filter><feImage href="#a"/></filter></svg>`, false},
		{`<svg><rect fill="url(https://example.com/x)"/></svg>`, false},
		{`<svg><rect class="x" data-src="https://example.com/x"/></svg>`, false},
	} {
		if validIcon([]byte(test.svg)) != test.valid {
			t.Errorf("validation mismatch %s", test.svg)
		}
	}
}

func TestD2TextIsLiteral(t *testing.T) {
	label := "${x} quotes \" slashes \\ and\nnewlines"
	g := &graph.Graph{Nodes: map[string]*graph.Node{"x": {ID: "x", Type: "example", Label: label}}}
	source, err := Emit(g, Options{Title: label, Legend: label})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SVG(context.Background(), source, "elk", ""); err != nil {
		t.Fatal(err)
	}
}

func TestBundledIcon(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "icon.svg")
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"><rect width="16" height="16" fill="red"/></svg>`), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &graph.Graph{Nodes: map[string]*graph.Node{"x": {ID: "x", Type: "test", Label: "test"}}}
	source, err := Emit(g, Options{Icons: map[string]string{"test": "icon.svg"}})
	if err != nil {
		t.Fatal(err)
	}
	svg, err := SVG(context.Background(), source, "elk", dir)
	if err != nil {
		t.Fatal(err)
	}
	// The same icon from another cache location yields byte-identical output.
	other := t.TempDir()
	if err := os.Link(path, filepath.Join(other, "icon.svg")); err != nil {
		t.Fatal(err)
	}
	again, err := SVG(context.Background(), source, "elk", other)
	if err != nil || !bytes.Equal(svg, again) {
		t.Fatalf("SVG depends on the icon directory: %v", err)
	}
	if !strings.Contains(string(svg), "data:image/svg+xml;base64,") {
		t.Fatal("icon not bundled")
	}
	if strings.Contains(string(svg), path) {
		t.Fatal("local path escaped into SVG")
	}
}
