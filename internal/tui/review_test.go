package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	m.detecting = false // fora da retomada, a revisão só abre depois da detecção
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

func longHomeSession(t *testing.T) *app.Session {
	t.Helper()
	s := testSession(t)
	s.P.Home = filepath.Join(t.TempDir(), strings.Repeat("longdir", 8))
	return s
}

func TestReviewMarkersSurviveLongPaths80Cols(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.En, i18n.PtBR} {
		s := longHomeSession(t)
		for _, f := range []string{".config/fzf/fzfrc", ".zshrc"} {
			p := filepath.Join(s.P.Home, f)
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.WriteFile(p, []byte("x\n"), 0o644)
		}
		m := reviewWith(t, s, plan.Selection{Add: []string{"fzf"}})
		i18n.SetLang(lang)
		m.th = NewTheme(map[string]string{"NO_COLOR": "1"}, false)
		m.review.rows = buildRows(m.review.groups)
		m = send(m, size(80, 24))
		m.review.rows = buildRows(m.review.groups)
		for i := 0; i < len(m.review.rows); i++ {
			m = m.layoutReview()
			m = send(m, tea.KeyMsg{Type: tea.KeyDown})
			for _, line := range strings.Split(m.View(), "\n") {
				if strings.Contains(line, "→") && strings.Contains(line, "…") && !strings.Contains(line, "("+i18n.T("plan.backup")+")") {
					t.Fatalf("%v: (backup) cortado: %q", lang, line)
				}
			}
		}
		out := m.renderRows(m.review)
		if n := strings.Count(out, "("+i18n.T("plan.backup")+")"); n != 2 {
			t.Errorf("%v: (backup) x%d, want 2:\n%s", lang, n, out)
		}
		if !strings.Contains(out, "("+i18n.T("plan.rc_block")+")") {
			t.Errorf("%v: marca do rc cortada", lang)
		}
		i18n.SetLang(i18n.En)
	}
}

func TestReviewRemoveMarkerSurvivesLongPath(t *testing.T) {
	m := reviewWith(t, testSession(t), plan.Selection{})
	long := "/" + strings.Repeat("abcdefghij/", 12)
	m.review.rows = buildRows([]app.Group{{Kind: app.GroupUninstall, Title: "x", Lines: []app.Line{{StepID: "u", Text: "u", Files: []app.FileLine{{Path: long, Remove: true, Exists: true}}}}}})
	m.w = 80
	for _, l := range strings.Split(m.renderRows(m.review), "\n") {
		if strings.Contains(l, "/abcdefghij") && (!strings.Contains(l, "✗") || lipgloss.Width(l) > 80) {
			t.Errorf("linha de remoção: %q", l)
		}
	}
}

func TestReviewDiffNoteOnRcAndGenericRows(t *testing.T) {
	s := testSession(t)
	m := reviewWith(t, s, plan.Selection{Add: []string{"fzf"}})
	d := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	for _, c := range []struct {
		match func(reviewRow) bool
		want  string
	}{
		{func(r reviewRow) bool { return r.rc }, i18n.T("tui.review.no_diff_rc")},
		{func(r reviewRow) bool { return r.style == rowTitle }, i18n.T("tui.review.no_diff")},
	} {
		for i, r := range m.review.rows {
			if c.match(r) {
				m.review.cursor = i
				break
			}
		}
		m = send(m.layoutReview(), d)
		if m.review.diffOpen || !strings.Contains(m.View(), c.want[:30]) {
			t.Fatalf("sem nota %q:\n%s", c.want, m.View())
		}
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
		if m.review.note != "" {
			t.Error("nota deveria sumir na próxima tecla")
		}
	}
}

func TestReviewDiffReadErrorIsNotFakeDiff(t *testing.T) {
	s := testSession(t)
	cfg := filepath.Join(s.P.Home, ".config", "fzf", "fzfrc")
	os.MkdirAll(cfg, 0o755) // diretório: ReadFile falha com erro que não é ErrNotExist
	m := reviewWith(t, s, plan.Selection{Add: []string{"fzf"}})
	for i, r := range m.review.rows {
		if r.stepID == "config:fzf-defaults" && r.file {
			m.review.cursor = i
		}
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.review.diffOpen || !strings.Contains(m.View(), "Cannot read") {
		t.Fatalf("esperava nota de erro:\n%s", m.View())
	}
}

// I5: o diff da revisão também sobrevive a um resize minúsculo seguido de rolagem.
func TestReviewDiffSurvivesTinyResize(t *testing.T) {
	s := testSession(t)
	m := reviewWith(t, s, plan.Selection{Add: []string{"fzf"}})
	for i, r := range m.review.rows {
		if r.stepID == "config:fzf-defaults" && r.file {
			m.review.cursor = i
		}
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.review.diffOpen {
		t.Fatal("diff deveria abrir")
	}
	m = send(m, size(80, 2))
	for i := 0; i < 40; i++ {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = send(m, size(100, 30))
	viewNoPanic(t, m)
	d := m.review.diff
	if d.Height < 1 || d.YOffset > d.TotalLineCount() {
		t.Fatalf("diff fora dos limites: height=%d yoffset=%d total=%d", d.Height, d.YOffset, d.TotalLineCount())
	}
}

// M2: enter, esc, mudar a seleção, enter — o plano da primeira ida chegando depois não pode
// sobrescrever a revisão da segunda.
func TestStalePlanMsgIsDropped(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter())
	m = pickProfile(t, m, "blank")
	m, first := m.beginReview(ScreenCategories)
	m = send(m, esc())
	m.sel.Toggle("bun")
	m, second := m.beginReview(ScreenCategories)
	m = deliverPlan(t, m, second)
	if _, ok := m.review.pl.Step("install:bun"); !ok {
		t.Fatal("o plano novo deveria instalar bun")
	}
	m = deliverPlan(t, m, first)
	if _, ok := m.review.pl.Step("install:bun"); !ok || m.review.loading {
		t.Fatal("um planMsg velho sobrescreveu a revisão")
	}
}
