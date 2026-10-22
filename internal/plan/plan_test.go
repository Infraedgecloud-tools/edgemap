package plan

import (
	"strings"
	"testing"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
)

func TestAddress(t *testing.T) {
	for _, input := range []string{`module.app["a.b"].aws_instance.web[0]`, `module.app["x]y"].aws_instance.web["key"]`} {
		got, err := Address(input)
		if err != nil || got != "module.app.aws_instance.web" {
			t.Fatalf("%q %v", got, err)
		}
	}
	if _, err := Address(`aws_instance.web["broken`); err == nil {
		t.Fatal("accepted malformed address")
	}
}

func TestActionsAndSecretExclusion(t *testing.T) {
	raw := `{"resource_changes":[
 {"address":"aws_instance.web[0]","type":"aws_instance","change":{"actions":["delete","create"],"before":{"secret":"DO_NOT_RETAIN"},"after":{"secret":"DO_NOT_RETAIN"}}},
 {"address":"aws_instance.web[1]","type":"aws_instance","change":{"actions":["update"]}},
 {"address":"aws_instance.web[2]","type":"aws_instance","change":{"actions":["delete"]}},
 {"address":"aws_s3_bucket.removed","type":"aws_s3_bucket","change":{"actions":["delete"]}}]}`
	changes, err := Read(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	g := &graph.Graph{Nodes: map[string]*graph.Node{"aws_instance.web": {ID: "aws_instance.web", Type: "aws_instance", Badge: []string{"×?"}}}}
	colors, err := Apply(g, changes)
	if err != nil {
		t.Fatal(err)
	}
	if colors["aws_instance.web"] != "#C0392B" {
		t.Fatal("destroy must dominate")
	}
	badge := strings.Join(g.Nodes["aws_instance.web"].Badge, " ")
	for _, want := range []string{"×2 after plan", "create 1", "change 1", "destroy 2"} {
		if !strings.Contains(badge, want) {
			t.Fatalf("missing %s: %s", want, badge)
		}
	}
	if strings.Contains(badge, "DO_NOT_RETAIN") {
		t.Fatal("secret retained")
	}
	if g.Nodes["aws_s3_bucket.removed"].Container != "plan-only" {
		t.Fatal("missing tombstone")
	}
}

func TestRejectInvalidPlans(t *testing.T) {
	for _, input := range []string{`{`, `{} {}`, `{"resource_changes":[{"address":"x","type":"x","change":{"actions":["surprise"]}}]}`} {
		if _, err := Read(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestForget(t *testing.T) {
	changes, err := Read(strings.NewReader(`{"resource_changes":[{"address":"aws_instance.old","type":"aws_instance","change":{"actions":["forget"]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	g := &graph.Graph{Nodes: map[string]*graph.Node{}}
	colors, err := Apply(g, changes)
	if err != nil {
		t.Fatal(err)
	}
	badge := strings.Join(g.Nodes["aws_instance.old"].Badge, " ")
	if !strings.Contains(badge, "×0 after plan") || !strings.Contains(badge, "forget 1") || colors["aws_instance.old"] != "#64748B" {
		t.Fatalf("incorrect forget annotation %s %v", badge, colors)
	}
}

func TestModuleAggregationAndActionValidation(t *testing.T) {
	raw := `{"resource_changes":[
 {"address":"module.remote[0].module.inner.aws_instance.web[0]","type":"aws_instance","change":{"actions":["create","forget"]}},
 {"address":"module.remote[1].module.inner.aws_instance.web[1]","type":"aws_instance","change":{"actions":["delete"]}},
 {"address":"module.local.aws_instance.web[0]","type":"aws_instance","change":{"actions":["update"]}}]}`
	changes, err := Read(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	g := &graph.Graph{Nodes: map[string]*graph.Node{
		"module.remote":                 {ID: "module.remote", Type: "module"},
		"module.local.aws_instance.web": {ID: "module.local.aws_instance.web", Type: "aws_instance"},
	}}
	colors, err := Apply(g, changes)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Nodes) != 2 || colors["module.remote"] != "#C0392B" || colors["module.local.aws_instance.web"] != "#B76E00" {
		t.Fatalf("wrong aggregation %v %v", g.IDs(), colors)
	}
	if !strings.Contains(strings.Join(g.Nodes["module.remote"].Badge, " "), "2 plan resources") {
		t.Fatal("missing aggregate count")
	}
	for _, actions := range []string{`["delete,create"]`, `["delete","delete"]`, `["no-op","create"]`} {
		if _, err := Read(strings.NewReader(`{"resource_changes":[{"address":"aws_instance.web","type":"aws_instance","change":{"actions":` + actions + `}}]}`)); err == nil {
			t.Fatal("accepted invalid action combination")
		}
	}
}

func TestSummaryOmitsZeroCounts(t *testing.T) {
	for _, tc := range []struct {
		c    counts
		want string
	}{
		{counts{unchanged: 3}, "×3"},
		{counts{create: 1, delete: 1, unchanged: 1}, "×3 · create 1 · destroy 1 · unchanged 1"},
		{counts{update: 2}, "×3 · change 2"},
	} {
		if got := summary("×3", &tc.c); got != tc.want {
			t.Errorf("summary(%+v) = %q, want %q", tc.c, got, tc.want)
		}
	}
}
