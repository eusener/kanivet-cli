package tui

import (
	"context"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

// logTail é quantas linhas do fim do log o resumo abre.
const logTail = 200

type summaryPanel int

const (
	panelNone summaryPanel = iota
	panelLog
	panelDiff
)

type summaryState struct {
	cursor  int
	panel   summaryPanel
	vp      viewport.Model
	title   string
	confirm bool              // F pediu confirmação (y/N) para o conflito do cursor
	forcing bool              // reaplicação em andamento
	outcome map[string]string // resultado da reaplicação, por caminho
}

// forceDoneMsg é o resultado de reaplicar uma config com força.
type forceDoneMsg struct {
	target string
	rep    executor.Report
	err    error
}

type sumRow struct {
	failed bool
	idx    int // índice em runRep.Failed ou runRep.Conflicts
}

func (m Model) summaryRows() []sumRow {
	var rows []sumRow
	for i := range m.runRep.Failed {
		rows = append(rows, sumRow{failed: true, idx: i})
	}
	for i := range m.runRep.Conflicts {
		rows = append(rows, sumRow{idx: i})
	}
	return rows
}

func (m Model) summaryRow() (sumRow, bool) {
	rows := m.summaryRows()
	if m.sum.cursor < 0 || m.sum.cursor >= len(rows) {
		return sumRow{}, false
	}
	return rows[m.sum.cursor], true
}

// logName espelha o nome do log usado pelo executor e pela CLI: o item, ou o id do passo com ":" trocado por "-".
func (m Model) logName(stepID string) string {
	if m.plan != nil {
		if s, ok := m.plan.Step(stepID); ok && s.ItemID != "" {
			return s.ItemID
		}
	}
	return strings.ReplaceAll(stepID, ":", "-")
}

func (m Model) failLog(stepID string) string {
	if m.runJ == nil {
		return ""
	}
	return m.runJ.LogPath(m.logName(stepID))
}

// nextSteps junta o aviso de terminal novo (houve passo de shell) e as notas dos itens instalados.
func (m Model) nextSteps() []string {
	var out []string
	var notes []string
	seen := map[string]bool{}
	for _, id := range m.runRep.Done {
		if id == plan.ShellStepID {
			out = append(out, i18n.T("summary.new_terminal"))
			continue
		}
		if m.plan == nil {
			continue
		}
		st, ok := m.plan.Step(id)
		if !ok || st.Kind != plan.KindInstall || seen[st.ItemID] {
			continue
		}
		seen[st.ItemID] = true
		if it := m.s.Cat.Items[st.ItemID]; it != nil {
			if n := strings.TrimSpace(it.Notes.Get()); n != "" {
				notes = append(notes, it.Name.Get()+": "+n)
			}
		}
	}
	return append(out, notes...)
}

// stepItem acha o passo no plano do run (m.exec.steps primeiro, depois m.plan).
func (m Model) stepItem(id string) (plan.Step, bool) {
	if i, ok := m.exec.index[id]; ok && i < len(m.exec.steps) {
		return m.exec.steps[i], true
	}
	if m.plan != nil {
		return m.plan.Step(id)
	}
	return plan.Step{}, false
}

// retrySelection monta a seleção do "tentar de novo": só os itens dos passos que falharam e
// que ainda são selecionáveis, com o perfil, as versões e as configs da seleção atual.
func (m Model) retrySelection() (plan.Selection, bool) {
	var out plan.Selection
	if m.sel == nil {
		return out, false
	}
	cur := m.sel.ToPlan()
	seen := map[string]bool{}
	for _, id := range m.runRep.Failed {
		st, ok := m.stepItem(id)
		if ok && st.Kind == plan.KindChsh { // a troca de shell volta como troca de shell, não como instalação do zsh
			out.SwitchToZsh = true
			continue
		}
		if !ok || st.ItemID == "" || seen[st.ItemID] || st.Kind == plan.KindUninstall {
			continue
		}
		if sel, _ := m.sel.Selectable(st.ItemID); !sel {
			continue
		}
		seen[st.ItemID] = true
		if st.Kind == plan.KindUpdate {
			out.Update = append(out.Update, st.ItemID)
		} else {
			out.Add = append(out.Add, st.ItemID)
		}
	}
	// só o que é dos itens que falham: o perfil expandiria itens que não falharam (e os padrões exclusivos)
	for id := range seen {
		if v, ok := cur.Versions[id]; ok {
			if out.Versions == nil {
				out.Versions = map[string]string{}
			}
			out.Versions[id] = v
		}
		if it := m.s.Cat.Items[id]; it != nil {
			for _, cfg := range it.Configs {
				if v, ok := cur.Configs[cfg.ID]; ok {
					if out.Configs == nil {
						out.Configs = map[string]bool{}
					}
					out.Configs[cfg.ID] = v
				}
			}
		}
	}
	return out, len(out.Add)+len(out.Update) > 0 || out.SwitchToZsh
}

// canRetry diz se o "tentar de novo" tem o que tentar.
func (m Model) canRetry() bool {
	_, ok := m.retrySelection()
	return ok
}

func (m Model) interrupted() bool {
	return m.exec.interrupted || errors.Is(m.runErr, context.Canceled)
}

// summaryHeading é o título grande com o resultado do run.
func (m Model) summaryHeading() string {
	var t string
	switch {
	case m.interrupted():
		t = i18n.T("tui.summary.title_interrupted")
	case len(m.runRep.Failed) > 0 || m.runErr != nil:
		t = i18n.T("tui.summary.title_failed")
	default:
		t = i18n.T("tui.summary.title_ok", fmtDur(m.exec.elapsedNow()))
	}
	return ansi.Truncate(m.th.Title.Render(strings.ToUpper(t)), m.contentWidth(), "…")
}

// summaryTags são as tags de totais; as que não couberem caem do fim.
func (m Model) summaryTags() string {
	r := m.runRep
	inst, cfg := 0, 0
	for _, id := range r.Done {
		if st, ok := m.stepItem(id); ok {
			switch st.Kind {
			case plan.KindInstall, plan.KindUpdate:
				inst++
			case plan.KindConfig:
				cfg++
			}
		}
	}
	tags := []string{
		m.th.Tag(i18n.N("tui.summary.tag.installed", inst), colMint),
		m.th.Tag(i18n.N("tui.summary.tag.configs", cfg), colSky),
	}
	if n := len(r.Failed); n > 0 {
		tags = append(tags, m.th.Tag(i18n.N("tui.summary.tag.failed", n), colCoral))
	}
	if n := len(r.Skipped); n > 0 {
		tags = append(tags, m.th.Tag(i18n.N("tui.summary.tag.skipped", n), colPeach))
	}
	if n := len(r.Conflicts); n > 0 {
		tags = append(tags, m.th.Tag(i18n.N("tui.summary.tag.conflicts", n), colSun))
	}
	sp := m.th.S().Render(" ")
	for len(tags) > 1 && lipgloss.Width(strings.Join(tags, sp)) > m.contentWidth() {
		tags = tags[:len(tags)-1]
	}
	return ansi.Truncate(strings.Join(tags, sp), m.contentWidth(), "")
}

// failReason é o motivo da falha: o erro do passo, ou o caminho do log quando não há.
func (m Model) failReason(id string) string {
	if n := strings.Join(strings.Fields(m.exec.notes[id]), " "); n != "" {
		return n
	}
	if p := m.failLog(id); p != "" {
		return i18n.T("summary.log", p)
	}
	return ""
}

// failName é o nome mostrado do passo que falhou: o do item, ou o id do passo.
func (m Model) failName(id string) string {
	if st, ok := m.stepItem(id); ok && st.ItemID != "" {
		if it := m.s.Cat.Items[st.ItemID]; it != nil {
			if n := it.Name.Get(); n != "" {
				return n
			}
		}
		return st.ItemID
	}
	return id
}

// sumRow desenha uma linha de caixa do resumo: glifo, nome (maiúsculas se selecionada) e o resto em ink-muted.
func (m Model) sumRow(glyph, name, rest string, sel bool, in int) string {
	base := m.th.R.NewStyle()
	muted := m.th.Ink(colInkMuted)
	if sel {
		base, muted = m.th.Selected, m.th.Selected
		name = strings.ToUpper(name)
	}
	lead := base.Render(" ")
	if glyph != "" {
		g := m.th.Danger.Render(glyph)
		if sel {
			g = base.Render(glyph)
		}
		lead += g + base.Render(" ")
	}
	room := max(in-lipgloss.Width(lead)-1, 0)
	text := name
	if rest != "" {
		text += "  " + rest
	}
	text = ansi.Truncate(text, room, "…")
	head := ansi.Truncate(name, room, "…")
	if lipgloss.Width(text) > lipgloss.Width(head) {
		text = base.Render(head) + muted.Render(ansi.Cut(text, lipgloss.Width(head), lipgloss.Width(text)))
	} else {
		text = base.Render(text)
	}
	line := lead + text
	if sel {
		line += base.Render(strings.Repeat(" ", max(in-lipgloss.Width(line), 0)))
	}
	return ansi.Truncate(line, in, "")
}

// keyHint é "L VER LOG" com a tecla em negrito, para o cartão de falhas.
func (m Model) keyHint(k, desc string) string {
	return m.th.Ink(colInkStrong).Bold(true).Render(strings.ToUpper(k)) + m.th.Ink(colInkStrong).Render(" "+strings.ToUpper(desc))
}

// window devolve o início de uma janela de room linhas sobre n, com cur visível (cur<0: início).
func window(n, cur, room int) int {
	if n <= room || cur < 0 {
		return 0
	}
	return min(max(cur-room/2, 0), n-room)
}

func (m Model) summaryLines(height int) []string {
	if m.sum.panel != panelNone {
		head := m.th.Title.Render(m.sum.title)
		return append([]string{ansi.Truncate(head, m.contentWidth(), "…")}, strings.Split(m.sum.vp.View(), "\n")...)
	}
	cw := m.contentWidth()
	in := m.th.BoxInner(cw)
	oh := m.th.BoxOverhead()
	gap := m.respiro() / 2
	r := m.runRep
	lines := []string{m.summaryHeading()}
	blank := func() {
		for i := 0; i < gap; i++ {
			lines = append(lines, "")
		}
	}
	blank()
	lines = append(lines, m.summaryTags())
	blank()
	if m.runErr != nil && !m.interrupted() {
		lines = append(lines, m.th.Danger.Render(ansi.Truncate(m.runErr.Error(), cw, "…")))
	}

	next := m.nextSteps()
	F, C := len(r.Failed), len(r.Conflicts)
	fr, cr, nr := F, C, len(next)
	boxes := func() int {
		h, n := 0, 0
		if F > 0 {
			h += fr + 1 + oh
			n++
		}
		if C > 0 {
			h += cr + oh
			n++
		}
		if nr > 0 {
			h += nr + oh
			n++
		}
		return h + max(n-1, 0)*gap
	}
	for height > 0 && len(lines)+boxes() > height { // corta primeiro os próximos passos, depois conflitos e falhas
		switch {
		case nr > 0:
			nr--
		case cr > 1:
			cr--
		case fr > 1:
			fr--
		default:
			height = 0
		}
	}
	box := func(body string, o BoxOpts) {
		lines = append(lines, strings.Split(m.th.Box(body, cw, o), "\n")...)
	}
	if F > 0 {
		top := window(F, min(m.sum.cursor, F-1), fr)
		var rows []string
		for i := top; i < top+fr; i++ {
			id := r.Failed[i]
			rows = append(rows, m.sumRow(m.th.StepIcon(state.StepFailed), m.failName(id), m.failReason(id), m.sum.cursor == i, in))
		}
		hints := " " + m.keyHint(m.keys.Log.Help().Key, m.keys.Log.Help().Desc)
		if m.canRetry() {
			hints += "   " + m.keyHint(m.keys.Retry.Help().Key, m.keys.Retry.Help().Desc)
		}
		rows = append(rows, ansi.Truncate(hints, in, "…"))
		box(strings.Join(rows, "\n"), BoxOpts{Title: i18n.T("tui.summary.failed_box"), Fill: colCoral, Fg: colInkStrong, Flush: true})
		blank()
	}
	if C > 0 {
		top := window(C, m.sum.cursor-F, cr)
		var rows []string
		for i := top; i < top+cr; i++ {
			c := r.Conflicts[i]
			rest := ""
			switch {
			case m.sum.forcing && m.cursorTarget() == c.Target:
				rest = i18n.T("tui.summary.forcing")
			case m.sum.outcome[c.Target] != "":
				rest = m.sum.outcome[c.Target]
			}
			rows = append(rows, m.sumRow("", shorten(c.Target, max(in-4-lipgloss.Width(rest), 8)), rest, m.sum.cursor == F+i, in))
		}
		box(strings.Join(rows, "\n"), BoxOpts{Title: i18n.T("tui.summary.conflicts"), Flush: true})
		blank()
	}
	if nr > 0 {
		var rows []string
		for i, n := range next[:nr] {
			rows = append(rows, ansi.Truncate(m.th.Ink(colInkStrong).Bold(true).Render(strconv.Itoa(i+1))+"  "+n, max(in-2, 1), "…"))
		}
		box(strings.Join(rows, "\n"), BoxOpts{Title: i18n.T("tui.summary.next")})
	}
	return lines
}

// cursorTarget devolve o caminho do conflito sob o cursor (vazio se o cursor está numa falha).
func (m Model) cursorTarget() string {
	if row, ok := m.summaryRow(); ok && !row.failed {
		return m.runRep.Conflicts[row.idx].Target
	}
	return ""
}

// panelHeight é a altura do painel do resumo, nunca menor que 1: com altura negativa o viewport
// deixa o YOffset passar do fim do conteúdo e o View seguinte entra em pânico.
func (m Model) panelHeight() int { return max(m.bodyHeight()-1, 1) }

func (m Model) layoutSummary() Model {
	m.sum.vp.Width, m.sum.vp.Height = m.contentWidth(), m.panelHeight()
	m.sum.vp.SetYOffset(m.sum.vp.YOffset) // reajusta o deslocamento à nova altura
	return m
}

func (m Model) openPanel(p summaryPanel, title, content string) Model {
	m.sum.panel, m.sum.title = p, title
	m.sum.vp = viewport.New(m.contentWidth(), m.panelHeight())
	m.sum.vp.SetContent(content)
	if p == panelLog {
		m.sum.vp.GotoBottom() // o fim do log é o que interessa
	}
	return m
}

// tailLog devolve as últimas n linhas do arquivo.
func tailLog(path string, n int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = cleanLine(l)
	}
	return strings.Join(lines, "\n"), nil
}

