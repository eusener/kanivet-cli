package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/runner"
	"github.com/eusener/kanivet-cli/internal/state"
)

func TestListJSON(t *testing.T) {
	h := newHarness(t)
	if code := h.run("list", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
	for _, id := range []string{`"fzf"`, `"lerd"`, `"flock"`} {
		if !strings.Contains(h.out.String(), id) {
			t.Fatalf("missing %s in %s", id, h.out)
		}
	}
}

func TestProfilesList(t *testing.T) {
	h := newHarness(t)
	if code := h.run("profiles"); code != 0 || !strings.Contains(h.out.String(), "laravel") {
		t.Fatalf("exit %d: %s", code, h.out)
	}
}

func TestStatusJSONShape(t *testing.T) {
	h := newHarness(t)
	h.present["fzf"] = true
	if code := h.run("status", "--json"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("json: %v\n%s", err, h.out)
	}
	var keys []string
	for k := range rows[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "category,id,latest,reason,status,version" {
		t.Fatalf("keys %v", keys)
	}
	for _, r := range rows {
		if r["id"] == "fzf" && r["status"] != "external" {
			t.Fatalf("fzf row %v", r)
		}
	}
}

func TestPlanPrintsGroups(t *testing.T) {
	h := newHarness(t)
	if code := h.run("plan", "--profile", "laravel", "--add", "flock", "--remove", "fzf"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, h.out, h.errOut)
	}
	out := h.out.String()
	if !strings.Contains(out, "Install") || !strings.Contains(out, "flock") || strings.Contains(out, "install:lerd") {
		t.Fatalf("plan output:\n%s", out)
	}
}

func TestPlanShowsSkipped(t *testing.T) {
	h := newHarness(t)
	h.env.Cat.Items["flock"].Requires.Distros = []string{"fedora"}
	if code := h.run("plan", "--add", "flock"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.out.String(), "Skip") || !strings.Contains(h.out.String(), "fedora") {
		t.Fatalf("plan output:\n%s", h.out)
	}
}

func TestPlanExclusiveConflictFails(t *testing.T) {
	h := newHarness(t)
	if code := h.run("plan", "--add", "lerd", "--add", "flock"); code != 1 || !strings.Contains(h.errOut.String(), "php-local") {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
}

func TestInstallNonTTYNeedsYes(t *testing.T) {
	h := newHarness(t)
	h.env.IsTTY = false
	if code := h.run("install", "--profile", "laravel"); code != 2 || !strings.Contains(h.errOut.String(), "--yes") {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
}

func TestInstallYesRuns(t *testing.T) {
	h := newHarness(t)
	if code := h.run("install", "--profile", "laravel", "--yes"); code != 0 {
		t.Fatalf("exit %d:\n%s\n%s", code, h.out, h.errOut)
	}
	if !h.ran("mise use -g aqua:junegunn/fzf@latest") {
		t.Fatalf("calls %v", h.fake.Strings())
	}
	st, _ := state.Load(filepath.Join(h.env.StateDir, "state.json"))
	if !st.Owned("fzf") || !st.Owned("lerd") {
		t.Fatalf("state %+v", st.Items)
	}
	if _, err := os.Stat(filepath.Join(h.env.P.Home, ".config", "fzf", "fzfrc")); err != nil {
		t.Fatalf("config not applied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.env.P.Home, ".config", "kanivet-cli", "shell", "init.zsh")); err != nil {
		t.Fatalf("shell init not written: %v", err)
	}
}

func TestInstallItemsAsArgs(t *testing.T) {
	h := newHarness(t)
	if code := h.run("install", "fzf", "--yes"); code != 0 || !h.ran("mise use -g aqua:junegunn/fzf@latest") || h.ran("sh") {
		t.Fatalf("exit %d calls %v", code, h.fake.Strings())
	}
}

func TestInstallDryRun(t *testing.T) {
	h := newHarness(t)
	if code := h.run("install", "--profile", "laravel", "--dry-run"); code != 0 {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
	for _, c := range h.fake.Strings() {
		if !strings.HasSuffix(c, "--version") {
			t.Fatalf("dry run executed %q", c)
		}
	}
	if !strings.Contains(h.out.String(), "fzf") {
		t.Fatalf("plan not printed:\n%s", h.out)
	}
}

func TestInstallExitCodeOnFailure(t *testing.T) {
	h := newHarness(t)
	h.fake.Responses = map[string]runner.Result{"mise use -g aqua:junegunn/fzf@latest": {ExitCode: 1, Output: "boom"}}
	if code := h.run("install", "fzf", "--yes"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(h.out.String(), filepath.Join("logs", "fzf.log")) {
		t.Fatalf("summary must show the log path:\n%s", h.out)
	}
}

func TestInstallResumePrompt(t *testing.T) {
	h := newHarness(t)
	pl := &plan.Plan{Steps: []plan.Step{
		{ID: "install:fzf", ItemID: "fzf", Kind: plan.KindInstall, Cmds: []runner.Cmd{{Shell: "echo fzf"}}},
		{ID: "install:lerd", ItemID: "lerd", Kind: plan.KindInstall, Cmds: []runner.Cmd{{Shell: "echo lerd"}}},
	}}
	data, _ := pl.MarshalJSON()
	j, _ := state.NewJournal(h.env.StateDir, h.env.Now(), data)
	_ = j.Set("install:fzf", state.StepDone)
	_ = j.Set("install:lerd", state.StepPending)
	if code := h.run("install", "--yes"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, h.out, h.errOut)
	}
	if h.ran("echo fzf") || !h.ran("echo lerd") {
		t.Fatalf("calls %v", h.fake.Strings())
	}
}

func TestPtBRMessages(t *testing.T) {
	h := newHarness(t)
	h.env.IsTTY = false
	h.env.Getenv["LANG"] = "pt_BR.UTF-8"
	if code := h.run("install", "fzf"); code != 2 || !strings.Contains(h.errOut.String(), "modo não interativo") {
		t.Fatalf("exit %d: %s", code, h.errOut)
	}
}
