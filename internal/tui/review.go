package tui

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

// planMsg entrega o plano montado para a revisão (ou o erro que o bloqueia).
type planMsg struct {
	pl     *plan.Plan
	groups []app.Group
	err    error
	req    int // pedido que gerou o plano: um planMsg de uma revisão anterior é descartado
}

type rowStyle int

const (
	rowPlain rowStyle = iota
	rowTitle
	rowWarn
	rowSubtle
)

// reviewRow é uma linha desenhada; stepID/path ligam as linhas de config ao diff.
type reviewRow struct {
	text         string
	style        rowStyle
	kind         app.GroupKind // linhas de título: o grupo que a tag abre
	stepID, path string
	// linhas de arquivo: marcador à frente, caminho (encurtável), nota e tag BACKUP depois
	lead, note string
	backup     bool
	file, rc   bool
}

type reviewState struct {
	caller   Screen // tela para onde o esc volta (wizard ou dashboard)
	profile  string
	loading  bool
	pl       *plan.Plan
	groups   []app.Group
	err      error
	rows     []reviewRow
	cursor   int
	vp       viewport.Model
	diffOpen bool
	diff     viewport.Model
	diffPath string
	diffText string // o diff cru; as linhas são pintadas no layout, na largura atual
	note     string // aviso curto (ex.: sem diff para a linha)
	// retomada: o plano são só os passos pendentes de journal, que o enter continua
	resume  bool
	journal *state.Journal
	req     int // o planMsg esperado (ver planMsg.req)
}

// rowKey devolve o id do passo da linha (vazio nas linhas sem passo).
func (r reviewState) rowKey(i int) string {
	if i < 0 || i >= len(r.rows) {
		return ""
	}
	return r.rows[i].stepID
}

// beginReview entra na revisão vindo de from e dispara o cálculo do plano.
func (m Model) beginReview(from Screen) (Model, tea.Cmd) {
	return m.beginReviewWith(from, m.sel.ToPlan())
}

// beginReviewWith entra na revisão com uma seleção explícita (o "tentar de novo" do resumo).
func (m Model) beginReviewWith(from Screen, sel plan.Selection) (Model, tea.Cmd) {
	m.screen = ScreenReview
	m.planSeq++
	req := m.planSeq
	m.review = reviewState{caller: from, profile: sel.Profile, loading: true, req: req}
	s := m.s
	return m, func() tea.Msg {
		pl, _, err := s.Plan(context.Background(), sel)
		var groups []app.Group
		if err == nil {
			groups = s.GroupPlan(pl)
		}
		return planMsg{pl: pl, groups: groups, err: err, req: req}
	}
}

func (m Model) onPlan(msg planMsg) Model {
	if m.screen != ScreenReview || msg.req != m.review.req {
		return m
	}
	r := m.review
	r.loading, r.pl, r.groups, r.err = false, msg.pl, msg.groups, msg.err
	r.cursor, r.diffOpen = 0, false
	r.rows = buildRows(r.groups)
	m.review = r
	return m.layoutReview()
}

// fileParts separa o marcador, a nota do rc e se a tag BACKUP acompanha um arquivo.
func fileParts(f app.FileLine) (lead, note string, backup bool) {
	if f.Rc {
		return "→ ", "(" + i18n.T("plan.rc_block") + ")", f.Exists
	}
	lead = "→ "
	if f.Remove {
		lead = "✗ "
	}
	return lead, "", f.Exists && !f.Remove
}

