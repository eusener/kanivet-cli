package tui

import (
	"context"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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

func (m Model) summaryLines(height int) []string {
	if m.sum.panel != panelNone {
		head := m.th.Title.Render(m.sum.title)
		return append([]string{ansi.Truncate(head, m.w, "…")}, strings.Split(m.sum.vp.View(), "\n")...)
	}
	r := m.runRep
	var lines []string
	cursorLine := 0
	add := func(s string) { lines = append(lines, ansi.Truncate(s, m.w, "…")) }
	add(m.th.Accent.Render(i18n.T("summary.counts", len(r.Done), len(r.Failed), len(r.Skipped))))
	if m.runErr != nil {
		add(m.th.Danger.Render(m.runErr.Error()))
	}
	row := 0
	mark := func() string {
		defer func() { row++ }()
		if row == m.sum.cursor {
			cursorLine = len(lines)
			return "> "
		}
		return "  "
	}
	if len(r.Failed) > 0 {
		add("")
		add(m.th.Title.Render(i18n.T("tui.summary.failures")))
		for _, id := range r.Failed {
			p := mark()
			add(p + m.th.Danger.Render(m.th.StepIcon(state.StepFailed)) + " " + id)
			add("      " + m.th.Subtle.Render(i18n.T("summary.log", shorten(m.failLog(id), m.w-14))))
		}
		add(m.th.Warn.Render(i18n.T("summary.retry")))
	}
	if len(r.Conflicts) > 0 {
		add("")
		add(m.th.Title.Render(i18n.T("tui.summary.conflicts")))
		for _, c := range r.Conflicts {
			p := mark()
			text := p + shorten(c.Target, m.w-24)
			switch {
			case m.sum.forcing && m.cursorTarget() == c.Target:
				text += " " + m.th.Subtle.Render(i18n.T("tui.summary.forcing"))
			case m.sum.outcome[c.Target] != "":
				text += " " + m.th.OK.Render(m.sum.outcome[c.Target])
			}
			add(text)
		}
	}
	if next := m.nextSteps(); len(next) > 0 {
		add("")
		add(m.th.Title.Render(i18n.T("tui.summary.next")))
		for _, n := range next {
			add("  " + n)
		}
	}
	if len(lines) > height && height > 0 { // janela que mantém o cursor visível
		top := cursorLine - height/2
		if top > len(lines)-height {
			top = len(lines) - height
		}
		if top < 0 {
			top = 0
		}
		lines = lines[top : top+height]
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

func (m Model) layoutSummary() Model {
	m.sum.vp.Width, m.sum.vp.Height = m.w, m.h-4
	return m
}

func (m Model) openPanel(p summaryPanel, title, content string) Model {
	m.sum.panel, m.sum.title = p, title
	m.sum.vp = viewport.New(m.w, m.h-4)
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
	return m.openPanel(panelLog, i18n.T("tui.summary.log_title", path, logTail), diffContent(text, m.w))
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
	case key.Matches(km, m.keys.Enter):
		if onRow && row.failed {
			return m.openLog(m.runRep.Failed[row.idx]), nil
		}
		m.screen = ScreenDashboard
	case key.Matches(km, m.keys.Diff):
		if onRow && !row.failed {
			c := m.runRep.Conflicts[row.idx]
			return m.openPanel(panelDiff, i18n.T("tui.summary.diff_title", c.Target), diffContent(c.Diff, m.w)), nil
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
	enter := m.keys.Enter
	if onRow && row.failed {
		enter.SetHelp("enter", i18n.T("tui.summary.key_log"))
	} else {
		enter.SetHelp("enter", i18n.T("tui.summary.key_dashboard"))
	}
	return []key.Binding{m.keys.Up, m.keys.Down, enter, diff, force, m.keys.Help, m.keys.Quit}
}
