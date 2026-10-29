package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

func keyRune(r string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }

// summaryModel monta o resumo com 1 sucesso, 1 falha (com log) e 1 conflito de config.
func summaryModel(t *testing.T) (Model, string) {
	t.Helper()
	i18n.SetLang(i18n.En)
	s := okSession(t)
	m := New(s, nil, ScreenSummary)
	m = send(m, size(100, 30))
	j, err := state.NewJournal(s.StateDir, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := j.LogWriter("node")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for i := 1; i <= 250; i++ {
		fmt.Fprintf(&sb, "log line %d\n", i)
	}
	w.Write([]byte(sb.String()))
	w.Close()
	target := s.ExpandTarget("{{.XDG_CONFIG}}/fzf/fzfrc")
	m.plan = &plan.Plan{Steps: []plan.Step{
		{ID: "install:fzf", ItemID: "fzf", Kind: plan.KindInstall},
		{ID: "install:node", ItemID: "node", Kind: plan.KindInstall},
		{ID: "config:fzf-defaults", ItemID: "fzf", Kind: plan.KindConfig},
		{ID: plan.ShellStepID, Kind: plan.KindShell},
	}}
	m.runJ = j
	m.runRep = executor.Report{
		Done:      []string{"install:fzf", "config:fzf-defaults", plan.ShellStepID},
		Failed:    []string{"install:node"},
		Conflicts: []configs.Conflict{{Target: target, Diff: "-old line\n+new line\n"}},
	}
	return m, target
}

func TestSummaryGolden(t *testing.T) {
	m, _ := summaryModel(t)
	m.s.Cat.Items["fzf"].Notes = i18n.Text{i18n.En: "Run fzf --help once."}
	m.runJ = &state.Journal{Dir: "/state/runs/R1"}
	m.runRep.Conflicts[0].Target = "/home/u/.config/fzf/fzfrc"
	m = send(m, size(80, 24))
	m.th = NewTheme(map[string]string{"NO_COLOR": "1"}, false)
	golden(t, "summary.golden", trimView(m.View()))
}

func TestSummaryOpensLog(t *testing.T) {
	m, _ := summaryModel(t)
	m = send(m, enter())
	v := m.View()
	if !strings.Contains(v, "log line 250") {
		t.Fatalf("o log deve abrir no fim:\n%s", v)
	}
	top := m
	for i := 0; i < 30; i++ {
		top = send(top, tea.KeyMsg{Type: tea.KeyPgUp})
	}
	tv := top.View()
	if !strings.Contains(tv, "log line 51 ") || strings.Contains(tv, "log line 50 ") {
		t.Fatalf("o topo deve ser a linha 51 (200 linhas):\n%s", tv)
	}
	m = send(m, esc())
	if strings.Contains(m.View(), "log line 250") {
		t.Fatal("esc não fechou o log")
	}
}

func TestSummaryDiffOfConflict(t *testing.T) {
	m, _ := summaryModel(t)
	m = send(m, keyRune("j"), keyRune("d"))
	if v := m.View(); !strings.Contains(v, "+new line") {
		t.Fatalf("sem diff:\n%s", v)
	}
	m = send(m, esc())
	if strings.Contains(m.View(), "+new line") {
		t.Fatal("esc não fechou o diff")
	}
}

func TestSummaryForceConflictAsksConfirmation(t *testing.T) {
	m, target := summaryModel(t)
	if err := os.MkdirAll(target[:strings.LastIndex(target, "/")], 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(target, []byte("mine\n"), 0o644)

	m = send(m, keyRune("j")) // cursor no conflito
	nm, cmd := m.Update(keyRune("F"))
	m = nm.(Model)
	if cmd != nil || !strings.Contains(m.View(), "[y/N]") {
		t.Fatalf("F deve só pedir confirmação:\n%s", m.View())
	}
	nm, cmd = m.Update(keyRune("n"))
	m = nm.(Model)
	if cmd != nil || strings.Contains(m.View(), "[y/N]") {
		t.Fatal("n deve cancelar")
	}
	if b, _ := os.ReadFile(target); string(b) != "mine\n" {
		t.Fatal("arquivo mexido sem confirmação")
	}

	m = send(m, keyRune("F"))
	nm, cmd = m.Update(keyRune("y"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("y deve produzir um cmd")
	}
	msg := cmd()
	if b, _ := os.ReadFile(target); string(b) == "mine\n" {
		t.Fatal("config não foi reaplicada com força")
	}
	m = send(m, msg)
	if v := m.View(); !strings.Contains(v, "reapplied") {
		t.Fatalf("sem resultado:\n%s", v)
	}
	// só o passo daquela config rodou: o journal novo tem um passo config:fzf-defaults
	if _, ok := msg.(forceDoneMsg); !ok {
		t.Fatalf("msg = %T", msg)
	}
}

func TestSummaryShowsItemNotes(t *testing.T) {
	m, _ := summaryModel(t)
	m.s.Cat.Items["fzf"].Notes = i18n.Text{i18n.En: "Run fzf --help once."}
	v := m.View()
	for _, want := range []string{"Run fzf --help once.", i18n.T("summary.new_terminal")} {
		if !strings.Contains(v, want) {
			t.Errorf("sem %q:\n%s", want, v)
		}
	}
}

func TestSummaryKeys(t *testing.T) {
	m, _ := summaryModel(t)
	m.sel = NewSelection(m.s.Cat, m.s.P, nil, ModeWizard)
	_, cmd := m.Update(keyRune("q"))
	if !isQuit(cmd) {
		t.Fatal("q deve sair direto")
	}
	m.runRep.Failed = nil
	m = send(m, enter())
	if m.screen != ScreenDashboard {
		t.Fatalf("enter sem falha: screen = %v", m.screen)
	}
}
