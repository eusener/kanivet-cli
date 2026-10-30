package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
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
	// perfis: backend, blank, laravel, ml, web (ordem do catálogo); acha o web.
	for m.profileID() != "web" {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
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
	if !strings.Contains(m.View(), "Review") {
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
	if !strings.Contains(v, "Step 3/5 · Terminal and shell (1/5)") {
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
	want := fmt.Sprintf("%d to install · %d configs", n, c)
	if !strings.Contains(m.View(), want) {
		t.Errorf("faltou %q em:\n%s", want, m.View())
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
	for m.profCursor > 0 {
		m = send(m, tea.KeyMsg{Type: tea.KeyUp})
	}
	for m.profileID() != id {
		m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	return send(m, enter())
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
