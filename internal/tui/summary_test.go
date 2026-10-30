package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/catalog"
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
	rendered, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	cfg := m.s.St.Configs[target]
	if cfg.SHA256 != shaHex(rendered) {
		t.Fatalf("St.Configs[target].SHA256 = %q, want sha do conteúdo reaplicado", cfg.SHA256)
	}
	if b, err := os.ReadFile(cfg.Backup); err != nil || string(b) != "mine\n" {
		t.Fatalf("backup = %q, %v", b, err)
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

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Forçar A não pode reescrever B, outro arquivo da mesma config que o usuário também editou.
func TestSummaryForceOnlyTouchesConfirmedFile(t *testing.T) {
	m, _ := summaryModel(t)
	s := m.s
	tpl := "configs/fzf/fzfrc"
	s.Cat.Items["fzf"].Configs = []catalog.ConfigSpec{{ID: "two", Files: []catalog.ConfigFile{
		{Template: tpl, Target: "{{.XDG_CONFIG}}/fzf/a"},
		{Template: tpl, Target: "{{.XDG_CONFIG}}/fzf/b"},
	}}}
	a, b := s.ExpandTarget("{{.XDG_CONFIG}}/fzf/a"), s.ExpandTarget("{{.XDG_CONFIG}}/fzf/b")
	if err := os.MkdirAll(a[:strings.LastIndex(a, "/")], 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(a, []byte("orig A\n"), 0o644)
	first := &plan.Plan{Steps: []plan.Step{{ID: "config:two", ItemID: "fzf", Kind: plan.KindConfig, Files: []plan.FileChange{
		{Target: "{{.XDG_CONFIG}}/fzf/a", Template: tpl, ItemID: "fzf"},
		{Target: "{{.XDG_CONFIG}}/fzf/b", Template: tpl, ItemID: "fzf"},
	}}}}
	if rep, _, err := s.Run(context.Background(), first, nil, app.RunOptions{}); err != nil || len(rep.Failed) > 0 {
		t.Fatalf("primeira aplicação: %+v %v", rep, err)
	}
	prevB := s.St.Configs[b]
	os.WriteFile(a, []byte("edited A\n"), 0o644)
	os.WriteFile(b, []byte("edited B\n"), 0o644)
	m.runRep.Conflicts = []configs.Conflict{{Target: a, Diff: "x"}, {Target: b, Diff: "y"}}
	m.sum = summaryState{cursor: 1} // falha em 0, conflito A em 1

	m = send(m, keyRune("F"))
	nm, cmd := m.Update(keyRune("y"))
	m = nm.(Model)
	if cmd == nil {
		t.Fatal("y deve produzir um cmd")
	}
	msg := cmd()
	if got, _ := os.ReadFile(b); string(got) != "edited B\n" {
		t.Fatalf("B foi sobrescrito sem confirmação: %q", got)
	}
	if got, _ := os.ReadFile(a); string(got) == "edited A\n" {
		t.Fatal("A não foi reaplicado")
	}
	if s.St.Configs[b] != prevB {
		t.Fatalf("estado de B mudou: %+v -> %+v", prevB, s.St.Configs[b])
	}
	got, _ := os.ReadFile(a)
	if s.St.Configs[a].SHA256 != shaHex(got) {
		t.Fatal("estado de A não reflete o conteúdo reaplicado")
	}
	if bk, err := os.ReadFile(s.St.Configs[a].Backup); err != nil || string(bk) != "orig A\n" {
		t.Fatalf("backup de A = %q, %v", bk, err)
	}
	_ = msg
}

// I5: encolher a janela abaixo de 4 linhas, rolar e voltar não pode estourar o viewport.
func TestSummaryPanelSurvivesTinyResize(t *testing.T) {
	m, _ := summaryModel(t)
	m = m.openLog("install:node")
	m = send(m, size(80, 2))
	for i := 0; i < 40; i++ {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, size(100, 30))
	viewNoPanic(t, m)
	if m.sum.vp.Height < 1 || m.sum.vp.YOffset > m.sum.vp.TotalLineCount() {
		t.Fatalf("viewport fora dos limites: height=%d yoffset=%d total=%d", m.sum.vp.Height, m.sum.vp.YOffset, m.sum.vp.TotalLineCount())
	}
}

func viewNoPanic(t *testing.T, m Model) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View entrou em pânico: %v", r)
		}
	}()
	_ = m.View()
}
