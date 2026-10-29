package backend

import (
	"strings"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// script baixa um instalador oficial (https), confere o SHA256 quando há e roda com bash.
// Não sabe desinstalar: o planner avisa que a remoção é manual.
type script struct{ base }

func init() { register(script{}) }

func (script) Name() string { return "script" }

func (script) Install(m catalog.Method, _ string, _ platform.Platform) []runner.Cmd {
	var b strings.Builder
	b.WriteString(`T=$(mktemp) && curl -fsSL ` + q(m.URL) + ` -o "$T"`)
	if m.SHA256 != "" {
		sum := `echo ` + q(m.SHA256+"  ") + `"$T"`
		b.WriteString(` && { if command -v sha256sum >/dev/null 2>&1; then ` + sum + ` | sha256sum -c -; else ` + sum + ` | shasum -a 256 -c -; fi; }`)
	}
	b.WriteString(` && bash "$T"`)
	for _, a := range m.Args {
		b.WriteString(" " + q(a))
	}
	b.WriteString(`; rc=$?; rm -f "$T"; exit $rc`)
	return []runner.Cmd{{Shell: b.String(), Display: "curl " + m.URL + " | bash"}}
}

func (s script) Update(m catalog.Method, p platform.Platform) []runner.Cmd {
	return s.Install(m, "", p)
}
