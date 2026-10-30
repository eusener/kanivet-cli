package catalog_test

import (
	"context"
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

var baseItems = []string{
	"kitty", "herdr", "herdr-plugins", "nerd-font-jetbrains", "herdr-icon-font", "starship", "zsh",
	"zsh-autosuggestions", "zsh-syntax-highlighting", "direnv", "atuin", "zoxide", "fzf", "eza", "bat",
	"fd", "ripgrep", "dust", "duf", "btop", "procs", "sd", "yazi", "television", "lazygit", "delta", "gh",
	"jj", "jq", "yq", "xh", "just", "hyperfine", "watchexec", "tldr", "mise",
}

func embedded(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEmbeddedCatalogLints(t *testing.T) {
	c := embedded(t)
	for _, err := range catalog.Lint(c, backend.Names()) {
		t.Error(err)
	}
	for _, id := range baseItems {
		if c.Items[id] == nil {
			t.Errorf("missing item %q", id)
		}
	}
}

var moreItems = []string{
	"node", "pnpm", "bun", "python", "go", "rust", "dotnet", "java", "php", "composer", "homebrew",
	"lerd", "flock", "uv", "ruff", "jupyter", "cuda", "podman", "docker", "lazydocker", "k9s", "kubectl",
	"neovim-bin", "helix", "zed", "vscode", "lazyvim", "astronvim", "nvchad", "kickstart", "neovim-plain",
}

var (
	linuxSystemd = platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "ubuntu", DistroVersion: "24.04", PkgManager: "apt", Shell: "zsh", Systemd: true, Graphical: true, Home: "/home/dev"}
	macARM       = platform.Platform{OS: platform.MacOS, Arch: "arm64", DistroVersion: "15.2", Shell: "zsh", Graphical: true, Home: "/Users/dev"}
)

// planFor monta o plano de um perfil numa máquina onde nada está instalado.
func planFor(t *testing.T, p platform.Platform, profile string) *plan.Plan {
	t.Helper()
	c := embedded(t)
	f := &runner.FakeRunner{Func: func(ctx context.Context, cmd runner.Cmd) (runner.Result, bool) {
		if strings.HasPrefix(cmd.Shell, "command -v nvidia-smi") {
			return runner.Result{ExitCode: 1}, true
		}
		return runner.Result{ExitCode: 1}, true // nada instalado
	}}
	selected := map[string]bool{}
	for _, pr := range c.Profiles {
		if pr.ID == profile {
			for _, id := range pr.Items {
				selected[id] = true
			}
			for _, id := range pr.ExclusiveByOS[p.OS] {
				selected[id] = true
			}
			for _, id := range pr.Exclusive {
				selected[id] = true
			}
		}
	}
	st := (&detect.Detector{R: f, P: p}).Detect(context.Background(), c, selected)
	pl, err := plan.Build(c, p, st, plan.Selection{Profile: profile})
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

func hasStep(pl *plan.Plan, id string) bool {
	_, ok := pl.Step(id)
	return ok
}

func TestEmbeddedHasAllItemsAndProfiles(t *testing.T) {
	c := embedded(t)
	for _, id := range moreItems {
		if c.Items[id] == nil {
			t.Errorf("missing item %q", id)
		}
	}
	got := map[string]bool{}
	for _, p := range c.Profiles {
		got[p.ID] = true
	}
	for _, id := range []string{"laravel", "web", "ml", "backend", "blank"} {
		if !got[id] {
			t.Errorf("missing profile %q", id)
		}
	}
}

func TestLaravelProfileProvidesPHP(t *testing.T) {
	pl := planFor(t, linuxSystemd, "laravel")
	if hasStep(pl, "install:php") || hasStep(pl, "install:composer") || !hasStep(pl, "install:lerd") || hasStep(pl, "install:flock") {
		t.Fatalf("steps: %v", stepIDs(pl))
	}
}

func TestLaravelMacUsesFlock(t *testing.T) {
	pl := planFor(t, macARM, "laravel")
	if !hasStep(pl, "install:flock") || hasStep(pl, "install:lerd") || hasStep(pl, "install:php") {
		t.Fatalf("steps: %v", stepIDs(pl))
	}
}

func TestLaravelIntelMacInstallsPHP(t *testing.T) {
	intel := macARM
	intel.Arch = "amd64"
	pl := planFor(t, intel, "laravel")
	if hasStep(pl, "install:flock") || !hasStep(pl, "install:php") {
		t.Fatalf("flock unsupported on Intel: php must be installed: %v", stepIDs(pl))
	}
}

func TestBackendProfileHasRust(t *testing.T) {
	pl := planFor(t, linuxSystemd, "backend")
	s, ok := pl.Step("install:rust")
	if !ok || !strings.Contains(s.Cmds[0].String(), "sh.rustup.rs") {
		t.Fatalf("rust step: %+v %v", s, ok)
	}
}

func TestHeadlessSkipsTerminal(t *testing.T) {
	headless := linuxSystemd
	headless.Graphical = false
	pl := planFor(t, headless, "blank")
	if hasStep(pl, "install:kitty") || hasStep(pl, "install:herdr") || !hasStep(pl, "install:fzf") {
		t.Fatalf("steps: %v", stepIDs(pl))
	}
}

func TestLinuxCLIToolsUseMise(t *testing.T) {
	pl := planFor(t, linuxSystemd, "blank")
	s, ok := pl.Step("install:fzf")
	if !ok || s.Backend != "mise" || !hasStep(pl, "install:mise") {
		t.Fatalf("fzf: %+v", s)
	}
	for _, st := range pl.Steps {
		if st.Backend == "brew" {
			t.Fatalf("no brew on Linux: %s", st.ID)
		}
	}
}

func stepIDs(pl *plan.Plan) []string {
	var out []string
	for _, s := range pl.Steps {
		out = append(out, s.ID)
	}
	return out
}
