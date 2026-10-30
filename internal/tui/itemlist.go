package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
)

// Atalhos próprios da lista (os genéricos vêm do KeyMap).
var (
	keyTop    = key.NewBinding(key.WithKeys("g", "home"))
	keyBottom = key.NewBinding(key.WithKeys("G", "end"))
	keyPgUp   = key.NewBinding(key.WithKeys("pgup"))
	keyPgDown = key.NewBinding(key.WithKeys("pgdown"))
	keyFocus  = key.NewBinding(key.WithKeys("tab"))
	keyConfig = key.NewBinding(key.WithKeys("c"))
)

// viewState guarda o que só a renderização sabe (a altura da janela).
// É um ponteiro para que View, com receptor por valor, possa lembrar a rolagem.
type viewState struct{ offset int }

// ItemList mostra os itens das categorias (marcação + detalhe + busca).
type ItemList struct {
	sel    *Selection
	th     Theme
	keys   KeyMap
	ids    []string
	cursor int
	cfgIdx int
	panel  bool // o foco está no painel de detalhe
	filter textinput.Model
	typing bool
	note   string
	vs     *viewState
}

// NewItemList lista os itens das categorias, na ordem dada (itens por id dentro de cada uma).
func NewItemList(sel *Selection, th Theme, keys KeyMap, categories []string) ItemList {
	var ids []string
	for _, cat := range categories {
		var in []string
		for id, it := range sel.c.Items {
			if it.Category == cat {
				in = append(in, id)
			}
		}
		sort.Strings(in)
		ids = append(ids, in...)
	}
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Width = 30
	return ItemList{sel: sel, th: th, keys: keys, ids: ids, filter: ti, vs: &viewState{}}
}

func (l ItemList) Filtering() bool { return l.typing }

// Current devolve o id sob o cursor ("" quando a lista está vazia).
func (l ItemList) Current() string {
	v := l.visible()
	if l.cursor < 0 || l.cursor >= len(v) {
		return ""
	}
	return v[l.cursor]
}

// visible aplica a busca (id, nome e descrição, sem diferenciar maiúsculas).
func (l ItemList) visible() []string {
	q := strings.ToLower(strings.TrimSpace(l.filter.Value()))
	if q == "" {
		return l.ids
	}
	var out []string
	for _, id := range l.ids {
		it := l.sel.c.Items[id]
		hay := strings.ToLower(id + " " + it.Name.Get() + " " + it.Description.Get())
		if strings.Contains(hay, q) {
			out = append(out, id)
		}
	}
	return out
}

func (l ItemList) item() *catalog.Item { return l.sel.c.Items[l.Current()] }

func (l ItemList) Update(msg tea.Msg) (ItemList, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		if l.typing {
			var c tea.Cmd
			l.filter, c = l.filter.Update(msg)
			return l, c
		}
		return l, nil
	}
	l.note = ""
	if l.typing {
		return l.updateTyping(km)
	}
	n := len(l.visible())
	switch {
	case key.Matches(km, l.keys.Search):
		l.typing = true
		return l, l.filter.Focus()
	case key.Matches(km, l.keys.Back):
		switch {
		case l.panel:
			l.panel = false
		case l.filter.Value() != "":
			l.filter.SetValue("")
			l.cursor = 0
		}
	case key.Matches(km, keyFocus):
		l.panel = !l.panel
	case key.Matches(km, keyConfig):
		// a primeira tecla só foca o painel; as seguintes avançam entre as configs
		if c := len(l.configs()); c > 0 {
			if l.panel {
				l.cfgIdx = (l.cfgIdx + 1) % c
			}
			l.panel = true
		}
	case key.Matches(km, l.keys.Up):
		l = l.move(-1, n)
	case key.Matches(km, l.keys.Down):
		l = l.move(1, n)
	case key.Matches(km, keyPgUp):
		l = l.moveItems(-10, n)
	case key.Matches(km, keyPgDown):
		l = l.moveItems(10, n)
	case key.Matches(km, keyTop):
		l = l.moveItems(-n, n)
	case key.Matches(km, keyBottom):
		l = l.moveItems(n, n)
	case key.Matches(km, l.keys.Left):
		l.sel.CycleVersion(l.Current(), -1)
	case key.Matches(km, l.keys.Right):
		l.sel.CycleVersion(l.Current(), 1)
	case key.Matches(km, l.keys.Toggle):
		l = l.toggle()
	}
	return l, nil
}