// shorten corta o meio do caminho para caber em max colunas.
func shorten(path string, max int) string {
	r := []rune(path)
	if len(r) <= max {
		return path
	}
	if max < 5 {
		max = 5
	}
	keep := max - 1
	tail := keep / 2
	head := keep - tail
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// groupTag devolve o rótulo e a cor da tag que abre um grupo.
func groupTag(k app.GroupKind) (string, lipgloss.Color) {
	switch k {
	case app.GroupSudo:
		return i18n.T("tui.review.tag.sudo"), colSun
	case app.GroupInstall:
		return i18n.T("tui.review.tag.install"), colSky
	case app.GroupUpdate:
		return i18n.T("tui.review.tag.update"), colSky
	case app.GroupUninstall:
		return i18n.T("tui.review.tag.uninstall"), colCoral
	case app.GroupConfig:
		return i18n.T("tui.review.tag.config"), colMint
	case app.GroupOrphans:
		return i18n.T("tui.review.tag.orphans"), colPeach
	case app.GroupWarnings:
		return i18n.T("tui.review.tag.warnings"), colSun
	}
	return i18n.T("tui.review.tag.skip"), colPeach
}

func buildRows(groups []app.Group) []reviewRow {
	var rows []reviewRow
	for gi, g := range groups {
		if gi > 0 {
			rows = append(rows, reviewRow{})
		}
		label, _ := groupTag(g.Kind)
		rows = append(rows, reviewRow{text: label, style: rowTitle, kind: g.Kind})
		switch g.Kind {
		case app.GroupSudo: // a senha é pedida uma vez
			rows = append(rows, reviewRow{text: "  " + i18n.T("tui.sudo_banner"), style: rowSubtle})
		case app.GroupOrphans:
			rows = append(rows, reviewRow{text: "  " + g.Title, style: rowSubtle})
		}
		for _, l := range g.Lines {
			if g.Kind == app.GroupWarnings {
				rows = append(rows, reviewRow{text: "  ! " + l.Text, style: rowWarn})
				continue
			}
			rows = append(rows, reviewRow{text: "  " + l.Text, stepID: l.StepID})
			for _, c := range l.Cmds {
				rows = append(rows, reviewRow{text: "      $ " + c, style: rowSubtle, stepID: l.StepID})
			}
			for _, f := range l.Files {
				lead, note, backup := fileParts(f)
				rows = append(rows, reviewRow{stepID: l.StepID, path: f.Path, lead: lead, note: note, backup: backup, file: true, rc: f.Rc})
			}
		}
	}
	return rows
}

// counts devolve instalar, atualizar, remover, configurar e pular.
func (r reviewState) counts() (inst, upd, rem, cfg, skip int) {
	if r.pl == nil {
		return
	}
	for _, st := range r.pl.Steps {
		switch st.Kind {
		case plan.KindInstall:
			inst++
		case plan.KindUpdate:
			upd++
		case plan.KindUninstall:
			rem++
		case plan.KindConfig, plan.KindShell, plan.KindChsh:
			cfg++
		}
	}
	return inst, upd, rem, cfg, len(r.pl.Skipped)
}

func (r reviewState) sudoCount() (n int) {
	if r.pl == nil {
		return 0
	}
	for _, st := range r.pl.Steps {
		if st.NeedsSudo {
			n++
		}
	}
	return n
}

func (r reviewState) hasSudo() bool { return r.sudoCount() > 0 }

func (r reviewState) blocked() bool { return r.err != nil }
func (r reviewState) empty() bool   { return r.err == nil && r.pl != nil && len(r.pl.Steps) == 0 }

// reviewRowsHeight é quantas linhas de conteúdo cabem na caixa dos grupos: o corpo menos
// as bordas e a linha reservada ao aviso (r.note), que fica logo abaixo da caixa.
func (m Model) reviewRowsHeight() int {
	return max(m.bodyHeight()-m.th.BoxOverhead()-1, 1)
}

// diffDialogWidth é a largura total da caixa centralizada do diff.
func (m Model) diffDialogWidth() int { return min(m.contentWidth(), 100) }

// diffRowsHeight é a altura do viewport do diff: o diff inteiro, se couber (a primeira
// linha da caixa é o caminho do arquivo).
func (m Model) diffRowsHeight(r reviewState) int {
	return min(max(r.diff.TotalLineCount(), 1), max(m.bodyHeight()-m.th.BoxOverhead()-1, 1))
}

// layoutReview ajusta o viewport ao tamanho atual e mantém o cursor visível.
func (m Model) layoutReview() Model {
	r := m.review
	height := m.reviewRowsHeight()
	cw := m.th.BoxInner(m.contentWidth())
	r.vp.Width, r.vp.Height = cw, height
	if r.diffOpen {
		dw := m.th.BoxInner(m.diffDialogWidth())
		r.diff.Width = dw
		r.diff.SetContent(paintDiff(m.th, r.diffText, dw))
		r.diff.Height = m.diffRowsHeight(r) // altura < 1 deixa o YOffset passar do fim (pânico no View)
		r.diff.SetYOffset(r.diff.YOffset)
	}
	r.vp.SetContent(m.renderRows(r))
	r.vp.SetYOffset(r.vp.YOffset)
	switch {
	case r.cursor < r.vp.YOffset:
		r.vp.SetYOffset(r.cursor)
	case r.cursor >= r.vp.YOffset+height:
		r.vp.SetYOffset(r.cursor - height + 1)
	}
	m.review = r
	return m
}

// renderRows desenha todas as linhas na largura útil da caixa; a do cursor vai em sky
// de borda a borda (no modo plain, com "> " e vídeo invertido).
func (m Model) renderRows(r reviewState) string {
	cw := m.th.BoxInner(m.contentWidth())
	out := make([]string, len(r.rows))
	for i, row := range r.rows {
		out[i] = m.renderRow(row, i == r.cursor, cw)
	}
	return strings.Join(out, "\n")
}

func (m Model) renderRow(row reviewRow, sel bool, cw int) string {
	th := m.th
	lead := "  "
	if sel && !th.Paper {
		lead = "> "
	}
	// base é o estilo do texto; o cursor troca por sky (ou vídeo invertido)
	base := th.R.NewStyle()
	switch row.style {
	case rowWarn:
		base = th.Ink(colInk).Bold(true)
	case rowSubtle:
		base = th.Ink(colInkMuted)
	}
	if row.file {
		base = th.R.NewStyle()
	}
	if sel {
		base = th.Selected
	}
	line := func(left, tags string) string {
		fill := max(cw-lipgloss.Width(lead)-lipgloss.Width(left)-lipgloss.Width(tags), 0)
		return padTo(base.Render(lead+left)+tags+base.Render(strings.Repeat(" ", fill)), cw)
	}
	room := max(cw-lipgloss.Width(lead)-1, 0)
	switch {
	case row.file:
		// a nota e a tag BACKUP nunca são cortadas: o caminho é que encolhe
		var tags string
		note := ""
		if row.note != "" {
			note = "  " + row.note
		}
		if row.backup {
			tags = " " + th.Tag(i18n.T("plan.backup"), colPeach)
		}
		fixed := 6 + lipgloss.Width(row.lead) + lipgloss.Width(note) + lipgloss.Width(tags) + 1
		left := "      " + row.lead + shorten(row.path, cw-lipgloss.Width(lead)-fixed) + note
		if !sel && note != "" {
			left = "      " + row.lead + shorten(row.path, cw-lipgloss.Width(lead)-fixed) + th.Ink(colInkMuted).Render(note)
		}
		return line(left, tags)
	case row.style == rowTitle:
		label, c := groupTag(row.kind)
		return line("", th.Tag(label, c)) // no cursor a tag se mantém; o resto da linha vai em sky
	case row.text == "":
		return line("", "")
	}
	text := row.text[len(row.text)-len(strings.TrimLeft(row.text, " ")):]
	indent := row.text[:len(row.text)-len(text)]
	if sel && row.style == rowPlain && row.stepID != "" { // o nome do passo em maiúsculas
		name, rest, found := strings.Cut(text, "  ")
		text = strings.ToUpper(name)
		if found {
			text += "  " + rest
		}
	}
	return line(ansi.Truncate(indent+text, room, "…"), "")
}

// paintDiff pinta as linhas de um diff na largura w: "+" em mint e "-" em coral-tint,
// de borda a borda. No modo plain ficam só os marcadores.
func paintDiff(th Theme, text string, w int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	header := len(lines) > 1 && strings.HasPrefix(lines[0], "--- ")
	for i, l := range lines {
		l = strings.ReplaceAll(strings.TrimRight(l, "\r"), "\t", "    ")
		l = padTo(ansi.Truncate(" "+l, w, "…"), w)
		switch {
		case !th.Paper:
		case header && i < 2:
			l = th.Ink(colInkMuted).Bold(true).Render(l)
		case strings.HasPrefix(l, " +"):
			l = th.R.NewStyle().Background(colMint).Foreground(colInkStrong).Render(l)
		case strings.HasPrefix(l, " -"):
			l = th.R.NewStyle().Background(colCoral).Foreground(colInkStrong).Render(l)
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// reviewHeading é o título da tela com as tags de totais (as que não couberem caem).
func (m Model) reviewHeading() string {
	title := m.th.Title.Render(m.upperTitle())
	r := m.review
	if r.loading || r.err != nil || r.pl == nil {
		return ansi.Truncate(title, m.contentWidth(), "…")
	}
	inst, upd, rem, cfg, _ := r.counts()
	tags := []string{m.th.Tag(i18n.T("tui.review.total.install", inst), colSky)}
	if upd > 0 {
		tags = append(tags, m.th.Tag(i18n.T("tui.review.total.update", upd), colSky))
	}
	if rem > 0 {
		tags = append(tags, m.th.Tag(i18n.T("tui.review.total.remove", rem), colCoral))
	}
	tags = append(tags, m.th.Tag(i18n.N("tui.review.total.configs", cfg), colMint))
	if n := r.sudoCount(); n > 0 {
		tags = append(tags, m.th.Tag(i18n.T("tui.review.total.sudo", n), colSun))
	}
	sp := m.th.S().Render(" ")
	for len(tags) > 0 {
		line := title + m.th.S().Render("   ") + strings.Join(tags, sp)
		if lipgloss.Width(line) <= m.contentWidth() {
			return line
		}
		tags = tags[:len(tags)-1]
	}
	return ansi.Truncate(title, m.contentWidth(), "…")
}

func (m Model) reviewLines(height int) []string {
	r := m.review
	cw := m.contentWidth()
	lines := []string{m.reviewHeading()}
	gap := m.respiro() / 2
	for i := 0; i < gap; i++ {
		lines = append(lines, "")
	}
	box := func(body string, o BoxOpts) []string { return strings.Split(m.th.Box(body, cw, o), "\n") }
	switch {
	case r.diffOpen:
		dw := m.diffDialogWidth()
		in := m.th.BoxInner(dw)
		body := m.th.Ink(colInkMuted).Render(padTo(" "+shorten(r.diffPath, max(in-2, 5)), in)) + "\n" + r.diff.View()
		return append(lines, m.centerBox(body, i18n.T("tui.review.diff_title"), dw, height, len(lines), true)...)
	case r.loading:
		return append(lines, box(i18n.T("tui.review.loading"), BoxOpts{})...)
	case r.err != nil:
		var msgs []string
		var pe *plan.PlanError
		if errors.As(r.err, &pe) {
			msgs = pe.Msgs
		} else {
			msgs = []string{r.err.Error()}
		}
		body := m.th.Ink(colInkStrong).Bold(true).Render(i18n.T("tui.review.error")) + "\n" + strings.Join(msgs, "\n")
		return append(lines, box(body, BoxOpts{Fill: colCoral, Fg: colInkStrong})...)
	case r.empty():
		return append(lines, box(i18n.T("plan.empty"), BoxOpts{})...)
	}
	lines = append(lines, box(r.vp.View(), BoxOpts{Flush: true})...)
	if r.note != "" {
		lines = append(lines, m.th.Warn.Render(ansi.Truncate(r.note, cw, "…")))
	}
	return lines
}

func (m Model) updateReview(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	r := m.review
	m.review.note = ""
	if r.diffOpen {
		if key.Matches(km, m.keys.Back) {
			m.review.diffOpen = false
			return m, nil
		}
		var c tea.Cmd
		m.review.diff, c = r.diff.Update(km)
		return m, c
	}
	back := func() (tea.Model, tea.Cmd) {
		if r.resume { // descartar a retomada não apaga o journal: o próximo run o substitui
			m.review = reviewState{}
			if r.caller == ScreenDashboard {
				return m.enterDashboard()
			}
			m.screen = r.caller
			return m, nil
		}
		if r.caller == ScreenDashboard || r.caller == ScreenSummary {
			m.screen = r.caller
			return m, nil
		}
		m.screen = ScreenCategories
		m.page = len(categoryPages) - 1
		if m.sel != nil {
			m.list = m.newList()
		}
		return m, nil
	}
	switch {
	case key.Matches(km, m.keys.Back):
		return back()
	case key.Matches(km, m.keys.Enter):
		switch {
		// m.detecting: na retomada a revisão abre com o detectCmd ainda rodando; o run espera por ele
		case r.loading || r.blocked() || m.detecting:
		case r.empty():
			return back()
		default:
			m.plan = r.pl
			return m.startRun()
		}
	case r.loading || r.blocked():
	case key.Matches(km, m.keys.Diff):
		return m.openDiff(), nil
	case key.Matches(km, m.keys.Up):
		m = m.moveCursor(-1)
	case key.Matches(km, m.keys.Down):
		m = m.moveCursor(1)
	case km.String() == "pgup":
		m = m.moveCursor(-r.vp.Height)
	case km.String() == "pgdown":
		m = m.moveCursor(r.vp.Height)
	}
	return m, nil
}

func (m Model) moveCursor(d int) Model {
	r := m.review
	r.cursor += d
	if r.cursor >= len(r.rows) {
		r.cursor = len(r.rows) - 1
	}
	if r.cursor < 0 {
		r.cursor = 0
	}
	m.review = r
	return m.layoutReview()
}

// openDiff abre o painel com o diff entre o arquivo atual e o template renderizado.
func (m Model) openDiff() Model {
	r := m.review
	if r.cursor >= len(r.rows) {
		return m
	}
	row := r.rows[r.cursor]
	var fc *plan.FileChange
	for si := range r.pl.Steps {
		st := &r.pl.Steps[si]
		if st.ID != row.stepID {
			continue
		}
		for fi := range st.Files {
			f := &st.Files[fi]
			if f.Template == "" || f.Remove || f.Dir {
				continue
			}
			if row.path == "" || m.s.ExpandTarget(f.Target) == row.path {
				fc = f
				break
			}
		}
	}
	if fc == nil {
		if row.rc {
			r.note = i18n.T("tui.review.no_diff_rc")
		} else {
			r.note = i18n.T("tui.review.no_diff")
		}
		m.review = r
		return m
	}
	target := m.s.ExpandTarget(fc.Target)
	vars := configs.VarsFor(m.s.P, m.s.Getenv)
	vars.Extras = m.s.ExtrasFor(r.profile)
	var text string
	if next, err := configs.Render(m.s.Cat.Files, fc.Template, vars); err != nil {
		text = err.Error()
	} else {
		cur, rerr := os.ReadFile(target)
		if rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			r.note = i18n.T("tui.review.diff_read_error", target, rerr.Error())
			m.review = r
			return m
		}
		text = configs.Diff(string(cur), string(next))
	}
	r.diffText = text
	r.diff.GotoTop()
	r.diffOpen, r.diffPath = true, target
	m.review = r
	return m.layoutReview()
}

func (m Model) reviewBindings() []key.Binding {
	r := m.review
	if r.diffOpen {
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Back, m.keys.Quit}
	}
	enter := m.keys.Enter
	if r.blocked() || r.loading || m.detecting {
		enter.SetEnabled(false)
	}
	if r.empty() {
		enter.SetHelp("enter", i18n.T("tui.key.back"))
	}
	return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Diff, enter, m.keys.Back, m.keys.Help, m.keys.Quit}
}

// diffContent prepara o texto de um diff para um viewport: corta cada linha na largura w.
func diffContent(text string, w int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(w).Render(l)
	}
	return strings.Join(lines, "\n")
}
