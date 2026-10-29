package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

type harness struct {
	env     *Env
	fake    *runner.FakeRunner
	out     *bytes.Buffer
	errOut  *bytes.Buffer
	present map[string]bool
	mu      sync.Mutex
}

// newHarness usa o catálogo de testdata numa plataforma Linux falsa; "<x> --version" falha
// a não ser que x esteja em present.
func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	c, err := catalog.Load(os.DirFS("../catalog/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	h := &harness{out: &bytes.Buffer{}, errOut: &bytes.Buffer{}, present: map[string]bool{}}
	h.fake = &runner.FakeRunner{Func: func(ctx context.Context, cmd runner.Cmd) (runner.Result, bool) {
		h.mu.Lock()
		defer h.mu.Unlock()
		// Instalar/remover via mise muda o que o detector enxerga.
		if len(cmd.Argv) >= 4 && cmd.Argv[0] == "mise" && cmd.Argv[1] == "use" {
			h.present[itemFromPackage(strings.Split(cmd.Argv[3], "@")[0])] = true
		}
		if len(cmd.Argv) >= 4 && cmd.Argv[0] == "mise" && cmd.Argv[1] == "uninstall" {
			delete(h.present, itemFromPackage(cmd.Argv[3]))
		}
		if id, ok := strings.CutSuffix(cmd.Shell, " --version"); ok {
			if h.present[id] {
				return runner.Result{Output: "1.0"}, true
			}
			return runner.Result{ExitCode: 1}, true
		}
		return runner.Result{}, false
	}}
	h.env = &Env{
		R: h.fake, Cat: c,
		P:        platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "ubuntu", PkgManager: "apt", Shell: "zsh", Systemd: true, Home: home},
		StateDir: filepath.Join(home, ".local", "state", "kanivet-cli"),
		Stdout:   h.out, Stderr: h.errOut, Stdin: strings.NewReader(""),
		IsTTY: true, Now: func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) },
		Getenv: map[string]string{},
	}
	return h
}

func (h *harness) run(args ...string) int {
	h.out.Reset()
	h.errOut.Reset()
	return Run(h.env, args)
}

func (h *harness) ran(cmd string) bool {
	for _, c := range h.fake.Strings() {
		if c == cmd {
			return true
		}
	}
	return false
}

// itemFromPackage: "aqua:junegunn/fzf" → "fzf", "node" → "node".
func itemFromPackage(pkg string) string {
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		return pkg[i+1:]
	}
	return pkg
}
