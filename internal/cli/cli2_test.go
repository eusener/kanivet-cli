package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eusener/kanivet-cli/internal/state"
)

func (h *harness) own(ids ...string) {
	st, _ := state.Load(filepath.Join(h.env.StateDir, "state.json"))
	for _, id := range ids {
		st.Items[id] = state.InstalledItem{Backend: "mise", Package: id, At: time.Now()}
		h.present[id] = true
	}
	_ = st.Save()
}

func TestUpdateOnlyOwned(t *testing.T) {
	h := newHarness(t)
	h.own("fzf")
	h.present["lerd"] = true
	if code := h.run("update", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, h.out, h.errOut)
	}
	if !h.ran("mise upgrade aqua:junegunn/fzf") {
		t.Fatalf("calls %v", h.fake.Strings())
	}
	for _, c := range h.fake.Strings() {
		if strings.Contains(c, "lerd.sh") {
			t.Fatalf("external lerd must not be updated: %v", h.fake.Strings())
		}
	}
}

func TestUpdateLanguageStaysInLine(t *testing.T) {
	h := newHarness(t)
	h.own("node", "mise")
	if code := h.run("update", "node", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
	if !h.ran("mise upgrade node") {
		t.Fatalf("calls %v", h.fake.Strings())
	}
	for _, c := range h.fake.Strings() {
		if strings.Contains(c, "node@latest") {
			t.Fatalf("must stay in line: %v", h.fake.Strings())
		}
	}
}

func TestUninstallRemovesConfigsAndShell(t *testing.T) {
	h := newHarness(t)
	if code := h.run("install", "fzf", "--yes"); code != 0 {
		t.Fatalf("install exit %d: %s", code, h.out)
	}
	cfg := filepath.Join(h.env.P.Home, ".config", "fzf", "fzfrc")
	initZsh := filepath.Join(h.env.P.Home, ".config", "kanivet-cli", "shell", "init.zsh")
	if data, _ := os.ReadFile(initZsh); !strings.Contains(string(data), "fzf --zsh") {
		t.Fatalf("init before uninstall:\n%s", data)
	}
	if code := h.run("uninstall", "fzf", "--yes"); code != 0 {
		t.Fatalf("uninstall exit %d: %s %s", code, h.out, h.errOut)
	}
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatalf("config must be removed: %v", err)
	}
	if data, _ := os.ReadFile(initZsh); strings.Contains(string(data), "fzf --zsh") {
		t.Fatalf("init after uninstall still has fzf:\n%s", data)
	}
	st, _ := state.Load(filepath.Join(h.env.StateDir, "state.json"))
	if st.Owned("fzf") {
		t.Fatal("fzf must not be owned anymore")
	}
}

func TestUninstallOrphansNotRemovedByDefault(t *testing.T) {
	h := newHarness(t)
	h.own("node", "mise")
	if code := h.run("uninstall", "node", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, h.out, h.errOut)
	}
	if !h.ran("mise uninstall --all node") || !strings.Contains(h.out.String(), "mise") {
		t.Fatalf("calls %v\n%s", h.fake.Strings(), h.out)
	}
	st, _ := state.Load(filepath.Join(h.env.StateDir, "state.json"))
	if !st.Owned("mise") {
		t.Fatal("orphan mise must stay installed")
	}
}

func TestUninstallScriptItemWarnsManual(t *testing.T) {
	h := newHarness(t)
	h.own("lerd")
	if code := h.run("uninstall", "lerd", "--yes"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.out.String(), "manual uninstall") {
		t.Fatalf("output:\n%s", h.out)
	}
	st, _ := state.Load(filepath.Join(h.env.StateDir, "state.json"))
	if !st.Owned("lerd") {
		t.Fatal("lerd must stay in state")
	}
}

func TestRestoreListsAndRestores(t *testing.T) {
	h := newHarness(t)
	cfg := filepath.Join(h.env.P.Home, ".config", "fzf", "fzfrc")
	_ = os.MkdirAll(filepath.Dir(cfg), 0o755)
	_ = os.WriteFile(cfg, []byte("mine\n"), 0o644)
	if code := h.run("install", "fzf", "--yes"); code != 0 {
		t.Fatalf("install exit %d", code)
	}
	if code := h.run("restore"); code != 0 || !strings.Contains(h.out.String(), "20261029-120000") {
		t.Fatalf("restore list exit %d:\n%s", code, h.out)
	}
	if code := h.run("restore", "20261029-120000"); code != 0 {
		t.Fatalf("restore exit %d: %s", code, h.errOut)
	}
	if data, _ := os.ReadFile(cfg); string(data) != "mine\n" {
		t.Fatalf("restored %q", data)
	}
}

func TestCatalogLintEmbedded(t *testing.T) {
	h := newHarness(t)
	if code := h.run("catalog", "lint"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.out)
	}
}

func TestCatalogLintBrokenDir(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "items"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "catalog.yaml"), []byte("schema: 1\ncategories: []\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "items", "x.yaml"), []byte("id: x\ncategory: nope\nname: x\ndescription: x\nplatforms: [linux]\n"), 0o644)
	if code := h.run("catalog", "lint", "--dir", dir); code != 1 || !strings.Contains(h.out.String(), "x: category") {
		t.Fatalf("exit %d:\n%s", code, h.out)
	}
}

func TestDoctor(t *testing.T) {
	h := newHarness(t)
	if code := h.run("doctor"); code != 0 || !strings.Contains(h.out.String(), ".zshrc") {
		t.Fatalf("exit %d:\n%s", code, h.out)
	}
}
