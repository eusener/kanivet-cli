package configs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/state"
)

func setup(t *testing.T) (*Applier, string) {
	t.Helper()
	home := t.TempDir()
	st, _ := state.Load(filepath.Join(home, "state.json"))
	files := fstest.MapFS{"configs/bat/config": {Data: []byte("--theme={{.OS}}\n")}}
	v := VarsFor(platform.Platform{OS: platform.Linux, Arch: "amd64", Home: home}, map[string]string{})
	return &Applier{St: st, BackupDir: filepath.Join(home, "backups", "run1"), Vars: v, Files: files}, home
}

var batFC = plan.FileChange{Target: "{{.XDG_CONFIG}}/bat/config", Template: "configs/bat/config", ItemID: "bat"}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRenderVars(t *testing.T) {
	v := Vars{HOME: "/h", XDG_CONFIG: "/h/.config"}
	out, err := Render(fstest.MapFS{"t": {Data: []byte("{{.HOME}}/x")}}, "t", v)
	if err != nil || string(out) != "/h/x" {
		t.Fatalf("%q %v", out, err)
	}
	if VarsFor(platform.Platform{Home: "/h"}, map[string]string{"XDG_CONFIG_HOME": "/xdg"}).XDG_CONFIG != "/xdg" {
		t.Fatal("XDG_CONFIG_HOME must win")
	}
}

func TestApplyBacksUpExisting(t *testing.T) {
	a, home := setup(t)
	target := filepath.Join(home, ".config", "bat", "config")
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	_ = os.WriteFile(target, []byte("old\n"), 0o600)
	if c, err := a.Apply(batFC, false); err != nil || c != nil {
		t.Fatalf("%v %v", c, err)
	}
	if read(t, target) != "--theme=linux\n" {
		t.Fatalf("content %q", read(t, target))
	}
	rec := a.St.Configs[target]
	if rec.Backup == "" || read(t, rec.Backup) != "old\n" || !strings.HasPrefix(rec.Backup, a.BackupDir) {
		t.Fatalf("backup %+v", rec)
	}
	if fi, _ := os.Stat(target); fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", fi.Mode().Perm())
	}
}

func TestApplyNewFileNoBackup(t *testing.T) {
	a, home := setup(t)
	if _, err := a.Apply(batFC, false); err != nil {
		t.Fatal(err)
	}
	if rec := a.St.Configs[filepath.Join(home, ".config", "bat", "config")]; rec.Backup != "" || rec.SHA256 == "" || rec.Item != "bat" {
		t.Fatalf("%+v", rec)
	}
}

func TestApplyConflictWhenUserEdited(t *testing.T) {
	a, home := setup(t)
	target := filepath.Join(home, ".config", "bat", "config")
	_, _ = a.Apply(batFC, false)
	_ = os.WriteFile(target, []byte("mine\n"), 0o644)
	c, err := a.Apply(batFC, false)
	if err != nil || c == nil || c.Target != target || !strings.Contains(c.Diff, "mine") {
		t.Fatalf("%+v %v", c, err)
	}
	if read(t, target) != "mine\n" {
		t.Fatal("file must stay untouched on conflict")
	}
	if c, err := a.Apply(batFC, true); err != nil || c != nil || read(t, target) != "--theme=linux\n" {
		t.Fatalf("force: %+v %v", c, err)
	}
}

func TestRemoveRestoresBackup(t *testing.T) {
	a, home := setup(t)
	target := filepath.Join(home, ".config", "bat", "config")
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	_ = os.WriteFile(target, []byte("old\n"), 0o644)
	_, _ = a.Apply(batFC, false)
	if err := a.Remove(batFC.Target); err != nil {
		t.Fatal(err)
	}
	if read(t, target) != "old\n" {
		t.Fatalf("restored %q", read(t, target))
	}
	if _, ok := a.St.Configs[target]; ok {
		t.Fatal("record must be gone")
	}
}

func TestRemoveDeletesWhenNoBackup(t *testing.T) {
	a, home := setup(t)
	target := filepath.Join(home, ".config", "bat", "config")
	_, _ = a.Apply(batFC, false)
	if err := a.Remove(batFC.Target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file must be deleted: %v", err)
	}
}

func TestEnsureRcBlockIdempotent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".zshrc")
	_ = os.WriteFile(rc, []byte("export A=1\n"), 0o644)
	if ch, err := EnsureRcBlock(rc, "zsh"); !ch || err != nil {
		t.Fatalf("%v %v", ch, err)
	}
	if ch, _ := EnsureRcBlock(rc, "zsh"); ch {
		t.Fatal("second call must not change")
	}
	got := read(t, rc)
	if strings.Count(got, RcMarkerStart) != 1 || !strings.HasPrefix(got, "export A=1\n") ||
		!strings.Contains(got, `[ -f "$HOME/.config/kanivet-cli/shell/init.zsh" ] && . "$HOME/.config/kanivet-cli/shell/init.zsh"`) {
		t.Fatalf("rc:\n%s", got)
	}
}

func TestRcSymlinkPreserved(t *testing.T) {
	home := t.TempDir()
	real := filepath.Join(home, "dotfiles", "zshrc")
	_ = os.MkdirAll(filepath.Dir(real), 0o755)
	_ = os.WriteFile(real, []byte("# mine\n"), 0o644)
	rc := filepath.Join(home, ".zshrc")
	_ = os.Symlink(real, rc)
	if _, err := EnsureRcBlock(rc, "zsh"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(rc); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("rc must still be a symlink")
	}
	if !strings.Contains(read(t, real), RcMarkerStart) {
		t.Fatal("real file must receive the block")
	}

	a, h := setup(t)
	cfgReal := filepath.Join(h, "dotfiles", "bat")
	target := filepath.Join(h, ".config", "bat", "config")
	_ = os.MkdirAll(filepath.Dir(cfgReal), 0o755)
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	_ = os.WriteFile(cfgReal, []byte("x\n"), 0o644)
	_ = os.Symlink(cfgReal, target)
	if _, err := a.Apply(batFC, false); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(target); fi.Mode()&os.ModeSymlink == 0 || read(t, cfgReal) != "--theme=linux\n" {
		t.Fatal("config symlink must be preserved and its target written")
	}
}

func TestRestoreRun(t *testing.T) {
	a, home := setup(t)
	target := filepath.Join(home, ".config", "bat", "config")
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	_ = os.WriteFile(target, []byte("old\n"), 0o644)
	_, _ = a.Apply(batFC, false)
	got, err := RestoreRun(a.BackupDir)
	if err != nil || len(got) != 1 || got[0] != target || read(t, target) != "old\n" {
		t.Fatalf("%v %v %q", got, err, read(t, target))
	}
}
