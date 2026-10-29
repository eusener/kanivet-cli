package backend

import (
	"strings"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// herdrPlugin instala plugins do herdr. Cada entrada de Args é
// "<plugin-id>=<owner>/<repo>[/subdir]@<commit>".
type herdrPlugin struct{ base }

func init() { register(herdrPlugin{}) }

func (herdrPlugin) Name() string { return "herdr-plugin" }

type pluginSpec struct{ id, source, ref string }

func parsePlugins(args []string) []pluginSpec {
	var out []pluginSpec
	for _, a := range args {
		id, rest, _ := strings.Cut(a, "=")
		src, ref, _ := strings.Cut(rest, "@")
		out = append(out, pluginSpec{id, src, ref})
	}
	return out
}

func (herdrPlugin) Install(m catalog.Method, _ string, _ platform.Platform) []runner.Cmd {
	var cmds []runner.Cmd
	for _, s := range parsePlugins(m.Args) {
		install := []string{"herdr", "plugin", "install", s.source, "--yes"}
		if s.ref != "" {
			install = append(install, "--ref", s.ref)
		}
		// Plugins instalados já vêm ativados; o enable fala com o servidor do herdr,
		// que pode não estar rodando (máquina nova, CI). Por isso é só uma garantia.
		enable := runner.Cmd{Shell: "herdr plugin enable " + q(s.id) + " >/dev/null 2>&1 || true", Lock: "herdr"}
		cmds = append(cmds, argv("herdr", install...), enable)
	}
	return cmds
}

func (herdrPlugin) Update(m catalog.Method, _ platform.Platform) []runner.Cmd {
	var cmds []runner.Cmd
	for _, s := range parsePlugins(m.Args) {
		cmds = append(cmds, argv("herdr", "herdr", "plugin", "install", s.source, "--yes"))
	}
	return cmds
}

func (herdrPlugin) Uninstall(m catalog.Method, _ platform.Platform) []runner.Cmd {
	var cmds []runner.Cmd
	for _, s := range parsePlugins(m.Args) {
		cmds = append(cmds, argv("herdr", "herdr", "plugin", "uninstall", s.id))
	}
	return cmds
}
