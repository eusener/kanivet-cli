package shellinit

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/catalog"
)

var update = flag.Bool("update", false, "regenerate golden files")

func fixture() []*catalog.Item {
	return []*catalog.Item{
		{ID: "zsh-syntax-highlighting", ZshOnly: true, Detect: catalog.Detect{Command: "test -f ~/.zsh-sh"},
			Shell: catalog.ShellSpec{InitOrder: 100, Init: map[string]string{"zsh": "source ~/.zsh-sh"}}},
		{ID: "atuin", Detect: catalog.Detect{Command: "atuin --version"},
			Shell: catalog.ShellSpec{InitOrder: 60, Init: map[string]string{"any": `eval "$(atuin init {{shell}} --disable-up-arrow)"`}}},
		{ID: "fzf", Detect: catalog.Detect{Command: "fzf --version"},
			Shell: catalog.ShellSpec{InitOrder: 55, Init: map[string]string{"any": `eval "$(fzf --{{shell}})"`},
				Env: map[string]string{"FZF_DEFAULT_COMMAND": "fd --type f --hidden --exclude .git"}}},
		{ID: "eza", Detect: catalog.Detect{Command: "eza --version"},
			Shell: catalog.ShellSpec{InitOrder: 50, Aliases: map[string]string{"ls": "eza --icons=auto", "ll": "eza -lah --git"}}},
		{ID: "starship", Detect: catalog.Detect{Command: "starship --version"},
			Shell: catalog.ShellSpec{InitOrder: 90, Init: map[string]string{"any": "if [ -z \"$STARSHIP_SHELL\" ]; then\n  eval \"$(starship init {{shell}})\"\nfi"}}},
		{ID: "bat", Detect: catalog.Detect{Command: "bat --version"}, Env: map[string]string{"MANROFFOPT": "-c"},
			Shell: catalog.ShellSpec{InitOrder: 50}},
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		_ = os.WriteFile(path, []byte(got), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", path, err)
	}
	if string(want) != got {
		t.Fatalf("%s mismatch:\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}

func TestGoldenZsh(t *testing.T)  { golden(t, "init.zsh.golden", Generate(fixture(), "zsh")) }
func TestGoldenBash(t *testing.T) { golden(t, "init.bash.golden", Generate(fixture(), "bash")) }

func TestOrderSyntaxHighlightingLast(t *testing.T) {
	out := Generate(fixture(), "zsh")
	sh := strings.Index(out, "source ~/.zsh-sh")
	for _, other := range []string{"atuin init", "fzf --zsh", "alias ls=", "starship init"} {
		if i := strings.Index(out, other); i < 0 || i > sh {
			t.Fatalf("%q must come before syntax highlighting (%d vs %d)", other, i, sh)
		}
	}
	if strings.Index(out, "fzf --zsh") > strings.Index(out, "atuin init") {
		t.Fatal("fzf must come before atuin")
	}
}

func TestBashOmitsZshOnly(t *testing.T) {
	if out := Generate(fixture(), "bash"); strings.Contains(out, "zsh-sh") || !strings.Contains(out, "fzf --bash") {
		t.Fatalf("bash output:\n%s", out)
	}
}

func TestGuardedByCommand(t *testing.T) {
	if out := Generate(fixture(), "zsh"); !strings.Contains(out, "if command -v eza >/dev/null 2>&1; then") {
		t.Fatalf("missing guard:\n%s", out)
	}
}

func TestGeneratedParses(t *testing.T) {
	for _, sh := range []string{"zsh", "bash"} {
		if _, err := exec.LookPath(sh); err != nil {
			t.Logf("%s not installed, skipping", sh)
			continue
		}
		f := filepath.Join(t.TempDir(), "init."+sh)
		_ = os.WriteFile(f, []byte(Generate(fixture(), sh)), 0o644)
		if out, err := exec.Command(sh, "-n", f).CombinedOutput(); err != nil {
			t.Fatalf("%s -n: %v\n%s", sh, err, out)
		}
	}
}

func TestGuardOverride(t *testing.T) {
	it := &catalog.Item{ID: "zsh-autosuggestions", Detect: catalog.Detect{Command: "for f in a b; do :; done"},
		Shell: catalog.ShellSpec{Guard: "zsh", Init: map[string]string{"zsh": "source x"}}}
	if out := Generate([]*catalog.Item{it}, "zsh"); !strings.Contains(out, "if command -v zsh >/dev/null 2>&1; then") {
		t.Fatalf("guard override not used:\n%s", out)
	}
}
