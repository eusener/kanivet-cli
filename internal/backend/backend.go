// Package backend traduz métodos de instalação do catálogo em comandos.
// Nenhum backend executa nada: quem roda os comandos é o executor.
package backend

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type Backend interface {
	Name() string
	Supports(m catalog.Method, p platform.Platform) bool
	Install(m catalog.Method, version string, p platform.Platform) []runner.Cmd
	Update(m catalog.Method, p platform.Platform) []runner.Cmd
	Uninstall(m catalog.Method, p platform.Platform) []runner.Cmd
	// Latest devolve pacote → versão mais nova disponível; nil quando o backend não sabe.
	Latest(ctx context.Context, r runner.Runner, ms []catalog.Method) map[string]string
}

var registry = map[string]Backend{}

func register(b Backend) { registry[b.Name()] = b }

func Get(name string) (Backend, bool) {
	b, ok := registry[name]
	return b, ok
}

func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Choose devolve o primeiro método do item compatível com a plataforma.
func Choose(it *catalog.Item, p platform.Platform) (catalog.Method, Backend, error) {
	for _, m := range it.Install {
		b, ok := Get(m.Backend)
		if ok && whenMatches(m.When, p) && b.Supports(m, p) {
			return m, b, nil
		}
	}
	return catalog.Method{}, nil, errors.New(i18n.T("err.no_method", string(p.OS)))
}

func whenMatches(w *catalog.When, p platform.Platform) bool {
	if w == nil {
		return true
	}
	if (w.OS != "" && w.OS != p.OS) || (w.Arch != "" && w.Arch != p.Arch) {
		return false
	}
	return w.PkgManager == "" || w.PkgManager == p.PkgManager
}

// q faz quoting seguro para sh.
func q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func expandHome(s, home string) string {
	if s == "~" {
		return home
	}
	if strings.HasPrefix(s, "~/") {
		return home + s[1:]
	}
	return s
}

func argv(lock string, args ...string) runner.Cmd { return runner.Cmd{Argv: args, Lock: lock} }

// base implementa os métodos opcionais com o comportamento "não suportado".
type base struct{}

func (base) Update(catalog.Method, platform.Platform) []runner.Cmd    { return nil }
func (base) Uninstall(catalog.Method, platform.Platform) []runner.Cmd { return nil }
func (base) Latest(context.Context, runner.Runner, []catalog.Method) map[string]string {
	return nil
}
func (base) Supports(catalog.Method, platform.Platform) bool { return true }
