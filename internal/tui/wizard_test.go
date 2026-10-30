package tui

import (
	"context"
	"fmt"
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

func newSession(t *testing.T, p platform.Platform) *app.Session {
	t.Helper()
	c, err := catalog.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	fake := &runner.FakeRunner{Func: func(ctx context.Context, cmd runner.Cmd) (runner.Result, bool) {
		return runner.Result{ExitCode: 1}, true
	}}
	s, err := app.Open(app.Session{R: fake, Cat: c, P: p, StateDir: t.TempDir(), Getenv: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func send(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		nm, _ := m.Update(msg)
		m = nm.(Model)
	}
	return m
}

func enter() tea.Msg { return tea.KeyMsg{Type: tea.KeyEnter} }
func esc() tea.Msg   { return tea.KeyMsg{Type: tea.KeyEsc} }
func size(w, h int) tea.Msg {
	return tea.WindowSizeMsg{Width: w, Height: h}
}

// detected roda o Init, recolhe o detectMsg e o entrega ao modelo.
func detected(t *testing.T, m Model) Model {
	t.Helper()
	var find func(tea.Msg) tea.Msg
	find = func(msg tea.Msg) tea.Msg {
		if d, ok := msg.(detectMsg); ok {
			return d
		}
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, c := range b {
				if c == nil {
					continue
				}
				if r := find(c()); r != nil {
					return r
				}
			}
		}
		return nil
	}
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init devolveu nil")
	}
	d := find(cmd())
	if d == nil {
		t.Fatal("Init não produziu detectMsg")
	}
	return send(m, size(100, 30), d)
}

func newModel(t *testing.T, p platform.Platform) Model {
	t.Helper()
	i18n.SetLang(i18n.En)
	t.Cleanup(func() { i18n.SetLang(i18n.En) })
	return New(newSession(t, p), nil, ScreenWelcome)
}

func TestWizardFlowToReview(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter())
	if m.screen != ScreenProfile {
		t.Fatalf("screen = %v, want profile", m.screen)
	}
	m = cursorOn(t, m, "web")
	m = send(m, enter())
	if m.screen != ScreenCategories {
		t.Fatalf("screen = %v, want categories", m.screen)
	}
	if !m.sel.Marked("node") {
		t.Error("perfil web deveria marcar node")
	}
	for i := 0; i < 4; i++ {
		m = send(m, enter())
		if m.screen != ScreenCategories {
			t.Fatalf("saiu das categorias na página %d", i+1)
		}
	}
	m = send(m, enter())
	if m.screen != ScreenReview {
		t.Fatalf("screen = %v, want review", m.screen)
	}
	if !strings.Contains(m.View(), "REVIEW THE PLAN") {
		t.Errorf("view da revisão:\n%s", m.View())
	}
}

func TestBackFromFirstCategoryGoesToProfile(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter(), enter())
	if m.screen != ScreenCategories {
		t.Fatalf("screen = %v", m.screen)
	}
	m = send(m, enter()) // página 2
	if m.page != 1 {
		t.Fatalf("page = %d", m.page)
	}
	m = send(m, esc())
	if m.screen != ScreenCategories || m.page != 0 {
		t.Fatalf("screen=%v page=%d", m.screen, m.page)
	}
	m = send(m, esc())
	if m.screen != ScreenProfile {
		t.Fatalf("screen = %v, want profile", m.screen)
	}
}

func TestWelcomeWarnsNoSystemd(t *testing.T) {
	p := linuxSystemd
	p.Systemd, p.Graphical = false, false
	m := detected(t, newModel(t, p))
	v := m.View()
	for _, w := range []string{"no systemd: lerd unavailable", "no graphical session: terminal will be left out"} {
		if !strings.Contains(v, w) {
			t.Errorf("faltou %q em:\n%s", w, v)
		}
	}
	// com systemd, sem aviso
	v = detected(t, newModel(t, linuxSystemd)).View()
	if strings.Contains(v, "lerd unavailable") {
		t.Errorf("aviso indevido:\n%s", v)
	}
}

func TestProfilePreviewGolden(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter())
	golden(t, "profile.golden", trimView(m.View()))
}

func TestWelcomeGolden(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	golden(t, "welcome.golden", trimView(m.View()))
}

func TestCategoriesGolden(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter(), enter())
	v := m.View()
	if !strings.Contains(v, "TERMINAL AND SHELL") || !strings.Contains(v, "CATEGORY 1 OF 5") {
		t.Errorf("título:\n%s", v)
	}
	golden(t, "categories.golden", trimView(v))
}

