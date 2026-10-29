package backend

import (
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type rustup struct{ base }

func init() { register(rustup{}) }

func (rustup) Name() string { return "rustup" }

func (rustup) Install(m catalog.Method, version string, _ platform.Platform) []runner.Cmd {
	if version == "" {
		version = m.Version
	}
	sh := "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path"
	if version != "" {
		sh += " --default-toolchain " + version
	}
	return []runner.Cmd{{Shell: sh, Display: "rustup (" + version + ")"}}
}

func (rustup) Update(catalog.Method, platform.Platform) []runner.Cmd {
	return []runner.Cmd{argv("", "rustup", "update")}
}

func (rustup) Uninstall(catalog.Method, platform.Platform) []runner.Cmd {
	return []runner.Cmd{argv("", "rustup", "self", "uninstall", "-y")}
}
