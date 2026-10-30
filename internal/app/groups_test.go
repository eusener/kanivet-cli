package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/runner"
)

func findGroup(t *testing.T, gs []Group, k GroupKind) Group {
	t.Helper()
	for _, g := range gs {
		if g.Kind == k {
			return g
		}
	}
	t.Fatalf("group %s not found in %+v", k, gs)
	return Group{}
}

func TestGroupPlanMarksExistingAndRc(t *testing.T) {
	f := newFixture(t)
	existing := filepath.Join(f.s.P.Home, "existing.conf")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc := f.s.RcPath()
	if err := os.WriteFile(rc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pl := &plan.Plan{Steps: []plan.Step{
		{ID: "config:a", Kind: plan.KindConfig, Files: []plan.FileChange{{Target: existing}, {Target: filepath.Join(f.s.P.Home, "new.conf")}}},
		{ID: plan.ShellStepID, Kind: plan.KindShell},
	}}
	g := findGroup(t, f.s.GroupPlan(pl), GroupConfig)
	files := g.Lines[0].Files
	if !files[0].Exists || files[1].Exists {
		t.Fatalf("files %+v", files)
	}
	shell := g.Lines[1].Files
	last := shell[len(shell)-1]
	if !last.Rc || last.Path != rc || !last.Exists {
		t.Fatalf("shell files %+v", shell)
	}
}

func TestGroupPlanCompactsMultilineCmds(t *testing.T) {
	f := newFixture(t)
	pl := &plan.Plan{Steps: []plan.Step{{ID: "install:x", Kind: plan.KindInstall, Backend: "mise",
		Cmds: []runner.Cmd{{Shell: "echo one\necho two"}, {Shell: "echo single"}}}}}
	g := findGroup(t, f.s.GroupPlan(pl), GroupInstall)
	got := g.Lines[0].Cmds
	if len(got) != 2 || got[0] != "echo one …" || got[1] != "echo single" {
		t.Fatalf("cmds %q", got)
	}
	if g.Lines[0].StepID != "install:x" || g.Lines[0].Text != "install:x  (mise)" {
		t.Fatalf("line %+v", g.Lines[0])
	}
}

func TestGroupPlanSudoSkipOrphansWarnings(t *testing.T) {
	f := newFixture(t)
	pl := &plan.Plan{
		Steps:    []plan.Step{{ID: "install:a", Kind: plan.KindInstall, NeedsSudo: true}, {ID: "update:b", Kind: plan.KindUpdate}},
		Skipped:  []plan.Skip{{ItemID: "z", Reason: "no support"}},
		Orphans:  []string{"o1", "o2"},
		Warnings: []string{"careful"},
	}
	gs := f.s.GroupPlan(pl)
	if findGroup(t, gs, GroupSudo).Lines[0].StepID != "install:a" || findGroup(t, gs, GroupUpdate).Lines[0].StepID != "update:b" {
		t.Fatalf("%+v", gs)
	}
	if findGroup(t, gs, GroupSkip).Lines[0].Text != "z — no support" ||
		findGroup(t, gs, GroupOrphans).Lines[0].Text != "o1, o2" ||
		findGroup(t, gs, GroupWarnings).Lines[0].Text != "careful" {
		t.Fatalf("%+v", gs)
	}
	for _, g := range gs {
		if g.Kind == GroupInstall {
			t.Fatalf("sudo step leaked into install group")
		}
	}
}

func TestGroupPlanListsZshrcWhenChsh(t *testing.T) {
	f := newFixture(t)
	f.s.P.Shell = "bash"
	zshrc := filepath.Join(f.s.P.Home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pl := &plan.Plan{Steps: []plan.Step{
		{ID: plan.ShellStepID, Kind: plan.KindShell},
		{ID: "chsh:zsh", ItemID: "zsh", Kind: plan.KindChsh},
	}}
	g := findGroup(t, f.s.GroupPlan(pl), GroupConfig)
	var rcs []FileLine
	for _, fl := range g.Lines[0].Files {
		if fl.Rc {
			rcs = append(rcs, fl)
		}
	}
	if len(rcs) != 2 || rcs[0].Path != f.s.RcPath() || rcs[1].Path != zshrc || !rcs[1].Exists {
		t.Fatalf("com chsh, o ~/.zshrc também recebe o bloco e precisa aparecer: %+v", g.Lines[0].Files)
	}
	// sem chsh, só o rc do shell atual
	pl.Steps = pl.Steps[:1]
	g = findGroup(t, f.s.GroupPlan(pl), GroupConfig)
	for _, fl := range g.Lines[0].Files {
		if fl.Path == zshrc {
			t.Fatalf("sem chsh o ~/.zshrc não é tocado: %+v", g.Lines[0].Files)
		}
	}
}