func (l ItemList) updateTyping(km tea.KeyMsg) (ItemList, tea.Cmd) {
	switch {
	case key.Matches(km, l.keys.Back):
		l.filter.SetValue("")
		l.filter.Blur()
		l.typing = false
		l.cursor = 0
		return l, nil
	case key.Matches(km, l.keys.Enter), km.Type == tea.KeyUp, km.Type == tea.KeyDown:
		l.filter.Blur()
		l.typing = false
		return l, nil
	}
	before := l.filter.Value()
	var c tea.Cmd
	l.filter, c = l.filter.Update(km)
	if l.filter.Value() != before {
		l.cursor, l.cfgIdx = 0, 0
	}
	return l, c
}

func (l ItemList) move(d, n int) ItemList {
	if l.panel {
		if c := len(l.configs()); c > 0 {
			l.cfgIdx = ((l.cfgIdx+d)%c + c) % c
		}
		return l
	}
	return l.moveItems(d, n)
}

func (l ItemList) moveItems(d, n int) ItemList {
	if n == 0 {
		return l
	}
	c := l.cursor + d
	if c < 0 {
		c = 0
	}
	if c > n-1 {
		c = n - 1
	}
	if c != l.cursor {
		l.cfgIdx = 0
	}
	l.cursor = c
	return l
}

func (l ItemList) toggle() ItemList {
	id := l.Current()
	if id == "" {
		return l
	}
	if l.panel {
		if cfgs := l.configs(); l.cfgIdx < len(cfgs) {
			l.sel.ToggleConfig(cfgs[l.cfgIdx].ID)
		}
		return l
	}
	if changed, note := l.sel.Toggle(id); !changed {
		l.note = note
	}
	return l
}

func (l ItemList) configs() []catalog.ConfigSpec {
	if it := l.item(); it != nil {
		return it.Configs
	}
	return nil
}

// View desenha a lista em cima e o detalhe (~40% da altura) embaixo.
func (l ItemList) View(width, height int) string {
	detailH := height * 4 / 10
	listH := height - detailH - 1 // 1 = separador
	if l.typing || l.filter.Value() != "" {
		listH--
	}
	if listH < 1 {
		listH = 1
	}
	var out []string
	if l.typing || l.filter.Value() != "" {
		out = append(out, l.filter.View())
	}
	out = append(out, l.listRows(width, listH)...)
	out = append(out, l.th.Border.Render(strings.Repeat("─", width)))
	out = append(out, l.detail(width, detailH)...)
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out[:height], "\n")
}

func (l ItemList) listRows(width, rows int) []string {
	v := l.visible()
	if len(v) == 0 {
		return pad([]string{l.th.Subtle.Render(i18n.T("tui.list.empty"))}, rows)
	}
	cur := l.cursor
	off := l.vs.offset
	if cur < off {
		off = cur
	}
	if cur >= off+rows {
		off = cur - rows + 1
	}
	if off > len(v)-rows {
		off = len(v) - rows
	}
	if off < 0 {
		off = 0
	}
	l.vs.offset = off
	nameW := 0
	for _, id := range v {
		nameW = max(nameW, lipgloss.Width(l.sel.c.Items[id].Name.Get()))
	}
	end := min(off+rows, len(v))
	out := make([]string, 0, rows)
	for i := off; i < end; i++ {
		out = append(out, l.row(v[i], i == cur, nameW, width))
	}
	return pad(out, rows)
}

func (l ItemList) row(id string, current bool, nameW, width int) string {
	it := l.sel.c.Items[id]
	x := l.sel.status(id)
	radio := false
	for _, c := range l.sel.c.Categories {
		if c.ID == it.Category && c.Exclusive {
			radio = true
		}
	}
	ver := x.Version
	if ver == "" && !present(x.Status) {
		ver = l.sel.Version(id)
	}
	name := it.Name.Get()
	if name == "" {
		name = id
	}
	name += strings.Repeat(" ", max(0, nameW-lipgloss.Width(name)))
	prefix := "  "
	if current {
		prefix = "> "
	}
	line := prefix + l.th.Check(l.sel.Marked(id), radio) + " " + name + "  " + l.th.StatusIcon(x.Status)
	if ver != "" {
		line += " " + ver
	}
	line = lipgloss.NewStyle().MaxWidth(width).Render(line)
	if ok, _ := l.sel.Selectable(id); !ok {
		return l.th.Subtle.Render(line)
	}
	if current {
		return l.th.Selected.Render(line)
	}
	return line
}

