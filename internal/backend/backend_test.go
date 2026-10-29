package backend

import (
	"context"
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

var (
	mac    = platform.Platform{OS: platform.MacOS, Arch: "arm64", Home: "/Users/dev"}
	ubuntu = platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "ubuntu", PkgManager: "apt", Home: "/home/dev"}
	fedora = platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "fedora", PkgManager: "dnf", Home: "/home/dev"}
)

func strs(cmds []runner.Cmd) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.String()
	}
	return out
}

func eq(t *testing.T, label string, got []runner.Cmd, want ...string) {
	t.Helper()
	g := strs(got)
	if strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s:\n got  %q\n want %q", label, g, want)
	}
}

func get(t *testing.T, name string) Backend {
	t.Helper()
	b, ok := Get(name)
	if !ok {
		t.Fatalf("backend %s not registered", name)
	}
	return b
}

func TestBrew(t *testing.T) {
	b := get(t, "brew")
	m := catalog.Method{Backend: "brew", Package: "fzf"}
	eq(t, "install", b.Install(m, "", mac), "brew install fzf")
	eq(t, "update", b.Update(m, mac), "brew upgrade fzf")
	eq(t, "uninstall", b.Uninstall(m, mac), "brew uninstall fzf")
	cask := catalog.Method{Backend: "brew", Package: "cask:zed"}
	eq(t, "cask", b.Install(cask, "", mac), "brew install --cask zed")
	if b.Install(m, "", mac)[0].Lock != "brew" {
		t.Fatal("brew lock")
	}
	if b.Supports(m, ubuntu) {
		t.Fatal("brew without explicit linux When must not be chosen on Linux")
	}
}

func TestMise(t *testing.T) {
	b := get(t, "mise")
	m := catalog.Method{Backend: "mise", Package: "node", Version: "22"}
	eq(t, "install", b.Install(m, "", ubuntu), "mise use -g node@22")
	eq(t, "override", b.Install(m, "24", ubuntu), "mise use -g node@24")
	eq(t, "latest", b.Install(catalog.Method{Package: "aqua:junegunn/fzf"}, "", ubuntu), "mise use -g aqua:junegunn/fzf@latest")
	eq(t, "update", b.Update(m, ubuntu), "mise upgrade node")
	eq(t, "uninstall", b.Uninstall(m, ubuntu), "mise uninstall --all node", "mise unuse -g node")
	if b.Install(m, "", ubuntu)[0].Lock != "mise" {
		t.Fatal("mise lock")
	}
}

func TestRustup(t *testing.T) {
	b := get(t, "rustup")
	m := catalog.Method{Backend: "rustup", Version: "stable"}
	eq(t, "install", b.Install(m, "", ubuntu),
		"curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path --default-toolchain stable")
	eq(t, "update", b.Update(m, ubuntu), "rustup update")
	eq(t, "uninstall", b.Uninstall(m, ubuntu), "rustup self uninstall -y")
}

func TestSystem(t *testing.T) {
	b := get(t, "system")
	m := catalog.Method{Backend: "system", Package: "apt=build-essential,dnf=@development-tools"}
	eq(t, "apt", b.Install(m, "", ubuntu), "sudo apt-get update", "sudo apt-get install -y build-essential")
	eq(t, "dnf", b.Install(m, "", fedora), "sudo dnf install -y @development-tools")
	eq(t, "dnf remove", b.Uninstall(m, fedora), "sudo dnf remove -y @development-tools")
	arch := platform.Platform{OS: platform.Linux, PkgManager: "pacman"}
	eq(t, "pacman", b.Install(catalog.Method{Package: "zsh"}, "", arch), "sudo pacman -S --needed --noconfirm zsh")
	eq(t, "pacman remove", b.Uninstall(catalog.Method{Package: "zsh"}, arch), "sudo pacman -R --noconfirm zsh")
	suse := platform.Platform{OS: platform.Linux, PkgManager: "zypper"}
	eq(t, "zypper", b.Install(catalog.Method{Package: "zsh"}, "", suse), "sudo zypper install -y zsh")
	c := b.Install(m, "", fedora)[0]
	if !c.Sudo || c.Lock != "dnf" {
		t.Fatalf("system cmd: %+v", c)
	}
	if b.Supports(m, platform.Platform{OS: platform.Linux, PkgManager: "pacman"}) {
		t.Fatal("no pacman mapping: must not support")
	}
	if b.Supports(catalog.Method{Package: "zsh"}, mac) {
		t.Fatal("system backend is Linux-only")
	}
}

func TestScript(t *testing.T) {
	b := get(t, "script")
	m := catalog.Method{Backend: "script", URL: "https://lerd.sh/install.sh", SHA256: "abc", Args: []string{"--yes"}}
	got := strs(b.Install(m, "", ubuntu))[0]
	for _, want := range []string{`curl -fsSL 'https://lerd.sh/install.sh' -o "$T"`, "abc", `bash "$T" '--yes'`} {
		if !strings.Contains(got, want) {
			t.Fatalf("script install %q missing %q", got, want)
		}
	}
	noSum := strs(b.Install(catalog.Method{URL: "https://x.dev/i.sh"}, "", ubuntu))[0]
	if strings.Contains(noSum, "256") {
		t.Fatalf("no checksum expected: %q", noSum)
	}
	if b.Uninstall(m, ubuntu) != nil {
		t.Fatal("script uninstall must be nil (manual)")
	}
	eq(t, "update", b.Update(m, ubuntu), strs(b.Install(m, "", ubuntu))...)
}