func TestEnterDisabledWhileDetecting(t *testing.T) {
	m := newModel(t, linuxSystemd)
	m = send(m, size(100, 30))
	if !strings.Contains(m.View(), "detecting") {
		t.Errorf("sem spinner:\n%s", m.View())
	}
	m = send(m, enter())
	if m.screen != ScreenWelcome {
		t.Fatalf("enter avançou durante a detecção: %v", m.screen)
	}
	m = detected(t, m)
	m = send(m, enter())
	if m.screen != ScreenProfile {
		t.Fatalf("enter não avançou após detecção: %v", m.screen)
	}
}

func TestTooSmall(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, size(70, 20))
	v := m.View()
	if !strings.Contains(v, "Terminal too small: 70x20 (minimum 80x24)") {
		t.Errorf("view:\n%s", v)
	}
	m = send(m, size(100, 30))
	if strings.Contains(m.View(), "too small") {
		t.Errorf("deveria voltar ao normal:\n%s", m.View())
	}
}

func TestFooterCountsAndQuitAgree(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter(), enter())
	n, _, _ := m.sel.Pending()
	c := m.sel.ConfigsOn()
	if n == 0 || c == 0 {
		t.Fatalf("esperava pendências, n=%d c=%d", n, c)
	}
	for _, want := range []string{fmt.Sprintf("%d to install", n), fmt.Sprintf("%d configs", c)} {
		if !strings.Contains(m.View(), strings.ToUpper(want)) {
			t.Errorf("faltou a tag %q em:\n%s", strings.ToUpper(want), m.View())
		}
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !m.confirmQuit || !strings.Contains(m.View(), "Quit with pending changes") {
		t.Error("q com pendências deveria pedir confirmação")
	}
	// sem pendências (sem seleção), a barra mostra zeros e q sai direto
	e := detected(t, newModel(t, linuxSystemd))
	e = send(e, enter())
	if a, b := e.pending(); a != 0 || b != 0 {
		t.Fatalf("pending = %d,%d", a, b)
	}
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("q sem pendências deveria sair")
	}
}

func TestKeyClosingHelpIsConsumed(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.showHelp {
		t.Fatal("ajuda deveria abrir")
	}
	m = send(m, enter())
	if m.showHelp || m.screen != ScreenWelcome {
		t.Fatalf("enter deveria só fechar a ajuda: help=%v screen=%v", m.showHelp, m.screen)
	}
	m = send(m, enter())
	if m.screen != ScreenProfile {
		t.Fatalf("screen = %v", m.screen)
	}
}

// pickProfile, na tela de perfil, leva o cursor até id e confirma.
func pickProfile(t *testing.T, m Model, id string) Model {
	t.Helper()
	if m.screen != ScreenProfile {
		t.Fatalf("screen = %v, want profile", m.screen)
	}
	return send(cursorOn(t, m, id), enter())
}

// cursorOn põe o cursor do perfil em id (a grade não tem ordem linear de setas).
func cursorOn(t *testing.T, m Model, id string) Model {
	t.Helper()
	for i, p := range m.profiles() {
		if p.ID == id {
			m.profCursor = i
			return m
		}
	}
	t.Fatalf("perfil %q não existe", id)
	return m
}

func TestWizardChangingProfileReplacesSelection(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter())
	m = pickProfile(t, m, "ml")
	if !m.sel.Marked("jupyter") {
		t.Fatal("ml deveria marcar jupyter")
	}
	m = send(m, enter(), esc(), esc()) // uma página adiante, volta, volta ao perfil
	m = pickProfile(t, m, "web")
	if m.sel.Marked("jupyter") || m.sel.Marked("uv") {
		t.Fatalf("trocar de perfil deve substituir a seleção: jupyter=%v uv=%v", m.sel.Marked("jupyter"), m.sel.Marked("uv"))
	}
	if !m.sel.Marked("node") {
		t.Fatal("web deveria marcar node")
	}
	// o mesmo perfil de novo mantém as mudanças do usuário
	m.sel.Toggle("node")
	m = send(m, esc())
	m = pickProfile(t, m, "web")
	if m.sel.Marked("node") {
		t.Fatal("reentrar no mesmo perfil não pode desfazer a escolha do usuário")
	}
}

func TestWelcomeHeadline(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	i18n.SetLang(i18n.PtBR)
	defer i18n.SetLang(i18n.En)
	v := m.View()
	if !strings.Contains(v, "SEU TERMINAL DE DEV") || !strings.Contains(v, "MONTADO EM MINUTOS") {
		t.Errorf("faltou o título grande:\n%s", v)
	}
	if strings.Contains(v, "SUDO EM") {
		t.Errorf("base sem sudo não leva a tag:\n%s", v)
	}
}

