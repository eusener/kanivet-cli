package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/backend"
	"github.com/eusener/kanivet-cli/internal/catalog"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/platform"
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
	head   *listHead // título da categoria (só no wizard)
	gap    int       // linhas de respiro entre o título e as caixas
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

// listHead guarda o cabeçalho da categoria (só o wizard desenha).
type listHead struct {
	title       string
	page, pages int // page é de 1 a pages
}

// withHead liga a linha de título da categoria acima das caixas.
func (l ItemList) withHead(title string, page, pages int) ItemList {
	l.head = &listHead{title: title, page: page, pages: pages}
	return l
}

// sideBySide é a largura mínima para lista e detalhe lado a lado; abaixo dela o detalhe
// vai para baixo da lista.
const sideBySide = 64

// boxOverhead é o que cada caixa gasta além das linhas de conteúdo: bordas e, no modo
// Paper, a linha da sombra.
func (l ItemList) boxOverhead() int { return l.th.BoxOverhead() }

// boxInner é a largura útil (de borda a borda) de uma caixa de largura total w.
func (l ItemList) boxInner(w int) int { return l.th.BoxInner(w) }

// ink devolve um estilo sem o fundo creme (para texto dentro de caixas).
func (l ItemList) ink(c lipgloss.Color) lipgloss.Style { return l.th.Ink(c) }

// View desenha o título da categoria (no wizard) e duas caixas: lista e detalhe.
func (l ItemList) View(width, height int) string {
	var out []string
	rows := height
	if l.head != nil {
		out = append(out, l.heading(width))
		rows--
		// o respiro é o primeiro corte; a lista precisa de ao menos 6 linhas
		if rows-l.gap >= l.boxOverhead()+6 {
			for i := 0; i < l.gap; i++ {
				out = append(out, "")
				rows--
			}
		}
	}
	out = append(out, l.boxes(width, rows)...)
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out[:height], "\n")
}

// heading é "TÍTULO   CATEGORIA N DE M · X DE Y MARCADOS"; sem espaço, o subtítulo cai.
func (l ItemList) heading(width int) string {
	title := strings.ToUpper(l.head.title)
	marked := 0
	for _, id := range l.ids {
		if l.sel.Marked(id) {
			marked++
		}
	}
	sub := strings.ToUpper(i18n.T("tui.list.sub", l.head.page, l.head.pages, marked, len(l.ids)))
	if lipgloss.Width(title)+3+lipgloss.Width(sub) > width {
		return l.th.Title.Render(ansi.Truncate(title, width, "…"))
	}
	return l.th.Title.Render(title) + l.th.S().Render("   ") + l.th.Subtle.Render(sub)
}

func (l ItemList) boxes(width, h int) []string {
	o := l.boxOverhead()
	if width >= sideBySide {
		r := max(h-o, 1)
		lw := width * 56 / 100
		dw := width - 2 - lw
		left := strings.Split(l.listBox(lw, r), "\n")
		right := strings.Split(l.detailBox(dw, r), "\n")
		gap := l.th.S().Render("  ")
		out := make([]string, len(left))
		for i := range left {
			out[i] = left[i] + gap
			if i < len(right) {
				out[i] += right[i]
			}
		}
		return out
	}
	if h < 2*o+2 { // sem altura para as duas caixas: só a lista, sem cortar a borda
		return strings.Split(l.listBox(width, max(h-o, 1)), "\n")
	}
	r := h - 2*o
	lr := max(r*6/10, 1)
	dr := max(r-lr, 1)
	return append(strings.Split(l.listBox(width, lr), "\n"), strings.Split(l.detailBox(width, dr), "\n")...)
}

func (l ItemList) listBox(w, rows int) string {
	cw := l.boxInner(w)
	var body []string
	if l.typing || l.filter.Value() != "" {
		body = append(body, padTo(" "+ansi.Truncate(l.filter.View(), cw-2, "…"), cw))
		rows--
	}
	body = append(body, l.listRows(cw, max(rows, 1))...)
	return l.th.Box(strings.Join(body, "\n"), w, BoxOpts{Flush: true})
}

