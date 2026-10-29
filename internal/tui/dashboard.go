package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

const catPaneW = 26

var keyRefresh = key.NewBinding(key.WithKeys("r"))

// updatesMsg entrega o resultado da verificação de atualizações (em background).
type updatesMsg struct {
	statuses map[string]detect.ItemStatus
	at       time.Time // quando a verificação foi feita (zero se desconhecido)
	gen      int       // descarta resultados de uma detecção anterior
}

type dashState struct {
	cat       int       // categoria selecionada (índice em Cat.Categories)
	focusList bool      // o foco está na lista de itens (não nas categorias)
	list      ItemList  // itens da categoria selecionada
	global    *ItemList // busca global (todas as categorias), quando aberta
	modal     bool      // seletor de perfil aberto
	profile   string    // perfil aplicado nesta sessão (vai em RunOptions.Profile)
	checking  bool
	checkedAt time.Time
	gen       int
}

// enterDashboard entra no dashboard com uma seleção nova: re-detecta os status (spinner enquanto isso).
func (m Model) enterDashboard() (Model, tea.Cmd) {
	m.screen, m.detecting, m.sel = ScreenDashboard, true, nil
	m.dash.gen++
	m.dash.global, m.dash.modal, m.dash.checking = nil, false, false
	return m, tea.Batch(m.spin.Tick, m.detectCmd())
}

// initDashboard monta a lista da categoria e dispara a verificação de atualizações.
func (m Model) initDashboard() (Model, tea.Cmd) {
	m.dash.list = m.catList()
	if m.dash.cat >= len(m.s.Cat.Categories) {
		m.dash.cat = 0
	}
	if m.s.UpdatesDisabled() {
		return m, nil
	}
	m.dash.checkedAt, _ = m.s.UpdatesCheckedAt()
	m.dash.checking = true
	s, gen := m.s, m.dash.gen
	cp := make(map[string]detect.ItemStatus, len(m.statuses))
	for id, x := range m.statuses {
		cp[id] = x
	}
	return m, func() tea.Msg {
		s.CheckUpdates(context.Background(), cp, app.UpdateCheckTTL)
		at, _ := s.UpdatesCheckedAt()
		return updatesMsg{statuses: cp, at: at, gen: gen}
	}
}

func (m Model) onUpdates(msg updatesMsg) Model {
	if msg.gen != m.dash.gen || m.sel == nil {
		return m
	}
	m.dash.checking = false
	if !msg.at.IsZero() {
		m.dash.checkedAt = msg.at
	}
	m.statuses = msg.statuses
	m.sel.SetStatuses(msg.statuses)
	return m
}

func (m Model) catList() ItemList {
	cats := m.s.Cat.Categories
	if len(cats) == 0 {
		return NewItemList(m.sel, m.th, m.keys, nil)
	}
	return NewItemList(m.sel, m.th, m.keys, []string{cats[min(m.dash.cat, len(cats)-1)].ID})
}

// dashList devolve a lista ativa: a busca global, se aberta, ou a da categoria.
func (m Model) dashList() ItemList {
	if m.dash.global != nil {
		return *m.dash.global
	}
	return m.dash.list
}

func (m Model) setDashList(l ItemList, c tea.Cmd) (Model, tea.Cmd) {
	if m.dash.global != nil {
		m.dash.global = &l
	} else {
		m.dash.list = l
	}
	return m, c
}

func (m Model) updateDashboard(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detecting || m.sel == nil {
		return m, nil
	}
	if m.dash.modal {
		return m.updateDashModal(km), nil
	}
	l := m.dashList()
	if l.Filtering() {
		nl, c := l.Update(km)
		return m.setDashList(nl, c)
	}
	focusList := m.dash.focusList || m.dash.global != nil
	switch {
	case key.Matches(km, m.keys.Apply):
		nm, c := m.beginReview(ScreenDashboard)
		nm.review.profile = m.dash.profile
		return nm, c
	case key.Matches(km, m.keys.UpdateAll):
		m.sel.ToggleUpdateAll()
	case key.Matches(km, m.keys.Profile):
		m.dash.modal = true
	case key.Matches(km, keyRefresh):
		return m.enterDashboard()
	case key.Matches(km, m.keys.Search):
		if m.dash.global == nil {
			var all []string
			for _, c := range m.s.Cat.Categories {
				all = append(all, c.ID)
			}
			g := NewItemList(m.sel, m.th, m.keys, all)
			m.dash.global = &g
		}
		nl, c := m.dashList().Update(km)
		return m.setDashList(nl, c)
	case key.Matches(km, m.keys.Update):
		if x := m.sel.status(l.Current()); x.Status == detect.StatusOutdated {
			m.sel.MarkUpdate(l.Current())
		} else if l.Current() != "" {
			l.note = i18n.T("tui.dash.not_outdated")
			return m.setDashList(l, nil)
		}
	case !focusList:
		return m.updateCatPane(km), nil
	case key.Matches(km, m.keys.Back) && !l.panel && l.filter.Value() == "":
		if m.dash.global != nil {
			m.dash.global = nil
		} else {
			m.dash.focusList = false
		}
	case key.Matches(km, keyFocus) && m.dash.global == nil:
		m.dash.focusList = false
	default:
		nl, c := l.Update(km)
		return m.setDashList(nl, c)
	}
	return m, nil
}