func TestWelcomeSudoTagFromCommonBase(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	if m.sudoCount() != 0 {
		t.Fatalf("base blank do catálogo não deveria pedir sudo no Linux, sudoCount = %d", m.sudoCount())
	}
	// dois itens da base passam a pedir sudo: um pelo item, outro por um método
	m.s.Cat.Items["bat"].NeedsSudo = true
	for i := range m.s.Cat.Items["fd"].Install {
		m.s.Cat.Items["fd"].Install[i].NeedsSudo = true
	}
	// item fora da base não conta
	m.s.Cat.Items["node"].NeedsSudo = true
	if got := m.sudoCount(); got != 2 {
		t.Fatalf("sudoCount = %d, want 2", got)
	}
	i18n.SetLang(i18n.PtBR)
	defer i18n.SetLang(i18n.En)
	if v := m.View(); !strings.Contains(v, "SUDO EM 2 ITENS") {
		t.Errorf("faltou a tag de sudo:\n%s", v)
	}
}

func TestProfileGridNavigation(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter())
	m = cursorOn(t, m, "laravel")
	right := tea.KeyMsg{Type: tea.KeyRight}
	left := tea.KeyMsg{Type: tea.KeyLeft}
	down := tea.KeyMsg{Type: tea.KeyDown}
	up := tea.KeyMsg{Type: tea.KeyUp}
	if m = send(m, right); m.profileID() != "web" {
		t.Fatalf("→ de laravel: %s", m.profileID())
	}
	if m = send(m, left); m.profileID() != "laravel" {
		t.Fatalf("← de web: %s", m.profileID())
	}
	if m = send(m, down); m.profileID() != "ml" {
		t.Fatalf("↓ de laravel: %s", m.profileID())
	}
	if m = send(m, down); m.profileID() != "blank" {
		t.Fatalf("↓↓ de laravel: %s", m.profileID())
	}
	if m = send(m, left, right); m.profileID() != "blank" {
		t.Fatalf("← → em blank saíram: %s", m.profileID())
	}
	if m = send(m, down); m.profileID() != "blank" {
		t.Fatalf("↓ em blank saiu: %s", m.profileID())
	}
	if m = send(m, up); m.profileID() == "blank" {
		t.Fatal("↑ em blank deveria voltar à grade")
	}
}

func TestProfileCardCounts(t *testing.T) {
	m := detected(t, newModel(t, linuxSystemd))
	m = send(m, enter())
	v := m.View()
	for _, pr := range m.profiles() {
		if pr.ID == "blank" {
			continue
		}
		n := len(app.SelectedFor(m.s.Cat, m.s.P, plan.Selection{Profile: pr.ID}))
		if want := fmt.Sprintf("%d ITEMS", n); !strings.Contains(v, want) {
			t.Errorf("perfil %s: faltou %q em:\n%s", pr.ID, want, v)
		}
		if got := m.profileItemCount(pr.ID); got != n {
			t.Errorf("profileItemCount(%s) = %d, want %d", pr.ID, got, n)
		}
	}
}

func TestProfileColor(t *testing.T) {
	for id, want := range map[string]lipgloss.Color{"laravel": colSun, "web": colPeach, "ml": colMint, "backend": colCoral, "blank": ""} {
		if got := profileColor(id); got != want {
			t.Errorf("profileColor(%s) = %q, want %q", id, got, want)
		}
	}
}

func TestWelcomeAndProfilePainted(t *testing.T) {
	for _, h := range []int{24, 30} {
		m := detected(t, newModel(t, linuxSystemd))
		m.th = paperTheme()
		m = send(m, size(100, h))
		assertPainted(t, m.View(), 100, h)
		m = send(m, enter())
		assertPainted(t, m.View(), 100, h)
		m = send(m, tea.KeyMsg{Type: tea.KeyDown}, tea.KeyMsg{Type: tea.KeyDown})
		assertPainted(t, m.View(), 100, h)
	}
	m := detected(t, newModel(t, linuxSystemd))
	m.th = paperTheme()
	m = send(m, size(80, 24))
	assertPainted(t, m.View(), 80, 24)
	m = send(m, enter())
	assertPainted(t, m.View(), 80, 24)
}

// T3: no cartão selecionado (sky) a tag de contagem mantém a cor do perfil.
func TestSelectedProfileCardKeepsColor(t *testing.T) {
	m := send(detected(t, newModel(t, linuxSystemd)), enter())
	m = withTheme(m, paperTheme())
	for _, pr := range m.profiles() {
		c := profileColor(pr.ID)
		if c == "" {
			continue
		}
		tag := m.th.Tag(i18n.N("tui.profile.items", m.profileItemCount(pr.ID)), c)
		for _, sel := range []bool{false, true} {
			card := m.profileCard(pr, sel, 38, 1)
			if !strings.Contains(card, tag) {
				t.Errorf("%s (selecionado=%v): tag sem a cor do perfil %s", pr.ID, sel, c)
			}
		}
	}
}
