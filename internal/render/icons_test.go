package render

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIconCacheAndFailure(t *testing.T) {
	g := &graph.Graph{Nodes: map[string]*graph.Node{"x": {ID: "x", Type: "aws_instance"}}}
	requests := 0
	status := http.StatusOK
	body := `<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.Host != "icons.terrastruct.com" {
			t.Fatal("unexpected host")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	cache := t.TempDir()
	for i := 0; i < 2; i++ {
		icons, warnings := iconSet(context.Background(), g, cache, client)
		if len(icons) != 1 || len(warnings) != 0 {
			t.Fatalf("icons %v warnings %v", icons, warnings)
		}
	}
	if requests != 1 {
		t.Fatalf("cache requested network %d times", requests)
	}
	status = http.StatusNotFound
	icons, warnings := iconSet(context.Background(), g, t.TempDir(), client)
	if len(icons) != 0 || len(warnings) != 1 {
		t.Fatal("fetch failure did not use plain shape")
	}
	status = http.StatusOK
	body = strings.Repeat("x", 512*1024+1)
	icons, warnings = iconSet(context.Background(), g, t.TempDir(), client)
	if len(icons) != 0 || len(warnings) != 1 {
		t.Fatal("oversized icon accepted")
	}
}
