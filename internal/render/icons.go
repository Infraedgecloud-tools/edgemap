package render

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Infraedgecloud-tools/edgemap/internal/catalog"
	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
	"github.com/Infraedgecloud-tools/edgemap/internal/source"
)

func Icons(ctx context.Context, g *graph.Graph) (map[string]string, []string) {
	icons := map[string]string{}
	cache, err := os.UserCacheDir()
	if err != nil {
		return icons, []string{"icons: no user cache; using plain shapes"}
	}
	cache = filepath.Join(cache, "edgemap")
	if err := os.MkdirAll(cache, 0o700); err != nil {
		return icons, []string{"icons: cannot create cache; using plain shapes"}
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, _ []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != "icons.terrastruct.com" {
			return fmt.Errorf("icon redirect rejected")
		}
		return nil
	}}
	return iconSet(ctx, g, cache, client)
}

func iconSet(ctx context.Context, g *graph.Graph, cache string, client *http.Client) (map[string]string, []string) {
	icons := map[string]string{}
	var warnings []string
	seen := map[string]bool{}
	for _, id := range g.IDs() {
		n := g.Nodes[id]
		item, _ := catalog.Lookup(n.Type)
		if item.Icon == "" || seen[n.Type] {
			continue
		}
		seen[n.Type] = true
		hash := sha256.Sum256([]byte(item.Icon))
		path := filepath.Join(cache, fmt.Sprintf("%x.svg", hash[:16]))
		if data, err := source.ReadRegularFile(path); err == nil && validIcon(data) {
			icons[n.Type] = path
			continue
		}
		if err := fetchIcon(ctx, client, "https://icons.terrastruct.com/aws/"+item.Icon, path); err != nil {
			warnings = append(warnings, "icons: "+n.Type+" unavailable; using plain shape")
			continue
		}
		icons[n.Type] = path
	}
	return icons, warnings
}

func fetchIcon(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("icon HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 512*1024+1))
	if err != nil {
		return err
	}
	if !validIcon(data) {
		return fmt.Errorf("invalid icon")
	}
	file, err := os.CreateTemp(filepath.Dir(path), "icon-*.svg")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func validIcon(data []byte) bool {
	if len(data) > 512*1024 {
		return false
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	root := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return root
		}
		if err != nil {
			return false
		}
		if element, ok := token.(xml.StartElement); ok {
			name := strings.ToLower(element.Name.Local)
			if !root {
				if name != "svg" {
					return false
				}
				root = true
			}
			if name == "script" || name == "foreignobject" || name == "image" {
				return false
			}
			for _, attr := range element.Attr {
				key := strings.ToLower(attr.Name.Local)
				value := strings.TrimSpace(attr.Value)
				if strings.HasPrefix(key, "on") || (key == "href" && !strings.HasPrefix(value, "#")) || strings.Contains(strings.ToLower(value), "url(") {
					return false
				}
			}
		}
	}
}
