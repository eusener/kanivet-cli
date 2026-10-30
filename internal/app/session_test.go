package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

type fixture struct {
	s       *Session
	fake    *runner.FakeRunner
	present map[string]bool
	mu      sync.Mutex
}

// newFixture usa o catálogo de testdata numa plataforma Linux falsa; "<x> --version" falha
// a não ser que x esteja em present (instalar via mise o marca como presente).
func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	i18n.SetLang(i18n.En)
	c, err := catalog.Load(os.DirFS("../catalog/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	f := &fixture{present: map[string]bool{}}
	f.fake = &runner.FakeRunner{Func: func(ctx context.Context, cmd runner.Cmd) (runner.Result, bool) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(cmd.Argv) >= 4 && cmd.Argv[0] == "mise" && cmd.Argv[1] == "use" {
			pkg := strings.Split(cmd.Argv[3], "@")[0]
			f.present[pkg[strings.LastIndex(pkg, "/")+1:]] = true
		}
		if id, ok := strings.CutSuffix(cmd.Shell, " --version"); ok {
			if f.present[id] {
				return runner.Result{Output: "1.0"}, true
			}
			return runner.Result{ExitCode: 1}, true
		}
		return runner.Result{}, false
	}}
	s, err := Open(Session{
		R: f.fake, Cat: c,
		P:        platform.Platform{OS: platform.Linux, Arch: "amd64", Distro: "ubuntu", PkgManager: "apt", Shell: "zsh", Systemd: true, Home: home},
		StateDir: filepath.Join(home, ".local", "state", "kanivet-cli"),
		Now:      func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) },
		Getenv:   map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.s = s
	return f
}

func TestSessionPlanAddsProfileItems(t *testing.T) {
	f := newFixture(t)
	pl, _, err := f.s.Plan(context.Background(), plan.Selection{Profile: "laravel"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"install:fzf", "install:lerd"} {
		if _, ok := pl.Step(id); !ok {
			t.Fatalf("missing %s in %+v", id, pl.Steps)
		}
	}
}

func TestSessionRunCreatesJournalAndState(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pl, _, err := f.s.Plan(ctx, plan.Selection{Add: []string{"fzf"}})
	if err != nil {
		t.Fatal(err)
	}
	rep, j, err := f.s.Run(ctx, pl, nil, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if j == nil || len(rep.Failed) > 0 {
		t.Fatalf("journal %v report %+v", j, rep)
	}
	if !f.s.St.Owned("fzf") {
		t.Fatalf("state %+v", f.s.St.Items)
	}
	if uj, _, err := f.s.Unfinished(); err != nil || uj != nil {
		t.Fatalf("unfinished %v err %v", uj, err)
	}
}

func TestSessionRunSetsProfile(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pl, _, _ := f.s.Plan(ctx, plan.Selection{Add: []string{"fzf"}})
	if _, _, err := f.s.Run(ctx, pl, nil, RunOptions{Profile: "laravel"}); err != nil {
		t.Fatal(err)
	}
	if f.s.St.Profile != "laravel" {
		t.Fatalf("profile %q", f.s.St.Profile)
	}
}

func TestSessionUnfinishedReturnsPendingOnly(t *testing.T) {
	f := newFixture(t)
	pl := &plan.Plan{Steps: []plan.Step{{ID: "a", Kind: plan.KindInstall}, {ID: "b", Kind: plan.KindInstall}}}
	data, _ := pl.MarshalJSON()
	j, err := state.NewJournal(f.s.StateDir, f.s.Now(), data)
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Set("a", state.StepDone)
	_ = j.Set("b", state.StepPending)
	gj, got, err := f.s.Unfinished()
	if err != nil || gj == nil || got == nil {
		t.Fatalf("journal %v plan %v err %v", gj, got, err)
	}
	if len(got.Steps) != 1 || got.Steps[0].ID != "b" {
		t.Fatalf("steps %+v", got.Steps)
	}
}

func TestSessionUnfinishedNone(t *testing.T) {
	f := newFixture(t)
	if j, p, err := f.s.Unfinished(); err != nil || j != nil || p != nil {
		t.Fatalf("%v %v %v", j, p, err)
	}
}

func TestSelectedForExclusiveByOS(t *testing.T) {
	f := newFixture(t)
	f.s.Cat.Profiles[0].ExclusiveByOS = map[platform.OS]map[string]string{platform.Linux: {"php-local": "flock"}}
	got := SelectedFor(f.s.Cat, f.s.P, plan.Selection{Profile: "laravel"})
	if !got["flock"] || got["lerd"] {
		t.Fatalf("selected %v", got)
	}
}

func TestExtrasFor(t *testing.T) {
	f := newFixture(t)
	f.s.Cat.Profiles[0].Extras = map[string][]string{"lazyvim": {"lang.php"}}
	if got := f.s.ExtrasFor("laravel"); len(got) != 1 || got[0] != "lang.php" {
		t.Fatalf("extras %v", got)
	}
}

func TestSessionRunPassesInteractiveHook(t *testing.T) {
	f := newFixture(t)
	pl := &plan.Plan{Steps: []plan.Step{{ID: "install:x", ItemID: "x", Kind: plan.KindInstall, NeedsSudo: true,
		Cmds: []runner.Cmd{{Shell: "true"}}}}}
	var calls []string
	hook := func(_ context.Context, c runner.Cmd) runner.Result {
		calls = append(calls, c.Shown())
		return runner.Result{}
	}
	if _, _, err := f.s.Run(context.Background(), pl, nil, RunOptions{RunInteractive: hook}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "sudo -v" {
		t.Fatalf("hook calls: %v", calls)
	}
	for _, c := range f.fake.Strings() {
		if c == "sudo -v" {
			t.Fatal("sudo -v reached the runner")
		}
	}
}

// I2: a detecção da sessão (o detectCmd da TUI) pode rodar junto com um run, que grava St.Items.
// Rode com -race: o detector precisa ler o estado sob o mesmo lock das escritas do executor.
func TestStatusesConcurrentWithRun(t *testing.T) {
	f := newFixture(t)
	pl, _, err := f.s.Plan(context.Background(), plan.Selection{Profile: "laravel"})
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				f.s.Statuses(context.Background(), plan.Selection{})
			}
		}
	}()
	if _, _, err := f.s.Run(context.Background(), pl, nil, RunOptions{}); err != nil {
		t.Fatal(err)
	}
	close(stop)
	<-done
}
