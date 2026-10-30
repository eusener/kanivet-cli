package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/eusener/kanivet-cli/internal/app"
	"github.com/eusener/kanivet-cli/internal/detect"
	"github.com/eusener/kanivet-cli/internal/executor"
	"github.com/eusener/kanivet-cli/internal/i18n"
	"github.com/eusener/kanivet-cli/internal/plan"
	"github.com/eusener/kanivet-cli/internal/state"
)

// Screen identifica a tela atual da máquina de estados.
type Screen int

const (
	ScreenWelcome Screen = iota
	ScreenProfile
	ScreenCategories
	ScreenReview
	ScreenExecute
	ScreenSummary
	ScreenDashboard
)

const (
	minWidth  = 80
	minHeight = 24
)

// detectMsg entrega o resultado da detecção inicial.
type detectMsg struct{ statuses map[string]detect.ItemStatus }

// Model é o modelo raiz: escolhe a tela e delega teclas e desenho a ela.
type Model struct {
	s      *app.Session
	env    map[string]string
	th     Theme
	keys   KeyMap
	help   help.Model
	spin   spinner.Model
	screen Screen
	w, h   int

	detecting   bool
	statuses    map[string]detect.ItemStatus
	sel         *Selection
	profCursor  int
	page        int
	list        ItemList
	showHelp    bool
	confirmQuit bool
	review      reviewState
	planSeq     int        // contador dos pedidos de plano (planMsg.req)
	plan        *plan.Plan // plano confirmado na revisão
	exec        execState
	sum         summaryState
	dash        dashState
	prog        *programRef // ligado ao *tea.Program pelo SetProgram

	// resultado do run, para o resumo
	runRep executor.Report
	runJ   *state.Journal
	runErr error
}

// New cria o modelo. start diz a tela inicial (o wizard começa em ScreenWelcome).
func New(s *app.Session, env map[string]string, start Screen) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Line
	return Model{
		s: s, env: env, th: NewTheme(env, false), keys: DefaultKeys(),
		help: help.New(), spin: sp, screen: start, w: minWidth, h: minHeight, detecting: true,
		prog: &programRef{},
	}
}

// Init dispara a detecção e o spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.detectCmd())
}

