package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	vcs := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "9f030c0a1b2c3d4e5f"},
		{Key: "vcs.time", Value: "2026-10-30T18:04:05Z"},
	}
	bi := func(mainVersion string, s []debug.BuildSetting) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: mainVersion}, Settings: s}, true
		}
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }

	cases := []struct {
		name            string
		v, commit, date string
		read            func() (*debug.BuildInfo, bool)
		want            string
	}{
		{"ldflags da release", "0.1.0", "9f030c0", "2026-10-31T12:00:00Z", none, "0.1.0 (9f030c0, 2026-10-31)"},
		{"go install de uma tag", "", "", "", bi("v0.1.0", nil), "0.1.0 (unknown, unknown)"},
		{"go build local com vcs", "", "", "", bi("(devel)", vcs), "dev (9f030c0, 2026-10-30)"},
		{"build com árvore suja", "", "", "", bi("v0.1.1-0.20261030180405-9f030c0a1b2c+dirty", vcs), "dev (9f030c0, 2026-10-30)"},
		{"go build fora de uma tag", "", "", "", bi("v0.0.0-20261030180405-9f030c0a1b2c", vcs), "dev (9f030c0, 2026-10-30)"},
		{"go install de commit depois de uma tag", "", "", "", bi("v0.1.1-0.20261030180405-9f030c0a1b2c", vcs), "dev (9f030c0, 2026-10-30)"},
		{"sem build info", "", "", "", none, "dev (unknown, unknown)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolve(c.v, c.commit, c.date, c.read).String(); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestStringNeverEmpty(t *testing.T) {
	i := Get()
	if i.Version == "" || i.Commit == "" || i.Date == "" {
		t.Fatalf("campo vazio: %+v", i)
	}
}
