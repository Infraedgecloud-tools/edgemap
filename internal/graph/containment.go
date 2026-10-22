package graph

import (
	"strings"
	"unicode"

	"github.com/Infraedgecloud-tools/edgemap/internal/catalog"
)

func (b *builder) group(id, label, kind, parent string) {
	if b.graph.Nodes[id] == nil {
		b.graph.Nodes[id] = &Node{ID: id, Label: label, Type: kind, Container: parent, Group: true}
	}
}

func (b *builder) containment() {
	original := b.graph.IDs()
	for _, id := range original {
		n := b.graph.Nodes[id]
		if !strings.HasPrefix(n.Type, "aws_") && n.Type != "module" {
			continue
		}
		region := "region:" + n.Region
		b.group(region, n.Region, "region", "")
		n.Container = region
		metadata, known := catalog.Lookup(n.Type)
		if metadata.Global {
			b.group("global", "Global", "global", "")
			n.Container = "global"
		} else if !known && strings.HasPrefix(n.Type, "aws_") && n.Type != "aws_vpc_module" {
			b.group(region+".other", "Other", "other", region)
			n.Container = region + ".other"
		}
		if n.Type == "aws_vpc" || n.Type == "aws_vpc_module" {
			n.Group = true
			for _, tier := range []string{"public", "private", "subnets"} {
				b.group(id+".tier."+tier, tier, "tier", id)
			}
		}
	}
	// Establish subnet tiers before placing resources that refer to them.
	public := b.internetSubnets()
	for _, id := range original {
		n := b.graph.Nodes[id]
		if n.Type != "aws_subnet" {
			continue
		}
		parents := b.references(n, "vpc_id")
		if len(parents) != 1 {
			b.graph.Warnings = append(b.graph.Warnings, id+": unresolved VPC")
			continue
		}
		parent := b.graph.Nodes[parents[0]]
		if parent == nil || !parent.Group {
			continue
		}
		n.Container = parent.ID + ".tier." + b.tier(n, public)
	}
	// A fixed point handles subnet-group and cluster indirection, without relying
	// on dependency edges as the source of containment.
	for pass := 0; pass < len(original); pass++ {
		changed := false
		for _, id := range original {
			n := b.graph.Nodes[id]
			if n.Type == "aws_subnet" || n.Group {
				continue
			}
			metadata, known := catalog.Lookup(n.Type)
			if !known || len(metadata.ParentAttributes) == 0 {
				continue
			}
			parents := b.references(n, metadata.ParentAttributes...)
			var containers []string
			for _, parent := range parents {
				target := b.graph.Nodes[parent]
				if target == nil {
					continue
				}
				candidate := target.Container
				if target.Type == "aws_vpc" || target.Type == "aws_vpc_module" || target.Type == "tier" {
					candidate = target.ID
				}
				// Only resolved network placement is useful evidence.
				c := b.graph.Nodes[candidate]
				if c != nil && (c.Type == "tier" || c.Type == "aws_vpc" || c.Type == "aws_vpc_module") {
					containers = append(containers, candidate)
				}
			}
			if len(containers) == 0 {
				continue
			}
			parent := b.commonContainer(containers)
			if parent != "" && parent != n.Container {
				n.Container = parent
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}

func (b *builder) prune() {
	// Empty tiers/regions add noise and would suggest nonexistent infrastructure.
	for pass := 0; pass < 3; pass++ {
		used := map[string]bool{}
		for _, edge := range b.graph.Edges {
			used[edge.From] = true
			used[edge.To] = true
		}
		for _, n := range b.graph.Nodes {
			used[n.Container] = true
		}
		for id, n := range b.graph.Nodes {
			if n.Group && !used[id] && (n.Type == "tier" || n.Type == "region" || n.Type == "other") {
				delete(b.graph.Nodes, id)
			}
		}
	}
}

func (b *builder) commonContainer(ids []string) string {
	candidate := ids[0]
	for candidate != "" {
		all := true
		for _, id := range ids[1:] {
			if id != candidate && !b.graph.Ancestor(id, candidate) {
				all = false
				break
			}
		}
		if all {
			return candidate
		}
		n := b.graph.Nodes[candidate]
		if n == nil {
			return ""
		}
		candidate = n.Container
	}
	return ""
}

// internetSubnets indexes subnets associated with a route table that has a
// default route to an internet gateway, inline or as a separate aws_route.
func (b *builder) internetSubnets() map[string]bool {
	internetTables := map[string]bool{}
	for _, id := range b.graph.IDs() {
		n := b.graph.Nodes[id]
		switch {
		case n.Type == "aws_route_table" && b.internetRoute(n):
			internetTables[id] = true
		case n.Type == "aws_route" && b.internetRoute(n):
			for _, table := range b.references(n, "route_table_id") {
				internetTables[table] = true
			}
		}
	}
	subnets := map[string]bool{}
	for _, id := range b.graph.IDs() {
		n := b.graph.Nodes[id]
		if n.Type != "aws_route_table_association" {
			continue
		}
		for _, table := range b.references(n, "route_table_id") {
			if internetTables[table] {
				for _, subnet := range b.references(n, "subnet_id") {
					subnets[subnet] = true
				}
			}
		}
	}
	return subnets
}

func (b *builder) tier(n *Node, internetSubnets map[string]bool) string {
	if n.Attributes["map_public_ip_on_launch"].Value == true {
		return "public"
	}
	// An IGW default route is stronger evidence than a private Name tag.
	if internetSubnets[n.ID] {
		return "public"
	}
	if tags, ok := n.Attributes["tags"].Value.(map[string]any); ok {
		name, _ := tags["Name"].(string)
		words := map[string]bool{}
		for _, word := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}) {
			words[word] = true
		}
		// A name that mentions both is ambiguous, not evidence for either tier.
		switch {
		case words["public"] && !words["private"]:
			return "public"
		case words["private"] && !words["public"]:
			return "private"
		}
	}
	return "subnets"
}

func (b *builder) internetRoute(n *Node) bool {
	for key, attr := range n.Attributes {
		if key != "gateway_id" && !strings.HasSuffix(key, ".gateway_id") {
			continue
		}
		prefix := strings.TrimSuffix(key, "gateway_id")
		cidr := textAttr(n.Attributes, prefix+"cidr_block")
		if cidr == "" {
			cidr = textAttr(n.Attributes, prefix+"destination_cidr_block")
		}
		ipv6 := textAttr(n.Attributes, prefix+"ipv6_cidr_block")
		if ipv6 == "" {
			ipv6 = textAttr(n.Attributes, prefix+"destination_ipv6_cidr_block")
		}
		if cidr != "0.0.0.0/0" && ipv6 != "::/0" {
			continue
		}
		for _, ref := range attr.References {
			for _, id := range b.resolve(n.Scope+ref, map[string]bool{}) {
				if target := b.graph.Nodes[id]; target != nil && target.Type == "aws_internet_gateway" {
					return true
				}
			}
		}
	}
	return false
}