func statusText(s detect.Status) string {
	switch s {
	case detect.StatusAbsent:
		return i18n.T("tui.status.absent")
	case detect.StatusInstalled:
		return i18n.T("tui.status.installed")
	case detect.StatusOutdated:
		return i18n.T("tui.status.outdated")
	case detect.StatusExternal:
		return i18n.T("tui.status.external")
	case detect.StatusExternalOutdated:
		return i18n.T("tui.status.external_old")
	case detect.StatusProvided:
		return i18n.T("tui.status.provided")
	case detect.StatusUnsupported:
		return i18n.T("tui.status.unsupported")
	}
	return i18n.T("tui.status.unknown")
}

func (l ItemList) detail(width, rows int) []string {
	it := l.item()
	if it == nil || rows < 1 {
		return pad(nil, rows)
	}
	id := it.ID
	x := l.sel.status(id)
	var b []string
	b = append(b, l.th.Title.Render(it.Name.Get()))
	if d := it.Description.Get(); d != "" {
		b = append(b, d)
	}
	st := l.th.StatusIcon(x.Status) + " " + statusText(x.Status)
	if x.Version != "" {
		st += " " + x.Version
	}
	if x.Reason != "" {
		st += " (" + x.Reason + ")"
	}
	b = append(b, i18n.T("tui.detail.status")+": "+st)
	if x.Status == detect.StatusUnknown {
		b = append(b, l.th.Warn.Render(i18n.T("tui.unknown_hint")))
	}
	if m, _, err := backend.Choose(it, l.sel.p); err == nil {
		s := i18n.T("tui.detail.method") + ": " + m.Backend
		if m.Package != "" {
			s += " " + m.Package
		}
		if m.NeedsSudo {
			s += " " + i18n.T("tui.detail.sudo")
		}
		b = append(b, s)
	}
	if n := it.Notes.Get(); n != "" {
		b = append(b, n)
	}
	if len(l.sel.lines(id)) > 1 {
		b = append(b, i18n.T("tui.detail.version")+": ‹ "+l.sel.Version(id)+" ›")
	}
	wrap := func(ls []string) []string {
		if len(ls) == 0 {
			return nil
		}
		return strings.Split(lipgloss.NewStyle().Width(width).Render(strings.Join(ls, "\n")), "\n")
	}
	var cfg []string
	active := 0 // primeira linha (já quebrada) da config sob o cursor
	for i, cf := range it.Configs {
		mark := "  "
		if l.panel && i == l.cfgIdx {
			mark = "> "
		}
		if i == l.cfgIdx {
			active = len(wrap(cfg))
		}
		cfg = append(cfg, mark+l.th.Check(l.sel.ConfigOn(cf.ID), false)+" "+cf.Description.Get())
	}
	head, cfgL := wrap(b), wrap(cfg)
	var note []string
	if l.note != "" {
		note = wrap([]string{l.th.Warn.Render(l.note)})
	}
	// Prioridade: nota de recusa e configs ficam visíveis; o cabeçalho é cortado primeiro.
	avail := max(rows-len(note), 1)
	if len(cfgL) > avail-1 && avail > 1 {
		size := avail - 1 // mantém ao menos o título
		start := min(max(active-size+1, 0), len(cfgL)-size)
		if active < size {
			start = 0
		}
		cfgL = cfgL[start : start+size]
	} else if avail == 1 {
		cfgL = nil
	}
	room := max(avail-len(cfgL), 0)
	if len(head) > room {
		head = head[:room]
	}
	lines := append(append(head, cfgL...), note...)
	if len(lines) > rows {
		lines = lines[:rows]
	}
	return pad(lines, rows)
}

func pad(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines
}