func (m Model) detectCmd() tea.Cmd {
	s := m.s
	return func() tea.Msg {
		return detectMsg{statuses: s.Statuses(context.Background(), plan.Selection{})}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.help.Width = msg.Width
		if m.screen == ScreenReview {
			m = m.layoutReview()
		}
		if m.screen == ScreenSummary {
			m = m.layoutSummary()
		}
		return m, nil
	case detectMsg:
		m.detecting = false
		m.statuses = msg.statuses
		m.th = NewTheme(m.env, HasNerdFont(msg.statuses))
		mode := ModeWizard
		if m.screen == ScreenDashboard {
			mode = ModeDashboard
		}
		m.sel = NewSelection(m.s.Cat, m.s.P, msg.statuses, mode)
		if mode == ModeDashboard {
			m.sel.at = func(id string) (time.Time, bool) {
				it, ok := m.s.St.Item(id)
				return it.At, ok
			}
			return m.initDashboard()
		}
		return m, nil
	case spinner.TickMsg:
		if !m.detecting && !m.dash.checking && !(m.screen == ScreenExecute && m.exec.running) {
			return m, nil
		}
		var c tea.Cmd
		m.spin, c = m.spin.Update(msg)
		return m, c
	case updatesMsg:
		return m.onUpdates(msg), nil
	case planMsg:
		return m.onPlan(msg), nil
	case eventMsg:
		return m.onEvent(msg)
	case eventsClosedMsg:
		done := m.exec.done
		return m, func() tea.Msg { return <-done }
	case interruptMsg:
		return m.interrupt(), nil
	case runDoneMsg:
		return m.onRunDone(msg)
	case forceDoneMsg:
		return m.onForceDone(msg), nil
	case interactiveReq:
		return m.onInteractive(msg)
	case interactiveDoneMsg:
		m.exec.busy = false
		// o ReleaseTerminal do ExecProcess desliga o mouse e o RestoreTerminal não o religa
		nm, c := m.nextInteractive()
		return nm, tea.Batch(tea.EnableMouseCellMotion, c)
	case tea.MouseMsg:
		if m.screen == ScreenSummary && m.sum.panel != panelNone {
			var c tea.Cmd
			m.sum.vp, c = m.sum.vp.Update(msg)
			return m, c
		}
		if m.screen == ScreenReview {
			var c tea.Cmd
			if m.review.diffOpen {
				m.review.diff, c = m.review.diff.Update(msg)
			} else {
				m.review.vp, c = m.review.vp.Update(msg)
			}
			return m, c
		}
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m Model) updateKey(km tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.screen == ScreenExecute { // q/ajuda não saem no meio do run: só ctrl+c interrompe
		return m.updateExecute(km)
	}
	if km.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.screen == ScreenSummary && m.sum.confirm { // y/N da reaplicação consome a tecla
		return m.updateSummary(km)
	}
	if m.confirmQuit {
		if km.String() == "q" {
			return m, tea.Quit
		}
		m.confirmQuit = false
		return m, nil
	}
	if m.showHelp {
		m.showHelp = false // a tecla que fecha a ajuda é consumida
		return m, nil
	}
	typing := (m.screen == ScreenCategories && m.list.Filtering()) || (m.screen == ScreenDashboard && m.sel != nil && m.dashList().Filtering())
	if !typing {
		switch {
		case key.Matches(km, m.keys.Help):
			m.showHelp = true
			return m, nil
		case key.Matches(km, m.keys.Quit):
			if n, c := m.pending(); n+c > 0 && m.screen != ScreenSummary { // depois do run não há nada pendente
				m.confirmQuit = true
				return m, nil
			}
			return m, tea.Quit
		}
	}
	switch m.screen {
	case ScreenWelcome:
		if key.Matches(km, m.keys.Enter) && !m.detecting {
			m.screen = ScreenProfile
		}
	case ScreenProfile:
		return m.updateProfile(km), nil
	case ScreenCategories:
		return m.updateCategories(km)
	case ScreenReview:
		return m.updateReview(km)
	case ScreenSummary:
		return m.updateSummary(km)
	case ScreenDashboard:
		return m.updateDashboard(km)
	}
	return m, nil
}

// headerHeight é a altura do cabeçalho (caixa de 1 linha com a sombra); no modo plain
// sem sombra sobra uma linha em branco para a altura não mudar.
const headerHeight = 4

// margin é a margem lateral (colunas) em cada lado de toda a tela.
const margin = 2

// contentWidth é a largura que cabeçalho, corpo e rodapé podem usar.
func (m Model) contentWidth() int { return max(m.w-2*margin, 1) }

// respiro devolve quantas linhas em branco separam cabeçalho, título e corpo. Em
// alturas curtas elas somem primeiro (primeiro corte da spec).
func (m Model) respiro() int {
	if m.h >= 28 {
		return 2
	}
	return 0
}

// bodyHeight é a altura que sobra ao corpo: tudo menos cabeçalho, título, respiro,
// e rodapé.
func (m Model) bodyHeight() int {
	return max(m.h-headerHeight-1-m.respiro()-1, 1)
}

// View desenha cabeçalho, título, corpo e rodapé (com contadores em tags) numa grade do
// tamanho exato do terminal; toda saída passa pelo canvas.
func (m Model) View() string {
	if m.w < minWidth || m.h < minHeight {
		msg := i18n.T("tui.too_small", m.w, m.h, minWidth, minHeight)
		return m.th.Canvas(strings.Split(ansi.Wrap(msg, max(m.w, 1), ""), "\n"), m.w, m.h)
	}
	bh := m.bodyHeight()
	// nas telas de itens e de revisão o título (com as tags) é a primeira linha do próprio corpo
	ownTitle := (m.screen == ScreenCategories || m.screen == ScreenReview || m.screen == ScreenExecute || (m.screen == ScreenSummary && m.sum.panel == panelNone) || m.screen == ScreenDashboard) && !m.showHelp && !m.confirmQuit
	if ownTitle {
		bh += 1 + m.respiro()/2
	}
	body := m.bodyLines(bh)
	for len(body) < bh {
		body = append(body, "")
	}
	lines := m.header()
	gap := make([]string, m.respiro()/2)
	lines = append(lines, gap...)
	if !ownTitle {
		lines = append(lines, m.th.Title.Render(m.upperTitle()))
		lines = append(lines, gap...)
	}
	lines = append(lines, body[:bh]...)
	lines = append(lines, m.footer())
	var framed []string
	pad := strings.Repeat(" ", margin)
	for _, l := range lines {
		for _, ln := range strings.Split(l, "\n") {
			framed = append(framed, pad+ln)
		}
	}
	return m.th.Canvas(framed, m.w, m.h)
}

// upperTitle põe o título em maiúsculas, menos o ID do run da retomada.
func (m Model) upperTitle() string {
	t := m.title()
	if m.screen == ScreenReview && m.review.resume && m.review.journal != nil {
		id := m.review.journal.RunID
		if i := strings.Index(t, id); i >= 0 {
			return strings.ToUpper(t[:i]) + id + strings.ToUpper(t[i+len(id):])
		}
	}
	return strings.ToUpper(t)
}

// trailStep devolve o passo atual da trilha (0 a 4) e se ela está concluída.
func (m Model) trailStep() (step int, done bool) {
	switch m.screen {
	case ScreenWelcome:
		return 0, false
	case ScreenProfile:
		return 1, false
	case ScreenCategories:
		return 2, false
	case ScreenReview:
		return 3, false
	case ScreenExecute:
		return 4, false
	}
	return 4, true
}

// header desenha a caixa de marca: tag KANIVET, trilha e N/5 (ou FIM) à direita. No
// painel, o perfil e os totais ocupam o lugar da trilha.
func (m Model) header() []string {
	var mid, right string
	if m.screen == ScreenDashboard {
		mid, right = m.dashHeader(), m.dashStatus()
	} else {
		step, done := m.trailStep()
		mid = m.th.Trail(step, done)
		right = fmt.Sprintf("%d/5", step+1)
		if done {
			right = strings.ToUpper(i18n.T("tui.trail.end"))
		}
		right = m.th.Subtle.Render(right)
	}
	inner := m.contentWidth() - 4 // bordas e padding
	if m.th.Paper {
		inner-- // sombra
	}
	left := m.th.Tag("KANIVET", colSun) + "  " + mid
	if fill := inner - lipgloss.Width(left) - lipgloss.Width(right); fill > 0 {
		left += strings.Repeat(" ", fill)
	}
	if m.screen == ScreenDashboard && lipgloss.Width(left)+lipgloss.Width(right)+2 > inner {
		right = "" // sem espaço, o status da verificação cai
	}
	// se faltar largura, corta a trilha e preserva o N/5
	row := ansi.Truncate(left, max(inner-lipgloss.Width(right), 0), "") + right
	out := strings.Split(m.th.Box(row, m.contentWidth(), BoxOpts{}), "\n")
	for len(out) < headerHeight {
		out = append(out, "")
	}
	return out
}

// primaryAction devolve a ação principal da tela (tecla e rótulo do botão); ok é
// false nas telas sem ação principal ou com ela indisponível.
func (m Model) primaryAction() (key, label string, ok bool) {
	switch m.screen {
	case ScreenWelcome:
		return "enter", i18n.T("tui.primary.start"), !m.detecting
	case ScreenReview:
		r := m.review
		return "enter", i18n.T("tui.primary.install"), !(r.diffOpen || r.blocked() || r.loading || m.detecting || r.empty())
	case ScreenSummary:
		row, onRow := m.summaryRow()
		return "enter", i18n.T("tui.primary.dashboard"), m.sum.panel == panelNone && !m.sum.confirm && !(onRow && row.failed)
	}
	return "", "", false
}

// keyCap põe a tecla em maiúsculas; letras já maiúsculas (U, F) viram shift+letra
// para não colidir com a minúscula.
func (m Model) keyCap(k string) string {
	if len(k) == 1 && k[0] >= 'A' && k[0] <= 'Z' {
		return pick(m.th.Nerd, "⇧", "S-") + k
	}
	return strings.ToUpper(k)
}

// footer desenha a ação principal à esquerda e os demais atalhos em ink-muted, com a
// tecla em maiúsculas. Confirmações pendentes substituem o rodapé.
func (m Model) footer() string {
	if m.screen == ScreenSummary && m.sum.confirm {
		// o alvo encolhe para o [y/N] nunca ser cortado
		room := m.contentWidth() - lipgloss.Width(i18n.T("tui.summary.force_confirm", ""))
		return m.th.Warn.Render(i18n.T("tui.summary.force_confirm", shorten(m.cursorTarget(), max(min(room, 40), 8))))
	}
	var head []string
	k, label, hasPrimary := m.primaryAction()
	if hasPrimary {
		head = append(head, m.th.Button(k, label))
	}
	head = append(head, m.counterTags()...)
	type hint struct {
		text string
		keep bool
	}
	var hints []hint
	for _, b := range m.bindings() {
		if !b.Enabled() || (hasPrimary && b.Help().Key == k) {
			continue
		}
		hints = append(hints, hint{m.th.Help.Render(m.keyCap(b.Help().Key) + " " + b.Help().Desc), b.Help().Key == "?" || b.Help().Key == "q"})
	}
	join := func() string {
		parts := append([]string{}, head...)
		for _, h := range hints {
			parts = append(parts, h.text)
		}
		return strings.Join(parts, "  ")
	}
	// sem largura, descarta os atalhos menos importantes; ajuda e sair ficam
	for lipgloss.Width(join()) > m.contentWidth() {
		dropped := false
		for i, h := range hints {
			if !h.keep {
				hints = append(hints[:i], hints[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			break
		}
	}
	return ansi.Truncate(join(), m.contentWidth(), "")
}

// counterTags devolve os contadores da tela como tags para o rodapé.
func (m Model) counterTags() []string {
	switch {
	case m.screen == ScreenExecute:
		return nil // o progresso fica no título da tela
	case m.screen == ScreenSummary || m.sel == nil:
		return nil
	case m.screen == ScreenDashboard:
		i, u, r := m.sel.Pending()
		return []string{
			m.th.Tag(i18n.T("tui.tag.install", i), colSky),
			m.th.Tag(i18n.T("tui.tag.update", u), colPeach),
			m.th.Tag(i18n.T("tui.tag.remove", r), colCoral),
		}
	}
	n, c := m.pending()
	return []string{m.th.Tag(i18n.T("tui.tag.install", n), colSky), m.th.Tag(i18n.T("tui.tag.configs", c), colMint)}
}

// pending devolve o que a barra mostra (itens a instalar, configs ligadas); o aviso ao sair usa o mesmo critério.
func (m Model) pending() (install, configs int) {
	if m.sel == nil {
		return 0, 0
	}
	install, update, remove := m.sel.Pending()
	if m.screen == ScreenDashboard { // no dashboard tudo que a revisão mostraria conta
		return install + update + remove, 0
	}
	return install, m.sel.ConfigsOn()
}

func (m Model) title() string {
	switch m.screen {
	case ScreenWelcome:
		return i18n.T("tui.step.welcome")
	case ScreenProfile:
		return i18n.T("tui.step.profile")
	case ScreenCategories:
		return i18n.T("tui.title.page", pageTitle(m.page), m.page+1, len(categoryPages))
	case ScreenReview:
		if m.review.resume && m.review.journal != nil {
			return i18n.T("tui.resume_title", m.review.journal.RunID)
		}
		return i18n.T("tui.step.review")
	case ScreenExecute:
		return i18n.T("tui.step.execute")
	case ScreenSummary:
		return i18n.T("tui.step.summary")
	case ScreenDashboard:
		return i18n.T("tui.step.dashboard")
	}
	return "kanivet-cli"
}

func (m Model) bodyLines(height int) []string {
	if m.confirmQuit {
		return m.overlay(i18n.T("tui.quit_confirm"), i18n.T("tui.dialog.quit_title"))
	}
	if m.showHelp {
		var rows []string
		for _, b := range m.bindings() {
			rows = append(rows, m.th.Ink(colInkStrong).Bold(true).Render(padTo(m.keyCap(b.Help().Key), 10))+" "+b.Help().Desc)
		}
		return m.overlay(strings.Join(rows, "\n"), i18n.T("tui.help.title"))
	}
	switch m.screen {
	case ScreenWelcome:
		return m.welcomeLines()
	case ScreenProfile:
		return m.profileLines(height)
	case ScreenCategories:
		l := m.list
		l.gap = m.respiro() / 2
		return strings.Split(l.View(m.contentWidth(), height), "\n")
	case ScreenReview:
		return m.reviewLines(height)
	case ScreenExecute:
		return m.executeLines(height)
	case ScreenSummary:
		return m.summaryLines(height)
	case ScreenDashboard:
		return m.dashLines(height)
	}
	return nil
}

// bindings devolve os atalhos da tela atual (o Enter some enquanto detecta).
func (m Model) bindings() []key.Binding {
	enter := m.keys.Enter
	enter.SetEnabled(!m.detecting)
	switch m.screen {
	case ScreenWelcome:
		return []key.Binding{enter, m.keys.Help, m.keys.Quit}
	case ScreenProfile:
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Left, m.keys.Right, enter, m.keys.Help, m.keys.Quit}
	case ScreenCategories:
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Toggle, m.keys.Left, m.keys.Search, enter, m.keys.Back, m.keys.Help, m.keys.Quit}
	case ScreenReview:
		return m.reviewBindings()
	case ScreenExecute:
		return []key.Binding{m.keys.Up, m.keys.Down, m.keys.Follow, m.keys.Interrupt}
	case ScreenSummary:
		return m.summaryBindings()
	case ScreenDashboard:
		return m.dashBindings()
	}
	return []key.Binding{m.keys.Back, m.keys.Quit}
}

// dialogWidth é a largura total das caixas centralizadas (ajuda, confirmação).
func (m Model) dialogWidth() int { return min(m.contentWidth(), 78) }

// centerBox desenha body numa caixa de largura dw centralizada na horizontal e, na
// vertical, numa área de height linhas da qual skip já foram usadas. Devolve o espaço
// acima da caixa e as linhas da caixa com o recuo à esquerda.
func (m Model) centerBox(body, title string, dw, height, skip int, flush bool) []string {
	dialog := strings.Split(m.th.Box(body, dw, BoxOpts{Title: title, Flush: flush}), "\n")
	top := max((height-skip-len(dialog))/2, 0)
	out := make([]string, top, top+len(dialog))
	left := m.th.S().Render(strings.Repeat(" ", max((m.contentWidth()-dw)/2, 0)))
	for _, l := range dialog {
		out = append(out, left+l)
	}
	return out
}

// overlay devolve o corpo da tela ocupado por uma caixa centralizada com o título
// dado; cabeçalho e rodapé continuam os da tela. O que não cabe é cortado com
// reticências.
func (m Model) overlay(body string, title string) []string {
	h := m.bodyHeight()
	dw := m.dialogWidth()
	maxRows := max(h-m.th.BoxOverhead(), 1)
	var rows []string
	for _, l := range strings.Split(body, "\n") {
		rows = append(rows, ansi.Truncate(l, m.th.BoxInner(dw)-2, "…"))
	}
	if len(rows) > maxRows {
		rows = append(rows[:maxRows-1], "…")
	}
	return m.centerBox(strings.Join(rows, "\n"), title, dw, h, 0, false)
}
