package main

import (
	"runtime/debug"
	"testing"
)

func TestBuildVersion(t *testing.T) {
	module := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	for _, tc := range []struct {
		ldflag string
		info   *debug.BuildInfo
		ok     bool
		want   string
	}{
		{"0.1.0", module("v0.1.0"), true, "0.1.0"},
		{"dev", module("v0.1.0"), true, "0.1.0"},
		{"dev", module("v0.0.0-20261022000000-abcdef123456+dirty"), true, "0.0.0-20261022000000-abcdef123456+dirty"},
		{"dev", module("(devel)"), true, "dev"},
		{"dev", nil, false, "dev"},
	} {
		if got := buildVersion(tc.ldflag, tc.info, tc.ok); got != tc.want {
			t.Errorf("buildVersion(%q, %v) = %q, want %q", tc.ldflag, tc.info, got, tc.want)
		}
	}
}
