package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Infraedgecloud-tools/edgemap/internal/source"
)

type builder struct {
	expansions map[string][]string
	moduleDeps map[string][]string
	graph      *Graph
	source     source.Source
	outputs    map[string][]string
	inputs     map[string][]string
	modules    map[string]string
	active     map[string]bool
	order      []string
	loads      int
}

// Static limits keep hostile or generated configuration from exhausting memory or time.
const (
	maxDepth       = 32
	maxNodes       = 10000
	maxModuleLoads = 10000
)

// errLimit marks a configuration that exceeds a static limit. Unlike other module
// errors it aborts the whole build instead of becoming a warning.
var errLimit = errors.New("static analysis limit exceeded")

func Build(ctx context.Context, input source.Source, dir string) (*Graph, error) {
	b := builder{expansions: map[string][]string{}, moduleDeps: map[string][]string{}, graph: &Graph{Nodes: map[string]*Node{}}, source: input, outputs: map[string][]string{}, inputs: map[string][]string{}, modules: map[string]string{}, active: map[string]bool{}}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	// Installed modules are optional; never fetch or initialise them.
	path := filepath.Join(root, ".terraform/modules/modules.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = ""
	}
	data, err := source.ReadRegularFile(path)
	if err == nil {
		var manifest struct{ Modules []struct{ Key, Dir string } }
		if err = json.Unmarshal(data, &manifest); err != nil {
			b.graph.Warnings = append(b.graph.Warnings, "invalid installed-module manifest")
		} else {
			for _, m := range manifest.Modules {
				if m.Key != "" {
					p := m.Dir
					if !filepath.IsAbs(p) {
						p = filepath.Join(root, p)
					}
					b.modules[m.Key] = p
				}
			}
		}
	} else if path != "" {
		b.graph.Warnings = append(b.graph.Warnings, "cannot read installed-module manifest")
	}
	if err := b.load(ctx, root, "", map[string]string{"aws": "unknown"}, 0); err != nil {
		return nil, err
	}
	b.containment()
	b.edges()
	b.prune()
	sort.Strings(b.graph.Warnings)
	return b.graph, nil
}