func (m Model) openLog(stepID string) Model {
	path := m.failLog(stepID)
	text, err := tailLog(path, logTail)
	if err != nil {
		return m.openPanel(panelLog, path, i18n.T("tui.summary.log_error", path, err.Error()))
	}
	return m.openPanel(panelLog, i18n.T("tui.summary.log_title", path, logTail), diffContent(text, m.contentWidth()))
}

// forcePlan acha a config do catálogo dona do arquivo e monta um plano de um passo que reaplica só esse arquivo
// (os irmãos da config ficam de fora: o usuário não os viu nem confirmou).
func (m Model) forcePlan(target string) (*plan.Plan, bool) {
	ids := make([]string, 0, len(m.s.Cat.Items))
	for id := range m.s.Cat.Items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, itemID := range ids {
		for _, cfg := range m.s.Cat.Items[itemID].Configs {
			for _, f := range cfg.Files {
				if f.Dir || m.s.ExpandTarget(f.Target) != target {
					continue
				}
				st := plan.Step{ID: "config:" + cfg.ID, ItemID: itemID, Kind: plan.KindConfig,
					Files: []plan.FileChange{{Target: f.Target, Template: f.Template, ItemID: itemID}}}
				return &plan.Plan{Steps: []plan.Step{st}}, true
			}
		}
	}
	return nil, false
}

