package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

var keyRefresh = key.NewBinding(key.WithKeys("r"))

// updatesMsg entrega o resultado da verificação de atualizações (em background).
type updatesMsg struct {
	statuses map[string]detect.ItemStatus
	at       time.Time // quando a verificação foi feita (zero se desconhecido)
	gen      int       // descarta resultados de uma detecção anterior
}

type dashState struct {
	cat       int       // aba ativa (índice em Cat.Categories)
	list      ItemList  // itens da categoria da aba ativa
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
	case key.Matches(km, m.keys.NextTab), key.Matches(km, m.keys.PrevTab):
		// tab é das abas: não chega à lista (lá alternaria o painel de configs)
		if m.dash.global == nil {
			d := -1
			if key.Matches(km, m.keys.NextTab) {
				d = 1
			}
			m = m.switchTab(d)
		}
	case key.Matches(km, m.keys.Back) && m.dash.global != nil && !l.panel && l.filter.Value() == "":
		m.dash.global = nil
	default:
		nl, c := l.Update(km)
		return m.setDashList(nl, c)
	}
	return m, nil
}

// switchTab move a aba ativa em d (circular) e remonta a lista da categoria.
func (m Model) switchTab(d int) Model {
	if n := len(m.s.Cat.Categories); n > 0 {
		m.dash.cat = ((m.dash.cat+d)%n + n) % n
		m.dash.list = m.catList()
	}
	return m
}

// updateDashModal trata o seletor de perfil: aplicar é aditivo (nunca desmarca).
func (m Model) updateDashModal(km tea.KeyMsg) Model {
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
		m.dash.modal = false
	case key.Matches(km, m.keys.Enter) && n > 0:
		m.sel.ApplyProfile(m.profileID())
		m.dash.profile = m.profileID()
		m.dash.modal = false
	}
	return m
}

func (m Model) dashBindings() []key.Binding {
	if m.dash.modal {
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back, m.keys.Quit}
	}
	apply := m.keys.Apply
	apply.SetEnabled(!m.detecting)
	version := key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", i18n.T("tui.key.version")))
	cfg := key.NewBinding(key.WithKeys("c"), key.WithHelp("c", i18n.T("tui.key.configs")))
	return []key.Binding{m.keys.Toggle, m.keys.Update, m.keys.UpdateAll, m.keys.Profile, m.keys.NextTab, m.keys.PrevTab,
		m.keys.Search, version, cfg, apply, m.keys.Help, m.keys.Quit}
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

// dashTags são as tags do cabeçalho do painel, da mais à menos importante: perfil e
// totais (atualizações e externos só quando há).
func (m Model) dashTags() []string {
	ref := m.s.St.Profile
	if m.dash.profile != "" {
		ref = m.dash.profile
	}
	if ref == "" {
		ref = "—"
	}
	out := []string{m.th.Tag(i18n.T("tui.dash.profile", ref), colPeach)}
	if m.detecting || m.sel == nil {
		return out
	}
	var inst, upd, ext int
	for id := range m.s.Cat.Items {
		switch x := m.sel.status(id).Status; {
		case owned(x):
			inst++
			if x == detect.StatusOutdated {
				upd++
			}
		case external(x):
			ext++
		}
	}
	out = append(out, m.th.Tag(i18n.T("tui.dash.hd_installed", inst), colMint))
	if upd > 0 {
		out = append(out, m.th.Tag(i18n.T("tui.dash.hd_updates", upd), colSun))
	}
	if ext > 0 {
		out = append(out, m.th.Tag(i18n.T("tui.dash.hd_external", ext), colPeach))
	}
	return out
}

// dashStatus é o estado da verificação de atualizações (à direita do cabeçalho).
func (m Model) dashStatus() string {
	var h string
	switch {
	case m.s.UpdatesDisabled():
		h = i18n.T("tui.dash.check_off")
	case m.dash.checking:
		h = m.spin.View() + " " + i18n.T("tui.dash.checking")
	case !m.dash.checkedAt.IsZero():
		now := time.Now()
		if m.s.Now != nil {
			now = m.s.Now()
		}
		h = i18n.T("tui.dash.verified", age(now.Sub(m.dash.checkedAt)))
	}
	return m.th.Subtle.Render(h)
}

// tabsLine é a linha de abas (ou o aviso da busca global, que cobre todas).
func (m Model) tabsLine(w int) string {
	if m.dash.global != nil {
		return padTo(m.th.Subtle.Render(ansi.Truncate(i18n.T("tui.dash.global"), w, "…")), w)
	}
	names := make([]string, len(m.s.Cat.Categories))
	for i, c := range m.s.Cat.Categories {
		names[i] = c.Name.Get()
	}
	return m.th.Tabs(names, m.dash.cat, w)
}

func (m Model) dashLines(height int) []string {
	if m.detecting || m.sel == nil {
		return []string{m.spin.View() + " " + i18n.T("tui.dash.detecting")}
	}
	if m.dash.modal {
		out := []string{m.th.Title.Render(i18n.T("tui.dash.profile_title"))}
		return append(out, m.profileLines(height-1)...)
	}
	out := []string{m.tabsLine(m.contentWidth())}
	return append(out, strings.Split(m.dashList().View(m.contentWidth(), height-1), "\n")...)
}
