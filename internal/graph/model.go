// Package graph builds a static, cloud-semantic architecture model.
package graph

import (
	"sort"

	"github.com/Infraedgecloud-tools/edgemap/internal/source"
)

type Node struct {
	ID         string
	Type       string
	Label      string
	Container  string
	Region     string
	Group      bool
	Data       bool
	Badge      []string
	Attributes map[string]source.Attribute
	Scope      string
}

type Edge struct {
	From, To string
	Count    int
}

type Graph struct {
	Nodes    map[string]*Node
	Edges    []Edge
	Warnings []string
}

func (g *Graph) IDs() []string {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (g *Graph) Ancestor(child, parent string) bool {
	for remaining := len(g.Nodes); remaining > 0; remaining-- {
		node := g.Nodes[child]
		if node == nil || node.Container == "" {
			return false
		}
		if node.Container == parent {
			return true
		}
		child = node.Container
	}
	return false
}
