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

type brew struct{}

func init() { register(brew{}) }

func (brew) Name() string { return "brew" }

// Supports: no Linux só quando o item pede brew explicitamente (When.OS: linux).
func (brew) Supports(m catalog.Method, p platform.Platform) bool {
	return p.OS == platform.MacOS || (m.When != nil && m.When.OS == platform.Linux)
}

func brewArgs(verb string, m catalog.Method) []string {
	if name, ok := strings.CutPrefix(m.Package, "cask:"); ok {
		return []string{"brew", verb, "--cask", name}
	}
	return []string{"brew", verb, m.Package}
}

func (brew) Install(m catalog.Method, _ string, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{argv("brew", brewArgs("install", m)...)}
}

func (brew) Update(m catalog.Method, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{argv("brew", brewArgs("upgrade", m)...)}
}

func (brew) Uninstall(m catalog.Method, _ platform.Platform) []runner.Cmd {
	return []runner.Cmd{argv("brew", brewArgs("uninstall", m)...)}
}

type brewOutdated struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
}

func (brew) Latest(ctx context.Context, r runner.Runner, ms []catalog.Method) map[string]string {
	res := r.Run(ctx, argv("brew", "brew", "outdated", "--json=v2"), io.Discard)
	if !res.OK() {
		return nil
	}
	var out struct {
		Formulae []brewOutdated `json:"formulae"`
		Casks    []brewOutdated `json:"casks"`
	}
	if json.Unmarshal([]byte(res.Output), &out) != nil {
		return nil
	}
	latest := map[string]string{}
	for _, f := range out.Formulae {
		latest[f.Name] = f.CurrentVersion
	}
	for _, c := range out.Casks {
		latest["cask:"+c.Name] = c.CurrentVersion
	}
	got := map[string]string{}
	for _, m := range ms {
		if v, ok := latest[m.Package]; ok {
			got[m.Package] = v
		}
	}
	return got
}
