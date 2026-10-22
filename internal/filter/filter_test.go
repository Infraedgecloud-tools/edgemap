package filter

import (
	"context"
	"strings"
	"testing"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
	"github.com/Infraedgecloud-tools/edgemap/internal/parse"
)

func TestFlagsAndBadges(t *testing.T) {
	original, err := graph.Build(context.Background(), parse.Terraform{}, "../../testdata/two-tier")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		opts Options
		keep string
	}{
		{"default", Options{}, ""}, {"iam", Options{ShowIAM: true}, "aws_iam_role.web"}, {"data", Options{ShowData: true}, "data.aws_caller_identity.current"}, {"sg", Options{ShowSG: true}, "aws_security_group.web"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := Apply(original, test.opts)
			if test.keep != "" && result.Nodes[test.keep] == nil {
				t.Fatalf("missing opted-in node %s", test.keep)
			}
			if !test.opts.ShowIAM && result.Nodes["aws_iam_role.web"] != nil {
				t.Fatal("IAM not hidden")
			}
			if !test.opts.ShowSG && result.Nodes["aws_security_group.web"] != nil {
				t.Fatal("SG not hidden")
			}
			if !test.opts.ShowData && result.Nodes["data.aws_caller_identity.current"] != nil {
				t.Fatal("data not hidden")
			}
			if test.name == "default" {
				badges := strings.Join(result.Nodes["aws_instance.web"].Badge, " ")
				if !strings.Contains(badges, "IAM:") || !strings.Contains(badges, "SG:") {
					t.Fatalf("badges %q", badges)
				}
			}
			for _, edge := range result.Edges {
				if result.Nodes[edge.From] == nil || result.Nodes[edge.To] == nil {
					t.Fatal("dangling edge")
				}
			}
		})
	}
	if len(original.Nodes["aws_instance.web"].Badge) != 0 {
		t.Fatal("input graph mutated")
	}
}

func TestHiddenContainerReparentsChildren(t *testing.T) {
	g := &graph.Graph{Nodes: map[string]*graph.Node{
		"region":                {ID: "region", Type: "region", Group: true},
		"data.aws_vpc.existing": {ID: "data.aws_vpc.existing", Type: "aws_vpc", Data: true, Group: true, Container: "region"},
		"tier":                  {ID: "tier", Type: "tier", Group: true, Container: "data.aws_vpc.existing"},
		"subnet":                {ID: "subnet", Type: "aws_subnet", Container: "tier"},
	}}
	filtered := Apply(g, Options{})
	if filtered.Nodes["data.aws_vpc.existing"] == nil || filtered.Nodes["tier"].Container != "data.aws_vpc.existing" {
		t.Fatal("data-source VPC holding resources was hidden, leaving its tier outside any VPC")
	}
	for _, n := range filtered.Nodes {
		if n.Container != "" && filtered.Nodes[n.Container] == nil {
			t.Fatal("missing parent")
		}
	}
}

func TestPruneNestedEmptyContainers(t *testing.T) {
	g := &graph.Graph{Nodes: map[string]*graph.Node{
		"r": {ID: "r", Type: "region", Group: true}, "o": {ID: "o", Type: "other", Group: true, Container: "r"},
		"d": {ID: "d", Type: "aws_unknown", Data: true, Container: "o"},
	}}
	if got := Apply(g, Options{}); len(got.Nodes) != 0 {
		t.Fatalf("empty containers remain: %v", got.IDs())
	}
}

func TestNonAWSUngrouped(t *testing.T) {
	g, err := graph.Build(context.Background(), parse.Terraform{}, "../../testdata/for-each")
	if err != nil {
		t.Fatal(err)
	}
	if g.Nodes["random_id.suffix"].Container != "" {
		t.Fatal("non-AWS resource grouped")
	}
}

func TestEmptyDataVPCIsPruned(t *testing.T) {
	g := &graph.Graph{Nodes: map[string]*graph.Node{
		"region":            {ID: "region", Type: "region", Group: true},
		"data.aws_vpc.peer": {ID: "data.aws_vpc.peer", Type: "aws_vpc", Data: true, Group: true, Container: "region"},
		"tier":              {ID: "tier", Type: "tier", Group: true, Container: "data.aws_vpc.peer"},
	}}
	g.Nodes["aws_route53_zone_association.a"] = &graph.Node{ID: "aws_route53_zone_association.a", Type: "aws_route53_zone_association", Container: "global"}
	g.Nodes["global"] = &graph.Node{ID: "global", Type: "global", Group: true}
	g.Edges = []graph.Edge{{From: "aws_route53_zone_association.a", To: "data.aws_vpc.peer", Count: 1}}
	got := Apply(g, Options{})
	if got.Nodes["data.aws_vpc.peer"] != nil || got.Nodes["tier"] != nil || got.Nodes["region"] != nil || len(got.Edges) != 0 {
		t.Fatalf("data VPC with only an incoming edge remains: %v %v", got.IDs(), got.Edges)
	}
	if got := Apply(g, Options{ShowData: true}); got.Nodes["data.aws_vpc.peer"] == nil {
		t.Fatal("--show-data hid a data VPC")
	}
}
