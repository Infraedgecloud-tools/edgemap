// Package render emits deterministic D2 and self-contained SVG diagrams.
package render

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2format"
)

type Options struct {
	Title  string
	Icons  map[string]string
	Colors map[string]string
	Legend string
}

func Emit(g *graph.Graph, options Options) (string, error) {
	var out strings.Builder
	out.WriteString("direction: down\n")
	if options.Title != "" {
		fmt.Fprintf(&out, "title: %s { shape: text; near: top-center }\n", quote(options.Title))
	}
	paths := map[string]string{}
	var path func(string, map[string]bool) (string, error)
	path = func(id string, seen map[string]bool) (string, error) {
		if value, ok := paths[id]; ok {
			return value, nil
		}
		if seen[id] {
			return "", fmt.Errorf("containment cycle at %s", id)
		}
		seen[id] = true
		n := g.Nodes[id]
		if n == nil {
			return "", fmt.Errorf("missing container %s", id)
		}
		sum := sha256.Sum256([]byte(id))
		value := fmt.Sprintf("n%x", sum[:8])
		if n.Container != "" {
			parent, err := path(n.Container, seen)
			if err != nil {
				return "", err
			}
			value = parent + "." + value
		}
		paths[id] = value
		return value, nil
	}
	for _, id := range g.IDs() {
		if _, err := path(id, map[string]bool{}); err != nil {
			return "", err
		}
	}
	for _, id := range g.IDs() {
		n := g.Nodes[id]
		label := n.Label
		if n.Type == "aws_vpc" || n.Type == "aws_vpc_module" {
			label = "VPC · " + label
		}
		if !n.Group {
			label += "\n" + n.Type
		}
		if len(n.Badge) > 0 {
			label += "\n" + strings.Join(n.Badge, " · ")
		}
		fmt.Fprintf(&out, "%s: %s {\n", paths[id], quote(label))
		if n.Group {
			out.WriteString("  style.fill: \"#F3F6FA\"\n  style.stroke: \"#8B9BB1\"\n  style.font-size: 20\n")
		} else {
			out.WriteString("  style.border-radius: 8\n  style.fill: \"#FFFFFF\"\n  style.font-size: 16\n")
		}
		if color := options.Colors[id]; color != "" {
			fmt.Fprintf(&out, "  style.stroke: %s\n  style.stroke-width: 3\n", quote(color))
		}
		if icon := options.Icons[n.Type]; icon != "" && !n.Group {
			fmt.Fprintf(&out, "  icon: %s\n", quote(icon))
		}
		out.WriteString("}\n")
	}
	for _, edge := range g.Edges {
		if paths[edge.From] == "" || paths[edge.To] == "" {
			return "", fmt.Errorf("edge has missing endpoint")
		}
		fmt.Fprintf(&out, "%s -> %s", paths[edge.From], paths[edge.To])
		if edge.Count > 1 {
			fmt.Fprintf(&out, ": %s", quote(fmt.Sprintf("%d refs", edge.Count)))
		}
		out.WriteString("\n")
	}
	legend := "Arrows: consumer → dependency; nesting replaces child → parent edges."
	if options.Legend != "" {
		legend += "\n" + options.Legend
	}
	fmt.Fprintf(&out, "legend: %s { shape: text; near: bottom-center }\n", quote(legend))
	return out.String(), nil
}

func quote(value string) string {
	return d2format.Format(&d2ast.DoubleQuotedString{Value: []d2ast.InterpolationBox{{String: &value}}})
}
