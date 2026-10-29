package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

var now = time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)

type env struct {
	ex      *Executor
	fake    *runner.FakeRunner
	present map[string]bool
	mu      sync.Mutex
	home    string
}

// newEnv monta um executor com FakeRunner: "<id> --version" falha, a não ser que o item esteja em present.
func newEnv(t *testing.T, items ...*catalog.Item) *env {
	t.Helper()
	home := t.TempDir()
	e := &env{present: map[string]bool{}, home: home}
	e.fake = &runner.FakeRunner{Func: func(ctx context.Context, c runner.Cmd) (runner.Result, bool) {
		if id, ok := strings.CutSuffix(c.Shell, " --version"); ok {
			e.mu.Lock()
			defer e.mu.Unlock()
			if e.present[id] {
				return runner.Result{Output: "1.0"}, true
			}
			return runner.Result{ExitCode: 1}, true
		}
		return runner.Result{}, false
	}}
	cat := &catalog.Catalog{Items: map[string]*catalog.Item{}}
	for _, it := range items {
		cat.Items[it.ID] = it
	}
	p := platform.Platform{OS: platform.Linux, Arch: "amd64", Shell: "zsh", Home: home}
	st, _ := state.Load(filepath.Join(home, "state.json"))
	j, err := state.NewJournal(home, now, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	e.ex = &Executor{
		R: e.fake, J: j, St: st, Cat: cat, P: p,
		Ap:       &configs.Applier{St: st, BackupDir: filepath.Join(home, "backups", j.RunID), Vars: configs.VarsFor(p, nil), Files: os.DirFS(home)},
		Det:      &detect.Detector{R: e.fake, P: p, Owner: st},
		ShellDir: filepath.Join(home, ".config", "kanivet-cli", "shell"),
		Rc:       filepath.Join(home, ".zshrc"),
	}
	return e
}

func item(id string) *catalog.Item {
	return &catalog.Item{ID: id, Name: i18n.Text{i18n.En: id}, Platforms: []platform.OS{platform.Linux},
		Detect: catalog.Detect{Command: id + " --version"}}
}

func install(id string, cmd string, deps ...string) plan.Step {
	return plan.Step{ID: "install:" + id, ItemID: id, Kind: plan.KindInstall, DependsOn: deps,
		Cmds: []runner.Cmd{{Shell: cmd}}, Backend: "mise", Package: id}
}

func TestFailureDoesNotAbortOthers(t *testing.T) {
	e := newEnv(t, item("a"), item("b"))
	e.fake.Responses = map[string]runner.Result{"install a": {ExitCode: 1}}
	pl := &plan.Plan{Steps: []plan.Step{install("a", "install a"), install("b", "install b")}}
	rep, err := e.ex.Execute(context.Background(), pl, Options{})
	if err != nil || len(rep.Failed) != 1 || rep.Failed[0] != "install:a" || len(rep.Done) != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if e.ex.J.Get("install:a") != state.StepFailed || e.ex.J.Get("install:b") != state.StepDone {
		t.Fatalf("journal %v", e.ex.J.Steps)
	}
}

func TestDependentSkipped(t *testing.T) {
	e := newEnv(t, item("a"), item("b"))
	e.fake.Responses = map[string]runner.Result{"install a": {ExitCode: 1}}
	pl := &plan.Plan{Steps: []plan.Step{install("a", "install a"), install("b", "install b", "install:a")}}
	rep, _ := e.ex.Execute(context.Background(), pl, Options{})
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "install:b" {
		t.Fatalf("%+v", rep)
	}
	for _, c := range e.fake.Strings() {
		if c == "install b" {
			t.Fatal("b must not run")
		}
	}
}

func TestLockSerializes(t *testing.T) {
	e := newEnv(t, item("a"), item("b"))
	var active, peak int32
	inner := e.fake.Func
	e.fake.Func = func(ctx context.Context, c runner.Cmd) (runner.Result, bool) {
		if c.Lock == "brew" {
			n := atomic.AddInt32(&active, 1)
			for {
				p := atomic.LoadInt32(&peak)
				if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
					break
				}
			}
			time.Sleep(30 * time.Millisecond)
			atomic.AddInt32(&active, -1)
			return runner.Result{}, true
		}
		return inner(ctx, c)
	}
	a, b := install("a", ""), install("b", "")
	a.Cmds = []runner.Cmd{{Argv: []string{"brew", "install", "a"}, Lock: "brew"}}
	b.Cmds = []runner.Cmd{{Argv: []string{"brew", "install", "b"}, Lock: "brew"}}
	if _, err := e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{a, b}}, Options{Concurrency: 4}); err != nil {
		t.Fatal(err)
	}
	if peak != 1 {
		t.Fatalf("peak concurrency on brew lock = %d", peak)
	}
}

