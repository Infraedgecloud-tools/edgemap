// Package plan reads only resource addresses, types and action lists.
package plan

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
)

type Change struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Change  struct {
		Actions []string `json:"actions"`
	} `json:"change"`
}

func Read(reader io.Reader) ([]Change, error) {
	// Unknown fields, including before/after, are skipped by encoding/json.
	// No generic map or RawMessage can retain their values.
	limited := &io.LimitedReader{R: reader, N: 16<<20 + 1}
	decoder := json.NewDecoder(limited)
	var document struct {
		Changes []Change `json:"resource_changes"`
	}
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid plan JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("plan contains trailing data")
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("plan exceeds 16 MiB limit")
	}
	for _, change := range document.Changes {
		if change.Address == "" || change.Type == "" {
			return nil, fmt.Errorf("plan change missing address or type")
		}
		if len(change.Change.Actions) == 0 {
			return nil, fmt.Errorf("plan change missing actions")
		}
		for _, action := range change.Change.Actions {
			if strings.Contains(action, ",") {
				return nil, fmt.Errorf("unsupported plan action")
			}
		}
		switch strings.Join(change.Change.Actions, ",") {
		case "create", "update", "delete", "no-op", "read", "forget", "delete,create", "create,delete", "create,forget":
		default:
			return nil, fmt.Errorf("unsupported plan action sequence")
		}
	}
	return document.Changes, nil
}

// Address removes instance keys while respecting escaped quoted strings.
func Address(input string) (string, error) {
	var out strings.Builder
	depth := 0
	quoted := false
	escaped := false
	for _, r := range input {
		if depth > 0 {
			if quoted {
				if escaped {
					escaped = false
				} else if r == '\\' {
					escaped = true
				} else if r == '"' {
					quoted = false
				}
				continue
			}
			switch r {
			case '"':
				quoted = true
			case '[':
				depth++
			case ']':
				depth--
			}
			continue
		}
		if r == '[' {
			depth++
			continue
		}
		if r == ']' {
			return "", fmt.Errorf("invalid plan address")
		}
		out.WriteRune(r)
	}
	if depth != 0 || quoted {
		return "", fmt.Errorf("invalid plan address")
	}
	return out.String(), nil
}

type counts struct{ total, after, create, update, delete, unchanged, forget int }

// Apply adds action badges and tombstones for resources absent from configuration.
func Apply(g *graph.Graph, changes []Change) (map[string]string, error) {
	grouped := map[string]*counts{}
	for _, change := range changes {
		id, err := Address(change.Address)
		if err != nil {
			return nil, err
		}
		target := id
		if g.Nodes[target] == nil {
			for candidate := id; strings.Contains(candidate, "."); {
				candidate = candidate[:strings.LastIndex(candidate, ".")]
				if node := g.Nodes[candidate]; node != nil && (node.Type == "module" || node.Type == "aws_vpc_module") {
					target = candidate
					break
				}
			}
		}
		if g.Nodes[target] == nil {
			if g.Nodes["plan-only"] == nil {
				g.Nodes["plan-only"] = &graph.Node{ID: "plan-only", Type: "plan", Label: "Plan-only · placement unknown", Group: true}
			}
			g.Nodes[target] = &graph.Node{ID: target, Type: change.Type, Label: target, Container: "plan-only", Data: strings.HasPrefix(target, "data.") || strings.Contains(target, ".data.")}
		}
		if len(g.Nodes) > 10000 {
			return nil, fmt.Errorf("plan exceeds 10000 graph nodes")
		}
		c := grouped[target]
		if c == nil {
			c = &counts{}
			grouped[target] = c
		}
		c.total++
		removedOnly := len(change.Change.Actions) == 1 && (change.Change.Actions[0] == "delete" || change.Change.Actions[0] == "forget")
		if !removedOnly {
			c.after++
		}
		for _, action := range change.Change.Actions {
			switch action {
			case "create":
				c.create++
			case "update":
				c.update++
			case "delete":
				c.delete++
			case "forget":
				c.forget++
			case "no-op", "read":
				c.unchanged++
			}
		}
	}
	colors := map[string]string{}
	ids := make([]string, 0, len(grouped))
	for id := range grouped {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := grouped[id]
		n := g.Nodes[id]
		kept := n.Badge[:0]
		for _, badge := range n.Badge {
			if !strings.HasPrefix(badge, "×") {
				kept = append(kept, badge)
			}
		}
		n.Badge = kept
		countLabel := fmt.Sprintf("×%d after plan", c.after)
		if n.Type == "module" || n.Type == "aws_vpc_module" {
			countLabel = fmt.Sprintf("%d plan resources", c.total)
		}
		n.Badge = append(n.Badge, summary(countLabel, c))
		if c.forget > 0 {
			n.Badge = append(n.Badge, fmt.Sprintf("forget %d (leave infrastructure)", c.forget))
		}
		switch {
		case c.delete > 0:
			colors[id] = "#C0392B"
		case c.update > 0:
			colors[id] = "#B76E00"
		case c.create > 0:
			colors[id] = "#16803C"
		case c.forget > 0:
			colors[id] = "#64748B"
		}
	}
	return colors, nil
}

// summary lists only the actions that occur. An all-unchanged resource shows
// just its count, so the changes stand out.
func summary(countLabel string, c *counts) string {
	parts := []string{countLabel}
	for _, action := range []struct {
		name  string
		count int
	}{{"create", c.create}, {"change", c.update}, {"destroy", c.delete}} {
		if action.count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", action.name, action.count))
		}
	}
	if c.unchanged > 0 && len(parts) > 1 {
		parts = append(parts, fmt.Sprintf("unchanged %d", c.unchanged))
	}
	return strings.Join(parts, " · ")
}

const Legend = "Green: create · amber: change · red: destroy (priority: destroy > change > create). Counts include replacements in both create and destroy. Grey: forget (leave infrastructure)."
