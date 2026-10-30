package backend

import (
	"strings"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// system instala pacotes do sistema (sempre com sudo). Package é "pkg" ou
// "apt=pkg1 pkg2,dnf=pkg3" para nomes diferentes por gerenciador.
type system struct{ base }

func init() { register(system{}) }

func (system) Name() string { return "system" }

var pkgCommands = map[string]struct{ install, remove []string }{
	"apt":    {[]string{"apt-get", "install", "-y"}, []string{"apt-get", "remove", "-y"}},
	"dnf":    {[]string{"dnf", "install", "-y"}, []string{"dnf", "remove", "-y"}},
	"pacman": {[]string{"pacman", "-S", "--needed", "--noconfirm"}, []string{"pacman", "-R", "--noconfirm"}},
	"zypper": {[]string{"zypper", "install", "-y"}, []string{"zypper", "remove", "-y"}},
}

func packagesFor(spec, manager string) []string {
	if !strings.Contains(spec, "=") {
		return strings.Fields(spec)
	}
	for _, part := range strings.Split(spec, ",") {
		if k, v, ok := strings.Cut(part, "="); ok && strings.TrimSpace(k) == manager {
			return strings.Fields(v)
		}
	}
	return nil
}

func (system) Supports(m catalog.Method, p platform.Platform) bool {
	_, known := pkgCommands[p.PkgManager]
	return p.OS == platform.Linux && known && len(packagesFor(m.Package, p.PkgManager)) > 0
}

func sudoCmd(lock string, args ...string) runner.Cmd {
	return runner.Cmd{Argv: args, Sudo: true, Lock: lock}
}

func (system) Install(m catalog.Method, _ string, p platform.Platform) []runner.Cmd {
	pc := pkgCommands[p.PkgManager]
	var cmds []runner.Cmd
	if p.PkgManager == "apt" {
		cmds = append(cmds, sudoCmd("apt", "apt-get", "update"))
	}
	args := append(append([]string{}, pc.install...), packagesFor(m.Package, p.PkgManager)...)
	return append(cmds, sudoCmd(p.PkgManager, args...))
}

func (s system) Update(m catalog.Method, p platform.Platform) []runner.Cmd {
	return s.Install(m, "", p)
}

func (system) Uninstall(m catalog.Method, p platform.Platform) []runner.Cmd {
	pc := pkgCommands[p.PkgManager]
	args := append(append([]string{}, pc.remove...), packagesFor(m.Package, p.PkgManager)...)
	return []runner.Cmd{sudoCmd(p.PkgManager, args...)}
}