func TestSudoOnce(t *testing.T) {
	e := newEnv(t, item("a"), item("b"), item("c"))
	var steps []plan.Step
	for _, id := range []string{"a", "b", "c"} {
		s := install(id, "x")
		s.Cmds = []runner.Cmd{{Argv: []string{"apt-get", "install", "-y", id}, Sudo: true}}
		s.NeedsSudo = true
		steps = append(steps, s)
	}
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: steps}, Options{})
	n := 0
	for _, c := range e.fake.Strings() {
		if c == "sudo -v" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("sudo -v called %d times: %v", n, e.fake.Strings())
	}
}

func TestSudoFailureSkipsSudoSteps(t *testing.T) {
	e := newEnv(t, item("a"), item("b"))
	e.fake.Responses = map[string]runner.Result{"sudo -v": {ExitCode: 1}}
	s := install("a", "x")
	s.NeedsSudo = true
	rep, _ := e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{s, install("b", "install b")}}, Options{})
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "install:a" || len(rep.Done) != 1 {
		t.Fatalf("%+v", rep)
	}
}

func TestResumeSkipsSatisfied(t *testing.T) {
	e := newEnv(t, item("a"))
	_ = e.ex.J.Set("install:a", state.StepRunning)
	e.present["a"] = true
	rep, _ := e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{install("a", "install a")}}, Options{})
	for _, c := range e.fake.Strings() {
		if c == "install a" {
			t.Fatal("satisfied step must not run again")
		}
	}
	if len(rep.Done) != 1 || !e.ex.St.Owned("a") {
		t.Fatalf("%+v owned=%v", rep, e.ex.St.Owned("a"))
	}
}

func TestResumeSkipsDoneSteps(t *testing.T) {
	e := newEnv(t, item("a"))
	_ = e.ex.J.Set("install:a", state.StepDone)
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{install("a", "install a")}}, Options{})
	if len(e.fake.Calls) != 0 {
		t.Fatalf("done step re-ran: %v", e.fake.Strings())
	}
}

func TestCancelLeavesRunning(t *testing.T) {
	e := newEnv(t, item("a"))
	inner := e.fake.Func
	e.fake.Func = func(ctx context.Context, c runner.Cmd) (runner.Result, bool) {
		if c.Shell == "install a" {
			<-ctx.Done()
			return runner.Result{ExitCode: -1, Err: ctx.Err()}, true
		}
		return inner(ctx, c)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := e.ex.Execute(ctx, &plan.Plan{Steps: []plan.Step{install("a", "install a")}}, Options{})
	if err == nil || e.ex.J.Get("install:a") != state.StepRunning {
		t.Fatalf("err=%v journal=%v", err, e.ex.J.Steps)
	}
}

func TestStateUpdated(t *testing.T) {
	e := newEnv(t, item("fzf"))
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{install("fzf", "install fzf")}}, Options{})
	if !e.ex.St.Owned("fzf") || e.ex.St.Items["fzf"].Backend != "mise" {
		t.Fatalf("state %+v", e.ex.St.Items)
	}
	saved, _ := state.Load(e.ex.St.Path)
	if !saved.Owned("fzf") {
		t.Fatal("state must be saved to disk")
	}
	un := plan.Step{ID: "uninstall:fzf", ItemID: "fzf", Kind: plan.KindUninstall, Cmds: []runner.Cmd{{Shell: "rm fzf"}}}
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{un}}, Options{})
	if e.ex.St.Owned("fzf") {
		t.Fatal("uninstall must drop ownership")
	}
}

