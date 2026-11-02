package backend

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type mise struct{ base }

func init() { register(mise{}) }

func (mise) Name() string { return "mise" }

func (mise) Install(m catalog.Method, version string, _ platform.Platform) []runner.Cmd {
	if version == "" {
		version = m.Version
	}
	if version == "" {
		version = "latest"
	}
	return []runner.Cmd{withGoToolchain(m, argv("mise", "mise", "use", "-g", m.Package+"@"+version))}
}

func (mise) Update(m catalog.Method, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{withGoToolchain(m, argv("mise", "mise", "upgrade", m.Package))}
}

// withGoToolchain deixa o `go install` dos pacotes go: baixar a toolchain que a ferramenta
// exige (gopls pede Go mais novo que o do catálogo). Distros como o Fedora fixam
// GOTOOLCHAIN=local no Go do sistema, e aí a instalação falharia.
func withGoToolchain(m catalog.Method, c runner.Cmd) runner.Cmd {
	if strings.HasPrefix(m.Package, "go:") {
		c.Env = append(c.Env, "GOTOOLCHAIN=auto")
	}
	return c
}

func (mise) Uninstall(m catalog.Method, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{
		argv("mise", "mise", "uninstall", "--all", m.Package),
		argv("mise", "mise", "unuse", "-g", m.Package),
	}
}

func (mise) Latest(ctx context.Context, r runner.Runner, ms []catalog.Method) map[string]string {
	res := r.Run(ctx, argv("mise", "mise", "outdated", "--json"), io.Discard)
	if !res.OK() {
		return nil
	}
	var out map[string]struct {
		Latest string `json:"latest"`
	}
	if json.Unmarshal([]byte(res.Output), &out) != nil {
		return nil
	}
	got := map[string]string{}
	for _, m := range ms {
		if o, ok := out[m.Package]; ok {
			got[m.Package] = o.Latest
		}
	}
	return got
}
