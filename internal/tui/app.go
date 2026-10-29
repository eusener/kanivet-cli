package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
)

// Screen identifica a tela atual da máquina de estados.
type Screen int

const (
	ScreenWelcome Screen = iota
	ScreenProfile
	ScreenCategories
	ScreenReview
	ScreenExecute
	ScreenSummary
	ScreenDashboard
)

const (
	minWidth  = 80
	minHeight = 24
)

// detectMsg entrega o resultado da detecção inicial.
type detectMsg struct{ statuses map[string]detect.ItemStatus }

// Model é o modelo raiz: escolhe a tela e delega teclas e desenho a ela.
type Model struct {
	s      *app.Session
	env    map[string]string
	th     Theme
	keys   KeyMap
	help   help.Model
	spin   spinner.Model
	screen Screen
	w, h   int

	detecting   bool
	statuses    map[string]detect.ItemStatus
	sel         *Selection
	profCursor  int
	page        int
	list        ItemList
	showHelp    bool
	confirmQuit bool
}

// New cria o modelo. start diz a tela inicial (o wizard começa em ScreenWelcome).
func New(s *app.Session, env map[string]string, start Screen) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Line
	return Model{
		s: s, env: env, th: NewTheme(env, false), keys: DefaultKeys(),
		help: help.New(), spin: sp, screen: start, w: minWidth, h: minHeight, detecting: true,
	}
}

// Init dispara a detecção e o spinner.
func (m Model) Init() tea.Cmd {
	s := m.s
	detectCmd := func() tea.Msg {
		return detectMsg{statuses: s.Statuses(context.Background(), plan.Selection{})}
	}
	return tea.Batch(m.spin.Tick, detectCmd)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.help.Width = msg.Width
		return m, nil
	case detectMsg:
		m.detecting = false
		m.statuses = msg.statuses
		m.th = NewTheme(m.env, HasNerdFont(msg.statuses))
		mode := ModeWizard
		if m.screen == ScreenDashboard {
			mode = ModeDashboard
		}
		m.sel = NewSelection(m.s.Cat, m.s.P, msg.statuses, mode)
		return m, nil
	case spinner.TickMsg:
		if !m.detecting {
			return m, nil
		}
		var c tea.Cmd
		m.spin, c = m.spin.Update(msg)
		return m, c
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m Model) updateKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	if km.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.confirmQuit {
		if km.String() == "q" {
			return m, tea.Quit
		}
		m.confirmQuit = false
		return m, nil
	}
	typing := m.screen == ScreenCategories && m.list.Filtering()
	if !typing {
		switch {
		case key.Matches(km, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(km, m.keys.Quit):
			if m.sel != nil {
				if n, u, r := m.sel.Pending(); n+u+r > 0 {
					m.confirmQuit = true
					return m, nil
				}
			}
			return m, tea.Quit
		}
	}
	m.showHelp = false
	switch m.screen {
	case ScreenWelcome:
		if key.Matches(km, m.keys.Enter) && !m.detecting {
			m.screen = ScreenProfile
		}
	case ScreenProfile:
		return m.updateProfile(km), nil
	case ScreenCategories:
		return m.updateCategories(km)
	case ScreenReview:
		if key.Matches(km, m.keys.Back) {
			m.screen = ScreenCategories
			m.page = len(categoryPages) - 1
			m.list = m.newList()
		}
	}
	return m, nil
}

// View desenha título, corpo, barra de pendências e atalhos (sempre visíveis).
func (m Model) View() string {
	if m.w < minWidth || m.h < minHeight {
		return i18n.T("tui.too_small", m.w, m.h, minWidth, minHeight)
	}
	body := m.bodyLines(m.h - 3)
	for len(body) < m.h-3 {
		body = append(body, "")
	}
	status := ""
	if m.sel != nil {
		n, u, _ := m.sel.Pending()
		status = m.th.Accent.Render(i18n.T("tui.pending", n, u))
	}
	foot := m.help.ShortHelpView(m.bindings())
	if m.confirmQuit {
		foot = m.th.Warn.Render(i18n.T("tui.quit_confirm"))
	}
	lines := append([]string{m.th.Title.Render(m.title())}, body[:m.h-3]...)
	lines = append(lines, status, lipgloss.NewStyle().MaxWidth(m.w).Render(foot))
	return strings.Join(lines, "\n")
}

func (m Model) title() string {
	switch m.screen {
	case ScreenWelcome:
		return i18n.T("tui.step", 1, i18n.T("tui.step.welcome"))
	case ScreenProfile:
		return i18n.T("tui.step", 2, i18n.T("tui.step.profile"))
	case ScreenCategories:
		return i18n.T("tui.step.page", pageTitle(m.page), m.page+1, len(categoryPages))
	}
	return "kanivet-cli"
}

func (m Model) bodyLines(height int) []string {
	if m.showHelp {
		all := []string{m.th.Title.Render(i18n.T("tui.help.title"))}
		for _, b := range m.bindings() {
			all = append(all, "  "+b.Help().Key+"  "+b.Help().Desc)
		}
		return all
	}
	switch m.screen {
	case ScreenWelcome:
		return m.welcomeLines()
	case ScreenProfile:
		return m.profileLines(height)
	case ScreenCategories:
		return strings.Split(m.list.View(m.w, height), "\n")
	case ScreenReview:
		return []string{i18n.T("tui.review.placeholder")}
	}
	return nil
}

// bindings devolve os atalhos da tela atual (o Enter some enquanto detecta).
func (m Model) bindings() []key.Binding {
	enter := m.keys.Enter
	enter.SetEnabled(!m.detecting)
	switch m.screen {
	case ScreenWelcome:
		return []key.Binding{enter, m.keys.Help, m.keys.Quit}
	case ScreenProfile:
		return []key.Binding{m.keys.Up, m.keys.Down, enter, m.keys.Help, m.keys.Quit}
	case ScreenCategories:
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Toggle, m.keys.Left, m.keys.Search, enter, m.keys.Back, m.keys.Help, m.keys.Quit}
	}
	return []key.Binding{m.keys.Back, m.keys.Quit}
}