func TestShellStepWritesInitAndRc(t *testing.T) {
	eza := item("eza")
	eza.Shell = catalog.ShellSpec{Aliases: map[string]string{"ls": "eza"}}
	e := newEnv(t, eza)
	pl := &plan.Plan{Steps: []plan.Step{install("eza", "install eza"), {ID: plan.ShellStepID, Kind: plan.KindShell, DependsOn: []string{"install:eza"}}}}
	if _, err := e.ex.Execute(context.Background(), pl, Options{}); err != nil {
		t.Fatal(err)
	}
	zsh, err := os.ReadFile(filepath.Join(e.ex.ShellDir, "init.zsh"))
	if err != nil || !strings.Contains(string(zsh), "alias ls='eza'") {
		t.Fatalf("init.zsh %q %v", zsh, err)
	}
	if _, err := os.Stat(filepath.Join(e.ex.ShellDir, "init.bash")); err != nil {
		t.Fatal(err)
	}
	rc, _ := os.ReadFile(e.ex.Rc)
	if !strings.Contains(string(rc), configs.RcMarkerStart) {
		t.Fatalf("rc %q", rc)
	}
}

func TestDryRunExecutesNothing(t *testing.T) {
	e := newEnv(t, item("a"))
	events := make(chan Event, 16)
	_, err := e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{install("a", "install a")}}, Options{DryRun: true, Events: events})
	close(events)
	if err != nil || len(e.fake.Calls) != 0 {
		t.Fatalf("calls %v err %v", e.fake.Strings(), err)
	}
	found := false
	for ev := range events {
		if ev.Line == "install a" {
			found = true
		}
	}
	if !found {
		t.Fatal("dry run must emit the command as an event line")
	}
}

func TestShellStepRunsDespiteFailedDependency(t *testing.T) {
	eza := item("eza")
	eza.Shell = catalog.ShellSpec{Aliases: map[string]string{"ls": "eza"}}
	e := newEnv(t, eza, item("procs"))
	e.fake.Responses = map[string]runner.Result{"install procs": {ExitCode: 1}}
	pl := &plan.Plan{Steps: []plan.Step{install("eza", "install eza"), install("procs", "install procs"),
		{ID: plan.ShellStepID, Kind: plan.KindShell, DependsOn: []string{"install:eza", "install:procs"}}}}
	rep, _ := e.ex.Execute(context.Background(), pl, Options{})
	if e.ex.J.Get(plan.ShellStepID) != state.StepDone || len(rep.Failed) != 1 {
		t.Fatalf("shell step must still run: %+v %v", rep, e.ex.J.Steps)
	}
	if data, _ := os.ReadFile(filepath.Join(e.ex.ShellDir, "init.zsh")); !strings.Contains(string(data), "alias ls='eza'") {
		t.Fatalf("init.zsh %q", data)
	}
}

// I2: detecção concorrente e registro de dono não podem disputar o mapa do estado.
func TestConcurrentInstallsNoRace(t *testing.T) {
	var items []*catalog.Item
	var steps []plan.Step
	for i := 0; i < 48; i++ {
		id := fmt.Sprintf("tool%02d", i)
		items = append(items, item(id))
		steps = append(steps, install(id, "install "+id))
	}
	e := newEnv(t, items...)
	for i := 0; i < 48; i += 2 {
		e.present[fmt.Sprintf("tool%02d", i)] = true
	}
	e.ex.Det.Owner = e.ex.St
	if _, err := e.ex.Execute(context.Background(), &plan.Plan{Steps: steps}, Options{Concurrency: 16}); err != nil {
		t.Fatal(err)
	}
}