func (m Model) forceCmd(target string) tea.Cmd {
	pl, _ := m.forcePlan(target)
	s := m.s
	return func() tea.Msg {
		rep, _, err := s.Run(context.Background(), pl, nil, app.RunOptions{ForceConfigs: true})
		return forceDoneMsg{target: target, rep: rep, err: err}
	}
}

func (m Model) onForceDone(msg forceDoneMsg) Model {
	m.sum.forcing = false
	if m.sum.outcome == nil {
		m.sum.outcome = map[string]string{}
	}
	switch {
	case msg.err != nil:
		m.sum.outcome[msg.target] = i18n.T("tui.summary.force_failed", msg.err.Error())
	case len(msg.rep.Failed) > 0:
		m.sum.outcome[msg.target] = i18n.T("tui.summary.force_failed", msg.rep.Failed[0])
	default:
		m.sum.outcome[msg.target] = i18n.T("tui.summary.forced")
	}
	return m
}

func (m Model) updateSummary(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.sum
	if s.confirm {
		m.sum.confirm = false
		if km.String() == "y" || km.String() == "Y" {
			target := m.cursorTarget()
			if _, ok := m.forcePlan(target); !ok {
				m.sum.outcome = map[string]string{target: i18n.T("tui.summary.force_unknown")}
				return m, nil
			}
			m.sum.forcing = true
			return m, m.forceCmd(target)
		}
		return m, nil
	}
	if s.panel != panelNone {
		if key.Matches(km, m.keys.Back) {
			m.sum.panel = panelNone
			return m, nil
		}
		var c tea.Cmd
		m.sum.vp, c = s.vp.Update(km)
		return m, c
	}
	row, onRow := m.summaryRow()
	switch {
	case key.Matches(km, m.keys.Up):
		if m.sum.cursor > 0 {
			m.sum.cursor--
		}
	case key.Matches(km, m.keys.Down):
		if m.sum.cursor < len(m.summaryRows())-1 {
			m.sum.cursor++
		}
	case key.Matches(km, m.keys.Log):
		if onRow && row.failed {
			return m.openLog(m.runRep.Failed[row.idx]), nil
		}
	case key.Matches(km, m.keys.Retry):
		if sel, ok := m.retrySelection(); ok {
			nm, cmd := m.beginReviewWith(ScreenSummary, sel)
			nm.review.profile = m.sel.ToPlan().Profile // o perfil segue como metadado do run, fora do plano
			return nm, cmd
		}
	case key.Matches(km, m.keys.Enter): // sempre o painel; o log da falha abre com l
		return m.enterDashboard()
	case key.Matches(km, m.keys.Diff):
		if onRow && !row.failed {
			c := m.runRep.Conflicts[row.idx]
			return m.openPanel(panelDiff, i18n.T("tui.summary.diff_title", c.Target), diffContent(c.Diff, m.contentWidth())), nil
		}
	case key.Matches(km, m.keys.Force):
		if onRow && !row.failed && !s.forcing && m.sum.outcome[m.cursorTarget()] == "" {
			m.sum.confirm = true
		}
	}
	return m, nil
}

func (m Model) summaryBindings() []key.Binding {
	if m.sum.panel != panelNone {
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Back, m.keys.Quit}
	}
	row, onRow := m.summaryRow()
	diff, force := m.keys.Diff, m.keys.Force
	conflict := onRow && !row.failed
	diff.SetEnabled(conflict)
	force.SetEnabled(conflict)
	logKey, retry := m.keys.Log, m.keys.Retry
	logKey.SetEnabled(onRow && row.failed)
	retry.SetEnabled(m.canRetry())
	enter := m.keys.Enter
	enter.SetHelp("enter", i18n.T("tui.summary.key_dashboard"))
	return []key.Binding{m.keys.Up, m.keys.Down, enter, logKey, retry, diff, force, m.keys.Help, m.keys.Quit}
}
