package main

import (
	"runtime/debug"
	"testing"
)

func TestCompilerVersion(t *testing.T) {
	set := func(kv ...string) []debug.BuildSetting {
		var s []debug.BuildSetting
		for i := 0; i < len(kv); i += 2 {
			s = append(s, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return s
	}
	rev := "0123456789abcdef"
	tm := "2026-03-04T05:06:07Z"
	tests := []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		want     string
	}{
		{"nil info", "dev", nil, "dev"},
		{"nil info injected", "v1.0.0", nil, "v1.0.0"},
		{"empty main version", "dev", &debug.BuildInfo{}, "dev"},
		{"devel main version", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
		{"module version", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0"},
		{"pseudo plain", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20261021065615-d7f5fab9d29d"}}, "dev"},
		{"pseudo pre", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3-rc.0.20261021065615-d7f5fab9d29d"}}, "dev"},
		{"pseudo next", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.2-0.20261021065615-d7f5fab9d29d"}}, "dev"},
		{"pseudo dirty", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.2-0.20261021065615-d7f5fab9d29d+dirty"}}, "dev"},
		{"tag dirty", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0+dirty"}}, "dev"},
		{"incompatible", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v2.0.0+incompatible"}}, "dev"},
		{"prerelease tag", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0-rc.1"}}, "v0.1.0-rc.1"},
		{"injected wins", "v0.2.0", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.2.0"},
		{"vcs clean", "v0.2.0", &debug.BuildInfo{Settings: set("vcs.revision", rev, "vcs.time", tm, "vcs.modified", "false")}, "v0.2.0 (0123456, 2026-03-04)"},
		{"vcs dirty", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: set("vcs.revision", rev, "vcs.time", tm, "vcs.modified", "true")}, "dev (0123456-dirty, 2026-03-04)"},
		{"vcs no time", "dev", &debug.BuildInfo{Settings: set("vcs.revision", rev)}, "dev (0123456)"},
		{"short rev", "dev", &debug.BuildInfo{Settings: set("vcs.revision", "abc", "vcs.time", tm)}, "dev (abc, 2026-03-04)"},
		{"time without revision", "dev", &debug.BuildInfo{Settings: set("vcs.time", tm)}, "dev"},
		{"module version and vcs", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}, Settings: set("vcs.revision", rev, "vcs.time", tm)}, "v0.1.0 (0123456, 2026-03-04)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compilerVersion(tt.injected, tt.info); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