func TestGit(t *testing.T) {
	b := get(t, "git")
	m := catalog.Method{Backend: "git", Repo: "https://github.com/LazyVim/starter", Ref: "main", Dest: "~/.config/nvim"}
	got := strs(b.Install(m, "", ubuntu))[0]
	for _, want := range []string{
		"git clone --depth 1 --branch 'main' 'https://github.com/LazyVim/starter' '/home/dev/.config/nvim'",
		"git -C '/home/dev/.config/nvim' fetch --depth 1 origin 'main'",
		"checkout -q FETCH_HEAD",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("git install %q missing %q", got, want)
		}
	}
	eq(t, "uninstall", b.Uninstall(m, ubuntu), "rm -rf '/home/dev/.config/nvim'")
}

func TestFont(t *testing.T) {
	b := get(t, "font")
	m := catalog.Method{Backend: "font", URL: "https://example.com/JetBrainsMono.zip"}
	lin := strs(b.Install(m, "", ubuntu))[0]
	if !strings.Contains(lin, "unzip -o -j") || !strings.Contains(lin, "'/home/dev/.local/share/fonts'") || !strings.Contains(lin, "fc-cache -f") {
		t.Fatalf("linux font: %q", lin)
	}
	osx := strs(b.Install(m, "", mac))[0]
	if !strings.Contains(osx, "'/Users/dev/Library/Fonts'") || strings.Contains(osx, "fc-cache") {
		t.Fatalf("mac font: %q", osx)
	}
}

func TestHerdrPlugin(t *testing.T) {
	b := get(t, "herdr-plugin")
	m := catalog.Method{Backend: "herdr-plugin", Args: []string{"herdr-bar=jeffarese/herdr-bar@a88c7b0"}}
	eq(t, "install", b.Install(m, "", mac),
		"herdr plugin install jeffarese/herdr-bar --yes --ref a88c7b0", "herdr plugin enable 'herdr-bar' >/dev/null 2>&1 || true")
	eq(t, "update", b.Update(m, mac), "herdr plugin install jeffarese/herdr-bar --yes")
	eq(t, "uninstall", b.Uninstall(m, mac), "herdr plugin uninstall herdr-bar")
}

func TestChooseMacPrefersFirstCompatible(t *testing.T) {
	it := &catalog.Item{ID: "fzf", Install: []catalog.Method{{Backend: "brew", Package: "fzf"}, {Backend: "mise", Package: "aqua:junegunn/fzf"}}}
	m, b, err := Choose(it, mac)
	if err != nil || b.Name() != "brew" || m.Package != "fzf" {
		t.Fatalf("choose: %v %v %v", m, b, err)
	}
}

func TestChooseLinuxSkipsBrewWhenOS(t *testing.T) {
	it := &catalog.Item{ID: "fzf", Install: []catalog.Method{
		{Backend: "brew", Package: "fzf", When: &catalog.When{OS: platform.MacOS}},
		{Backend: "mise", Package: "aqua:junegunn/fzf"},
	}}
	_, b, err := Choose(it, ubuntu)
	if err != nil || b.Name() != "mise" {
		t.Fatalf("choose: %v %v", b, err)
	}
}

func TestChooseNone(t *testing.T) {
	it := &catalog.Item{ID: "x", Install: []catalog.Method{{Backend: "brew", Package: "x", When: &catalog.When{OS: platform.MacOS}}}}
	if _, _, err := Choose(it, ubuntu); err == nil || !strings.Contains(err.Error(), "linux") {
		t.Fatalf("want error, got %v", err)
	}
}

func TestBrewLatestParses(t *testing.T) {
	f := &runner.FakeRunner{Responses: map[string]runner.Result{"brew outdated --json=v2": {Output: `{"formulae":[{"name":"fzf","installed_versions":["0.55.0"],"current_version":"0.56.3"}],"casks":[{"name":"zed","installed_versions":"0.1","current_version":"0.2"}]}`}}}
	got := get(t, "brew").Latest(context.Background(), f, []catalog.Method{{Package: "fzf"}, {Package: "cask:zed"}})
	if got["fzf"] != "0.56.3" || got["cask:zed"] != "0.2" {
		t.Fatalf("latest: %v", got)
	}
}

func TestMiseLatestParses(t *testing.T) {
	f := &runner.FakeRunner{Responses: map[string]runner.Result{"mise outdated --json": {Output: `{"node":{"name":"node","requested":"22","current":"22.1.0","latest":"22.2.0"}}`}}}
	got := get(t, "mise").Latest(context.Background(), f, []catalog.Method{{Package: "node"}})
	if got["node"] != "22.2.0" {
		t.Fatalf("latest: %v", got)
	}
}

func TestScriptInline(t *testing.T) {
	eq(t, "inline", get(t, "script").Install(catalog.Method{Inline: "herdr plugin action invoke x"}, "", ubuntu), "herdr plugin action invoke x")
}

func TestChooseRespectsArch(t *testing.T) {
	it := &catalog.Item{ID: "procs", Install: []catalog.Method{
		{Backend: "mise", Package: "aqua:dalance/procs", When: &catalog.When{OS: platform.Linux, Arch: "amd64"}},
		{Backend: "system", Package: "procs"},
	}}
	arm := ubuntu
	arm.Arch = "arm64"
	if _, b, err := Choose(it, arm); err != nil || b.Name() != "system" {
		t.Fatalf("arm64: %v %v", b, err)
	}
	if _, b, err := Choose(it, ubuntu); err != nil || b.Name() != "mise" {
		t.Fatalf("amd64: %v %v", b, err)
	}
}
