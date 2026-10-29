package tui

import (
	"sort"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

// categoryPages são as 5 páginas do wizard (managers entra só como dependência).
var categoryPages = [][]string{
	{"terminal", "shell"},
	{"replacements", "navigation", "git", "data-apis"},
	{"neovim", "editors"},
	{"languages", "php-local", "python-data"},
	{"container-engine", "containers"},
}

func pageTitle(i int) string {
	switch i {
	case 0:
		return i18n.T("tui.page.1")
	case 1:
		return i18n.T("tui.page.2")
	case 2:
		return i18n.T("tui.page.3")
	case 3:
		return i18n.T("tui.page.4")
	}
	return i18n.T("tui.page.5")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (m Model) newList() ItemList {
	return NewItemList(m.sel, m.th, m.keys, categoryPages[m.page])
}

func (m Model) updateCategories(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Com a busca aberta (ou algo a desfazer com esc), a lista fica com a tecla.
	if m.list.Filtering() || (key.Matches(km, m.keys.Back) && (m.list.panel || m.list.filter.Value() != "")) {
		var c tea.Cmd
		m.list, c = m.list.Update(km)
		return m, c
	}
	switch {
	case key.Matches(km, m.keys.Enter):
		if m.page == len(categoryPages)-1 {
			m.screen = ScreenReview
			return m, nil
		}
		m.page++
		m.list = m.newList()
	case key.Matches(km, m.keys.Back):
		if m.page == 0 {
			m.screen = ScreenProfile
			return m, nil
		}
		m.page--
		m.list = m.newList()
	default:
		var c tea.Cmd
		m.list, c = m.list.Update(km)
		return m, c
	}
	return m, nil
}