// I3: externo velho demais é reinstalado, não "já presente".
func TestInstallRunsOverOutdatedExternal(t *testing.T) {
	node := item("node")
	node.MinVersion = "18"
	node.Detect.VersionRegex = `([\d.]+)`
	e := newEnv(t, node)
	e.present["node"] = true // responde "1.0"
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{install("node", "install node")}}, Options{})
	found := false
	for _, c := range e.fake.Strings() {
		found = found || c == "install node"
	}
	if !found {
		t.Fatalf("outdated external must be installed: %v", e.fake.Strings())
	}
}

// I5: o rc ganha backup antes do bloco ser adicionado.
func TestShellStepBacksUpRc(t *testing.T) {
	e := newEnv(t, item("eza"))
	_ = os.WriteFile(e.ex.Rc, []byte("# meu zshrc\n"), 0o644)
	if _, err := e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{{ID: plan.ShellStepID, Kind: plan.KindShell}}}, Options{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(e.ex.Ap.BackupDir, strings.TrimPrefix(e.ex.Rc, "/")))
	if err != nil || string(b) != "# meu zshrc\n" {
		t.Fatalf("rc backup %q %v", b, err)
	}
}

// I6: com chsh para zsh, o ~/.zshrc também recebe o bloco.
func TestChshAlsoWritesZshrc(t *testing.T) {
	e := newEnv(t, item("zsh"))
	e.ex.P.Shell = "bash"
	e.ex.Rc = filepath.Join(e.home, ".bashrc")
	pl := &plan.Plan{Steps: []plan.Step{
		{ID: "chsh:zsh", ItemID: "zsh", Kind: plan.KindChsh, Cmds: []runner.Cmd{{Shell: "chsh"}}},
		{ID: plan.ShellStepID, Kind: plan.KindShell},
	}}
	_, _ = e.ex.Execute(context.Background(), pl, Options{})
	for _, rc := range []string{".bashrc", ".zshrc"} {
		b, _ := os.ReadFile(filepath.Join(e.home, rc))
		if !strings.Contains(string(b), configs.RcMarkerStart) {
			t.Fatalf("%s without block: %q", rc, b)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(e.home, ".zshrc")); !strings.Contains(string(b), "init.zsh") {
		t.Fatalf(".zshrc must load init.zsh: %q", b)
	}
}

// I9: desinstalar uma distro Neovim guarda a config atual e devolve a original.
func TestUninstallDirConfigRestoresOriginal(t *testing.T) {
	e := newEnv(t, item("lazyvim"))
	nvim := filepath.Join(e.home, ".config", "nvim")
	_ = os.MkdirAll(nvim, 0o755)
	_ = os.WriteFile(filepath.Join(nvim, "init.lua"), []byte("-- original\n"), 0o644)
	fc := plan.FileChange{Target: "~/.config/nvim", ItemID: "lazyvim", Dir: true}
	inst := install("lazyvim", "mkdir -p "+nvim)
	inst.DependsOn = []string{"config:nvim-backup"}
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{
		{ID: "config:nvim-backup", ItemID: "lazyvim", Kind: plan.KindConfig, Files: []plan.FileChange{fc}}, inst}}, Options{})
	_ = os.MkdirAll(nvim, 0o755)
	_ = os.WriteFile(filepath.Join(nvim, "init.lua"), []byte("-- lazyvim custom\n"), 0o644)

	j2, _ := state.NewJournal(e.home, now.Add(time.Hour), []byte(`{}`))
	e.ex.J = j2
	e.ex.Ap.BackupDir = filepath.Join(e.home, "backups", j2.RunID)
	rm := fc
	rm.Remove = true
	_, _ = e.ex.Execute(context.Background(), &plan.Plan{Steps: []plan.Step{{ID: "uninstall:lazyvim", ItemID: "lazyvim", Kind: plan.KindUninstall, Files: []plan.FileChange{rm}}}}, Options{})
	if b, _ := os.ReadFile(filepath.Join(nvim, "init.lua")); string(b) != "-- original\n" {
		t.Fatalf("original config not restored: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(e.ex.Ap.BackupDir, strings.TrimPrefix(nvim, "/"), "init.lua")); string(b) != "-- lazyvim custom\n" {
		t.Fatalf("customized config must be backed up: %q", b)
	}
}
