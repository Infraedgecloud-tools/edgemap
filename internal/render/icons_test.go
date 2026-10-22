package render

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestIconFetchStopsAfterTransportFailure(t *testing.T) {
	g := &graph.Graph{Nodes: map[string]*graph.Node{
		"a": {ID: "a", Type: "aws_instance"}, "b": {ID: "b", Type: "aws_lb"},
		"c": {ID: "c", Type: "aws_db_instance"}, "d": {ID: "d", Type: "aws_s3_bucket"},
	}}
	requests := 0
	client := &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("network unreachable")
	})}
	icons, warnings := iconSet(context.Background(), g, t.TempDir(), client)
	if requests != 1 || len(icons) != 0 || len(warnings) != 1 {
		t.Fatalf("requests %d icons %v warnings %v", requests, icons, warnings)
	}
}

// Icons already in the local cache came from the real catalog host; they must
// stay valid under the allowlist. Skipped when nothing has been cached.
func TestCachedCatalogIconsStayValid(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip("no user cache")
	}
	files, _ := filepath.Glob(filepath.Join(cache, "edgemap", "*.svg"))
	if len(files) == 0 {
		t.Skip("no cached icons")
	}
	for _, file := range files {
		data, err := os.ReadFile(file) // #nosec G304 -- test reads the tool's own cache.
		if err != nil {
			t.Fatal(err)
		}
		if !validIcon(data) {
			t.Errorf("cached catalog icon rejected: %s", file)
		}
	}
}
