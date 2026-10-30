package backend

import (
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// git clona (ou atualiza) um repositório raso em Dest.
type git struct{ base }

func init() { register(git{}) }

func (git) Name() string { return "git" }

func (git) Install(m catalog.Method, _ string, p platform.Platform) []runner.Cmd {
	dest := q(expandHome(m.Dest, p.Home))
	ref := "HEAD"
	branch := ""
	if m.Ref != "" {
		ref, branch = q(m.Ref), " --branch "+q(m.Ref)
	}
	sh := `if [ -d ` + dest + `/.git ]; then git -C ` + dest + ` fetch --depth 1 origin ` + ref +
		` && git -C ` + dest + ` checkout -q FETCH_HEAD; else git clone --depth 1` + branch + ` ` + q(m.Repo) + ` ` + dest + `; fi`
	return []runner.Cmd{{Shell: sh, Display: "git clone " + m.Repo}}
}

func (g git) Update(m catalog.Method, p platform.Platform) []runner.Cmd { return g.Install(m, "", p) }

func (git) Uninstall(m catalog.Method, p platform.Platform) []runner.Cmd {
	return []runner.Cmd{{Shell: "rm -rf " + q(expandHome(m.Dest, p.Home))}}
}