// updateCatPane trata as teclas com o foco na lista de categorias.
func (m Model) updateCatPane(km tea.KeyMsg) Model {
	n := len(m.s.Cat.Categories)
	switch {
	case key.Matches(km, m.keys.Up) && m.dash.cat > 0:
		m.dash.cat--
		m.dash.list = m.catList()
	case key.Matches(km, m.keys.Down) && m.dash.cat < n-1:
		m.dash.cat++
		m.dash.list = m.catList()
	case key.Matches(km, m.keys.Left), key.Matches(km, m.keys.Right), key.Matches(km, keyFocus), key.Matches(km, m.keys.Enter):
		m.dash.focusList = true
	}
	return m
}

// updateDashModal trata o seletor de perfil: aplicar é aditivo (nunca desmarca).
func (m Model) updateDashModal(km tea.KeyMsg) Model {
	n := len(m.profiles())
	switch {
	case key.Matches(km, m.keys.Up) && m.profCursor > 0:
		m.profCursor--
	case key.Matches(km, m.keys.Down) && m.profCursor < n-1:
		m.profCursor++
	case key.Matches(km, m.keys.Back):
		m.dash.modal = false
	case key.Matches(km, m.keys.Enter) && n > 0:
		m.sel.ApplyProfile(m.profileID())
		m.dash.profile = m.profileID()
		m.dash.modal = false
	}
	return m
}

func (m Model) dashStatus() string {
	if m.sel == nil {
		return ""
	}
	i, u, r := m.sel.Pending()
	return m.th.Accent.Render(i18n.T("tui.dash.pending", i, u, r))
}

func (m Model) dashBindings() []key.Binding {
	if m.dash.modal {
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back, m.keys.Quit}
	}
	apply := m.keys.Apply
	apply.SetEnabled(!m.detecting)
	return []key.Binding{m.keys.Toggle, m.keys.Update, m.keys.UpdateAll, m.keys.Profile, m.keys.Search, apply, m.keys.Help, m.keys.Quit}
}

// age formata a idade da verificação (5m, 2h, 3d).
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (m Model) dashHeader() string {
	ref := m.s.St.Profile
	if m.dash.profile != "" {
		ref = m.dash.profile
	}
	if ref == "" {
		ref = "—"
	}
	h := i18n.T("tui.dash.profile", ref)
	switch {
	case m.s.UpdatesDisabled():
		h += "  ·  " + i18n.T("tui.dash.check_off")
	case m.dash.checking:
		h += "  ·  " + m.spin.View() + " " + i18n.T("tui.dash.checking")
	case !m.dash.checkedAt.IsZero():
		now := time.Now()
		if m.s.Now != nil {
			now = m.s.Now()
		}
		h += "  ·  " + i18n.T("tui.dash.verified", age(now.Sub(m.dash.checkedAt)))
	}
	return m.th.Subtle.Render(h)
}

// catLines desenha as categorias com o contador instalados/total.
func (m Model) catLines(rows int) []string {
	var out []string
	for i, c := range m.s.Cat.Categories {
		have, total := 0, 0
		for id, it := range m.s.Cat.Items {
			if it.Category != c.ID {
				continue
			}
			total++
			if s := m.sel.status(id).Status; owned(s) || external(s) {
				have++
			}
		}
		prefix, cnt := "  ", fmt.Sprintf("%d/%d", have, total)
		if i == m.dash.cat {
			prefix = "> "
		}
		name := lipgloss.NewStyle().MaxWidth(catPaneW - len(prefix) - len(cnt) - 1).Render(c.Name.Get())
		line := prefix + name
		line += strings.Repeat(" ", max(1, catPaneW-lipgloss.Width(line)-len(cnt))) + cnt
		switch {
		case i == m.dash.cat && !m.dash.focusList && m.dash.global == nil:
			line = m.th.Selected.Render(line)
		case i == m.dash.cat:
			line = m.th.Accent.Render(line)
		}
		out = append(out, line)
	}
	return pad(out, rows)[:rows]
}

func (m Model) dashLines(height int) []string {
	if m.detecting || m.sel == nil {
		return []string{m.spin.View() + " " + i18n.T("tui.dash.detecting")}
	}
	if m.dash.modal {
		out := []string{m.th.Title.Render(i18n.T("tui.dash.profile_title"))}
		return append(out, m.profileLines(height-1)...)
	}
	rows := height - 1
	rightW := m.w - catPaneW - 3
	left := m.catLines(rows)
	right := strings.Split(m.dashList().View(rightW, rows), "\n")
	sep := m.th.Border.Render(" │ ")
	out := []string{m.dashHeader()}
	for i := 0; i < rows; i++ {
		l := left[i]
		l += strings.Repeat(" ", max(0, catPaneW-lipgloss.Width(l)))
		out = append(out, l+sep+right[i])
	}
	return out
}