func (b *builder) load(ctx context.Context, dir, scope string, inherited map[string]string, depth int) error {
	if depth > maxDepth {
		return fmt.Errorf("module nesting exceeds %d levels", maxDepth)
	}
	if b.loads++; b.loads > maxModuleLoads {
		return fmt.Errorf("%w: more than %d module calls expanded", errLimit, maxModuleLoads)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("resolve module directory: %w", err)
	}
	if b.active[real] {
		b.graph.Warnings = append(b.graph.Warnings, scope+": module directory cycle")
		return nil
	}
	b.active[real] = true
	defer delete(b.active, real)
	result, err := b.source.Load(ctx, real)
	if err != nil {
		return err
	}
	for _, warning := range result.Warnings {
		b.graph.Warnings = append(b.graph.Warnings, scope+warning)
	}
	regions := map[string]string{}
	for key, value := range inherited {
		regions[key] = value
	}
	locals := map[string]source.Attribute{}
	for _, block := range result.Blocks {
		if block.Kind == "locals" {
			for name, attr := range block.Attributes {
				if attr.Literal {
					locals["local."+name] = attr
				}
			}
		}
	}
	for _, block := range result.Blocks {
		if block.Kind == "provider" && block.Labels[0] == "aws" {
			alias := textAttr(block.Attributes, "alias")
			key := "aws"
			if alias != "" {
				key += "." + alias
			}
			attr := resolveLiteral(block.Attributes["region"], locals)
			region, ok := attr.Value.(string)
			if !ok || !attr.Literal {
				region = "unknown"
				if attr.Raw != "" {
					region = "region: " + attr.Raw
				}
			}
			regions[key] = region
		}
	}
	for _, block := range result.Blocks {
		for name, attr := range block.Attributes {
			block.Attributes[name] = resolveLiteral(attr, locals)
		}
		switch block.Kind {
		case "output":
			b.outputs[scope+"output."+block.Labels[0]] = scopedRefs(scope, block.Attributes["value"].References)
		case "resource", "data":
			id := scope + strings.Join(block.Labels, ".")
			if block.Kind == "data" {
				id = scope + "data." + strings.Join(block.Labels, ".")
			}
			provider := block.Attributes["provider"].Raw
			if refs := block.Attributes["provider"].References; len(refs) == 1 {
				provider = refs[0]
			}
			if provider == "" {
				provider = "aws"
			}
			region := regions[provider]
			if region == "" {
				region = "unknown"
				b.graph.Warnings = append(b.graph.Warnings, id+": unresolved provider region")
			}
			n := &Node{ID: id, Type: block.Labels[0], Label: block.Labels[1], Data: block.Kind == "data", Region: region, Scope: scope, Attributes: block.Attributes}
			if count, ok := block.Attributes["count"]; ok {
				badge := "×?"
				if v, ok := count.Value.(float64); ok && count.Literal {
					badge = fmt.Sprintf("×%.0f", v)
				}
				n.Badge = append(n.Badge, badge)
			}
			if _, ok := block.Attributes["for_each"]; ok {
				n.Badge = append(n.Badge, "×?")
			}
			for key, attr := range block.Attributes {
				if !attr.Literal {
					b.graph.Warnings = append(b.graph.Warnings, id+": static expression retained for "+key)
				}
			}
			b.add(n)
		case "module":
			name := block.Labels[0]
			id := scope + "module." + name
			src := textAttr(block.Attributes, "source")
			version := textAttr(block.Attributes, "version")
			local := strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../")
			installed := b.modules[manifestKey(id)]
			childProviders := map[string]string{"aws": regions["aws"]}
			for key, provider := range block.Attributes["providers"].Bindings {
				region := regions[provider]
				if region == "" {
					region = "unknown"
				}
				childProviders[key] = region
			}
			if local || installed != "" {
				child := installed
				if local {
					child = filepath.Join(real, src)
				}
				for input, attr := range block.Attributes {
					b.inputs[id+".var."+input] = scopedRefs(scope, attr.References)
				}
				start := len(b.order)
				if err := b.load(ctx, child, id+".", childProviders, depth+1); errors.Is(err, errLimit) {
					return err
				} else if err != nil {
					b.graph.Warnings = append(b.graph.Warnings, id+": "+err.Error())
				}
				for _, childID := range b.order[start:] {
					b.expansions[id] = append(b.expansions[id], childID)
					b.moduleDeps[childID] = append(b.moduleDeps[childID], scopedRefs(scope, block.Attributes["depends_on"].References)...)
					for _, badge := range multiplicity(block.Attributes) {
						b.graph.Nodes[childID].Badge = append(b.graph.Nodes[childID].Badge, "module "+name+" "+badge)
					}
				}
			} else {
				label := name + "\n" + src
				if version != "" {
					label += " @ " + version
				}
				n := &Node{ID: id, Type: "module", Label: label, Region: childProviders["aws"], Scope: scope, Attributes: block.Attributes, Badge: multiplicity(block.Attributes)}
				if src == "terraform-aws-modules/vpc/aws" || strings.Contains(src, "/terraform-aws-vpc") {
					n.Type = "aws_vpc_module"
					n.Group = true
				}
				b.add(n)
				b.graph.Warnings = append(b.graph.Warnings, id+": module not installed; collapsed")
			}
		}
		if len(b.graph.Nodes) > maxNodes {
			return fmt.Errorf("%w: configuration exceeds %d nodes", errLimit, maxNodes)
		}
	}
	return nil
}

func resolveLiteral(attr source.Attribute, locals map[string]source.Attribute) source.Attribute {
	if len(attr.References) == 1 && strings.TrimSpace(attr.Raw) == attr.References[0] {
		if local, ok := locals[attr.References[0]]; ok {
			attr.Value = local.Value
			attr.Literal = true
		}
	}
	return attr
}

func (b *builder) add(n *Node) {
	if b.graph.Nodes[n.ID] == nil {
		b.order = append(b.order, n.ID)
	}
	b.graph.Nodes[n.ID] = n
}

