// Package filter removes display noise without changing the source graph.
package filter

import (
	"sort"
	"strings"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
)

type Options struct{ ShowIAM, ShowData, ShowSG bool }

func Apply(input *graph.Graph, options Options) *graph.Graph {
	result := &graph.Graph{Nodes: map[string]*graph.Node{}, Warnings: append([]string(nil), input.Warnings...)}
	hidden := map[string]bool{}
	for id, node := range input.Nodes {
		// A data-source VPC is still the real container of the resources placed in it;
		// it is hidden only when nothing visible remains inside (pruned below).
		hidden[id] = (node.Data && !node.Group && !options.ShowData) || (iam(node.Type) && !options.ShowIAM) || (sg(node.Type) && !options.ShowSG)
	}
	outgoing := map[string][]string{}
	for _, edge := range input.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
	}
	for _, id := range input.IDs() {
		if hidden[id] {
			continue
		}
		original := input.Nodes[id]
		node := *original
		node.Badge = append([]string(nil), original.Badge...)
		for hidden[node.Container] {
			parent := input.Nodes[node.Container]
			if parent == nil {
				node.Container = ""
				break
			}
			node.Container = parent.Container
		}
		labels := map[string]bool{}
		seen := map[string]bool{}
		var collect func(string)
		collect = func(target string) {
			if seen[target] || !hidden[target] {
				return
			}
			seen[target] = true
			n := input.Nodes[target]
			if n == nil {
				return
			}
			if iam(n.Type) {
				labels["IAM: "+n.Label] = true
			}
			if sg(n.Type) {
				labels["SG: "+n.Label] = true
			}
			for _, next := range outgoing[target] {
				collect(next)
			}
		}
		for _, target := range outgoing[id] {
			collect(target)
		}
		badges := make([]string, 0, len(labels))
		for label := range labels {
			badges = append(badges, label)
		}
		sort.Strings(badges)
		node.Badge = append(node.Badge, badges...)
		result.Nodes[id] = &node
	}
	for _, edge := range input.Edges {
		if !hidden[edge.From] && !hidden[edge.To] {
			result.Edges = append(result.Edges, edge)
		}
	}
	// Hide empty synthetic and data-source containers, but preserve real VPCs.
	// A data-source VPC is kept only for what it contains, not for edges to it.
	for {
		used, contains := map[string]bool{}, map[string]bool{}
		for _, n := range result.Nodes {
			used[n.Container] = true
			contains[n.Container] = true
		}
		for _, e := range result.Edges {
			used[e.From] = true
			used[e.To] = true
		}
		removed := false
		for id, n := range result.Nodes {
			synthetic := n.Type == "region" || n.Type == "other" || n.Type == "tier" || n.Type == "global"
			if n.Group && (synthetic && !used[id] || n.Data && !options.ShowData && !contains[id]) {
				delete(result.Nodes, id)
				removed = true
			}
		}
		if !removed {
			break
		}
	}
	edges := result.Edges[:0]
	for _, e := range result.Edges {
		if result.Nodes[e.From] != nil && result.Nodes[e.To] != nil {
			edges = append(edges, e)
		}
	}
	result.Edges = edges
	return result
}

func iam(kind string) bool { return strings.HasPrefix(kind, "aws_iam_") }
func sg(kind string) bool {
	return kind == "aws_security_group" || strings.HasPrefix(kind, "aws_security_group_") || strings.HasPrefix(kind, "aws_vpc_security_group_")
}
