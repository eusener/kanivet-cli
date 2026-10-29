package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/platform"
	"github.com/eusener/kanivet-cli/internal/runner"
)

// run executa o cmd (e os cmds de um Batch) e entrega o planMsg ao modelo.
func deliverPlan(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	var visit func(tea.Cmd)
	visit = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch r := c().(type) {
		case planMsg:
			m = send(m, r)
		case tea.BatchMsg:
			for _, cc := range r {
				visit(cc)
			}
		}
	}
	visit(cmd)
	return m
}

// reviewFor leva o wizard (perfil blank) até a revisão com o plano carregado.
func reviewFor(t *testing.T, p platform.Platform) Model {
	t.Helper()
	m := detected(t, newModel(t, p))
	m = send(m, size(80, 24), enter())
	for m.profileID() != "blank" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, enter())
	for i := 0; i < 4; i++ {
		m = send(m, enter())
	}
	nm, cmd := m.Update(enter())
	m = nm.(Model)
	if m.screen != ScreenReview {
		t.Fatalf("screen = %v, want review", m.screen)
	}
	return deliverPlan(t, m, cmd)
}

func TestReviewGolden(t *testing.T) {
	m := reviewFor(t, linuxSystemd)
	golden(t, "review.golden", m.View())
}

func TestReviewShowsSudoBanner(t *testing.T) {
	i18n.SetLang(i18n.En)
	s := newSession(t, linuxSystemd)
	m := reviewWith(t, s, plan.Selection{Add: []string{"zsh"}})
	if !m.review.hasSudo() {
		t.Fatal("o plano de zsh deveria ter sudo")
	}
	if !strings.Contains(m.View(), i18n.T("tui.sudo_banner")) {
		t.Errorf("faixa de sudo ausente:\n%s", m.View())
	}
	m = reviewWith(t, s, plan.Selection{})
	if strings.Contains(m.View(), i18n.T("tui.sudo_banner")) {
		t.Error("faixa de sudo sem passos com sudo")
	}
}

func testSession(t *testing.T) *app.Session {
	t.Helper()
	c, err := catalog.Load(os.DirFS("../catalog/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	fake := &runner.FakeRunner{Func: func(ctx context.Context, cmd runner.Cmd) (runner.Result, bool) {
		return runner.Result{ExitCode: 1}, true
	}}
	p := linuxSystemd
	p.Home = t.TempDir()
	s, err := app.Open(app.Session{R: fake, Cat: c, P: p, StateDir: t.TempDir(), Getenv: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func reviewWith(t *testing.T, s *app.Session, sel plan.Selection) Model {
	t.Helper()
	i18n.SetLang(i18n.En)
	m := New(s, nil, ScreenReview)
	m = send(m, size(100, 30))
	pl, _, err := s.Plan(context.Background(), sel)
	m.review = reviewState{caller: ScreenCategories, loading: true}
	return send(m, planMsg{pl: pl, groups: planGroups(s, pl, err), err: err})
}

func planGroups(s *app.Session, pl *plan.Plan, err error) []app.Group {
	if err != nil {
		return nil
	}
	return s.GroupPlan(pl)
}

func TestReviewDiffOpensAndCloses(t *testing.T) {
	s := testSession(t)
	cfg := filepath.Join(s.P.Home, ".config", "fzf", "fzfrc")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("--height 10%\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := reviewWith(t, s, plan.Selection{Add: []string{"fzf"}})
	m = send(m, size(200, 30))
	down := tea.KeyMsg{Type: tea.KeyDown}
	for i := 0; i < 50 && !strings.Contains(m.review.rowKey(m.review.cursor), "fzf-defaults"); i++ {
		m = send(m, down)
	}
	if !strings.Contains(m.View(), "(backup)") {
		t.Errorf("arquivo existente sem (backup):\n%s", m.View())
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	v := m.View()
	if !strings.Contains(v, "+--height 40%") || !strings.Contains(v, "---height 10%") {
		t.Fatalf("diff ausente:\n%s", v)
	}
	m = send(m, esc())
	if strings.Contains(m.View(), "+--height 40%") || m.screen != ScreenReview {
		t.Fatalf("esc não fechou só o painel (screen=%v):\n%s", m.screen, m.View())
	}
}

func TestReviewEmptyPlan(t *testing.T) {
	s := testSession(t)
	m := reviewWith(t, s, plan.Selection{})
	if !strings.Contains(m.View(), i18n.T("plan.empty")) {
		t.Fatalf("sem plan.empty:\n%s", m.View())
	}
	m = send(m, enter())
	if m.screen == ScreenExecute {
		t.Fatal("plano vazio não pode executar")
	}
}

func TestReviewPlanErrorBlocksEnter(t *testing.T) {
	s := testSession(t)
	m := reviewWith(t, s, plan.Selection{})
	m = send(m, planMsg{err: &plan.PlanError{Msgs: []string{"lerd conflita com flock"}}})
	if !strings.Contains(m.View(), "lerd conflita com flock") {
		t.Fatalf("erro não exibido:\n%s", m.View())
	}
	m = send(m, enter())
	if m.screen != ScreenReview {
		t.Fatalf("enter com erro saiu da revisão: %v", m.screen)
	}
}

func TestReviewEnterStartsExecution(t *testing.T) {
	s := testSession(t)
	m := reviewWith(t, s, plan.Selection{Add: []string{"fzf"}})
	m = send(m, enter())
	if m.screen != ScreenExecute || m.plan == nil {
		t.Fatalf("screen=%v plan=%v", m.screen, m.plan)
	}
}

func TestReviewEscReturnsToCaller(t *testing.T) {
	m := reviewFor(t, linuxSystemd)
	m = send(m, esc())
	if m.screen != ScreenCategories || m.page != len(categoryPages)-1 {
		t.Fatalf("wizard: screen=%v page=%d", m.screen, m.page)
	}
	d := reviewFor(t, linuxSystemd)
	d.review.caller = ScreenDashboard
	d = send(d, esc())
	if d.screen != ScreenDashboard {
		t.Fatalf("dashboard: screen=%v", d.screen)
	}
}

func TestReviewScrollsWithPageKeys(t *testing.T) {
	m := reviewFor(t, linuxSystemd)
	before := m.View()
	m = send(m, tea.KeyMsg{Type: tea.KeyPgDown}, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.review.cursor == 0 || m.View() == before {
		t.Fatalf("pgdown não rolou (cursor=%d)", m.review.cursor)
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyPgUp}, tea.KeyMsg{Type: tea.KeyPgUp})
	if m.review.cursor != 0 {
		t.Fatalf("pgup não voltou (cursor=%d)", m.review.cursor)
	}
}
