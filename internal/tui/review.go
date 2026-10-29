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
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/configs"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
)

// planMsg entrega o plano montado para a revisão (ou o erro que o bloqueia).
type planMsg struct {
	pl     *plan.Plan
	groups []app.Group
	err    error
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
	stepID, path string
	// linhas de arquivo: marcador à frente, caminho (encurtável) e marcas depois
	lead, post string
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
	note     string // aviso curto (ex.: sem diff para a linha)
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
	sel := m.sel.ToPlan()
	m.screen = ScreenReview
	m.review = reviewState{caller: from, profile: sel.Profile, loading: true}
	s := m.s
	return m, func() tea.Msg {
		pl, _, err := s.Plan(context.Background(), sel)
		var groups []app.Group
		if err == nil {
			groups = s.GroupPlan(pl)
		}
		return planMsg{pl: pl, groups: groups, err: err}
	}
}

func (m Model) onPlan(msg planMsg) Model {
	if m.screen != ScreenReview {
		return m
	}
	r := m.review
	r.loading, r.pl, r.groups, r.err = false, msg.pl, msg.groups, msg.err
	r.cursor, r.diffOpen = 0, false
	r.rows = buildRows(r.groups)
	m.review = r
	return m.layoutReview()
}

// fileParts separa o marcador, o caminho e as marcas finais ((backup), rc) de um arquivo.
func fileParts(f app.FileLine) (lead, post string) {
	if f.Rc {
		post = "  (" + i18n.T("plan.rc_block") + ")"
		if f.Exists {
			post += " (" + i18n.T("plan.backup") + ")"
		}
		return "→ ", post
	}
	lead = "→ "
	if f.Remove {
		lead = "✗ "
	}
	if f.Exists && !f.Remove {
		post = "  (" + i18n.T("plan.backup") + ")"
	}
	return lead, post
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

func buildRows(groups []app.Group) []reviewRow {
	var rows []reviewRow
	for gi, g := range groups {
		if gi > 0 {
			rows = append(rows, reviewRow{})
		}
		rows = append(rows, reviewRow{text: g.Title, style: rowTitle})
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
				lead, post := fileParts(f)
				rows = append(rows, reviewRow{stepID: l.StepID, path: f.Path, lead: lead, post: post, file: true, rc: f.Rc})
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

func (r reviewState) hasSudo() bool {
	if r.pl == nil {
		return false
	}
	for _, st := range r.pl.Steps {
		if st.NeedsSudo {
			return true
		}
	}
	return false
}

func (r reviewState) blocked() bool { return r.err != nil }
func (r reviewState) empty() bool   { return r.err == nil && r.pl != nil && len(r.pl.Steps) == 0 }

// header devolve as linhas fixas do topo: contadores e faixa de sudo.
func (m Model) reviewHeader() []string {
	r := m.review
	if r.loading || r.err != nil || r.pl == nil {
		return nil
	}
	i, u, rm, c, s := r.counts()
	h := []string{m.th.Accent.Render(i18n.T("tui.review.counts", i, u, rm, c, s))}
	if r.hasSudo() {
		h = append(h, m.th.Warn.Render(i18n.T("tui.sudo_banner")))
	}
	if r.note != "" {
		return append(h, m.th.Warn.Render(r.note))
	}
	return append(h, "")
}

// layoutReview ajusta o viewport ao tamanho atual e mantém o cursor visível.
func (m Model) layoutReview() Model {
	r := m.review
	body := m.h - 3
	height := body - len(m.reviewHeader())
	if r.empty() {
		height -= 2
	}
	if height < 1 {
		height = 1
	}
	r.vp.Width, r.vp.Height = m.w, height
	r.diff.Width, r.diff.Height = m.w, body-1
	r.vp.SetContent(m.renderRows(r))
	switch {
	case r.cursor < r.vp.YOffset:
		r.vp.SetYOffset(r.cursor)
	case r.cursor >= r.vp.YOffset+height:
		r.vp.SetYOffset(r.cursor - height + 1)
	}
	m.review = r
	return m
}

func (m Model) renderRows(r reviewState) string {
	out := make([]string, len(r.rows))
	for i, row := range r.rows {
		prefix := "  "
		if i == r.cursor {
			prefix = m.th.Selected.Render("> ")
		}
		st := lipgloss.NewStyle()
		if row.file {
			// as marcas (backup/rc/remover) nunca são cortadas: o caminho é que encolhe
			fixed := lipgloss.Width(prefix) + 6 + lipgloss.Width(row.lead) + lipgloss.Width(row.post)
			row.text = "      " + row.lead + shorten(row.path, m.w-fixed) + row.post
		}
		switch row.style {
		case rowTitle:
			st = m.th.Title
		case rowWarn:
			st = m.th.Warn
		case rowSubtle:
			st = m.th.Subtle
		}
		out[i] = lipgloss.NewStyle().MaxWidth(m.w).Render(prefix + st.Render(row.text))
	}
	return strings.Join(out, "\n")
}

func (m Model) reviewLines(height int) []string {
	r := m.review
	if r.diffOpen {
		head := m.th.Title.Render(i18n.T("tui.review.diff_title", r.diffPath))
		return append([]string{head}, strings.Split(r.diff.View(), "\n")...)
	}
	switch {
	case r.loading:
		return []string{i18n.T("tui.review.loading")}
	case r.err != nil:
		var msgs []string
		var pe *plan.PlanError
		if errors.As(r.err, &pe) {
			msgs = pe.Msgs
		} else {
			msgs = []string{r.err.Error()}
		}
		box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.NoColor{}).
			Width(m.w - 2).Render(m.th.Danger.Render(i18n.T("tui.review.error") + "\n" + strings.Join(msgs, "\n")))
		return strings.Split(box, "\n")
	}
	lines := m.reviewHeader()
	if r.empty() {
		lines = append(lines, i18n.T("plan.empty"), "")
	}
	return append(lines, strings.Split(r.vp.View(), "\n")...)
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
	back := func() Model {
		if r.caller == ScreenDashboard {
			m.screen = ScreenDashboard
			return m
		}
		m.screen = ScreenCategories
		m.page = len(categoryPages) - 1
		if m.sel != nil {
			m.list = m.newList()
		}
		return m
	}
	switch {
	case key.Matches(km, m.keys.Back):
		return back(), nil
	case key.Matches(km, m.keys.Enter):
		switch {
		case r.loading || r.blocked():
		case r.empty():
			return back(), nil
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
	r.diff.SetContent(diffContent(text, m.w))
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
	if r.blocked() || r.loading {
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
