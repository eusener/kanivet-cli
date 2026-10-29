package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

func (m Model) profiles() []catalog.Profile { return m.s.Cat.Profiles }

// profileID devolve o perfil sob o cursor.
func (m Model) profileID() string {
	ps := m.profiles()
	if m.profCursor < 0 || m.profCursor >= len(ps) {
		return ""
	}
	return ps[m.profCursor].ID
}

func (m Model) updateProfile(km tea.KeyMsg) Model {
	n := len(m.profiles())
	switch {
	case key.Matches(km, m.keys.Up):
		if m.profCursor > 0 {
			m.profCursor--
		}
	case key.Matches(km, m.keys.Down):
		if m.profCursor < n-1 {
			m.profCursor++
		}
	case key.Matches(km, m.keys.Back):
		m.screen = ScreenWelcome
	case key.Matches(km, m.keys.Enter) && !m.detecting && n > 0:
		m.sel.ApplyProfile(m.profileID())
		m.screen, m.page = ScreenCategories, 0
		m.list = m.newList()
	}
	return m
}

func (m Model) profileLines(height int) []string {
	var out []string
	for i, pr := range m.profiles() {
		prefix := "  "
		if i == m.profCursor {
			prefix = "> "
		}
		line := prefix + pr.Name.Get() + " — " + pr.Description.Get()
		line = lipgloss.NewStyle().MaxWidth(m.w).Render(line)
		if i == m.profCursor {
			line = m.th.Selected.Render(line)
		}
		out = append(out, line)
	}
	out = append(out, m.th.Border.Render(strings.Repeat("─", m.w)), m.th.Subtle.Render(i18n.T("tui.profile.preview")+":"))
	room := max(height-len(out), 1)
	prev := strings.Split(lipgloss.NewStyle().Width(m.w).Render(strings.Join(m.previewNames(), ", ")), "\n")
	if len(prev) > room {
		prev = prev[:room]
	}
	return append(out, prev...)
}

// previewNames lista os nomes dos itens do perfil sob o cursor (com as exclusivas deste sistema).
func (m Model) previewNames() []string {
	ps := m.profiles()
	if m.profCursor >= len(ps) {
		return nil
	}
	pr := ps[m.profCursor]
	ids := append([]string{}, pr.Items...)
	excl := map[string]string{}
	for cat, id := range pr.Exclusive {
		excl[cat] = id
	}
	for cat, id := range pr.ExclusiveByOS[m.s.P.OS] {
		excl[cat] = id
	}
	for _, cat := range sortedKeys(excl) {
		ids = append(ids, excl[cat])
	}
	var names []string
	for _, id := range ids {
		if it := m.s.Cat.Items[id]; it != nil {
			names = append(names, it.Name.Get())
		}
	}
	return names
}