func (l ItemList) listRows(cw, rows int) []string {
	v := l.visible()
	if len(v) == 0 {
		empty := " " + ansi.Truncate(l.ink(colInkMuted).Render(i18n.T("tui.list.empty")), cw-2, "…")
		return pad([]string{padTo(empty, cw)}, rows)
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
	end := min(off+rows, len(v))
	out := make([]string, 0, rows)
	for i := off; i < end; i++ {
		out = append(out, l.row(v[i], i == cur, cw))
	}
	return pad(out, rows)
}

// methodTag devolve o nome do backend escolhido para o item ("" quando não há).
func methodTag(it *catalog.Item, p platform.Platform) string {
	if it == nil {
		return ""
	}
	m, _, err := backend.Choose(it, p)
	if err != nil {
		return ""
	}
	return m.Backend
}

// needsSudo diz se instalar o item pede sudo (o item ou o método escolhido).
func (l ItemList) needsSudo(it *catalog.Item) bool {
	if it.NeedsSudo {
		return true
	}
	m, _, err := backend.Choose(it, l.sel.p)
	return err == nil && m.NeedsSudo
}

// statusTag devolve a tag de estado do item ("" quando não há nada a dizer).
func (l ItemList) statusTag(id string) string {
	it := l.sel.c.Items[id]
	if it == nil {
		return ""
	}
	x := l.sel.status(id)
	switch x.Status {
	case detect.StatusInstalled, detect.StatusProvided:
		if l.sel.mode == ModeDashboard {
			return l.th.Tag(i18n.T("tui.stag.uptodate"), colMint)
		}
		return l.th.Tag(i18n.T("tui.stag.installed"), colMint)
	case detect.StatusOutdated:
		next := i18n.T("tui.stag.update")
		if l.sel.mode == ModeDashboard && x.Latest != "" {
			next = x.Latest
		}
		return l.th.Tag(pick(l.th.Nerd, "⇡ ", "^ ")+next, colSun)
	case detect.StatusExternal, detect.StatusExternalOutdated:
		return l.th.Tag(i18n.T("tui.stag.external"), colPeach)
	case detect.StatusUnsupported:
		txt := strings.ToUpper(i18n.T("tui.stag.unavailable"))
		if !l.th.Paper {
			txt = "[" + txt + "]"
		}
		return l.ink(colInkMuted).Render(txt)
	case detect.StatusAbsent:
		if l.needsSudo(it) {
			return l.th.Tag(i18n.T("tui.stag.sudo"), colSun)
		}
		if l.sel.Marked(id) {
			return l.th.Tag(i18n.T("tui.stag.new"), colSun)
		}
		if l.sel.mode == ModeDashboard {
			return l.ink(colInkMuted).Render("—")
		}
	}
	return ""
}

func (l ItemList) row(id string, current bool, cw int) string {
	it := l.sel.c.Items[id]
	x := l.sel.status(id)
	radio := false
	for _, c := range l.sel.c.Categories {
		if c.ID == it.Category && c.Exclusive {
			radio = true
		}
	}
	name := it.Name.Get()
	if name == "" {
		name = id
	}
	if current {
		name = strings.ToUpper(name)
	}
	if l.sel.mode == ModeDashboard && x.Version != "" {
		name += " " + x.Version
	}
	var tags []string
	if !present(x.Status) && x.Status != detect.StatusUnsupported {
		if m := methodTag(it, l.sel.p); m != "" {
			tags = append(tags, l.th.Tag(m, colPeach))
		}
	}
	if t := l.statusTag(id); t != "" {
		tags = append(tags, t)
	}
	tagStr := strings.Join(tags, l.th.S().Render(" "))
	tagsW := lipgloss.Width(tagStr)

	base := l.th.R.NewStyle()
	if ok, _ := l.sel.Selectable(id); !ok {
		base = l.ink(colInkMuted)
	}
	if current {
		base = l.th.Selected
	}
	mark := l.th.Check(l.sel.Marked(id), radio)
	fixed := 1 + lipgloss.Width(mark) + 1
	avail := cw - fixed - 1
	if tagsW > 0 {
		avail -= tagsW + 1
	}
	left := ansi.Truncate(name, max(avail, 0), "…")
	fill := max(cw-fixed-lipgloss.Width(left)-1-tagsW, 0)
	line := base.Render(" "+mark+" "+left) + base.Render(strings.Repeat(" ", fill)) + tagStr + base.Render(" ")
	return padTo(line, cw)
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

func (l ItemList) detailBox(w, rows int) string {
	cw := l.boxInner(w)
	lines := l.detail(max(cw-2, 1), rows)
	for i, ln := range lines {
		lines[i] = padTo(" "+ln, cw)
	}
	return l.th.Box(strings.Join(lines, "\n"), w, BoxOpts{Flush: true})
}

// wrapAll quebra cada linha em tw colunas, sem deixar nada passar da largura.
func wrapAll(ls []string, tw int) []string {
	var out []string
	for _, s := range ls {
		if s == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(s, tw, ""), "\n")...)
	}
	return out
}