// manifestKey converts module.a.module.b into the modules.json key a.b.
// Module names may themselves contain "module", so split rather than replace.
func manifestKey(id string) string {
	parts := strings.Split(id, ".")
	names := make([]string, 0, len(parts)/2)
	for i := 1; i < len(parts); i += 2 {
		names = append(names, parts[i])
	}
	return strings.Join(names, ".")
}

func scopedRefs(scope string, refs []string) []string {
	result := make([]string, 0, len(refs))
	for _, r := range refs {
		result = append(result, scope+r)
	}
	return result
}

func textAttr(attrs map[string]source.Attribute, key string) string {
	attr := attrs[key]
	if !attr.Literal {
		return ""
	}
	s, _ := attr.Value.(string)
	return s
}

func (b *builder) resolve(ref string, seen map[string]bool) []string {
	if seen[ref] {
		return nil
	}
	seen[ref] = true
	if ids, ok := b.expansions[ref]; ok {
		return ids
	}
	for key, refs := range b.inputs {
		if ref == key || strings.HasPrefix(ref, key+".") {
			return b.resolveAll(refs, seen)
		}
	}
	// Expanded modules expose only declared outputs.
	parts := strings.Split(ref, ".")
	for i := len(parts) - 1; i >= 2; i-- {
		if parts[i-2] == "module" {
			key := strings.Join(parts[:i], ".") + ".output." + parts[i]
			if refs, ok := b.outputs[key]; ok {
				return b.resolveAll(refs, seen)
			}
		}
	}
	for candidate := ref; candidate != ""; {
		if n := b.graph.Nodes[candidate]; n != nil {
			if n.Type == "aws_vpc_module" {
				// Only the subnet ID outputs place resources; private_subnets_cidr_blocks
				// and similar outputs are ordinary references to the module.
				output, _, _ := strings.Cut(strings.TrimPrefix(ref, candidate+"."), ".")
				switch output {
				case "private_subnets":
					return []string{candidate + ".tier.private"}
				case "public_subnets":
					return []string{candidate + ".tier.public"}
				}
			}
			return []string{candidate}
		}
		i := strings.LastIndex(candidate, ".")
		if i < 0 {
			break
		}
		candidate = candidate[:i]
	}
	return nil
}

func (b *builder) resolveAll(refs []string, seen map[string]bool) []string {
	var ids []string
	for _, ref := range refs {
		ids = append(ids, b.resolve(ref, seen)...)
	}
	return ids
}

func (b *builder) references(n *Node, keys ...string) []string {
	var refs []string
	for path, attr := range n.Attributes {
		match := len(keys) == 0
		for _, key := range keys {
			if path == key || strings.HasSuffix(path, "."+key) {
				match = true
			}
		}
		if match {
			for _, ref := range attr.References {
				refs = append(refs, b.resolve(n.Scope+ref, map[string]bool{})...)
			}
		}
	}
	sort.Strings(refs)
	return refs
}

func (b *builder) edges() {
	counts := map[[2]string]int{}
	for _, id := range b.graph.IDs() {
		n := b.graph.Nodes[id]
		refs := b.references(n)
		refs = append(refs, b.resolveAll(b.moduleDeps[id], map[string]bool{})...)
		for _, ref := range refs {
			if ref != id && !b.graph.Ancestor(id, ref) {
				counts[[2]string{id, ref}]++
			}
		}
	}
	keys := make([][2]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	for _, key := range keys {
		b.graph.Edges = append(b.graph.Edges, Edge{From: key[0], To: key[1], Count: counts[key]})
	}
}

func multiplicity(attrs map[string]source.Attribute) []string {
	if count, ok := attrs["count"]; ok {
		if v, ok := count.Value.(float64); ok && count.Literal {
			return []string{fmt.Sprintf("×%.0f", v)}
		}
		return []string{"×?"}
	}
	if _, ok := attrs["for_each"]; ok {
		return []string{"×?"}
	}
	return nil
}
