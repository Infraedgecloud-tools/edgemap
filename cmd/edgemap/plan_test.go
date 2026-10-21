package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanRenderCollapsesIndexedActionsAndExcludesValues(t *testing.T) {
	const secret = "FAKE_PLAN_SECRET_MUST_NOT_APPEAR"
	input := `{"resource_changes":[
 {"address":"aws_instance.workers[\"x]y\"]","type":"aws_instance","change":{"actions":["delete","create"],"before":{"password":"FAKE_PLAN_SECRET_MUST_NOT_APPEAR"},"after":{"password":"FAKE_PLAN_SECRET_MUST_NOT_APPEAR"}}},
 {"address":"aws_instance.workers[\"b\"]","type":"aws_instance","change":{"actions":["update"]}},
 {"address":"aws_instance.workers[\"c\"]","type":"aws_instance","change":{"actions":["delete"]}},
 {"address":"aws_instance.workers[\"d\"]","type":"aws_instance","change":{"actions":["no-op"]}},
 {"address":"aws_instance.fixed[0]","type":"aws_instance","change":{"actions":["create"]}},
 {"address":"aws_instance.fixed[1]","type":"aws_instance","change":{"actions":["update"]}},
 {"address":"aws_s3_bucket.archive","type":"aws_s3_bucket","change":{"actions":["create"]}}
 ],"prior_state":{"values":{"secret":"FAKE_PLAN_SECRET_MUST_NOT_APPEAR"}}}`
	dir := t.TempDir()
	planPath := writeSyntheticPlan(t, dir, input)
	output := filepath.Join(dir, "overlay.d2")
	stdout, stderr, err := executeRender("render", "../../testdata/for-each", "--plan", planPath, "--no-icons", "--format", "d2", "--out", output)
	if err != nil {
		t.Fatalf("render plan: %v\n%s", err, stderr)
	}
	data, err := os.ReadFile(output) // #nosec G304 -- test-generated output in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	d2 := string(data)
	for _, stream := range []string{d2, stdout, stderr} {
		if strings.Contains(stream, secret) {
			t.Fatal("plan before/after or prior-state value escaped into output")
		}
	}
	for _, tc := range []struct {
		label, count, color string
	}{
		{"workers\\naws_instance", "×3 after plan · create 1 · change 1 · destroy 2 · unchanged 1", "#C0392B"},
		{"fixed\\naws_instance", "×2 after plan · create 1 · change 1 · destroy 0 · unchanged 0", "#B76E00"},
		{"archive\\naws_s3_bucket", "×1 after plan · create 1 · change 0 · destroy 0 · unchanged 0", "#16803C"},
	} {
		block := d2NodeBlock(t, d2, tc.label)
		if !strings.Contains(block, tc.count) {
			t.Errorf("%s action summary missing: %s", tc.label, block)
		}
		if !strings.Contains(block, `style.stroke: "`+tc.color+`"`) {
			t.Errorf("%s wrong action color: %s", tc.label, block)
		}
		if strings.Contains(block, "×?") {
			t.Errorf("%s retains stale static fan-out badge", tc.label)
		}
	}
	if !strings.Contains(d2, "priority: destroy > change > create") {
		t.Error("plan color priority legend missing")
	}
	if strings.Contains(d2, "x]y") {
		t.Error("collapsed node leaked an individual instance key")
	}
}

func TestInvalidPlanProducesNoDiagram(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"syntax", `{"resource_changes":["FAKE_INVALID_PLAN_SECRET"`},
		{"action", `{"resource_changes":[{"address":"aws_instance.workers[0]","type":"aws_instance","change":{"actions":["FAKE_INVALID_PLAN_SECRET"]}}]}`},
		{"address", `{"resource_changes":[{"address":"aws_instance.workers[FAKE_INVALID_PLAN_SECRET","type":"aws_instance","change":{"actions":["create"]}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			planPath := writeSyntheticPlan(t, dir, tc.input)
			output := filepath.Join(dir, "invalid.svg")
			stdout, stderr, err := executeRender("render", "../../testdata/for-each", "--plan", planPath, "--no-icons", "--out", output)
			if err == nil {
				t.Fatal("invalid plan accepted")
			}
			for _, text := range []string{stdout, stderr, err.Error()} {
				if strings.Contains(text, "FAKE_INVALID_PLAN_SECRET") {
					t.Fatal("invalid plan diagnostic reveals input value")
				}
			}
			for _, path := range []string{output, strings.TrimSuffix(output, ".svg") + ".d2"} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("failed render left output %s: %v", filepath.Base(path), err)
				}
			}
		})
	}
}

func writeSyntheticPlan(t *testing.T, dir, input string) string {
	t.Helper()
	path := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func d2NodeBlock(t *testing.T, d2, label string) string {
	t.Helper()
	lines := strings.Split(d2, "\n")
	for i, line := range lines {
		if !strings.Contains(line, `: "`+label) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if lines[j] == "}" {
				return strings.Join(lines[i:j+1], "\n")
			}
		}
	}
	t.Fatalf("node label %q not found", label)
	return ""
}