func (l ItemList) detail(tw, rows int) []string {
	it := l.item()
	if it == nil || rows < 1 {
		return pad(nil, rows)
	}
	id := it.ID
	x := l.sel.status(id)
	muted := l.ink(colInkMuted)
	label := func(k string) string { return muted.Render(strings.ToUpper(i18n.T(k))) + "  " }
	name := it.Name.Get()
	if name == "" {
		name = id
	}
	b := []string{l.ink(colInkStrong).Bold(true).Render(ansi.Truncate(strings.ToUpper(name), tw, "…"))}
	if hp := strings.TrimPrefix(strings.TrimPrefix(it.Homepage, "https://"), "http://"); hp != "" {
		b = append(b, ansi.Truncate(muted.Render(hp), tw, "…"))
	}
	if d := it.Description.Get(); d != "" {
		b = append(b, "", d)
	}
	st := l.th.StatusIcon(x.Status) + " " + statusText(x.Status)
	if x.Version != "" {
		st += " " + x.Version
	}
	if x.Reason != "" {
		st += " (" + x.Reason + ")"
	}
	b = append(b, "", label("tui.detail.status")+st)
	if l.sel.mode == ModeDashboard && l.sel.at != nil {
		if at, ok := l.sel.at(id); ok && !at.IsZero() {
			b = append(b, muted.Render(strings.ToUpper(i18n.T("tui.detail.by_kanivet"))+" · "+at.Local().Format("02/01")))
		}
	}
	if x.Status == detect.StatusUnknown {
		b = append(b, l.ink(colInk).Bold(true).Render(i18n.T("tui.unknown_hint")))
	}
	if m, _, err := backend.Choose(it, l.sel.p); err == nil {
		s := label("tui.detail.method") + m.Backend
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
		b = append(b, label("tui.detail.version")+"‹ "+l.sel.Version(id)+" ›")
	}
	var cfg []string
	active := 0 // primeira linha (já quebrada) da config sob o cursor
	for i, cf := range it.Configs {
		mark := "  "
		if l.panel && i == l.cfgIdx {
			mark = "> "
		}
		if i == l.cfgIdx {
			active = len(wrapAll(cfg, tw))
		}
		cfg = append(cfg, mark+l.th.Check(l.sel.ConfigOn(cf.ID), false)+" "+cf.Description.Get())
	}
	head, cfgL := wrapAll(b, tw), wrapAll(cfg, tw)
	var note []string
	if l.note != "" {
		note = wrapAll([]string{l.ink(colInk).Bold(true).Render(l.note)}, tw)
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
	// primeiro some o respiro entre os blocos; depois o resto é cortado com "…"
	for len(head) > room {
		blank := -1
		for i := len(head) - 1; i > 0; i-- {
			if head[i] == "" {
				blank = i
				break
			}
		}
		if blank < 0 {
			break
		}
		head = append(head[:blank], head[blank+1:]...)
	}
	if len(head) > room {
		head = head[:room]
		if room > 0 {
			head[room-1] = ansi.Truncate(head[room-1], max(tw-1, 0), "") + "…"
		}
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
