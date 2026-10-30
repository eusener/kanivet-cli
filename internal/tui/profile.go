package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
)

const (
	blankProfile = "blank"
	gridCols     = 2
	cardGap      = 2
)

// profileOrder é a ordem de exibição: a grade (laravel, web, ml, backend), os demais
// perfis e, por último, o "blank", que fica fora da grade.
var profileOrder = []string{"laravel", "web", "ml", "backend"}

// profiles devolve os perfis na ordem de exibição; o cursor indexa esta lista.
func (m Model) profiles() []catalog.Profile {
	var out, rest []catalog.Profile
	var blank *catalog.Profile
	byID := map[string]catalog.Profile{}
	for _, p := range m.s.Cat.Profiles {
		byID[p.ID] = p
	}
	seen := map[string]bool{}
	for _, id := range profileOrder {
		if p, ok := byID[id]; ok {
			out = append(out, p)
			seen[id] = true
		}
	}
	for i := range m.s.Cat.Profiles {
		p := m.s.Cat.Profiles[i]
		switch {
		case seen[p.ID]:
		case p.ID == blankProfile:
			blank = &p
		default:
			rest = append(rest, p)
		}
	}
	out = append(out, rest...)
	if blank != nil {
		out = append(out, *blank)
	}
	return out
}

// gridLen é quantos perfis ficam na grade (todos menos o blank, que é o último).
func (m Model) gridLen() int {
	ps := m.profiles()
	if n := len(ps); n > 0 && ps[n-1].ID == blankProfile {
		return n - 1
	}
	return len(ps)
}

// profileID devolve o perfil sob o cursor.
func (m Model) profileID() string {
	ps := m.profiles()
	if m.profCursor < 0 || m.profCursor >= len(ps) {
		return ""
	}
	return ps[m.profCursor].ID
}

// profileColor é a cor da tag de contagem do perfil; "" para os sem cor.
func profileColor(id string) lipgloss.Color {
	switch id {
	case "laravel":
		return colSun
	case "web":
		return colPeach
	case "ml":
		return colMint
	case "backend":
		return colCoral
	}
	return ""
}

// profileItemCount é quantos itens o perfil marca neste sistema.
func (m Model) profileItemCount(id string) int {
	return len(app.SelectedFor(m.s.Cat, m.s.P, plan.Selection{Profile: id}))
}

// moveProfile move o cursor na grade de 2 colunas; o blank fica numa linha abaixo.
func (m Model) moveProfile(dr, dc int) Model {
	n, total := m.gridLen(), len(m.profiles())
	c := m.profCursor
	onBlank := c >= n
	switch {
	case dc != 0 && !onBlank:
		if nc := c + dc; nc >= 0 && nc < n && nc/gridCols == c/gridCols {
			m.profCursor = nc
		}
	case dr > 0 && !onBlank:
		if c+gridCols < n {
			m.profCursor = c + gridCols
		} else if total > n {
			m.profCursor = n
		}
	case dr < 0 && onBlank && n > 0:
		m.profCursor = ((n - 1) / gridCols) * gridCols
	case dr < 0 && !onBlank && c-gridCols >= 0:
		m.profCursor = c - gridCols
	}
	return m
}

func (m Model) updateProfile(km tea.KeyMsg) Model {
	n := len(m.profiles())
	switch {
	case key.Matches(km, m.keys.Up):
		m = m.moveProfile(-1, 0)
	case key.Matches(km, m.keys.Down):
		m = m.moveProfile(1, 0)
	case key.Matches(km, m.keys.Left):
		m = m.moveProfile(0, -1)
	case key.Matches(km, m.keys.Right):
		m = m.moveProfile(0, 1)
	case key.Matches(km, m.keys.Back):
		m.screen = ScreenWelcome
	case key.Matches(km, m.keys.Enter) && !m.detecting && n > 0:
		// outro perfil substitui a seleção (não soma ao anterior); o mesmo mantém as mudanças
		if id := m.profileID(); m.sel.profile != id {
			if m.sel.profile != "" {
				m.sel = NewSelection(m.s.Cat, m.s.P, m.sel.st, ModeWizard)
			}
			m.sel.ApplyProfile(id)
		}
		m.screen, m.page = ScreenCategories, 0
		m.list = m.newList()
	}
	return m
}

// boxInner é a largura útil do texto numa caixa de largura total w.
func (m Model) boxInner(w int) int {
	if m.th.Paper {
		return max(w-5, 1)
	}
	return max(w-4, 1)
}

// clampLines quebra s em linhas de no máximo w colunas e fica com n; se cortar, a
// última termina em "…".
func clampLines(s string, w, n int) []string {
	ls := strings.Split(ansi.Wordwrap(s, w, ""), "\n")
	cut := len(ls) > n
	if cut {
		ls = ls[:n]
	}
	for i := range ls {
		if w1 := ansi.Truncate(ls[i], w, ""); w1 != ls[i] {
			ls[i] = ansi.Truncate(ls[i], w-1, "") + "…"
		}
	}
	if cut {
		ls[n-1] = ansi.Truncate(ls[n-1], w-1, "") + "…"
	}
	return ls
}

func (m Model) profileCard(pr catalog.Profile, selected bool, w, descLines int) string {
	inner := m.boxInner(w)
	name := m.th.R.NewStyle().Bold(true).Render(strings.ToUpper(ansi.Truncate(pr.Name.Get(), max(inner-4, 1), "…")))
	body := []string{m.th.Check(selected, true) + " " + name}
	desc := clampLines(pr.Description.Get(), inner, descLines)
	for len(desc) < descLines {
		desc = append(desc, "")
	}
	body = append(body, desc...)
	tagBg := profileColor(pr.ID)
	if selected || tagBg == "" {
		tagBg = colCard
	}
	body = append(body, m.th.Tag(i18n.T("tui.profile.items", m.profileItemCount(pr.ID)), tagBg))
	o := BoxOpts{}
	if selected {
		o.Fill, o.Fg = colSky, colInkStrong
	}
	return m.th.Box(strings.Join(body, "\n"), w, o)
}

func (m Model) profileLines(height int) []string {
	cw := m.contentWidth()
	cardW := (cw - cardGap) / gridCols
	descLines := 2
	if m.h < 30 {
		descLines = 1
	}
	ps := m.profiles()
	n := m.gridLen()
	var out []string
	for r := 0; r*gridCols < n; r++ {
		var cards []string
		for c := 0; c < gridCols && r*gridCols+c < n; c++ {
			i := r*gridCols + c
			if c > 0 {
				cards = append(cards, strings.Repeat(" ", cardGap))
			}
			cards = append(cards, m.profileCard(ps[i], i == m.profCursor, cardW, descLines))
		}
		out = append(out, strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, cards...), "\n")...)
		out = append(out, "")
	}
	if len(ps) > n {
		bl := ps[n]
		sel := m.profCursor == n
		txt := ansi.Truncate(m.th.Check(sel, true)+" "+strings.ToUpper(bl.Name.Get())+"  "+bl.Description.Get(), cw, "…")
		if sel {
			txt = m.th.Selected.Render(txt)
		} else {
			head := m.th.Check(sel, true) + " " + strings.ToUpper(bl.Name.Get())
			if lipgloss.Width(txt) > lipgloss.Width(head) {
				txt = m.th.S().Render(head) + m.th.Subtle.Render(txt[len(head):])
			}
		}
		out = append(out, txt)
	}
	return out
}
